package isochrone

import (
	"testing"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

func TestPolarNoGoZone(t *testing.T) {
	polarTable := polar.Get36ftKetchPolar()

	// 1. Head to wind (TWA = 0°) -> 0 knots
	spd0 := polarTable.InterpolateSpeed(16.0, 0.0)
	if spd0 != 0.0 {
		t.Fatalf("Expected 0.0 kts at TWA 0°, got %.2f kts", spd0)
	}

	// 2. In-irons (TWA = 15°) -> 0 knots
	spd15 := polarTable.InterpolateSpeed(16.0, 15.0)
	if spd15 != 0.0 {
		t.Fatalf("Expected 0.0 kts at TWA 15°, got %.2f kts", spd15)
	}

	// 3. Close-hauled upwind (TWA = 45°) -> ~6.4-6.6 knots
	spd45 := polarTable.InterpolateSpeed(16.0, 45.0)
	if spd45 < 5.5 || spd45 > 7.5 {
		t.Fatalf("Expected ~6.5 kts at TWA 45°, got %.2f kts", spd45)
	}
}

// ConstantNorthWindProvider simulates a constant 16-knot wind blowing from True North (000°).
type ConstantNorthWindProvider struct{}

func (c *ConstantNorthWindProvider) GetWind(lat, lon float64, t time.Time) weather.WindCondition {
	return weather.WindCondition{
		TWS: 16.0,
		TWD: 0.0, // Blowing from North (0 deg)
		U:   0.0,
		V:   -16.0 * weather.KnotsToMS,
	}
}

func (c *ConstantNorthWindProvider) GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep float64, t time.Time) [][]weather.WindCondition {
	return nil
}

func TestPureUpwindBeating(t *testing.T) {
	// Start at Equator (0,0), Destination directly North (1.5, 0) into a headwind
	start := geo.Point{Lat: 0.0, Lon: 0.0}
	dest := geo.Point{Lat: 1.5, Lon: 0.0} // ~90 NM straight North

	startTime := time.Now().UTC()
	polarTable := polar.Get36ftKetchPolar()
	weatherProvider := &ConstantNorthWindProvider{}

	cfg := DefaultRouterConfig()
	cfg.TimeStep = 1 * time.Hour

	route, err := CalculateOptimalRoute(
		start,
		dest,
		startTime,
		polarTable,
		weatherProvider,
		nil, // no land in open ocean
		cfg,
	)

	if err != nil {
		t.Fatalf("Upwind routing calculation failed: %v", err)
	}

	if !route.DestinationReached {
		t.Fatalf("Expected destination to be reached via upwind tacking")
	}

	// Verify that NO waypoint sails into the no-go zone (TWA < 28°)
	for i, wp := range route.Waypoints {
		if wp.TWADeg < 28.0 && wp.BoatSpeedKts > 0.5 {
			t.Fatalf("Waypoint %d violated no-go zone: TWA=%.1f° with speed=%.2f kts", i, wp.TWADeg, wp.BoatSpeedKts)
		}
	}

	// Verify that the route tacked (total distance > direct distance due to zig-zag beating)
	directDist := geo.DistanceNM(start, dest)
	if route.TotalDistanceNM <= directDist {
		t.Fatalf("Upwind tacking distance (%.1f NM) must exceed direct distance (%.1f NM)", route.TotalDistanceNM, directDist)
	}
}

func TestIsochroneRoutingNewportToBermuda(t *testing.T) {
	// Newport, RI (Castle Hill / Brenton Reef departure) to Bermuda (St. David's Approach)
	newport := geo.Point{Lat: 41.40, Lon: -71.35}
	bermuda := geo.Point{Lat: 32.40, Lon: -64.55}

	startTime := time.Now().UTC()
	polarTable := polar.Get36ftKetchPolar()
	weatherEngine := weather.NewRealisticGFSEngine(startTime)
	landMask := landmask.NewGSHHGLandMask()

	cfg := DefaultRouterConfig()
	cfg.TimeStep = 3 * time.Hour

	route, err := CalculateOptimalRoute(
		newport,
		bermuda,
		startTime,
		polarTable,
		weatherEngine,
		landMask,
		cfg,
	)

	if err != nil {
		t.Fatalf("Routing calculation failed: %v", err)
	}

	if len(route.Waypoints) < 10 {
		t.Fatalf("Expected at least 10 waypoints, got %d", len(route.Waypoints))
	}

	if route.TotalDistanceNM < 600.0 || route.TotalDistanceNM > 850.0 {
		t.Fatalf("Expected total distance ~630-800 NM, got %.2f NM", route.TotalDistanceNM)
	}

	// For a 36ft cruising ketch (avg speed 6.0 - 7.5 kts), Newport to Bermuda is ~85 to 110 hours
	if route.TotalDurationHours < 60.0 || route.TotalDurationHours > 140.0 {
		t.Fatalf("Expected passage time ~60-140 hours, got %.1f hours", route.TotalDurationHours)
	}

	for _, wp := range route.Waypoints {
		if wp.TWADeg < 25.0 && wp.BoatSpeedKts > 0.5 {
			t.Fatalf("Found invalid waypoint sailing inside no-go zone: TWA=%.1f°, Spd=%.1f kts", wp.TWADeg, wp.BoatSpeedKts)
		}
	}
}

