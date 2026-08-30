package weather

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// LiveNOAAGFSEngine fetches, caches, and provides real operational NOAA GFS 0.25° weather forecast grids.
type LiveNOAAGFSEngine struct {
	mu           sync.RWMutex
	grids        map[string]*cachedGrid
	fallback     *GFSWeatherEngine
	httpClient   *http.Client
	forecastDays int
	apiBaseURL   string
}

type cachedGrid struct {
	grid      *WeatherGrid
	fetchedAt time.Time
}

// OpenMeteoGFSResponse models the batch JSON returned by the Open-Meteo NOAA GFS service.
type OpenMeteoGFSResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Hourly    struct {
		Time             []string  `json:"time"`
		WindSpeed10m     []float64 `json:"wind_speed_10m"`     // [knots]
		WindDirection10m []float64 `json:"wind_direction_10m"` // [degrees, from]
	} `json:"hourly"`
}

// NewLiveNOAAGFSEngine creates a new live NOAA GFS provider with fallback to realistic physics.
func NewLiveNOAAGFSEngine(startTime time.Time) *LiveNOAAGFSEngine {
	baseURL := strings.TrimRight(strings.TrimSpace(getEnv("METEO_SERVICE_URL", "https://api.open-meteo.com")), "/")

	return &LiveNOAAGFSEngine{
		grids:    make(map[string]*cachedGrid),
		fallback: NewRealisticGFSEngine(startTime),
		httpClient: &http.Client{
			Timeout: 4 * time.Second,
		},
		forecastDays: 16,
		apiBaseURL:   baseURL,
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// FetchRegion downloads and builds a 4D WeatherGrid from live NOAA GFS data for the specified bounding box.
func (e *LiveNOAAGFSEngine) FetchRegion(minLat, maxLat, minLon, maxLon float64, latStep, lonStep float64) (*WeatherGrid, error) {
	e.mu.RLock()
	// Check if any existing cached grid covers this region and is fresh (< 6 hours)
	for _, cg := range e.grids {
		if time.Since(cg.fetchedAt) < 6*time.Hour {
			g := cg.grid
			if minLat >= g.MinLat-0.5 && maxLat <= g.MaxLat+0.5 && minLon >= g.MinLon-0.5 && maxLon <= g.MaxLon+0.5 {
				e.mu.RUnlock()
				return g, nil
			}
		}
	}
	e.mu.RUnlock()

	// Normalize resolution to ensure fast single-request ocean fetching (target ~80-150 points total)
	latSpan := maxLat - minLat
	lonSpan := maxLon - minLon

	if latSpan > 15.0 {
		latStep = math.Max(latStep, 2.0)
	}
	if lonSpan > 20.0 {
		lonStep = math.Max(lonStep, 2.5)
	}
	if latStep <= 0 {
		latStep = 2.0
	}
	if lonStep <= 0 {
		lonStep = 2.5
	}

	nLat := int(math.Round(latSpan/latStep)) + 1
	nLon := int(math.Round(lonSpan/lonStep)) + 1

	if nLat <= 0 || nLon <= 0 {
		return nil, fmt.Errorf("invalid grid dimensions (%d x %d)", nLat, nLon)
	}

	var lats []string
	var lons []string
	type ptIndex struct {
		i, j int
	}
	indexMap := make([]ptIndex, 0, nLat*nLon)

	for i := 0; i < nLat; i++ {
		lat := minLat + float64(i)*latStep
		for j := 0; j < nLon; j++ {
			lon := minLon + float64(j)*lonStep
			lats = append(lats, fmt.Sprintf("%.2f", lat))
			lons = append(lons, fmt.Sprintf("%.2f", lon))
			indexMap = append(indexMap, ptIndex{i: i, j: j})
		}
	}

	apiURL := fmt.Sprintf(
		"%s/v1/gfs?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
		e.apiBaseURL,
		strings.Join(lats, ","),
		strings.Join(lons, ","),
		e.forecastDays,
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "SailboatWeatherRouter/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("Live NOAA GFS HTTP query failed for [%.1f..%.1f Lat, %.1f..%.1f Lon]: %v. Using fallback.", minLat, maxLat, minLon, maxLon, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("NOAA GFS API status %d: %s", resp.StatusCode, string(body))
		log.Printf("Live NOAA GFS API returned error: %v. Using fallback.", err)
		return nil, err
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var rawPoints []OpenMeteoGFSResponse
	if strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "[") {
		if err := json.Unmarshal(bodyBytes, &rawPoints); err != nil {
			return nil, fmt.Errorf("failed to parse JSON array: %w", err)
		}
	} else {
		var single OpenMeteoGFSResponse
		if err := json.Unmarshal(bodyBytes, &single); err != nil {
			return nil, fmt.Errorf("failed to parse single JSON: %w", err)
		}
		rawPoints = []OpenMeteoGFSResponse{single}
	}

	if len(rawPoints) == 0 || len(rawPoints[0].Hourly.Time) == 0 {
		return nil, fmt.Errorf("no forecast points returned from NOAA GFS")
	}

	// Parse timestamps
	nTime := len(rawPoints[0].Hourly.Time)
	timestamps := make([]time.Time, nTime)
	for tIdx, timeStr := range rawPoints[0].Hourly.Time {
		tParsed, err := time.Parse("2006-01-02T15:04", timeStr)
		if err != nil {
			tParsed = time.Now().UTC().Add(time.Duration(tIdx) * time.Hour)
		}
		timestamps[tIdx] = tParsed.UTC()
	}

	// Initialize 4D WeatherGrid
	grid := NewWeatherGrid(minLat, maxLat, latStep, minLon, maxLon, lonStep, timestamps)

	// Populate U and V tensors
	for ptIdx, item := range rawPoints {
		if ptIdx >= len(indexMap) {
			break
		}
		i := indexMap[ptIdx].i
		j := indexMap[ptIdx].j

		for tIdx := 0; tIdx < nTime; tIdx++ {
			spdKts := 0.0
			dirDeg := 0.0
			if tIdx < len(item.Hourly.WindSpeed10m) {
				spdKts = item.Hourly.WindSpeed10m[tIdx]
			}
			if tIdx < len(item.Hourly.WindDirection10m) {
				dirDeg = item.Hourly.WindDirection10m[tIdx]
			}

			// Convert to eastward (U) and northward (V) velocities [m/s]
			spdMS := spdKts * KnotsToMS
			dirRad := dirDeg * math.Pi / 180.0
			u := -spdMS * math.Sin(dirRad)
			v := -spdMS * math.Cos(dirRad)

			grid.UData[tIdx][i][j] = u
			grid.VData[tIdx][i][j] = v
		}
	}

	cacheKey := fmt.Sprintf("%.1f_%.1f_%.1f_%.1f", minLat, maxLat, minLon, maxLon)
	e.mu.Lock()
	e.grids[cacheKey] = &cachedGrid{
		grid:      grid,
		fetchedAt: time.Now().UTC(),
	}
	e.mu.Unlock()

	log.Printf("Successfully loaded live NOAA GFS grid [%.1f..%.1f Lat, %.1f..%.1f Lon, %d time steps across %d points in 1 request]", minLat, maxLat, minLon, maxLon, nTime, len(rawPoints))
	return grid, nil
}

// GetWind samples wind condition at arbitrary latitude, longitude, and time.
func (e *LiveNOAAGFSEngine) GetWind(lat, lon float64, t time.Time) WindCondition {
	e.mu.RLock()
	for _, cg := range e.grids {
		g := cg.grid
		if lat >= g.MinLat && lat <= g.MaxLat && lon >= g.MinLon && lon <= g.MaxLon {
			res := g.Interpolate(lat, lon, t)
			e.mu.RUnlock()
			return res
		}
	}
	e.mu.RUnlock()

	// Fall back to physics-based meteorological simulation
	return e.fallback.GetWind(lat, lon, t)
}

// GetGrid extracts a 2D wind slice across a lat/lon domain at time t.
func (e *LiveNOAAGFSEngine) GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep float64, t time.Time) [][]WindCondition {
	grid, err := e.FetchRegion(minLat, maxLat, minLon, maxLon, latStep, lonStep)
	if err == nil && grid != nil {
		nLat := int(math.Round((maxLat-minLat)/latStep)) + 1
		nLon := int(math.Round((maxLon-minLon)/lonStep)) + 1
		res := make([][]WindCondition, nLat)
		for i := 0; i < nLat; i++ {
			lat := minLat + float64(i)*latStep
			res[i] = make([]WindCondition, nLon)
			for j := 0; j < nLon; j++ {
				lon := minLon + float64(j)*lonStep
				res[i][j] = grid.Interpolate(lat, lon, t)
			}
		}
		return res
	}

	// Fallback to simulation grid
	return e.fallback.GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep, t)
}
