package landmask

import (
	"github.com/jaclar/routing-service/geo"
)

// LandMask provides high-speed collision checking against global landmasses (based on GSHHG boundaries).
type LandMask struct {
	polygons []Polygon
}

// Polygon represents a closed geographic boundary (e.g. continent or island).
type Polygon struct {
	Name     string
	MinLat   float64
	MaxLat   float64
	MinLon   float64
	MaxLon   float64
	Vertices []geo.Point
}

// NewGSHHGLandMask creates a LandMask initialized with major continental and coastal boundaries.
func NewGSHHGLandMask() *LandMask {
	lm := &LandMask{}
	lm.initializeStandardLandmasses()
	return lm
}

func (lm *LandMask) initializeStandardLandmasses() {
	// North America Main (including US East Coast, Cape Cod, Florida, Gulf of Mexico)
	lm.addPolygon("North America", []geo.Point{
		{Lat: 15.0, Lon: -90.0},
		{Lat: 25.0, Lon: -80.5},
		{Lat: 25.2, Lon: -80.2}, // Florida Keys
		{Lat: 28.5, Lon: -80.5}, // Cape Canaveral
		{Lat: 32.0, Lon: -80.8}, // Georgia
		{Lat: 34.0, Lon: -77.9}, // Cape Fear
		{Lat: 35.2, Lon: -75.5}, // Cape Hatteras
		{Lat: 37.0, Lon: -75.9}, // Chesapeake
		{Lat: 39.0, Lon: -74.8}, // Cape May
		{Lat: 40.5, Lon: -74.0}, // New York Harbor
		{Lat: 41.1, Lon: -73.5}, // CT Coast
		{Lat: 41.3, Lon: -72.4}, // Connecticut River
		{Lat: 41.6, Lon: -71.4}, // Narragansett Bay head (Providence)
		{Lat: 41.7, Lon: -70.6}, // Buzzards Bay head
		{Lat: 42.05, Lon: -70.18}, // Provincetown / Cape Cod Tip
		{Lat: 41.65, Lon: -69.95}, // Chatham, Cape Cod
		{Lat: 42.4, Lon: -70.9}, // Boston
		{Lat: 44.0, Lon: -69.0}, // Maine
		{Lat: 45.0, Lon: -66.0}, // Bay of Fundy
		{Lat: 44.5, Lon: -63.5}, // Halifax, NS
		{Lat: 46.0, Lon: -60.0}, // Cape Breton
		{Lat: 50.0, Lon: -55.0}, // Newfoundland
		{Lat: 60.0, Lon: -65.0}, // Labrador
		{Lat: 72.0, Lon: -120.0},
		{Lat: 60.0, Lon: -140.0}, // Alaska
		{Lat: 48.0, Lon: -124.7}, // Washington
		{Lat: 37.8, Lon: -122.5}, // San Francisco
		{Lat: 32.7, Lon: -117.2}, // San Diego
		{Lat: 22.9, Lon: -109.9}, // Cabo San Lucas
		{Lat: 15.0, Lon: -95.0},
	})

	// Europe & British Isles
	lm.addPolygon("Europe", []geo.Point{
		{Lat: 36.0, Lon: -5.5}, // Gibraltar
		{Lat: 37.0, Lon: -8.9}, // Cape St Vincent, Portugal
		{Lat: 43.4, Lon: -8.4}, // A Coruña, Spain
		{Lat: 43.4, Lon: -1.7}, // Bay of Biscay
		{Lat: 48.0, Lon: -4.5}, // Brittany, France
		{Lat: 51.0, Lon: 1.5},  // English Channel
		{Lat: 53.5, Lon: 8.0},  // Germany
		{Lat: 58.0, Lon: 8.0},  // Norway
		{Lat: 71.0, Lon: 28.0}, // North Cape
		{Lat: 60.0, Lon: 30.0},
		{Lat: 40.0, Lon: 25.0}, // Greece
		{Lat: 38.0, Lon: 15.0}, // Italy
		{Lat: 43.0, Lon: 5.0},  // French Riviera
		{Lat: 36.0, Lon: -5.5},
	})

	// Great Britain & Ireland
	lm.addPolygon("Great Britain & Ireland", []geo.Point{
		{Lat: 50.0, Lon: -5.7}, // Lands End
		{Lat: 50.5, Lon: -2.0}, // Isle of Wight / Solent
		{Lat: 51.3, Lon: 1.4},  // Dover
		{Lat: 55.0, Lon: -1.5}, // Newcastle
		{Lat: 58.6, Lon: -3.0}, // John o Groats
		{Lat: 56.5, Lon: -6.0}, // Hebrides
		{Lat: 51.5, Lon: -9.5}, // Fastnet / SW Ireland
		{Lat: 53.0, Lon: -10.0},
		{Lat: 55.5, Lon: -7.5}, // North Ireland
		{Lat: 52.0, Lon: -5.0}, // Wales
		{Lat: 50.0, Lon: -5.7},
	})

	// Cuba & Caribbean Islands
	lm.addPolygon("Cuba", []geo.Point{
		{Lat: 21.8, Lon: -84.9},
		{Lat: 23.2, Lon: -80.5},
		{Lat: 20.0, Lon: -74.2},
		{Lat: 19.8, Lon: -77.5},
		{Lat: 21.8, Lon: -84.9},
	})

	// South America
	lm.addPolygon("South America", []geo.Point{
		{Lat: 11.5, Lon: -73.0},
		{Lat: 5.0, Lon: -52.0},
		{Lat: -5.5, Lon: -35.0}, // Recife
		{Lat: -23.0, Lon: -43.0}, // Rio
		{Lat: -35.0, Lon: -55.0}, // Rio de la Plata
		{Lat: -55.0, Lon: -66.0}, // Cape Horn
		{Lat: -40.0, Lon: -74.0}, // Chile
		{Lat: -5.0, Lon: -81.0},  // Peru
		{Lat: 9.0, Lon: -79.5},   // Panama
		{Lat: 11.5, Lon: -73.0},
	})

	// Bermuda Islands (Narrow island hook)
	lm.addPolygon("Bermuda Landmass", []geo.Point{
		{Lat: 32.25, Lon: -64.88},
		{Lat: 32.36, Lon: -64.68},
		{Lat: 32.38, Lon: -64.65},
		{Lat: 32.33, Lon: -64.67},
		{Lat: 32.24, Lon: -64.86},
		{Lat: 32.25, Lon: -64.88},
	})
}

