package api

import (
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

type RouteRequest struct {
	Start              geo.Point         `json:"start"`
	Dest               geo.Point         `json:"dest"`
	StartTime          *time.Time        `json:"start_time,omitempty"`
	BoatPreset         string            `json:"boat_preset,omitempty"`
	Model              string            `json:"model,omitempty"` // "all", "gfs_0p25", "ifs_0p25", "icon_global"
	TimeStepHours      float64           `json:"time_step_hours,omitempty"`
	TackPenaltyMinutes *float64          `json:"tack_penalty_minutes,omitempty"`
	GybePenaltyMinutes *float64          `json:"gybe_penalty_minutes,omitempty"`
	CustomBoat         interface{}       `json:"custom_boat,omitempty"`
	CustomPolar        *polar.PolarTable `json:"custom_polar,omitempty"`
}

type MultiModelRouteResult struct {
	ActiveModel string                            `json:"active_model"`
	Routes      map[string]*isochrone.RouteResult `json:"routes"`
	*isochrone.RouteResult
}

type WeatherGridRequest struct {
	Model   string     `json:"model,omitempty"` // "gfs_0p25", "ifs_0p25", "icon_global"
	MinLat  float64    `json:"min_lat"`
	MaxLat  float64    `json:"max_lat"`
	MinLon  float64    `json:"min_lon"`
	MaxLon  float64    `json:"max_lon"`
	LatStep float64    `json:"lat_step,omitempty"`
	LonStep float64    `json:"lon_step,omitempty"`
	Time    *time.Time `json:"time,omitempty"`
}

type WeatherGridResponse struct {
	Model   string                    `json:"model"`
	Time    time.Time                 `json:"time"`
	MinLat  float64                   `json:"min_lat"`
	MaxLat  float64                   `json:"max_lat"`
	MinLon  float64                   `json:"min_lon"`
	MaxLon  float64                   `json:"max_lon"`
	LatStep float64                   `json:"lat_step"`
	LonStep float64                   `json:"lon_step"`
	Grid    [][]weather.WindCondition `json:"grid"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type LandmaskResponse struct {
	Polygons []landmask.Polygon `json:"polygons"`
}
