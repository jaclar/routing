package weather

import (
	"math"
	"time"
)

const (
	MSToKnots = 3600.0 / 1852.0
	KnotsToMS = 1852.0 / 3600.0
)

// Supported canonical model identifiers (matching meteo service storage keys)
const (
	ModelGFS025     = "gfs_0p25"
	ModelIFS025     = "ifs_0p25"
	ModelICONGlobal = "icon_global"
	ModelGEFS050    = "gefs_0p50"
	ModelIFSEns025  = "ifs_ens_0p25"
	ModelICONEPS    = "icon_eps_global"
	ModelAll        = "all"
)

// NormalizeModelID converts user/UI model aliases to canonical storage model IDs.
func NormalizeModelID(id string) string {
	switch id {
	case "gfs", "gfs_seamless", "gfs_0p25":
		return ModelGFS025
	case "ecmwf", "ifs", "ifs_seamless", "ifs_0p25":
		return ModelIFS025
	case "icon", "dwd_icon", "icon_seamless", "icon_global":
		return ModelICONGlobal
	case "gefs", "gefs_0p50", "noaa_gefs", "gefs050", "gefs_seamless":
		return ModelGEFS050
	case "ifs_ens", "ifs_ens_0p25", "ecmwf_ens", "ecmwf_ifs_ens":
		return ModelIFSEns025
	case "icon_eps", "icon_eps_global", "dwd_icon_eps":
		return ModelICONEPS
	case "", "all":
		return ModelAll
	default:
		return id
	}
}

// AvailableModelIDs returns the list of primary canonical models.
func AvailableModelIDs() []string {
	return []string{ModelGFS025, ModelIFS025, ModelICONGlobal, ModelGEFS050, ModelIFSEns025, ModelICONEPS}
}

// WindCondition represents wind speed and meteorological direction at a specific location and time.
type WindCondition struct {
	TWS float64 `json:"tws_kts"` // True Wind Speed [knots]
	TWD float64 `json:"twd_deg"` // True Wind Direction (blowing FROM) in degrees [0, 360)
	U   float64 `json:"u_ms"`    // U component (eastward) [m/s]
	V   float64 `json:"v_ms"`    // V component (northward) [m/s]
}

// UVToWindCondition converts eastward (U) and northward (V) wind components in m/s to TWS and TWD.
func UVToWindCondition(u, v float64) WindCondition {
	speedMS := math.Hypot(u, v)
	speedKts := speedMS * MSToKnots

	// Meteorological direction: direction from which the wind is blowing
	// atan2(-u, -v) gives angle in radians from North
	dirRad := math.Atan2(-u, -v)
	dirDeg := dirRad * 180.0 / math.Pi
	if dirDeg < 0 {
		dirDeg += 360.0
	}

	return WindCondition{
		TWS: speedKts,
		TWD: dirDeg,
		U:   u,
		V:   v,
	}
}

// WeatherProvider is the interface for sampling wind fields anywhere on Earth and across time.
type WeatherProvider interface {
	GetWind(lat, lon float64, t time.Time) WindCondition
	GetGrid(minLat, maxLat, minLon, maxLon float64, latStep, lonStep float64, t time.Time) ([][]WindCondition, error)
}

// WeatherGrid represents a structured 4D regular grid of wind forecasts.
type WeatherGrid struct {
	MinLat     float64
	MaxLat     float64
	LatStep    float64
	MinLon     float64
	MaxLon     float64
	LonStep    float64
	Timestamps []time.Time
	UData      [][][]float64 // [timeIdx][latIdx][lonIdx]
	VData      [][][]float64 // [timeIdx][latIdx][lonIdx]
}

// NewWeatherGrid initializes an allocated WeatherGrid.
func NewWeatherGrid(minLat, maxLat, latStep, minLon, maxLon, lonStep float64, timestamps []time.Time) *WeatherGrid {
	nLat := int(math.Round((maxLat-minLat)/latStep)) + 1
	nLon := int(math.Round((maxLon-minLon)/lonStep)) + 1
	nTime := len(timestamps)

	u := make([][][]float64, nTime)
	v := make([][][]float64, nTime)
	for t := 0; t < nTime; t++ {
		u[t] = make([][]float64, nLat)
		v[t] = make([][]float64, nLat)
		for i := 0; i < nLat; i++ {
			u[t][i] = make([]float64, nLon)
			v[t][i] = make([]float64, nLon)
		}
	}

	return &WeatherGrid{
		MinLat:     minLat,
		MaxLat:     maxLat,
		LatStep:    latStep,
		MinLon:     minLon,
		MaxLon:     maxLon,
		LonStep:    lonStep,
		Timestamps: timestamps,
		UData:      u,
		VData:      v,
	}
}

