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
	mu          sync.RWMutex
	RootDir     string
	Cycle       time.Time
	Steps       []int
	Members     []int
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

// StoreWriter writes raw forecast slices and builds chunked Zarr stores with low-memory disk streaming.
type StoreWriter struct {
	mu         sync.Mutex
	dir        string
	slicesDir  string
	cycle      *model.ModelCycle
	stepMap    map[int]int // forecastStep -> stepIndex
	steps      []int
	members    []int
	isEnsemble bool
	latStart   float64
	latEnd     float64
	latStep    float64
	lonStart   float64
	lonEnd     float64
	lonStep    float64
	nlats             int
	nlons             int
	chunkLat          int
	chunkLon          int
	variables         []string
	storeFullEnsemble bool
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

	sw := &StoreWriter{
		dir:               dir,
		slicesDir:         slicesDir,
		cycle:             cycle,
		stepMap:           stepMap,
		steps:             cycle.ForecastSteps,
		members:           members,
		isEnsemble:        cycle.IsEnsemble || len(members) > 1,
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
	}

	return sw, nil
}

// WriteSlice saves a 2D scalar field for a given variable and forecast step to temporary slice files.
func (sw *StoreWriter) WriteSlice(slice *model.RawGridSlice) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	_, ok := sw.stepMap[slice.StepHours]
	if !ok {
		return fmt.Errorf("unknown forecast step %d for cycle", slice.StepHours)
	}

	gridPoints := sw.nlats * sw.nlons

	// If slice contains bundled multiple members (e.g. DWD ICON-EPS)
	if len(slice.MembersData) > 0 {
		for mIdx, rawFloats := range slice.MembersData {
			memberNum := mIdx
			if mIdx < len(sw.members) {
				memberNum = sw.members[mIdx]
			}
			if len(rawFloats) != gridPoints {
				padded := make([]float32, gridPoints)
				for i := range padded {
					padded[i] = float32(math.NaN())
				}
				copy(padded, rawFloats)
				rawFloats = padded
			}

			buf := make([]byte, gridPoints*4)
			for i, f := range rawFloats {
				binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], math.Float32bits(f))
			}

			slicePath := filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d_m%d.bin", slice.Variable, slice.StepHours, memberNum))
			if err := os.WriteFile(slicePath, buf, 0644); err != nil {
				return fmt.Errorf("failed to write slice file %s: %w", slicePath, err)
			}
		}
		return nil
	}

	// Single member slice
	rawFloats := slice.Data
	if len(rawFloats) != gridPoints {
		padded := make([]float32, gridPoints)
		for i := range padded {
			padded[i] = float32(math.NaN())
		}
		copyLen := min(len(slice.Data), gridPoints)
		copy(padded[:copyLen], slice.Data[:copyLen])
		rawFloats = padded
	}

	buf := make([]byte, gridPoints*4)
	for i, f := range rawFloats {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], math.Float32bits(f))
	}

	slicePath := filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d_m%d.bin", slice.Variable, slice.StepHours, slice.Member))
	if err := os.WriteFile(slicePath, buf, 0644); err != nil {
		return fmt.Errorf("failed to write slice file %s: %w", slicePath, err)
	}

	return nil
}

