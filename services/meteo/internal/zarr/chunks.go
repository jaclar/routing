package zarr

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// packJob describes one Zarr array to build from the per-forecast-step grid files staged on disk.
// The same compressed chunk is written to every directory in outDirs, so a field published under
// two names (a canonical variable and its "_mean" alias) is only compressed once.
type packJob struct {
	name      string
	outDirs   []string
	stepFile  func(step int) string
	chunkName func(cLat, cLon int) string
}

// chunk3DName is the Zarr V2 chunk key for a [step, lat, lon] array.
func chunk3DName(cLat, cLon int) string {
	return fmt.Sprintf("0.%d.%d", cLat, cLon)
}

// chunk4DName builds the Zarr V2 chunk key for one member of a [step, member, lat, lon] array.
func chunk4DName(member int) func(cLat, cLon int) string {
	return func(cLat, cLon int) string {
		return fmt.Sprintf("0.%d.%d.%d", member, cLat, cLon)
	}
}

// writeZArray creates an array directory and writes its Zarr V2 .zarray metadata.
func writeZArray(dir string, shape, chunks []int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	meta := ZArrayMeta{
		ZarrFormat: 2,
		Shape:      shape,
		Chunks:     chunks,
		DType:      "<f4",
		Order:      "C",
		FillValue:  "NaN",
	}
	meta.Compressor.ID = "zstd"
	meta.Compressor.Level = 3

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".zarray"), data, 0644)
}

// bandLoader reads horizontal bands of rows out of full-grid float32 files. It reads only the
// bytes the band covers rather than the whole grid, and reuses its buffer across calls.
type bandLoader struct {
	raw []byte
}

// load fills dst with bandPoints values starting at grid offset rowOffset, reporting whether the
// file supplied them. A missing or truncated file leaves dst untouched.
func (bl *bandLoader) load(path string, rowOffset, bandPoints int, dst []float32) bool {
	need := bandPoints * 4
	if cap(bl.raw) < need {
		bl.raw = make([]byte, need)
	}
	buf := bl.raw[:need]

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	if _, err := f.ReadAt(buf, int64(rowOffset)*4); err != nil {
		return false
	}

	for i := 0; i < bandPoints; i++ {
		dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return true
}

// packVariable builds every chunk of one Zarr array from its per-step grid files. The grid is
// walked one latitude band at a time so peak memory stays proportional to a single chunk row
// rather than to the whole forecast.
func (sw *StoreWriter) packVariable(job packJob) error {
	nSteps := len(sw.steps)
	nLatChunks := (sw.nlats + sw.chunkLat - 1) / sw.chunkLat
	nLonChunks := (sw.nlons + sw.chunkLon - 1) / sw.chunkLon

	stepStride := sw.chunkLat * sw.nlons
	band := make([]float32, nSteps*stepStride)
	chunkBuf := make([]byte, nSteps*sw.chunkLat*sw.chunkLon*4)

	var loader bandLoader

	for cLat := 0; cLat < nLatChunks; cLat++ {
		latBase := cLat * sw.chunkLat
		nBandLats := min(sw.chunkLat, sw.nlats-latBase)
		bandPoints := nBandLats * sw.nlons

		for i := range band {
			band[i] = nan32
		}
		for stepIdx, step := range sw.steps {
			dst := band[stepIdx*stepStride:][:bandPoints]
			loader.load(job.stepFile(step), latBase*sw.nlons, bandPoints, dst)
		}

		for cLon := 0; cLon < nLonChunks; cLon++ {
			lonBase := cLon * sw.chunkLon

			idx := 0
			hasData := false
			for stepIdx := 0; stepIdx < nSteps; stepIdx++ {
				base := stepIdx * stepStride
				for dLat := 0; dLat < sw.chunkLat; dLat++ {
					for dLon := 0; dLon < sw.chunkLon; dLon++ {
						val := nan32
						if lon := lonBase + dLon; dLat < nBandLats && lon < sw.nlons {
							val = band[base+dLat*sw.nlons+lon]
							if !math.IsNaN(float64(val)) {
								hasData = true
							}
						}
						binary.LittleEndian.PutUint32(chunkBuf[idx*4:], math.Float32bits(val))
						idx++
					}
				}
			}

			if !hasData {
				continue
			}

			compressed := CompressZstd(chunkBuf[:idx*4])
			name := job.chunkName(cLat, cLon)
			for _, dir := range job.outDirs {
				if err := os.WriteFile(filepath.Join(dir, name), compressed, 0644); err != nil {
					return fmt.Errorf("failed to write chunk %s for %s: %w", name, job.name, err)
				}
			}
		}
	}

	return nil
}

// runPackJobs packs arrays across all available cores, since each array is built independently.
func (sw *StoreWriter) runPackJobs(jobs []packJob) error {
	workers := runtime.NumCPU()
	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers < 1 {
		return nil
	}

	queue := make(chan packJob, len(jobs))
	for _, j := range jobs {
		queue <- j
	}
	close(queue)

	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue {
				if err := sw.packVariable(job); err != nil {
					errOnce.Do(func() { firstErr = err })
					return
				}
			}
		}()
	}
	wg.Wait()

	return firstErr
}
