package driver

import (
	"context"
	"strings"
	"testing"
	"time"

	"sailboat/meteo/internal/model"
)

func TestGFSDriverIndexParsing(t *testing.T) {
	idxContent := `1:0:d=2026083006:TMP:surface:12 hour fcst:
2:100000:d=2026083006:UGRD:10 m above ground:12 hour fcst:
3:250000:d=2026083006:VGRD:10 m above ground:12 hour fcst:
4:400000:d=2026083006:PRMSL:mean sea level:12 hour fcst:
5:550000:d=2026083006:TMP:2 m above ground:12 hour fcst:
6:700000:d=2026083006:PRES:surface:12 hour fcst:`

	// Lookup UGRD
	start, end, err := ParseIndexByteRange(strings.NewReader(idxContent), ":UGRD:10 m above ground:")
	if err != nil {
		t.Fatalf("lookup UGRD failed: %v", err)
	}
	if start != 100000 || end != 249999 {
		t.Errorf("expected UGRD range 100000-249999, got %d-%d", start, end)
	}

	// Lookup VGRD
	startV, endV, err := ParseIndexByteRange(strings.NewReader(idxContent), ":VGRD:10 m above ground:")
	if err != nil {
		t.Fatalf("lookup VGRD failed: %v", err)
	}
	if startV != 250000 || endV != 399999 {
		t.Errorf("expected VGRD range 250000-399999, got %d-%d", startV, endV)
	}
}

func TestDiscoverSlices(t *testing.T) {
	gfs := NewGFSDriver(nil)
	cycle := &model.ModelCycle{
		ModelName:     model.ModelGFS025,
		ReferenceTime: time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC),
		ForecastSteps: []int{0, 3, 6},
	}
	vars := []string{model.VarWindU10m, model.VarWindV10m, model.VarMSLP}

	tasks, err := gfs.DiscoverSlices(cycle, vars)
	if err != nil {
		t.Fatalf("discover slices failed: %v", err)
	}

	expectedCount := len(cycle.ForecastSteps) * len(vars) // 3 * 3 = 9
	if len(tasks) != expectedCount {
		t.Errorf("expected %d tasks, got %d", expectedCount, len(tasks))
	}
}

func TestDownloadAndInspectECMWF(t *testing.T) {
	driver := NewECMWFDriver(model.ModelIFS025, nil)
	cycle := &model.ModelCycle{
		ModelName:     model.ModelIFS025,
		ReferenceTime: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		ResolutionDeg: 0.25,
		ForecastSteps: []int{0},
	}

	tasks, err := driver.DiscoverSlices(cycle, []string{model.VarWindU10m})
	if err != nil || len(tasks) == 0 {
		t.Skipf("DiscoverSlices failed: %v", err)
	}

	sliceU, err := driver.IngestSlice(context.Background(), tasks[0])
	if err != nil {
		t.Fatalf("IngestSlice for 10u failed: %v", err)
	}
	if sliceU.LonStart != 0.0 || sliceU.LonEnd != 359.75 {
		t.Errorf("expected LonStart 0.0 and LonEnd 359.75, got %f and %f", sliceU.LonStart, sliceU.LonEnd)
	}

	tasksV, err := driver.DiscoverSlices(cycle, []string{model.VarWindV10m})
	if err != nil || len(tasksV) == 0 {
		t.Fatalf("DiscoverSlices for 10v failed: %v", err)
	}
	sliceV, err := driver.IngestSlice(context.Background(), tasksV[0])
	if err != nil {
		t.Fatalf("IngestSlice for 10v failed: %v", err)
	}

	// Sample Grenada in normalized grid: Lat 12.0 (row 312), Lon -61.75 / 298.25°E (col 1193)
	idxNorm := 312*1440 + 1193
	uNorm := float64(sliceU.Data[idxNorm])
	vNorm := float64(sliceV.Data[idxNorm])
	spdNorm, dirNorm := model.UVToSpeedAndDirection(uNorm, vNorm)
	t.Logf("Normalized ECMWF at Grenada (12.0, -61.75): U=%.2f m/s, V=%.2f m/s -> Spd=%.1f kts, Dir=%.1f°",
		uNorm, vNorm, spdNorm*1.943844, dirNorm)
}

func TestDownloadAndInspectICON(t *testing.T) {
	driver := NewICONDriver(nil)
	cycle, err := driver.CheckLatestCycle(context.Background())
	if err != nil {
		t.Skipf("Network not available or cycle check failed: %v", err)
	}

	tasks, err := driver.DiscoverSlices(cycle, []string{model.VarWindU10m})
	if err != nil || len(tasks) == 0 {
		t.Skipf("DiscoverSlices failed: %v", err)
	}

	slice, err := driver.IngestSlice(context.Background(), tasks[0])
	if err != nil {
		t.Skipf("ICON IngestSlice skipped: %v", err)
	}
	if slice.NLats != 721 || slice.NLons != 1440 || len(slice.Data) != 1038240 {
		t.Fatalf("unexpected slice dimensions: NLats=%d, NLons=%d, LenData=%d", slice.NLats, slice.NLons, len(slice.Data))
	}
	t.Logf("ICON Slice result: NLats=%d, NLons=%d, LenData=%d", slice.NLats, slice.NLons, len(slice.Data))
}
