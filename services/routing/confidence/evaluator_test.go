package confidence

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jaclar/routing-service/isochrone"
	"github.com/jaclar/routing-service/polar"
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
	eval := NewEvaluator("", nil)

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
	eval := NewEvaluator("", nil)

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
