package zarr

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	ZarrFormat int       `json:"zarr_format"`
	Shape      []int     `json:"shape"`
	Chunks     []int     `json:"chunks"`
	DType      string    `json:"dtype"`
	Order      string    `json:"order"`
	FillValue  any       `json:"fill_value"`
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
	Lats        []float32
	Lons        []float32
	LatStep     float64
	LonStep     float64
	Variables   []string
	ChunkLat    int
	ChunkLon    int
	NLats       int
	NLons       int
	NSteps      int
	chunkCache  map[string][]float32
	cacheMu     sync.RWMutex
	maxCacheLen int
}

// OpenStore loads a Zarr dataset from directory.
func OpenStore(dir string) (*Store, error) {
	groupPath := filepath.Join(dir, ".zgroup")
	if _, err := os.Stat(groupPath); err != nil {
		return nil, fmt.Errorf("not a valid zarr store: %w", err)
	}

	// Read reference time / metadata
	metaPath := filepath.Join(dir, "metadata.json")
	var cycleMeta struct {
		ModelName     string    `json:"model_name"`
		ReferenceTime time.Time `json:"reference_time"`
		ResolutionDeg float64   `json:"resolution_deg"`
		Steps         []int     `json:"steps"`
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

	return &Store{
		RootDir:     dir,
		Cycle:       cycleMeta.ReferenceTime,
		Steps:       cycleMeta.Steps,
		Lats:        lats,
		Lons:        lons,
		LatStep:     cycleMeta.LatStep,
		LonStep:     cycleMeta.LonStep,
		Variables:   cycleMeta.Variables,
		ChunkLat:    chunkLat,
		ChunkLon:    chunkLon,
		NLats:       cycleMeta.NLats,
		NLons:       cycleMeta.NLons,
		NSteps:      len(cycleMeta.Steps),
		chunkCache:  make(map[string][]float32, 1024),
		maxCacheLen: 4096,
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
	cacheKey := fmt.Sprintf("%s/%d.%d", variable, cLat, cLon)

	s.cacheMu.RLock()
	chunkData, ok := s.chunkCache[cacheKey]
	s.cacheMu.RUnlock()

	if !ok {
		// Read and decompress chunk from disk
		chunkPath := filepath.Join(s.RootDir, variable, fmt.Sprintf("0.%d.%d", cLat, cLon))
		compressed, err := os.ReadFile(chunkPath)
		if err != nil {
			// Chunk might not exist (e.g. all NaNs or unwritten), return NaNs
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
			// Simple clear if oversized
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

// StoreWriter writes raw forecast slices and builds chunked Zarr stores with low-memory disk streaming.
type StoreWriter struct {
	mu        sync.Mutex
	dir       string
	slicesDir string
	cycle     *model.ModelCycle
	stepMap   map[int]int // forecastStep -> stepIndex
	steps     []int
	latStart  float64
	latEnd    float64
	latStep   float64
	lonStart  float64
	lonEnd    float64
	lonStep   float64
	nlats     int
	nlons     int
	chunkLat  int
	chunkLon  int
	variables []string
}

// NewStoreWriter initializes a staging Zarr directory.
func NewStoreWriter(dir string, cycle *model.ModelCycle, latStart, latEnd, latStep, lonStart, lonEnd, lonStep float64, variables []string) (*StoreWriter, error) {
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

	sw := &StoreWriter{
		dir:       dir,
		slicesDir: slicesDir,
		cycle:     cycle,
		stepMap:   stepMap,
		steps:     cycle.ForecastSteps,
		latStart:  latStart,
		latEnd:    latEnd,
		latStep:   latStep,
		lonStart:  lonStart,
		lonEnd:    lonEnd,
		lonStep:   lonStep,
		nlats:     nlats,
		nlons:     nlons,
		chunkLat:  DefaultChunkLat,
		chunkLon:  DefaultChunkLon,
		variables: variables,
	}

	return sw, nil
}

// WriteSlice saves a 2D scalar field for a given variable and forecast step to a temporary slice file.
func (sw *StoreWriter) WriteSlice(slice *model.RawGridSlice) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	_, ok := sw.stepMap[slice.StepHours]
	if !ok {
		return fmt.Errorf("unknown forecast step %d for cycle", slice.StepHours)
	}

	gridPoints := sw.nlats * sw.nlons
	rawFloats := slice.Data
	if len(rawFloats) != gridPoints {
		rawFloats = make([]float32, gridPoints)
		for i := range rawFloats {
			rawFloats[i] = float32(math.NaN())
		}
		copyLen := min(len(slice.Data), gridPoints)
		copy(rawFloats[:copyLen], slice.Data[:copyLen])
	}

	// Convert float32s to bytes (4 MB)
	buf := make([]byte, gridPoints*4)
	for i, f := range rawFloats {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], math.Float32bits(f))
	}

	slicePath := filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d.bin", slice.Variable, slice.StepHours))
	if err := os.WriteFile(slicePath, buf, 0644); err != nil {
		return fmt.Errorf("failed to write slice file %s: %w", slicePath, err)
	}

	return nil
}

// Finalize streams temporary slice files into compressed Zarr chunks (<30MB RAM footprint).
func (sw *StoreWriter) Finalize() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	nSteps := len(sw.steps)
	nLatChunks := (sw.nlats + sw.chunkLat - 1) / sw.chunkLat
	nLonChunks := (sw.nlons + sw.chunkLon - 1) / sw.chunkLon
	gridPoints := sw.nlats * sw.nlons

	for _, v := range sw.variables {
		varDir := filepath.Join(sw.dir, v)
		if err := os.MkdirAll(varDir, 0755); err != nil {
			return err
		}

		// Write .zarray metadata
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
		if err := os.WriteFile(filepath.Join(varDir, ".zarray"), metaJSON, 0644); err != nil {
			return err
		}

		// Process in latitude bands: read 16 rows across all steps (~7.4 MB RAM)
		for cLat := 0; cLat < nLatChunks; cLat++ {
			latBase := cLat * sw.chunkLat
			nBandLats := min(sw.chunkLat, sw.nlats-latBase)
			bandPointsPerStep := nBandLats * sw.nlons

			// Load band rows from step files
			bandData := make([]float32, nSteps*bandPointsPerStep)
			for i := range bandData {
				bandData[i] = float32(math.NaN())
			}

			for stepIdx, step := range sw.steps {
				slicePath := filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d.bin", v, step))
				rawBytes, err := os.ReadFile(slicePath)
				if err == nil && len(rawBytes) == gridPoints*4 {
					stepBandOffset := stepIdx * bandPointsPerStep
					rowByteOffset := latBase * sw.nlons * 4
					bandByteLen := bandPointsPerStep * 4

					if rowByteOffset+bandByteLen <= len(rawBytes) {
						bandBytes := rawBytes[rowByteOffset : rowByteOffset+bandByteLen]
						for k := 0; k < bandPointsPerStep; k++ {
							bits := binary.LittleEndian.Uint32(bandBytes[k*4 : (k+1)*4])
							bandData[stepBandOffset+k] = math.Float32frombits(bits)
						}
					}
				}
			}

			// Pack each spatial chunk in this latitude band
			chunkSize := nSteps * sw.chunkLat * sw.chunkLon
			chunkBuf := make([]byte, chunkSize*4)

			for cLon := 0; cLon < nLonChunks; cLon++ {
				lonBase := cLon * sw.chunkLon
				var hasValidData bool
				chunkFloatIdx := 0

				for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
					stepBandOffset := stepIdx * bandPointsPerStep

					for dLat := 0; dLat < sw.chunkLat; dLat++ {
						latInBand := dLat
						for dLon := 0; dLon < sw.chunkLon; dLon++ {
							lon := lonBase + dLon

							var val float32 = float32(math.NaN())
							if latInBand < nBandLats && lon < sw.nlons {
								val = bandData[stepBandOffset+latInBand*sw.nlons+lon]
								if !math.IsNaN(float64(val)) {
									hasValidData = true
								}
							}

							binary.LittleEndian.PutUint32(chunkBuf[chunkFloatIdx*4:(chunkFloatIdx+1)*4], math.Float32bits(val))
							chunkFloatIdx++
						}
					}
				}

				if !hasValidData {
					continue
				}

				// Compress chunk with Zstandard
				compressed := CompressZstd(chunkBuf[:chunkFloatIdx*4])
				chunkPath := filepath.Join(varDir, fmt.Sprintf("0.%d.%d", cLat, cLon))
				if err := os.WriteFile(chunkPath, compressed, 0644); err != nil {
					return fmt.Errorf("failed to write chunk %s: %w", chunkPath, err)
				}
			}
		}
	}

	// Remove temporary .slices folder
	_ = os.RemoveAll(sw.slicesDir)

	// Write metadata.json for the store
	storeMeta := struct {
		ModelName     string    `json:"model_name"`
		ReferenceTime time.Time `json:"reference_time"`
		ResolutionDeg float64   `json:"resolution_deg"`
		Steps         []int     `json:"steps"`
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
		Variables:     sw.variables,
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
