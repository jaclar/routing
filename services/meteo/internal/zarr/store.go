package zarr

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"sailboat/meteo/internal/model"
)

const (
	DefaultChunkLat = 16
	DefaultChunkLon = 16
)

// ZArrayMeta models standard Zarr V2 .zarray metadata.
type ZArrayMeta struct {
	ZarrFormat int    `json:"zarr_format"`
	Shape      []int  `json:"shape"`
	Chunks     []int  `json:"chunks"`
	DType      string `json:"dtype"`
	Order      string `json:"order"`
	FillValue  any    `json:"fill_value"`
	Compressor struct {
		ID    string `json:"id"`
		Level int    `json:"level"`
	} `json:"compressor"`
}

// ZGroupMeta models standard Zarr V2 .zgroup metadata.
type ZGroupMeta struct {
	ZarrFormat int `json:"zarr_format"`
}

// Store represents an open Zarr dataset ready for sub-millisecond point time-series queries.
type Store struct {
	mu           sync.RWMutex
	RootDir      string
	Cycle        time.Time
	Steps        []int
	Members      []int
	IsEnsemble   bool
	StoreMembers bool
	NMembers     int
	Lats         []float32
	Lons         []float32
	LatStep      float64
	LonStep      float64
	Variables    []string
	ChunkLat     int
	ChunkLon     int
	NLats        int
	NLons        int
	NSteps       lenSteps
	chunkCache   map[string][]float32
	cacheMu      sync.RWMutex
	maxCacheLen  int
}

type lenSteps = int

// OpenStore loads a Zarr dataset from directory.
func OpenStore(dir string) (*Store, error) {
	groupPath := filepath.Join(dir, ".zgroup")
	if _, err := os.Stat(groupPath); err != nil {
		return nil, fmt.Errorf("not a valid zarr store: %w", err)
	}

	// Read reference time / metadata
	metaPath := filepath.Join(dir, "metadata.json")
	var cycleMeta struct {
		ModelName          string    `json:"model_name"`
		ReferenceTime      time.Time `json:"reference_time"`
		ResolutionDeg      float64   `json:"resolution_deg"`
		Steps              []int     `json:"steps"`
		Members            []int     `json:"members,omitempty"`
		IsEnsemble         bool      `json:"is_ensemble,omitempty"`
		StoreMembers       *bool     `json:"store_members,omitempty"`
		FullEnsembleStored *bool     `json:"full_ensemble_stored,omitempty"`
		Variables          []string  `json:"variables"`
		LatStart           float64   `json:"lat_start"`
		LatEnd             float64   `json:"lat_end"`
		LatStep            float64   `json:"lat_step"`
		LonStart           float64   `json:"lon_start"`
		LonEnd             float64   `json:"lon_end"`
		LonStep            float64   `json:"lon_step"`
		NLats              int       `json:"nlats"`
		NLons              int       `json:"nlons"`
		ChunkLat           int       `json:"chunk_lat"`
		ChunkLon           int       `json:"chunk_lon"`
	}

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read store metadata: %w", err)
	}
	if err := json.Unmarshal(metaBytes, &cycleMeta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.json: %w", err)
	}

	chunkLat := cycleMeta.ChunkLat
	if chunkLat <= 0 {
		chunkLat = DefaultChunkLat
	}
	chunkLon := cycleMeta.ChunkLon
	if chunkLon <= 0 {
		chunkLon = DefaultChunkLon
	}

	lats := make([]float32, cycleMeta.NLats)
	for i := 0; i < cycleMeta.NLats; i++ {
		lats[i] = float32(cycleMeta.LatStart - float64(i)*cycleMeta.LatStep)
	}

	lons := make([]float32, cycleMeta.NLons)
	for j := 0; j < cycleMeta.NLons; j++ {
		lons[j] = float32(cycleMeta.LonStart + float64(j)*cycleMeta.LonStep)
	}

	members := cycleMeta.Members
	if len(members) == 0 {
		members = []int{0}
	}

	storeMembers := false
	if cycleMeta.StoreMembers != nil {
		storeMembers = *cycleMeta.StoreMembers
	} else if cycleMeta.FullEnsembleStored != nil {
		storeMembers = *cycleMeta.FullEnsembleStored
	} else if cycleMeta.IsEnsemble || len(members) > 1 {
		// Fallback check for legacy 4D chunk files
		testVar := "wind_u_10m"
		if len(cycleMeta.Variables) > 0 {
			testVar = cycleMeta.Variables[0]
		}
		if _, err := os.Stat(filepath.Join(dir, testVar, "0.0.0.0")); err == nil {
			storeMembers = true
		}
	}

	return &Store{
		RootDir:      dir,
		Cycle:        cycleMeta.ReferenceTime,
		Steps:        cycleMeta.Steps,
		Members:      members,
		IsEnsemble:   cycleMeta.IsEnsemble || len(members) > 1,
		StoreMembers: storeMembers,
		NMembers:     len(members),
		Lats:         lats,
		Lons:         lons,
		LatStep:      cycleMeta.LatStep,
		LonStep:      cycleMeta.LonStep,
		Variables:    cycleMeta.Variables,
		ChunkLat:     chunkLat,
		ChunkLon:     chunkLon,
		NLats:        cycleMeta.NLats,
		NLons:        cycleMeta.NLons,
		NSteps:       len(cycleMeta.Steps),
		chunkCache:   make(map[string][]float32, 1024),
		maxCacheLen:  4096,
	}, nil
}

