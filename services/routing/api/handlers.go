package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaclar/routing-service/confidence"
	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

type Server struct {
	weatherProvider *weather.MultiModelWeatherProvider
	landMask        *landmask.LandMask
	vppClient       *polar.VPPClient
	confEvaluator   *confidence.Evaluator
}

func NewServer(vppBaseURL string) *Server {
	now := time.Now().UTC()
	meteoURL := strings.TrimRight(strings.TrimSpace(os.Getenv("METEO_SERVICE_URL")), "/")
	if meteoURL == "" {
		meteoURL = "https://routing.jaclar.net"
	}
	lm := landmask.NewGSHHGLandMask()
	return &Server{
		weatherProvider: weather.NewMultiModelWeatherProvider(now),
		landMask:        lm,
		vppClient:       polar.NewVPPClient(vppBaseURL),
		confEvaluator:   confidence.NewEvaluator(meteoURL, nil, lm),
	}
}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "healthy",
		Service: "routing-service",
		Version: "0.3.0",
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

	var polarTable *polar.PolarTable
	if req.CustomPolar != nil && len(req.CustomPolar.TWSList) > 0 && len(req.CustomPolar.TWAList) > 0 && len(req.CustomPolar.Speeds) > 0 {
		polarTable = req.CustomPolar
	} else {
		var err error
		polarTable, err = s.vppClient.FetchPolar(preset, req.CustomBoat)
		if err != nil || polarTable == nil {
			polarTable = polar.Get36ftKetchPolar()
		}
	}

	cfg := isochrone.DefaultRouterConfig()
	if req.TimeStepHours > 0 {
		cfg.TimeStep = time.Duration(req.TimeStepHours * float64(time.Hour))
		stepDistanceEstNM := req.TimeStepHours * 6.5
		cfg.ArrivalRadiusNM = math.Max(0.5, math.Min(stepDistanceEstNM*1.1, 12.0))
	} else {
		directDistNM := geo.DistanceNM(req.Start, req.Dest)
		if directDistNM <= 100 {
			cfg.TimeStep = 5 * time.Minute
			cfg.ArrivalRadiusNM = 0.6
		} else if directDistNM <= 250 {
			cfg.TimeStep = 15 * time.Minute
			cfg.ArrivalRadiusNM = 1.6
		} else if directDistNM <= 500 {
			cfg.TimeStep = 30 * time.Minute
			cfg.ArrivalRadiusNM = 3.5
		} else if directDistNM <= 1200 {
			cfg.TimeStep = 1 * time.Hour
			cfg.ArrivalRadiusNM = 7.0
		} else {
			cfg.TimeStep = 2 * time.Hour
			cfg.ArrivalRadiusNM = 12.0
		}
	}
	if req.TackPenaltyMinutes != nil {
		cfg.TackPenaltyMinutes = *req.TackPenaltyMinutes
	}
	if req.GybePenaltyMinutes != nil {
		cfg.GybePenaltyMinutes = *req.GybePenaltyMinutes
	}

	// Calculate region bounding box
	minLat := math.Min(req.Start.Lat, req.Dest.Lat) - 6.0
	maxLat := math.Max(req.Start.Lat, req.Dest.Lat) + 6.0
	minLon := math.Min(req.Start.Lon, req.Dest.Lon) - 6.0
	maxLon := math.Max(req.Start.Lon, req.Dest.Lon) + 6.0

	targetModels := []string{weather.ModelGFS025, weather.ModelIFS025, weather.ModelICONGlobal}
	normalizedModel := weather.NormalizeModelID(req.Model)
	if normalizedModel != weather.ModelAll && normalizedModel != "" {
		targetModels = []string{normalizedModel}
	}

	type solveResult struct {
		modelID  string
		route    *isochrone.RouteResult
		baseGrid *weather.WeatherGrid
		err      error
	}

	var wg sync.WaitGroup
	resultChan := make(chan solveResult, len(targetModels))

	for _, mID := range targetModels {
		wg.Add(1)
		go func(modelID string) {
			defer wg.Done()
			engine := s.weatherProvider.GetEngine(modelID)
			// Prefetch region data and fail explicitly if live forecast is unreachable
			baseGrid, fetchErr := engine.FetchRegion(minLat, maxLat, minLon, maxLon, 1.5, 1.5)
			if fetchErr != nil {
				log.Printf("[ERROR] Live weather fetch failed for model %s: %v", modelID, fetchErr)
				resultChan <- solveResult{modelID: modelID, route: nil, baseGrid: nil, err: fmt.Errorf("live weather fetch failed for %s: %w", modelID, fetchErr)}
				return
			}

			route, err := isochrone.CalculateOptimalRoute(
				req.Start,
				req.Dest,
				startTime,
				polarTable,
				engine,
				s.landMask,
				cfg,
			)
			resultChan <- solveResult{modelID: modelID, route: route, baseGrid: baseGrid, err: err}
		}(mID)
	}

	wg.Wait()
	close(resultChan)

	routesMap := make(map[string]*isochrone.RouteResult)
	var firstRoute *isochrone.RouteResult
	var lastErr error

	for res := range resultChan {
		if res.err != nil {
			log.Printf("[ERROR] Route calculation failed for model %s: %v", res.modelID, res.err)
			lastErr = res.err
		} else if res.route != nil {
			res.route.ModelID = res.modelID
			// Evaluate ensemble confidence with full multi-isochrone solves across all N members
			conf, confErr := s.confEvaluator.EvaluateRouteMultiIsochrone(
				r.Context(),
				res.route,
				req.Start,
				req.Dest,
				polarTable,
				res.baseGrid,
				res.modelID,
				cfg,
			)
			if confErr == nil && conf != nil {
				res.route.Confidence = conf
				for i := range res.route.Waypoints {
					if i < len(conf.Waypoints) {
						wpC := conf.Waypoints[i]
						res.route.Waypoints[i].ConfidenceScore = wpC.Score
						res.route.Waypoints[i].ConfidenceScoreA = wpC.ScoreStrategyA
						res.route.Waypoints[i].ConfidenceScoreB = wpC.ScoreStrategyB
						res.route.Waypoints[i].WindSpeedStdKts = wpC.WindSpeedStd
						res.route.Waypoints[i].WindSpeedP10Kts = wpC.WindSpeedP10
						res.route.Waypoints[i].WindSpeedP90Kts = wpC.WindSpeedP90
						res.route.Waypoints[i].WindDirSpreadDeg = wpC.WindDirSpreadDeg
						res.route.Waypoints[i].GaleProbability = wpC.GaleProbability
					}
				}
			}
			routesMap[res.modelID] = res.route
			if firstRoute == nil || res.modelID == weather.ModelGFS025 {
				firstRoute = res.route
			}
		}
	}

	if len(routesMap) == 0 {
		log.Printf("[ERROR] Routing calculation failed across all models: %v", lastErr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":  true,
			"reason": fmt.Sprintf("Routing calculation failed across all requested models: %v", lastErr),
		})
		return
	}

	activeModel := weather.ModelGFS025
	if _, ok := routesMap[activeModel]; !ok {
		for k := range routesMap {
			activeModel = k
			break
		}
	}

	multiResponse := MultiModelRouteResult{
		ActiveModel: activeModel,
		Routes:      routesMap,
		RouteResult: routesMap[activeModel],
	}

	writeJSON(w, http.StatusOK, multiResponse)
}

