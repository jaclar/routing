package isochrone

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

// Node represents a single state point on the isochrone wavefront.
type Node struct {
	Point            geo.Point
	Time             time.Time
	Parent           *Node
	DistanceToDest   float64 // [meters]
	Heading          float64 // [degrees]
	TWS              float64 // [knots]
	TWD              float64 // [degrees]
	TWA              float64 // [degrees]
	BoatSpeed        float64 // [knots]
	DistanceTraveled float64 // [NM]
	Maneuver         string  // "none", "tack", "gybe"
	TackCount        int
	GybeCount        int
}

// Waypoint represents an output waypoint on the solved optimal route.
type Waypoint struct {
	Lat              float64   `json:"lat"`
	Lon              float64   `json:"lon"`
	Time             time.Time `json:"time"`
	HeadingDeg       float64   `json:"heading_deg"`
	BoatSpeedKts     float64   `json:"boat_speed_kts"`
	TWSKts           float64   `json:"tws_kts"`
	TWDDeg           float64   `json:"twd_deg"`
	TWADeg           float64   `json:"twa_deg"`
	DistanceNM       float64   `json:"distance_nm"`
	DistanceToDestNM float64   `json:"distance_to_dest_nm"`
	EstimatedHeelDeg float64   `json:"estimated_heel_deg"`
	Maneuver         string    `json:"maneuver,omitempty"` // "none", "tack", "gybe"
}

// IsochroneWave represents a single time-frontier line on the map.
type IsochroneWave struct {
	StepIndex int         `json:"step_index"`
	Time      time.Time   `json:"time"`
	Points    []geo.Point `json:"points"`
}

// RouteResult holds the complete calculated weather routing solution.
type RouteResult struct {
	BoatName            string          `json:"boat_name"`
	StartPoint          geo.Point       `json:"start_point"`
	DestPoint           geo.Point       `json:"dest_point"`
	StartTime           time.Time       `json:"start_time"`
	ArrivalTime         time.Time       `json:"arrival_time"`
	TotalDurationHours  float64         `json:"total_duration_hours"`
	TotalDistanceNM     float64         `json:"total_distance_nm"`
	DirectDistanceNM    float64         `json:"direct_distance_nm"`
	AverageSpeedKts     float64         `json:"average_speed_kts"`
	MaxWindEncountered  float64         `json:"max_wind_kts"`
	TotalTacks          int             `json:"total_tacks"`
	TotalGybes          int             `json:"total_gybes"`
	TackPenaltyMinutes  float64         `json:"tack_penalty_minutes"`
	GybePenaltyMinutes  float64         `json:"gybe_penalty_minutes"`
	Waypoints           []Waypoint      `json:"waypoints"`
	Isochrones          []IsochroneWave `json:"isochrones"`
	DestinationReached  bool            `json:"destination_reached"`
}

// RouterConfig contains tuning parameters for the isochrone propagation.
type RouterConfig struct {
	TimeStep           time.Duration
	MaxDurationHours   float64
	ArrivalRadiusNM    float64
	HeadingSpreadDeg   float64
	HeadingStepDeg     float64
	NumAngularBins     int
	TackPenaltyMinutes float64
	GybePenaltyMinutes float64
}

func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		TimeStep:           2 * time.Hour,
		MaxDurationHours:   400.0, // Up to ~16 days max for long ocean passages
		ArrivalRadiusNM:    15.0,  // Arrival capture radius
		HeadingSpreadDeg:   125.0, // +/- 125 degrees from destination bearing for wide tacking/gybing
		HeadingStepDeg:     5.0,   // Ray angular spacing
		NumAngularBins:     120,   // Frontier pruning sectors
		TackPenaltyMinutes: 5.0,   // Default 5 minutes lost per tack for cruisers
		GybePenaltyMinutes: 8.0,   // Default 8 minutes lost per gybe for cruisers
	}
}

// DetectManeuver checks if a course change across the wind from parentHeading to newHeading is a Tack or Gybe.
func DetectManeuver(parentHeading, newHeading, twd float64) string {
	relParent := geo.NormalizeAngle360(parentHeading - twd)
	if relParent > 180.0 {
		relParent -= 360.0
	}

	relNew := geo.NormalizeAngle360(newHeading - twd)
	if relNew > 180.0 {
		relNew -= 360.0
	}

	// If boat stays on same tack (same sign of relative angle), no maneuver
	if relParent*relNew >= 0 || math.Abs(relParent) < 1.0 || math.Abs(relNew) < 1.0 {
		return "none"
	}

	// Crossed the wind:
	// Upwind/reaching cross (< 90 deg) -> TACK through bow
	// Downwind/running cross (>= 90 deg) -> GYBE through stern
	avgAbsAngle := (math.Abs(relParent) + math.Abs(relNew)) / 2.0
	if avgAbsAngle < 90.0 {
		return "tack"
	}
	return "gybe"
}

