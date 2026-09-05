package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"sailboat/meteo/internal/driver"
	"sailboat/meteo/internal/scheduler"
	"sailboat/meteo/internal/zarr"
)

func main() {
	defaultDataDir := "./data/store"
	if envDir := os.Getenv("DATA_DIR"); envDir != "" {
		defaultDataDir = envDir
	}

	defaultStoreFullEnsemble := false
	if envFull := os.Getenv("STORE_FULL_ENSEMBLE"); envFull != "" {
		defaultStoreFullEnsemble = strings.EqualFold(envFull, "true") || envFull == "1"
	}

	var (
		dataDir           = flag.String("data-dir", defaultDataDir, "Root path for Zarr store")
		daemonMode        = flag.Bool("daemon", false, "Run continuous ingestion daemon")
		modelFlag         = flag.String("model", "gfs_0p25", "Model to ingest: gfs_0p25, ifs_0p25, icon_global, gefs_0p50, ifs_ens_0p25, icon_eps_global, ensemble, deterministic, all")
		varsFlag          = flag.String("variables", "wind_u_10m,wind_v_10m,wind_gust_10m,mslp,temp_2m,precip_accum", "Comma-separated canonical variables")
		concurrency       = flag.Int("concurrency", 4, "Number of concurrent slice fetch workers per model")
		pollMinutes       = flag.Int("poll-interval", 10, "Polling interval in minutes for daemon mode")
		storeFullEnsemble = flag.Bool("store-full-ensemble", defaultStoreFullEnsemble, "Store raw 4D individual ensemble member slices (defaults to false; only statistical summaries are stored for Strategy A to minimize storage costs)")
	)
	flag.Parse()

	mgr, err := zarr.NewStoreManager(*dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize store manager at %s: %v", *dataDir, err)
	}

	debugPort := os.Getenv("DEBUG_PORT")
	if debugPort == "" {
		debugPort = "4082"
	}
	startDebugServer(debugPort, mgr)

	// Register all available drivers (deterministic + ensemble)
	drivers := driver.NewAllDrivers(nil)

	var varList []string
	for _, v := range strings.Split(*varsFlag, ",") {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			varList = append(varList, trimmed)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *daemonMode {
		daemonCfg := scheduler.DaemonConfig{
			PollInterval:      time.Duration(*pollMinutes) * time.Minute,
			Concurrency:       *concurrency,
			Variables:         varList,
			Retention:         2,
			StoreFullEnsemble: *storeFullEnsemble,
		}
		d := scheduler.NewIngestionDaemon(daemonCfg, mgr, drivers)
		d.Start(ctx)
		return
	}

	// Manual multi/single-run mode
	var targetDrivers []driver.ModelDriver
	if strings.EqualFold(*modelFlag, "all") || *modelFlag == "" {
		targetDrivers = drivers
	} else if strings.EqualFold(*modelFlag, "deterministic") {
		targetDrivers = driver.NewDeterministicDrivers(nil)
	} else if strings.EqualFold(*modelFlag, "ensemble") {
		targetDrivers = driver.NewEnsembleDrivers(nil)
	} else {
		requested := strings.Split(*modelFlag, ",")
		for _, req := range requested {
			req = strings.TrimSpace(req)
			found := false
			for _, d := range drivers {
				if strings.EqualFold(d.ModelID(), req) {
					targetDrivers = append(targetDrivers, d)
					found = true
					break
				}
			}
			if !found {
				log.Fatalf("Unknown model %q (available: gfs_0p25, ifs_0p25, icon_global, gefs_0p50, ifs_ens_0p25, icon_eps_global, ensemble, deterministic, all)", req)
			}
		}
	}

	var wg sync.WaitGroup
	for _, targetDriver := range targetDrivers {
		wg.Add(1)
		go func(drv driver.ModelDriver) {
			defer wg.Done()
			log.Printf("Discovering latest cycle for %s...", drv.ModelID())
			cycle, err := drv.CheckLatestCycle(ctx)
			if err != nil {
				log.Printf("[ERROR] Failed to discover cycle for %s: %v", drv.ModelID(), err)
				return
			}

			log.Printf("Ingesting cycle for %s: %s (%s)", drv.ModelID(), cycle.ModelName, cycle.ReferenceTime.Format("2006-01-02 15:04 UTC"))
			if err := scheduler.IngestCycle(ctx, drv, mgr, cycle, varList, *concurrency, *storeFullEnsemble); err != nil {
				log.Printf("[ERROR] Ingestion failed for %s: %v", drv.ModelID(), err)
				return
			}
			log.Printf("Ingestion completed successfully for %s.", drv.ModelID())
		}(targetDriver)
	}
	wg.Wait()
	log.Println("Manual ingestion run finished.")
}

// startDebugServer exposes a read-only status endpoint reporting which model cycles are
// fully downloaded and stored on disk (with size) vs. currently in progress (with a completion
// percentage) and the relevant start/download-end/write-end timestamps for each.
func startDebugServer(port string, mgr *zarr.StoreManager) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	// Sizing every store touches a lot of files the first time. Bound it so a slow or large
	// volume degrades the response instead of hanging the request.
	r.Use(middleware.Timeout(20 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "meteo-worker",
		})
	})

	r.Get("/debug/status", func(w http.ResponseWriter, req *http.Request) {
		finalized, err := mgr.ScanModelStores(req.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":  true,
				"reason": err.Error(),
			})
			return
		}

		inProgress := scheduler.Progress.Snapshot()
		enriched := make([]map[string]any, 0, len(inProgress))
		for _, p := range inProgress {
			stagingSizeBytes, _ := zarr.DirSize(p.StagingDir)
			enriched = append(enriched, map[string]any{
				"model_id":           p.ModelID,
				"cycle":              p.Cycle,
				"reference_time":     p.ReferenceTime,
				"stage":              p.Stage,
				"total_slices":       p.TotalSlices,
				"completed_slices":   p.CompletedSlices,
				"percent_complete":   p.PercentComplete,
				"started_at":         p.StartedAt,
				"download_ended_at":  p.DownloadEndedAt,
				"staging_size_bytes": stagingSizeBytes,
				"error":              p.Error,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"finalized":   finalized,
			"in_progress": enriched,
		})
	})

	go func() {
		log.Printf("[Debug] Worker debug/status server listening on :%s", port)
		if err := http.ListenAndServe(":"+port, r); err != nil {
			log.Printf("[Debug] server exited: %v", err)
		}
	}()
}