// Finalize streams temporary slice files into compressed Zarr chunks (<30MB RAM footprint).
// Computes Strategy A statistics (mean, std, p10, p50, p90, exceedance probabilities) and Strategy B 4D member chunks.
func (sw *StoreWriter) Finalize() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	nSteps := len(sw.steps)
	nMembers := len(sw.members)
	if nMembers == 0 {
		nMembers = 1
	}

	nLatChunks := (sw.nlats + sw.chunkLat - 1) / sw.chunkLat
	nLonChunks := (sw.nlons + sw.chunkLon - 1) / sw.chunkLon
	gridPoints := sw.nlats * sw.nlons

	createdVariables := make(map[string]bool)
	for _, v := range sw.variables {
		createdVariables[v] = true
	}

	// 1. Process each canonical variable
	for _, v := range sw.variables {
		varDir := filepath.Join(sw.dir, v)
		if err := os.MkdirAll(varDir, 0755); err != nil {
			return err
		}

		if sw.isEnsemble && nMembers > 1 {
			if sw.storeFullEnsemble {
				// Strategy B: 4D array [nSteps, nMembers, nlats, nlons]
				meta4D := ZArrayMeta{
					ZarrFormat: 2,
					Shape:      []int{nSteps, nMembers, sw.nlats, sw.nlons},
					Chunks:     []int{nSteps, 1, sw.chunkLat, sw.chunkLon},
					DType:      "<f4",
					Order:      "C",
					FillValue:  "NaN",
				}
				meta4D.Compressor.ID = "zstd"
				meta4D.Compressor.Level = 3
				metaJSON4D, _ := json.MarshalIndent(meta4D, "", "  ")
				_ = os.WriteFile(filepath.Join(varDir, ".zarray"), metaJSON4D, 0644)
			} else {
				// Statistical 3D array for canonical variable (stores ensemble mean) [nSteps, nlats, nlons]
				meta3D := ZArrayMeta{
					ZarrFormat: 2,
					Shape:      []int{nSteps, sw.nlats, sw.nlons},
					Chunks:     []int{nSteps, sw.chunkLat, sw.chunkLon},
					DType:      "<f4",
					Order:      "C",
					FillValue:  "NaN",
				}
				meta3D.Compressor.ID = "zstd"
				meta3D.Compressor.Level = 3
				metaJSON3D, _ := json.MarshalIndent(meta3D, "", "  ")
				_ = os.WriteFile(filepath.Join(varDir, ".zarray"), metaJSON3D, 0644)
			}

			// Strategy A: Precomputed statistics 3D arrays [nSteps, nlats, nlons]
			statsDirs := []string{
				filepath.Join(sw.dir, v+"_mean"),
				filepath.Join(sw.dir, v+"_std"),
				filepath.Join(sw.dir, v+"_p10"),
				filepath.Join(sw.dir, v+"_p50"),
				filepath.Join(sw.dir, v+"_p90"),
			}
			for _, sDir := range statsDirs {
				_ = os.MkdirAll(sDir, 0755)
				meta3D := ZArrayMeta{
					ZarrFormat: 2,
					Shape:      []int{nSteps, sw.nlats, sw.nlons},
					Chunks:     []int{nSteps, sw.chunkLat, sw.chunkLon},
					DType:      "<f4",
					Order:      "C",
					FillValue:  "NaN",
				}
				meta3D.Compressor.ID = "zstd"
				meta3D.Compressor.Level = 3
				metaJSON3D, _ := json.MarshalIndent(meta3D, "", "  ")
				_ = os.WriteFile(filepath.Join(sDir, ".zarray"), metaJSON3D, 0644)
			}
			createdVariables[v+"_mean"] = true
			createdVariables[v+"_std"] = true
			createdVariables[v+"_p10"] = true
			createdVariables[v+"_p50"] = true
			createdVariables[v+"_p90"] = true
		} else {
			// Deterministic 3D array
			meta := ZArrayMeta{
				ZarrFormat: 2,
				Shape:      []int{nSteps, sw.nlats, sw.nlons},
				Chunks:     []int{nSteps, sw.chunkLat, sw.chunkLon},
				DType:      "<f4",
				Order:      "C",
				FillValue:  "NaN",
			}
			meta.Compressor.ID = "zstd"
			meta.Compressor.Level = 3
			metaJSON, _ := json.MarshalIndent(meta, "", "  ")
			_ = os.WriteFile(filepath.Join(varDir, ".zarray"), metaJSON, 0644)
		}

		// Process in latitude bands
		for cLat := 0; cLat < nLatChunks; cLat++ {
			latBase := cLat * sw.chunkLat
			nBandLats := min(sw.chunkLat, sw.nlats-latBase)
			bandPointsPerStep := nBandLats * sw.nlons

			// Load band rows across all steps and members: [nSteps][nMembers][bandPointsPerStep]
			bandData := make([][][]float32, nSteps)
			for sIdx := range bandData {
				bandData[sIdx] = make([][]float32, nMembers)
				for mIdx := range bandData[sIdx] {
					b := make([]float32, bandPointsPerStep)
					for i := range b {
						b[i] = float32(math.NaN())
					}
					bandData[sIdx][mIdx] = b
				}
			}

			for stepIdx, step := range sw.steps {
				for mIdx, mID := range sw.members {
					slicePath := filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d_m%d.bin", v, step, mID))
					rawBytes, err := os.ReadFile(slicePath)
					if err != nil && mID == 0 {
						// Backward compatibility: check without _m0
						rawBytes, err = os.ReadFile(filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d.bin", v, step)))
					}

					if err == nil && len(rawBytes) == gridPoints*4 {
						rowByteOffset := latBase * sw.nlons * 4
						bandByteLen := bandPointsPerStep * 4

						if rowByteOffset+bandByteLen <= len(rawBytes) {
							bandBytes := rawBytes[rowByteOffset : rowByteOffset+bandByteLen]
							target := bandData[stepIdx][mIdx]
							for k := 0; k < bandPointsPerStep; k++ {
								bits := binary.LittleEndian.Uint32(bandBytes[k*4 : (k+1)*4])
								target[k] = math.Float32frombits(bits)
							}
						}
					}
				}
			}

			// Pack chunks
			chunkSize := nSteps * sw.chunkLat * sw.chunkLon

			for cLon := 0; cLon < nLonChunks; cLon++ {
				lonBase := cLon * sw.chunkLon

				if sw.isEnsemble && nMembers > 1 {
					// 1. Write 4D chunks per member ONLY if storeFullEnsemble is enabled
					if sw.storeFullEnsemble {
						for mIdx := range sw.members {
							chunkBuf := make([]byte, chunkSize*4)
							chunkFloatIdx := 0
							var hasValidData bool

							for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
								for dLat := 0; dLat < sw.chunkLat; dLat++ {
									latInBand := dLat
									for dLon := 0; dLon < sw.chunkLon; dLon++ {
										lon := lonBase + dLon
										var val float32 = float32(math.NaN())
										if latInBand < nBandLats && lon < sw.nlons {
											val = bandData[stepIdx][mIdx][latInBand*sw.nlons+lon]
											if !math.IsNaN(float64(val)) {
												hasValidData = true
											}
										}
										binary.LittleEndian.PutUint32(chunkBuf[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(val))
										chunkFloatIdx++
									}
								}
							}

							if hasValidData {
								compressed := CompressZstd(chunkBuf[:chunkFloatIdx*4])
								chunkPath := filepath.Join(varDir, fmt.Sprintf("0.%d.%d.%d", sw.members[mIdx], cLat, cLon))
								_ = os.WriteFile(chunkPath, compressed, 0644)
							}
						}
					}

					// 2. Strategy A: Write 3D statistical chunks: mean, std, p10, p50, p90
					bufMean := make([]byte, chunkSize*4)
					bufStd := make([]byte, chunkSize*4)
					bufP10 := make([]byte, chunkSize*4)
					bufP50 := make([]byte, chunkSize*4)
					bufP90 := make([]byte, chunkSize*4)

					chunkFloatIdx := 0
					var hasStatData bool
					memberSample := make([]float32, nMembers)

					for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
						for dLat := 0; dLat < sw.chunkLat; dLat++ {
							latInBand := dLat
							for dLon := 0; dLon < sw.chunkLon; dLon++ {
								lon := lonBase + dLon
								var meanVal, stdVal, p10Val, p50Val, p90Val float32 = float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())

								if latInBand < nBandLats && lon < sw.nlons {
									validCount := 0
									for mIdx := range sw.members {
										mVal := bandData[stepIdx][mIdx][latInBand*sw.nlons+lon]
										if !math.IsNaN(float64(mVal)) {
											memberSample[validCount] = mVal
											validCount++
										}
									}

									if validCount > 0 {
										hasStatData = true
										meanVal, stdVal, p10Val, p50Val, p90Val = calcStats(memberSample[:validCount])
									}
								}

								binary.LittleEndian.PutUint32(bufMean[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(meanVal))
								binary.LittleEndian.PutUint32(bufStd[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(stdVal))
								binary.LittleEndian.PutUint32(bufP10[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(p10Val))
								binary.LittleEndian.PutUint32(bufP50[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(p50Val))
								binary.LittleEndian.PutUint32(bufP90[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(p90Val))
								chunkFloatIdx++
							}
						}
					}

					if hasStatData {
						chunkName := fmt.Sprintf("0.%d.%d", cLat, cLon)
						_ = os.WriteFile(filepath.Join(sw.dir, v, chunkName), CompressZstd(bufMean[:chunkFloatIdx*4]), 0644)
						_ = os.WriteFile(filepath.Join(sw.dir, v+"_mean", chunkName), CompressZstd(bufMean[:chunkFloatIdx*4]), 0644)
						_ = os.WriteFile(filepath.Join(sw.dir, v+"_std", chunkName), CompressZstd(bufStd[:chunkFloatIdx*4]), 0644)
						_ = os.WriteFile(filepath.Join(sw.dir, v+"_p10", chunkName), CompressZstd(bufP10[:chunkFloatIdx*4]), 0644)
						_ = os.WriteFile(filepath.Join(sw.dir, v+"_p50", chunkName), CompressZstd(bufP50[:chunkFloatIdx*4]), 0644)
						_ = os.WriteFile(filepath.Join(sw.dir, v+"_p90", chunkName), CompressZstd(bufP90[:chunkFloatIdx*4]), 0644)
					}
				} else {
					// Deterministic 3D chunk
					chunkBuf := make([]byte, chunkSize*4)
					chunkFloatIdx := 0
					var hasValidData bool

					for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
						for dLat := 0; dLat < sw.chunkLat; dLat++ {
							latInBand := dLat
							for dLon := 0; dLon < sw.chunkLon; dLon++ {
								lon := lonBase + dLon
								var val float32 = float32(math.NaN())
								if latInBand < nBandLats && lon < sw.nlons {
									val = bandData[stepIdx][0][latInBand*sw.nlons+lon]
									if !math.IsNaN(float64(val)) {
										hasValidData = true
									}
								}
								binary.LittleEndian.PutUint32(chunkBuf[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(val))
								chunkFloatIdx++
							}
						}
					}

					if hasValidData {
						compressed := CompressZstd(chunkBuf[:chunkFloatIdx*4])
						chunkPath := filepath.Join(varDir, fmt.Sprintf("0.%d.%d", cLat, cLon))
						_ = os.WriteFile(chunkPath, compressed, 0644)
					}
				}
			}
		}
	}

	// 2. If ensemble and wind components exist, precompute derived wind speed statistics & exceedance probabilities
	hasU := false
	hasV := false
	for _, v := range sw.variables {
		if v == model.VarWindU10m {
			hasU = true
		}
		if v == model.VarWindV10m {
			hasV = true
		}
	}

	if sw.isEnsemble && nMembers > 1 && hasU && hasV {
		windDerivedVars := []string{
			"wind_speed_mean",
			"wind_speed_std",
			"wind_speed_p10",
			"wind_speed_p50",
			"wind_speed_p90",
			"prob_wind_ge_25kt",
			"prob_wind_ge_34kt",
		}

		for _, wd := range windDerivedVars {
			dir := filepath.Join(sw.dir, wd)
			_ = os.MkdirAll(dir, 0755)
			meta3D := ZArrayMeta{
				ZarrFormat: 2,
				Shape:      []int{nSteps, sw.nlats, sw.nlons},
				Chunks:     []int{nSteps, sw.chunkLat, sw.chunkLon},
				DType:      "<f4",
				Order:      "C",
				FillValue:  "NaN",
			}
			meta3D.Compressor.ID = "zstd"
			meta3D.Compressor.Level = 3
			metaJSON3D, _ := json.MarshalIndent(meta3D, "", "  ")
			_ = os.WriteFile(filepath.Join(dir, ".zarray"), metaJSON3D, 0644)
			createdVariables[wd] = true
		}

		// Process bands for derived wind metrics
		for cLat := 0; cLat < nLatChunks; cLat++ {
			latBase := cLat * sw.chunkLat
			nBandLats := min(sw.chunkLat, sw.nlats-latBase)
			bandPointsPerStep := nBandLats * sw.nlons

			bandDataU := make([][][]float32, nSteps)
			bandDataV := make([][][]float32, nSteps)
			for sIdx := range bandDataU {
				bandDataU[sIdx] = make([][]float32, nMembers)
				bandDataV[sIdx] = make([][]float32, nMembers)
				for mIdx := range bandDataU[sIdx] {
					bu := make([]float32, bandPointsPerStep)
					bv := make([]float32, bandPointsPerStep)
					for i := range bu {
						bu[i] = float32(math.NaN())
						bv[i] = float32(math.NaN())
					}
					bandDataU[sIdx][mIdx] = bu
					bandDataV[sIdx][mIdx] = bv
				}
			}

			for stepIdx, step := range sw.steps {
				for mIdx, mID := range sw.members {
					uBytes, _ := os.ReadFile(filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d_m%d.bin", model.VarWindU10m, step, mID)))
					vBytes, _ := os.ReadFile(filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d_m%d.bin", model.VarWindV10m, step, mID)))

					rowByteOffset := latBase * sw.nlons * 4
					bandByteLen := bandPointsPerStep * 4

					if len(uBytes) == gridPoints*4 && rowByteOffset+bandByteLen <= len(uBytes) {
						subU := uBytes[rowByteOffset : rowByteOffset+bandByteLen]
						targetU := bandDataU[stepIdx][mIdx]
						for k := 0; k < bandPointsPerStep; k++ {
							targetU[k] = math.Float32frombits(binary.LittleEndian.Uint32(subU[k*4 : (k+1)*4]))
						}
					}

					if len(vBytes) == gridPoints*4 && rowByteOffset+bandByteLen <= len(vBytes) {
						subV := vBytes[rowByteOffset : rowByteOffset+bandByteLen]
						targetV := bandDataV[stepIdx][mIdx]
						for k := 0; k < bandPointsPerStep; k++ {
							targetV[k] = math.Float32frombits(binary.LittleEndian.Uint32(subV[k*4 : (k+1)*4]))
						}
					}
				}
			}

			chunkSize := nSteps * sw.chunkLat * sw.chunkLon

			for cLon := 0; cLon < nLonChunks; cLon++ {
				lonBase := cLon * sw.chunkLon

				bufMean := make([]byte, chunkSize*4)
				bufStd := make([]byte, chunkSize*4)
				bufP10 := make([]byte, chunkSize*4)
				bufP50 := make([]byte, chunkSize*4)
				bufP90 := make([]byte, chunkSize*4)
				bufProb25 := make([]byte, chunkSize*4)
				bufProb34 := make([]byte, chunkSize*4)

				chunkFloatIdx := 0
				var hasWindData bool
				speedSample := make([]float32, nMembers)

				const thresh25ktMS = 25.0 * model.KnotsToMS // ~12.86 m/s
				const thresh34ktMS = 34.0 * model.KnotsToMS // ~17.49 m/s

				for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
					for dLat := 0; dLat < sw.chunkLat; dLat++ {
						latInBand := dLat
						for dLon := 0; dLon < sw.chunkLon; dLon++ {
							lon := lonBase + dLon
							var meanW, stdW, p10W, p50W, p90W, prob25, prob34 float32 = float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())

							if latInBand < nBandLats && lon < sw.nlons {
								validCount := 0
								count25 := 0
								count34 := 0

								for mIdx := range sw.members {
									uVal := bandDataU[stepIdx][mIdx][latInBand*sw.nlons+lon]
									vVal := bandDataV[stepIdx][mIdx][latInBand*sw.nlons+lon]
									if !math.IsNaN(float64(uVal)) && !math.IsNaN(float64(vVal)) {
										spd := float32(math.Hypot(float64(uVal), float64(vVal)))
										speedSample[validCount] = spd
										validCount++
										if float64(spd) >= thresh25ktMS {
											count25++
										}
										if float64(spd) >= thresh34ktMS {
											count34++
										}
									}
								}

								if validCount > 0 {
									hasWindData = true
									meanW, stdW, p10W, p50W, p90W = calcStats(speedSample[:validCount])
									prob25 = float32(count25) / float32(validCount)
									prob34 = float32(count34) / float32(validCount)
								}
							}

							binary.LittleEndian.PutUint32(bufMean[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(meanW))
							binary.LittleEndian.PutUint32(bufStd[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(stdW))
							binary.LittleEndian.PutUint32(bufP10[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(p10W))
							binary.LittleEndian.PutUint32(bufP50[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(p50W))
							binary.LittleEndian.PutUint32(bufP90[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(p90W))
							binary.LittleEndian.PutUint32(bufProb25[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(prob25))
							binary.LittleEndian.PutUint32(bufProb34[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(prob34))
							chunkFloatIdx++
						}
					}
				}

				if hasWindData {
					chunkName := fmt.Sprintf("0.%d.%d", cLat, cLon)
					_ = os.WriteFile(filepath.Join(sw.dir, "wind_speed_mean", chunkName), CompressZstd(bufMean[:chunkFloatIdx*4]), 0644)
					_ = os.WriteFile(filepath.Join(sw.dir, "wind_speed_std", chunkName), CompressZstd(bufStd[:chunkFloatIdx*4]), 0644)
					_ = os.WriteFile(filepath.Join(sw.dir, "wind_speed_p10", chunkName), CompressZstd(bufP10[:chunkFloatIdx*4]), 0644)
					_ = os.WriteFile(filepath.Join(sw.dir, "wind_speed_p50", chunkName), CompressZstd(bufP50[:chunkFloatIdx*4]), 0644)
					_ = os.WriteFile(filepath.Join(sw.dir, "wind_speed_p90", chunkName), CompressZstd(bufP90[:chunkFloatIdx*4]), 0644)
					_ = os.WriteFile(filepath.Join(sw.dir, "prob_wind_ge_25kt", chunkName), CompressZstd(bufProb25[:chunkFloatIdx*4]), 0644)
					_ = os.WriteFile(filepath.Join(sw.dir, "prob_wind_ge_34kt", chunkName), CompressZstd(bufProb34[:chunkFloatIdx*4]), 0644)
				}
			}
		}
	}

	// Remove temporary .slices folder
	_ = os.RemoveAll(sw.slicesDir)

	// Collect variable names for metadata
	finalVarList := make([]string, 0, len(createdVariables))
	for vName := range createdVariables {
		finalVarList = append(finalVarList, vName)
	}
	sort.Strings(finalVarList)

	// Write metadata.json for the store
	storeMeta := struct {
		ModelName     string    `json:"model_name"`
		ReferenceTime time.Time `json:"reference_time"`
		ResolutionDeg float64   `json:"resolution_deg"`
		Steps         []int     `json:"steps"`
		Members       []int     `json:"members,omitempty"`
		IsEnsemble    bool      `json:"is_ensemble,omitempty"`
		StoreMembers  bool      `json:"store_members"`
		Variables     []string  `json:"variables"`
		LatStart      float64   `json:"lat_start"`
		LatEnd        float64   `json:"lat_end"`
		LatStep       float64   `json:"lat_step"`
		LonStart      float64   `json:"lon_start"`
		LonEnd        float64   `json:"lon_end"`
		LonStep       float64   `json:"lon_step"`
		NLats         int       `json:"nlats"`
		NLons         int       `json:"nlons"`
		ChunkLat      int       `json:"chunk_lat"`
		ChunkLon      int       `json:"chunk_lon"`
	}{
		ModelName:     sw.cycle.ModelName,
		ReferenceTime: sw.cycle.ReferenceTime,
		ResolutionDeg: sw.cycle.ResolutionDeg,
		Steps:         sw.steps,
		Members:       sw.members,
		IsEnsemble:    sw.isEnsemble,
		StoreMembers:  sw.storeFullEnsemble && sw.isEnsemble,
		Variables:     finalVarList,
		LatStart:      sw.latStart,
		LatEnd:        sw.latEnd,
		LatStep:       sw.latStep,
		LonStart:      sw.lonStart,
		LonEnd:        sw.lonEnd,
		LonStep:       sw.lonStep,
		NLats:         sw.nlats,
		NLons:         sw.nlons,
		ChunkLat:      sw.chunkLat,
		ChunkLon:      sw.chunkLon,
	}

	metaJSON, _ := json.MarshalIndent(storeMeta, "", "  ")
	return os.WriteFile(filepath.Join(sw.dir, "metadata.json"), metaJSON, 0644)
}

// calcStats calculates mean, sample standard deviation, and percentiles (p10, p50, p90) from valid float32 values.
func calcStats(vals []float32) (mean, std, p10, p50, p90 float32) {
	n := len(vals)
	if n == 0 {
		return float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN()), float32(math.NaN())
	}
	if n == 1 {
		return vals[0], 0, vals[0], vals[0], vals[0]
	}

	var sum float64
	for _, v := range vals {
		sum += float64(v)
	}
	meanF := sum / float64(n)
	mean = float32(meanF)

	var sumSqDiff float64
	for _, v := range vals {
		d := float64(v) - meanF
		sumSqDiff += d * d
	}
	std = float32(math.Sqrt(sumSqDiff / float64(n-1)))

	// Sort for percentiles
	sorted := make([]float32, n)
	copy(sorted, vals)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	p10 = calcPercentile(sorted, 10.0)
	p50 = calcPercentile(sorted, 50.0)
	p90 = calcPercentile(sorted, 90.0)
	return mean, std, p10, p50, p90
}

// calcPercentile computes linear rank percentile for sorted slice.
func calcPercentile(sorted []float32, p float64) float32 {
	n := len(sorted)
	if n == 0 {
		return float32(math.NaN())
	}
	if n == 1 {
		return sorted[0]
	}

	rank := (p / 100.0) * float64(n-1)
	low := int(math.Floor(rank))
	high := int(math.Ceil(rank))
	frac := float32(rank - float64(low))

	if low == high || high >= n {
		return sorted[low]
	}

	return sorted[low]*(1.0-frac) + sorted[high]*frac
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