// CalculateOptimalRoute executes the isochrone weather routing search.
func CalculateOptimalRoute(
	start geo.Point,
	dest geo.Point,
	startTime time.Time,
	polarTable *polar.PolarTable,
	weatherProvider weather.WeatherProvider,
	landMask *landmask.LandMask,
	cfg RouterConfig,
) (*RouteResult, error) {
	directDistMeters := geo.DistanceMeters(start, dest)
	directDistNM := directDistMeters * geo.MetersToNM

	if directDistNM < 0.5 {
		return nil, fmt.Errorf("start and destination are too close (%.2f NM)", directDistNM)
	}

	if landMask != nil && landMask.IsLand(start) {
		return nil, fmt.Errorf("start location (%.3f, %.3f) is on land", start.Lat, start.Lon)
	}
	if landMask != nil && landMask.IsLand(dest) {
		return nil, fmt.Errorf("destination location (%.3f, %.3f) is on land", dest.Lat, dest.Lon)
	}

	timeStepSec := cfg.TimeStep.Seconds()
	arrivalRadiusMeters := cfg.ArrivalRadiusNM * geo.NMToMeters

	// Root start node
	initWind := weatherProvider.GetWind(start.Lat, start.Lon, startTime)
	initBearing := geo.InitialBearing(start, dest)
	initTWA := math.Abs(geo.NormalizeAngle360(initBearing - initWind.TWD))
	if initTWA > 180.0 {
		initTWA = 360.0 - initTWA
	}

	startNode := &Node{
		Point:            start,
		Time:             startTime,
		Parent:           nil,
		DistanceToDest:   directDistMeters,
		Heading:          initBearing,
		TWS:              initWind.TWS,
		TWD:              initWind.TWD,
		TWA:              initTWA,
		BoatSpeed:        0.0,
		DistanceTraveled: 0.0,
		Maneuver:         "none",
		TackCount:        0,
		GybeCount:        0,
	}

	frontier := []*Node{startNode}
	isochrones := make([]IsochroneWave, 0)
	var bestArrivalNode *Node
	closestNode := startNode

	step := 0
	currentTime := startTime

	for len(frontier) > 0 && currentTime.Sub(startTime).Hours() < cfg.MaxDurationHours {
		step++
		currentTime = currentTime.Add(cfg.TimeStep)

		// 1. Record current isochrone wave for visualization
		wavePoints := make([]geo.Point, len(frontier))
		for i, n := range frontier {
			wavePoints[i] = n.Point
		}
		isochrones = append(isochrones, IsochroneWave{
			StepIndex: step - 1,
			Time:      frontier[0].Time,
			Points:    wavePoints,
		})

		// 2. Check if destination is reached
		for _, n := range frontier {
			if n.DistanceToDest <= arrivalRadiusMeters {
				if bestArrivalNode == nil || n.DistanceToDest < bestArrivalNode.DistanceToDest {
					bestArrivalNode = n
				}
			}
			if n.DistanceToDest < closestNode.DistanceToDest {
				closestNode = n
			}
		}

		if bestArrivalNode != nil {
			break
		}

		// 3. Propagate next generation of candidate nodes
		candidates := make([]*Node, 0, len(frontier)*30)

		for _, n := range frontier {
			destBearing := geo.InitialBearing(n.Point, dest)

			// Heading fan from (destBearing - spread) to (destBearing + spread)
			for dH := -cfg.HeadingSpreadDeg; dH <= cfg.HeadingSpreadDeg; dH += cfg.HeadingStepDeg {
				heading := geo.NormalizeAngle360(destBearing + dH)

				// Sample weather at origin node
				wind := weatherProvider.GetWind(n.Point.Lat, n.Point.Lon, n.Time)

				// Calculate True Wind Angle (TWA)
				twa := math.Abs(geo.NormalizeAngle360(heading - wind.TWD))
				if twa > 180.0 {
					twa = 360.0 - twa
				}

				// Reject headings inside the Aerodynamic No-Go Zone (< 28 deg to wind)
				if twa < 28.0 {
					continue
				}

				// Look up boat speed
				boatSpeedKts := polarTable.InterpolateSpeed(wind.TWS, twa)
				if boatSpeedKts <= 0.3 {
					continue
				}

				// Detect Tack or Gybe maneuver relative to current node heading
				maneuver := "none"
				penaltySec := 0.0
				newTackCount := n.TackCount
				newGybeCount := n.GybeCount

				if n.Parent != nil {
					maneuver = DetectManeuver(n.Heading, heading, wind.TWD)
					if maneuver == "tack" {
						penaltySec = cfg.TackPenaltyMinutes * 60.0
						newTackCount++
					} else if maneuver == "gybe" {
						penaltySec = cfg.GybePenaltyMinutes * 60.0
						newGybeCount++
					}
				}

				// Effective sailing time deducting maneuver penalty
				effTimeSec := math.Max(0.1, timeStepSec-penaltySec)
				distTraveledMeters := (boatSpeedKts * weather.KnotsToMS) * effTimeSec
				nextPoint := geo.DestinationPoint(n.Point, distTraveledMeters, heading)

				// Land collision check
				if landMask != nil && landMask.SegmentIntersectsLand(n.Point, nextPoint, 4) {
					continue
				}

				distToDest := geo.DistanceMeters(nextPoint, dest)

				candNode := &Node{
					Point:            nextPoint,
					Time:             currentTime,
					Parent:           n,
					DistanceToDest:   distToDest,
					Heading:          heading,
					TWS:              wind.TWS,
					TWD:              wind.TWD,
					TWA:              twa,
					BoatSpeed:        boatSpeedKts,
					DistanceTraveled: n.DistanceTraveled + (distTraveledMeters * geo.MetersToNM),
					Maneuver:         maneuver,
					TackCount:        newTackCount,
					GybeCount:        newGybeCount,
				}
				candidates = append(candidates, candNode)
			}
		}

		if len(candidates) == 0 {
			break
		}

		// 4. Prune candidates to retain advancing convex frontier
		frontier = pruneFrontier(candidates, start, dest, cfg.NumAngularBins)
	}

	// 5. Select terminal node and backtrack path
	terminalNode := bestArrivalNode
	destReached := true
	if terminalNode == nil {
		terminalNode = closestNode
		destReached = false
	}

	waypoints := backtrackRoute(terminalNode)

	// Calculate summary stats
	totalDurHours := terminalNode.Time.Sub(startTime).Hours()
	totDistNM := terminalNode.DistanceTraveled
	avgSpd := 0.0
	if totalDurHours > 0.01 {
		avgSpd = totDistNM / totalDurHours
	}

	maxWind := 0.0
	for _, wp := range waypoints {
		if wp.TWSKts > maxWind {
			maxWind = wp.TWSKts
		}
	}

	return &RouteResult{
		BoatName:           polarTable.BoatName,
		StartPoint:         start,
		DestPoint:          dest,
		StartTime:          startTime,
		ArrivalTime:        terminalNode.Time,
		TotalDurationHours: totalDurHours,
		TotalDistanceNM:    totDistNM,
		DirectDistanceNM:   directDistNM,
		AverageSpeedKts:    avgSpd,
		MaxWindEncountered: maxWind,
		TotalTacks:         terminalNode.TackCount,
		TotalGybes:         terminalNode.GybeCount,
		TackPenaltyMinutes: cfg.TackPenaltyMinutes,
		GybePenaltyMinutes: cfg.GybePenaltyMinutes,
		Waypoints:          waypoints,
		Isochrones:         isochrones,
		DestinationReached: destReached,
	}, nil
}

