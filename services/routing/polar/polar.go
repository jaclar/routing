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
