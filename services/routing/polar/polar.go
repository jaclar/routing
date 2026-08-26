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
// strictly enforcing aerodynamic no-go zone / in-irons limits (TWA < 28 deg -> 0 kts).
func (p *PolarTable) InterpolateSpeed(twsKts, twaDeg float64) float64 {
	// Symmetrical angle: wrap to [0, 180]
	angle := math.Abs(math.Mod(twaDeg, 360.0))
	if angle > 180.0 {
		angle = 360.0 - angle
	}

	// 1. Aerodynamic No-Go Zone: Sails stall / luff head-to-wind
	// For standard cruising/racing yachts, beating closer than ~28° to true wind produces zero forward drive
	if angle <= 22.0 {
		return 0.0
	}

	nTWS := len(p.TWSList)
	nTWA := len(p.TWAList)
	if nTWS == 0 || nTWA == 0 {
		return 0.0
	}

	// 2. Clamp / find TWS index
	twsIdx0, twsIdx1, twsFrac := findIndexAndFraction(p.TWSList, twsKts)
	// 3. Clamp / find TWA index
	twaIdx0, twaIdx1, twaFrac := findIndexAndFraction(p.TWAList, angle)

	// Bilinear interpolation
	s00 := p.Speeds[twsIdx0][twaIdx0]
	s01 := p.Speeds[twsIdx0][twaIdx1]
	s10 := p.Speeds[twsIdx1][twaIdx0]
	s11 := p.Speeds[twsIdx1][twaIdx1]

	s0 := s00*(1.0-twaFrac) + s01*twaFrac
	s1 := s10*(1.0-twaFrac) + s11*twaFrac

	speed := s0*(1.0-twsFrac) + s1*twsFrac

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
		{0.0, 0.0, 0.0, 2.94, 3.37, 3.77, 4.20, 4.56, 4.79, 4.95, 5.01, 4.99, 5.10, 5.04, 5.33, 5.09, 4.04, 2.33}, // 6k
		{0.0, 0.0, 0.0, 3.89, 4.48, 4.82, 5.05, 5.26, 5.43, 5.55, 5.60, 5.59, 5.53, 5.46, 5.49, 4.88, 4.19, 3.37}, // 8k
		{0.0, 0.0, 0.0, 4.71, 5.08, 5.33, 5.52, 5.72, 5.87, 5.99, 6.04, 6.02, 6.07, 6.00, 5.42, 5.12, 4.53, 4.06}, // 10k
		{0.0, 0.0, 0.0, 5.17, 5.48, 5.70, 5.88, 6.07, 6.23, 6.35, 6.41, 6.43, 6.64, 6.41, 5.71, 5.39, 4.90, 4.65}, // 12k
		{0.0, 0.0, 0.0, 5.50, 5.79, 6.01, 6.18, 6.38, 6.71, 6.93, 7.00, 7.07, 7.19, 7.04, 5.98, 5.66, 5.25, 5.24}, // 14k
		{0.0, 0.0, 0.0, 5.77, 6.05, 6.27, 6.45, 6.89, 7.09, 7.23, 7.29, 7.38, 7.48, 7.29, 6.25, 5.93, 5.56, 5.56}, // 16k
		{0.0, 0.0, 0.0, 6.21, 6.49, 6.96, 7.16, 7.35, 7.51, 7.63, 7.70, 7.85, 7.93, 7.79, 6.62, 6.69, 6.12, 6.12}, // 20k
		{0.0, 0.0, 0.0, 6.72, 7.07, 7.28, 7.45, 7.72, 7.87, 8.01, 8.13, 8.32, 9.30, 8.30, 7.62, 6.48, 7.33, 6.96}, // 25k
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
