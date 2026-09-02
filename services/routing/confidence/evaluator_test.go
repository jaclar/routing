package confidence

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

func TestConfidenceCategorization(t *testing.T) {
	cases := []struct {
		score    float64
		expected string
	}{
		{92.0, "Very High"},
		{85.0, "Very High"},
		{75.0, "High"},
		{60.0, "Moderate"},
		{40.0, "Low"},
		{20.0, "High Uncertainty"},
		{0.0, "High Uncertainty"},
	}

	for _, c := range cases {
		cat := CategorizeConfidence(c.score)
		if cat != c.expected {
			t.Errorf("score %f: expected category %s, got %s", c.score, c.expected, cat)
		}
	}
}

func TestEvaluateRouteSynthetic(t *testing.T) {
	eval := NewEvaluator("", nil, nil)

	startTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	wps := []isochrone.Waypoint{
		{
			Lat:          12.0,
			Lon:          -61.75,
			Time:         startTime,
			HeadingDeg:   180.0,
			BoatSpeedKts: 7.5,
			TWSKts:       16.0,
			TWDDeg:       85.0,
			DistanceNM:   0.0,
		},
		{
			Lat:          11.5,
			Lon:          -61.70,
			Time:         startTime.Add(3 * time.Hour),
			HeadingDeg:   180.0,
			BoatSpeedKts: 7.2,
			TWSKts:       18.0,
			TWDDeg:       90.0,
			DistanceNM:   22.5,
		},
		{
			Lat:          10.8,
			Lon:          -61.65,
			Time:         startTime.Add(8 * time.Hour),
			HeadingDeg:   180.0,
			BoatSpeedKts: 7.0,
			TWSKts:       20.0,
			TWDDeg:       95.0,
			DistanceNM:   58.0,
		},
	}

	route := &isochrone.RouteResult{
		BoatName:           "Test Cruiser",
		StartTime:          startTime,
		ArrivalTime:        startTime.Add(8 * time.Hour),
		TotalDurationHours: 8.0,
		TotalDistanceNM:    58.0,
		Waypoints:          wps,
	}

	polarTable := polar.Get36ftKetchPolar()

	conf, err := eval.EvaluateRoute(context.Background(), route, polarTable, "gefs_0p50")
	if err != nil {
		t.Fatalf("EvaluateRoute failed: %v", err)
	}

	if conf.OverallScore <= 0 || conf.OverallScore > 100 {
		t.Errorf("invalid overall score: %f", conf.OverallScore)
	}

	if conf.NumMembers != 31 {
		t.Errorf("expected 31 members for GEFS, got %d", conf.NumMembers)
	}

	if len(conf.Waypoints) != 3 {
		t.Fatalf("expected 3 waypoints confidence, got %d", len(conf.Waypoints))
	}

	// First waypoint (departure) should have higher confidence than later waypoints
	if conf.Waypoints[0].Score < conf.Waypoints[2].Score {
		t.Errorf("expected departure confidence (%f) >= downstream confidence (%f)",
			conf.Waypoints[0].Score, conf.Waypoints[2].Score)
	}

	// Strategy A & B comparison
	if conf.ScoreStrategyA <= 0 || conf.ScoreStrategyB <= 0 {
		t.Errorf("expected positive Strategy A and B scores: A=%f, B=%f", conf.ScoreStrategyA, conf.ScoreStrategyB)
	}

	if conf.EnsembleComparison == nil {
		t.Fatalf("expected EnsembleComparison to be populated")
	}

	if conf.EnsembleComparison.MemberCount != 31 {
		t.Errorf("expected 31 members in comparison, got %d", conf.EnsembleComparison.MemberCount)
	}

	if conf.EnsembleComparison.MinDurationHours <= 0 || conf.EnsembleComparison.MaxDurationHours < conf.EnsembleComparison.MinDurationHours {
		t.Errorf("invalid duration bounds: min=%f, max=%f", conf.EnsembleComparison.MinDurationHours, conf.EnsembleComparison.MaxDurationHours)
	}
}

