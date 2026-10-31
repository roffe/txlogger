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
		{0, 0, 0},
		{1, 0, 10},
		{0, 1, 20},
		{1, 1, 30},
		{0.5, 0, 5},
		{0, 0.5, 10},
		{0.5, 0.5, 15},
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

// bracket finds the surrounding breakpoints and fraction, clamping outside the
// table and treating a breakpoint as the end of the segment below it.
func TestBracket(t *testing.T) {
	tab := []float64{1, 2, 4}
	cases := []struct {
		tab    []float64
		v      float64
		i0, i1 int
		frac   float64
	}{
		{nil, 5, 0, 0, 0},
		{[]float64{7}, 5, 0, 0, 0},
		{tab, 0, 0, 0, 0},
		{tab, 1, 0, 0, 0},
		{tab, 1.5, 0, 1, 0.5},
		{tab, 2, 0, 1, 1},
		{tab, 3, 1, 2, 0.5},
		{tab, 4, 2, 2, 0},
		{tab, 99, 2, 2, 0},
	}
	for _, c := range cases {
		i0, i1, frac := bracket(c.tab, c.v)
		if i0 != c.i0 || i1 != c.i1 || math.Abs(frac-c.frac) > 1e-9 {
			t.Errorf("bracket(%v, %v) = %d,%d,%v; want %d,%d,%v", c.tab, c.v, i0, i1, frac, c.i0, c.i1, c.frac)
		}
	}
}

// flatSim builds replay inputs at a fixed operating point with the given
// LoadDiff and open-loop flag per sample, and 1x1 maps with the given constants.
func flatSim(loadDiff, twopct []float64, p, i, d, iFacMax, filter float64) (simInputs, simMaps) {
	n := len(loadDiff)
	fill := func(v float64) []float64 {
		s := make([]float64, n)
		for k := range s {
			s[k] = v
		}
		return s
	}
	in := simInputs{
		n: n, rpm: fill(3000), setv: fill(1000), loadDiff: loadDiff,
		regCon: fill(450), pFac: fill(0), iFac: fill(0), dFac: fill(0),
		adap: fill(0), pwmCalc: fill(450), pwm2pct: twopct,
	}
	m := simMaps{
		setLoadXSP: []float64{1000}, nEngSP: []float64{3000}, regMapRaw: []float64{450},
		pidXSP: []float64{0}, pidYSP: []float64{3000},
		pMap: []float64{p}, iMap: []float64{i}, dMap: []float64{d},
		iFacMax: iFacMax, filterFactor: filter,
	}
	return in, m
}

// The I term grows by iBuff/1000 each time the accumulator passes ±1000 and is
// clamped to ±IFacMax.
func TestSimulate_IntegratorClamp(t *testing.T) {
	ld := []float64{2, 2, 2, -2, -2, -2, -2}
	in, m := flatSim(ld, make([]float64, len(ld)), 0, 600, 0, 2, 0)
	out := simulate(in, m)
	want := []float64{1.2, 2, 2, 0.8, -0.4, -1.6, -2}
	for k := range want {
		if math.Abs(out.iFac[k]-want[k]) > 1e-9 {
			t.Fatalf("iFac = %v, want %v", out.iFac, want)
		}
	}
}

// Open loop ramps the PID off: its sample has zero P/I/D and the integrator
// restarts from zero once the loop re-engages.
func TestSimulate_OpenLoopResetsIntegrator(t *testing.T) {
	in, m := flatSim([]float64{2, 2, 2}, []float64{0, 1, 0}, 10, 600, 0, 350, 0)
	out := simulate(in, m)
	if want := []float64{1.2, 0, 1.2}; math.Abs(out.iFac[0]-want[0]) > 1e-9 || out.iFac[1] != 0 || math.Abs(out.iFac[2]-want[2]) > 1e-9 {
		t.Fatalf("iFac = %v, want %v", out.iFac, want)
	}
	if out.pFac[1] != 0 || out.dFac[1] != 0 {
		t.Fatalf("open-loop P/D = %v/%v, want 0", out.pFac[1], out.dFac[1])
	}
	// P = 2*10/100; predicted = regCon + P + I (no residual, no adaption).
	if want := 450 + 0.2 + 1.2; math.Abs(out.predicted[2]-want) > 1e-9 {
		t.Fatalf("predicted = %v, want %v", out.predicted[2], want)
	}
}

// A sample with any missing channel yields NaN outputs and forgets the previous
// LoadDiff, so the D term restarts without a derivative kick; the filtered D
// state itself carries over and decays.
func TestSimulate_NaNGap(t *testing.T) {
	ld := []float64{0, 50, math.NaN(), 80}
	in, m := flatSim(ld, make([]float64, len(ld)), 0, 0, 20, 350, 10)
	out := simulate(in, m)
	if !math.IsNaN(out.predicted[2]) || !math.IsNaN(out.regCon[2]) || !math.IsNaN(out.pFac[2]) ||
		!math.IsNaN(out.iFac[2]) || !math.IsNaN(out.dFac[2]) {
		t.Fatalf("gap outputs = %v %v %v %v %v, want NaN", out.predicted[2], out.regCon[2], out.pFac[2], out.iFac[2], out.dFac[2])
	}
	// D1 = (50-0)*20/(20+10); D3 = (0*20 + D1*10)/(20+10).
	d1 := 50.0 * 20 / 30
	if math.Abs(out.dFac[1]-d1) > 1e-9 {
		t.Fatalf("dFac[1] = %v, want %v", out.dFac[1], d1)
	}
	if want := d1 * 10 / 30; math.Abs(out.dFac[3]-want) > 1e-9 {
		t.Fatalf("dFac[3] = %v, want %v", out.dFac[3], want)
	}
	if math.Abs(out.regCon[3]-450) > 1e-9 {
		t.Fatalf("regCon[3] = %v, want 450", out.regCon[3])
	}
}
