package interp

import (
	"math"
	"time"

	"sailboat/meteo/internal/zarr"
)

// PointSample holds interpolated scalar or vector meteorological values at a specific (lat, lon, time).
type PointSample struct {
	Lat       float64
	Lon       float64
	Time      time.Time
	Values    map[string]float64 // Canonical variable -> value
	WindSpeed float64            // [m/s]
	WindDir   float64            // [degrees]
	WindGust  float64            // [m/s]
	MSLP      float64            // [Pa]
	Temp2m    float64            // [K]
	Precip    float64            // [mm]
	WaveHgt   float64            // [m]
	WaveDir   float64            // [deg]
	WavePer   float64            // [s]
}

// SpatialInterpolator performs 2D bilinear interpolation on grid coordinates with spherical antimeridian wrapping.
type SpatialInterpolator struct {
	NLats    int
	NLons    int
	LatStart float64
	LatEnd   float64
	LatStep  float64
	LonStart float64
	LonEnd   float64
	LonStep  float64
}

// NewSpatialInterpolator creates an interpolator matching a Zarr store grid.
func NewSpatialInterpolator(store *zarr.Store) *SpatialInterpolator {
	latStart := float64(store.Lats[0])
	latEnd := float64(store.Lats[len(store.Lats)-1])
	lonStart := float64(store.Lons[0])
	lonEnd := float64(store.Lons[len(store.Lons)-1])

	return &SpatialInterpolator{
		NLats:    store.NLats,
		NLons:    store.NLons,
		LatStart: latStart,
		LatEnd:   latEnd,
		LatStep:  store.LatStep,
		LonStart: lonStart,
		LonEnd:   lonEnd,
		LonStep:  store.LonStep,
	}
}

// GridCoords computes surrounding integer indices and fractional offsets for a given (lat, lon).
func (si *SpatialInterpolator) GridCoords(lat, lon float64) (i0, i1, j0, j1 int, uFrac, vFrac float64) {
	// Normalize longitude to [0, 360) if LonStart >= 0, else [-180, 180)
	normLon := lon
	if si.LonStart >= 0 {
		normLon = math.Mod(math.Mod(lon, 360.0)+360.0, 360.0)
	} else {
		for normLon < -180.0 {
			normLon += 360.0
		}
		for normLon >= 180.0 {
			normLon -= 360.0
		}
	}

	// Clamp latitude
	clampedLat := lat
	if clampedLat > 90.0 {
		clampedLat = 90.0
	} else if clampedLat < -90.0 {
		clampedLat = -90.0
	}

	// Latitude index (assuming descending 90 to -90)
	var latIndexExact float64
	if si.LatStart > si.LatEnd {
		latIndexExact = (si.LatStart - clampedLat) / si.LatStep
	} else {
		latIndexExact = (clampedLat - si.LatStart) / si.LatStep
	}

	i0 = int(math.Floor(latIndexExact))
	if i0 < 0 {
		i0 = 0
	}
	if i0 >= si.NLats-1 {
		i0 = si.NLats - 2
		if i0 < 0 {
			i0 = 0
		}
	}
	i1 = i0 + 1
	if i1 >= si.NLats {
		i1 = si.NLats - 1
	}
	uFrac = latIndexExact - float64(i0)
	if uFrac < 0 {
		uFrac = 0
	} else if uFrac > 1 {
		uFrac = 1
	}

	// Longitude index
	lonIndexExact := (normLon - si.LonStart) / si.LonStep
	j0 = int(math.Floor(lonIndexExact))
	for j0 < 0 {
		j0 += si.NLons
	}
	j0 = j0 % si.NLons
	j1 = (j0 + 1) % si.NLons // Antimeridian wrapping
	vFrac = lonIndexExact - math.Floor(lonIndexExact)
	if vFrac < 0 {
		vFrac = 0
	} else if vFrac > 1 {
		vFrac = 1
	}

	return i0, i1, j0, j1, uFrac, vFrac
}

// BilinearInterp computes the weighted average of 4 surrounding corner values.
func BilinearInterp(v00, v10, v01, v11 float32, u, v float64) float64 {
	// Handle NaNs gracefully
	var sumWeight, sumVal float64

	w00 := (1.0 - u) * (1.0 - v)
	if !math.IsNaN(float64(v00)) {
		sumVal += w00 * float64(v00)
		sumWeight += w00
	}

	w10 := u * (1.0 - v)
	if !math.IsNaN(float64(v10)) {
		sumVal += w10 * float64(v10)
		sumWeight += w10
	}

	w01 := (1.0 - u) * v
	if !math.IsNaN(float64(v01)) {
		sumVal += w01 * float64(v01)
		sumWeight += w01
	}

	w11 := u * v
	if !math.IsNaN(float64(v11)) {
		sumVal += w11 * float64(v11)
		sumWeight += w11
	}

	if sumWeight < 1e-6 {
		return math.NaN()
	}

	return sumVal / sumWeight
}

// InterpolateTimeSeries performs spatial bilinear interpolation across all forecast steps for a single variable.
func InterpolateTimeSeries(store *zarr.Store, si *SpatialInterpolator, variable string, lat, lon float64) ([]float64, error) {
	i0, i1, j0, j1, u, v := si.GridCoords(lat, lon)

	// Retrieve time series at 4 surrounding corners
	ts00, err := store.GetPointTimeSeries(variable, i0, j0)
	if err != nil {
		return nil, err
	}
	ts10, err := store.GetPointTimeSeries(variable, i1, j0)
	if err != nil {
		return nil, err
	}
	ts01, err := store.GetPointTimeSeries(variable, i0, j1)
	if err != nil {
		return nil, err
	}
	ts11, err := store.GetPointTimeSeries(variable, i1, j1)
	if err != nil {
		return nil, err
	}

	nSteps := store.NSteps
	res := make([]float64, nSteps)

	for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
		res[stepIdx] = BilinearInterp(ts00[stepIdx], ts10[stepIdx], ts01[stepIdx], ts11[stepIdx], u, v)
	}

	return res, nil
}

// InterpolateMemberTimeSeries performs spatial bilinear interpolation across all forecast steps for a specific ensemble member.
func InterpolateMemberTimeSeries(store *zarr.Store, si *SpatialInterpolator, variable string, member int, lat, lon float64) ([]float64, error) {
	i0, i1, j0, j1, u, v := si.GridCoords(lat, lon)

	ts00, err := store.GetMemberPointTimeSeries(variable, member, i0, j0)
	if err != nil {
		return nil, err
	}
	ts10, err := store.GetMemberPointTimeSeries(variable, member, i1, j0)
	if err != nil {
		return nil, err
	}
	ts01, err := store.GetMemberPointTimeSeries(variable, member, i0, j1)
	if err != nil {
		return nil, err
	}
	ts11, err := store.GetMemberPointTimeSeries(variable, member, i1, j1)
	if err != nil {
		return nil, err
	}

	nSteps := store.NSteps
	res := make([]float64, nSteps)

	for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
		res[stepIdx] = BilinearInterp(ts00[stepIdx], ts10[stepIdx], ts01[stepIdx], ts11[stepIdx], u, v)
	}

	return res, nil
}