// Interpolate performs 4D spatial (bilinear) and temporal (linear) interpolation of the wind field.
func (g *WeatherGrid) Interpolate(lat, lon float64, t time.Time) WindCondition {
	if len(g.Timestamps) == 0 {
		return WindCondition{TWS: 12.0, TWD: 45.0, U: -4.4, V: -4.4}
	}

	// 1. Time interpolation index
	tIdx0, tIdx1, tFrac := g.findTimeBracket(t)

	// 2. Spatial interpolation at time tIdx0
	u0, v0 := g.interpolateSpatial(lat, lon, tIdx0)

	// If single time slice or exact match
	if tIdx0 == tIdx1 || tFrac <= 1e-6 {
		return UVToWindCondition(u0, v0)
	}

	// 3. Spatial interpolation at time tIdx1
	u1, v1 := g.interpolateSpatial(lat, lon, tIdx1)

	// 4. Linear temporal blending
	u := u0*(1.0-tFrac) + u1*tFrac
	v := v0*(1.0-tFrac) + v1*tFrac

	return UVToWindCondition(u, v)
}

func (g *WeatherGrid) findTimeBracket(t time.Time) (int, int, float64) {
	n := len(g.Timestamps)
	if n == 1 || t.Before(g.Timestamps[0]) {
		return 0, 0, 0.0
	}
	if !t.Before(g.Timestamps[n-1]) {
		return n - 1, n - 1, 0.0
	}

	for i := 0; i < n-1; i++ {
		t0 := g.Timestamps[i]
		t1 := g.Timestamps[i+1]
		if !t.Before(t0) && t.Before(t1) {
			duration := t1.Sub(t0).Seconds()
			if duration <= 0 {
				return i, i, 0.0
			}
			frac := t.Sub(t0).Seconds() / duration
			return i, i + 1, frac
		}
	}

	return n - 1, n - 1, 0.0
}

func (g *WeatherGrid) interpolateSpatial(lat, lon float64, tIdx int) (float64, float64) {
	// Clamp latitude
	cLat := math.Max(g.MinLat, math.Min(g.MaxLat, lat))
	cLon := lon
	// Wrap lon into grid bounds
	for cLon < g.MinLon {
		cLon += 360.0
	}
	for cLon > g.MaxLon {
		cLon -= 360.0
	}
	cLon = math.Max(g.MinLon, math.Min(g.MaxLon, cLon))

	latFracPos := (cLat - g.MinLat) / g.LatStep
	lonFracPos := (cLon - g.MinLon) / g.LonStep

	latIdx0 := int(math.Floor(latFracPos))
	latIdx1 := latIdx0 + 1
	nLat := len(g.UData[tIdx])
	if latIdx0 >= nLat-1 {
		latIdx0 = nLat - 1
		latIdx1 = nLat - 1
	}

	lonIdx0 := int(math.Floor(lonFracPos))
	lonIdx1 := lonIdx0 + 1
	nLon := len(g.UData[tIdx][0])
	if lonIdx0 >= nLon-1 {
		lonIdx0 = nLon - 1
		lonIdx1 = nLon - 1
	}

	dLat := latFracPos - float64(latIdx0)
	dLon := lonFracPos - float64(lonIdx0)

	uGrid := g.UData[tIdx]
	vGrid := g.VData[tIdx]

	u00 := uGrid[latIdx0][lonIdx0]
	u01 := uGrid[latIdx0][lonIdx1]
	u10 := uGrid[latIdx1][lonIdx0]
	u11 := uGrid[latIdx1][lonIdx1]

	v00 := vGrid[latIdx0][lonIdx0]
	v01 := vGrid[latIdx0][lonIdx1]
	v10 := vGrid[latIdx1][lonIdx0]
	v11 := vGrid[latIdx1][lonIdx1]

	// Bilinear interpolation
	u := (1.0-dLat)*(1.0-dLon)*u00 + (1.0-dLat)*dLon*u01 + dLat*(1.0-dLon)*u10 + dLat*dLon*u11
	v := (1.0-dLat)*(1.0-dLon)*v00 + (1.0-dLat)*dLon*v01 + dLat*(1.0-dLon)*v10 + dLat*dLon*v11

	return u, v
}