// GetPointTimeSeries extracts time series across all forecast steps for a specific (latIdx, lonIdx) grid point.
// Returns []float32 of length len(s.Steps).
func (s *Store) GetPointTimeSeries(variable string, latIdx, lonIdx int) ([]float32, error) {
	if latIdx < 0 || latIdx >= s.NLats || lonIdx < 0 || lonIdx >= s.NLons {
		return nil, fmt.Errorf("grid index out of bounds: lat=%d (max %d), lon=%d (max %d)", latIdx, s.NLats, lonIdx, s.NLons)
	}

	cLat := latIdx / s.ChunkLat
	cLon := lonIdx / s.ChunkLon
	cacheKey := fmt.Sprintf("%s/0.%d.%d", variable, cLat, cLon)

	s.cacheMu.RLock()
	chunkData, ok := s.chunkCache[cacheKey]
	s.cacheMu.RUnlock()

	if !ok {
		// Read and decompress chunk from disk: first try 3D chunk (0.cLat.cLon), then fallback to 4D member 0 chunk (0.0.cLat.cLon)
		chunkPath := filepath.Join(s.RootDir, variable, fmt.Sprintf("0.%d.%d", cLat, cLon))
		compressed, err := os.ReadFile(chunkPath)
		if err != nil {
			// Fallback: try 4D member 0 chunk
			chunkPath4D := filepath.Join(s.RootDir, variable, fmt.Sprintf("0.0.%d.%d", cLat, cLon))
			compressed, err = os.ReadFile(chunkPath4D)
			if err != nil {
				// Chunk might not exist, return NaNs
				res := make([]float32, s.NSteps)
				for i := range res {
					res[i] = float32(math.NaN())
				}
				return res, nil
			}
		}

		rawBytes, err := DecompressZstd(compressed)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress chunk %s: %w", chunkPath, err)
		}

		// Convert raw bytes to float32 slice
		numFloats := len(rawBytes) / 4
		chunkData = make([]float32, numFloats)
		for i := 0; i < numFloats; i++ {
			bits := binary.LittleEndian.Uint32(rawBytes[i*4 : (i+1)*4])
			chunkData[i] = math.Float32frombits(bits)
		}

		// Cache hot chunk
		s.cacheMu.Lock()
		if len(s.chunkCache) > s.maxCacheLen {
			s.chunkCache = make(map[string][]float32, 1024)
		}
		s.chunkCache[cacheKey] = chunkData
		s.cacheMu.Unlock()
	}

	// Extract time series: chunk layout is [NSteps, chunkLats, chunkLons]
	latOffset := latIdx % s.ChunkLat
	lonOffset := lonIdx % s.ChunkLon
	chunkStrideLat := s.ChunkLon
	chunkStrideStep := s.ChunkLat * s.ChunkLon

	out := make([]float32, s.NSteps)
	for stepIdx := 0; stepIdx < s.NSteps; stepIdx++ {
		idx := stepIdx*chunkStrideStep + latOffset*chunkStrideLat + lonOffset
		if idx < len(chunkData) {
			out[stepIdx] = chunkData[idx]
		} else {
			out[stepIdx] = float32(math.NaN())
		}
	}

	return out, nil
}

