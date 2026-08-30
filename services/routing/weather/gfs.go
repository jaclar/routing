package weather

import (
	"math"
	"time"
)

// GFSWeatherEngine implements WeatherProvider using GFS forecast data or realistic hydrodynamic/meteorological simulation.
type GFSWeatherEngine struct {
	grid *WeatherGrid
}

// NewRealisticGFSEngine constructs a 4D GFS weather forecast grid covering the globe for a 16-day forecast window.
func NewRealisticGFSEngine(startTime time.Time) *GFSWeatherEngine {
	minLat := -85.0
	maxLat := 85.0
	latStep := 1.0 // 1 degree grid
	minLon := -180.0
	maxLon := 180.0
	lonStep := 1.0

	// 16 days forecast with 3-hour steps = 129 time slices
	nSteps := 129
	timestamps := make([]time.Time, nSteps)
	for i := 0; i < nSteps; i++ {
		timestamps[i] = startTime.Add(time.Duration(i*3) * time.Hour)
	}

	grid := NewWeatherGrid(minLat, maxLat, latStep, minLon, maxLon, lonStep, timestamps)

	nLat := len(grid.UData[0])
	nLon := len(grid.UData[0][0])

	for tIdx, t := range timestamps {
		hoursFromStart := t.Sub(startTime).Hours()

		// 1. Moving Cyclonic Lows (propagating eastwards at ~18 knots / ~7 deg lon per 24h)
		// Atlantic Low (propagates from Newfoundland towards UK/Biscay)
		atlLowLat := 50.0 + 3.0*math.Sin(hoursFromStart*math.Pi/48.0)
		atlLowLon := -55.0 + (hoursFromStart / 24.0 * 7.5)

		// Pacific Low (propagates from Gulf of Alaska towards Pacific NW / Canada)
		pacLowLat := 52.0 + 2.5*math.Cos(hoursFromStart*math.Pi/60.0)
		pacLowLon := -155.0 + (hoursFromStart / 24.0 * 6.5)

		// 2. Subtropical High Pressure Systems
		// Bermuda/Azores High in the North Atlantic
		atlHighLat := 32.0 + 1.0*math.Sin(hoursFromStart*math.Pi/96.0)
		atlHighLon := -42.0 + 1.5*math.Cos(hoursFromStart*math.Pi/72.0)

		// Eastern Pacific / Hawaiian Subtropical High (centered at ~36N, 140W)
		pacHighLat := 36.0 + 1.2*math.Cos(hoursFromStart*math.Pi/96.0)
		pacHighLon := -140.0 + 2.0*math.Sin(hoursFromStart*math.Pi/80.0)

		for i := 0; i < nLat; i++ {
			lat := minLat + float64(i)*latStep
			for j := 0; j < nLon; j++ {
				lon := minLon + float64(j)*lonStep

				// 1. Zonal planetary wind belts (Hadley & Ferrel cells)
				var uBase, vBase float64
				if lat >= 0.0 && lat <= 30.0 {
					// Northern Hemisphere Trade Winds: blowing towards WSW (negative U, negative V)
					// TWD approx 055° - 075° (Northeasterlies)
					tradeStrength := 9.5 + 2.0*math.Sin(lat*math.Pi/30.0)
					uBase = -tradeStrength * math.Sin(65.0*math.Pi/180.0) // negative U (westward)
					vBase = -tradeStrength * math.Cos(65.0*math.Pi/180.0) // negative V (southward)
				} else if lat > 30.0 && lat <= 38.0 {
					// Subtropical Highs / Transition zone: steady 12-16 knot southwesterly breeze
					transFrac := (lat - 30.0) / 8.0
					uBase = 4.0 + 4.0*transFrac
					vBase = 3.5 + 2.5*math.Sin((lon+hoursFromStart*1.5)*math.Pi/45.0)
				} else if lat > 38.0 && lat <= 65.0 {
					// Mid-latitude Westerlies: blowing towards ENE (positive U, positive V)
					// TWD approx 240° - 270° (Westerlies)
					westerliesFactor := math.Sin((lat - 38.0) / 27.0 * math.Pi)
					uBase = 9.0 + 8.5*westerliesFactor
					vBase = 3.0 + 2.5*math.Sin((lon+hoursFromStart*1.5)*math.Pi/45.0)
				} else if lat >= -30.0 && lat < 0.0 {
					// Southern Hemisphere Trade Winds: blowing towards WNW (negative U, positive V)
					tradeStrength := 8.0 + 2.0*math.Sin(math.Abs(lat)*math.Pi/30.0)
					uBase = -tradeStrength * math.Sin(60.0*math.Pi/180.0)
					vBase = tradeStrength * math.Cos(60.0*math.Pi/180.0)
				} else {
					// Southern Ocean Roaring 40s & 50s
					uBase = 12.0 + 5.0*math.Sin(math.Abs(lat)*math.Pi/30.0)
					vBase = -2.0
				}

				// 2. Cyclonic Low Pressure Systems (Counter-Clockwise in Northern Hemisphere)
				var uLow, vLow float64

				// Atlantic Low
				dLatAtlLow := lat - atlLowLat
				dLonAtlLow := lon - atlLowLon
				distAtlLow := math.Hypot(dLatAtlLow, dLonAtlLow)
				if distAtlLow < 25.0 {
					str := 14.0 * math.Exp(-(distAtlLow*distAtlLow)/80.0)
					ang := math.Atan2(dLatAtlLow, dLonAtlLow)
					uLow += -str * math.Sin(ang)
					vLow += str * math.Cos(ang)
				}

				// Pacific Low
				dLatPacLow := lat - pacLowLat
				dLonPacLow := lon - pacLowLon
				distPacLow := math.Hypot(dLatPacLow, dLonPacLow)
				if distPacLow < 25.0 {
					str := 14.0 * math.Exp(-(distPacLow*distPacLow)/80.0)
					ang := math.Atan2(dLatPacLow, dLonPacLow)
					uLow += -str * math.Sin(ang)
					vLow += str * math.Cos(ang)
				}

				// 3. Anticyclonic Subtropical High Pressure Systems (Clockwise in Northern Hemisphere)
				var uHigh, vHigh float64

				// Atlantic / Azores High
				dLatAtlHigh := lat - atlHighLat
				dLonAtlHigh := lon - atlHighLon
				distAtlHigh := math.Hypot(dLatAtlHigh, dLonAtlHigh)
				if distAtlHigh < 30.0 {
					str := 7.5 * math.Exp(-(distAtlHigh*distAtlHigh)/140.0)
					ang := math.Atan2(dLatAtlHigh, dLonAtlHigh)
					uHigh += str * math.Sin(ang)
					vHigh += -str * math.Cos(ang)
				}

				// Eastern Pacific High (Dominates the California Current & NE Pacific Trade Slot)
				dLatPacHigh := lat - pacHighLat
				dLonPacHigh := lon - pacHighLon
				distPacHigh := math.Hypot(dLatPacHigh, dLonPacHigh)
				if distPacHigh < 35.0 {
					str := 8.5 * math.Exp(-(distPacHigh*distPacHigh)/160.0)
					ang := math.Atan2(dLatPacHigh, dLonPacHigh)
					uHigh += str * math.Sin(ang)
					vHigh += -str * math.Cos(ang)
				}

				// Combine atmospheric components
				uTotal := uBase + uLow + uHigh
				vTotal := vBase + vLow + vHigh

				grid.UData[tIdx][i][j] = uTotal
				grid.VData[tIdx][i][j] = vTotal
			}
		}
	}

	return &GFSWeatherEngine{grid: grid}
}

func (e *GFSWeatherEngine) GetWind(lat, lon float64, t time.Time) WindCondition {
	return e.grid.Interpolate(lat, lon, t)
}

func (e *GFSWeatherEngine) GetGrid(minLat, maxLat, minLon, maxLon, latStep, lonStep float64, t time.Time) [][]WindCondition {
	nLat := int(math.Round((maxLat-minLat)/latStep)) + 1
	nLon := int(math.Round((maxLon-minLon)/lonStep)) + 1

	res := make([][]WindCondition, nLat)
	for i := 0; i < nLat; i++ {
		lat := minLat + float64(i)*latStep
		res[i] = make([]WindCondition, nLon)
		for j := 0; j < nLon; j++ {
			lon := minLon + float64(j)*lonStep
			res[i][j] = e.GetWind(lat, lon, t)
		}
	}
	return res
}
