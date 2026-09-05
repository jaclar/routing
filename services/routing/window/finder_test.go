package window

import (
	"context"
	"testing"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/polar/polartest"
	"github.com/jaclar/routing-service/weather"
)

func TestCalculateCoarseness(t *testing.T) {
	// 1. Short passage: Grenada to Trinidad (~80 NM)
	depStep80, isoStep80, cfg80 := CalculateCoarseness(80.0, 48.0)
	if depStep80 != 3.0 {
		t.Errorf("Expected 3.0h departure step for 80 NM / 48h window, got %.1f", depStep80)
	}
	if isoStep80 != 20*time.Minute {
		t.Errorf("Expected 20m isochrone step for 80 NM, got %v", isoStep80)
	}
	if cfg80.MaxFrontierNodes != 200 {
		t.Errorf("Expected 200 max frontier nodes, got %d", cfg80.MaxFrontierNodes)
	}

	// 2. Medium coastal: Cowes to Fastnet (~320 NM)
	depStep320, isoStep320, _ := CalculateCoarseness(320.0, 120.0)
	if depStep320 != 12.0 {
		t.Errorf("Expected 12.0h departure step for 320 NM / 120h window, got %.1f", depStep320)
	}
	if isoStep320 != 45*time.Minute {
		t.Errorf("Expected 45m isochrone step for 320 NM, got %v", isoStep320)
	}

	// 3. Offshore / Ocean: Newport to Bermuda (~630 NM)
	depStep630, isoStep630, _ := CalculateCoarseness(630.0, 168.0)
	if depStep630 != 12.0 {
		t.Errorf("Expected 12.0h departure step for 630 NM / 7-day window, got %.1f", depStep630)
	}
	if isoStep630 != 90*time.Minute {
		t.Errorf("Expected 90m isochrone step for 630 NM, got %v", isoStep630)
	}

	// 4. Ocean Transpac (> 800 NM)
	depStep2000, isoStep2000, _ := CalculateCoarseness(2000.0, 240.0)
	if depStep2000 != 24.0 {
		t.Errorf("Expected 24.0h departure step for 2000 NM / 10-day window, got %.1f", depStep2000)
	}
	if isoStep2000 != 2*time.Hour {
		t.Errorf("Expected 2h isochrone step for 2000 NM, got %v", isoStep2000)
	}
}

