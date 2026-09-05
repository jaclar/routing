package window

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

// WindowRequest defines parameters for searching weather windows.
type WindowRequest struct {
	Start             geo.Point         `json:"start"`
	Dest              geo.Point         `json:"dest"`
	EarliestDeparture time.Time         `json:"earliest_departure"`
	LatestDeparture   *time.Time        `json:"latest_departure,omitempty"`
	BoatPreset        string            `json:"boat_preset,omitempty"`
	Model             string            `json:"model,omitempty"`
	CustomBoat        interface{}       `json:"custom_boat,omitempty"`
	CustomPolar       *polar.PolarTable `json:"custom_polar,omitempty"`
}

// RepresentativeWeatherEvent holds the weather state and a 2D wind grid slice at a key voyage moment.
type RepresentativeWeatherEvent struct {
	Type         string                    `json:"type"` // "mid_passage" | "peak_wind"
	Time         time.Time                 `json:"time"`
	Description  string                    `json:"description"`
	Location     geo.Point                 `json:"location"`
	WindSpeedKts float64                   `json:"wind_speed_kts"`
	WindDirDeg   float64                   `json:"wind_dir_deg"`
	WaveHeightM  float64                   `json:"wave_height_m"`
	WavePeriodS  float64                   `json:"wave_period_s"`
	GridMinLat   float64                   `json:"grid_min_lat,omitempty"`
	GridMaxLat   float64                   `json:"grid_max_lat,omitempty"`
	GridMinLon   float64                   `json:"grid_min_lon,omitempty"`
	GridMaxLon   float64                   `json:"grid_max_lon,omitempty"`
	GridLatStep  float64                   `json:"grid_lat_step,omitempty"`
	GridLonStep  float64                   `json:"grid_lon_step,omitempty"`
	WeatherGrid  [][]weather.WindCondition `json:"weather_grid,omitempty"`
}

// WindowCandidate encapsulates a complete solved route and nautical comfort evaluation for a departure.
type WindowCandidate struct {
	DepartureTime      time.Time                  `json:"departure_time"`
	ArrivalTime        time.Time                  `json:"arrival_time"`
	DurationHours      float64                    `json:"duration_hours"`
	DistanceNM         float64                    `json:"distance_nm"`
	ComfortScore       float64                    `json:"comfort_score"` // [0..100]
	ComfortRank        int                        `json:"comfort_rank"`
	ConfidenceScore    float64                    `json:"confidence_score"` // [0..100]

	// Point of sail breakdown (percentages summing to ~100%)
	UpwindFraction     float64                    `json:"upwind_fraction"`     // Beating <60°
	CloseReachFraction float64                    `json:"close_reach_fraction"` // 60-80°
	BeamReachFraction  float64                    `json:"beam_reach_fraction"`  // 80-120° (the sweet spot)
	BroadReachFraction float64                    `json:"broad_reach_fraction"` // 120-150°
	DownwindFraction   float64                    `json:"downwind_fraction"`   // >150°

	// Passage statistics
	AvgWindKts         float64                    `json:"avg_wind_kts"`
	MaxWindKts         float64                    `json:"max_wind_kts"`
	AvgWaveHeightM     float64                    `json:"avg_wave_height_m"`
	MaxWaveHeightM     float64                    `json:"max_wave_height_m"`
	AvgWavePeriodS     float64                    `json:"avg_wave_period_s"`
	MaxHeelDeg         float64                    `json:"max_heel_deg"`
	TotalTacks         int                        `json:"total_tacks"`
	TotalGybes         int                        `json:"total_gybes"`

	// Safety warnings
	GaleWarning          bool                       `json:"gale_warning"`
	GaleWarningDetail    string                     `json:"gale_warning_detail,omitempty"`
	LowWindWarning       bool                       `json:"low_wind_warning"`
	LowWindWarningDetail string                     `json:"low_wind_warning_detail,omitempty"`
	NightArrivalWarning       bool                  `json:"night_arrival_warning"`
	NightArrivalWarningDetail string                `json:"night_arrival_warning_detail,omitempty"`

	// Representative weather event
	RepresentativeEvent RepresentativeWeatherEvent `json:"representative_event"`

	// Full calculated route
	Route              *isochrone.RouteResult     `json:"route"`
}

