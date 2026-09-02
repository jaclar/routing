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

// MultiModelWeatherProvider coordinates live weather engines across multiple meteorological models.
type MultiModelWeatherProvider struct {
	mu         sync.RWMutex
	apiBaseURL string
	startTime  time.Time
	engines    map[string]*LiveWeatherEngine
}

// NewMultiModelWeatherProvider creates a manager that instantiates and caches model-specific weather engines.
func NewMultiModelWeatherProvider(startTime time.Time) *MultiModelWeatherProvider {
	baseURL := strings.TrimRight(strings.TrimSpace(getEnv("METEO_SERVICE_URL", "https://routing.jaclar.net")), "/")

	m := &MultiModelWeatherProvider{
		apiBaseURL: baseURL,
		startTime:  startTime,
		engines:    make(map[string]*LiveWeatherEngine),
	}

	// Pre-initialize default engines
	for _, id := range AvailableModelIDs() {
		m.engines[id] = NewLiveWeatherEngine(id, startTime, baseURL)
	}

	return m
}

// GetEngine returns the LiveWeatherEngine for a canonical model ID (e.g. gfs_0p25, ifs_0p25, icon_global).
func (m *MultiModelWeatherProvider) GetEngine(modelID string) *LiveWeatherEngine {
	canonicalID := NormalizeModelID(modelID)
	if canonicalID == ModelAll || canonicalID == "" {
		canonicalID = ModelGFS025
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if eng, ok := m.engines[canonicalID]; ok {
		return eng
	}

	eng := NewLiveWeatherEngine(canonicalID, m.startTime, m.apiBaseURL)
	m.engines[canonicalID] = eng
	return eng
}

// LiveWeatherEngine fetches, caches, and interpolates 4D wind fields for a specific meteorological model.
type LiveWeatherEngine struct {
	mu           sync.RWMutex
	modelID      string
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

// OpenMeteoPointResponse models the JSON returned by Open-Meteo compatible endpoints.
type OpenMeteoPointResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Hourly    struct {
		Time             []string  `json:"time"`
		WindSpeed10m     []float64 `json:"wind_speed_10m"`     // [knots]
		WindDirection10m []float64 `json:"wind_direction_10m"` // [degrees, from]
	} `json:"hourly"`
}

// NewLiveWeatherEngine initializes a model-specific weather provider.
func NewLiveWeatherEngine(modelID string, startTime time.Time, apiBaseURL string) *LiveWeatherEngine {
	if apiBaseURL == "" {
		apiBaseURL = strings.TrimRight(strings.TrimSpace(getEnv("METEO_SERVICE_URL", "https://routing.jaclar.net")), "/")
	}

	return &LiveWeatherEngine{
		modelID:  modelID,
		grids:    make(map[string]*cachedGrid),
		fallback: NewRealisticGFSEngine(startTime),
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
		forecastDays: 16,
		apiBaseURL:   apiBaseURL,
	}
}

// NewLiveNOAAGFSEngine is retained for backward compatibility, returning the GFS 0.25 engine.
func NewLiveNOAAGFSEngine(startTime time.Time) *LiveWeatherEngine {
	return NewLiveWeatherEngine(ModelGFS025, startTime, "")
}

// LiveNOAAGFSEngine alias for backward compatibility.
type LiveNOAAGFSEngine = LiveWeatherEngine

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// ModelID returns the model identifier of this engine.
func (e *LiveWeatherEngine) ModelID() string {
	return e.modelID
}

// buildEndpoint constructs the URL path for this model.
func (e *LiveWeatherEngine) buildEndpoint(latsStr, lonsStr string) string {
	switch e.modelID {
	case ModelGFS025:
		return fmt.Sprintf("%s/v1/gfs?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, latsStr, lonsStr, e.forecastDays)
	case ModelIFS025:
		return fmt.Sprintf("%s/v1/ecmwf?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, latsStr, lonsStr, e.forecastDays)
	case ModelICONGlobal:
		return fmt.Sprintf("%s/v1/dwd-icon?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, latsStr, lonsStr, e.forecastDays)
	case ModelGEFS050:
		return fmt.Sprintf("%s/v1/gefs?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, latsStr, lonsStr, e.forecastDays)
	case ModelIFSEns025:
		return fmt.Sprintf("%s/v1/ifs-ens?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, latsStr, lonsStr, e.forecastDays)
	case ModelICONEPS:
		return fmt.Sprintf("%s/v1/icon-eps?latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, latsStr, lonsStr, e.forecastDays)
	default:
		return fmt.Sprintf("%s/v1/forecast?models=%s&latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&forecast_days=%d",
			e.apiBaseURL, e.modelID, latsStr, lonsStr, e.forecastDays)
	}
}

func (e *LiveWeatherEngine) fetchEndpointPoints(url string) ([]OpenMeteoPointResponse, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SailboatWeatherRouter/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("meteo HTTP status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawPoints []OpenMeteoPointResponse
	if strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "[") {
		if err := json.Unmarshal(bodyBytes, &rawPoints); err != nil {
			return nil, err
		}
	} else {
		var single OpenMeteoPointResponse
		if err := json.Unmarshal(bodyBytes, &single); err != nil {
			return nil, err
		}
		rawPoints = []OpenMeteoPointResponse{single}
	}
	return rawPoints, nil
}

// FetchRegion downloads and builds a 4D WeatherGrid from live weather model data for the specified bounding box.
func (e *LiveWeatherEngine) FetchRegion(minLat, maxLat, minLon, maxLon float64, latStep, lonStep float64) (*WeatherGrid, error) {
	e.mu.RLock()
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

	latsStr := strings.Join(lats, ",")
	lonsStr := strings.Join(lons, ",")
	apiURL := e.buildEndpoint(latsStr, lonsStr)

	rawPoints, err := e.fetchEndpointPoints(apiURL)
	if err != nil {
		log.Printf("[ERROR] Weather model %s fetch failed: %v", e.modelID, err)
		return nil, fmt.Errorf("weather model %s unavailable: %w", e.modelID, err)
	}

	if len(rawPoints) == 0 || len(rawPoints[0].Hourly.Time) == 0 {
		err := fmt.Errorf("no forecast points returned for model %s", e.modelID)
		log.Printf("[ERROR] %v", err)
		return nil, err
	}

	nTime := len(rawPoints[0].Hourly.Time)
	timestamps := make([]time.Time, nTime)
	for tIdx, timeStr := range rawPoints[0].Hourly.Time {
		tParsed, err := time.Parse("2006-01-02T15:04", timeStr)
		if err != nil {
			tParsed = time.Now().UTC().Add(time.Duration(tIdx) * time.Hour)
		}
		timestamps[tIdx] = tParsed.UTC()
	}

	grid := NewWeatherGrid(minLat, maxLat, latStep, minLon, maxLon, lonStep, timestamps)

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
	if len(e.grids) >= 4 {
		var oldestKey string
		var oldestTime time.Time
		for k, cg := range e.grids {
			if oldestKey == "" || cg.fetchedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = cg.fetchedAt
			}
		}
		if oldestKey != "" {
			delete(e.grids, oldestKey)
		}
	}
	e.grids[cacheKey] = &cachedGrid{
		grid:      grid,
		fetchedAt: time.Now().UTC(),
	}
	e.mu.Unlock()

	log.Printf("Successfully loaded live %s grid [%.1f..%.1f Lat, %.1f..%.1f Lon, %d time steps across %d points]",
		e.modelID, minLat, maxLat, minLon, maxLon, nTime, len(rawPoints))
	return grid, nil
}

// GetWind samples wind condition at arbitrary latitude, longitude, and time.
func (e *LiveWeatherEngine) GetWind(lat, lon float64, t time.Time) WindCondition {
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

	return e.fallback.GetWind(lat, lon, t)
}

// GetGrid extracts a 2D wind slice across a lat/lon domain at time t.
func (e *LiveWeatherEngine) GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep float64, t time.Time) [][]WindCondition {
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

	return e.fallback.GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep, t)
}
