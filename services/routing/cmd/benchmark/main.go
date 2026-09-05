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

type StrategyInfo struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Strategy    isochrone.PruningStrategy `json:"strategy"`
	Description string                    `json:"description"`
}

type BenchmarkResult struct {
	StrategyID       string    `json:"strategy_id"`
	StrategyName     string    `json:"strategy_name"`
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

type WavefrontExport struct {
	PresetID     string                    `json:"preset_id"`
	PresetName   string                    `json:"preset_name"`
	Start        geo.Point                 `json:"start"`
	Dest         geo.Point                 `json:"dest"`
	TimeStep     string                    `json:"time_step"`
	DirectDistNM float64                   `json:"direct_distance_nm"`
	Runs         map[string]StrategyOutput `json:"runs"`
}

type StrategyOutput struct {
	StrategyID    string                    `json:"strategy_id"`
	StrategyName  string                    `json:"strategy_name"`
	TotalDistance float64                   `json:"total_distance_nm"`
	TotalDuration float64                   `json:"total_duration_hours"`
	AverageSpeed  float64                   `json:"average_speed_kts"`
	TotalTacks    int                       `json:"total_tacks"`
	TotalGybes    int                       `json:"total_gybes"`
	Reached       bool                      `json:"destination_reached"`
	Waypoints     []isochrone.Waypoint      `json:"waypoints"`
	Isochrones    []isochrone.IsochroneWave `json:"isochrones"`
	MeanTimeMs    float64                   `json:"mean_time_ms"`
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
			BoatName:  "36ft Racing Sloop",
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
			BoatName:  "40ft Cruiser",
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

func getAllStrategies() []StrategyInfo {
	return []StrategyInfo{
		{
			ID:          "radial_sector",
			Name:        "1. Radial Sector",
			Strategy:    isochrone.PruningRadialSector,
			Description: "Classic Chichester angular sector bucketing",
		},
		{
			ID:          "spatial_grid",
			Name:        "2. 2D Spatial Grid",
			Strategy:    isochrone.PruningSpatialGrid,
			Description: "2D Local Spatial Dominance (Default)",
		},
		{
			ID:          "astar_beam",
			Name:        "3. A* Beam Search",
			Strategy:    isochrone.PruningAStarBeam,
			Description: "Heuristic A* cost sorting with spatial diversity",
		},
		{
			ID:          "pareto_envelope",
			Name:        "4. Pareto Envelope",
			Strategy:    isochrone.PruningParetoEnvelope,
			Description: "Non-dominated progress envelope along course",
		},
		{
			ID:          "state_space_grid",
			Name:        "5. State-Space Grid",
			Strategy:    isochrone.PruningStateSpaceGrid,
			Description: "Grid bucketing with Tack and Point of Sail modes",
		},
	}
}

func main() {
	cpuProfile := flag.String("cpuprofile", "", "Write cpu profile to file")
	memProfile := flag.String("memprofile", "", "Write memory profile to file")
	presetFilter := flag.String("preset", "all", "Preset ID to benchmark or 'all'")
	strategyFilter := flag.String("strategy", "all", "Pruning strategy ID to benchmark or 'all'")
	iterations := flag.Int("iterations", 5, "Number of benchmark iterations per preset")
	warmup := flag.Int("warmup", 1, "Number of warmup iterations per preset")
	jsonOutput := flag.String("json", "", "Save JSON metrics to file")
	wavefrontOutput := flag.String("wavefronts", "", "Save wavefront export data to JSON file")
	flag.Parse()

	log.Println("==================================================================")
	log.Println("     ISOCHRONE ROUTING PRUNING STRATEGIES BENCHMARK SUITE         ")
	log.Println("==================================================================")

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
			log.Fatalf("Unknown preset ID '%s'", *presetFilter)
		}
	}

	allStrats := getAllStrategies()
	var selectedStrats []StrategyInfo

	if *strategyFilter == "all" || *strategyFilter == "" {
		selectedStrats = allStrats
	} else {
		for _, s := range allStrats {
			if s.ID == *strategyFilter {
				selectedStrats = append(selectedStrats, s)
			}
		}
		if len(selectedStrats) == 0 {
			log.Fatalf("Unknown strategy ID '%s'", *strategyFilter)
		}
	}

	vppURL := os.Getenv("VPP_SERVICE_URL")
	if vppURL == "" {
		vppURL = "http://localhost:4001"
	}
	vppClient := polar.NewVPPClient(vppURL)

	results := make([]BenchmarkResult, 0)
	wavefrontExports := make(map[string]WavefrontExport)

	for _, preset := range selectedPresets {
		log.Printf("\n==================================================================")
		log.Printf(">>> SCENARIO: %s (TimeStep: %s)", preset.Name, preset.TimeStep)
		log.Printf("==================================================================")
		directNM := geo.DistanceNM(preset.Start, preset.Dest)
		// Polars come from the VPP service, the same as in production; the client caches
		// each preset after its first fetch so repeated scenarios do not re-solve it.
		polarTable, err := vppClient.FetchPolar(preset.BoatID, nil)
		if err != nil {
			log.Fatalf("could not fetch polar for %s from the VPP service at %s: %v\n"+
				"Set VPP_SERVICE_URL, or start the service, and try again.",
				preset.BoatID, vppURL, err)
		}

		wfExport := WavefrontExport{
			PresetID:     preset.ID,
			PresetName:   preset.Name,
			Start:        preset.Start,
			Dest:         preset.Dest,
			TimeStep:     preset.TimeStep.String(),
			DirectDistNM: directNM,
			Runs:         make(map[string]StrategyOutput),
		}

		for _, strat := range selectedStrats {
			cfg := isochrone.DefaultRouterConfig()
			cfg.TimeStep = preset.TimeStep
			cfg.PruningStrategy = strat.Strategy

			// Warmup runs
			for w := 0; w < *warmup; w++ {
				_, _ = isochrone.CalculateOptimalRoute(
					preset.Start,
					preset.Dest,
					startTime,
					polarTable,
					weatherEngine,
					landMask,
					cfg,
				)
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
				if err == nil {
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
				StrategyID:       strat.ID,
				StrategyName:     strat.Name,
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

				wfExport.Runs[strat.ID] = StrategyOutput{
					StrategyID:    strat.ID,
					StrategyName:  strat.Name,
					TotalDistance: lastResult.TotalDistanceNM,
					TotalDuration: lastResult.TotalDurationHours,
					AverageSpeed:  lastResult.AverageSpeedKts,
					TotalTacks:    lastResult.TotalTacks,
					TotalGybes:    lastResult.TotalGybes,
					Reached:       lastResult.DestinationReached,
					Waypoints:     lastResult.Waypoints,
					Isochrones:    lastResult.Isochrones,
					MeanTimeMs:    meanMs,
				}
			}

			results = append(results, res)

			log.Printf("  [%-18s] Mean: %6.2f ms | Dist: %6.1f NM | Time: %5.1f h | Allocs: %7d | Reached: %v",
				strat.Name, meanMs, res.RouteDistanceNM, res.DurationHours, allocCount, res.Success)
		}

		wavefrontExports[preset.ID] = wfExport
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
	fmt.Println("\n" + formatMatrixSummaryTable(results))

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

	// Save Wavefront export data if requested
	if *wavefrontOutput != "" {
		data, err := json.MarshalIndent(wavefrontExports, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal wavefronts json: %v", err)
		}
		if err := os.WriteFile(*wavefrontOutput, data, 0644); err != nil {
			log.Fatalf("Failed to write wavefronts json file: %v", err)
		}
		log.Printf("Wavefront export data saved -> %s", *wavefrontOutput)
	}
}

func formatMatrixSummaryTable(results []BenchmarkResult) string {
	var out string
	out += "========================================================================================================================\n"
	out += fmt.Sprintf("%-22s | %-19s | %-9s | %-9s | %-9s | %-9s | %-10s | %-7s\n",
		"Scenario", "Pruning Strategy", "Mean ms", "Route NM", "Time (h)", "Memory/Op", "Allocs/Op", "Status")
	out += "-----------------------+---------------------+-----------+-----------+-----------+-----------+------------+---------\n"
	for _, r := range results {
		status := "PASS"
		if !r.Success {
			status = "FAIL"
		}
		out += fmt.Sprintf("%-22s | %-19s | %7.2fms | %7.1fNM | %7.1fh  | %6.2f MB | %9d  | %-7s\n",
			truncate(r.PresetName, 22),
			r.StrategyName,
			r.MeanTimeMs,
			r.RouteDistanceNM,
			r.DurationHours,
			float64(r.AllocBytesPerOp)/(1024*1024),
			r.AllocsPerOp,
			status,
		)
	}
	out += "========================================================================================================================\n"
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
