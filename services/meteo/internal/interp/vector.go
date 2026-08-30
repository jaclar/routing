package interp

import (
	"math"
	"time"

	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/zarr"
)

// InterpolatedForecastPoint contains time series arrays for standard and derived meteorology variables at a single coordinate.
type InterpolatedForecastPoint struct {
	Lat             float64
	Lon             float64
	Times           []string
	ForecastSteps   []int
	WindSpeed10m    []float64
	WindDirection10m[]float64
	WindGust10m     []float64
	PressureMSL     []float64
	Temperature2m   []float64
	Precipitation   []float64
	WaveHeight      []float64
	WaveDirection   []float64
	WavePeriod      []float64
}

// ComputePointForecast interpolates all requested variables at (lat, lon) and reconstructs physical vector fields.
func ComputePointForecast(store *zarr.Store, si *SpatialInterpolator, lat, lon float64) (*InterpolatedForecastPoint, error) {
	nSteps := store.NSteps

	// Format timestamps (e.g. "2026-08-30T06:00")
	times := make([]string, nSteps)
	for i, step := range store.Steps {
		t := store.Cycle.Add(timeDurationHours(step))
		times[i] = t.Format("2006-01-02T15:04")
	}

	result := &InterpolatedForecastPoint{
		Lat:           lat,
		Lon:           lon,
		Times:         times,
		ForecastSteps: store.Steps,
	}

	// 1. Interpolate Wind U and V components and calculate speed & direction
	uSeries, errU := InterpolateTimeSeries(store, si, model.VarWindU10m, lat, lon)
	vSeries, errV := InterpolateTimeSeries(store, si, model.VarWindV10m, lat, lon)

	if errU == nil && errV == nil {
		result.WindSpeed10m = make([]float64, nSteps)
		result.WindDirection10m = make([]float64, nSteps)

		for i := 0; i < nSteps; i++ {
			u := uSeries[i]
			v := vSeries[i]
			if !math.IsNaN(u) && !math.IsNaN(v) {
				spd, dir := model.UVToSpeedAndDirection(u, v)
				result.WindSpeed10m[i] = spd
				result.WindDirection10m[i] = dir
			} else {
				result.WindSpeed10m[i] = math.NaN()
				result.WindDirection10m[i] = math.NaN()
			}
		}
	}

	// 2. Wind Gusts
	if gustSeries, err := InterpolateTimeSeries(store, si, model.VarWindGust10m, lat, lon); err == nil {
		result.WindGust10m = gustSeries
	}

	// 3. Mean Sea Level Pressure (Pa -> hPa)
	if mslpSeries, err := InterpolateTimeSeries(store, si, model.VarMSLP, lat, lon); err == nil {
		result.PressureMSL = make([]float64, nSteps)
		for i, p := range mslpSeries {
			result.PressureMSL[i] = model.ConvertPressure(p, "hpa")
		}
	}

	// 4. Temperature at 2m (K -> Celsius)
	if tempSeries, err := InterpolateTimeSeries(store, si, model.VarTemp2m, lat, lon); err == nil {
		result.Temperature2m = make([]float64, nSteps)
		for i, k := range tempSeries {
			result.Temperature2m[i] = model.ConvertTemp(k, "celsius")
		}
	}

	// 5. Precipitation Accumulation
	if precipSeries, err := InterpolateTimeSeries(store, si, model.VarPrecipAccum, lat, lon); err == nil {
		result.Precipitation = precipSeries
	}

	// 6. Waves
	if waveHgt, err := InterpolateTimeSeries(store, si, model.VarWaveHeightSig, lat, lon); err == nil {
		result.WaveHeight = waveHgt
	}
	if waveDir, err := InterpolateTimeSeries(store, si, model.VarWaveDirPrim, lat, lon); err == nil {
		result.WaveDirection = waveDir
	}
	if wavePer, err := InterpolateTimeSeries(store, si, model.VarWavePeriodPeak, lat, lon); err == nil {
		result.WavePeriod = wavePer
	}

	return result, nil
}

func timeDurationHours(hours int) time.Duration {
	return time.Duration(hours) * time.Hour
}