// WindowSearchResponse is the response returned to API clients.
type WindowSearchResponse struct {
	Start              geo.Point         `json:"start"`
	Dest               geo.Point         `json:"dest"`
	DirectDistanceNM   float64           `json:"direct_distance_nm"`
	TimeStepHours      float64           `json:"time_step_hours"`
	DepartureStepHours float64           `json:"departure_step_hours"`
	EvaluatedDepartures int              `json:"evaluated_departures"`
	EarliestDeparture  time.Time         `json:"earliest_departure"`
	LatestDeparture    time.Time         `json:"latest_departure"`
	Windows            []WindowCandidate `json:"windows"`
}

// WindowFinder handles search, simulation, and scoring of departure windows.
type WindowFinder struct {
	weatherProvider weather.WeatherProvider
	landMask        *landmask.LandMask
}

// NewWindowFinder initializes a WindowFinder.
func NewWindowFinder(wp weather.WeatherProvider, lm *landmask.LandMask) *WindowFinder {
	return &WindowFinder{
		weatherProvider: wp,
		landMask:        lm,
	}
}

// CalculateCoarseness determines the departure sampling step and isochrone router config
// dynamically based on great-circle distance and search window length.
func CalculateCoarseness(directDistNM, windowHours float64) (depStepHours float64, isoStep time.Duration, cfg isochrone.RouterConfig) {
	cfg = isochrone.DefaultRouterConfig()
	cfg.PruningStrategy = isochrone.PruningSpatialGrid
	cfg.HeadingSpreadDeg = 150.0
	cfg.TackPenaltyMinutes = 5.0
	cfg.GybePenaltyMinutes = 8.0

	if directDistNM <= 100.0 {
		// Short coastal/island passage (< 100 NM, ~10-16h sailing)
		if windowHours <= 48.0 {
			depStepHours = 3.0
		} else if windowHours <= 120.0 {
			depStepHours = 6.0
		} else {
			depStepHours = 12.0
		}
		isoStep = 20 * time.Minute
		cfg.TimeStep = isoStep
		cfg.ArrivalRadiusNM = 1.8
		cfg.HeadingStepDeg = 10.0
		cfg.MaxFrontierNodes = 200
	} else if directDistNM <= 350.0 {
		// Medium coastal / channel passage (100 - 350 NM, ~1-3 days)
		if windowHours <= 96.0 {
			depStepHours = 6.0
		} else {
			depStepHours = 12.0
		}
		isoStep = 45 * time.Minute
		cfg.TimeStep = isoStep
		cfg.ArrivalRadiusNM = 4.0
		cfg.HeadingStepDeg = 12.0
		cfg.MaxFrontierNodes = 220
	} else if directDistNM <= 800.0 {
		// Offshore passage (350 - 800 NM, ~3-6 days)
		if windowHours <= 168.0 {
			depStepHours = 12.0
		} else {
			depStepHours = 24.0
		}
		isoStep = 90 * time.Minute
		cfg.TimeStep = isoStep
		cfg.ArrivalRadiusNM = 8.0
		cfg.HeadingStepDeg = 14.0
		cfg.MaxFrontierNodes = 220
	} else {
		// Ocean crossing (> 800 NM)
		if windowHours <= 168.0 {
			depStepHours = 12.0
		} else {
			depStepHours = 24.0
		}
		isoStep = 2 * time.Hour
		cfg.TimeStep = isoStep
		cfg.ArrivalRadiusNM = 12.0
		cfg.HeadingStepDeg = 15.0
		cfg.MaxFrontierNodes = 250
	}

	return depStepHours, isoStep, cfg
}

