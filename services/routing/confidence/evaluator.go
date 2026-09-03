package confidence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

// Evaluator evaluates route confidence using both Strategy A (precomputed stats) and Strategy B (4D member simulation).
type Evaluator struct {
	httpClient *http.Client
	meteoURL   string
	landMask   *landmask.LandMask
}

// NewEvaluator creates a new confidence evaluator.
func NewEvaluator(meteoURL string, client *http.Client, lm *landmask.LandMask) *Evaluator {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	if meteoURL == "" {
		meteoURL = "https://routing.jaclar.net"
	}
	return &Evaluator{
		httpClient: client,
		meteoURL:   strings.TrimRight(strings.TrimSpace(meteoURL), "/"),
		landMask:   lm,
	}
}

// OpenMeteoMultiPointResponse models the JSON returned by multi-point Open-Meteo queries.
type OpenMeteoMultiPointResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Hourly    struct {
		Time               []string  `json:"time"`
		WindSpeed10m       []float64 `json:"wind_speed_10m"`
		WindSpeed10mStd    []float64 `json:"wind_speed_10m_std"`
		WindSpeed10mP10    []float64 `json:"wind_speed_10m_p10"`
		WindSpeed10mP90    []float64 `json:"wind_speed_10m_p90"`
		WindDirection10m   []float64 `json:"wind_direction_10m"`
		ProbWindGE25kt     []float64 `json:"prob_wind_ge_25kt"`
		ProbWindGE34kt     []float64 `json:"prob_wind_ge_34kt"`
	} `json:"hourly"`
}

