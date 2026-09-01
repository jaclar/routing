package query

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/zarr"
)

func TestParseHTTPRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/forecast?latitude=12.05,-15.5&longitude=-61.75,45.2&hourly=wind_speed_10m,wind_direction_10m&wind_speed_unit=kn&models=gfs_seamless", nil)
	parsed, err := ParseHTTPRequest(req)
	if err != nil {
		t.Fatalf("ParseHTTPRequest failed: %v", err)
	}

	if len(parsed.Latitudes) != 2 || len(parsed.Longitudes) != 2 {
		t.Fatalf("expected 2 coordinates, got %d", len(parsed.Latitudes))
	}
	if parsed.Latitudes[0] != 12.05 || parsed.Longitudes[0] != -61.75 {
		t.Errorf("coordinate 0 mismatch: %f, %f", parsed.Latitudes[0], parsed.Longitudes[0])
	}
	if parsed.WindSpeedUnit != "kn" {
		t.Errorf("expected wind_speed_unit kn, got %s", parsed.WindSpeedUnit)
	}
	if len(parsed.Hourly) != 2 {
		t.Errorf("expected 2 hourly vars, got %d", len(parsed.Hourly))
	}
}

func TestEngineForecastExecution(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "meteo_engine_test_*")
	defer os.RemoveAll(tmpDir)

	mgr, _ := zarr.NewStoreManager(tmpDir)
	refTime := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	cycle := &model.ModelCycle{
		ModelName:     model.ModelGFS025,
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: []int{0, 3},
	}

	latStart, latEnd, latStep := 15.0, 10.0, 0.25
	lonStart, lonEnd, lonStep := -65.0, -60.0, 0.25
	vars := []string{model.VarWindU10m, model.VarWindV10m, model.VarMSLP}

	writer, stagingDir, _ := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, vars)
	nlats := 21
	nlons := 21

	for _, step := range cycle.ForecastSteps {
		uData := make([]float32, nlats*nlons)
		vData := make([]float32, nlats*nlons)
		mslpData := make([]float32, nlats*nlons)

		for i := 0; i < nlats*nlons; i++ {
			uData[i] = 10.0
			vData[i] = 0.0
			mslpData[i] = 101325.0
		}
		_ = writer.WriteSlice(&model.RawGridSlice{Variable: model.VarWindU10m, StepHours: step, Data: uData})
		_ = writer.WriteSlice(&model.RawGridSlice{Variable: model.VarWindV10m, StepHours: step, Data: vData})
		_ = writer.WriteSlice(&model.RawGridSlice{Variable: model.VarMSLP, StepHours: step, Data: mslpData})
	}
	_ = writer.Finalize()
	_ = mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir)

	engine := NewEngine(mgr)
	ctx := context.Background()

	// Single point query
	reqSingle := &ForecastRequest{
		Latitudes:     []float64{12.05},
		Longitudes:    []float64{-61.75},
		Hourly:        []string{"wind_speed_10m", "wind_direction_10m", "pressure_msl"},
		WindSpeedUnit: "kn",
	}

	resp, err := engine.ExecuteForecast(ctx, reqSingle)
	if err != nil {
		t.Fatalf("ExecuteForecast failed: %v", err)
	}

	omResp, ok := resp.(*OpenMeteoResponse)
	if !ok {
		t.Fatalf("expected *OpenMeteoResponse, got %T", resp)
	}

	if omResp.Latitude != 12.05 || omResp.Longitude != -61.75 {
		t.Errorf("lat/lon mismatch: %f, %f", omResp.Latitude, omResp.Longitude)
	}

	speeds, ok := omResp.Hourly["wind_speed_10m"].([]float64)
	if !ok || len(speeds) != 2 {
		t.Fatalf("expected 2 wind speeds, got %v", omResp.Hourly["wind_speed_10m"])
	}

	// 10 m/s in knots = 19.44 kn
	if speeds[0] < 19.4 || speeds[0] > 19.5 {
		t.Errorf("expected ~19.44 knots, got %f", speeds[0])
	}
}

func TestRealStoresInspection(t *testing.T) {
	dataDir := "../../data/store"
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		dataDir = "../data/store"
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		dataDir = "/Users/jaclar/Projects/routing/data/store"
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skip("data/store not found, skipping real store test")
	}

	mgr, err := zarr.NewStoreManager(dataDir)
	if err != nil {
		t.Fatalf("Failed to create StoreManager: %v", err)
	}

	engine := NewEngine(mgr)
	ctx := context.Background()

	modelsToTest := []string{"gfs_0p25", "ifs_0p25", "icon_global"}
	for _, m := range modelsToTest {
		t.Run(m, func(t *testing.T) {
			req := &ForecastRequest{
				Latitudes:     []float64{12.05},
				Longitudes:    []float64{-61.75},
				Hourly:        []string{"wind_speed_10m", "wind_direction_10m", "pressure_msl"},
				WindSpeedUnit: "kn",
				Models:        []string{m},
			}
			resp, err := engine.ExecuteForecast(ctx, req)
			if err != nil {
				t.Logf("Model %s returned error (expected if not populated): %v", m, err)
				return
			}
			omResp, ok := resp.(*OpenMeteoResponse)
			if !ok {
				t.Fatalf("expected *OpenMeteoResponse, got %T", resp)
			}
			speeds, _ := omResp.Hourly["wind_speed_10m"].([]float64)
			dirs, _ := omResp.Hourly["wind_direction_10m"].([]float64)
			t.Logf("Model %s: %d time points. First 5 speeds: %v, First 5 dirs: %v", m, len(speeds), speeds[:min(5, len(speeds))], dirs[:min(5, len(dirs))])
		})
	}
}

