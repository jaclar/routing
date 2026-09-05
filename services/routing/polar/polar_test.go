package polar

import (
	"math"
	"testing"
)

func TestPolarZeroAndLowWind(t *testing.T) {
	p := testTable()

	// 1. Zero and dead air wind (<= 3.0 kts) must produce 0.0 kts boat speed
	if spd := p.InterpolateSpeed(0.0, 90.0); spd != 0.0 {
		t.Errorf("expected 0.0 kts speed at 0 TWS, got %f", spd)
	}

	if spd := p.InterpolateSpeed(1.5, 90.0); spd != 0.0 {
		t.Errorf("expected 0.0 kts speed at 1.5 TWS, got %f", spd)
	}

	if spd := p.InterpolateSpeed(3.0, 90.0); spd != 0.0 {
		t.Errorf("expected 0.0 kts speed at 3.0 TWS, got %f", spd)
	}

	// 2. Light wind (4.5 kts, halfway between 3.0 and 6.0 kts -> 50% of 6.0 kt baseline)
	spd4p5 := p.InterpolateSpeed(4.5, 90.0)
	spd6 := p.InterpolateSpeed(6.0, 90.0)
	if spd4p5 <= 0.0 || spd4p5 >= spd6 {
		t.Errorf("expected 4.5kt wind speed to be between 0 and %f, got %f", spd6, spd4p5)
	}
	expectedApprox := spd6 * 0.5
	if math.Abs(spd4p5-expectedApprox) > 0.1 {
		t.Errorf("expected ~%f kts at 4.5kt wind, got %f", expectedApprox, spd4p5)
	}

	// 3. No-go zone (TWA < 22 deg) must produce 0.0 kts
	if spd := p.InterpolateSpeed(15.0, 15.0); spd != 0.0 {
		t.Errorf("expected 0.0 kts in no-go zone (15 deg), got %f", spd)
	}

	// 4. Normal sailing (12.0 kts at 90 deg)
	spd12 := p.InterpolateSpeed(12.0, 90.0)
	if spd12 < 5.8 || spd12 > 6.2 {
		t.Errorf("expected ~6.0 kts at 12kt wind 90 deg, got %f", spd12)
	}
}

// testTable is a fixed cruising-yacht polar for tests in this package. It duplicates
// polartest.Table, which cannot be imported here without an import cycle.
func testTable() *PolarTable {
	return &PolarTable{
		BoatName: "Test Cruising Yacht",
		TWSList:  []float64{6.0, 8.0, 10.0, 12.0, 14.0, 16.0, 20.0, 25.0},
		TWAList:  []float64{0.0, 20.0, 25.0, 30.0, 35.0, 40.0, 45.0, 52.0, 60.0, 70.0, 80.0, 90.0, 110.0, 120.0, 135.0, 150.0, 165.0, 180.0},
		Speeds: [][]float64{
			{0.00, 0.00, 0.00, 2.90, 3.32, 3.76, 4.01, 4.22, 4.38, 4.49, 4.54, 4.53, 4.42, 4.36, 4.06, 3.50, 2.99, 2.70},
			{0.00, 0.00, 0.00, 3.84, 4.19, 4.42, 4.59, 4.76, 4.90, 5.00, 5.05, 5.03, 5.04, 5.00, 4.63, 4.38, 3.90, 3.61},
			{0.00, 0.00, 0.00, 4.36, 4.63, 4.83, 4.99, 5.16, 5.30, 5.41, 5.45, 5.46, 5.56, 5.44, 5.11, 4.94, 4.34, 4.30},
			{0.00, 0.00, 0.00, 4.72, 4.97, 5.17, 5.33, 5.50, 5.80, 5.92, 5.96, 6.01, 6.09, 5.98, 5.53, 4.81, 4.70, 4.77},
			{0.00, 0.00, 0.00, 5.01, 5.26, 5.46, 5.75, 5.95, 6.07, 6.16, 6.19, 6.27, 6.32, 6.19, 6.03, 5.17, 5.08, 5.17},
			{0.00, 0.00, 0.00, 5.27, 5.51, 5.86, 6.01, 6.15, 6.25, 6.33, 6.38, 6.46, 6.50, 6.40, 6.27, 5.48, 5.42, 5.52},
			{0.00, 0.00, 0.00, 5.83, 6.06, 6.20, 6.31, 6.43, 6.52, 6.60, 6.70, 6.81, 6.79, 6.75, 6.54, 6.01, 6.08, 6.20},
			{0.00, 0.00, 0.00, 6.12, 6.29, 6.41, 6.50, 6.69, 6.80, 6.90, 7.04, 7.18, 7.18, 7.16, 6.80, 6.43, 6.50, 6.62},
		},
	}
}