func TestStatisticalHelpers(t *testing.T) {
	vals := []float64{10.0, 12.0, 14.0, 16.0, 18.0}
	m, s := meanAndStd(vals)
	if math.Abs(m-14.0) > 1e-6 {
		t.Errorf("expected mean 14.0, got %f", m)
	}
	if s <= 0 {
		t.Errorf("expected positive std, got %f", s)
	}

	p50 := percentile(vals, 0.50)
	if math.Abs(p50-14.0) > 1e-6 {
		t.Errorf("expected P50 14.0, got %f", p50)
	}
}

func TestDeterministicModelMapping(t *testing.T) {
	eval := NewEvaluator("", nil, nil)

	startTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	wps := []isochrone.Waypoint{
		{Lat: 12.0, Lon: -61.75, Time: startTime, HeadingDeg: 180.0, BoatSpeedKts: 7.5, TWSKts: 16.0, TWDDeg: 85.0, DistanceNM: 0.0},
		{Lat: 11.5, Lon: -61.70, Time: startTime.Add(3 * time.Hour), HeadingDeg: 180.0, BoatSpeedKts: 7.2, TWSKts: 18.0, TWDDeg: 90.0, DistanceNM: 22.5},
		{Lat: 10.8, Lon: -61.65, Time: startTime.Add(8 * time.Hour), HeadingDeg: 180.0, BoatSpeedKts: 7.0, TWSKts: 20.0, TWDDeg: 95.0, DistanceNM: 58.0},
	}
	route := &isochrone.RouteResult{
		BoatName:           "Test Cruiser",
		StartTime:          startTime,
		ArrivalTime:        startTime.Add(8 * time.Hour),
		TotalDurationHours: 8.0,
		TotalDistanceNM:    58.0,
		Waypoints:          wps,
	}

	conf, err := eval.EvaluateRoute(context.Background(), route, polar.Get36ftKetchPolar(), "gfs_0p25")
	if err != nil {
		t.Fatalf("EvaluateRoute failed: %v", err)
	}

	if conf.NumMembers != 31 {
		t.Errorf("expected 31 members for gfs_0p25 mapped to GEFS, got %d", conf.NumMembers)
	}

	if conf.EnsembleComparison == nil {
		t.Fatalf("expected EnsembleComparison to be populated")
	}

	if conf.EnsembleComparison.MinDurationHours >= conf.EnsembleComparison.MaxDurationHours {
		t.Errorf("expected distinct spread: min=%f, max=%f", conf.EnsembleComparison.MinDurationHours, conf.EnsembleComparison.MaxDurationHours)
	}

	t.Logf("Evaluated GEFS comparison: Mean=%.1fh, Min=%.1fh, Max=%.1fh, IQR=%.1fh, ScoreA=%.1f%%, ScoreB=%.1f%%, Agreement=%.1f%%",
		conf.EnsembleComparison.MeanDurationHours,
		conf.EnsembleComparison.MinDurationHours,
		conf.EnsembleComparison.MaxDurationHours,
		conf.EnsembleComparison.IQRDurationHours,
		conf.ScoreStrategyA,
		conf.ScoreStrategyB,
		conf.AgreementScore,
	)
}

