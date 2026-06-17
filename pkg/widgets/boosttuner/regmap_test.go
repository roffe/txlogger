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
