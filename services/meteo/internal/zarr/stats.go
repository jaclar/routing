package zarr

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"sailboat/meteo/internal/model"
)

// reduceBandRows is how many grid rows are reduced at a time. Reducing in bands keeps peak
// memory proportional to the band rather than to (ensemble size x full grid), which matters
// for the larger ensembles: 50 ECMWF members at 0.25 degrees is over 200MB per forecast step.
const reduceBandRows = 32

// nan32 is the fill value for grid points with no usable data.
var nan32 = float32(math.NaN())

// statSuffixes names the statistics derived for every ensemble variable, in the order gridStats
// stores them. They are published as separate Zarr arrays, e.g. "wind_u_10m_p90".
var statSuffixes = [5]string{"mean", "std", "p10", "p50", "p90"}

// gridStats holds one full-grid field per entry in statSuffixes.
type gridStats [5][]float32

func newGridStats(gridPoints int) gridStats {
	var g gridStats
	for i := range g {
		g[i] = make([]float32, gridPoints)
	}
	return g
}

// stepKey identifies the ensemble of one variable at one forecast step.
type stepKey struct {
	variable string
	step     int
}

// windThresholds are the wind speeds whose exceedance probability is precomputed, named by the
// Zarr array each probability is published as.
var windThresholds = []struct {
	array string
	speed float64
}{
	{"prob_wind_ge_25kt", 25.0 * model.KnotsToMS},
	{"prob_wind_ge_34kt", 34.0 * model.KnotsToMS},
}

// windDerivedArrays lists every array produced from the paired u/v ensembles.
func windDerivedArrays() []string {
	out := make([]string, 0, len(statSuffixes)+len(windThresholds))
	for _, s := range statSuffixes {
		out = append(out, "wind_speed_"+s)
	}
	for _, t := range windThresholds {
		out = append(out, t.array)
	}
	return out
}

// memberSlicePath is where the raw grid for one member of a (variable, step) is staged.
func (sw *StoreWriter) memberSlicePath(variable string, step, member int) string {
	return filepath.Join(sw.slicesDir, fmt.Sprintf("%s_%d_m%d.bin", variable, step, member))
}

// statGridPath is where a reduced statistic grid is staged until Finalize packs it into chunks.
func (sw *StoreWriter) statGridPath(array string, step int) string {
	return filepath.Join(sw.statsDir, fmt.Sprintf("%s_%d.bin", array, step))
}

// reduceStep collapses every member of one variable at one forecast step into ensemble
// statistics and stages them. Called as soon as the last member arrives, so this work overlaps
// with the remaining downloads instead of running as a serial tail in Finalize.
func (sw *StoreWriter) reduceStep(variable string, step int) error {
	gridPoints := sw.nlats * sw.nlons
	out := newGridStats(gridPoints)

	bands := sw.newMemberBands()
	loaders := make([]bandLoader, len(sw.members))
	sample := make([]float32, len(sw.members))

	for rowBase := 0; rowBase < sw.nlats; rowBase += reduceBandRows {
		n := min(reduceBandRows, sw.nlats-rowBase) * sw.nlons
		offset := rowBase * sw.nlons

		for mIdx, mID := range sw.members {
			sw.loadMemberBand(&loaders[mIdx], variable, step, mID, offset, n, bands[mIdx])
		}

		for p := 0; p < n; p++ {
			valid := 0
			for _, b := range bands {
				if v := b[p]; !math.IsNaN(float64(v)) {
					sample[valid] = v
					valid++
				}
			}

			target := offset + p
			if valid == 0 {
				for s := range out {
					out[s][target] = nan32
				}
				continue
			}

			mean, std, p10, p50, p90 := calcStats(sample[:valid])
			out[0][target], out[1][target] = mean, std
			out[2][target], out[3][target], out[4][target] = p10, p50, p90
		}
	}

	for i, suffix := range statSuffixes {
		if err := writeGrid(sw.statGridPath(variable+"_"+suffix, step), out[i]); err != nil {
			return err
		}
	}
	return nil
}