// EvaluateRoute computes overall and per-waypoint confidence scores and comparison statistics.
func (e *Evaluator) EvaluateRoute(ctx context.Context, route *isochrone.RouteResult, polarTable *polar.PolarTable, modelID string) (*RouteConfidence, error) {
	if route == nil || len(route.Waypoints) == 0 {
		return nil, fmt.Errorf("empty route")
	}

	wps := route.Waypoints
	nWps := len(wps)

	// Subsample waypoints for remote HTTP query to bound memory and network overhead
	sampleStep := 1
	if nWps > 30 {
		sampleStep = int(math.Ceil(float64(nWps) / 30.0))
	}
	var sampleIndices []int
	var lats, lons []string
	for i := 0; i < nWps; i += sampleStep {
		sampleIndices = append(sampleIndices, i)
		lats = append(lats, fmt.Sprintf("%.3f", wps[i].Lat))
		lons = append(lons, fmt.Sprintf("%.3f", wps[i].Lon))
	}
	if len(sampleIndices) > 0 && sampleIndices[len(sampleIndices)-1] != nWps-1 {
		sampleIndices = append(sampleIndices, nWps-1)
		lats = append(lats, fmt.Sprintf("%.3f", wps[nWps-1].Lat))
		lons = append(lons, fmt.Sprintf("%.3f", wps[nWps-1].Lon))
	}
	// Determine ensemble model and member count
	ensModel := "gefs_0p50"
	numMembers := 31
	canonicalModel := strings.ToLower(modelID)
	switch {
	case strings.Contains(canonicalModel, "gefs") || strings.Contains(canonicalModel, "gfs"):
		ensModel = "gefs_0p50"
		numMembers = 31
	case strings.Contains(canonicalModel, "ifs_ens") || strings.Contains(canonicalModel, "ecmwf_ens") || strings.Contains(canonicalModel, "ifs") || strings.Contains(canonicalModel, "ecmwf"):
		ensModel = "ifs_ens_0p25"
		numMembers = 50
	case strings.Contains(canonicalModel, "icon_eps") || strings.Contains(canonicalModel, "icon"):
		ensModel = "icon_eps_global"
		numMembers = 40
	default:
		ensModel = "gefs_0p50"
		numMembers = 31
	}

	latsStr := strings.Join(lats, ",")
	lonsStr := strings.Join(lons, ",")

	// 1. Fetch Strategy A statistical fields from Meteo API
	statURL := fmt.Sprintf("%s/v1/forecast?models=%s&latitude=%s&longitude=%s&hourly=wind_speed_10m,wind_speed_10m_std,wind_speed_10m_p10,wind_speed_10m_p90,wind_direction_10m,prob_wind_ge_25kt,prob_wind_ge_34kt&wind_speed_unit=kn",
		e.meteoURL, ensModel, latsStr, lonsStr)

	pointsData, err := e.fetchMeteoPoints(ctx, statURL)
	hasLiveData := err == nil && len(pointsData) == len(sampleIndices)

	// Map each waypoint index to its closest sampled point index
	wpToSampleIdx := make([]int, nWps)
	for i := 0; i < nWps; i++ {
		bestSample := 0
		bestDist := 1e9
		for sIdx, origIdx := range sampleIndices {
			d := math.Abs(float64(origIdx - i))
			if d < bestDist {
				bestDist = d
				bestSample = sIdx
			}
		}
		wpToSampleIdx[i] = bestSample
	}

	// 2. Evaluate waypoints
	waypointConf := make([]WaypointConfidence, nWps)
	startTime := route.StartTime

	var totalWeightedScoreA float64
	var totalDist float64

	// Strategy A theoretical error propagation structures
	var totalNominalDuration float64
	var sumLegDurationStd float64
	var sumSquareLegDurationStd float64

	leftBoundary := make([]geo.Point, nWps)
	rightBoundary := make([]geo.Point, nWps)
	var maxLateralNM float64

	for i, wp := range wps {
		horizonHours := wp.Time.Sub(startTime).Hours()
		if horizonHours < 0 {
			horizonHours = 0
		}

		var meanSpd, stdSpd, p10Spd, p90Spd, dirSpread, prob25, prob34 float64

		if hasLiveData {
			pt := pointsData[wpToSampleIdx[i]]
			timeIdx := findClosestTimeIndex(pt.Hourly.Time, wp.Time)

			if len(pt.Hourly.WindSpeed10m) > timeIdx {
				meanSpd = pt.Hourly.WindSpeed10m[timeIdx]
			}
			if len(pt.Hourly.WindSpeed10mStd) > timeIdx {
				stdSpd = pt.Hourly.WindSpeed10mStd[timeIdx]
			}
			if len(pt.Hourly.WindSpeed10mP10) > timeIdx {
				p10Spd = pt.Hourly.WindSpeed10mP10[timeIdx]
			}
			if len(pt.Hourly.WindSpeed10mP90) > timeIdx {
				p90Spd = pt.Hourly.WindSpeed10mP90[timeIdx]
			}
			if len(pt.Hourly.ProbWindGE25kt) > timeIdx {
				prob25 = pt.Hourly.ProbWindGE25kt[timeIdx] / 100.0
			}
			if len(pt.Hourly.ProbWindGE34kt) > timeIdx {
				prob34 = pt.Hourly.ProbWindGE34kt[timeIdx] / 100.0
			}
		}

		// Fallback statistical estimates
		if meanSpd <= 0 {
			meanSpd = math.Max(wp.TWSKts, 8.0)
		}
		if stdSpd <= 0 {
			cvEst := 0.12 + 0.003*horizonHours
			if cvEst > 0.45 {
				cvEst = 0.45
			}
			stdSpd = meanSpd * cvEst
			p10Spd = math.Max(0, meanSpd-1.28*stdSpd)
			p90Spd = meanSpd + 1.28*stdSpd
		}
		if p10Spd <= 0 && meanSpd > 0 {
			p10Spd = math.Max(0, meanSpd-1.28*stdSpd)
		}
		if p90Spd <= 0 && meanSpd > 0 {
			p90Spd = meanSpd + 1.28*stdSpd
		}

		// Directional spread estimate (increases smoothly from 5° to 35° with lead time)
		dirSpread = math.Min(45.0, 5.0+0.18*horizonHours)

		// Strategy A Scoring Formula
		cv := stdSpd / math.Max(meanSpd, 5.0)
		fSpeed := math.Exp(-2.0 * cv)
		fDir := math.Exp(-dirSpread / 40.0)
		fGale := 1.0 - 0.75*prob34
		fHorizon := math.Exp(-horizonHours / 360.0)

		scoreA := clamp(100.0*fSpeed*fDir*fGale*fHorizon, 5.0, 99.0)

		var legDist float64
		if i > 0 {
			legDist = wp.DistanceNM - wps[i-1].DistanceNM
			if legDist <= 0 {
				legDist = geo.DistanceNM(geo.Point{Lat: wps[i-1].Lat, Lon: wps[i-1].Lon}, geo.Point{Lat: wp.Lat, Lon: wp.Lon})
			}
		}

		// Spatial Uncertainty Envelope Corridor:
		// Plume half-width widens with lead time and directional spread.
		// As in real ocean navigation, the skipper navigates to the expected endpoint,
		// so the envelope is anchored at departure and smoothly collapses back over the ideal route at destination.
		sProgress := float64(i) / math.Max(1.0, float64(nWps-1))
		taperStart := math.Min(1.0, sProgress/0.08)
		taperEnd := math.Min(1.0, (1.0-sProgress)/0.25)
		// Smooth Hermite cubic interpolation for convergence toward arrival waypoint
		taperEnd = taperEnd * taperEnd * (3.0 - 2.0*taperEnd)
		convergenceFactor := taperStart * taperEnd

		rawSpreadNM := 0.5
		if i > 0 {
			expansionFactor := 1.0 + 0.008*horizonHours
			spreadFactor := dirSpread / 20.0
			rawSpreadNM = math.Min(60.0, math.Max(1.0, (wp.DistanceNM+5.0)*0.045*spreadFactor*expansionFactor))
		}
		lateralSpreadNM := rawSpreadNM * convergenceFactor
		if i == 0 || i == nWps-1 {
			lateralSpreadNM = 0.0
		}

		if lateralSpreadNM > maxLateralNM {
			maxLateralNM = lateralSpreadNM
		}

		// Calculate perpendicular boundary points for the corridor
		wpPt := geo.Point{Lat: wp.Lat, Lon: wp.Lon}
		leftPt := wpPt
		rightPt := wpPt

		var heading float64
		if i < nWps-1 {
			heading = geo.InitialBearing(wpPt, geo.Point{Lat: wps[i+1].Lat, Lon: wps[i+1].Lon})
		} else if i > 0 {
			heading = geo.InitialBearing(geo.Point{Lat: wps[i-1].Lat, Lon: wps[i-1].Lon}, wpPt)
		} else {
			heading = wp.HeadingDeg
		}

		thetaLeft := geo.NormalizeAngle360(heading - 90.0)
		thetaRight := geo.NormalizeAngle360(heading + 90.0)
		distMeters := lateralSpreadNM * geo.NMToMeters

		if e.landMask != nil && distMeters > 50.0 {
			leftPt = e.findNavigableEnvelopePoint(wpPt, distMeters, thetaLeft)
			rightPt = e.findNavigableEnvelopePoint(wpPt, distMeters, thetaRight)
		} else {
			leftPt = geo.DestinationPoint(wpPt, distMeters, thetaLeft)
			rightPt = geo.DestinationPoint(wpPt, distMeters, thetaRight)
		}

		leftBoundary[i] = leftPt
		rightBoundary[i] = rightPt

		waypointConf[i] = WaypointConfidence{
			Index:                 i,
			Time:                  wp.Time,
			Score:                 round1(scoreA),
			ScoreStrategyA:        round1(scoreA),
			LateralUncertaintyNM:  round1(lateralSpreadNM),
			WindSpeedMean:         round1(meanSpd),
			WindSpeedStd:          round1(stdSpd),
			WindSpeedP10:          round1(p10Spd),
			WindSpeedP90:          round1(p90Spd),
			WindDirSpreadDeg:      round1(dirSpread),
			GaleProbability:       round2(prob34),
			StrongWindProbability: round2(prob25),
		}

		// Strategy A theoretical error propagation
		var vMean, vP10, vP90 float64
		if polarTable != nil {
			vMean = polarTable.InterpolateSpeed(meanSpd, wp.TWADeg)
			vP10 = polarTable.InterpolateSpeed(math.Max(1.0, meanSpd-stdSpd), wp.TWADeg)
			vP90 = polarTable.InterpolateSpeed(meanSpd+stdSpd, wp.TWADeg)
		} else {
			vMean = wp.BoatSpeedKts
			vP10 = wp.BoatSpeedKts * (1.0 - 0.4*stdSpd/math.Max(meanSpd, 1.0))
			vP90 = wp.BoatSpeedKts * (1.0 + 0.4*stdSpd/math.Max(meanSpd, 1.0))
		}
		if vMean < 1.0 {
			vMean = math.Max(wp.BoatSpeedKts, 1.0)
		}
		vStd := math.Abs(vP90-vP10) / 2.0
		if legDist > 0 && vMean > 0 {
			nominalLegDuration := legDist / vMean
			legDurationStd := nominalLegDuration * (vStd / vMean)
			totalNominalDuration += nominalLegDuration
			sumLegDurationStd += legDurationStd
			sumSquareLegDurationStd += legDurationStd * legDurationStd
		}

		weight := legDist
		if weight <= 0 {
			weight = 1.0
		}
		totalWeightedScoreA += scoreA * weight
		totalDist += weight
	}

	// 3. Compute Overall Score
	overallScoreA := totalWeightedScoreA / math.Max(totalDist, 1.0)

	// Strategy A Theoretical Arrival Metrics
	nominalDurationA := route.TotalDurationHours
	if totalNominalDuration > 0 {
		nominalDurationA = totalNominalDuration
	}
	stdDurationA := 0.50*sumLegDurationStd + 0.50*math.Sqrt(sumSquareLegDurationStd)
	if stdDurationA <= 0.1 {
		stdDurationA = nominalDurationA * 0.04
	}
	minDurationA := math.Max(nominalDurationA*0.7, nominalDurationA-1.28*stdDurationA)
	maxDurationA := nominalDurationA + 1.28*stdDurationA
	iqrDurationA := 1.349 * stdDurationA

	statComparison := &StatisticalComparison{
		MeanDurationHours: round1(nominalDurationA),
		StdDurationHours:  round1(stdDurationA),
		MinDurationHours:  round1(minDurationA),
		MaxDurationHours:  round1(maxDurationA),
		IQRDurationHours:  round1(iqrDurationA),
	}

	// Assemble closed envelope polygon [LeftStart -> LeftEnd -> RightEnd -> RightStart]
	polygon := make([]geo.Point, 0, nWps*2)
	for i := 0; i < nWps; i++ {
		polygon = append(polygon, leftBoundary[i])
	}
	for i := nWps - 1; i >= 0; i-- {
		polygon = append(polygon, rightBoundary[i])
	}

	return &RouteConfidence{
		OverallScore:          round1(overallScoreA),
		Category:              CategorizeConfidence(overallScoreA),
		ScoreStrategyA:        round1(overallScoreA),
		ModelID:               modelID,
		NumMembers:            numMembers,
		Waypoints:             waypointConf,
		StatisticalComparison: statComparison,
		EnsembleComparison:    nil, // Pure statistical evaluation; no fake member simulations
		UncertaintyEnvelope: &UncertaintyEnvelope{
			LeftBoundary:    leftBoundary,
			RightBoundary:   rightBoundary,
			Polygon:         polygon,
			ConfidenceLevel: "80% (P10 - P90) Corridor",
			MaxLateralNM:    round1(maxLateralNM),
		},
	}, nil
}

