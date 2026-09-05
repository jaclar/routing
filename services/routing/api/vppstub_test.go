package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaclar/routing-service/polar/polartest"
)

// newStubVPP starts a stand-in for the VPP service that answers polar requests with the
// shared test fixture.
//
// The routing service holds no built-in polars, so these tests need something to fetch from.
// A stub server keeps them exercising the real client path — request, decode, cache — rather
// than reaching around it.
func newStubVPP(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/solve/matrix" {
			http.NotFound(w, r)
			return
		}
		table := polartest.Table()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"boat_name":    table.BoatName,
			"tws_list":     table.TWSList,
			"twa_list":     table.TWAList,
			"speed_matrix": table.Speeds,
		})
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}
