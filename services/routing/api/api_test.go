package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaclar/routing-service/geo"
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
		Start:         geo.Point{Lat: 41.45, Lon: -71.35}, // Newport
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