// FindWindows searches for departure windows over the requested time horizon,
// solves routes in parallel, calculates comfort scores, and returns ranked candidates.
func (f *WindowFinder) FindWindows(
	ctx context.Context,
	req WindowRequest,
	polarTable *polar.PolarTable,
) (*WindowSearchResponse, error) {
	directDistNM := geo.DistanceNM(req.Start, req.Dest)
	if directDistNM < 0.5 {
		return nil, fmt.Errorf("start and destination are too close (%.2f NM)", directDistNM)
	}

	if f.landMask != nil && f.landMask.IsLand(req.Start) {
		return nil, fmt.Errorf("start location (%.3f, %.3f) is on land", req.Start.Lat, req.Start.Lon)
	}
	if f.landMask != nil && f.landMask.IsLand(req.Dest) {
		return nil, fmt.Errorf("destination location (%.3f, %.3f) is on land", req.Dest.Lat, req.Dest.Lon)
	}

	startTime := req.EarliestDeparture
	if startTime.IsZero() {
		startTime = time.Now().UTC()
	}

	// Determine search window duration
	estPassageHours := directDistNM / 6.0
	defaultWindowHours := math.Max(168.0, estPassageHours*1.5) // Default ~7 days
	if defaultWindowHours > 336.0 {
		defaultWindowHours = 336.0 // Cap at 14 days
	}

	endTime := startTime.Add(time.Duration(defaultWindowHours * float64(time.Hour)))
	if req.LatestDeparture != nil && !req.LatestDeparture.IsZero() {
		if req.LatestDeparture.After(startTime) {
			endTime = *req.LatestDeparture
		}
	}

	windowHours := endTime.Sub(startTime).Hours()
	if windowHours < 6.0 {
		windowHours = 6.0
		endTime = startTime.Add(6 * time.Hour)
	}
	// Max forecast horizon ceiling: 14 days (336 hours)
	if windowHours > 336.0 {
		windowHours = 336.0
		endTime = startTime.Add(336 * time.Hour)
	}

	depStepHours, isoStep, cfg := CalculateCoarseness(directDistNM, windowHours)

	// Build list of departure candidate timestamps
	var departures []time.Time
	depStep := time.Duration(depStepHours * float64(time.Hour))
	for t := startTime; !t.After(endTime); t = t.Add(depStep) {
		departures = append(departures, t)
	}

	// Limit to max 24 departures to maintain snappy response
	if len(departures) > 24 {
		skip := int(math.Ceil(float64(len(departures)) / 24.0))
		sampled := make([]time.Time, 0, 24)
		for i := 0; i < len(departures); i += skip {
			sampled = append(sampled, departures[i])
		}
		departures = sampled
	}

	type solveItem struct {
		depTime time.Time
		route   *isochrone.RouteResult
		err     error
	}

	solveChan := make(chan solveItem, len(departures))
	// Worker pool concurrency
	concurrency := 4
	if len(departures) < concurrency {
		concurrency = len(departures)
	}

	depChan := make(chan time.Time, len(departures))
	for _, d := range departures {
		depChan <- d
	}
	close(depChan)

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dep := range depChan {
				select {
				case <-ctx.Done():
					solveChan <- solveItem{depTime: dep, route: nil, err: ctx.Err()}
					return
				default:
				}

				route, err := isochrone.CalculateOptimalRoute(
					req.Start,
					req.Dest,
					dep,
					polarTable,
					f.weatherProvider,
					f.landMask,
					cfg,
				)
				solveChan <- solveItem{depTime: dep, route: route, err: err}
			}
		}()
	}

	wg.Wait()
	close(solveChan)

	// Bounding box for representative weather grid sampling
	minLat := math.Min(req.Start.Lat, req.Dest.Lat) - 3.0
	maxLat := math.Max(req.Start.Lat, req.Dest.Lat) + 3.0
	minLon := math.Min(req.Start.Lon, req.Dest.Lon) - 3.0
	maxLon := math.Max(req.Start.Lon, req.Dest.Lon) + 3.0

	var candidates []WindowCandidate
	now := time.Now().UTC()

	for item := range solveChan {
		if item.err != nil || item.route == nil || !item.route.DestinationReached {
			continue
		}
		c := evaluateWindowCandidate(item.route, startTime, now, f.weatherProvider, minLat, maxLat, minLon, maxLon)
		candidates = append(candidates, c)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no viable weather windows found between %s and %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	}

	// Sort by Comfort Score descending (Rank 1 is most comfortable)
	sort.Slice(candidates, func(i, j int) bool {
		if math.Abs(candidates[i].ComfortScore-candidates[j].ComfortScore) > 0.05 {
			return candidates[i].ComfortScore > candidates[j].ComfortScore
		}
		// Tie-break: faster duration
		return candidates[i].DurationHours < candidates[j].DurationHours
	})

	for i := range candidates {
		candidates[i].ComfortRank = i + 1
	}

	resp := &WindowSearchResponse{
		Start:               req.Start,
		Dest:                req.Dest,
		DirectDistanceNM:    directDistNM,
		TimeStepHours:       isoStep.Hours(),
		DepartureStepHours:  depStepHours,
		EvaluatedDepartures: len(departures),
		EarliestDeparture:   startTime,
		LatestDeparture:     endTime,
		Windows:             candidates,
	}

	return resp, nil
}

