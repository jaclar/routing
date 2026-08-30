package isochrone

import (
	"math"
	"sort"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/weather"
)

// PruningStrategy defines the algorithm used to deduplicate and select surviving frontier nodes.
type PruningStrategy string

const (
	PruningRadialSector   PruningStrategy = "radial_sector"    // 1. Classic Chichester/Franke radial sector bucketing
	PruningSpatialGrid    PruningStrategy = "spatial_grid"     // 2. 2D Local Spatial Grid Dominance (Default)
	PruningAStarBeam      PruningStrategy = "astar_beam"       // 3. Heuristic A* Beam Search with spatial diversity
	PruningParetoEnvelope PruningStrategy = "pareto_envelope"  // 4. Non-Dominated Pareto Progress Envelope
	PruningStateSpaceGrid PruningStrategy = "state_space_grid" // 5. State-Space Grid (Lat, Lon, Heading, Tack, Sail Mode)
)

// pruneFrontier dispatches candidate pruning to the requested PruningStrategy.
func pruneFrontier(
	candidates []*Node,
	start, dest geo.Point,
	maxNodes int,
	timeStep time.Duration,
	strategy PruningStrategy,
	startTime time.Time,
) []*Node {
	if len(candidates) <= 20 {
		return candidates
	}

	switch strategy {
	case PruningRadialSector:
		return pruneRadialSector(candidates, start, dest, maxNodes)
	case PruningAStarBeam:
		return pruneAStarBeam(candidates, start, dest, maxNodes, timeStep, startTime)
	case PruningParetoEnvelope:
		return pruneParetoEnvelope(candidates, start, dest, maxNodes, timeStep)
	case PruningStateSpaceGrid:
		return pruneStateSpaceGrid(candidates, start, dest, maxNodes, timeStep)
	case PruningSpatialGrid:
		fallthrough
	default:
		return pruneSpatialGrid(candidates, start, dest, maxNodes, timeStep)
	}
}

// 1. Radial / Angular Sector Bucketing (Classic Chichester / Hagiwara Isochrone)
func pruneRadialSector(candidates []*Node, start, dest geo.Point, maxNodes int) []*Node {
	directBearing := geo.InitialBearing(start, dest)

	type candAngle struct {
		cand     *Node
		relAngle float64
	}

	candAngles := make([]candAngle, len(candidates))
	minAngle := 180.0
	maxAngle := -180.0

	for i, c := range candidates {
		bearing := geo.InitialBearing(start, c.Point)
		rel := geo.NormalizeAngle360(bearing - directBearing)
		if rel > 180.0 {
			rel -= 360.0
		}
		candAngles[i] = candAngle{cand: c, relAngle: rel}
		if rel < minAngle {
			minAngle = rel
		}
		if rel > maxAngle {
			maxAngle = rel
		}
	}

	span := maxAngle - minAngle
	if span < 0.1 {
		span = 0.1
	}

	numSectors := maxNodes
	if numSectors > 300 {
		numSectors = 300
	}
	if numSectors <= 0 {
		numSectors = 100
	}

	sectorMap := make(map[int]*Node, numSectors)

	for _, ca := range candAngles {
		sectorIdx := int(math.Floor(((ca.relAngle - minAngle) / span) * float64(numSectors)))
		if sectorIdx >= numSectors {
			sectorIdx = numSectors - 1
		}
		if sectorIdx < 0 {
			sectorIdx = 0
		}

		existing, found := sectorMap[sectorIdx]
		if !found || ca.cand.DistanceToDest < existing.DistanceToDest {
			sectorMap[sectorIdx] = ca.cand
		}
	}

	survivors := make([]*Node, 0, len(sectorMap))
	for _, node := range sectorMap {
		survivors = append(survivors, node)
	}

	if len(survivors) > maxNodes && maxNodes > 0 {
		sort.Slice(survivors, func(i, j int) bool {
			return survivors[i].DistanceToDest < survivors[j].DistanceToDest
		})
		survivors = survivors[:maxNodes]
	}

	sortWavefront(survivors)
	return survivors
}

