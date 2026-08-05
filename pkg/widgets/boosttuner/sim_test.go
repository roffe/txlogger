package boosttuner

import (
	"math"
	"testing"
)

func TestBilerp_CornersAndCenter(t *testing.T) {
	xtab := []float64{0, 1}
	ytab := []float64{0, 1}
	z := []float64{0, 10, 20, 30} // [y][x]: (0,0)=0 (1,0)=10 (0,1)=20 (1,1)=30

	cases := []struct {
		x, y, want float64
	}{
		{0, 0, 0}, {1, 0, 10}, {0, 1, 20}, {1, 1, 30},
		{0.5, 0, 5}, {0, 0.5, 10}, {0.5, 0.5, 15},
		{-5, -5, 0},  // clamp low
		{99, 99, 30}, // clamp high
	}
	for _, c := range cases {
		if got := bilerp(xtab, ytab, z, c.x, c.y); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("bilerp(%v,%v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

// TestSimulate_OpenLoopIdentity: with the loop disengaged and unchanged RegMap,
// the predicted PWM equals the logged PWMCalc (the residual carries everything).
func TestSimulate_OpenLoopIdentity(t *testing.T) {
	m := simMaps{
		setLoadXSP: []float64{1000}, nEngSP: []float64{3000},
		regMapRaw: []float64{450}, // bilerp -> 450, matching logged regCon
		pidXSP:    []float64{0}, pidYSP: []float64{3000},
		pMap: []float64{0}, iMap: []float64{0}, dMap: []float64{0},
		iFacMax: 350,
	}
	in := simInputs{
		n: 1, rpm: []float64{3000}, setv: []float64{1000}, loadDiff: []float64{0},
		regCon: []float64{450}, pFac: []float64{0}, iFac: []float64{0}, dFac: []float64{0},
		adap: []float64{30}, pwmCalc: []float64{520}, pwm2pct: []float64{1}, // open loop
	}
	out := simulate(in, m)
	if math.Abs(out.predicted[0]-520) > 1e-9 {
		t.Fatalf("predicted = %v, want 520 (logged)", out.predicted[0])
	}
}

// TestSimulate_ClosedLoopRecomputesP checks the P term is recomputed from the map
// and the environment residual is preserved.
func TestSimulate_ClosedLoopRecomputesP(t *testing.T) {
	m := simMaps{
		setLoadXSP: []float64{1000}, nEngSP: []float64{3000},
		regMapRaw: []float64{450},
		pidXSP:    []float64{0}, pidYSP: []float64{3000},
		pMap: []float64{100}, iMap: []float64{0}, dMap: []float64{0}, // P const 100
		iFacMax: 350,
	}
	// Logged decomposition: 450+5+20+0+30 = 505 == pwmCalc, so env residual = 0.
	in := simInputs{
		n: 1, rpm: []float64{3000}, setv: []float64{1000}, loadDiff: []float64{10},
		regCon: []float64{450}, pFac: []float64{5}, iFac: []float64{20}, dFac: []float64{0},
		adap: []float64{30}, pwmCalc: []float64{505}, pwm2pct: []float64{0},
	}
	out := simulate(in, m)
	// P_sim = loadDiff*Pconst/100 = 10*100/100 = 10; I=0; D=0.
	// predicted = 450 + 10 + 0 + 0 + 30 + env(0) = 490.
	if math.Abs(out.pFac[0]-10) > 1e-9 {
		t.Errorf("P_sim = %v, want 10", out.pFac[0])
	}
	if math.Abs(out.predicted[0]-490) > 1e-9 {
		t.Errorf("predicted = %v, want 490", out.predicted[0])
	}
}
