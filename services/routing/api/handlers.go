package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

type Server struct {
	weatherEngine weather.WeatherProvider
	landMask      *landmask.LandMask
	vppClient     *polar.VPPClient
}

func NewServer(vppBaseURL string) *Server {
	now := time.Now().UTC()
	return &Server{
		weatherEngine: weather.NewLiveNOAAGFSEngine(now),
		landMask:      landmask.NewGSHHGLandMask(),
		vppClient:     polar.NewVPPClient(vppBaseURL),
	}
}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "healthy",
		Service: "routing-service",
		Version: "0.2.0",
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) HandleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	startTime := time.Now().UTC()
	if req.StartTime != nil {
		startTime = *req.StartTime
	}

	preset := req.BoatPreset
	if preset == "" {
		preset = "36ft-ketch"
	}

	polarTable, err := s.vppClient.FetchPolar(preset, req.CustomBoat)
	if err != nil {
		polarTable = polar.Get36ftKetchPolar()
	}

	cfg := isochrone.DefaultRouterConfig()
	if req.TimeStepHours > 0 {
		cfg.TimeStep = time.Duration(req.TimeStepHours * float64(time.Hour))
		// Scale arrival capture radius dynamically with time step (e.g. ~0.5 NM for 5-min step)
		stepDistanceEstNM := req.TimeStepHours * 6.5
		cfg.ArrivalRadiusNM = math.Max(0.5, math.Min(stepDistanceEstNM*1.1, 12.0))
	}
	if req.TackPenaltyMinutes != nil {
		cfg.TackPenaltyMinutes = *req.TackPenaltyMinutes
	}
	if req.GybePenaltyMinutes != nil {
		cfg.GybePenaltyMinutes = *req.GybePenaltyMinutes
	}

	// Prefetch live NOAA GFS data for the route's bounding box
	minLat := math.Min(req.Start.Lat, req.Dest.Lat) - 6.0
	maxLat := math.Max(req.Start.Lat, req.Dest.Lat) + 6.0
	minLon := math.Min(req.Start.Lon, req.Dest.Lon) - 6.0
	maxLon := math.Max(req.Start.Lon, req.Dest.Lon) + 6.0

	if liveEngine, ok := s.weatherEngine.(*weather.LiveNOAAGFSEngine); ok {
		liveEngine.FetchRegion(minLat, maxLat, minLon, maxLon, 1.5, 1.5)
	}

	route, err := isochrone.CalculateOptimalRoute(
		req.Start,
		req.Dest,
		startTime,
		polarTable,
		s.weatherEngine,
		s.landMask,
		cfg,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, route)
}

func (s *Server) HandleWeatherGrid(w http.ResponseWriter, r *http.Request) {
	var req WeatherGridRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if r.Method == http.MethodGet {
		// Default Atlantic coverage
		req = WeatherGridRequest{
			MinLat:  20.0,
			MaxLat:  50.0,
			MinLon:  -80.0,
			MaxLon:  -40.0,
			LatStep: 1.5,
			LonStep: 1.5,
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if req.LatStep <= 0 {
		req.LatStep = 1.5
	}
	if req.LonStep <= 0 {
		req.LonStep = 1.5
	}

	t := time.Now().UTC()
	if req.Time != nil {
		t = *req.Time
	}

	grid := s.weatherEngine.GetGrid(req.MinLat, req.MaxLat, req.MinLon, req.MaxLon, req.LatStep, req.LonStep, t)

	resp := WeatherGridResponse{
		Time:    t,
		MinLat:  req.MinLat,
		MaxLat:  req.MaxLat,
		MinLon:  req.MinLon,
		MaxLon:  req.MaxLon,
		LatStep: req.LatStep,
		LonStep: req.LonStep,
		Grid:    grid,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) HandleLandmaskPolygons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	minLatStr := q.Get("min_lat")
	maxLatStr := q.Get("max_lat")
	minLonStr := q.Get("min_lon")
	maxLonStr := q.Get("max_lon")

	var polygons []landmask.Polygon
	if minLatStr != "" && maxLatStr != "" && minLonStr != "" && maxLonStr != "" {
		minLat, _ := strconv.ParseFloat(minLatStr, 64)
		maxLat, _ := strconv.ParseFloat(maxLatStr, 64)
		minLon, _ := strconv.ParseFloat(minLonStr, 64)
		maxLon, _ := strconv.ParseFloat(maxLonStr, 64)
		polygons = s.landMask.GetPolygonsInRegion(minLat, maxLat, minLon, maxLon)
	} else {
		// Default: Caribbean / West Indies passage region
		polygons = s.landMask.GetPolygonsInRegion(9.5, 14.0, -63.0, -59.0)
		if len(polygons) == 0 {
			polygons = s.landMask.GetPolygonsInRegion(9.0, 45.0, -80.0, -50.0)
		}
	}

	resp := LandmaskResponse{
		Polygons: polygons,
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}


