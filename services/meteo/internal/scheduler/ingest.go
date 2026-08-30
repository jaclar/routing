package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"sailboat/meteo/internal/driver"
	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/zarr"
)

// IngestCycle performs a full cycle ingestion: discovers slices, downloads/decodes in parallel, and writes to staging store.
func IngestCycle(ctx context.Context, drv driver.ModelDriver, mgr *zarr.StoreManager, cycle *model.ModelCycle, variables []string, concurrency int) error {
	log.Printf("[Ingest] Starting cycle ingestion for %s (Cycle: %s, %d forecast steps, %d variables)",
		cycle.ModelName, cycle.ReferenceTime.Format("2006-01-02 15:04 UTC"), len(cycle.ForecastSteps), len(variables))

	if concurrency <= 0 {
		concurrency = 8
	}

	// 1. Discover all slices
	tasks, err := drv.DiscoverSlices(cycle, variables)
	if err != nil {
		return fmt.Errorf("failed to discover slices: %w", err)
	}
	log.Printf("[Ingest] Discovered %d slice tasks to fetch", len(tasks))

	// Global 0.25° grid bounds (90 to -90 Lat, 0 to 359.75 Lon)
	latStart, latEnd, latStep := 90.0, -90.0, cycle.ResolutionDeg
	lonStart, lonEnd, lonStep := 0.0, 360.0 - cycle.ResolutionDeg, cycle.ResolutionDeg

	// 2. Create staging Zarr writer
	writer, stagingDir, err := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, variables)
	if err != nil {
		return fmt.Errorf("failed to create staging writer: %w", err)
	}

	// 3. Concurrently fetch and decode slices with worker pool
	taskChan := make(chan model.FetchTask, len(tasks))
	for _, t := range tasks {
		taskChan <- t
	}
	close(taskChan)

	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error

	var completedCount int64
	var countMu sync.Mutex

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				if ctx.Err() != nil {
					return
				}

				// Retry up to 3 times on transient network error
				var slice *model.RawGridSlice
				var fetchErr error
				for attempt := 0; attempt < 3; attempt++ {
					slice, fetchErr = drv.IngestSlice(ctx, task)
					if fetchErr == nil {
						break
					}
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				}

				if fetchErr != nil {
					log.Printf("[Ingest] Worker %d error fetching %s step %d: %v", workerID, task.Variable, task.StepHours, fetchErr)
					errOnce.Do(func() {
						firstErr = fetchErr
					})
					return
				}

				if err := writer.WriteSlice(slice); err != nil {
					log.Printf("[Ingest] Worker %d error writing slice: %v", workerID, err)
					errOnce.Do(func() {
						firstErr = err
					})
					return
				}

				countMu.Lock()
				completedCount++
				if completedCount%10 == 0 || completedCount == int64(len(tasks)) {
					log.Printf("[Ingest] Progress: %d/%d slices processed (%.1f%%)",
						completedCount, len(tasks), float64(completedCount)/float64(len(tasks))*100.0)
				}
				countMu.Unlock()
			}
		}(w)
	}

	wg.Wait()

	if firstErr != nil {
		_ = os.RemoveAll(stagingDir)
		return fmt.Errorf("cycle ingestion aborted (upstream files still in-flight or incomplete): %w", firstErr)
	}

	// 4. Finalize staging store
	log.Printf("[Ingest] Packing and compressing Zarr store...")
	if err := writer.Finalize(); err != nil {
		return fmt.Errorf("failed to finalize staging store: %w", err)
	}

	// 5. Atomically promote staging store and update symlink
	log.Printf("[Ingest] Promoting staging store to permanent cycle %s...", zarr.CycleSlug(cycle.ReferenceTime))
	if err := mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir); err != nil {
		return fmt.Errorf("failed to promote store: %w", err)
	}

	// 6. Prune old runs (keep latest 2)
	_ = mgr.PruneOldCycles(cycle.ModelName, 2)

	log.Printf("[Ingest] Successfully completed ingestion for %s (Cycle: %s)", cycle.ModelName, cycle.ReferenceTime.Format("2006-01-02 15:04 UTC"))
	return nil
}
