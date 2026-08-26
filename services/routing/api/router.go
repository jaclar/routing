package api

import (
	"net/http"
)

// SetupRouter creates an HTTP Handler with routing and CORS middleware.
func (s *Server) SetupRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.HandleHealth)
	mux.HandleFunc("/api/v1/health", s.HandleHealth)
	mux.HandleFunc("/api/v1/route", s.HandleRoute)
	mux.HandleFunc("/api/v1/weather/grid", s.HandleWeatherGrid)

	return enableCORS(mux)
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
