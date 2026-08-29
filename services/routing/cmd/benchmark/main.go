package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

type PresetConfig struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	StartName string        `json:"start_name"`
	Start     geo.Point     `json:"start"`
	DestName  string        `json:"dest_name"`
	Dest      geo.Point     `json:"dest"`
	DirectNM  float64       `json:"direct_distance_nm"`
	TimeStep  time.Duration `json:"time_step"`
	BoatID    string        `json:"boat_id"`
	BoatName  string        `json:"boat_name"`
}

type BenchmarkResult struct {
	PresetID         string    `json:"preset_id"`
	PresetName       string    `json:"preset_name"`
	BoatName         string    `json:"boat_name"`
	TimeStepStr      string    `json:"time_step"`
	DirectDistanceNM float64   `json:"direct_distance_nm"`
	RouteDistanceNM  float64   `json:"route_distance_nm"`
	DurationHours    float64   `json:"duration_hours"`
	WaypointsCount   int       `json:"waypoints_count"`
	WavefrontSteps   int       `json:"wavefront_steps"`
	TotalTacks       int       `json:"total_tacks"`
	TotalGybes       int       `json:"total_gybes"`
	MaxWindKts       float64   `json:"max_wind_kts"`
	Iterations       int       `json:"iterations"`
	MinTimeMs        float64   `json:"min_time_ms"`
	MeanTimeMs       float64   `json:"mean_time_ms"`
	MedianTimeMs     float64   `json:"median_time_ms"`
	MaxTimeMs        float64   `json:"max_time_ms"`
	StdDevMs         float64   `json:"std_dev_ms"`
	ThroughputPerSec float64   `json:"throughput_per_sec"`
	AllocBytesPerOp  uint64    `json:"alloc_bytes_per_op"`
	AllocsPerOp      uint64    `json:"allocs_per_op"`
	Success          bool      `json:"success"`
}

func getAllPresets() []PresetConfig {
	return []PresetConfig{
		{
			ID:        "grenada_trinidad",
			Name:      "Prickly Bay (Grenada) to Chaguaramas (Trinidad)",
			StartName: "Prickly Bay",
			Start:     geo.Point{Lat: 11.975, Lon: -61.765},
			DestName:  "Chaguaramas",
			Dest:      geo.Point{Lat: 10.675, Lon: -61.645},
			TimeStep:  5 * time.Minute,
			BoatID:    "36ft-ketch",
			BoatName:  "36ft Cruising Ketch",
		},
		{
			ID:        "cowes_fastnet",
			Name:      "Cowes to Fastnet Rock (Rolex Fastnet Race)",
			StartName: "Cowes",
			Start:     geo.Point{Lat: 50.76, Lon: -1.20},
			DestName:  "Fastnet Rock",
			Dest:      geo.Point{Lat: 51.39, Lon: -9.60},
			TimeStep:  30 * time.Minute,
			BoatID:    "36ft-sloop",
			BoatName:  "36ft Racer-Cruiser Sloop",
		},
		{
			ID:        "newport_bermuda",
			Name:      "Newport to Bermuda (Classic Ocean Race)",
			StartName: "Newport",
			Start:     geo.Point{Lat: 41.40, Lon: -71.35},
			DestName:  "Bermuda",
			Dest:      geo.Point{Lat: 32.40, Lon: -64.55},
			TimeStep:  1 * time.Hour,
			BoatID:    "40ft-cruiser",
			BoatName:  "40ft Performance Cruiser",
		},
		{
			ID:        "lisbon_madeira",
			Name:      "Lisbon to Madeira Island (Atlantic Crossing)",
			StartName: "Lisbon (Cascais)",
			Start:     geo.Point{Lat: 38.67, Lon: -9.42},
			DestName:  "Madeira (Funchal)",
			Dest:      geo.Point{Lat: 32.64, Lon: -16.90},
			TimeStep:  1 * time.Hour,
			BoatID:    "36ft-ketch",
			BoatName:  "36ft Cruising Ketch",
		},
		{
			ID:        "sf_hawaii",
			Name:      "San Francisco to Hawaii (Transpac Passage)",
			StartName: "San Francisco",
			Start:     geo.Point{Lat: 37.75, Lon: -122.60},
			DestName:  "Honolulu",
			Dest:      geo.Point{Lat: 21.25, Lon: -157.60},
			TimeStep:  4 * time.Hour,
			BoatID:    "36ft-ketch",
			BoatName:  "36ft Cruising Ketch",
		},
	}
}