func (e *Evaluator) findNavigableEnvelopePoint(center geo.Point, distMeters float64, bearing float64) geo.Point {
	if e.landMask == nil {
		return geo.DestinationPoint(center, distMeters, bearing)
	}
	for _, scale := range []float64{1.0, 0.85, 0.70, 0.55, 0.40, 0.25, 0.10, 0.0} {
		candidate := geo.DestinationPoint(center, distMeters*scale, bearing)
		if !e.landMask.IsLand(candidate) && !e.landMask.SegmentIntersectsLand(center, candidate, 3) {
			return candidate
		}
	}
	return center
}

func (e *Evaluator) fetchMeteoPoints(ctx context.Context, url string) ([]OpenMeteoMultiPointResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SailboatWeatherRouter/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meteo query failed with HTTP status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var points []OpenMeteoMultiPointResponse
	if strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "[") {
		if err := json.Unmarshal(bodyBytes, &points); err != nil {
			return nil, err
		}
	} else {
		var single OpenMeteoMultiPointResponse
		if err := json.Unmarshal(bodyBytes, &single); err != nil {
			return nil, err
		}
		points = []OpenMeteoMultiPointResponse{single}
	}

	return points, nil
}

func findClosestTimeIndex(times []string, target time.Time) int {
	if len(times) == 0 {
		return 0
	}
	targetSec := target.Unix()
	bestIdx := 0
	bestDiff := int64(1e12)

	for i, tStr := range times {
		t, err := time.Parse("2006-01-02T15:04", tStr)
		if err != nil {
			continue
		}
		diff := absInt64(t.Unix() - targetSec)
		if diff < bestDiff {
			bestDiff = diff
			bestIdx = i
		}
	}
	return bestIdx
}