func TestTrajectoryLandCollisionAvoidance(t *testing.T) {
	lm := landmask.NewGSHHGLandMask()
	eval := NewEvaluator("", nil, lm)

	startTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// Kattegat route near Denmark / Sweden
	wps := []isochrone.Waypoint{
		{Lat: 57.5, Lon: 11.0, Time: startTime, HeadingDeg: 120.0, BoatSpeedKts: 7.0, TWSKts: 15.0, TWDDeg: 240.0, DistanceNM: 0.0},
		{Lat: 57.2, Lon: 11.5, Time: startTime.Add(3 * time.Hour), HeadingDeg: 120.0, BoatSpeedKts: 7.0, TWSKts: 15.0, TWDDeg: 240.0, DistanceNM: 25.0},
		{Lat: 56.8, Lon: 11.9, Time: startTime.Add(6 * time.Hour), HeadingDeg: 120.0, BoatSpeedKts: 7.0, TWSKts: 15.0, TWDDeg: 240.0, DistanceNM: 50.0},
	}
	route := &isochrone.RouteResult{
		BoatName:           "Test Cruiser",
		StartTime:          startTime,
		ArrivalTime:        startTime.Add(6 * time.Hour),
		TotalDurationHours: 6.0,
		TotalDistanceNM:    50.0,
		Waypoints:          wps,
	}

	conf, err := eval.EvaluateRoute(context.Background(), route, polar.Get36ftKetchPolar(), "gfs_0p25")
	if err != nil {
		t.Fatalf("EvaluateRoute failed: %v", err)
	}

	if conf.EnsembleComparison == nil || len(conf.EnsembleComparison.Members) == 0 {
		t.Fatalf("expected populated ensemble members")
	}

	for _, m := range conf.EnsembleComparison.Members {
		for ptIdx, pt := range m.Trajectory {
			if lm.IsLand(pt) {
				t.Errorf("Member %d waypoint %d (lat=%f, lon=%f) is on land!", m.MemberID, ptIdx, pt.Lat, pt.Lon)
			}
		}
	}
}

func TestMultiIsochroneEnsembleSolve(t *testing.T) {
	lm := landmask.NewGSHHGLandMask()
	eval := NewEvaluator("", nil, lm)

	startTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	start := geo.Point{Lat: 20.0, Lon: -60.0}
	dest := geo.Point{Lat: 22.0, Lon: -57.0}

	timestamps := []time.Time{startTime, startTime.Add(6 * time.Hour), startTime.Add(12 * time.Hour), startTime.Add(24 * time.Hour)}
	baseGrid := weather.NewWeatherGrid(18.0, 24.0, 1.0, -62.0, -55.0, 1.0, timestamps)
	for tIdx := range timestamps {
		for i := range baseGrid.UData[tIdx] {
			for j := range baseGrid.UData[tIdx][i] {
				baseGrid.UData[tIdx][i][j] = -7.0 // easterly trade wind
				baseGrid.VData[tIdx][i][j] = -2.0
			}
		}
	}

	cfg := isochrone.DefaultRouterConfig()
	cfg.TimeStep = 30 * time.Minute

	primaryRoute, err := isochrone.CalculateOptimalRoute(
		start,
		dest,
		startTime,
		polar.Get36ftKetchPolar(),
		weather.NewMemberWeatherEngine(0, baseGrid),
		lm,
		cfg,
	)
	if err != nil {
		t.Fatalf("Primary route calculation failed: %v", err)
	}

	conf, err := eval.EvaluateRouteMultiIsochrone(
		context.Background(),
		primaryRoute,
		start,
		dest,
		polar.Get36ftKetchPolar(),
		baseGrid,
		"gefs_0p50",
		cfg,
	)
	if err != nil {
		t.Fatalf("EvaluateRouteMultiIsochrone failed: %v", err)
	}

	if conf.EnsembleComparison == nil {
		t.Fatalf("Expected EnsembleComparison to be populated")
	}

	if len(conf.EnsembleComparison.Members) != 31 {
		t.Fatalf("Expected 31 solved member routes, got %d", len(conf.EnsembleComparison.Members))
	}

	for _, m := range conf.EnsembleComparison.Members {
		if m.TotalDurationHours <= 0 {
			t.Errorf("Member %d duration invalid: %f", m.MemberID, m.TotalDurationHours)
		}
		if len(m.Trajectory) == 0 {
			t.Errorf("Member %d has empty trajectory", m.MemberID)
		}
		if len(m.Waypoints) == 0 {
			t.Errorf("Member %d has empty waypoints", m.MemberID)
		}
	}

	t.Logf("Multi-isochrone solve successful! 31 members: Fastest=%.1fh (M#%d), Slowest=%.1fh (M#%d), IQR=%.1fh, Strategy B Score=%.1f%%",
		conf.EnsembleComparison.MinDurationHours,
		conf.EnsembleComparison.FastestMemberID,
		conf.EnsembleComparison.MaxDurationHours,
		conf.EnsembleComparison.SlowestMemberID,
		conf.EnsembleComparison.IQRDurationHours,
		conf.ScoreStrategyB,
	)
}
