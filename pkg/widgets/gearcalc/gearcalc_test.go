package gearcalc

import (
	"math"
	"testing"
)

// FM55 1st gear: values cross-checked against the original t7gearcal JS app
func TestGearCalc(t *testing.T) {
	speed, ratio := gearCalc(3000, 3.38, 4.05, 0.626)
	if math.Round(ratio) != 1160 {
		t.Errorf("ratio = %f, want 1160", ratio)
	}
	if rng := math.Round(ratio * 25.9 / 100); rng != 300 {
		t.Errorf("range = %f, want 300", rng)
	}
	if math.Abs(speed-25.86) > 0.01 {
		t.Errorf("speed = %f, want ~25.86", speed)
	}
}
