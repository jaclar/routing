package zarr

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sailboat/meteo/internal/model"
)

// StoreManager coordinates lifecycle, atomic activation, and retention for model stores.
type StoreManager struct {
	BaseDir string
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

