package query

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"sailboat/meteo/internal/interp"
	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/zarr"
)

// Engine manages multi-model forecast querying and serialization.
type Engine struct {
	mu           sync.RWMutex
	storeManager *zarr.StoreManager
	stores       map[string]*cachedStore
}

type cachedStore struct {
	store       *zarr.Store
	interp      *interp.SpatialInterpolator
	lastChecked time.Time
}

// NewEngine creates a new query engine backed by a Zarr StoreManager.
func NewEngine(mgr *zarr.StoreManager) *Engine {
	return &Engine{
		storeManager: mgr,
		stores:       make(map[string]*cachedStore),
	}
}

// resolveStore returns the active store for the requested model ID, reloading if necessary.
func (e *Engine) resolveStore(modelID string) (*zarr.Store, *interp.SpatialInterpolator, error) {
	// Normalize model ID
	canonicalModel := normalizeModelID(modelID)

	e.mu.RLock()
	cs, exists := e.stores[canonicalModel]
	e.mu.RUnlock()

	if exists && time.Since(cs.lastChecked) < 30*time.Second {
		return cs.store, cs.interp, nil
	}

	// Load latest store from disk
	store, err := e.storeManager.OpenLatest(canonicalModel)
	if err != nil {
		if exists {
			// If reload fails but we have an existing store, keep using it
			return cs.store, cs.interp, nil
		}
		return nil, nil, fmt.Errorf("model %s not available in store: %w", canonicalModel, err)
	}

	si := interp.NewSpatialInterpolator(store)
	newCached := &cachedStore{
		store:       store,
		interp:      si,
		lastChecked: time.Now(),
	}

	e.mu.Lock()
	e.stores[canonicalModel] = newCached
	e.mu.Unlock()

	return store, si, nil
}

var ErrModelNotFound = errors.New("model not available in store")

// ExecuteForecast executes the forecast query and formats the Open-Meteo compliant output.
func (e *Engine) ExecuteForecast(ctx context.Context, req *ForecastRequest) (any, error) {
	start := time.Now()

	primaryModel := model.ModelGFS025
	if len(req.Models) > 0 && req.Models[0] != "" {
		primaryModel = req.Models[0]
	}

	store, si, err := e.resolveStore(primaryModel)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModelNotFound, err)
	}

	// If single coordinate, return a single JSON object
	if len(req.Latitudes) == 1 {
		return e.buildSingleResponse(store, si, req.Latitudes[0], req.Longitudes[0], req, start)
	}

	// Multiple coordinates -> return JSON array of responses
	responses := make([]*OpenMeteoResponse, len(req.Latitudes))
	for i := range req.Latitudes {
		res, err := e.buildSingleResponse(store, si, req.Latitudes[i], req.Longitudes[i], req, start)
		if err != nil {
			return nil, err
		}
		responses[i] = res
	}

	return responses, nil
}