// 2. 2D Local Spatial Grid Dominance (Default)
func pruneSpatialGrid(candidates []*Node, start, dest geo.Point, maxNodes int, timeStep time.Duration) []*Node {
	stepDistEstNM := math.Max(0.8, timeStep.Hours()*6.5)
	cellSizeDeg := math.Max(0.015, (stepDistEstNM/60.0)*0.75)

	type spatialKey struct {
		latIdx  int
		lonIdx  int
		quadIdx int
	}

	bucketMap := make(map[spatialKey]*Node, len(candidates))

	for _, cand := range candidates {
		latIdx := int(math.Floor((cand.Point.Lat + 90.0) / cellSizeDeg))
		lonIdx := int(math.Floor((cand.Point.Lon + 180.0) / cellSizeDeg))
		quadIdx := int(math.Floor(cand.Heading/90.0)) % 4
		if quadIdx < 0 {
			quadIdx += 4
		}

		key := spatialKey{latIdx: latIdx, lonIdx: lonIdx, quadIdx: quadIdx}

		existing, found := bucketMap[key]
		if !found || cand.DistanceToDest < existing.DistanceToDest {
			bucketMap[key] = cand
		}
	}

	survivors := make([]*Node, 0, len(bucketMap))
	for _, node := range bucketMap {
		survivors = append(survivors, node)
	}

	if len(survivors) > maxNodes && maxNodes > 0 {
		sort.Slice(survivors, func(i, j int) bool {
			return survivors[i].DistanceToDest < survivors[j].DistanceToDest
		})
		survivors = survivors[:maxNodes]
	}

	sortWavefront(survivors)
	return survivors
}