// reduceWindStep derives wind-speed statistics and threshold exceedance probabilities for one
// forecast step from the paired u/v member grids. It runs once both components of the step have
// been reduced, which is also the point at which their member grids become disposable.
func (sw *StoreWriter) reduceWindStep(step int) error {
	gridPoints := sw.nlats * sw.nlons
	speed := newGridStats(gridPoints)

	probs := make([][]float32, len(windThresholds))
	for i := range probs {
		probs[i] = make([]float32, gridPoints)
	}

	uBands, vBands := sw.newMemberBands(), sw.newMemberBands()
	uLoaders := make([]bandLoader, len(sw.members))
	vLoaders := make([]bandLoader, len(sw.members))
	sample := make([]float32, len(sw.members))
	exceed := make([]int, len(windThresholds))

	for rowBase := 0; rowBase < sw.nlats; rowBase += reduceBandRows {
		n := min(reduceBandRows, sw.nlats-rowBase) * sw.nlons
		offset := rowBase * sw.nlons

		for mIdx, mID := range sw.members {
			sw.loadMemberBand(&uLoaders[mIdx], model.VarWindU10m, step, mID, offset, n, uBands[mIdx])
			sw.loadMemberBand(&vLoaders[mIdx], model.VarWindV10m, step, mID, offset, n, vBands[mIdx])
		}

		for p := 0; p < n; p++ {
			valid := 0
			for i := range exceed {
				exceed[i] = 0
			}

			for mIdx := range sw.members {
				u, v := uBands[mIdx][p], vBands[mIdx][p]
				if math.IsNaN(float64(u)) || math.IsNaN(float64(v)) {
					continue
				}
				spd := math.Hypot(float64(u), float64(v))
				sample[valid] = float32(spd)
				valid++
				for i, t := range windThresholds {
					if spd >= t.speed {
						exceed[i]++
					}
				}
			}

			target := offset + p
			if valid == 0 {
				for s := range speed {
					speed[s][target] = nan32
				}
				for i := range probs {
					probs[i][target] = nan32
				}
				continue
			}

			mean, std, p10, p50, p90 := calcStats(sample[:valid])
			speed[0][target], speed[1][target] = mean, std
			speed[2][target], speed[3][target], speed[4][target] = p10, p50, p90
			for i := range probs {
				probs[i][target] = float32(exceed[i]) / float32(valid)
			}
		}
	}

	for i, suffix := range statSuffixes {
		if err := writeGrid(sw.statGridPath("wind_speed_"+suffix, step), speed[i]); err != nil {
			return err
		}
	}
	for i, t := range windThresholds {
		if err := writeGrid(sw.statGridPath(t.array, step), probs[i]); err != nil {
			return err
		}
	}
	return nil
}

// newMemberBands allocates one reusable band buffer per ensemble member.
func (sw *StoreWriter) newMemberBands() [][]float32 {
	bands := make([][]float32, len(sw.members))
	for i := range bands {
		bands[i] = make([]float32, reduceBandRows*sw.nlons)
	}
	return bands
}

// loadMemberBand fills band[:n] with one member's rows, leaving NaN where the member is missing.
func (sw *StoreWriter) loadMemberBand(bl *bandLoader, variable string, step, member, offset, n int, band []float32) {
	dst := band[:n]
	for i := range dst {
		dst[i] = nan32
	}
	bl.load(sw.memberSlicePath(variable, step, member), offset, n, dst)
}

// releaseMemberSlices deletes the raw member grids of a step once every statistic that needs
// them has been staged. This is what keeps a full ensemble from accumulating on disk for the
// whole ingestion; when the full ensemble is being persisted the grids are kept, because
// Finalize still has to pack them into 4D member chunks.
func (sw *StoreWriter) releaseMemberSlices(variable string, step int) {
	if sw.storeFullEnsemble {
		return
	}
	for _, mID := range sw.members {
		_ = os.Remove(sw.memberSlicePath(variable, step, mID))
	}
}

// afterStepReduced releases the member grids a freshly reduced step no longer needs. The wind
// components are held back until the derived wind-speed statistics for that step have also been
// computed, since those need the raw per-member u/v pairs.
func (sw *StoreWriter) afterStepReduced(variable string, step int) error {
	isWindComponent := variable == model.VarWindU10m || variable == model.VarWindV10m
	if !sw.windDerived || !isWindComponent {
		sw.releaseMemberSlices(variable, step)
		return nil
	}

	sw.mu.Lock()
	pairReady := sw.reduced[stepKey{model.VarWindU10m, step}] &&
		sw.reduced[stepKey{model.VarWindV10m, step}] &&
		!sw.windDone[step]
	if pairReady {
		sw.windDone[step] = true
	}
	sw.mu.Unlock()

	if !pairReady {
		return nil
	}

	if err := sw.reduceWindStep(step); err != nil {
		return fmt.Errorf("failed to derive wind statistics for step %d: %w", step, err)
	}

	sw.releaseMemberSlices(model.VarWindU10m, step)
	sw.releaseMemberSlices(model.VarWindV10m, step)
	return nil
}

// writeGrid stores a full-grid float32 field as little-endian binary.
func writeGrid(path string, values []float32) error {
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	if err := os.WriteFile(path, buf, 0644); err != nil {
		return fmt.Errorf("failed to write grid %s: %w", path, err)
	}
	return nil
}

// calcStats returns the mean, sample standard deviation and p10/p50/p90 of vals.
// vals is sorted in place, so callers must pass a scratch slice they own.
func calcStats(vals []float32) (mean, std, p10, p50, p90 float32) {
	n := len(vals)
	if n == 0 {
		return nan32, nan32, nan32, nan32, nan32
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

	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })

	return mean, std, calcPercentile(vals, 10.0), calcPercentile(vals, 50.0), calcPercentile(vals, 90.0)
}

// calcPercentile computes a linear-rank percentile over an ascending slice.
func calcPercentile(sorted []float32, p float64) float32 {
	n := len(sorted)
	if n == 0 {
		return nan32
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
