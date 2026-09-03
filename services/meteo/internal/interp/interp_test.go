package interp

import (
	"math"
	"os"
	"testing"
	"time"

	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/zarr"
)

func TestSpatialInterpolatorGridCoords(t *testing.T) {
	// Global 0.25° store
	store := &zarr.Store{
		NLats:   721,
		NLons:   1440,
		LatStep: 0.25,
		LonStep: 0.25,
		Lats:    make([]float32, 721),
		Lons:    make([]float32, 1440),
	}
	for i := 0; i < 721; i++ {
		store.Lats[i] = float32(90.0 - float64(i)*0.25)
	}
	for j := 0; j < 1440; j++ {
		store.Lons[j] = float32(float64(j) * 0.25)
	}

	si := NewSpatialInterpolator(store)

	// Test point at (12.10, -61.75 -> 298.25 E)
	i0, i1, j0, _, u, v := si.GridCoords(12.10, -61.75)

	// 90 - 12.10 = 77.9 / 0.25 = 311.6 -> i0 = 311 (lat = 12.25), i1 = 312 (lat = 12.0)
	if i0 != 311 || i1 != 312 {
		t.Errorf("expected i0=311, i1=312, got %d, %d", i0, i1)
	}
	if math.Abs(u-0.6) > 1e-4 {
		t.Errorf("expected u=0.6, got %f", u)
	}

	// -61.75 normalized to 0..360 is 298.25. 298.25 / 0.25 = 1193.0
	if j0 != 1193 {
		t.Errorf("expected j0=1193, got %d", j0)
	}
	if math.Abs(v-0.0) > 1e-4 {
		t.Errorf("expected v=0.0, got %f", v)
	}

	// Test antimeridian wrapping at lon = 359.9
	_, _, aj0, aj1, _, av := si.GridCoords(0.0, 359.9)
	if aj0 != 1439 || aj1 != 0 {
		t.Errorf("expected aj0=1439, aj1=0 (wrapping to first meridian), got %d, %d", aj0, aj1)
	}
	if av <= 0.0 {
		t.Errorf("expected av > 0, got %f", av)
	}
}

func TestBilinearInterp(t *testing.T) {
	// Corners: (0,0)=10, (1,0)=20, (0,1)=10, (1,1)=20
	// Center (0.5, 0.5) should be 15.0
	val := BilinearInterp(10.0, 20.0, 10.0, 20.0, 0.5, 0.5)
	if math.Abs(val-15.0) > 1e-4 {
		t.Errorf("expected 15.0, got %f", val)
	}
}

func TestComputePointForecast(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meteo_interp_test_*")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, _ := zarr.NewStoreManager(tmpDir)
	refTime := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	cycle := &model.ModelCycle{
		ModelName:     "gfs_0p25",
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: []int{0, 3},
	}

	latStart, latEnd, latStep := 15.0, 10.0, 0.25
	lonStart, lonEnd, lonStep := -65.0, -60.0, 0.25
	vars := []string{model.VarWindU10m, model.VarWindV10m, model.VarMSLP}

	writer, stagingDir, _ := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, vars, false)
	nlats := int(math.Round((latStart-latEnd)/latStep)) + 1
	nlons := int(math.Round((lonEnd-lonStart)/lonStep)) + 1

	for _, step := range cycle.ForecastSteps {
		uData := make([]float32, nlats*nlons)
		vData := make([]float32, nlats*nlons)
		mslpData := make([]float32, nlats*nlons)

		for i := 0; i < nlats*nlons; i++ {
			uData[i] = 10.0 // 10 m/s Eastward
			vData[i] = 0.0  // 0 m/s Northward -> Wind blowing from 270 deg (West) at 10 m/s
			mslpData[i] = 101325.0 // 1013.25 hPa
		}

		writer.WriteSlice(&model.RawGridSlice{Variable: model.VarWindU10m, StepHours: step, Data: uData})
		writer.WriteSlice(&model.RawGridSlice{Variable: model.VarWindV10m, StepHours: step, Data: vData})
		writer.WriteSlice(&model.RawGridSlice{Variable: model.VarMSLP, StepHours: step, Data: mslpData})
	}
	_ = writer.Finalize()
	_ = mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir)

	store, err := mgr.OpenLatest(cycle.ModelName)
	if err != nil {
		t.Fatalf("open store failed: %v", err)
	}

	si := NewSpatialInterpolator(store)
	pt, err := ComputePointForecast(store, si, 12.05, -61.75)
	if err != nil {
		t.Fatalf("compute point forecast failed: %v", err)
	}

	if len(pt.WindSpeed10m) != 2 {
		t.Fatalf("expected 2 speed steps, got %d", len(pt.WindSpeed10m))
	}

	if math.Abs(pt.WindSpeed10m[0]-10.0) > 1e-4 {
		t.Errorf("expected wind speed 10.0, got %f", pt.WindSpeed10m[0])
	}
	if math.Abs(pt.WindDirection10m[0]-270.0) > 1e-4 {
		t.Errorf("expected wind direction 270.0 (from West), got %f", pt.WindDirection10m[0])
	}
	if math.Abs(pt.PressureMSL[0]-1013.25) > 1e-2 {
		t.Errorf("expected MSLP 1013.25 hPa, got %f", pt.PressureMSL[0])
	}
}
