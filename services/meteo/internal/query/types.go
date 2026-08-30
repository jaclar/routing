package query

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ForecastRequest represents a parsed Open-Meteo HTTP query.
type ForecastRequest struct {
	Latitudes         []float64 `json:"latitudes"`
	Longitudes        []float64 `json:"longitudes"`
	Hourly            []string  `json:"hourly"`
	Daily             []string  `json:"daily"`
	CurrentWeather    bool      `json:"current_weather"`
	Models            []string  `json:"models"`
	WindSpeedUnit     string    `json:"wind_speed_unit"`     // "kn", "ms", "kmh", "mph"
	TemperatureUnit   string    `json:"temperature_unit"`   // "celsius", "fahrenheit"
	PrecipitationUnit string    `json:"precipitation_unit"` // "mm", "inch"
	Timezone          string    `json:"timezone"`
	ForecastDays      int       `json:"forecast_days"`
	PastDays          int       `json:"past_days"`
}

// ParseHTTPRequest parses standard Open-Meteo query parameters from an incoming HTTP request.
func ParseHTTPRequest(r *http.Request) (*ForecastRequest, error) {
	q := r.URL.Query()

	latStr := q.Get("latitude")
	lonStr := q.Get("longitude")
	if latStr == "" || lonStr == "" {
		return nil, fmt.Errorf("missing required 'latitude' or 'longitude' query parameters")
	}

	latParts := strings.Split(latStr, ",")
	lonParts := strings.Split(lonStr, ",")
	if len(latParts) != len(lonParts) {
		return nil, fmt.Errorf("mismatched number of latitudes (%d) and longitudes (%d)", len(latParts), len(lonParts))
	}

	lats := make([]float64, len(latParts))
	lons := make([]float64, len(lonParts))
	for i := range latParts {
		latVal, err := strconv.ParseFloat(strings.TrimSpace(latParts[i]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid latitude: %s", latParts[i])
		}
		lonVal, err := strconv.ParseFloat(strings.TrimSpace(lonParts[i]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid longitude: %s", lonParts[i])
		}
		lats[i] = latVal
		lons[i] = lonVal
	}

	var hourly []string
	if h := q.Get("hourly"); h != "" {
		for _, item := range strings.Split(h, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				hourly = append(hourly, trimmed)
			}
		}
	}

	var modelsList []string
	if m := q.Get("models"); m != "" {
		for _, item := range strings.Split(m, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				modelsList = append(modelsList, trimmed)
			}
		}
	}

	windUnit := strings.ToLower(q.Get("wind_speed_unit"))
	if windUnit == "" {
		windUnit = "kmh" // Standard Open-Meteo default
	}

	tempUnit := strings.ToLower(q.Get("temperature_unit"))
	if tempUnit == "" {
		tempUnit = "celsius"
	}

	precipUnit := strings.ToLower(q.Get("precipitation_unit"))
	if precipUnit == "" {
		precipUnit = "mm"
	}

	fcDays := 7
	if d := q.Get("forecast_days"); d != "" {
		if val, err := strconv.Atoi(d); err == nil && val > 0 {
			fcDays = val
		}
	}

	currentWeather := q.Get("current_weather") == "true" || q.Get("current") != ""

	return &ForecastRequest{
		Latitudes:         lats,
		Longitudes:        lons,
		Hourly:            hourly,
		CurrentWeather:    currentWeather,
		Models:            modelsList,
		WindSpeedUnit:     windUnit,
		TemperatureUnit:   tempUnit,
		PrecipitationUnit: precipUnit,
		Timezone:          q.Get("timezone"),
		ForecastDays:      fcDays,
	}, nil
}

// OpenMeteoResponse represents a single coordinate forecast output in exact Open-Meteo JSON format.
type OpenMeteoResponse struct {
	Latitude             float64           `json:"latitude"`
	Longitude            float64           `json:"longitude"`
	GenerationTimeMS     float64           `json:"generationtime_ms"`
	UTCOffsetSeconds     int               `json:"utc_offset_seconds"`
	Timezone             string            `json:"timezone"`
	TimezoneAbbreviation string            `json:"timezone_abbreviation"`
	Elevation            float64           `json:"elevation"`
	HourlyUnits          map[string]string `json:"hourly_units,omitempty"`
	Hourly               map[string]any    `json:"hourly,omitempty"`
	CurrentWeather       *CurrentWeather   `json:"current_weather,omitempty"`
}

// CurrentWeather represents real-time current condition summary.
type CurrentWeather struct {
	Time          string  `json:"time"`
	Temperature   float64 `json:"temperature"`
	WindSpeed     float64 `json:"windspeed"`
	WindDirection float64 `json:"winddirection"`
	WeatherCode   int     `json:"weathercode"`
	IsDay         int     `json:"is_day"`
}

// GridResponse represents a 2D scalar/vector bounding-box slice for high-speed routing engines.
type GridResponse struct {
	Model     string      `json:"model"`
	Cycle     time.Time   `json:"cycle"`
	ValidTime time.Time   `json:"valid_time"`
	StepHours int         `json:"step_hours"`
	MinLat    float64     `json:"min_lat"`
	MaxLat    float64     `json:"max_lat"`
	LatStep   float64     `json:"lat_step"`
	MinLon    float64     `json:"min_lon"`
	MaxLon    float64     `json:"max_lon"`
	LonStep   float64     `json:"lon_step"`
	NLats     int         `json:"nlats"`
	NLons     int         `json:"nlons"`
	UData     [][]float32 `json:"u_data"` // [nlats][nlons] in m/s
	VData     [][]float32 `json:"v_data"` // [nlats][nlons] in m/s
	MSLPData  [][]float32 `json:"mslp_data,omitempty"`
}
