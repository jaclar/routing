package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/polar"
)

func TestHealthEndpoint(t *testing.T) {
	server := NewServer("http://localhost:8000")
	handler := server.SetupRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to parse health response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Fatalf("Expected status 'healthy', got %s", resp.Status)
	}
}

func TestRouteEndpoint(t *testing.T) {
	server := NewServer("http://localhost:8000")
	handler := server.SetupRouter()

	routeReq := RouteRequest{
		Start:         geo.Point{Lat: 41.40, Lon: -71.35}, // Newport Brenton Reef offshore
		Dest:          geo.Point{Lat: 32.40, Lon: -64.55}, // Bermuda
		BoatPreset:    "36ft-ketch",
		TimeStepHours: 3.0,
	}

	body, _ := json.Marshal(routeReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/route", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRouteEndpointWithCustomPolar(t *testing.T) {
	server := NewServer("http://localhost:8000")
	handler := server.SetupRouter()

	customPolar := &polar.PolarTable{
		BoatName: "Uploaded POL Racer",
		TWSList:  []float64{6.0, 10.0, 15.0, 20.0},
		TWAList:  []float64{30.0, 45.0, 90.0, 135.0, 180.0},
		Speeds: [][]float64{
			{3.5, 4.8, 5.5, 5.0, 3.2},
			{4.8, 6.2, 7.1, 6.8, 4.5},
			{5.5, 7.1, 8.4, 8.0, 5.8},
			{6.0, 7.8, 9.2, 9.0, 6.5},
		},
	}

	routeReq := RouteRequest{
		Start:         geo.Point{Lat: 41.40, Lon: -71.35},
		Dest:          geo.Point{Lat: 32.40, Lon: -64.55},
		BoatPreset:    "custom-pol",
		CustomPolar:   customPolar,
		TimeStepHours: 3.0,
	}

	body, _ := json.Marshal(routeReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/route", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for custom polar route, got %d: %s", rec.Code, rec.Body.String())
	}

	var res isochrone.RouteResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode route response: %v", err)
	}

	if len(res.Waypoints) == 0 {
		t.Fatalf("Expected route waypoints with custom polar, got 0")
	}

	if res.BoatName != "Uploaded POL Racer" {
		t.Fatalf("Expected boat name 'Uploaded POL Racer', got '%s'", res.BoatName)
	}
}

func TestWeatherGridEndpoint(t *testing.T) {
	server := NewServer("http://localhost:8000")
	handler := server.SetupRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather/grid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}
}

func TestLandmaskPolygonsEndpoint(t *testing.T) {
	server := NewServer("http://localhost:8000")
	handler := server.SetupRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/landmask/polygons", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp LandmaskResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode landmask response: %v", err)
	}

	if len(resp.Polygons) == 0 {
		t.Fatalf("Expected at least 1 land polygon, got 0")
	}
}
