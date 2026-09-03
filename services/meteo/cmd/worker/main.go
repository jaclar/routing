package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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
