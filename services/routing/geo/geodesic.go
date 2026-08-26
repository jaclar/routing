package geo

import (
	"math"
)

const (
	EarthRadiusMeters = 6371000.0 // Mean Earth radius in meters
	MetersToNM        = 1.0 / 1852.0
	NMToMeters        = 1852.0
)

// Point represents a geographic coordinate in decimal degrees.
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func DegToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}

func RadToDeg(rad float64) float64 {
	return rad * 180.0 / math.Pi
}

// NormalizeAngle360 normalizes an angle in degrees to [0, 360).
func NormalizeAngle360(deg float64) float64 {
	d := math.Mod(deg, 360.0)
	if d < 0 {
		d += 360.0
	}
	return d
}

// NormalizeLon normalizes longitude to [-180, 180).
func NormalizeLon(lon float64) float64 {
	l := math.Mod(lon+180.0, 360.0)
	if l < 0 {
		l += 360.0
	}
	return l - 180.0
}

// DistanceMeters calculates the great-circle distance between two points using the Haversine formula.
func DistanceMeters(p1, p2 Point) float64 {
	lat1Rad := DegToRad(p1.Lat)
	lon1Rad := DegToRad(p1.Lon)
	lat2Rad := DegToRad(p2.Lat)
	lon2Rad := DegToRad(p2.Lon)

	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	a := math.Sin(dLat/2.0)*math.Sin(dLat/2.0) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLon/2.0)*math.Sin(dLon/2.0)
	c := 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))

	return EarthRadiusMeters * c
}

// DistanceNM calculates distance in nautical miles.
func DistanceNM(p1, p2 Point) float64 {
	return DistanceMeters(p1, p2) * MetersToNM
}

// InitialBearing calculates the initial forward bearing (azimuth) from p1 to p2 in degrees [0, 360).
func InitialBearing(p1, p2 Point) float64 {
	lat1Rad := DegToRad(p1.Lat)
	lat2Rad := DegToRad(p2.Lat)
	dLonRad := DegToRad(p2.Lon - p1.Lon)

	y := math.Sin(dLonRad) * math.Cos(lat2Rad)
	x := math.Cos(lat1Rad)*math.Sin(lat2Rad) -
		math.Sin(lat1Rad)*math.Cos(lat2Rad)*math.Cos(dLonRad)

	bearingRad := math.Atan2(y, x)
	return NormalizeAngle360(RadToDeg(bearingRad))
}

// DestinationPoint calculates the destination Point reached after traveling a given distance along a bearing.
func DestinationPoint(start Point, distanceMeters, bearingDeg float64) Point {
	distRatio := distanceMeters / EarthRadiusMeters
	bearingRad := DegToRad(bearingDeg)

	lat1Rad := DegToRad(start.Lat)
	lon1Rad := DegToRad(start.Lon)

	lat2Rad := math.Asin(
		math.Sin(lat1Rad)*math.Cos(distRatio) +
			math.Cos(lat1Rad)*math.Sin(distRatio)*math.Cos(bearingRad),
	)

	lon2Rad := lon1Rad + math.Atan2(
		math.Sin(bearingRad)*math.Sin(distRatio)*math.Cos(lat1Rad),
		math.Cos(distRatio)-math.Sin(lat1Rad)*math.Sin(lat2Rad),
	)

	return Point{
		Lat: RadToDeg(lat2Rad),
		Lon: NormalizeLon(RadToDeg(lon2Rad)),
	}
}

// InterpolateGreatCircle calculates an intermediate point at fraction f (0.0 to 1.0) along the great circle.
func InterpolateGreatCircle(p1, p2 Point, f float64) Point {
	if f <= 0.0 {
		return p1
	}
	if f >= 1.0 {
		return p2
	}

	d := DistanceMeters(p1, p2) / EarthRadiusMeters
	if d < 1e-12 {
		return p1
	}

	lat1 := DegToRad(p1.Lat)
	lon1 := DegToRad(p1.Lon)
	lat2 := DegToRad(p2.Lat)
	lon2 := DegToRad(p2.Lon)

	a := math.Sin((1.0-f)*d) / math.Sin(d)
	b := math.Sin(f*d) / math.Sin(d)

	x := a*math.Cos(lat1)*math.Cos(lon1) + b*math.Cos(lat2)*math.Cos(lon2)
	y := a*math.Cos(lat1)*math.Sin(lon1) + b*math.Cos(lat2)*math.Sin(lon2)
	z := a*math.Sin(lat1) + b*math.Sin(lat2)

	lat3 := math.Atan2(z, math.Sqrt(x*x+y*y))
	lon3 := math.Atan2(y, x)

	return Point{
		Lat: RadToDeg(lat3),
		Lon: NormalizeLon(RadToDeg(lon3)),
	}
}

// CrossTrackDistanceMeters calculates the perpendicular distance of a point p from a great circle path from start to end.
func CrossTrackDistanceMeters(p, start, end Point) float64 {
	d13 := DistanceMeters(start, p) / EarthRadiusMeters
	b13 := DegToRad(InitialBearing(start, p))
	b12 := DegToRad(InitialBearing(start, end))

	dXt := math.Asin(math.Sin(d13) * math.Sin(b13-b12))
	return math.Abs(dXt * EarthRadiusMeters)
}
