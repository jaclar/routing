package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/query"
	"sailboat/meteo/internal/zarr"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4081"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data/store"
	}

	mgr, err := zarr.NewStoreManager(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize store manager at %s: %v", dataDir, err)
	}

	engine := query.NewEngine(mgr)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health Check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "meteo-api",
		})
	})

	// Open-Meteo Compatibility Endpoint
	r.Get("/v1/forecast", func(w http.ResponseWriter, r *http.Request) {
		handleForecast(w, r, engine, "")
	})

	// Model-specific alias endpoints
	r.Get("/v1/gfs", func(w http.ResponseWriter, r *http.Request) {
		handleForecast(w, r, engine, model.ModelGFS025)
	})

	r.Get("/v1/ecmwf", func(w http.ResponseWriter, r *http.Request) {
		handleForecast(w, r, engine, model.ModelIFS025)
	})

	r.Get("/v1/dwd-icon", func(w http.ResponseWriter, r *http.Request) {
		handleForecast(w, r, engine, model.ModelICON025)
	})

	// High-speed 2D bounding-box / corridor endpoint for routing engines
	r.Get("/v1/grid", func(w http.ResponseWriter, r *http.Request) {
		handleGrid(w, r, engine)
	})
	r.Post("/v1/grid", func(w http.ResponseWriter, r *http.Request) {
		handleGrid(w, r, engine)
	})

	log.Printf("Meteo API server listening on :%s (Store: %s)", port, dataDir)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), r); err != nil {
		log.Fatalf("Server exited: %v", err)
	}
}

func handleForecast(w http.ResponseWriter, r *http.Request, engine *query.Engine, forcedModel string) {
	req, err := query.ParseHTTPRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": true, "reason": %q}`, err.Error()), http.StatusBadRequest)
		return
	}

	if forcedModel != "" {
		req.Models = []string{forcedModel}
	}

	resp, err := engine.ExecuteForecast(r.Context(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": true, "reason": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleGrid(w http.ResponseWriter, r *http.Request, engine *query.Engine) {
	q := r.URL.Query()

	minLat, _ := strconv.ParseFloat(q.Get("min_lat"), 64)
	maxLat, _ := strconv.ParseFloat(q.Get("max_lat"), 64)
	minLon, _ := strconv.ParseFloat(q.Get("min_lon"), 64)
	maxLon, _ := strconv.ParseFloat(q.Get("max_lon"), 64)
	latStep, _ := strconv.ParseFloat(q.Get("lat_step"), 64)
	lonStep, _ := strconv.ParseFloat(q.Get("lon_step"), 64)
	stepHour, _ := strconv.Atoi(q.Get("step"))
	modelID := q.Get("model")
	if modelID == "" {
		modelID = model.ModelGFS025
	}

	if maxLat == 0 && minLat == 0 && maxLon == 0 && minLon == 0 {
		// Default to Caribbean / North Atlantic domain if unspecified
		minLat, maxLat = 10.0, 25.0
		minLon, maxLon = -70.0, -55.0
	}

	res, err := engine.ExecuteGrid(r.Context(), modelID, minLat, maxLat, minLon, maxLon, latStep, lonStep, stepHour)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": true, "reason": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
