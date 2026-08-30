package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jaclar/routing-service/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4080"
	}

	vppURL := os.Getenv("VPP_SERVICE_URL")
	if vppURL == "" {
		vppURL = "http://localhost:4001"
	}

	server := api.NewServer(vppURL)
	handler := server.SetupRouter()

	log.Printf("Starting Weather Routing Service on :%s (VPP at %s) ...", port, vppURL)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