// evaluateWindowCandidate analyzes point of sail, waves, wind, confidence, and warnings for a route.
func evaluateWindowCandidate(
	r *isochrone.RouteResult,
	refTime time.Time,
	now time.Time,
	wp weather.WeatherProvider,
	minLat, maxLat, minLon, maxLon float64,
) WindowCandidate {
	totalWps := len(r.Waypoints)
	if totalWps == 0 {
		return WindowCandidate{Route: r}
	}

	// 1. Points of sail accumulation
	upwindCount := 0
	closeReachCount := 0
	beamReachCount := 0
	broadReachCount := 0
	downwindCount := 0

	sumWind := 0.0
	maxWind := 0.0
	sumWaveHeight := 0.0
	maxWaveHeight := 0.0
	sumWavePeriod := 0.0
	maxHeel := 0.0

	calmHours := 0.0
	maxCalmHours := 0.0
	currentCalmHours := 0.0

	var maxWindWp isochrone.Waypoint
	var midWp isochrone.Waypoint

	midTargetTime := r.StartTime.Add(time.Duration(r.TotalDurationHours / 2.0 * float64(time.Hour)))
	minMidDiff := time.Duration(1<<62 - 1)

	for i, wp := range r.Waypoints {
		twa := math.Abs(wp.TWADeg)
		switch {
		case twa < 60.0:
			upwindCount++ // Close hauled (< 60°)
		case twa < 75.0:
			closeReachCount++ // Close reach (60° – 75°)
		case twa <= 105.0:
			beamReachCount++ // Beam reach (75° – 105°)
		case twa <= 150.0:
			broadReachCount++ // Broad reach (105° – 150°)
		default:
			downwindCount++ // Dead downwind (150° – 180°)
		}

		sumWind += wp.TWSKts
		if wp.TWSKts > maxWind {
			maxWind = wp.TWSKts
			maxWindWp = wp
		}

		sumWaveHeight += wp.WaveHeightM
		if wp.WaveHeightM > maxWaveHeight {
			maxWaveHeight = wp.WaveHeightM
		}

		sumWavePeriod += wp.WavePeriodS
		if wp.EstimatedHeelDeg > maxHeel {
			maxHeel = wp.EstimatedHeelDeg
		}

		// Light air tracking
		if wp.TWSKts < 6.0 {
			if i > 0 {
				dt := wp.Time.Sub(r.Waypoints[i-1].Time).Hours()
				currentCalmHours += dt
				calmHours += dt
				if currentCalmHours > maxCalmHours {
					maxCalmHours = currentCalmHours
				}
			}
		} else {
			currentCalmHours = 0.0
		}

		// Mid-passage waypoint finding
		diff := wp.Time.Sub(midTargetTime)
		if diff < 0 {
			diff = -diff
		}
		if diff < minMidDiff {
			minMidDiff = diff
			midWp = wp
		}
	}

	n := float64(totalWps)
	upwindFrac := math.Round((float64(upwindCount)/n)*1000.0) / 10.0
	closeReachFrac := math.Round((float64(closeReachCount)/n)*1000.0) / 10.0
	beamReachFrac := math.Round((float64(beamReachCount)/n)*1000.0) / 10.0
	broadReachFrac := math.Round((float64(broadReachCount)/n)*1000.0) / 10.0
	downwindFrac := math.Round((float64(downwindCount)/n)*1000.0) / 10.0

	avgWind := math.Round((sumWind/n)*10.0) / 10.0
	avgWaveHeight := math.Round((sumWaveHeight/n)*100.0) / 100.0
	avgWavePeriod := math.Round((sumWavePeriod/n)*10.0) / 10.0

	// 2. Component Comfort Scores (each [0..100])
	// A. Point of Sail Comfort (reward beam reach and broad reach, dead downwind next, close reach after, close hauled least desired)
	// Close Hauled (<60°) = 15, Close Reach (60-80°) = 40, Beam Reach (80-110°) = 100, Broad Reach (110-160°) = 95, Dead Downwind (160-180°) = 75
	scorePos := (float64(upwindCount)*15.0 +
		float64(closeReachCount)*40.0 +
		float64(beamReachCount)*100.0 +
		float64(broadReachCount)*95.0 +
		float64(downwindCount)*75.0) / n

	// B. Wave Comfort: Smaller waves with longer period are vastly more comfortable
	// Steepness index: waveHeight^1.2 / wavePeriod^0.8
	// Ideal: < 1.2m with period >= 8s -> ~95-100 score
	// Rough: > 2.5m with period < 6s -> low score
	wavePenalty := 22.0 * math.Pow(math.Max(0.1, avgWaveHeight), 1.15)
	periodBonus := math.Min(15.0, math.Max(-15.0, (avgWavePeriod-6.0)*2.8))
	scoreWave := 100.0 - wavePenalty + periodBonus
	if maxWaveHeight > 2.5 {
		scoreWave -= (maxWaveHeight - 2.5) * 12.0
	}
	scoreWave = math.Max(5.0, math.Min(100.0, scoreWave))

	// C. Wind Intensity Comfort: Ideal 12 - 20 kts
	var scoreWind float64
	switch {
	case avgWind >= 12.0 && avgWind <= 18.0:
		scoreWind = 100.0
	case avgWind > 18.0 && avgWind <= 24.0:
		scoreWind = 100.0 - (avgWind-18.0)*2.5
	case avgWind > 24.0 && avgWind <= 30.0:
		scoreWind = 85.0 - (avgWind-24.0)*5.0
	case avgWind > 30.0:
		scoreWind = math.Max(10.0, 55.0-(avgWind-30.0)*4.0)
	default: // < 12 kts
		scoreWind = 100.0 - (12.0-avgWind)*4.0
	}
	if maxWind > 28.0 {
		scoreWind -= (maxWind - 28.0) * 2.0
	}
	scoreWind = math.Max(5.0, math.Min(100.0, scoreWind))

	// D. Confidence Score: Decreases smoothly with forecast lead time
	leadHours := math.Max(0.0, r.StartTime.Sub(now).Hours())
	scoreConf := math.Max(35.0, math.Min(98.0, 96.0-0.16*leadHours))

	// 3. Safety Warnings
	// Night Arrival Warning: For passages < 48 hours, arriving in darkness is hazardous/stressful.
	// We estimate local solar time at the destination using longitude (15 deg = 1 hour).
	nightArrivalWarning := false
	nightArrivalDetail := ""
	if r.TotalDurationHours < 48.0 {
		destLon := r.DestPoint.Lon
		lonOffsetHours := destLon / 15.0
		arrUTC := r.ArrivalTime.UTC()
		localHour := float64(arrUTC.Hour()) + float64(arrUTC.Minute())/60.0 + lonOffsetHours
		for localHour < 0 {
			localHour += 24.0
		}
		for localHour >= 24.0 {
			localHour -= 24.0
		}
		// Darkness: arrival between 20:00 (8 PM) and 06:00 (6 AM) local solar time
		if localHour >= 20.0 || localHour < 6.0 {
			nightArrivalWarning = true
			h := int(localHour)
			m := int((localHour - float64(h)) * 60.0)
			nightArrivalDetail = fmt.Sprintf("Night Arrival Warning: Estimated arrival at %s (~%02d:%02d local time) in darkness. Harbor entry and coastal navigation in the dark.",
				arrUTC.Format("15:04 UTC"), h, m)
		}
	}

	// Combined Comfort Score:
	// 40% Point of Sail + 30% Waves + 15% Wind Intensity + 15% Confidence
	combinedScore := 0.40*scorePos + 0.30*scoreWave + 0.15*scoreWind + 0.15*scoreConf
	if nightArrivalWarning {
		combinedScore -= 8.0 // Comfort penalty for arriving in the dark on short passage (<48h)
	}
	comfortScore := math.Round(math.Max(0.0, math.Min(100.0, combinedScore))*10.0) / 10.0
	confidenceScore := math.Round(scoreConf*10.0) / 10.0

	// Gale Warning: peak wind >= 30 kts or wave height >= 3.5m
	galeWarning := false
	galeDetail := ""
	if maxWind >= 30.0 || maxWaveHeight >= 3.5 {
		galeWarning = true
		galeDetail = fmt.Sprintf("Gale Warning: Peak wind %.0f kts (gusts %.0f kts), seas %.1fm forecast around %s",
			maxWindWp.TWSKts, maxWindWp.GustKts, maxWindWp.WaveHeightM, maxWindWp.Time.Format("Mon Jan 02 15:04 UTC"))
	}

	// Low Wind Warning: avg wind < 8 kts or calm hours >= 4h
	lowWindWarning := false
	lowWindDetail := ""
	if avgWind < 8.0 || maxCalmHours >= 4.0 || calmHours > (r.TotalDurationHours*0.25) {
		lowWindWarning = true
		lowWindDetail = fmt.Sprintf("Low Wind Warning: Extended light airs (<6 kts) for up to %.0f consecutive hours (avg %.1f kts). Motoring likely required.",
			maxCalmHours, avgWind)
	}

	// 4. Representative Weather Map Event
	// Use peak wind event if TWS >= 22 kts, otherwise mid-passage
	var repEvent RepresentativeWeatherEvent
	if maxWind >= 22.0 && maxWindWp.TWSKts > 0 {
		repEvent = RepresentativeWeatherEvent{
			Type:         "peak_wind",
			Time:         maxWindWp.Time,
			Description:  fmt.Sprintf("Peak Wind Event: %.0f kts from %03.0f°", maxWindWp.TWSKts, maxWindWp.TWDDeg),
			Location:     geo.Point{Lat: maxWindWp.Lat, Lon: maxWindWp.Lon},
			WindSpeedKts: maxWindWp.TWSKts,
			WindDirDeg:   maxWindWp.TWDDeg,
			WaveHeightM:  maxWindWp.WaveHeightM,
			WavePeriodS:  maxWindWp.WavePeriodS,
		}
	} else {
		repEvent = RepresentativeWeatherEvent{
			Type:         "mid_passage",
			Time:         midWp.Time,
			Description:  "Mid-Passage Conditions",
			Location:     geo.Point{Lat: midWp.Lat, Lon: midWp.Lon},
			WindSpeedKts: midWp.TWSKts,
			WindDirDeg:   midWp.TWDDeg,
			WaveHeightM:  midWp.WaveHeightM,
			WavePeriodS:  midWp.WavePeriodS,
		}
	}

	// Fetch compact 2D weather grid slice for this representative timestamp
	latStep := math.Max(1.5, math.Round((maxLat-minLat)/6.0*10.0)/10.0)
	lonStep := math.Max(1.5, math.Round((maxLon-minLon)/6.0*10.0)/10.0)
	gridSlice, err := wp.GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep, repEvent.Time)
	if err == nil && len(gridSlice) > 0 {
		repEvent.GridMinLat = minLat
		repEvent.GridMaxLat = maxLat
		repEvent.GridMinLon = minLon
		repEvent.GridMaxLon = maxLon
		repEvent.GridLatStep = latStep
		repEvent.GridLonStep = lonStep
		repEvent.WeatherGrid = gridSlice
	}

	return WindowCandidate{
		DepartureTime:             r.StartTime,
		ArrivalTime:               r.ArrivalTime,
		DurationHours:             math.Round(r.TotalDurationHours*10.0) / 10.0,
		DistanceNM:                math.Round(r.TotalDistanceNM*10.0) / 10.0,
		ComfortScore:              comfortScore,
		ConfidenceScore:           confidenceScore,
		UpwindFraction:            upwindFrac,
		CloseReachFraction:        closeReachFrac,
		BeamReachFraction:         beamReachFrac,
		BroadReachFraction:        broadReachFrac,
		DownwindFraction:          downwindFrac,
		AvgWindKts:                avgWind,
		MaxWindKts:                math.Round(maxWind*10.0) / 10.0,
		AvgWaveHeightM:            avgWaveHeight,
		MaxWaveHeightM:            math.Round(maxWaveHeight*100.0) / 100.0,
		AvgWavePeriodS:            avgWavePeriod,
		MaxHeelDeg:                math.Round(maxHeel*10.0) / 10.0,
		TotalTacks:                r.TotalTacks,
		TotalGybes:                r.TotalGybes,
		GaleWarning:               galeWarning,
		GaleWarningDetail:         galeDetail,
		LowWindWarning:            lowWindWarning,
		LowWindWarningDetail:      lowWindDetail,
		NightArrivalWarning:       nightArrivalWarning,
		NightArrivalWarningDetail: nightArrivalDetail,
		RepresentativeEvent:       repEvent,
		Route:                     r,
	}
}
