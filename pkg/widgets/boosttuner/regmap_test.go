package boosttuner

import (
	"math"
	"testing"
)

// TestLearnRegMap_FoldsCorrectionsToPercent checks the core learning behaviour:
// the learned cell value is (RegConValue + P + I + D + Adaption) averaged and
// converted from raw 0.1% units to %, and that the sample filter rejects
// off-target, clamped, open-loop and transient samples.
func TestLearnRegMap_FoldsCorrectionsToPercent(t *testing.T) {
	axisX := []float64{800, 900, 1000} // load
	axisY := []float64{1000, 2000}     // rpm
	cols, rows := len(axisX), len(axisY)
	current := make([]float64, cols*rows) // all zero

	// Helper to append one sample to every channel.
	var in regMapInputs
	add := func(rpm, load, regCon, p, i, d, adap, loadDiff, pwm, twopct float64) {
		in.rpm = append(in.rpm, rpm)
		in.setv = append(in.setv, load)
		in.regCon = append(in.regCon, regCon)
		in.pFac = append(in.pFac, p)
		in.iFac = append(in.iFac, i)
		in.dFac = append(in.dFac, d)
		in.adap = append(in.adap, adap)
		in.loadDiff = append(in.loadDiff, loadDiff)
		in.pwmCalc = append(in.pwmCalc, pwm)
		in.pwm2pct = append(in.pwm2pct, twopct)
	}

	// Two good samples at cell (load=900 -> col1, rpm=2000 -> row1).
	// raw sum = 400 + 30 + 20 + 0 + 50 = 500 -> 50.0% ; pwmCalc 500 (in band).
	add(2000, 900, 400, 30, 20, 0, 50, 5, 500, 0)
	add(2000, 905, 400, 30, 20, 0, 50, -5, 500, 0) // steady (small steps)

	// Rejected: off target (loadDiff beyond onTarget).
	add(2000, 900, 400, 30, 20, 0, 50, 500, 500, 0)
	// Rejected: open-loop flag set.
	add(2000, 900, 400, 30, 20, 0, 50, 5, 500, 1)
	// Rejected: clamped at the upper rail.
	add(2000, 900, 400, 30, 20, 0, 50, 5, 980, 0)
	in.n = len(in.rpm)

	p := regMapParams{
		axisX: axisX, axisY: axisY, cols: cols, rows: rows, current: current,
		onTarget: 30, rpmStab: 150, loadStab: 50,
	}
	learned, counts, used, filtered := learnRegMap(in, p)

	if used != 2 {
		t.Fatalf("used = %d, want 2", used)
	}
	if filtered != 3 {
		t.Fatalf("filtered = %d, want 3", filtered)
	}
	cell := 1*cols + 1 // row1 (rpm 2000), col1 (load 900)
	if counts[cell] != 2 {
		t.Fatalf("counts[cell] = %d, want 2", counts[cell])
	}
	if math.Abs(learned[cell]-50.0) > 1e-9 {
		t.Fatalf("learned[cell] = %v, want 50.0", learned[cell])
	}
	// Untouched cells fall back to current (0 here).
	if learned[0] != current[0] {
		t.Fatalf("empty cell learned = %v, want current %v", learned[0], current[0])
	}
}

// TestAcceptSample_Steadiness verifies the transient gate uses the previous
// sample's rpm/load steps.
func TestAcceptSample_Steadiness(t *testing.T) {
	p := regMapParams{onTarget: 30, rpmStab: 150, loadStab: 50}
	// Big rpm jump from the previous sample -> rejected.
	if acceptSample(p, 3000, 900, 0, 500, 0, 2000, 900) {
		t.Fatal("expected rejection on large rpm step")
	}
	// First sample (prev = NaN) with everything in band -> accepted.
	if !acceptSample(p, 3000, 900, 0, 500, 0, math.NaN(), math.NaN()) {
		t.Fatal("expected acceptance for first in-band sample")
	}
}

// acceptSample rejects off-target, open-loop, rail-clamped and unsteady samples
// and accepts the boundaries of each band.
func TestAcceptSample_Gates(t *testing.T) {
	p := regMapParams{onTarget: 30, rpmStab: 150, loadStab: 50}
	nan := math.NaN()
	cases := []struct {
		name                                  string
		loadDiff, pwm, twopct, prevRPM, prevL float64
		want                                  bool
	}{
		{"in band", 0, 500, 0, 3000, 900, true},
		{"on-target edge", -30, 500, 0, 3000, 900, true},
		{"off target", 31, 500, 0, 3000, 900, false},
		{"open loop", 0, 500, 1, 3000, 900, false},
		{"lower rail", 0, 20, 0, 3000, 900, false},
		{"just above lower rail", 0, 21, 0, 3000, 900, true},
		{"upper rail", 0, 980, 0, 3000, 900, false},
		{"rpm step edge", 0, 500, 0, 3150, 900, true},
		{"load step", 0, 500, 0, 3000, 951, false},
		{"load step edge", 0, 500, 0, 3000, 850, true},
		{"no previous", 0, 500, 0, nan, nan, true},
	}
	for _, c := range cases {
		if got := acceptSample(p, 3000, 900, c.loadDiff, c.pwm, c.twopct, c.prevRPM, c.prevL); got != c.want {
			t.Errorf("%s: accept = %v, want %v", c.name, got, c.want)
		}
	}
}

// learnRegMap skips samples with any missing channel without counting them as
// filtered, and a gap in rpm/load restarts the steadiness check.
func TestLearnRegMap_SkipsGaps(t *testing.T) {
	nan := math.NaN()
	row := func(rpm, pFac float64) []float64 {
		// rpm, setv, regCon, p, i, d, adap, loadDiff, pwm, 2%
		return []float64{rpm, 900, 400, pFac, 0, 0, 0, 0, 500, 0}
	}
	rows := [][]float64{
		row(2000, 10),
		row(2000, nan), // missing P: skipped
		row(nan, 10),   // missing rpm: skipped, and forgets the previous rpm
		row(4000, 10),  // 2000 rpm away from the last real rpm, but accepted
	}
	var in regMapInputs
	for _, r := range rows {
		in.rpm = append(in.rpm, r[0])
		in.setv = append(in.setv, r[1])
		in.regCon = append(in.regCon, r[2])
		in.pFac = append(in.pFac, r[3])
		in.iFac = append(in.iFac, r[4])
		in.dFac = append(in.dFac, r[5])
		in.adap = append(in.adap, r[6])
		in.loadDiff = append(in.loadDiff, r[7])
		in.pwmCalc = append(in.pwmCalc, r[8])
		in.pwm2pct = append(in.pwm2pct, r[9])
	}
	in.n = len(rows)
	p := regMapParams{
		axisX: []float64{900}, axisY: []float64{2000, 4000}, cols: 1, rows: 2,
		current: []float64{0, 0}, onTarget: 30, rpmStab: 150, loadStab: 50,
	}
	learned, counts, used, filtered := learnRegMap(in, p)
	if used != 2 || filtered != 0 {
		t.Fatalf("used = %d, filtered = %d; want 2, 0", used, filtered)
	}
	if counts[0] != 1 || counts[1] != 1 {
		t.Fatalf("counts = %v, want [1 1]", counts)
	}
	if math.Abs(learned[0]-41) > 1e-9 || math.Abs(learned[1]-41) > 1e-9 {
		t.Fatalf("learned = %v, want [41 41]", learned)
	}
}
