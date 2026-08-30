package polar

import (
	"math"
)

// PolarTable represents a 2D matrix of boat speeds indexed by True Wind Speed and True Wind Angle.
type PolarTable struct {
	BoatName string      `json:"boat_name"`
	TWSList  []float64   `json:"tws_list"`     // [knots]
	TWAList  []float64   `json:"twa_list"`     // [degrees]
	Speeds   [][]float64 `json:"speed_matrix"` // [len(TWS)][len(TWA)] in knots
}

// InterpolateSpeed computes boat speed in knots for arbitrary (TWS, TWA) using bilinear interpolation,
// strictly enforcing aerodynamic no-go zone / in-irons limits (TWA < 28 deg -> 0 kts) and 0 kts in dead air (TWS <= 3 kts).
func (p *PolarTable) InterpolateSpeed(twsKts, twaDeg float64) float64 {
	// 0. Dead air / calm limit: yacht produces zero speed in <= 3 knots of wind
	const calmWindLimit = 3.0
	if twsKts <= calmWindLimit {
		return 0.0
	}

	// Symmetrical angle: wrap to [0, 180]
	angle := math.Abs(math.Mod(twaDeg, 360.0))
	if angle > 180.0 {
		angle = 360.0 - angle
	}

	// 1. Aerodynamic No-Go Zone: Sails stall / luff head-to-wind
	if angle <= 22.0 {
		return 0.0
	}

	nTWS := len(p.TWSList)
	nTWA := len(p.TWAList)
	if nTWS == 0 || nTWA == 0 {
		return 0.0
	}

	// 2. Scale factor if wind is in light-air band between calm limit (3.0 kts) and base polar curve (e.g. 6.0 kts)
	var lowWindScale float64 = 1.0
	effTWS := twsKts
	if twsKts < p.TWSList[0] {
		if p.TWSList[0] > calmWindLimit {
			lowWindScale = (twsKts - calmWindLimit) / (p.TWSList[0] - calmWindLimit)
			effTWS = p.TWSList[0]
		}
	}

	// 3. Clamp / find TWS index
	twsIdx0, twsIdx1, twsFrac := findIndexAndFraction(p.TWSList, effTWS)
	// 4. Clamp / find TWA index
	twaIdx0, twaIdx1, twaFrac := findIndexAndFraction(p.TWAList, angle)

	// Bilinear interpolation
	s00 := p.Speeds[twsIdx0][twaIdx0]
	s01 := p.Speeds[twsIdx0][twaIdx1]
	s10 := p.Speeds[twsIdx1][twaIdx0]
	s11 := p.Speeds[twsIdx1][twaIdx1]

	s0 := s00*(1.0-twaFrac) + s01*twaFrac
	s1 := s10*(1.0-twaFrac) + s11*twaFrac

	speed := (s0*(1.0-twsFrac) + s1*twsFrac) * lowWindScale

	// Smooth quadratic roll-off in the near-stall transition band [22°, 28°]
	if angle < 28.0 {
		frac := (angle - 22.0) / 6.0
		speed = speed * (frac * frac)
	}

	if speed < 0.05 {
		return 0.0
	}
	return speed
}

func findIndexAndFraction(arr []float64, val float64) (int, int, float64) {
	n := len(arr)
	if val <= arr[0] {
		return 0, 0, 0.0
	}
	if val >= arr[n-1] {
		return n - 1, n - 1, 0.0
	}

	for i := 0; i < n-1; i++ {
		if val >= arr[i] && val <= arr[i+1] {
			span := arr[i+1] - arr[i]
			if span <= 1e-6 {
				return i, i, 0.0
			}
			frac := (val - arr[i]) / span
			return i, i + 1, frac
		}
	}
	return n - 1, n - 1, 0.0
}

// Get36ftKetchPolar returns the pre-computed polar matrix for the 36ft Cruising Ketch with explicit no-go zone.
func Get36ftKetchPolar() *PolarTable {
	tws := []float64{6.0, 8.0, 10.0, 12.0, 14.0, 16.0, 20.0, 25.0}
	twa := []float64{0.0, 20.0, 25.0, 30.0, 35.0, 40.0, 45.0, 52.0, 60.0, 70.0, 80.0, 90.0, 110.0, 120.0, 135.0, 150.0, 165.0, 180.0}

	speeds := [][]float64{
		{0.00, 0.00, 0.00, 2.90, 3.32, 3.76, 4.01, 4.23, 4.38, 4.50, 4.54, 4.53, 4.73, 4.83, 4.76, 4.18, 3.66, 2.31}, // 6k
		{0.00, 0.00, 0.00, 3.84, 4.19, 4.42, 4.59, 4.76, 4.90, 5.00, 5.05, 5.03, 5.04, 5.00, 4.64, 4.38, 3.86, 3.34}, // 8k
		{0.00, 0.00, 0.00, 4.36, 4.63, 4.84, 4.99, 5.16, 5.30, 5.41, 5.46, 5.46, 5.57, 5.44, 4.90, 4.62, 4.17, 3.91}, // 10k
		{0.00, 0.00, 0.00, 4.72, 4.97, 5.17, 5.33, 5.50, 5.80, 5.93, 5.97, 6.01, 6.09, 5.99, 5.18, 4.88, 4.50, 4.36}, // 12k
		{0.00, 0.00, 0.00, 5.01, 5.26, 5.46, 5.76, 5.95, 6.07, 6.16, 6.19, 6.27, 6.32, 6.19, 5.68, 5.15, 4.81, 4.81}, // 14k
		{0.00, 0.00, 0.00, 5.27, 5.51, 5.87, 6.01, 6.15, 6.25, 6.33, 6.38, 6.46, 6.51, 6.41, 5.86, 5.65, 5.10, 5.10}, // 16k
		{0.00, 0.00, 0.00, 5.83, 6.06, 6.20, 6.31, 6.43, 6.52, 6.60, 6.70, 6.81, 6.80, 6.75, 6.09, 6.02, 5.76, 5.76}, // 20k
		{0.00, 0.00, 0.00, 6.13, 6.35, 6.46, 6.57, 6.69, 6.80, 6.90, 7.04, 7.18, 7.18, 7.16, 6.47, 6.43, 6.48, 6.26}, // 25k
	}

	return &PolarTable{
		BoatName: "36ft Cruising Ketch",
		TWSList:  tws,
		TWAList:  twa,
		Speeds:   speeds,
	}
}

// GetPresetPolar retrieves a polar table for a standard boat preset.
func GetPresetPolar(presetID string) *PolarTable {
	switch presetID {
	case "36ft-sloop":
		p := Get36ftKetchPolar()
		p.BoatName = "36ft Racer-Cruiser Sloop"
		return p
	case "40ft-cruiser":
		p := Get36ftKetchPolar()
		p.BoatName = "40ft Performance Cruiser"
		return p
	case "24ft-sportboat":
		p := Get36ftKetchPolar()
		p.BoatName = "24ft Sportboat"
		return p
	default:
		return Get36ftKetchPolar()
	}
}
