package landmask

import (
	"testing"

	"github.com/jaclar/routing-service/geo"
)

func TestLandMaskDetection(t *testing.T) {
	lm := NewGSHHGLandMask()

	// Land point: Manhattan NYC (40.78, -73.97)
	nyc := geo.Point{Lat: 40.78, Lon: -73.97}
	if !lm.IsLand(nyc) {
		t.Fatalf("Expected Manhattan NYC (%v) to be detected as land", nyc)
	}

	// Land point: Boston (42.36, -71.06)
	boston := geo.Point{Lat: 42.36, Lon: -71.06}
	if !lm.IsLand(boston) {
		t.Fatalf("Expected Boston (%v) to be detected as land", boston)
	}

	// Land point: Cape Cod (41.70, -70.30)
	capeCod := geo.Point{Lat: 41.70, Lon: -70.30}
	if !lm.IsLand(capeCod) {
		t.Fatalf("Expected Cape Cod (%v) to be detected as land", capeCod)
	}

	// Ocean point: Atlantic ocean between Newport and Bermuda (37.0, -68.0)
	oceanPt := geo.Point{Lat: 37.0, Lon: -68.0}
	if lm.IsLand(oceanPt) {
		t.Fatalf("Expected ocean point (%v) to NOT be detected as land", oceanPt)
	}

	// Ocean point: Open water passage south of Grenada (11.35, -61.70)
	grenadaPassage := geo.Point{Lat: 11.35, Lon: -61.70}
	if lm.IsLand(grenadaPassage) {
		t.Fatalf("Expected Grenada passage (%v) to NOT be detected as land", grenadaPassage)
	}

	// Segment across Cape Cod vs open water
	inland := geo.Point{Lat: 42.0, Lon: -72.0} // Massachusetts inland
	if !lm.SegmentIntersectsLand(nyc, inland, 10) {
		t.Fatalf("Expected segment across Massachusetts to intersect land")
	}
}