// 3. Heuristic A* Beam Search (Score f(n) = g(n) + h(n) with spatial diversity preservation)
func pruneAStarBeam(candidates []*Node, start, dest geo.Point, maxNodes int, timeStep time.Duration, startTime time.Time) []*Node {
	const maxSpeedMS = 8.5 * weather.KnotsToMS

	type scoredNode struct {
		node  *Node
		score float64
	}

	scored := make([]scoredNode, len(candidates))
	for i, c := range candidates {
		g := c.Time.Sub(startTime).Seconds()
		h := c.DistanceToDest / maxSpeedMS
		scored[i] = scoredNode{
			node:  c,
			score: g + h,
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	// Spatial diversity filter: ensure beam does not collapse into single cluster
	stepDistEstNM := math.Max(0.8, timeStep.Hours()*6.5)
	cellSizeDeg := math.Max(0.015, (stepDistEstNM/60.0)*0.5)

	type cellKey struct {
		latIdx int
		lonIdx int
	}
	cellCounts := make(map[cellKey]int)
	survivors := make([]*Node, 0, maxNodes)

	for _, s := range scored {
		latIdx := int(math.Floor((s.node.Point.Lat + 90.0) / cellSizeDeg))
		lonIdx := int(math.Floor((s.node.Point.Lon + 180.0) / cellSizeDeg))
		key := cellKey{latIdx: latIdx, lonIdx: lonIdx}

		count := cellCounts[key]
		if count < 2 { // Allow up to 2 nodes per local cell for beam width
			cellCounts[key] = count + 1
			survivors = append(survivors, s.node)
			if len(survivors) >= maxNodes && maxNodes > 0 {
				break
			}
		}
	}

	sortWavefront(survivors)
	return survivors
}

// 4. Non-Dominated Pareto Progress Envelope
func pruneParetoEnvelope(candidates []*Node, start, dest geo.Point, maxNodes int, timeStep time.Duration) []*Node {
	directBearing := geo.InitialBearing(start, dest)
	stepDistEstNM := math.Max(0.8, timeStep.Hours()*6.5)
	binSizeNM := math.Max(1.5, stepDistEstNM*0.8)

	type paretoKey struct {
		binIdx  int
		quadIdx int
	}

	bestInBin := make(map[paretoKey]*Node, len(candidates))

	for _, cand := range candidates {
		// Calculate cross-track offset from great circle route line
		bearingFromStart := geo.InitialBearing(start, cand.Point)
		relAngleDeg := geo.NormalizeAngle360(bearingFromStart - directBearing)
		if relAngleDeg > 180.0 {
			relAngleDeg -= 360.0
		}

		distFromStartNM := geo.DistanceMeters(start, cand.Point) * geo.MetersToNM
		crossTrackNM := distFromStartNM * math.Sin(geo.DegToRad(relAngleDeg))

		binIdx := int(math.Floor(crossTrackNM / binSizeNM))
		quadIdx := int(math.Floor(cand.Heading/90.0)) % 4
		if quadIdx < 0 {
			quadIdx += 4
		}

		key := paretoKey{binIdx: binIdx, quadIdx: quadIdx}

		existing, found := bestInBin[key]
		if !found || cand.DistanceToDest < existing.DistanceToDest {
			bestInBin[key] = cand
		}
	}

	survivors := make([]*Node, 0, len(bestInBin))
	for _, node := range bestInBin {
		survivors = append(survivors, node)
	}

	if len(survivors) > maxNodes && maxNodes > 0 {
		sort.Slice(survivors, func(i, j int) bool {
			return survivors[i].DistanceToDest < survivors[j].DistanceToDest
		})
		survivors = survivors[:maxNodes]
	}

	sortWavefront(survivors)
	return survivors
}

// 5. State-Space Grid (Lat, Lon, Heading, Tack, Sail Mode)
func pruneStateSpaceGrid(candidates []*Node, start, dest geo.Point, maxNodes int, timeStep time.Duration) []*Node {
	stepDistEstNM := math.Max(0.8, timeStep.Hours()*6.5)
	cellSizeDeg := math.Max(0.015, (stepDistEstNM/60.0)*0.75)

	type stateSpaceKey struct {
		latIdx      int
		lonIdx      int
		quadIdx     int
		tackIdx     int // 0 = Port Tack, 1 = Starboard Tack
		sailModeIdx int // 0 = Beating (<60°), 1 = Reaching (60-135°), 2 = Running (>135°)
	}

	bucketMap := make(map[stateSpaceKey]*Node, len(candidates))

	for _, cand := range candidates {
		latIdx := int(math.Floor((cand.Point.Lat + 90.0) / cellSizeDeg))
		lonIdx := int(math.Floor((cand.Point.Lon + 180.0) / cellSizeDeg))
		quadIdx := int(math.Floor(cand.Heading/90.0)) % 4
		if quadIdx < 0 {
			quadIdx += 4
		}

		// Tack identification: relative angle to true wind direction
		relWind := geo.NormalizeAngle360(cand.Heading - cand.TWD)
		if relWind > 180.0 {
			relWind -= 360.0
		}
		tackIdx := 0
		if relWind >= 0 {
			tackIdx = 1
		}

		// Point of sail mode
		sailModeIdx := 1 // Reaching
		if cand.TWA < 60.0 {
			sailModeIdx = 0 // Close-hauled / Beating
		} else if cand.TWA > 135.0 {
			sailModeIdx = 2 // Downwind / Running
		}

		key := stateSpaceKey{
			latIdx:      latIdx,
			lonIdx:      lonIdx,
			quadIdx:     quadIdx,
			tackIdx:     tackIdx,
			sailModeIdx: sailModeIdx,
		}

		existing, found := bucketMap[key]
		if !found || cand.DistanceToDest < existing.DistanceToDest {
			bucketMap[key] = cand
		}
	}

	survivors := make([]*Node, 0, len(bucketMap))
	for _, node := range bucketMap {
		survivors = append(survivors, node)
	}

	if len(survivors) > maxNodes && maxNodes > 0 {
		sort.Slice(survivors, func(i, j int) bool {
			return survivors[i].DistanceToDest < survivors[j].DistanceToDest
		})
		survivors = survivors[:maxNodes]
	}

	sortWavefront(survivors)
	return survivors
}

func sortWavefront(nodes []*Node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Point.Lon != nodes[j].Point.Lon {
			return nodes[i].Point.Lon < nodes[j].Point.Lon
		}
		return nodes[i].Point.Lat < nodes[j].Point.Lat
	})
}
