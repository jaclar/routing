package zarr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScanModelStoresCaches checks that sizing a store with many chunk files is paid once, so a
// status request against a real deployment does not restat the whole volume every time.
func TestScanModelStoresCaches(t *testing.T) {
	base := t.TempDir()

	// Two cycles of a store shaped like a real ensemble: 43 arrays x 1035 chunks.
	const arrays, chunks = 12, 180
	for _, cycleName := range []string{"20260830_00Z.zarr", "20260830_06Z.zarr"} {
		dir := filepath.Join(base, "gefs_0p50", cycleName)
		for a := 0; a < arrays; a++ {
			varDir := filepath.Join(dir, fmt.Sprintf("var_%d", a))
			if err := os.MkdirAll(varDir, 0755); err != nil {
				t.Fatal(err)
			}
			for c := 0; c < chunks; c++ {
				if err := os.WriteFile(filepath.Join(varDir, fmt.Sprintf("0.%d.%d", c/45, c%45)), []byte("chunkdata"), 0644); err != nil {
					t.Fatal(err)
				}
			}
		}
		meta := `{"model_name":"gefs_0p50","reference_time":"2026-08-30T06:00:00Z","members":[0,1],"is_ensemble":true,"store_members":false,"variables":["wind_u_10m"]}`
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mgr, err := NewStoreManager(base)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	first, err := mgr.ScanModelStores(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	coldElapsed := time.Since(start)

	start = time.Now()
	second, err := mgr.ScanModelStores(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	warmElapsed := time.Since(start)

	if len(first) != 1 || len(first[0].Cycles) != 2 {
		t.Fatalf("expected 1 model with 2 cycles, got %+v", first)
	}
	for _, c := range first[0].Cycles {
		if want := int64(arrays * chunks * len("chunkdata")); c.SizeBytes < want {
			t.Errorf("cycle %s size %d is below the %d bytes of chunk data written", c.Cycle, c.SizeBytes, want)
		}
	}
	if first[0].Cycles[0].SizeBytes != second[0].Cycles[0].SizeBytes {
		t.Errorf("cached size disagrees with cold size: %d vs %d",
			first[0].Cycles[0].SizeBytes, second[0].Cycles[0].SizeBytes)
	}

	t.Logf("%d files: cold=%v warm=%v (%.0fx faster)",
		2*arrays*chunks, coldElapsed.Round(time.Millisecond), warmElapsed.Round(time.Microsecond),
		float64(coldElapsed)/float64(warmElapsed))

	if warmElapsed > coldElapsed/10 {
		t.Errorf("cached scan not meaningfully faster: cold=%v warm=%v", coldElapsed, warmElapsed)
	}
}

// TestScanModelStoresRespectsContext verifies an expired context degrades the response rather
// than hanging the request.
func TestScanModelStoresRespectsContext(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "gefs_0p50", "20260830_06Z.zarr")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := `{"model_name":"gefs_0p50","reference_time":"2026-08-30T06:00:00Z","variables":["wind_u_10m"]}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out, err := mustManager(t, base).ScanModelStores(ctx)
	if err != nil {
		t.Fatalf("expected degraded result, got error: %v", err)
	}
	if len(out) != 1 || len(out[0].Cycles) != 1 {
		t.Fatalf("expected the cycle to still be listed, got %+v", out)
	}
	if got := out[0].Cycles[0].SizeBytes; got != -1 {
		t.Errorf("expected size -1 for an unmeasured cycle, got %d", got)
	}
}

func mustManager(t *testing.T, base string) *StoreManager {
	t.Helper()
	mgr, err := NewStoreManager(base)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}