func TestSanFranciscoToHawaii(t *testing.T) {
	// San Francisco to Honolulu
	sf := geo.Point{Lat: 37.75, Lon: -122.60}
	honolulu := geo.Point{Lat: 21.25, Lon: -157.60}

	startTime := time.Now().UTC()
	polarTable := polar.Get36ftKetchPolar()
	weatherEngine := weather.NewRealisticGFSEngine(startTime)
	landMask := landmask.NewGSHHGLandMask()

	cfg := DefaultRouterConfig()
	cfg.TimeStep = 4 * time.Hour

	route, err := CalculateOptimalRoute(
		sf,
		honolulu,
		startTime,
		polarTable,
		weatherEngine,
		landMask,
		cfg,
	)

	if err != nil {
		t.Fatalf("SF to Hawaii routing calculation failed: %v", err)
	}

	// Verify all waypoints obey the no-go zone
	for _, wp := range route.Waypoints {
		if wp.TWADeg < 25.0 && wp.BoatSpeedKts > 0.5 {
			t.Fatalf("SF-Hawaii waypoint violated no-go zone: TWA=%.1f°, Spd=%.1f kts", wp.TWADeg, wp.BoatSpeedKts)
		}
	}

	// Transpac distance is ~2,050 - 2,500 NM (Great Circle ~2,073 NM)
	if route.TotalDistanceNM < 2000.0 || route.TotalDistanceNM > 2900.0 {
		t.Fatalf("Expected SF-Hawaii distance ~2000-2900 NM, got %.1f NM", route.TotalDistanceNM)
	}
}

func TestTackPenaltyManeuvers(t *testing.T) {
	// DetectManeuver test
	twd := 0.0 // North wind (000°)

	// Port tack close-hauled (045°) to Starboard tack close-hauled (315°): TACK through 000°
	m1 := DetectManeuver(45.0, 315.0, twd)
	if m1 != "tack" {
		t.Fatalf("Expected tack through eye of wind, got %s", m1)
	}

	// Port broad reach (135°) to Starboard broad reach (225°): GYBE through 180°
	m2 := DetectManeuver(135.0, 225.0, twd)
	if m2 != "gybe" {
		t.Fatalf("Expected gybe through stern, got %s", m2)
	}
}

func TestPricklyBayToChaguaramas(t *testing.T) {
	// Prickly Bay Grenada to Chaguaramas Trinidad
	start := geo.Point{Lat: 11.975, Lon: -61.765}
	dest := geo.Point{Lat: 10.675, Lon: -61.645}

	startTime := time.Now().UTC()
	polarTable := polar.Get36ftKetchPolar()
	weatherEngine := weather.NewRealisticGFSEngine(startTime)
	landMask := landmask.NewGSHHGLandMask()

	cfg := DefaultRouterConfig()
	cfg.TimeStep = 5 * time.Minute // 5-minute isochrones

	t0 := time.Now()
	route, err := CalculateOptimalRoute(
		start,
		dest,
		startTime,
		polarTable,
		weatherEngine,
		landMask,
		cfg,
	)
	dur := time.Since(t0)
	t.Logf("Calculated route in %v (Waypoints: %d, Distance: %.2f NM, Time: %.2f h, Reached: %v)",
		dur, len(route.Waypoints), route.TotalDistanceNM, route.TotalDurationHours, route.DestinationReached)

	for i := 0; i < len(route.Waypoints); i += 20 {
		wp := route.Waypoints[i]
		t.Logf("Step %3d (Time: %6.2fh): Lat %.4f, Lon %.4f, DistToDest: %.2f NM, Spd: %.2f kts",
			i, wp.Time.Sub(startTime).Hours(), wp.Lat, wp.Lon, wp.DistanceToDestNM, wp.BoatSpeedKts)
	}
	if len(route.Waypoints) > 0 {
		last := route.Waypoints[len(route.Waypoints)-1]
		t.Logf("FINAL WP: Lat %.4f, Lon %.4f, DistToDest: %.2f NM, Spd: %.2f kts",
			last.Lat, last.Lon, last.DistanceToDestNM, last.BoatSpeedKts)
	}

	if err != nil {
		t.Fatalf("Routing calculation failed: %v", err)
	}
	if !route.DestinationReached {
		t.Fatalf("Expected destination to be reached")
	}
}