func meanAndStd(vals []float64) (mean, std float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / float64(len(vals))
	var sumSq float64
	for _, v := range vals {
		d := v - mean
		sumSq += d * d
	}
	std = math.Sqrt(sumSq / float64(len(vals)))
	return mean, std
}

func percentile(sortedVals []float64, p float64) float64 {
	if len(sortedVals) == 0 {
		return 0
	}
	if p <= 0 {
		return sortedVals[0]
	}
	if p >= 1.0 {
		return sortedVals[len(sortedVals)-1]
	}
	idx := p * float64(len(sortedVals)-1)
	low := int(math.Floor(idx))
	high := int(math.Ceil(idx))
	frac := idx - float64(low)
	return sortedVals[low]*(1.0-frac) + sortedVals[high]*frac
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func round1(val float64) float64 {
	return math.Round(val*10.0) / 10.0
}

func round2(val float64) float64 {
	return math.Round(val*100.0) / 100.0
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// SolveMultiIsochroneEnsemble executes independent isochrone wavefront pathfinding solves for all N ensemble members.
func (e *Evaluator) SolveMultiIsochroneEnsemble(
	ctx context.Context,
	start geo.Point,
	dest geo.Point,
	startTime time.Time,
	polarTable *polar.PolarTable,
	baseGrid *weather.WeatherGrid,
	numMembers int,
	cfg isochrone.RouterConfig,
) ([]MemberOutcome, error) {
	if baseGrid == nil || numMembers <= 0 {
		return nil, fmt.Errorf("invalid base grid or member count")
	}

	memberGrids := weather.GenerateEnsembleMemberGrids(baseGrid, numMembers)
	if len(memberGrids) == 0 {
		return nil, fmt.Errorf("failed to generate ensemble member grids")
	}

	outcomes := make([]MemberOutcome, numMembers)
	type memberJob struct {
		mID  int
		grid *weather.WeatherGrid
	}
	type memberResult struct {
		mID     int
		outcome MemberOutcome
		err     error
	}

	jobChan := make(chan memberJob, numMembers)
	resChan := make(chan memberResult, numMembers)

	numWorkers := 8
	if numWorkers > numMembers {
		numWorkers = numMembers
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				select {
				case <-ctx.Done():
					resChan <- memberResult{mID: job.mID, err: ctx.Err()}
					return
				default:
				}

				mEngine := weather.NewMemberWeatherEngine(job.mID, job.grid)
				mRoute, err := isochrone.CalculateOptimalRoute(
					start,
					dest,
					startTime,
					polarTable,
					mEngine,
					e.landMask,
					cfg,
				)
				if err != nil {
					resChan <- memberResult{mID: job.mID, err: err}
					continue
				}

				traj := make([]geo.Point, len(mRoute.Waypoints))
				for idx, wp := range mRoute.Waypoints {
					traj[idx] = geo.Point{Lat: wp.Lat, Lon: wp.Lon}
				}

				outcome := MemberOutcome{
					MemberID:           job.mID,
					TotalDurationHours: round1(mRoute.TotalDurationHours),
					TotalDistanceNM:    round1(mRoute.TotalDistanceNM),
					AverageSpeedKts:    round1(mRoute.AverageSpeedKts),
					MaxWindKts:         round1(mRoute.MaxWindEncountered),
					TotalTacks:         mRoute.TotalTacks,
					DestinationReached: mRoute.DestinationReached,
					Waypoints:          mRoute.Waypoints,
					Trajectory:         traj,
				}
				resChan <- memberResult{mID: job.mID, outcome: outcome, err: nil}
			}
		}()
	}

	for m := 0; m < numMembers; m++ {
		jobChan <- memberJob{mID: m, grid: memberGrids[m]}
	}
	close(jobChan)

	wg.Wait()
	close(resChan)

	validCount := 0
	for res := range resChan {
		if res.err == nil && res.outcome.TotalDurationHours > 0 {
			outcomes[res.mID] = res.outcome
			validCount++
		}
	}

	if validCount == 0 {
		return nil, fmt.Errorf("all member isochrone solves failed")
	}

	var cleanOutcomes []MemberOutcome
	for _, o := range outcomes {
		if o.TotalDurationHours > 0 {
			cleanOutcomes = append(cleanOutcomes, o)
		}
	}

	return cleanOutcomes, nil
}

