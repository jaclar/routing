package zarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"sailboat/meteo/internal/model"
)

// StoreManager coordinates lifecycle, atomic activation, and retention for model stores.
type StoreManager struct {
	BaseDir string

	// sizeCache memoizes the on-disk size of promoted cycle directories. A promoted store is
	// immutable until it is pruned, so each directory is only ever walked once per process.
	sizeMu    sync.Mutex
	sizeCache map[string]int64
}

// NewStoreManager initializes a StoreManager for a given root directory (e.g. data/store).
func NewStoreManager(baseDir string) (*StoreManager, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base store dir: %w", err)
	}
	return &StoreManager{BaseDir: baseDir}, nil
}

// ModelDir returns the directory path for a specific model (e.g. data/store/gfs_0p25).
func (m *StoreManager) ModelDir(modelID string) string {
	return filepath.Join(m.BaseDir, modelID)
}

// CycleSlug creates a directory-safe name for a forecast cycle (e.g. "20260830_06Z").
func CycleSlug(t time.Time) string {
	return fmt.Sprintf("%04d%02d%02d_%02dZ", t.Year(), t.Month(), t.Day(), t.Hour())
}

// CreateStagingWriter prepares a temporary staging Zarr store before atomic promotion.
func (m *StoreManager) CreateStagingWriter(cycle *model.ModelCycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep float64, variables []string, storeFullEnsemble bool) (*StoreWriter, string, error) {
	modelDir := m.ModelDir(cycle.ModelName)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return nil, "", err
	}

	slug := CycleSlug(cycle.ReferenceTime)
	stagingDir := filepath.Join(modelDir, fmt.Sprintf("%s.staging.zarr", slug))
	_ = os.RemoveAll(stagingDir) // Clean any previous failed attempt

	writer, err := NewStoreWriter(stagingDir, cycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep, variables, storeFullEnsemble)
	if err != nil {
		return nil, "", err
	}

	return writer, stagingDir, nil
}

// PromoteStagingStore atomically promotes staging Zarr store to permanent cycle and updates latest.zarr symlink.
func (m *StoreManager) PromoteStagingStore(modelID string, refTime time.Time, stagingDir string) error {
	modelDir := m.ModelDir(modelID)
	slug := CycleSlug(refTime)
	finalDir := filepath.Join(modelDir, fmt.Sprintf("%s.zarr", slug))

	// Remove any existing final directory with this cycle name
	_ = os.RemoveAll(finalDir)

	// Rename staging directory to final directory
	if err := os.Rename(stagingDir, finalDir); err != nil {
		return fmt.Errorf("failed to promote staging store to %s: %w", finalDir, err)
	}

	// Atomically update `latest.zarr` symlink
	latestSymlink := filepath.Join(modelDir, "latest.zarr")
	tmpSymlink := filepath.Join(modelDir, fmt.Sprintf(".latest.tmp.%d", time.Now().UnixNano()))

	targetBase := filepath.Base(finalDir)
	if err := os.Symlink(targetBase, tmpSymlink); err != nil {
		return fmt.Errorf("failed to create temporary symlink: %w", err)
	}

	if err := os.Rename(tmpSymlink, latestSymlink); err != nil {
		_ = os.Remove(tmpSymlink)
		return fmt.Errorf("failed to atomically update latest.zarr symlink: %w", err)
	}

	return nil
}

// OpenLatest loads the active `latest.zarr` dataset for a model.
func (m *StoreManager) OpenLatest(modelID string) (*Store, error) {
	latestPath := filepath.Join(m.ModelDir(modelID), "latest.zarr")
	return OpenStore(latestPath)
}