func TestEngineEnsembleForecastExecution(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "meteo_engine_ens_test_*")
	defer os.RemoveAll(tmpDir)

	mgr, _ := zarr.NewStoreManager(tmpDir)
	refTime := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	members := []int{0, 1, 2, 3, 4}
	cycle := &model.ModelCycle{
		ModelName:     model.ModelGEFS050,
		ReferenceTime: refTime,
		ResolutionDeg: 0.50,
		ForecastSteps: []int{0, 6},
		Members:       members,
		IsEnsemble:    true,
	}

	latStart, latEnd, latStep := 15.0, 10.0, 0.50
	lonStart, lonEnd, lonStep := -65.0, -60.0, 0.50
	vars := []string{model.VarWindU10m, model.VarWindV10m}

	writer, stagingDir, _ := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, vars)
	nlats := 11
	nlons := 11

	for _, step := range cycle.ForecastSteps {
		for _, mID := range members {
			uData := make([]float32, nlats*nlons)
			vData := make([]float32, nlats*nlons)

			for i := 0; i < nlats*nlons; i++ {
				uData[i] = float32(10.0 + float64(mID)*2.0)
				vData[i] = 0.0
			}
			_ = writer.WriteSlice(&model.RawGridSlice{Variable: model.VarWindU10m, StepHours: step, Member: mID, Data: uData})
			_ = writer.WriteSlice(&model.RawGridSlice{Variable: model.VarWindV10m, StepHours: step, Member: mID, Data: vData})
		}
	}
	_ = writer.Finalize()
	_ = mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir)

	engine := NewEngine(mgr)
	ctx := context.Background()

	req := &ForecastRequest{
		Latitudes:     []float64{12.0},
		Longitudes:    []float64{-62.0},
		Models:        []string{"gefs_0p50"},
		Hourly:        []string{"wind_speed_10m", "wind_speed_10m_p10", "wind_speed_10m_p90", "wind_speed_10m_std", "prob_wind_ge_25kt", "wind_speed_10m_member02"},
		WindSpeedUnit: "kn",
	}

	resp, err := engine.ExecuteForecast(ctx, req)
	if err != nil {
		t.Fatalf("ExecuteForecast for GEFS ensemble failed: %v", err)
	}

	omResp, ok := resp.(*OpenMeteoResponse)
	if !ok {
		t.Fatalf("expected *OpenMeteoResponse, got %T", resp)
	}

	// Verify mean wind speed
	if meanSpd, ok := omResp.Hourly["wind_speed_10m"].([]float64); !ok || len(meanSpd) != 2 {
		t.Errorf("missing or invalid wind_speed_10m: %v", omResp.Hourly["wind_speed_10m"])
	}

	// Verify percentiles
	if p10, ok := omResp.Hourly["wind_speed_10m_p10"].([]float64); !ok || len(p10) != 2 {
		t.Errorf("missing or invalid wind_speed_10m_p10: %v", omResp.Hourly["wind_speed_10m_p10"])
	}
	if p90, ok := omResp.Hourly["wind_speed_10m_p90"].([]float64); !ok || len(p90) != 2 {
		t.Errorf("missing or invalid wind_speed_10m_p90: %v", omResp.Hourly["wind_speed_10m_p90"])
	}

	// Verify member 2
	if m2, ok := omResp.Hourly["wind_speed_10m_member02"].([]float64); !ok || len(m2) != 2 {
		t.Errorf("missing or invalid wind_speed_10m_member02: %v", omResp.Hourly["wind_speed_10m_member02"])
	} else {
		// Member 2 U = 10 + 2*2 = 14 m/s = 27.21 kn
		if m2[0] < 27.0 || m2[0] > 27.5 {
			t.Errorf("expected member 2 ~27.2 kn, got %f", m2[0])
		}
	}

	// Test ExecuteGridWithStat
	gridResp, err := engine.ExecuteGridWithStat(ctx, "gefs_0p50", "p90", -1, 11.0, 14.0, -64.0, -61.0, 0.5, 0.5, 0)
	if err != nil {
		t.Fatalf("ExecuteGridWithStat failed: %v", err)
	}
	if gridResp.NLats <= 0 || gridResp.NLons <= 0 || len(gridResp.UData) == 0 {
		t.Fatalf("invalid grid response dimensions: NLats=%d, NLons=%d", gridResp.NLats, gridResp.NLons)
	}
}


func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