// GetMemberPointTimeSeries extracts time series for a specific ensemble member from 4D array store.
func (s *Store) GetMemberPointTimeSeries(variable string, memberIdx, latIdx, lonIdx int) ([]float32, error) {
	if latIdx < 0 || latIdx >= s.NLats || lonIdx < 0 || lonIdx >= s.NLons {
		return nil, fmt.Errorf("grid index out of bounds: lat=%d (max %d), lon=%d (max %d)", latIdx, s.NLats, lonIdx, s.NLons)
	}

	if !s.StoreMembers {
		if memberIdx == 0 {
			return s.GetPointTimeSeries(variable, latIdx, lonIdx)
		}
		return nil, fmt.Errorf("individual ensemble members not stored on disk for model at %s (only statistical summaries stored); enable STORE_FULL_ENSEMBLE to store full member data", s.RootDir)
	}

	cLat := latIdx / s.ChunkLat
	cLon := lonIdx / s.ChunkLon
	cacheKey := fmt.Sprintf("%s/0.%d.%d.%d", variable, memberIdx, cLat, cLon)

	s.cacheMu.RLock()
	chunkData, ok := s.chunkCache[cacheKey]
	s.cacheMu.RUnlock()

	if !ok {
		chunkPath := filepath.Join(s.RootDir, variable, fmt.Sprintf("0.%d.%d.%d", memberIdx, cLat, cLon))
		compressed, err := os.ReadFile(chunkPath)
		if err != nil {
			// Fallback: try 3D chunk if single member
			if memberIdx == 0 {
				return s.GetPointTimeSeries(variable, latIdx, lonIdx)
			}
			res := make([]float32, s.NSteps)
			for i := range res {
				res[i] = float32(math.NaN())
			}
			return res, nil
		}

		rawBytes, err := DecompressZstd(compressed)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress chunk %s: %w", chunkPath, err)
		}

		numFloats := len(rawBytes) / 4
		chunkData = make([]float32, numFloats)
		for i := 0; i < numFloats; i++ {
			bits := binary.LittleEndian.Uint32(rawBytes[i*4 : (i+1)*4])
			chunkData[i] = math.Float32frombits(bits)
		}

		s.cacheMu.Lock()
		if len(s.chunkCache) > s.maxCacheLen {
			s.chunkCache = make(map[string][]float32, 1024)
		}
		s.chunkCache[cacheKey] = chunkData
		s.cacheMu.Unlock()
	}

	latOffset := latIdx % s.ChunkLat
	lonOffset := lonIdx % s.ChunkLon
	chunkStrideLat := s.ChunkLon
	chunkStrideStep := s.ChunkLat * s.ChunkLon

	out := make([]float32, s.NSteps)
	for stepIdx := 0; stepIdx < s.NSteps; stepIdx++ {
		idx := stepIdx*chunkStrideStep + latOffset*chunkStrideLat + lonOffset
		if idx < len(chunkData) {
			out[stepIdx] = chunkData[idx]
		} else {
			out[stepIdx] = float32(math.NaN())
		}
	}

	return out, nil
}

// GetAllMembersPointTimeSeries extracts time series for all ensemble members at a specific coordinate.
// Returns [NMembers][NSteps]float32.
func (s *Store) GetAllMembersPointTimeSeries(variable string, latIdx, lonIdx int) ([][]float32, error) {
	if !s.StoreMembers {
		return nil, fmt.Errorf("individual ensemble members not stored on disk for model at %s (only statistical summaries stored); enable STORE_FULL_ENSEMBLE to store full member data", s.RootDir)
	}

	nMembers := len(s.Members)
	if nMembers <= 1 {
		ts, err := s.GetPointTimeSeries(variable, latIdx, lonIdx)
		if err != nil {
			return nil, err
		}
		return [][]float32{ts}, nil
	}

	allMembers := make([][]float32, nMembers)
	for mIdx, mID := range s.Members {
		ts, err := s.GetMemberPointTimeSeries(variable, mID, latIdx, lonIdx)
		if err != nil {
			ts = make([]float32, s.NSteps)
			for i := range ts {
				ts[i] = float32(math.NaN())
			}
		}
		allMembers[mIdx] = ts
	}

	return allMembers, nil
}

