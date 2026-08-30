package zarr

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sailboat/meteo/internal/model"
)

func TestZarrStoreWriteRead(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meteo_zarr_test_*")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewStoreManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to init store manager: %v", err)
	}

	refTime := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	cycle := &model.ModelCycle{
		ModelName:     "gfs_0p25",
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: []int{0, 3, 6, 9},
	}

	// 20x20 test grid around (10..15 Lat, -65..-60 Lon)
	latStart, latEnd, latStep := 15.0, 10.25, 0.25
	lonStart, lonEnd, lonStep := -65.0, -60.25, 0.25
	vars := []string{model.VarWindU10m, model.VarWindV10m}

	writer, stagingDir, err := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, vars)
	if err != nil {
		t.Fatalf("failed to create staging writer: %v", err)
	}

	nlats := int(math.Round((latStart-latEnd)/latStep)) + 1
	nlons := int(math.Round((lonEnd-lonStart)/lonStep)) + 1

	// Write slices for steps 0, 3, 6, 9
	for _, step := range cycle.ForecastSteps {
		uData := make([]float32, nlats*nlons)
		vData := make([]float32, nlats*nlons)

		for i := 0; i < nlats; i++ {
			for j := 0; j < nlons; j++ {
				idx := i*nlons + j
				uData[idx] = float32(float64(step) + float64(i)*0.1)
				vData[idx] = float32(float64(step)*2.0 + float64(j)*0.1)
			}
		}

		err = writer.WriteSlice(&model.RawGridSlice{
			Variable:  model.VarWindU10m,
			StepHours: step,
			Data:      uData,
		})
		if err != nil {
			t.Fatalf("write u slice failed: %v", err)
		}

		err = writer.WriteSlice(&model.RawGridSlice{
			Variable:  model.VarWindV10m,
			StepHours: step,
			Data:      vData,
		})
		if err != nil {
			t.Fatalf("write v slice failed: %v", err)
		}
	}

	if err := writer.Finalize(); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}

	// Promote to permanent cycle and update symlink
	if err := mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir); err != nil {
		t.Fatalf("promotion failed: %v", err)
	}

	// Open latest store
	store, err := mgr.OpenLatest(cycle.ModelName)
	if err != nil {
		t.Fatalf("open latest failed: %v", err)
	}

	if store.NSteps != 4 {
		t.Errorf("expected 4 steps, got %d", store.NSteps)
	}

	// Test point query at latIdx=2, lonIdx=3
	uSeries, err := store.GetPointTimeSeries(model.VarWindU10m, 2, 3)
	if err != nil {
		t.Fatalf("get point time series failed: %v", err)
	}

	if len(uSeries) != 4 {
		t.Fatalf("expected 4 values in time series, got %d", len(uSeries))
	}

	for sIdx, step := range cycle.ForecastSteps {
		expectedVal := float32(float64(step) + 2.0*0.1)
		if math.Abs(float64(uSeries[sIdx]-expectedVal)) > 1e-4 {
			t.Errorf("step %d: expected %f, got %f", step, expectedVal, uSeries[sIdx])
		}
	}

	// Verify Pruning
	oldCycleTime := refTime.Add(-12 * time.Hour)
	oldDir := filepath.Join(mgr.ModelDir(cycle.ModelName), fmt.Sprintf("%s.zarr", CycleSlug(oldCycleTime)))
	_ = os.MkdirAll(oldDir, 0755)

	if err := mgr.PruneOldCycles(cycle.ModelName, 1); err != nil {
		t.Fatalf("pruning failed: %v", err)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("expected old cycle %s to be pruned", oldDir)
	}
}