// EvaluateRouteMultiIsochrone evaluates confidence by executing full multi-isochrone solves across all N ensemble members.
func (e *Evaluator) EvaluateRouteMultiIsochrone(
	ctx context.Context,
	primaryRoute *isochrone.RouteResult,
	start geo.Point,
	dest geo.Point,
	polarTable *polar.PolarTable,
	baseGrid *weather.WeatherGrid,
	requestedModel string,
	cfg isochrone.RouterConfig,
) (*RouteConfidence, error) {
	if primaryRoute == nil || len(primaryRoute.Waypoints) == 0 {
		return nil, fmt.Errorf("cannot evaluate empty route")
	}

	canonicalModel := weather.NormalizeModelID(requestedModel)
	var numMembers int
	switch canonicalModel {
	case weather.ModelIFSEns025:
		numMembers = 50
	case weather.ModelICONEPS:
		numMembers = 40
	case weather.ModelGEFS050, weather.ModelGFS025, weather.ModelIFS025, weather.ModelICONGlobal:
		numMembers = 31
	default:
		numMembers = 31
	}

	// Solve all N member routes independently
	memberOutcomes, err := e.SolveMultiIsochroneEnsemble(ctx, start, dest, primaryRoute.StartTime, polarTable, baseGrid, numMembers, cfg)
	if err != nil || len(memberOutcomes) == 0 {
		// Fall back to corridor evaluation if full multi-solve encountered errors
		return e.EvaluateRoute(ctx, primaryRoute, polarTable, requestedModel)
	}

	directDistNM := geo.DistanceMeters(start, dest) * geo.MetersToNM
	minArrivedDistNM := directDistNM * 0.50

	// Separate fully arrived members from incomplete/stalled wavefronts
	var arrivedDurations []float64
	var arrivedMembers []MemberOutcome
	fastestID, slowestID := 0, 0
	fastestTime, slowestTime := 1e9, -1.0

	for _, m := range memberOutcomes {
		// A member is considered arrived if destination was reached or sailed >= 50% of direct distance
		if m.DestinationReached || m.TotalDistanceNM >= minArrivedDistNM {
			arrivedDurations = append(arrivedDurations, m.TotalDurationHours)
			arrivedMembers = append(arrivedMembers, m)
			if m.TotalDurationHours < fastestTime {
				fastestTime = m.TotalDurationHours
				fastestID = m.MemberID
			}
			if m.TotalDurationHours > slowestTime {
				slowestTime = m.TotalDurationHours
				slowestID = m.MemberID
			}
		}
	}

	// If no member arrived, use all outcomes
	if len(arrivedDurations) == 0 {
		for _, m := range memberOutcomes {
			arrivedDurations = append(arrivedDurations, m.TotalDurationHours)
		}
	}

	meanDur, stdDur := meanAndStd(arrivedDurations)
	sort.Float64s(arrivedDurations)
	minDur := arrivedDurations[0]
	maxDur := arrivedDurations[len(arrivedDurations)-1]
	p10Dur := percentile(arrivedDurations, 0.10)
	p90Dur := percentile(arrivedDurations, 0.90)
	iqrDur := percentile(arrivedDurations, 0.75) - percentile(arrivedDurations, 0.25)

	// Strategy B Score derived from duration spread ratio with stall penalty
	durSpreadRatio := stdDur / math.Max(meanDur, 1.0)
	scoreB := clamp(100.0*math.Exp(-2.2*durSpreadRatio), 15.0, 98.0)
	arrivalRatio := float64(len(arrivedDurations)) / float64(len(memberOutcomes))
	scoreB = clamp(scoreB*arrivalRatio, 10.0, 98.0)

	// Run standard Strategy A statistical evaluation for theoretical comparison and waypoint scores
	baseConf, _ := e.EvaluateRoute(ctx, primaryRoute, polarTable, requestedModel)

	var scoreA float64 = 75.0
	var waypointsConf []WaypointConfidence
	var statComp *StatisticalComparison
	meanA := primaryRoute.TotalDurationHours
	stdA := meanA * 0.05
	if baseConf != nil {
		scoreA = baseConf.ScoreStrategyA
		waypointsConf = baseConf.Waypoints
		statComp = baseConf.StatisticalComparison
		if statComp != nil {
			meanA = statComp.MeanDurationHours
			stdA = statComp.StdDurationHours
		}
	}

	overallScore := round1((scoreA*0.5 + scoreB*0.5))

	// Physical Cross-Model Agreement:
	// 1. Relative difference between Strategy A mean duration and Strategy B mean duration
	nomDur := math.Max(primaryRoute.TotalDurationHours, 1.0)
	durRelDiff := math.Abs(meanDur-meanA) / nomDur
	durAgreement := math.Max(0.0, 100.0*(1.0-3.5*durRelDiff))

	// 2. Relative difference between Strategy A and Strategy B uncertainty spreads
	spreadRatio := math.Min(stdA, stdDur) / math.Max(math.Max(stdA, stdDur), 0.1)
	spreadAgreement := 100.0 * spreadRatio

	// Combined Physical Agreement Index
	agreementScore := round1(clamp(0.70*durAgreement+0.30*spreadAgreement, 10.0, 99.0))

	var env *UncertaintyEnvelope
	if baseConf != nil {
		env = baseConf.UncertaintyEnvelope
	}

	return &RouteConfidence{
		OverallScore:          overallScore,
		Category:              CategorizeConfidence(overallScore),
		ScoreStrategyA:        round1(scoreA),
		ScoreStrategyB:        round1(scoreB),
		AgreementScore:        agreementScore,
		NumMembers:            len(memberOutcomes),
		Waypoints:             waypointsConf,
		StatisticalComparison: statComp,
		EnsembleComparison: &EnsembleComparison{
			MeanDurationHours: round1(meanDur),
			StdDurationHours:  round1(stdDur),
			MinDurationHours:  round1(minDur),
			MaxDurationHours:  round1(maxDur),
			IQRDurationHours:  round1(iqrDur),
			P10DurationHours:  round1(p10Dur),
			P90DurationHours:  round1(p90Dur),
			FastestMemberID:   fastestID,
			SlowestMemberID:   slowestID,
			MemberCount:       len(memberOutcomes),
			Members:           memberOutcomes,
		},
		UncertaintyEnvelope: env,
	}, nil
}
