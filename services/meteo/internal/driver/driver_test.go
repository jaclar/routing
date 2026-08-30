package driver

import (
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