// buildSingleResponse calculates the interpolated metrics for one point and populates the Open-Meteo schema.
func (e *Engine) buildSingleResponse(store *zarr.Store, si *interp.SpatialInterpolator, lat, lon float64, req *ForecastRequest, startTime time.Time) (*OpenMeteoResponse, error) {
	pt, err := interp.ComputePointForecast(store, si, lat, lon)
	if err != nil {
		return nil, err
	}

	hourlyUnits := make(map[string]string)
	hourly := make(map[string]any)

	hourlyUnits["time"] = "iso8601"
	hourly["time"] = pt.Times

	// Map requested variables
	reqMap := make(map[string]bool)
	for _, h := range req.Hourly {
		reqMap[strings.ToLower(h)] = true
	}
	if len(req.Hourly) == 0 {
		// Default variables if none specified
		reqMap[model.OMWindSpeed10m] = true
		reqMap[model.OMWindDirection10m] = true
		reqMap[model.OMPressureMSL] = true
	}

	// 1. Wind Speed
	if reqMap[model.OMWindSpeed10m] || reqMap["windspeed_10m"] {
		unit := req.WindSpeedUnit
		hourlyUnits[model.OMWindSpeed10m] = unit
		converted := make([]float64, len(pt.WindSpeed10m))
		for i, s := range pt.WindSpeed10m {
			if !math.IsNaN(s) {
				converted[i] = round2(model.ConvertSpeed(s, unit))
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMWindSpeed10m] = converted
	}

	// 2. Wind Direction
	if reqMap[model.OMWindDirection10m] || reqMap["winddirection_10m"] {
		hourlyUnits[model.OMWindDirection10m] = "°"
		converted := make([]float64, len(pt.WindDirection10m))
		for i, d := range pt.WindDirection10m {
			if !math.IsNaN(d) {
				converted[i] = round1(d)
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMWindDirection10m] = converted
	}

	// 3. Wind Gusts
	if reqMap[model.OMWindGusts10m] || reqMap["wind_gusts_10m"] || reqMap["windgusts_10m"] {
		unit := req.WindSpeedUnit
		hourlyUnits[model.OMWindGusts10m] = unit
		converted := make([]float64, len(pt.WindGust10m))
		for i, g := range pt.WindGust10m {
			if !math.IsNaN(g) {
				converted[i] = round2(model.ConvertSpeed(g, unit))
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMWindGusts10m] = converted
	}

	// 4. Pressure MSL
	if reqMap[model.OMPressureMSL] || reqMap[model.OMSurfacePressure] {
		hourlyUnits[model.OMPressureMSL] = "hPa"
		converted := make([]float64, len(pt.PressureMSL))
		for i, p := range pt.PressureMSL {
			if !math.IsNaN(p) {
				converted[i] = round1(p)
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMPressureMSL] = converted
	}

	// 5. Temperature 2m
	if reqMap[model.OMTemperature2m] || reqMap["temperature_2m"] {
		hourlyUnits[model.OMTemperature2m] = "°C"
		if req.TemperatureUnit == "fahrenheit" {
			hourlyUnits[model.OMTemperature2m] = "°F"
		}
		converted := make([]float64, len(pt.Temperature2m))
		for i, t := range pt.Temperature2m {
			if !math.IsNaN(t) {
				if req.TemperatureUnit == "fahrenheit" {
					converted[i] = round1(t*1.8 + 32.0)
				} else {
					converted[i] = round1(t)
				}
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMTemperature2m] = converted
	}

	// 6. Precipitation
	if reqMap[model.OMPrecipitation] || reqMap["precipitation"] {
		hourlyUnits[model.OMPrecipitation] = req.PrecipitationUnit
		converted := make([]float64, len(pt.Precipitation))
		for i, pr := range pt.Precipitation {
			if !math.IsNaN(pr) {
				if req.PrecipitationUnit == "inch" {
					converted[i] = round2(pr / 25.4)
				} else {
					converted[i] = round2(pr)
				}
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMPrecipitation] = converted
	}

	// 7. Waves
	if reqMap[model.OMWaveHeight] && len(pt.WaveHeight) > 0 {
		hourlyUnits[model.OMWaveHeight] = "m"
		converted := make([]float64, len(pt.WaveHeight))
		for i, h := range pt.WaveHeight {
			if !math.IsNaN(h) {
				converted[i] = round2(h)
			} else {
				converted[i] = 0
			}
		}
		hourly[model.OMWaveHeight] = converted
	}

	// 8. Ensemble Statistics: Percentiles, Spread, and Exceedance Probabilities
	unit := req.WindSpeedUnit

	// Percentiles (P10, P50, P90)
	if reqMap[model.OMWindSpeed10mP10] || reqMap["windspeed_10m_p10"] {
		hourlyUnits[model.OMWindSpeed10mP10] = unit
		if ts, err := interp.InterpolateTimeSeries(store, si, "wind_speed_p10", lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round2(model.ConvertSpeed(v, unit))
				}
			}
			hourly[model.OMWindSpeed10mP10] = converted
		}
	}

	if reqMap[model.OMWindSpeed10mP50] || reqMap["windspeed_10m_p50"] {
		hourlyUnits[model.OMWindSpeed10mP50] = unit
		if ts, err := interp.InterpolateTimeSeries(store, si, "wind_speed_p50", lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round2(model.ConvertSpeed(v, unit))
				}
			}
			hourly[model.OMWindSpeed10mP50] = converted
		}
	}

	if reqMap[model.OMWindSpeed10mP90] || reqMap["windspeed_10m_p90"] {
		hourlyUnits[model.OMWindSpeed10mP90] = unit
		if ts, err := interp.InterpolateTimeSeries(store, si, "wind_speed_p90", lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round2(model.ConvertSpeed(v, unit))
				}
			}
			hourly[model.OMWindSpeed10mP90] = converted
		}
	}

	if reqMap[model.OMWindSpeed10mStd] || reqMap["windspeed_10m_std"] {
		hourlyUnits[model.OMWindSpeed10mStd] = unit
		if ts, err := interp.InterpolateTimeSeries(store, si, "wind_speed_std", lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round2(model.ConvertSpeed(v, unit))
				}
			}
			hourly[model.OMWindSpeed10mStd] = converted
		}
	}

	if reqMap[model.OMWindGusts10mP90] || reqMap["wind_gusts_10m_p90"] {
		hourlyUnits[model.OMWindGusts10mP90] = unit
		varSrc := model.VarWindGust10m + "_p90"
		if ts, err := interp.InterpolateTimeSeries(store, si, varSrc, lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round2(model.ConvertSpeed(v, unit))
				}
			}
			hourly[model.OMWindGusts10mP90] = converted
		} else if len(pt.WindGust10m) > 0 {
			converted := make([]float64, len(pt.WindGust10m))
			for i, v := range pt.WindGust10m {
				if !math.IsNaN(v) {
					converted[i] = round2(model.ConvertSpeed(v, unit))
				}
			}
			hourly[model.OMWindGusts10mP90] = converted
		}
	}

	// Exceedance probabilities
	if reqMap[model.OMProbWindGE25kt] || reqMap["prob_wind_ge_25kt"] {
		hourlyUnits[model.OMProbWindGE25kt] = "%"
		if ts, err := interp.InterpolateTimeSeries(store, si, "prob_wind_ge_25kt", lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round1(v * 100.0)
				}
			}
			hourly[model.OMProbWindGE25kt] = converted
		}
	}

	if reqMap[model.OMProbWindGE34kt] || reqMap["prob_wind_ge_34kt"] {
		hourlyUnits[model.OMProbWindGE34kt] = "%"
		if ts, err := interp.InterpolateTimeSeries(store, si, "prob_wind_ge_34kt", lat, lon); err == nil {
			converted := make([]float64, len(ts))
			for i, v := range ts {
				if !math.IsNaN(v) {
					converted[i] = round1(v * 100.0)
				}
			}
			hourly[model.OMProbWindGE34kt] = converted
		}
	}

	// 9. Individual Member Queries (e.g. wind_speed_10m_member01, wind_speed_10m_member02)
	for reqKey := range reqMap {
		if strings.HasPrefix(reqKey, "wind_speed_10m_member") || strings.HasPrefix(reqKey, "windspeed_10m_member") {
			parts := strings.Split(reqKey, "_member")
			if len(parts) == 2 {
				var mID int
				_, err := fmt.Sscanf(parts[1], "%d", &mID)
				if err == nil {
					uMem, errU := interp.InterpolateMemberTimeSeries(store, si, model.VarWindU10m, mID, lat, lon)
					vMem, errV := interp.InterpolateMemberTimeSeries(store, si, model.VarWindV10m, mID, lat, lon)
					if errU == nil && errV == nil {
						hourlyUnits[reqKey] = unit
						memSpeed := make([]float64, len(uMem))
						for idx := range uMem {
							if !math.IsNaN(uMem[idx]) && !math.IsNaN(vMem[idx]) {
								spd := math.Hypot(uMem[idx], vMem[idx])
								memSpeed[idx] = round2(model.ConvertSpeed(spd, unit))
							} else {
								memSpeed[idx] = 0
							}
						}
						hourly[reqKey] = memSpeed
					}
				}
			}
		}
	}

	elapsedMS := float64(time.Since(startTime).Microseconds()) / 1000.0

	res := &OpenMeteoResponse{
		Latitude:             lat,
		Longitude:            lon,
		GenerationTimeMS:     elapsedMS,
		UTCOffsetSeconds:     0,
		Timezone:             "GMT",
		TimezoneAbbreviation: "GMT",
		Elevation:            0.0,
		HourlyUnits:          hourlyUnits,
		Hourly:               hourly,
	}

	if req.CurrentWeather && len(pt.WindSpeed10m) > 0 {
		spd := pt.WindSpeed10m[0]
		dir := pt.WindDirection10m[0]
		temp := 20.0
		if len(pt.Temperature2m) > 0 && !math.IsNaN(pt.Temperature2m[0]) {
			temp = pt.Temperature2m[0]
		}
		res.CurrentWeather = &CurrentWeather{
			Time:          pt.Times[0],
			Temperature:   round1(temp),
			WindSpeed:     round2(model.ConvertSpeed(spd, req.WindSpeedUnit)),
			WindDirection: round1(dir),
			WeatherCode:   0,
			IsDay:         1,
		}
	}

	return res, nil
}

