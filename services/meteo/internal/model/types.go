package model

import (
	"math"
	"time"
)

// Standard Canonical Variable Names
const (
	VarWindU10m      = "wind_u_10m"      // U-component of wind at 10m [m/s]
	VarWindV10m      = "wind_v_10m"      // V-component of wind at 10m [m/s]
	VarWindGust10m   = "wind_gust_10m"   // Wind gust speed at 10m [m/s]
	VarMSLP          = "mslp"            // Mean sea level pressure [Pa]
	VarTemp2m        = "temp_2m"          // Temperature at 2m [K]
	VarPrecipAccum   = "precip_accum"    // Total precipitation accumulation [kg/m^2 = mm]
	VarWaveHeightSig = "wave_height_sig" // Significant wave height [m]
	VarWaveDirPrim   = "wave_dir_primary"// Primary wave direction [deg]
	VarWavePeriodPeak= "wave_period_peak"// Peak wave period [s]
)

// Standard Open-Meteo Variable Names
const (
	OMWindSpeed10m     = "wind_speed_10m"
	OMWindDirection10m = "wind_direction_10m"
	OMWindGusts10m     = "wind_gusts_10m"
	OMPressureMSL      = "pressure_msl"
	OMSurfacePressure  = "surface_pressure"
	OMTemperature2m    = "temperature_2m"
	OMPrecipitation    = "precipitation"
	OMWaveHeight       = "wave_height"
	OMWaveDirection    = "wave_direction"
	OMWavePeriod       = "wave_period"
)

// Supported Model IDs
const (
	ModelGFS025    = "gfs_0p25"
	ModelIFS025    = "ifs_0p25"
	ModelAIFS025   = "aifs_0p25"
	ModelICON025   = "icon_global"
)

// ModelCycle represents a specific forecast reference cycle run (e.g. 2026-08-30 06:00 UTC).
type ModelCycle struct {
	ModelName     string    `json:"model_name"`
	ReferenceTime time.Time `json:"reference_time"`
	ResolutionDeg float64   `json:"resolution_deg"` // e.g. 0.25
	ForecastSteps []int     `json:"forecast_steps"` // e.g. [0, 1, 2, ... 240]
}

// FetchTask describes a slice of data to be downloaded and ingested.
type FetchTask struct {
	ModelName   string
	Cycle       time.Time
	StepHours   int
	Variable    string
	SourceURL   string
	ByteStart   int64
	ByteEnd     int64
	ExtraParams map[string]string
}

// RawGridSlice represents a decoded 2D scalar field for one variable and forecast step.
type RawGridSlice struct {
	Variable     string
	ValidTime    time.Time
	StepHours    int
	NLats        int
	NLons        int
	LatStart     float64 // e.g. 90.0
	LatEnd       float64 // e.g. -90.0
	LatStep      float64 // e.g. 0.25
	LonStart     float64 // e.g. 0.0 or -180.0
	LonEnd       float64 // e.g. 359.75 or 179.75
	LonStep      float64 // e.g. 0.25
	Lats         []float32 // Optional explicit coordinates
	Lons         []float32
	Data         []float32 // Row-major flattened: NLats * NLons
}

// Unit Conversion Helpers
const (
	KnotsToMS = 0.514444
	MSToKnots = 1.943844
	MSToKMH   = 3.6
	MSToMPH   = 2.236936
	PaToHPa   = 0.01
	KelvinOffset = 273.15
)

// UVToSpeedAndDirection converts orthogonal wind components (u, v in m/s) to speed (m/s) and meteorological direction (degrees from).
func UVToSpeedAndDirection(u, v float64) (speed float64, dirDeg float64) {
	speed = math.Hypot(u, v)
	if speed < 1e-6 {
		return 0, 0
	}
	// Meteorological direction: direction FROM which wind blows
	// In math atan2(v, u): u is eastward (0 deg/East), v is northward (90 deg/North)
	// Meteorological: North = 0/360, East = 90, South = 180, West = 270
	rad := math.Atan2(v, u)
	deg := (270.0 - rad*180.0/math.Pi)
	for deg < 0 {
		deg += 360.0
	}
	for deg >= 360.0 {
		deg -= 360.0
	}
	return speed, deg
}

// SpeedAndDirectionToUV converts speed and meteorological direction to U (eastward) and V (northward) components.
func SpeedAndDirectionToUV(speed, dirDeg float64) (u, v float64) {
	rad := dirDeg * math.Pi / 180.0
	// Wind blowing FROM dirDeg means vector points towards dirDeg + 180
	u = -speed * math.Sin(rad)
	v = -speed * math.Cos(rad)
	return u, v
}

// ConvertSpeed converts m/s to the target unit ("kn", "kmh", "mph", "ms").
func ConvertSpeed(ms float64, unit string) float64 {
	switch unit {
	case "kn", "knots", "knot":
		return ms * MSToKnots
	case "kmh", "km/h":
		return ms * MSToKMH
	case "mph":
		return ms * MSToMPH
	default:
		return ms
	}
}

// ConvertTemp converts Kelvin to target unit ("celsius", "fahrenheit", "kelvin").
func ConvertTemp(k float64, unit string) float64 {
	switch unit {
	case "celsius", "c", "°C":
		return k - KelvinOffset
	case "fahrenheit", "f", "°F":
		return (k-KelvinOffset)*1.8 + 32.0
	default:
		return k
	}
}

// ConvertPressure converts Pa to target unit ("hpa", "kpa", "pa", "bar", "mbar").
func ConvertPressure(pa float64, unit string) float64 {
	switch unit {
	case "hpa", "hPa", "mbar":
		return pa * PaToHPa
	case "kpa", "kPa":
		return pa * 0.001
	case "bar":
		return pa * 1e-5
	default:
		return pa * PaToHPa // Default to hPa for weather
	}
}