// PruneOldCycles keeps the most recent N cycles and deletes older runs to preserve disk space.
func (m *StoreManager) PruneOldCycles(modelID string, retainCount int) error {
	if retainCount < 1 {
		retainCount = 2
	}

	modelDir := m.ModelDir(modelID)
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cycleDirs []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && strings.HasSuffix(name, ".zarr") && name != "latest.zarr" && !strings.Contains(name, "staging") {
			cycleDirs = append(cycleDirs, filepath.Join(modelDir, name))
		}
	}

	sort.Strings(cycleDirs) // Lexicographical sort corresponds to timestamp (YYYYMMDD_HHZ)

	if len(cycleDirs) > retainCount {
		toDelete := cycleDirs[:len(cycleDirs)-retainCount]
		for _, dir := range toDelete {
			_ = os.RemoveAll(dir)
		}
	}

	return nil
}

// CycleSummary describes a finalized on-disk Zarr store for one model cycle, for debug/status reporting.
type CycleSummary struct {
	Cycle         string    `json:"cycle"`
	Path          string    `json:"path"`
	ReferenceTime time.Time `json:"reference_time"`
	IsLatest      bool      `json:"is_latest"`
	IsEnsemble    bool      `json:"is_ensemble"`
	NMembers      int       `json:"n_members"`
	StoreMembers  bool      `json:"store_members"`
	Variables     []string  `json:"variables"`
	SizeBytes     int64     `json:"size_bytes"`

	// Nil for stores written before ingestion timing was recorded. These are pointers
	// because encoding/json cannot omit a zero time.Time, which would otherwise report a
	// legacy store as having been ingested in year 1.
	IngestStartedAt *time.Time `json:"ingest_started_at"`
	DownloadEndedAt *time.Time `json:"download_ended_at"`
	WriteEndedAt    *time.Time `json:"write_ended_at"`
}

// ModelStoreSummary lists all finalized cycles currently on disk for one model.
type ModelStoreSummary struct {
	ModelID string         `json:"model_id"`
	Cycles  []CycleSummary `json:"cycles"`
}

// ScanModelStores reports every finalized (non-staging) Zarr store per model, including on-disk
// size and the ingest/download/write timestamps persisted in metadata.json. Sizing a store means
// walking every chunk file in it, so results are cached per cycle directory and computed in
// parallel; ctx bounds the first, uncached scan.
func (m *StoreManager) ScanModelStores(ctx context.Context) ([]ModelStoreSummary, error) {
	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []ModelStoreSummary
	for _, modelEntry := range entries {
		if !modelEntry.IsDir() {
			continue
		}
		modelID := modelEntry.Name()
		modelDir := filepath.Join(m.BaseDir, modelID)

		latestTarget := ""
		if target, err := os.Readlink(filepath.Join(modelDir, "latest.zarr")); err == nil {
			latestTarget = target
		}

		cycleEntries, err := os.ReadDir(modelDir)
		if err != nil {
			continue
		}

		summary := ModelStoreSummary{ModelID: modelID}
		for _, ce := range cycleEntries {
			name := ce.Name()
			if !ce.IsDir() || !strings.HasSuffix(name, ".zarr") || name == "latest.zarr" || strings.Contains(name, "staging") {
				continue
			}

			cyclePath := filepath.Join(modelDir, name)
			meta, err := readStoreMetadata(cyclePath)
			if err != nil {
				continue
			}

			summary.Cycles = append(summary.Cycles, CycleSummary{
				Cycle:           name,
				Path:            cyclePath,
				ReferenceTime:   meta.ReferenceTime,
				IsLatest:        name == latestTarget,
				IsEnsemble:      meta.IsEnsemble,
				NMembers:        len(meta.Members),
				StoreMembers:    meta.StoreMembers,
				Variables:       meta.Variables,
				IngestStartedAt: nonZeroTime(meta.IngestStartedAt),
				DownloadEndedAt: nonZeroTime(meta.DownloadEndedAt),
				WriteEndedAt:    nonZeroTime(meta.WriteEndedAt),
			})
		}

		sort.Slice(summary.Cycles, func(i, j int) bool {
			return summary.Cycles[i].ReferenceTime.After(summary.Cycles[j].ReferenceTime)
		})

		out = append(out, summary)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ModelID < out[j].ModelID })

	if err := m.fillSizes(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// fillSizes measures every listed cycle, in parallel and reusing cached measurements. A cycle
// whose size cannot be determined before ctx expires is reported with SizeBytes of -1 rather
// than failing the whole request.
func (m *StoreManager) fillSizes(ctx context.Context, models []ModelStoreSummary) error {
	type target struct{ model, cycle int }

	var targets []target
	for mi := range models {
		for ci := range models[mi].Cycles {
			targets = append(targets, target{mi, ci})
		}
	}
	if len(targets) == 0 {
		return nil
	}

	workers := runtime.NumCPU()
	if workers > len(targets) {
		workers = len(targets)
	}

	queue := make(chan target, len(targets))
	for _, t := range targets {
		queue <- t
	}
	close(queue)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range queue {
				cycle := &models[t.model].Cycles[t.cycle]
				if ctx.Err() != nil {
					cycle.SizeBytes = -1
					continue
				}
				cycle.SizeBytes = m.cachedDirSize(cycle.Path)
			}
		}()
	}
	wg.Wait()

	return nil
}

