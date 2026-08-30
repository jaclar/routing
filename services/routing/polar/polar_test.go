package polar

import (
	"math"
	"testing"
)

func TestPolarZeroAndLowWind(t *testing.T) {
	p := Get36ftKetchPolar()

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
