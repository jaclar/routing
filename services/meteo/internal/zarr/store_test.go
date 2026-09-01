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

func TestZarrEnsembleHybridStoreWriteRead(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meteo_zarr_ens_test_*")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr, err := NewStoreManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to init store manager: %v", err)
	}

	refTime := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	members := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9} // 10 members
	cycle := &model.ModelCycle{
		ModelName:     "gefs_0p50",
		ReferenceTime: refTime,
		ResolutionDeg: 0.50,
		ForecastSteps: []int{0, 6, 12},
		Members:       members,
		IsEnsemble:    true,
	}

	latStart, latEnd, latStep := 15.0, 10.0, 0.50
	lonStart, lonEnd, lonStep := -65.0, -60.0, 0.50
	vars := []string{model.VarWindU10m, model.VarWindV10m}

	writer, stagingDir, err := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, vars)
	if err != nil {
		t.Fatalf("failed to create staging writer: %v", err)
	}

	nlats := int(math.Round((latStart-latEnd)/latStep)) + 1
	nlons := int(math.Round((lonEnd-lonStart)/lonStep)) + 1

	// Write slices for 10 members across 3 steps
	for _, step := range cycle.ForecastSteps {
		for _, mID := range members {
			uData := make([]float32, nlats*nlons)
			vData := make([]float32, nlats*nlons)

			// Member mID adds (mID * 2.0 m/s) perturbation
			for i := 0; i < nlats; i++ {
				for j := 0; j < nlons; j++ {
					idx := i*nlons + j
					uData[idx] = float32(10.0 + float64(mID)*2.0 + float64(step)*0.5)
					vData[idx] = float32(5.0 + float64(mID)*1.0 + float64(step)*0.2)
				}
			}

			err = writer.WriteSlice(&model.RawGridSlice{
				Variable:  model.VarWindU10m,
				StepHours: step,
				Member:    mID,
				Data:      uData,
			})
			if err != nil {
				t.Fatalf("write u slice failed for member %d: %v", mID, err)
			}

			err = writer.WriteSlice(&model.RawGridSlice{
				Variable:  model.VarWindV10m,
				StepHours: step,
				Member:    mID,
				Data:      vData,
			})
			if err != nil {
				t.Fatalf("write v slice failed for member %d: %v", mID, err)
			}
		}
	}

	if err := writer.Finalize(); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}

	if err := mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir); err != nil {
		t.Fatalf("promotion failed: %v", err)
	}

	store, err := mgr.OpenLatest(cycle.ModelName)
	if err != nil {
		t.Fatalf("open latest ensemble store failed: %v", err)
	}

	if !store.IsEnsemble {
		t.Errorf("expected IsEnsemble to be true")
	}
	if store.NMembers != 10 {
		t.Errorf("expected 10 members, got %d", store.NMembers)
	}

	// 1. Strategy B Verification: Retrieve specific member time series
	member3U, err := store.GetMemberPointTimeSeries(model.VarWindU10m, 3, 2, 2)
	if err != nil {
		t.Fatalf("GetMemberPointTimeSeries failed for member 3: %v", err)
	}
	if len(member3U) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(member3U))
	}
	// Expected u for member 3 at step 0: 10 + 3*2 + 0 = 16.0
	if math.Abs(float64(member3U[0]-16.0)) > 1e-4 {
		t.Errorf("member 3 step 0 U: expected 16.0, got %f", member3U[0])
	}

	// 2. Strategy B Verification: Retrieve all members time series
	allU, err := store.GetAllMembersPointTimeSeries(model.VarWindU10m, 2, 2)
	if err != nil {
		t.Fatalf("GetAllMembersPointTimeSeries failed: %v", err)
	}
	if len(allU) != 10 {
		t.Fatalf("expected 10 members in allU, got %d", len(allU))
	}
	for mIdx := 0; mIdx < 10; mIdx++ {
		expVal := float32(10.0 + float64(mIdx)*2.0)
		if math.Abs(float64(allU[mIdx][0]-expVal)) > 1e-4 {
			t.Errorf("allU member %d step 0: expected %f, got %f", mIdx, expVal, allU[mIdx][0])
		}
	}

	// 3. Strategy A Verification: Precomputed statistics (Mean, Std, P10, P50, P90)
	meanU, err := store.GetPointTimeSeries(model.VarWindU10m+"_mean", 2, 2)
	if err != nil {
		t.Fatalf("get wind_u_10m_mean failed: %v", err)
	}
	// Mean of [10, 12, 14, 16, 18, 20, 22, 24, 26, 28] = 19.0
	if math.Abs(float64(meanU[0]-19.0)) > 1e-3 {
		t.Errorf("wind_u_10m_mean step 0: expected 19.0, got %f", meanU[0])
	}

	p50U, err := store.GetPointTimeSeries(model.VarWindU10m+"_p50", 2, 2)
	if err != nil {
		t.Fatalf("get wind_u_10m_p50 failed: %v", err)
	}
	if math.Abs(float64(p50U[0]-19.0)) > 1e-3 {
		t.Errorf("wind_u_10m_p50 step 0: expected 19.0, got %f", p50U[0])
	}

	p90U, err := store.GetPointTimeSeries(model.VarWindU10m+"_p90", 2, 2)
	if err != nil {
		t.Fatalf("get wind_u_10m_p90 failed: %v", err)
	}
	// 90th percentile of 10 items (ranks 0..9) is rank 8.1 -> between 26 and 28 -> 26.2
	if p90U[0] < 25.0 || p90U[0] > 28.0 {
		t.Errorf("wind_u_10m_p90 step 0 unexpected: %f", p90U[0])
	}

	// 4. Strategy A Verification: Derived wind speed stats & exceedance probabilities
	meanSpd, err := store.GetPointTimeSeries("wind_speed_mean", 2, 2)
	if err != nil {
		t.Fatalf("get wind_speed_mean failed: %v", err)
	}
	if meanSpd[0] <= 0 {
		t.Errorf("expected positive wind_speed_mean, got %f", meanSpd[0])
	}

	prob25, err := store.GetPointTimeSeries("prob_wind_ge_25kt", 2, 2)
	if err != nil {
		t.Fatalf("get prob_wind_ge_25kt failed: %v", err)
	}
	if prob25[0] < 0 || prob25[0] > 1.0 {
		t.Errorf("prob_wind_ge_25kt out of bounds [0, 1]: %f", prob25[0])
	}
}