func (lm *LandMask) addPolygon(name string, vertices []geo.Point) {
	if len(vertices) < 3 {
		return
	}
	minLat, maxLat := 90.0, -90.0
	minLon, maxLon := 180.0, -180.0

	for _, v := range vertices {
		if v.Lat < minLat {
			minLat = v.Lat
		}
		if v.Lat > maxLat {
			maxLat = v.Lat
		}
		if v.Lon < minLon {
			minLon = v.Lon
		}
		if v.Lon > maxLon {
			maxLon = v.Lon
		}
	}

	lm.polygons = append(lm.polygons, Polygon{
		Name:     name,
		MinLat:   minLat,
		MaxLat:   maxLat,
		MinLon:   minLon,
		MaxLon:   maxLon,
		Vertices: vertices,
	})
}

// IsLand checks if a point (lat, lon) is inside any land polygon using the Ray-Casting algorithm.
func (lm *LandMask) IsLand(p geo.Point) bool {
	for i := range lm.polygons {
		poly := &lm.polygons[i]
		// Bounding box pre-check
		if p.Lat < poly.MinLat || p.Lat > poly.MaxLat || p.Lon < poly.MinLon || p.Lon > poly.MaxLon {
			continue
		}
		if pointInPolygon(p, poly.Vertices) {
			return true
		}
	}
	return false
}

// SegmentIntersectsLand samples along the path between p1 and p2 to verify no land is crossed.
func (lm *LandMask) SegmentIntersectsLand(p1, p2 geo.Point, samples int) bool {
	if samples < 2 {
		samples = 5
	}
	for i := 0; i <= samples; i++ {
		f := float64(i) / float64(samples)
		pt := geo.InterpolateGreatCircle(p1, p2, f)
		if lm.IsLand(pt) {
			return true
		}
	}
	return false
}

func pointInPolygon(point geo.Point, vertices []geo.Point) bool {
	n := len(vertices)
	inside := false
	x := point.Lon
	y := point.Lat

	j := n - 1
	for i := 0; i < n; i++ {
		xi := vertices[i].Lon
		yi := vertices[i].Lat
		xj := vertices[j].Lon
		yj := vertices[j].Lat

		intersect := ((yi > y) != (yj > y)) &&
			(x < (xj-xi)*(y-yi)/(yj-yi)+xi)
		if intersect {
			inside = !inside
		}
		j = i
	}
	return inside
}