func main() {
	cpuProfile := flag.String("cpuprofile", "", "Write cpu profile to file")
	memProfile := flag.String("memprofile", "", "Write memory profile to file")
	presetFilter := flag.String("preset", "all", "Preset ID to benchmark or 'all'")
	iterations := flag.Int("iterations", 5, "Number of benchmark iterations per preset")
	warmup := flag.Int("warmup", 1, "Number of warmup iterations per preset")
	jsonOutput := flag.String("json", "", "Save JSON metrics to file")
	flag.Parse()

	log.Println("================================================================")
	log.Println("     ISOCHRONE ROUTING BENCHMARK & PERFORMANCE SUITE            ")
	log.Println("================================================================")

	// CPU profiling if requested
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalf("Could not create CPU profile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("Could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
		log.Printf("CPU profiling enabled -> %s", *cpuProfile)
	}

	// 1. One-time initialization of static resources (landmask, weather)
	log.Println("Initializing global GSHHG landmask database...")
	t0 := time.Now()
	landMask := landmask.NewGSHHGLandMask()
	log.Printf("Landmask loaded in %v", time.Since(t0))

	startTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	weatherEngine := weather.NewRealisticGFSEngine(startTime)

	allPresets := getAllPresets()
	var selectedPresets []PresetConfig

	if *presetFilter == "all" || *presetFilter == "" {
		selectedPresets = allPresets
	} else {
		for _, p := range allPresets {
			if p.ID == *presetFilter {
				selectedPresets = append(selectedPresets, p)
			}
		}
		if len(selectedPresets) == 0 {
			log.Fatalf("Unknown preset ID '%s'. Valid options: grenada_trinidad, cowes_fastnet, newport_bermuda, lisbon_madeira, sf_hawaii, all", *presetFilter)
		}
	}

	results := make([]BenchmarkResult, 0, len(selectedPresets))

	for _, preset := range selectedPresets {
		log.Printf("\n--> Benchmarking Preset: %s (%s)", preset.Name, preset.TimeStep)
		directNM := geo.DistanceNM(preset.Start, preset.Dest)

		polarTable := polar.GetPresetPolar(preset.BoatID)
		cfg := isochrone.DefaultRouterConfig()
		cfg.TimeStep = preset.TimeStep

		// Warmup runs
		for w := 0; w < *warmup; w++ {
			_, err := isochrone.CalculateOptimalRoute(
				preset.Start,
				preset.Dest,
				startTime,
				polarTable,
				weatherEngine,
				landMask,
				cfg,
			)
			if err != nil {
				log.Printf("Warning: warmup run failed: %v", err)
			}
		}

		// Timed iterations
		durations := make([]time.Duration, *iterations)
		var lastResult *isochrone.RouteResult
		var memStatsBefore, memStatsAfter runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&memStatsBefore)

		for i := 0; i < *iterations; i++ {
			iterStart := time.Now()
			route, err := isochrone.CalculateOptimalRoute(
				preset.Start,
				preset.Dest,
				startTime,
				polarTable,
				weatherEngine,
				landMask,
				cfg,
			)
			durations[i] = time.Since(iterStart)
			if err != nil {
				log.Printf("Error during iteration %d: %v", i+1, err)
			} else {
				lastResult = route
			}
		}

		runtime.ReadMemStats(&memStatsAfter)

		// Calculate statistics
		timesMs := make([]float64, *iterations)
		var sumMs float64
		for i, d := range durations {
			ms := float64(d.Microseconds()) / 1000.0
			timesMs[i] = ms
			sumMs += ms
		}
		sort.Float64s(timesMs)

		minMs := timesMs[0]
		maxMs := timesMs[len(timesMs)-1]
		meanMs := sumMs / float64(*iterations)
		medianMs := timesMs[len(timesMs)/2]

		var varianceSum float64
		for _, ms := range timesMs {
			diff := ms - meanMs
			varianceSum += diff * diff
		}
		stdDevMs := math.Sqrt(varianceSum / float64(*iterations))
		throughput := 1000.0 / meanMs

		allocBytes := (memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc) / uint64(*iterations)
		allocCount := (memStatsAfter.Mallocs - memStatsBefore.Mallocs) / uint64(*iterations)

		res := BenchmarkResult{
			PresetID:         preset.ID,
			PresetName:       preset.Name,
			BoatName:         preset.BoatName,
			TimeStepStr:      preset.TimeStep.String(),
			DirectDistanceNM: directNM,
			Iterations:       *iterations,
			MinTimeMs:        minMs,
			MeanTimeMs:       meanMs,
			MedianTimeMs:     medianMs,
			MaxTimeMs:        maxMs,
			StdDevMs:         stdDevMs,
			ThroughputPerSec: throughput,
			AllocBytesPerOp:  allocBytes,
			AllocsPerOp:      allocCount,
			Success:          lastResult != nil && lastResult.DestinationReached,
		}

		if lastResult != nil {
			res.RouteDistanceNM = lastResult.TotalDistanceNM
			res.DurationHours = lastResult.TotalDurationHours
			res.WaypointsCount = len(lastResult.Waypoints)
			res.WavefrontSteps = len(lastResult.Isochrones)
			res.TotalTacks = lastResult.TotalTacks
			res.TotalGybes = lastResult.TotalGybes
			res.MaxWindKts = lastResult.MaxWindEncountered
		}

		results = append(results, res)

		log.Printf("    Mean: %.2f ms | Min: %.2f ms | Median: %.2f ms | Max: %.2f ms (±%.2f ms)",
			meanMs, minMs, medianMs, maxMs, stdDevMs)
		log.Printf("    Throughput: %.1f routes/sec | Memory: %.2f MB/op | Allocs: %d/op",
			throughput, float64(allocBytes)/(1024*1024), allocCount)
		log.Printf("    Route: %.1f NM in %.1f h | Waypoints: %d | Isochrone Waves: %d | Tacks: %d",
			res.RouteDistanceNM, res.DurationHours, res.WaypointsCount, res.WavefrontSteps, res.TotalTacks)
	}

	// Memory profile if requested
	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			log.Fatalf("Could not create memory profile: %v", err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatalf("Could not write memory profile: %v", err)
		}
		log.Printf("Memory profile written -> %s", *memProfile)
	}

	// Print Summary Table
	fmt.Println("\n" + formatSummaryTable(results))

	// Save JSON if requested
	if *jsonOutput != "" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal json: %v", err)
		}
		if err := os.WriteFile(*jsonOutput, data, 0644); err != nil {
			log.Fatalf("Failed to write json file: %v", err)
		}
		log.Printf("JSON benchmark metrics saved -> %s", *jsonOutput)
	}
}

func formatSummaryTable(results []BenchmarkResult) string {
	var out string
	out += "========================================================================================================\n"
	out += fmt.Sprintf("%-28s | %-6s | %-9s | %-9s | %-9s | %-8s | %-10s | %-9s\n",
		"Preset Scenario", "Step", "Direct NM", "Route NM", "Time (h)", "Mean ms", "Throughput", "Memory/Op")
	out += "-----------------------------+--------+-----------+-----------+-----------+----------+------------+----------\n"
	for _, r := range results {
		out += fmt.Sprintf("%-28s | %-6s | %8.1f  | %8.1f  | %8.1f  | %7.2fms | %7.1f/s  | %6.2f MB\n",
			truncate(r.PresetName, 28),
			r.TimeStepStr,
			r.DirectDistanceNM,
			r.RouteDistanceNM,
			r.DurationHours,
			r.MeanTimeMs,
			r.ThroughputPerSec,
			float64(r.AllocBytesPerOp)/(1024*1024),
		)
	}
	out += "========================================================================================================\n"
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