func TestFindWindowsRealisticSimulation(t *testing.T) {
	startTime := time.Date(2026, 6, 1, 6, 0, 0, 0, time.UTC)
	weatherEngine := weather.NewRealisticGFSEngine(startTime)
	finder := NewWindowFinder(weatherEngine, nil) // no landmask to keep test fast

	// Newport to Bermuda coordinates (~630 NM)
	start := geo.Point{Lat: 41.40, Lon: -71.35}
	dest := geo.Point{Lat: 32.40, Lon: -64.55}
	latest := startTime.Add(48 * time.Hour) // 48h search window = 4-5 departures

	polarTable := polartest.Table()

	req := WindowRequest{
		Start:             start,
		Dest:              dest,
		EarliestDeparture: startTime,
		LatestDeparture:   &latest,
		BoatPreset:        "36ft-ketch",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := finder.FindWindows(ctx, req, polarTable)
	if err != nil {
		t.Fatalf("FindWindows failed: %v", err)
	}

	if len(resp.Windows) == 0 {
		t.Fatalf("Expected at least 1 window candidate, got 0")
	}

	t.Logf("Evaluated %d departures, found %d viable windows (TimeStep: %.2fh, DepStep: %.1fh)",
		resp.EvaluatedDepartures, len(resp.Windows), resp.TimeStepHours, resp.DepartureStepHours)

	// Check that windows are sorted by ComfortScore descending
	for i := 0; i < len(resp.Windows)-1; i++ {
		curr := resp.Windows[i]
		next := resp.Windows[i+1]
		if curr.ComfortScore < next.ComfortScore {
			t.Errorf("Window %d (score %.1f) ranked above window %d (score %.1f)",
				i, curr.ComfortScore, i+1, next.ComfortScore)
		}
		if curr.ComfortRank != i+1 {
			t.Errorf("Expected rank %d, got %d", i+1, curr.ComfortRank)
		}
	}

	// Verify telemetry fields on best window
	best := resp.Windows[0]
	if best.DurationHours <= 0 {
		t.Errorf("Expected positive duration, got %.1f", best.DurationHours)
	}
	if best.DistanceNM <= 0 {
		t.Errorf("Expected positive distance, got %.1f", best.DistanceNM)
	}
	if best.ComfortScore <= 0 || best.ComfortScore > 100 {
		t.Errorf("ComfortScore out of range [0..100]: %.1f", best.ComfortScore)
	}
	if best.ConfidenceScore <= 0 || best.ConfidenceScore > 100 {
		t.Errorf("ConfidenceScore out of range [0..100]: %.1f", best.ConfidenceScore)
	}

	// Verify Point of Sail fractions sum approximately to 100%
	posSum := best.UpwindFraction + best.CloseReachFraction + best.BeamReachFraction + best.BroadReachFraction + best.DownwindFraction
	if posSum < 90.0 || posSum > 110.0 {
		t.Errorf("Points of sail fractions do not sum near 100%%: %.1f%%", posSum)
	}

	// Verify Representative Weather Event
	rep := best.RepresentativeEvent
	if rep.Type != "mid_passage" && rep.Type != "peak_wind" {
		t.Errorf("Unexpected representative event type: %s", rep.Type)
	}
	if rep.Description == "" {
		t.Errorf("Expected non-empty description for representative event")
	}
	if len(rep.WeatherGrid) == 0 {
		t.Errorf("Expected representative weather grid slice to be populated")
	} else {
		t.Logf("Representative event: %s at %s (Grid: %d x %d)",
			rep.Description, rep.Time.Format(time.RFC3339), len(rep.WeatherGrid), len(rep.WeatherGrid[0]))
	}
}

func TestGaleAndLowWindWarnings(t *testing.T) {
	// Create synthetic routes with high winds and low winds to verify warning triggers
	refTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := refTime

	// 1. Route with Gale Force Winds (36 kts)
	galeRoute := &isochrone.RouteResult{
		StartTime:          refTime,
		ArrivalTime:        refTime.Add(24 * time.Hour),
		TotalDurationHours: 24.0,
		TotalDistanceNM:    140.0,
		Waypoints: []isochrone.Waypoint{
			{Time: refTime, TWSKts: 18.0, TWADeg: 90.0, WaveHeightM: 1.5, WavePeriodS: 7.0},
			{Time: refTime.Add(6 * time.Hour), TWSKts: 36.0, GustKts: 44.0, TWADeg: 85.0, WaveHeightM: 4.2, WavePeriodS: 6.5},
			{Time: refTime.Add(12 * time.Hour), TWSKts: 28.0, TWADeg: 90.0, WaveHeightM: 3.0, WavePeriodS: 7.0},
			{Time: refTime.Add(24 * time.Hour), TWSKts: 16.0, TWADeg: 95.0, WaveHeightM: 1.8, WavePeriodS: 7.5},
		},
	}

	weatherEngine := weather.NewRealisticGFSEngine(refTime)
	galeCand := evaluateWindowCandidate(galeRoute, refTime, now, weatherEngine, 10, 20, -70, -60)
	if !galeCand.GaleWarning {
		t.Errorf("Expected GaleWarning to be true for 36 kts wind")
	}
	if galeCand.GaleWarningDetail == "" {
		t.Errorf("Expected non-empty GaleWarningDetail")
	}
	if galeCand.RepresentativeEvent.Type != "peak_wind" {
		t.Errorf("Expected peak_wind representative event for gale passage, got %s", galeCand.RepresentativeEvent.Type)
	}

	// 2. Route with Prolonged Calms (< 5 kts for 8 hours)
	calmRoute := &isochrone.RouteResult{
		StartTime:          refTime,
		ArrivalTime:        refTime.Add(20 * time.Hour),
		TotalDurationHours: 20.0,
		TotalDistanceNM:    90.0,
		Waypoints: []isochrone.Waypoint{
			{Time: refTime, TWSKts: 12.0, TWADeg: 90.0, WaveHeightM: 0.8, WavePeriodS: 6.0},
			{Time: refTime.Add(4 * time.Hour), TWSKts: 4.0, TWADeg: 90.0, WaveHeightM: 0.4, WavePeriodS: 5.0},
			{Time: refTime.Add(8 * time.Hour), TWSKts: 3.5, TWADeg: 90.0, WaveHeightM: 0.3, WavePeriodS: 5.0},
			{Time: refTime.Add(12 * time.Hour), TWSKts: 4.5, TWADeg: 90.0, WaveHeightM: 0.3, WavePeriodS: 5.0},
			{Time: refTime.Add(20 * time.Hour), TWSKts: 10.0, TWADeg: 90.0, WaveHeightM: 0.6, WavePeriodS: 6.0},
		},
	}

	calmCand := evaluateWindowCandidate(calmRoute, refTime, now, weatherEngine, 10, 20, -70, -60)
	if !calmCand.LowWindWarning {
		t.Errorf("Expected LowWindWarning to be true for prolonged calm")
	}
	if calmCand.LowWindWarningDetail == "" {
		t.Errorf("Expected non-empty LowWindWarningDetail")
	}

	// 3. Short Passage Arriving in Darkness (< 48 hours)
	// Dest at lon 0.0 (Greenwich, UTC = Local), arrival at 23:00 UTC (11 PM night)
	nightRoute := &isochrone.RouteResult{
		StartTime:          refTime,
		ArrivalTime:        refTime.Add(11 * time.Hour), // 12:00 + 11h = 23:00 UTC
		TotalDurationHours: 11.0,
		TotalDistanceNM:    65.0,
		DestPoint:          geo.Point{Lat: 50.0, Lon: 0.0},
		Waypoints: []isochrone.Waypoint{
			{Time: refTime, TWSKts: 14.0, TWADeg: 90.0, WaveHeightM: 0.9, WavePeriodS: 7.0},
			{Time: refTime.Add(11 * time.Hour), TWSKts: 14.0, TWADeg: 90.0, WaveHeightM: 0.9, WavePeriodS: 7.0},
		},
	}
	nightCand := evaluateWindowCandidate(nightRoute, refTime, now, weatherEngine, 45, 55, -5, 5)
	if !nightCand.NightArrivalWarning {
		t.Errorf("Expected NightArrivalWarning to be true for 23:00 arrival on 11h passage")
	}

	// 4. Short Passage Arriving in Daylight (< 48 hours)
	// Dest at lon 0.0, arrival at 14:00 UTC (2 PM day)
	dayRoute := &isochrone.RouteResult{
		StartTime:          refTime,
		ArrivalTime:        refTime.Add(2 * time.Hour), // 12:00 + 2h = 14:00 UTC
		TotalDurationHours: 2.0,
		TotalDistanceNM:    15.0,
		DestPoint:          geo.Point{Lat: 50.0, Lon: 0.0},
		Waypoints: []isochrone.Waypoint{
			{Time: refTime, TWSKts: 14.0, TWADeg: 90.0, WaveHeightM: 0.9, WavePeriodS: 7.0},
			{Time: refTime.Add(2 * time.Hour), TWSKts: 14.0, TWADeg: 90.0, WaveHeightM: 0.9, WavePeriodS: 7.0},
		},
	}
	dayCand := evaluateWindowCandidate(dayRoute, refTime, now, weatherEngine, 45, 55, -5, 5)
	if dayCand.NightArrivalWarning {
		t.Errorf("Expected NightArrivalWarning to be false for 14:00 arrival on 2h passage")
	}

	// 5. Long Passage Arriving in Darkness (>= 48 hours: should NOT trigger warning)
	longNightRoute := &isochrone.RouteResult{
		StartTime:          refTime,
		ArrivalTime:        refTime.Add(59 * time.Hour), // 12:00 + 59h = 23:00 UTC
		TotalDurationHours: 59.0,
		TotalDistanceNM:    360.0,
		DestPoint:          geo.Point{Lat: 50.0, Lon: 0.0},
		Waypoints: []isochrone.Waypoint{
			{Time: refTime, TWSKts: 14.0, TWADeg: 90.0, WaveHeightM: 0.9, WavePeriodS: 7.0},
			{Time: refTime.Add(59 * time.Hour), TWSKts: 14.0, TWADeg: 90.0, WaveHeightM: 0.9, WavePeriodS: 7.0},
		},
	}
	longCand := evaluateWindowCandidate(longNightRoute, refTime, now, weatherEngine, 45, 55, -5, 5)
	if longCand.NightArrivalWarning {
		t.Errorf("Expected NightArrivalWarning to be false for passage >= 48 hours")
	}
}
