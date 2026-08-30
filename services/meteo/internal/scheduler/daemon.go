package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"sailboat/meteo/internal/driver"
	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/zarr"
)

// DaemonConfig defines polling parameters and target models.
type DaemonConfig struct {
	PollInterval time.Duration
	Concurrency  int
	Variables    []string
	Retention    int
}

// IngestionDaemon runs continuously in the background, checking upstream model runs and ingesting new cycles.
type IngestionDaemon struct {
	cfg          DaemonConfig
	storeManager *zarr.StoreManager
	drivers      map[string]driver.ModelDriver
	lastCycles   map[string]time.Time
	mu           sync.Mutex
}

// NewIngestionDaemon creates an ingestion daemon.
func NewIngestionDaemon(cfg DaemonConfig, mgr *zarr.StoreManager, drivers []driver.ModelDriver) *IngestionDaemon {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Minute
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 8
	}
	if len(cfg.Variables) == 0 {
		cfg.Variables = []string{
			model.VarWindU10m,
			model.VarWindV10m,
			model.VarWindGust10m,
			model.VarMSLP,
			model.VarTemp2m,
			model.VarPrecipAccum,
		}
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 2
	}

	driverMap := make(map[string]driver.ModelDriver, len(drivers))
	lastCycles := make(map[string]time.Time)
	for _, d := range drivers {
		driverMap[d.ModelID()] = d
		if latestOnDisk, exists, err := mgr.GetLatestCycleTime(d.ModelID()); err == nil && exists {
			lastCycles[d.ModelID()] = latestOnDisk
			log.Printf("[Daemon] Loaded existing active cycle for %s: %s",
				d.ModelID(), latestOnDisk.Format("2006-01-02 15:04 UTC"))
		}
	}

	return &IngestionDaemon{
		cfg:          cfg,
		storeManager: mgr,
		drivers:      driverMap,
		lastCycles:   lastCycles,
	}
}

// Start begins background polling until context is cancelled.
func (d *IngestionDaemon) Start(ctx context.Context) {
	log.Printf("[Daemon] Ingestion daemon started (Poll Interval: %v, Concurrency: %d, Models: %d)",
		d.cfg.PollInterval, d.cfg.Concurrency, len(d.drivers))

	// Initial check on startup
	d.checkAllModels(ctx)

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Daemon] Ingestion daemon stopping...")
			return
		case <-ticker.C:
			d.checkAllModels(ctx)
		}
	}
}

func (d *IngestionDaemon) checkAllModels(ctx context.Context) {
	for modelID, drv := range d.drivers {
		if ctx.Err() != nil {
			return
		}

		latestCycle, err := drv.CheckLatestCycle(ctx)
		if err != nil {
			log.Printf("[Daemon] Error checking latest cycle for %s: %v", modelID, err)
			continue
		}

		d.mu.Lock()
		lastCycleTime, exists := d.lastCycles[modelID]
		if !exists {
			if latestOnDisk, onDiskExists, err := d.storeManager.GetLatestCycleTime(modelID); err == nil && onDiskExists {
				lastCycleTime = latestOnDisk
				exists = true
				d.lastCycles[modelID] = latestOnDisk
			}
		}
		d.mu.Unlock()

		if !exists || latestCycle.ReferenceTime.After(lastCycleTime) {
			log.Printf("[Daemon] New cycle discovered for %s: %s (Previous: %s)",
				modelID, latestCycle.ReferenceTime.Format("2006-01-02 15:04 UTC"), lastCycleTime.Format("2006-01-02 15:04 UTC"))

			// Run ingestion
			err := IngestCycle(ctx, drv, d.storeManager, latestCycle, d.cfg.Variables, d.cfg.Concurrency)
			if err != nil {
				log.Printf("[Daemon] Ingestion failed for %s cycle %s: %v", modelID, latestCycle.ReferenceTime, err)
				continue
			}

			d.mu.Lock()
			d.lastCycles[modelID] = latestCycle.ReferenceTime
			d.mu.Unlock()
		} else {
			log.Printf("[Daemon] Model %s is up to date (Active cycle: %s)",
				modelID, lastCycleTime.Format("2006-01-02 15:04 UTC"))
		}
	}
}