func TestCowesToFastnetRock(t *testing.T) {
	// Cowes (Isle of Wight) to Fastnet Rock (Ireland)
	start := geo.Point{Lat: 50.76, Lon: -1.20}
	dest := geo.Point{Lat: 51.39, Lon: -9.60}

	startTime := time.Now().UTC()
	polarTable := polar.Get36ftKetchPolar()
	weatherEngine := weather.NewRealisticGFSEngine(startTime)
	landMask := landmask.NewGSHHGLandMask()

	cfg := DefaultRouterConfig()
	cfg.TimeStep = 30 * time.Minute // 30-min isochrones for 320 NM offshore passage

	t0 := time.Now()
	route, err := CalculateOptimalRoute(
		start,
		dest,
		startTime,
		polarTable,
		weatherEngine,
		landMask,
		cfg,
	)
	dur := time.Since(t0)
	t.Logf("Cowes -> Fastnet calculated in %v (Waypoints: %d, Distance: %.2f NM, Time: %.2f h, Reached: %v)",
		dur, len(route.Waypoints), route.TotalDistanceNM, route.TotalDurationHours, route.DestinationReached)

	if len(route.Waypoints) > 0 {
		for i := 0; i < len(route.Waypoints); i += 10 {
			wp := route.Waypoints[i]
			t.Logf("WP %3d (Time %5.1fh): Lat %.4f, Lon %.4f, DistToDest: %.1f NM",
				i, wp.Time.Sub(startTime).Hours(), wp.Lat, wp.Lon, wp.DistanceToDestNM)
		}
		last := route.Waypoints[len(route.Waypoints)-1]
		t.Logf("FINAL WP: Lat %.4f, Lon %.4f, DistToDest: %.2f NM", last.Lat, last.Lon, last.DistanceToDestNM)
	}

	if err != nil {
		t.Fatalf("Routing calculation failed: %v", err)
	}
	if !route.DestinationReached {
		t.Fatalf("Expected Fastnet Rock to be reached, but router got trapped")
	}
}

func TestPruningStrategiesComparison(t *testing.T) {
	start := geo.Point{Lat: 41.40, Lon: -71.35} // Newport
	dest := geo.Point{Lat: 32.40, Lon: -64.55}  // Bermuda

	startTime := time.Now().UTC()
	polarTable := polar.Get36ftKetchPolar()
	weatherEngine := weather.NewRealisticGFSEngine(startTime)
	landMask := landmask.NewGSHHGLandMask()

	strategies := []struct {
		name     string
		strategy PruningStrategy
	}{
		{"RadialSector", PruningRadialSector},
		{"SpatialGrid", PruningSpatialGrid},
		{"AStarBeam", PruningAStarBeam},
		{"ParetoEnvelope", PruningParetoEnvelope},
		{"StateSpaceGrid", PruningStateSpaceGrid},
	}

	for _, tc := range strategies {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultRouterConfig()
			cfg.TimeStep = 2 * time.Hour
			cfg.PruningStrategy = tc.strategy

			t0 := time.Now()
			route, err := CalculateOptimalRoute(
				start,
				dest,
				startTime,
				polarTable,
				weatherEngine,
				landMask,
				cfg,
			)
			dur := time.Since(t0)

			if err != nil {
				t.Fatalf("Strategy %s failed: %v", tc.name, err)
			}
			if !route.DestinationReached {
				t.Fatalf("Strategy %s did not reach destination", tc.name)
			}

			t.Logf("[%s] Solved in %v: Route %.1f NM, Duration %.1f h, %d waypoints, %d isochrones",
				tc.name, dur, route.TotalDistanceNM, route.TotalDurationHours, len(route.Waypoints), len(route.Isochrones))
		})
	}
}


