// Package status tracks in-progress ingestion cycles in memory so a debug
// endpoint can report live download/write progress alongside the on-disk
// state of finalized Zarr stores.
package status

import (
	"sync"
	"time"
)

// CycleProgress describes the live state of a single model's in-flight ingestion cycle.
type CycleProgress struct {
	ModelID         string     `json:"model_id"`
	ModelName       string     `json:"model_name"`
	Cycle           string     `json:"cycle"`
	ReferenceTime   time.Time  `json:"reference_time"`
	Stage           string     `json:"stage"` // "downloading", "compressing", "failed"
	TotalSlices     int        `json:"total_slices"`
	CompletedSlices int64      `json:"completed_slices"`
	PercentComplete float64    `json:"percent_complete"`
	StartedAt       time.Time  `json:"started_at"`
	DownloadEndedAt *time.Time `json:"download_ended_at,omitempty"`
	StagingDir      string     `json:"staging_dir"`
	Error           string     `json:"error,omitempty"`
}

// Tracker holds the current in-progress cycle per model ID.
type Tracker struct {
	mu    sync.RWMutex
	items map[string]*CycleProgress
}

// NewTracker creates an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{items: make(map[string]*CycleProgress)}
}

// Start records the beginning of a new ingestion cycle for a model, replacing any prior entry.
func (t *Tracker) Start(modelID, modelName, cycle string, referenceTime time.Time, stagingDir string, totalSlices int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items[modelID] = &CycleProgress{
		ModelID:       modelID,
		ModelName:     modelName,
		Cycle:         cycle,
		ReferenceTime: referenceTime,
		Stage:         "downloading",
		TotalSlices:   totalSlices,
		StartedAt:     time.Now().UTC(),
		StagingDir:    stagingDir,
	}
}

// SetCompleted updates the number of slices downloaded and written to staging so far.
func (t *Tracker) SetCompleted(modelID string, completed int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	it, ok := t.items[modelID]
	if !ok {
		return
	}
	it.CompletedSlices = completed
	if it.TotalSlices > 0 {
		it.PercentComplete = float64(completed) / float64(it.TotalSlices) * 100.0
	}
}

// MarkDownloadEnded records that all slices have finished downloading and the cycle has moved to compression/finalization.
func (t *Tracker) MarkDownloadEnded(modelID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	it, ok := t.items[modelID]
	if !ok {
		return
	}
	now := time.Now().UTC()
	it.DownloadEndedAt = &now
	it.Stage = "compressing"
}

// MarkFailed records that the cycle aborted with an error.
func (t *Tracker) MarkFailed(modelID string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	it, ok := t.items[modelID]
	if !ok {
		return
	}
	it.Stage = "failed"
	if err != nil {
		it.Error = err.Error()
	}
}

// Clear removes a model's in-progress entry, typically once the cycle has been promoted to a finalized store.
func (t *Tracker) Clear(modelID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.items, modelID)
}

// Snapshot returns a copy of all currently tracked cycles.
func (t *Tracker) Snapshot() []CycleProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]CycleProgress, 0, len(t.items))
	for _, it := range t.items {
		out = append(out, *it)
	}
	return out
}