// StoreWriter stages downloaded forecast slices, reduces ensembles to statistics as the members
// arrive, and packs the result into a chunked Zarr store.
type StoreWriter struct {
	mu                sync.Mutex
	dir               string
	slicesDir         string
	statsDir          string
	cycle             *model.ModelCycle
	stepMap           map[int]int // forecastStep -> stepIndex
	steps             []int
	members           []int
	isEnsemble        bool
	latStart          float64
	latEnd            float64
	latStep           float64
	lonStart          float64
	lonEnd            float64
	lonStep           float64
	nlats             int
	nlons             int
	chunkLat          int
	chunkLon          int
	variables         []string
	storeFullEnsemble bool
	startedAt         time.Time
	downloadEndedAt   time.Time

	// reduceEnsemble is set when this cycle carries several members and therefore has
	// statistics to compute; windDerived additionally requires both wind components.
	reduceEnsemble bool
	windDerived    bool

	// Ensemble completion bookkeeping, all guarded by mu. A step is claimed by the download
	// worker that stages its last member, reduced once its statistics are on disk, and — for
	// the wind pair — marked done once the derived wind arrays are on disk too.
	arrived  map[stepKey]int
	claimed  map[stepKey]bool
	reduced  map[stepKey]bool
	windDone map[int]bool
}

// NewStoreWriter initializes a staging Zarr directory.
func NewStoreWriter(dir string, cycle *model.ModelCycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep float64, variables []string, storeFullEnsemble bool) (*StoreWriter, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create zarr dir: %w", err)
	}

	slicesDir := filepath.Join(dir, ".slices")
	if err := os.MkdirAll(slicesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create slices staging dir: %w", err)
	}

	statsDir := filepath.Join(dir, ".stats")
	if err := os.MkdirAll(statsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create stats staging dir: %w", err)
	}

	nlats := int(math.Round(math.Abs(latStart-latEnd)/latStep)) + 1
	nlons := int(math.Round(math.Abs(lonEnd-lonStart)/lonStep)) + 1

	stepMap := make(map[int]int, len(cycle.ForecastSteps))
	for idx, step := range cycle.ForecastSteps {
		stepMap[step] = idx
	}

	// Write root .zgroup
	groupData, _ := json.Marshal(ZGroupMeta{ZarrFormat: 2})
	if err := os.WriteFile(filepath.Join(dir, ".zgroup"), groupData, 0644); err != nil {
		return nil, err
	}

	members := cycle.Members
	if len(members) == 0 {
		members = []int{0}
	}

	isEnsemble := cycle.IsEnsemble || len(members) > 1
	reduceEnsemble := isEnsemble && len(members) > 1

	hasU, hasV := false, false
	for _, v := range variables {
		switch v {
		case model.VarWindU10m:
			hasU = true
		case model.VarWindV10m:
			hasV = true
		}
	}

	sw := &StoreWriter{
		dir:               dir,
		slicesDir:         slicesDir,
		statsDir:          statsDir,
		cycle:             cycle,
		stepMap:           stepMap,
		steps:             cycle.ForecastSteps,
		members:           members,
		isEnsemble:        isEnsemble,
		latStart:          latStart,
		latEnd:            latEnd,
		latStep:           latStep,
		lonStart:          lonStart,
		lonEnd:            lonEnd,
		lonStep:           lonStep,
		nlats:             nlats,
		nlons:             nlons,
		chunkLat:          DefaultChunkLat,
		chunkLon:          DefaultChunkLon,
		variables:         variables,
		storeFullEnsemble: storeFullEnsemble,
		startedAt:         time.Now().UTC(),
		reduceEnsemble:    reduceEnsemble,
		windDerived:       reduceEnsemble && hasU && hasV,
		arrived:           make(map[stepKey]int),
		claimed:           make(map[stepKey]bool),
		reduced:           make(map[stepKey]bool),
		windDone:          make(map[int]bool),
	}

	return sw, nil
}

// MarkDownloadComplete records the timestamp at which all slice downloads for this cycle finished successfully.
func (sw *StoreWriter) MarkDownloadComplete() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.downloadEndedAt = time.Now().UTC()
}

