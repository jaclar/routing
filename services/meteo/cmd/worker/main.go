package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sailboat/meteo/internal/driver"
	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/scheduler"
	"sailboat/meteo/internal/zarr"
)

func main() {
	defaultDataDir := "./data/store"
	if envDir := os.Getenv("DATA_DIR"); envDir != "" {
		defaultDataDir = envDir
	}

	var (
		dataDir     = flag.String("data-dir", defaultDataDir, "Root path for Zarr store")
		daemonMode  = flag.Bool("daemon", false, "Run continuous ingestion daemon")
		modelFlag   = flag.String("model", "gfs_0p25", "Model to ingest: gfs_0p25, ifs_0p25, icon_global")
		varsFlag    = flag.String("variables", "wind_u_10m,wind_v_10m,wind_gust_10m,mslp,temp_2m,precip_accum", "Comma-separated canonical variables")
		concurrency = flag.Int("concurrency", 8, "Number of concurrent slice fetch workers")
		pollMinutes = flag.Int("poll-interval", 10, "Polling interval in minutes for daemon mode")
	)
	flag.Parse()

	mgr, err := zarr.NewStoreManager(*dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize store manager at %s: %v", *dataDir, err)
	}

	// Register drivers
	drivers := []driver.ModelDriver{
		driver.NewGFSDriver(nil),
		driver.NewECMWFDriver(model.ModelIFS025, nil),
		driver.NewICONDriver(nil),
	}

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
			PollInterval: time.Duration(*pollMinutes) * time.Minute,
			Concurrency:  *concurrency,
			Variables:    varList,
			Retention:    2,
		}
		d := scheduler.NewIngestionDaemon(daemonCfg, mgr, drivers)
		d.Start(ctx)
		return
	}

	// Manual single-run mode
	var targetDriver driver.ModelDriver
	for _, d := range drivers {
		if strings.EqualFold(d.ModelID(), *modelFlag) {
			targetDriver = d
			break
		}
	}

	if targetDriver == nil {
		log.Fatalf("Unknown model %q (available: gfs_0p25, ifs_0p25, icon_global)", *modelFlag)
	}

	log.Printf("Discovering latest cycle for %s...", targetDriver.ModelID())
	cycle, err := targetDriver.CheckLatestCycle(ctx)
	if err != nil {
		log.Fatalf("Failed to discover cycle: %v", err)
	}

	log.Printf("Ingesting cycle: %s (%s)", cycle.ModelName, cycle.ReferenceTime.Format("2006-01-02 15:04 UTC"))
	if err := scheduler.IngestCycle(ctx, targetDriver, mgr, cycle, varList, *concurrency); err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}

	log.Println("Ingestion completed successfully.")
}