// pruneFrontier partitions candidate nodes into angular sectors relative to the start-destination axis
// and retains only the node with minimum distance to destination in each sector.
func pruneFrontier(candidates []*Node, start, dest geo.Point, numBins int) []*Node {
	if len(candidates) <= numBins {
		return candidates
	}

	refBearing := geo.InitialBearing(start, dest)
	binMap := make(map[int]*Node, numBins)

	for _, cand := range candidates {
		bearingFromStart := geo.InitialBearing(start, cand.Point)
		relAngle := geo.NormalizeAngle360(bearingFromStart - refBearing)
		if relAngle > 180.0 {
			relAngle -= 360.0
		}

		binIdx := int(math.Floor((relAngle + 120.0) / 240.0 * float64(numBins)))
		if binIdx < 0 {
			binIdx = 0
		}
		if binIdx >= numBins {
			binIdx = numBins - 1
		}

		existing, found := binMap[binIdx]
		if !found || cand.DistanceToDest < existing.DistanceToDest {
			binMap[binIdx] = cand
		}
	}

	res := make([]*Node, 0, len(binMap))
	for _, node := range binMap {
		res = append(res, node)
	}

	sort.Slice(res, func(i, j int) bool {
		bI := geo.InitialBearing(start, res[i].Point)
		bJ := geo.InitialBearing(start, res[j].Point)
		return bI < bJ
	})

	return res
}

func backtrackRoute(terminal *Node) []Waypoint {
	nodes := make([]*Node, 0)
	curr := terminal
	for curr != nil {
		nodes = append(nodes, curr)
		curr = curr.Parent
	}

	// Reverse to chronological order
	for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}

	waypoints := make([]Waypoint, len(nodes))
	for i, n := range nodes {
		estimatedHeel := estimateHeelAngle(n.TWS, n.TWA)
		waypoints[i] = Waypoint{
			Lat:              n.Point.Lat,
			Lon:              n.Point.Lon,
			Time:             n.Time,
			HeadingDeg:       n.Heading,
			BoatSpeedKts:     n.BoatSpeed,
			TWSKts:           n.TWS,
			TWDDeg:           n.TWD,
			TWADeg:           n.TWA,
			DistanceNM:       n.DistanceTraveled,
			DistanceToDestNM: n.DistanceToDest * geo.MetersToNM,
			EstimatedHeelDeg: estimatedHeel,
			Maneuver:         n.Maneuver,
		}
	}

	return waypoints
}

func estimateHeelAngle(tws, twa float64) float64 {
	if twa < 20.0 {
		return 3.0
	}
	if twa <= 50.0 {
		return math.Min(tws*1.3, 24.0)
	}
	if twa <= 110.0 {
		return math.Min(tws*1.1, 20.0)
	}
	return math.Min(tws*0.35, 8.0)
}