// WriteSlice stages the member grids carried by one downloaded slice. Once every member of a
// (variable, forecast step) has arrived it reduces them to ensemble statistics straight away, so
// that work overlaps with the remaining downloads and the raw member grids can be dropped as
// soon as nothing needs them. Safe to call concurrently from several download workers.
func (sw *StoreWriter) WriteSlice(slice *model.RawGridSlice) error {
	if _, ok := sw.stepMap[slice.StepHours]; !ok {
		return fmt.Errorf("unknown forecast step %d for cycle", slice.StepHours)
	}

	staged := 0

	// A slice either carries one member, or bundles the whole ensemble (e.g. DWD ICON-EPS).
	if len(slice.MembersData) > 0 {
		for mIdx, rawFloats := range slice.MembersData {
			member := mIdx
			if mIdx < len(sw.members) {
				member = sw.members[mIdx]
			}
			if err := sw.stageMemberGrid(slice.Variable, slice.StepHours, member, rawFloats); err != nil {
				return err
			}
			staged++
		}
	} else {
		if err := sw.stageMemberGrid(slice.Variable, slice.StepHours, slice.Member, slice.Data); err != nil {
			return err
		}
		staged = 1
	}

	return sw.recordArrivals(slice.Variable, slice.StepHours, staged)
}

// stageMemberGrid writes one member's grid to the staging area, padding short grids with NaN.
func (sw *StoreWriter) stageMemberGrid(variable string, step, member int, values []float32) error {
	gridPoints := sw.nlats * sw.nlons

	if len(values) != gridPoints {
		padded := make([]float32, gridPoints)
		for i := range padded {
			padded[i] = nan32
		}
		copy(padded, values[:min(len(values), gridPoints)])
		values = padded
	}

	return writeGrid(sw.memberSlicePath(variable, step, member), values)
}

// recordArrivals counts the members staged for a (variable, step) and reduces that step as soon
// as the whole ensemble is present. Exactly one caller claims the reduction, and it runs outside
// the lock so concurrent download workers never serialize behind it.
func (sw *StoreWriter) recordArrivals(variable string, step, staged int) error {
	if !sw.reduceEnsemble {
		return nil
	}

	key := stepKey{variable: variable, step: step}

	sw.mu.Lock()
	sw.arrived[key] += staged
	claim := sw.arrived[key] >= len(sw.members) && !sw.claimed[key]
	if claim {
		sw.claimed[key] = true
	}
	sw.mu.Unlock()

	if !claim {
		return nil
	}

	if err := sw.reduceStep(variable, step); err != nil {
		return fmt.Errorf("failed to reduce %s step %d: %w", variable, step, err)
	}

	sw.mu.Lock()
	sw.reduced[key] = true
	sw.mu.Unlock()

	return sw.afterStepReduced(variable, step)
}

// Finalize packs the staged grids into compressed Zarr chunks and writes the store metadata.
// Ensemble statistics were already computed as the members arrived, so this stage only assembles
// chunks, which it does across all cores since every array is independent.
func (sw *StoreWriter) Finalize() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	jobs, arrays, err := sw.planArrays()
	if err != nil {
		return err
	}

	if err := sw.runPackJobs(jobs); err != nil {
		return err
	}

	_ = os.RemoveAll(sw.slicesDir)
	_ = os.RemoveAll(sw.statsDir)

	return sw.writeMetadata(arrays, time.Now().UTC())
}

