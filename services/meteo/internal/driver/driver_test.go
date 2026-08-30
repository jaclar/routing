package driver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"sailboat/meteo/internal/grib2"
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
	cycle, err := driver.CheckLatestCycle(context.Background())
	if err != nil {
		t.Skipf("Network not available or cycle check failed: %v", err)
	}

	tasks, err := driver.DiscoverSlices(cycle, []string{model.VarWindU10m})
	if err != nil || len(tasks) == 0 {
		t.Skipf("DiscoverSlices failed: %v", err)
	}

	offset, length, lookupErr := driver.lookupECMWFIndex(context.Background(), tasks[0].ExtraParams["idx_url"], tasks[0].ExtraParams["param"])
	if lookupErr != nil || length == 0 {
		t.Skipf("lookupECMWFIndex skipped: %v", lookupErr)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tasks[0].SourceURL, nil)
	if err != nil {
		t.Skipf("Create request failed: %v", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	resp, err := driver.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		t.Skipf("ECMWF upstream fetch skipped: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		t.Skipf("ReadAll failed: %v", err)
	}

	msg, err := grib2.Parse(data)
	if err != nil {
		t.Fatalf("grib2.Parse failed: %v", err)
	}
	if len(msg.Values) != 1038240 {
		t.Fatalf("expected 1038240 values, got %d", len(msg.Values))
	}
	t.Logf("Parsed ECMWF message: Ni=%d, Nj=%d, LenValues=%d", msg.Ni, msg.Nj, len(msg.Values))
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
