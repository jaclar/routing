package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"sailboat/meteo/internal/driver"
	"sailboat/meteo/internal/model"
	"sailboat/meteo/internal/status"
	"sailboat/meteo/internal/zarr"
)

// Progress tracks in-flight ingestion cycles across all models, for the worker's debug/status endpoint.
var Progress = status.NewTracker()

// IngestCycle performs a full cycle ingestion: discovers slices, downloads/decodes in parallel, and writes to staging store.
func IngestCycle(ctx context.Context, drv driver.ModelDriver, mgr *zarr.StoreManager, cycle *model.ModelCycle, variables []string, concurrency int, storeFullEnsemble bool) error {
	tag := fmt.Sprintf("[%s %s]", cycle.ModelName, cycle.ReferenceTime.Format("2006-01-02 15:04 UTC"))

	log.Printf("[Ingest]%s Starting cycle ingestion (%d forecast steps, %d variables, storeFullEnsemble=%v)",
		tag, len(cycle.ForecastSteps), len(variables), storeFullEnsemble)

	if concurrency <= 0 {
		concurrency = 8
	}

	// 1. Discover all slices
	tasks, err := drv.DiscoverSlices(cycle, variables)
	if err != nil {
		return fmt.Errorf("failed to discover slices for %s: %w", tag, err)
	}
	log.Printf("[Ingest]%s Discovered %d slice tasks to fetch", tag, len(tasks))

	// Global 0.25° grid bounds (90 to -90 Lat, 0 to 359.75 Lon)
	latStart, latEnd, latStep := 90.0, -90.0, cycle.ResolutionDeg
	lonStart, lonEnd, lonStep := 0.0, 360.0-cycle.ResolutionDeg, cycle.ResolutionDeg

	// 2. Create staging Zarr writer
	writer, stagingDir, err := mgr.CreateStagingWriter(cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, variables, storeFullEnsemble)
	if err != nil {
		return fmt.Errorf("failed to create staging writer for %s: %w", tag, err)
	}

	Progress.Start(cycle.ModelName, cycle.ModelName, zarr.CycleSlug(cycle.ReferenceTime), cycle.ReferenceTime, stagingDir, len(tasks))

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

				// Retry transient errors up to 8 times with exponential backoff and randomized jitter
				const maxAttempts = 8
				var slice *model.RawGridSlice
				var fetchErr error
				for attempt := 0; attempt < maxAttempts; attempt++ {
					if attempt > 0 {
						backoffMs := 1000 * (1 << (attempt - 1))
						if backoffMs > 30000 {
							backoffMs = 30000
						}
						jitterMs := int(time.Now().UnixNano() % 500)
						sleepDuration := time.Duration(backoffMs+jitterMs) * time.Millisecond
						log.Printf("[Ingest]%s Worker %d transient retry (%d/%d) for %s step %d in %v: %v",
							tag, workerID, attempt, maxAttempts, task.Variable, task.StepHours, sleepDuration.Round(time.Millisecond), fetchErr)
						select {
						case <-ctx.Done():
							return
						case <-time.After(sleepDuration):
						}
					}

					slice, fetchErr = drv.IngestSlice(ctx, task)
					if fetchErr == nil {
						break
					}

					// If file is not found (HTTP 404, NoSuchKey, or missing index pattern), abort immediately without retrying
					if isNotFoundError(fetchErr) {
						log.Printf("[Ingest]%s Worker %d: %s step %d not yet available upstream: %v",
							tag, workerID, task.Variable, task.StepHours, fetchErr)
						break
					}
				}

				if fetchErr != nil {
					log.Printf("[Ingest]%s Worker %d error fetching %s step %d: %v", tag, workerID, task.Variable, task.StepHours, fetchErr)
					errOnce.Do(func() {
						firstErr = fetchErr
					})
					return
				}

				if err := writer.WriteSlice(slice); err != nil {
					log.Printf("[Ingest]%s Worker %d error writing slice %s step %d: %v", tag, workerID, task.Variable, task.StepHours, err)
					errOnce.Do(func() {
						firstErr = err
					})
					return
				}

				countMu.Lock()
				completedCount++
				Progress.SetCompleted(cycle.ModelName, completedCount)
				if completedCount%10 == 0 || completedCount == int64(len(tasks)) {
					log.Printf("[Ingest]%s Progress: %d/%d slices processed (%.1f%%)",
						tag, completedCount, len(tasks), float64(completedCount)/float64(len(tasks))*100.0)
				}
				countMu.Unlock()

				// Slight pacing delay to prevent micro-burst rate limits on upstream CDNs
				time.Sleep(50 * time.Millisecond)
			}
		}(w)
	}

	wg.Wait()

	if firstErr != nil {
		Progress.MarkFailed(cycle.ModelName, firstErr)
		_ = os.RemoveAll(stagingDir)
		return fmt.Errorf("cycle ingestion aborted for %s (upstream files still in-flight or incomplete): %w", tag, firstErr)
	}

	writer.MarkDownloadComplete()
	Progress.MarkDownloadEnded(cycle.ModelName)

	// 4. Finalize staging store
	log.Printf("[Ingest]%s Packing and compressing Zarr store...", tag)
	if err := writer.Finalize(); err != nil {
		Progress.MarkFailed(cycle.ModelName, err)
		_ = os.RemoveAll(stagingDir)
		return fmt.Errorf("failed to finalize staging store for %s: %w", tag, err)
	}

	// 5. Atomically promote staging store and update symlink
	log.Printf("[Ingest]%s Promoting staging store to permanent cycle %s...", tag, zarr.CycleSlug(cycle.ReferenceTime))
	if err := mgr.PromoteStagingStore(cycle.ModelName, cycle.ReferenceTime, stagingDir); err != nil {
		Progress.MarkFailed(cycle.ModelName, err)
		_ = os.RemoveAll(stagingDir)
		return fmt.Errorf("failed to promote store for %s: %w", tag, err)
	}

	Progress.Clear(cycle.ModelName)

	// 6. Prune old runs (keep latest 2)
	_ = mgr.PruneOldCycles(cycle.ModelName, 2)

	log.Printf("[Ingest]%s Successfully completed cycle ingestion", tag)
	return nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "404") ||
		strings.Contains(s, "not found") ||
		strings.Contains(s, "nosuchkey")
}
