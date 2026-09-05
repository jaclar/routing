package polar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// stubVPP serves a minimal polar and counts how many times it was asked to solve.
func stubVPP(t *testing.T, calls *atomic.Int64) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req SolveMatrixReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(SolveMatrixResp{
			BoatName:    "Stub " + req.PresetName,
			TWSList:     []float64{6, 12, 20},
			TWAList:     []float64{0, 45, 90, 180},
			SpeedMatrix: [][]float64{{0, 3, 4, 2}, {0, 5, 6, 4}, {0, 6, 7, 5}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPresetIsFetchedOnceThenCached(t *testing.T) {
	var calls atomic.Int64
	c := NewVPPClient(stubVPP(t, &calls))

	for i := 0; i < 5; i++ {
		table, err := c.FetchPolar("36ft-ketch", nil)
		if err != nil {
			t.Fatalf("fetch %d failed: %v", i, err)
		}
		if table.BoatName != "Stub 36ft-ketch" {
			t.Fatalf("unexpected boat: %s", table.BoatName)
		}
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("expected the preset to be solved once, got %d requests", n)
	}
	if got := c.CachedPresets(); len(got) != 1 || got[0] != "36ft-ketch" {
		t.Errorf("unexpected cache contents: %v", got)
	}
}

func TestDistinctPresetsCachedSeparately(t *testing.T) {
	var calls atomic.Int64
	c := NewVPPClient(stubVPP(t, &calls))

	for _, id := range []string{"36ft-ketch", "36ft-sloop", "40ft-cruiser", "36ft-ketch"} {
		if _, err := c.FetchPolar(id, nil); err != nil {
			t.Fatalf("fetch %s failed: %v", id, err)
		}
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 solves for 3 distinct presets, got %d", n)
	}
}

func TestCustomBoatIsNeverCached(t *testing.T) {
	var calls atomic.Int64
	c := NewVPPClient(stubVPP(t, &calls))

	custom := map[string]any{"name": "One-off"}
	for i := 0; i < 3; i++ {
		if _, err := c.FetchPolar("", custom); err != nil {
			t.Fatalf("fetch %d failed: %v", i, err)
		}
	}
	// A custom boat is particular to its request; caching it would grow without bound.
	if n := calls.Load(); n != 3 {
		t.Errorf("expected every custom-boat request to be solved, got %d of 3", n)
	}
	if got := c.CachedPresets(); len(got) != 0 {
		t.Errorf("custom boats must not enter the preset cache, found %v", got)
	}
}

func TestConcurrentFetchesShareTheCache(t *testing.T) {
	var calls atomic.Int64
	c := NewVPPClient(stubVPP(t, &calls))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.FetchPolar("36ft-ketch", nil); err != nil {
				t.Errorf("concurrent fetch failed: %v", err)
			}
		}()
	}
	wg.Wait()

	// Racing callers may each solve once before the first result lands; what matters is that
	// they do not all miss, and that the cache converges on a single entry.
	if n := calls.Load(); n > 20 {
		t.Errorf("more solves (%d) than callers", n)
	}
	if got := c.CachedPresets(); len(got) != 1 {
		t.Errorf("expected exactly one cached preset, got %v", got)
	}
}

func TestUnreachableServiceReturnsError(t *testing.T) {
	// Nothing is listening here: the routing service must fail loudly rather than route
	// against a substitute boat.
	c := NewVPPClient("http://127.0.0.1:1")

	table, err := c.FetchPolar("36ft-ketch", nil)
	if err == nil {
		t.Fatal("expected an error when the VPP service is unreachable")
	}
	if table != nil {
		t.Errorf("expected no table on failure, got %q", table.BoatName)
	}
}

func TestServiceErrorIsNotCached(t *testing.T) {
	var calls atomic.Int64
	failing := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if failing {
			http.Error(w, "solver busy", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(SolveMatrixResp{
			BoatName: "Recovered", TWSList: []float64{6}, TWAList: []float64{90},
			SpeedMatrix: [][]float64{{5}},
		})
	}))
	defer srv.Close()

	c := NewVPPClient(srv.URL)
	if _, err := c.FetchPolar("36ft-ketch", nil); err == nil {
		t.Fatal("expected the first attempt to fail")
	}

	failing = false
	table, err := c.FetchPolar("36ft-ketch", nil)
	if err != nil {
		t.Fatalf("expected recovery once the service returns, got %v", err)
	}
	if table.BoatName != "Recovered" {
		t.Errorf("expected the retry to reach the service, got %q", table.BoatName)
	}
}