func (s *Server) HandleWeatherGrid(w http.ResponseWriter, r *http.Request) {
	var req WeatherGridRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if r.Method == http.MethodGet {
		req = WeatherGridRequest{
			Model:   r.URL.Query().Get("model"),
			MinLat:  20.0,
			MaxLat:  50.0,
			MinLon:  -80.0,
			MaxLon:  -40.0,
			LatStep: 1.5,
			LonStep: 1.5,
		}
		if latStr := r.URL.Query().Get("min_lat"); latStr != "" {
			if v, err := strconv.ParseFloat(latStr, 64); err == nil {
				req.MinLat = v
			}
		}
		if latStr := r.URL.Query().Get("max_lat"); latStr != "" {
			if v, err := strconv.ParseFloat(latStr, 64); err == nil {
				req.MaxLat = v
			}
		}
		if lonStr := r.URL.Query().Get("min_lon"); lonStr != "" {
			if v, err := strconv.ParseFloat(lonStr, 64); err == nil {
				req.MinLon = v
			}
		}
		if lonStr := r.URL.Query().Get("max_lon"); lonStr != "" {
			if v, err := strconv.ParseFloat(lonStr, 64); err == nil {
				req.MaxLon = v
			}
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

	canonicalModel := weather.NormalizeModelID(req.Model)
	if canonicalModel == weather.ModelAll || canonicalModel == "" {
		canonicalModel = weather.ModelGFS025
	}

	engine := s.weatherProvider.GetEngine(canonicalModel)
	grid, err := engine.GetGrid(req.MinLat, req.MaxLat, req.MinLon, req.MaxLon, req.LatStep, req.LonStep, t)
	if err != nil {
		log.Printf("[ERROR] Live weather grid failed for model %s: %v", canonicalModel, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":  true,
			"reason": fmt.Sprintf("Live weather grid unavailable: %v", err),
		})
		return
	}

	resp := WeatherGridResponse{
		Model:   canonicalModel,
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

	minLat := -90.0
	maxLat := 90.0
	minLon := -180.0
	maxLon := 180.0

	q := r.URL.Query()
	if latStr := q.Get("min_lat"); latStr != "" {
		if v, err := strconv.ParseFloat(latStr, 64); err == nil {
			minLat = v
		}
	}
	if latStr := q.Get("max_lat"); latStr != "" {
		if v, err := strconv.ParseFloat(latStr, 64); err == nil {
			maxLat = v
		}
	}
	if lonStr := q.Get("min_lon"); lonStr != "" {
		if v, err := strconv.ParseFloat(lonStr, 64); err == nil {
			minLon = v
		}
	}
	if lonStr := q.Get("max_lon"); lonStr != "" {
		if v, err := strconv.ParseFloat(lonStr, 64); err == nil {
			maxLon = v
		}
	}

	polys := s.landMask.GetPolygonsInRegion(minLat, maxLat, minLon, maxLon)
	if len(polys) > 1500 {
		polys = polys[:1500]
	}
	resp := LandmaskResponse{
		Polygons: polys,
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