// cachedDirSize returns the on-disk size of a promoted cycle directory, walking it only the
// first time. Promoted stores never change, so a cached size cannot go stale; pruning simply
// leaves an unused entry behind.
func (m *StoreManager) cachedDirSize(path string) int64 {
	m.sizeMu.Lock()
	if size, ok := m.sizeCache[path]; ok {
		m.sizeMu.Unlock()
		return size
	}
	m.sizeMu.Unlock()

	size, err := dirSize(path)
	if err != nil {
		return -1
	}

	m.sizeMu.Lock()
	if m.sizeCache == nil {
		m.sizeCache = make(map[string]int64)
	}
	m.sizeCache[path] = size
	m.sizeMu.Unlock()

	return size
}

type storeMetadataFile struct {
	ReferenceTime   time.Time `json:"reference_time"`
	Members         []int     `json:"members,omitempty"`
	IsEnsemble      bool      `json:"is_ensemble,omitempty"`
	StoreMembers    bool      `json:"store_members"`
	Variables       []string  `json:"variables"`
	IngestStartedAt time.Time `json:"ingest_started_at"`
	DownloadEndedAt time.Time `json:"download_ended_at"`
	WriteEndedAt    time.Time `json:"write_ended_at"`
}

// nonZeroTime returns nil for a missing timestamp so it serializes as null rather than year 1.
func nonZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func readStoreMetadata(dir string) (*storeMetadataFile, error) {
	metaBytes, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var meta storeMetadataFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// dirSize sums the size of all regular files under path. It uses WalkDir rather than Walk so
// directory entries are not stat'ed twice, which matters across the tens of thousands of chunk
// files a single ensemble cycle contains.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

// DirSize exposes dirSize for callers outside this package (e.g. reporting live staging directory size).
func DirSize(path string) (int64, error) {
	return dirSize(path)
}

// ParseCycleSlug parses a slug like "20260830_06Z" back into time.Time.
func ParseCycleSlug(slug string) (time.Time, error) {
	slug = strings.TrimSuffix(slug, ".zarr")
	return time.Parse("20060102_15Z", slug)
}

// GetLatestCycleTime checks the local store to find the most recent successfully ingested cycle for a model.
func (m *StoreManager) GetLatestCycleTime(modelID string) (time.Time, bool, error) {
	modelDir := m.ModelDir(modelID)

	// 1. Check if latest.zarr exists and read its metadata
	store, err := m.OpenLatest(modelID)
	if err == nil {
		refTime := store.Cycle
		if !refTime.IsZero() {
			return refTime, true, nil
		}
	}

	// 2. Fallback: inspect directory entries for YYYYMMDD_HHZ.zarr folders
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}

	var validTimes []time.Time
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".zarr") && name != "latest.zarr" && !strings.Contains(name, "staging") {
			t, err := ParseCycleSlug(name)
			if err == nil {
				validTimes = append(validTimes, t)
			}
		}
	}

	if len(validTimes) == 0 {
		return time.Time{}, false, nil
	}

	sort.Slice(validTimes, func(i, j int) bool {
		return validTimes[i].Before(validTimes[j])
	})

	return validTimes[len(validTimes)-1], true, nil
}