// planArrays creates the directory and .zarray metadata for every array this store publishes,
// and returns the packing jobs that fill them alongside the sorted list of array names.
func (sw *StoreWriter) planArrays() ([]packJob, []string, error) {
	nSteps := len(sw.steps)
	shape3D := []int{nSteps, sw.nlats, sw.nlons}
	chunks3D := []int{nSteps, sw.chunkLat, sw.chunkLon}

	var jobs []packJob
	arrays := make(map[string]bool)

	// add publishes one 3D array, optionally mirrored into extra directories so a field stored
	// under two names is compressed once and written twice.
	add := func(name string, alsoInto []string, source func(step int) string) error {
		dirs := make([]string, 0, len(alsoInto)+1)
		for _, n := range append([]string{name}, alsoInto...) {
			if err := writeZArray(filepath.Join(sw.dir, n), shape3D, chunks3D); err != nil {
				return err
			}
			dirs = append(dirs, filepath.Join(sw.dir, n))
			arrays[n] = true
		}
		jobs = append(jobs, packJob{name: name, outDirs: dirs, stepFile: source, chunkName: chunk3DName})
		return nil
	}

	for _, v := range sw.variables {
		if !sw.reduceEnsemble {
			// Deterministic: the single member's grid is the array.
			member := sw.members[0]
			if err := add(v, nil, func(step int) string {
				return sw.memberSlicePath(v, step, member)
			}); err != nil {
				return nil, nil, err
			}
			continue
		}

		// Ensemble: publish each statistic, and let the canonical variable name serve the
		// mean so a caller that does not ask for a statistic still gets a usable field.
		for _, suffix := range statSuffixes {
			array := v + "_" + suffix
			mirror := []string{}
			if suffix == "mean" {
				mirror = append(mirror, v)
			}
			if err := add(array, mirror, func(step int) string {
				return sw.statGridPath(array, step)
			}); err != nil {
				return nil, nil, err
			}
		}
	}

	if sw.windDerived {
		for _, array := range windDerivedArrays() {
			if err := add(array, nil, func(step int) string {
				return sw.statGridPath(array, step)
			}); err != nil {
				return nil, nil, err
			}
		}
	}

	// Strategy B: the raw members, as a 4D array per canonical variable. The canonical
	// directory already holds the 3D mean chunks written above, so only its metadata changes.
	if sw.reduceEnsemble && sw.storeFullEnsemble {
		shape4D := []int{nSteps, len(sw.members), sw.nlats, sw.nlons}
		chunks4D := []int{nSteps, 1, sw.chunkLat, sw.chunkLon}

		for _, v := range sw.variables {
			varDir := filepath.Join(sw.dir, v)
			if err := writeZArray(varDir, shape4D, chunks4D); err != nil {
				return nil, nil, err
			}
			for _, member := range sw.members {
				jobs = append(jobs, packJob{
					name:      fmt.Sprintf("%s member %d", v, member),
					outDirs:   []string{varDir},
					stepFile:  func(step int) string { return sw.memberSlicePath(v, step, member) },
					chunkName: chunk4DName(member),
				})
			}
		}
	}

	names := make([]string, 0, len(arrays))
	for name := range arrays {
		names = append(names, name)
	}
	sort.Strings(names)

	return jobs, names, nil
}

// writeMetadata records what this store contains and how long each ingestion stage took.
func (sw *StoreWriter) writeMetadata(arrays []string, writeEndedAt time.Time) error {
	storeMeta := struct {
		ModelName       string    `json:"model_name"`
		ReferenceTime   time.Time `json:"reference_time"`
		ResolutionDeg   float64   `json:"resolution_deg"`
		Steps           []int     `json:"steps"`
		Members         []int     `json:"members,omitempty"`
		IsEnsemble      bool      `json:"is_ensemble,omitempty"`
		StoreMembers    bool      `json:"store_members"`
		Variables       []string  `json:"variables"`
		LatStart        float64   `json:"lat_start"`
		LatEnd          float64   `json:"lat_end"`
		LatStep         float64   `json:"lat_step"`
		LonStart        float64   `json:"lon_start"`
		LonEnd          float64   `json:"lon_end"`
		LonStep         float64   `json:"lon_step"`
		NLats           int       `json:"nlats"`
		NLons           int       `json:"nlons"`
		ChunkLat        int       `json:"chunk_lat"`
		ChunkLon        int       `json:"chunk_lon"`
		IngestStartedAt time.Time `json:"ingest_started_at"`
		DownloadEndedAt time.Time `json:"download_ended_at"`
		WriteEndedAt    time.Time `json:"write_ended_at"`
	}{
		ModelName:       sw.cycle.ModelName,
		ReferenceTime:   sw.cycle.ReferenceTime,
		ResolutionDeg:   sw.cycle.ResolutionDeg,
		Steps:           sw.steps,
		Members:         sw.members,
		IsEnsemble:      sw.isEnsemble,
		StoreMembers:    sw.storeFullEnsemble && sw.isEnsemble,
		Variables:       arrays,
		LatStart:        sw.latStart,
		LatEnd:          sw.latEnd,
		LatStep:         sw.latStep,
		LonStart:        sw.lonStart,
		LonEnd:          sw.lonEnd,
		LonStep:         sw.lonStep,
		NLats:           sw.nlats,
		NLons:           sw.nlons,
		ChunkLat:        sw.chunkLat,
		ChunkLon:        sw.chunkLon,
		IngestStartedAt: sw.startedAt,
		DownloadEndedAt: sw.downloadEndedAt,
		WriteEndedAt:    writeEndedAt,
	}

	metaJSON, err := json.MarshalIndent(storeMeta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sw.dir, "metadata.json"), metaJSON, 0644)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
