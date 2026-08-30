package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"sailboat/meteo/internal/zarr"
)

func main() {
	storePath := flag.String("store", "./data/store/gfs_0p25/latest.zarr", "Path to Zarr store directory")
	latVal := flag.Float64("lat", 12.05, "Latitude to sample")
	lonVal := flag.Float64("lon", -61.75, "Longitude to sample")
	flag.Parse()

	absPath, _ := filepath.Abs(*storePath)
	store, err := zarr.OpenStore(absPath)
	if err != nil {
		log.Fatalf("Failed to open store at %s: %v", absPath, err)
	}

	fmt.Printf("=== Zarr Store Summary ===\n")
	fmt.Printf("Path:            %s\n", store.RootDir)
	fmt.Printf("Cycle:           %s\n", store.Cycle.Format("2006-01-02 15:04 UTC"))
	fmt.Printf("Dimensions:      Lats: %d (%.2f to %.2f), Lons: %d (%.2f to %.2f), Steps: %d\n",
		store.NLats, store.Lats[0], store.Lats[len(store.Lats)-1],
		store.NLons, store.Lons[0], store.Lons[len(store.Lons)-1],
		store.NSteps)
	fmt.Printf("Chunk Size:      [%d, %d, %d]\n", store.NSteps, store.ChunkLat, store.ChunkLon)
	fmt.Printf("Variables:       %v\n", store.Variables)
	fmt.Printf("Forecast Steps:  %v\n", store.Steps)

	// Sample point
	for _, v := range store.Variables {
		// Calculate lat/lon index
		normLon := *lonVal
		if normLon < 0 {
			normLon += 360.0
		}
		latIdx := int((90.0 - *latVal) / store.LatStep)
		lonIdx := int(normLon / store.LonStep)

		series, err := store.GetPointTimeSeries(v, latIdx, lonIdx)
		if err != nil {
			fmt.Printf("Var %s (latIdx=%d, lonIdx=%d): error %v\n", v, latIdx, lonIdx, err)
		} else {
			sampleVal := float32(0)
			if len(series) > 0 {
				sampleVal = series[0]
			}
			fmt.Printf("Var %s (latIdx=%d, lonIdx=%d, len=%d): step0=%.3f, all=%v\n",
				v, latIdx, lonIdx, len(series), sampleVal, series[:min(5, len(series))])
		}
	}
}
