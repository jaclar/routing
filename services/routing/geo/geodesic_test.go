package geo

import (
	"math"
	"testing"
)

func TestDistanceAndBearing(t *testing.T) {
	// Newport, RI (41.4901, -71.3128) to Hamilton, Bermuda (32.2949, -64.7830)
	newport := Point{Lat: 41.4901, Lon: -71.3128}
	bermuda := Point{Lat: 32.2949, Lon: -64.7830}

	distNM := DistanceNM(newport, bermuda)
	// Known Newport to Bermuda distance is ~635-645 nautical miles
	if distNM < 630.0 || distNM > 650.0 {
		t.Fatalf("Expected Newport-Bermuda distance ~635-645 NM, got %.2f NM", distNM)
	}

	bearing := InitialBearing(newport, bermuda)
	// Initial bearing is ~145-155 degrees (SSE)
	if bearing < 140.0 || bearing > 160.0 {
		t.Fatalf("Expected bearing ~145-155 deg, got %.2f deg", bearing)
	}

	// Test DestinationPoint
	dest := DestinationPoint(newport, distNM*NMToMeters, bearing)
	if math.Abs(dest.Lat-bermuda.Lat) > 0.05 || math.Abs(dest.Lon-bermuda.Lon) > 0.05 {
		t.Fatalf("Destination point mismatch: expected (%f, %f), got (%f, %f)",
			bermuda.Lat, bermuda.Lon, dest.Lat, dest.Lon)
	}
}

func TestInterpolateGreatCircle(t *testing.T) {
	p1 := Point{Lat: 0.0, Lon: 0.0}
	p2 := Point{Lat: 0.0, Lon: 10.0}

	mid := InterpolateGreatCircle(p1, p2, 0.5)
	if math.Abs(mid.Lat-0.0) > 1e-4 || math.Abs(mid.Lon-5.0) > 1e-4 {
		t.Fatalf("Expected midpoint (0, 5), got (%f, %f)", mid.Lat, mid.Lon)
	}
}