// ExecuteGrid extracts a 2D bounding box slice across the grid at a specific forecast step.
func (e *Engine) ExecuteGrid(ctx context.Context, modelID string, minLat, maxLat, minLon, maxLon, latStep, lonStep float64, stepHour int) (*GridResponse, error) {
	return e.ExecuteGridWithStat(ctx, modelID, "mean", -1, minLat, maxLat, minLon, maxLon, latStep, lonStep, stepHour)
}

// ExecuteGridWithStat extracts a 2D bounding box slice with statistic or specific ensemble member selection.
func (e *Engine) ExecuteGridWithStat(ctx context.Context, modelID, stat string, member int, minLat, maxLat, minLon, maxLon, latStep, lonStep float64, stepHour int) (*GridResponse, error) {
	store, _, err := e.resolveStore(modelID)
	if err != nil {
		return nil, err
	}

	// Find step index
	stepIdx := 0
	for i, s := range store.Steps {
		if s >= stepHour {
			stepIdx = i
			break
		}
	}

	latSpan := maxLat - minLat
	lonSpan := maxLon - minLon
	if latStep <= 0 {
		latStep = 0.25
	}
	if lonStep <= 0 {
		lonStep = 0.25
	}

	nlats := int(math.Round(latSpan/latStep)) + 1
	nlons := int(math.Round(lonSpan/lonStep)) + 1

	uGrid := make([][]float32, nlats)
	vGrid := make([][]float32, nlats)

	si := interp.NewSpatialInterpolator(store)

	// Determine variable source based on stat / member
	var uVar, vVar string
	if stat != "" && stat != "mean" && store.IsEnsemble {
		uVar = model.VarWindU10m + "_" + stat
		vVar = model.VarWindV10m + "_" + stat
	} else {
		uVar = model.VarWindU10m
		vVar = model.VarWindV10m
	}

	for i := 0; i < nlats; i++ {
		lat := minLat + float64(i)*latStep
		uGrid[i] = make([]float32, nlons)
		vGrid[i] = make([]float32, nlons)

		for j := 0; j < nlons; j++ {
			lon := minLon + float64(j)*lonStep
			i0, i1, j0, j1, u, v := si.GridCoords(lat, lon)

			var u00, u10, u01, u11 []float32
			var v00, v10, v01, v11 []float32

			if member >= 0 && store.IsEnsemble {
				u00, _ = store.GetMemberPointTimeSeries(model.VarWindU10m, member, i0, j0)
				u10, _ = store.GetMemberPointTimeSeries(model.VarWindU10m, member, i1, j0)
				u01, _ = store.GetMemberPointTimeSeries(model.VarWindU10m, member, i0, j1)
				u11, _ = store.GetMemberPointTimeSeries(model.VarWindU10m, member, i1, j1)

				v00, _ = store.GetMemberPointTimeSeries(model.VarWindV10m, member, i0, j0)
				v10, _ = store.GetMemberPointTimeSeries(model.VarWindV10m, member, i1, j0)
				v01, _ = store.GetMemberPointTimeSeries(model.VarWindV10m, member, i0, j1)
				v11, _ = store.GetMemberPointTimeSeries(model.VarWindV10m, member, i1, j1)
			} else {
				u00, _ = store.GetPointTimeSeries(uVar, i0, j0)
				u10, _ = store.GetPointTimeSeries(uVar, i1, j0)
				u01, _ = store.GetPointTimeSeries(uVar, i0, j1)
				u11, _ = store.GetPointTimeSeries(uVar, i1, j1)

				v00, _ = store.GetPointTimeSeries(vVar, i0, j0)
				v10, _ = store.GetPointTimeSeries(vVar, i1, j0)
				v01, _ = store.GetPointTimeSeries(vVar, i0, j1)
				v11, _ = store.GetPointTimeSeries(vVar, i1, j1)
			}

			if len(u00) > stepIdx && len(u10) > stepIdx && len(u01) > stepIdx && len(u11) > stepIdx {
				uInterp := interp.BilinearInterp(u00[stepIdx], u10[stepIdx], u01[stepIdx], u11[stepIdx], u, v)
				uGrid[i][j] = float32(uInterp)
			}
			if len(v00) > stepIdx && len(v10) > stepIdx && len(v01) > stepIdx && len(v11) > stepIdx {
				vInterp := interp.BilinearInterp(v00[stepIdx], v10[stepIdx], v01[stepIdx], v11[stepIdx], u, v)
				vGrid[i][j] = float32(vInterp)
			}
		}
	}

	validTime := store.Cycle.Add(time.Duration(store.Steps[stepIdx]) * time.Hour)

	return &GridResponse{
		Model:     modelID,
		Cycle:     store.Cycle,
		ValidTime: validTime,
		StepHours: store.Steps[stepIdx],
		MinLat:    minLat,
		MaxLat:    maxLat,
		LatStep:   latStep,
		MinLon:    minLon,
		MaxLon:    maxLon,
		LonStep:   lonStep,
		NLats:     nlats,
		NLons:     nlons,
		UData:     uGrid,
		VData:     vGrid,
	}, nil
}

func normalizeModelID(slug string) string {
	slug = strings.ToLower(slug)
	switch slug {
	case "gfs", "gfs_seamless", "gfs_0p25", "gfs025":
		return model.ModelGFS025
	case "ifs", "ecmwf", "ecmwf_ifs025", "ifs_0p25":
		return model.ModelIFS025
	case "aifs", "ecmwf_aifs025", "aifs_0p25":
		return model.ModelAIFS025
	case "icon", "icon_global", "dwd_icon":
		return model.ModelICON025
	case "gefs", "gefs_0p50", "noaa_gefs", "gefs050", "gefs_seamless":
		return model.ModelGEFS050
	case "ifs_ens", "ifs_ens_0p25", "ecmwf_ens", "ecmwf_ifs_ens", "ifs_0p25_ens":
		return model.ModelIFSEns025
	case "icon_eps", "icon_eps_global", "icon_eps_0p25", "dwd_icon_eps":
		return model.ModelICONEPS025
	default:
		return slug
	}
}

func round1(val float64) float64 {
	return math.Round(val*10.0) / 10.0
}

func round2(val float64) float64 {
	return math.Round(val*100.0) / 100.0
}

