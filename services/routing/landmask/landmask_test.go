package landmask

import (
	"testing"

	"github.com/jaclar/routing-service/geo"
)

func TestLandMaskDetection(t *testing.T) {
	lm := NewGSHHGLandMask()

	// Land point: New York City (40.71, -74.00)
	nyc := geo.Point{Lat: 40.71, Lon: -74.00}
	if !lm.IsLand(nyc) {
		t.Fatalf("Expected NYC (%v) to be detected as land", nyc)
	}

	// Ocean point: Atlantic ocean between Newport and Bermuda (37.0, -68.0)
	oceanPt := geo.Point{Lat: 37.0, Lon: -68.0}
	if lm.IsLand(oceanPt) {
		t.Fatalf("Expected ocean point (%v) to NOT be detected as land", oceanPt)
	}

	// Segment through Cape Cod vs open water
	inland := geo.Point{Lat: 42.0, Lon: -72.0} // Massachusetts inland
	if !lm.SegmentIntersectsLand(nyc, inland, 10) {
		t.Fatalf("Expected segment across Massachusetts to intersect land")
	}
}
