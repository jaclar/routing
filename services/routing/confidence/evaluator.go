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
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/polar"
)

// Evaluator evaluates route confidence using both Strategy A (precomputed stats) and Strategy B (4D member simulation).
type Evaluator struct {
	httpClient *http.Client
	meteoURL   string
}

// NewEvaluator creates a new confidence evaluator.
func NewEvaluator(meteoURL string, client *http.Client) *Evaluator {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	if meteoURL == "" {
		meteoURL = "https://routing.jaclar.net"
	}
	return &Evaluator{
		httpClient: client,
		meteoURL:   strings.TrimRight(strings.TrimSpace(meteoURL), "/"),
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
	var totalWeightedScoreB float64
	var totalDist float64

	// Strategy A theoretical error propagation structures
	var totalNominalDuration float64
	var sumLegDurationStd float64
	var sumSquareLegDurationStd float64

	// Strategy B simulation structures
	memberDurations := make([]float64, numMembers)
	memberTotalDist := make([]float64, numMembers)
	memberMaxWind := make([]float64, numMembers)
	memberTrajectories := make([][]geo.Point, numMembers)
	for m := 0; m < numMembers; m++ {
		memberTrajectories[m] = make([]geo.Point, 0, nWps)
	}

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

		// A. Strategy A Scoring Formula
		cv := stdSpd / math.Max(meanSpd, 5.0)
		fSpeed := math.Exp(-2.0 * cv)
		fDir := math.Exp(-dirSpread / 40.0)
		fGale := 1.0 - 0.75*prob34
		fHorizon := math.Exp(-horizonHours / 360.0)

		scoreA := clamp(100.0*fSpeed*fDir*fGale*fHorizon, 5.0, 99.0)

		// B. Strategy B Member Simulation across N members
		memberSpeeds := make([]float64, numMembers)
		var legDist float64
		if i > 0 {
			legDist = wp.DistanceNM - wps[i-1].DistanceNM
			if legDist <= 0 {
				legDist = geo.DistanceNM(geo.Point{Lat: wps[i-1].Lat, Lon: wps[i-1].Lon}, geo.Point{Lat: wp.Lat, Lon: wp.Lon})
			}
		}

		// Parabolic ensemble plume envelope: expands mid-route and reconverges at destination
		sProgress := float64(i) / math.Max(1.0, float64(nWps-1))
		envelope := 4.0 * sProgress * (1.0 - sProgress)
		lateralSpreadNM := (dirSpread / 25.0) * math.Min(45.0, math.Max(10.0, route.TotalDistanceNM*0.06))

		for m := 0; m < numMembers; m++ {
			// Generate realistic member perturbation around mean
			var mPerturb float64
			if numMembers > 1 {
				mPerturb = (float64(m) - float64(numMembers-1)/2.0) / (float64(numMembers-1) / 2.0)
			}
			// 1. Scale wind speed across the [-1.0σ, +1.0σ] ensemble dispersion interval.
			// mPerturb ranges from -1.0 to +1.0; multiplying by stdSpd * 1.0 maps members
			// directly to the true 1-sigma credible interval without artificial amplification.
			mWindSpeed := meanSpd + mPerturb*stdSpd*1.0

			// 2. Physical floor (1.0 kt): Prevents polar speed collapsing to 0.0 kts,
			// which would cause division-by-zero asymptotes (Δt = Δd / SOG -> ∞).
			// Open-ocean surface boundary layer turbulence ensures baseline drift >= 1.0 kt.
			if mWindSpeed < 1.0 {
				mWindSpeed = 1.0
			}
			mDir := wp.TWDDeg + mPerturb*dirSpread*0.6

			// Compute TWA for this member given the planned course heading
			mTWA := math.Abs(wp.HeadingDeg - mDir)
			for mTWA > 180 {
				mTWA = 360 - mTWA
			}

			// In real sailing, if an adverse wind shift pushes the boat into the no-go dead zone (TWA < 38°),
			// the helmsman adjusts heading to maintain optimal close-hauled VMG (TWA_opt ~ 38°-42°),
			// rather than stalling in the no-go zone. Effective speed along the waypoint course is reduced by cos(heading_adjustment).
			effectiveTWA := mTWA
			headingCorrectionRad := 0.0
			const minSailableTWA = 38.0

			if wp.TWADeg >= minSailableTWA && effectiveTWA < minSailableTWA {
				// Wind headed the boat into no-go zone: helmsman heads off to minimum sailable angle
				neededAdjustmentDeg := minSailableTWA - effectiveTWA
				effectiveTWA = minSailableTWA
				headingCorrectionRad = (neededAdjustmentDeg * math.Pi) / 180.0
			}

			// Polar lookup with effective sailable angle
			var mSOG float64
			if polarTable != nil {
				mSOG = polarTable.InterpolateSpeed(mWindSpeed, effectiveTWA)
			} else {
				mSOG = wp.BoatSpeedKts * (1.0 + 0.4*(mWindSpeed-meanSpd)/math.Max(meanSpd, 1.0))
			}

			// Velocity made good along the waypoint track leg
			effectiveSOG := mSOG * math.Cos(headingCorrectionRad)
			if effectiveSOG < 1.5 {
				effectiveSOG = 1.5
			}

			memberSpeeds[m] = effectiveSOG

			if legDist > 0 {
				memberDurations[m] += legDist / effectiveSOG
				memberTotalDist[m] += legDist
			}
			if mWindSpeed > memberMaxWind[m] {
				memberMaxWind[m] = mWindSpeed
			}

			// Spatial track point
			if i == 0 || i == nWps-1 || envelope <= 0.001 {
				memberTrajectories[m] = append(memberTrajectories[m], geo.Point{Lat: wp.Lat, Lon: wp.Lon})
			} else {
				offsetDistMeters := mPerturb * lateralSpreadNM * envelope * geo.NMToMeters
				offsetBearing := geo.NormalizeAngle360(wp.HeadingDeg + 90.0)
				mPt := geo.DestinationPoint(geo.Point{Lat: wp.Lat, Lon: wp.Lon}, offsetDistMeters, offsetBearing)
				memberTrajectories[m] = append(memberTrajectories[m], mPt)
			}
		}

		// Calculate member speed distribution at this waypoint
		mMeanSOG, mStdSOG := meanAndStd(memberSpeeds)
		sort.Float64s(memberSpeeds)
		mP10SOG := percentile(memberSpeeds, 0.10)
		mP90SOG := percentile(memberSpeeds, 0.90)

		// Strategy B Waypoint Score
		speedCV_B := mStdSOG / math.Max(mMeanSOG, 2.0)
		scoreB := clamp(100.0*math.Exp(-2.5*speedCV_B)*math.Exp(-dirSpread/45.0)*fHorizon, 5.0, 99.0)

		// Combined waypoint score (50/50 blend of Strategy A & B)
		combinedScore := round1((scoreA + scoreB) / 2.0)

		waypointConf[i] = WaypointConfidence{
			Index:                 i,
			Time:                  wp.Time,
			Score:                 combinedScore,
			ScoreStrategyA:        round1(scoreA),
			ScoreStrategyB:        round1(scoreB),
			WindSpeedMean:         round1(meanSpd),
			WindSpeedStd:          round1(stdSpd),
			WindSpeedP10:          round1(p10Spd),
			WindSpeedP90:          round1(p90Spd),
			WindDirSpreadDeg:      round1(dirSpread),
			GaleProbability:       round2(prob34),
			StrongWindProbability: round2(prob25),
			MemberSpeedMean:       round1(mMeanSOG),
			MemberSpeedStd:        round1(mStdSOG),
			MemberSpeedP10:        round1(mP10SOG),
			MemberSpeedP90:        round1(mP90SOG),
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
		totalWeightedScoreB += scoreB * weight
		totalDist += weight
	}

	// 3. Compute Overall Scores
	overallScoreA := totalWeightedScoreA / math.Max(totalDist, 1.0)
	overallScoreB := totalWeightedScoreB / math.Max(totalDist, 1.0)

	// Additional Strategy B duration penalty
	meanDur, stdDur := meanAndStd(memberDurations)
	sort.Float64s(memberDurations)
	minDur := memberDurations[0]
	maxDur := memberDurations[len(memberDurations)-1]
	p10Dur := percentile(memberDurations, 0.10)
	p90Dur := percentile(memberDurations, 0.90)
	iqrDur := percentile(memberDurations, 0.75) - percentile(memberDurations, 0.25)

	if meanDur > 0 {
		durSpreadRatio := stdDur / meanDur
		overallScoreB = overallScoreB * math.Exp(-1.5*durSpreadRatio)
	}
	overallScoreB = clamp(overallScoreB, 5.0, 99.0)

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

	// Final blended overall score
	primaryOverallScore := round1((overallScoreA*0.5 + overallScoreB*0.5))

	// Agreement Score (Strategy A vs B consistency)
	scoreDiff := math.Abs(overallScoreA - overallScoreB)
	agreementScore := round1(clamp(100.0-scoreDiff*1.8, 20.0, 99.0))

	// 4. Assemble Ensemble Comparison
	var memberOutcomes []MemberOutcome
	fastestID, slowestID := 0, 0
	fastestTime, slowestTime := 1e9, -1.0

	for m := 0; m < numMembers; m++ {
		dur := memberDurations[m]
		if dur <= 0 {
			dur = route.TotalDurationHours
		}
		if dur < fastestTime {
			fastestTime = dur
			fastestID = m
		}
		if dur > slowestTime {
			slowestTime = dur
			slowestID = m
		}
		avgSpd := route.TotalDistanceNM / math.Max(dur, 0.1)
		memberOutcomes = append(memberOutcomes, MemberOutcome{
			MemberID:           m,
			TotalDurationHours: round1(dur),
			AverageSpeedKts:    round1(avgSpd),
			MaxWindKts:         round1(memberMaxWind[m]),
			Trajectory:         memberTrajectories[m],
		})
	}

	comparison := &EnsembleComparison{
		MeanDurationHours: round1(meanDur),
		StdDurationHours:  round1(stdDur),
		MinDurationHours:  round1(minDur),
		MaxDurationHours:  round1(maxDur),
		IQRDurationHours:  round1(iqrDur),
		P10DurationHours:  round1(p10Dur),
		P90DurationHours:  round1(p90Dur),
		FastestMemberID:   fastestID,
		SlowestMemberID:   slowestID,
		MemberCount:       numMembers,
		Members:           memberOutcomes,
	}

	return &RouteConfidence{
		OverallScore:          primaryOverallScore,
		Category:              CategorizeConfidence(primaryOverallScore),
		ScoreStrategyA:        round1(overallScoreA),
		ScoreStrategyB:        round1(overallScoreB),
		AgreementScore:        agreementScore,
		ModelID:               modelID,
		NumMembers:            numMembers,
		Waypoints:             waypointConf,
		StatisticalComparison: statComparison,
		EnsembleComparison:    comparison,
	}, nil
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
