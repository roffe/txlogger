package boosttuner

import "testing"

// buildTrace turns a sequence of (rpm, loadDiff, pwm2pct) samples into inputs.
func buildTrace(samples [][3]float64) pidSuggestInputs {
	in := pidSuggestInputs{n: len(samples)}
	for _, s := range samples {
		in.rpm = append(in.rpm, s[0])
		in.loadDiff = append(in.loadDiff, s[1])
		in.pwm2pct = append(in.pwm2pct, s[2])
	}
	return in
}

// TestDetectTransients_OvershootEvent feeds a spool-up that overshoots and
// oscillates, and checks the detector measures it.
func TestDetectTransients_OvershootEvent(t *testing.T) {
	// error: large positive -> crosses 0 -> negative (overshoot) -> back up.
	trace := [][3]float64{
		{3000, 150, 0}, // onset (err>startErr, engaged)
		{3000, 90, 0},
		{3000, 30, 0},
		{3000, -40, 0}, // crossing 1, overshoot 40
		{3000, -60, 0}, // overshoot 60
		{3000, 10, 0},  // crossing 2, settled (|err|<=20)
		{3000, 5, 0},
		{3000, 4, 0},
	}
	ts := detectTransients(buildTrace(trace), defaultTransientCfg())
	if len(ts) != 1 {
		t.Fatalf("got %d transients, want 1", len(ts))
	}
	got := ts[0]
	if got.overshoot < 59 || got.overshoot > 61 {
		t.Errorf("overshoot = %v, want ~60", got.overshoot)
	}
	if got.crossings < 2 {
		t.Errorf("crossings = %d, want >=2", got.crossings)
	}
	if !got.settled {
		t.Errorf("expected event to settle")
	}
}

// TestSuggestPID_TrimsOnOvershoot checks an overshooting band yields a P/D trim.
func TestSuggestPID_TrimsOnOvershoot(t *testing.T) {
	ysp := []float64{1000, 2000, 3000, 4000, 5000, 6000}
	ts := []transient{
		{rpm: 3000, overshoot: 80, crossings: 3, riseSamples: 6, settled: true, ssError: 2},
		{rpm: 3100, overshoot: 70, crossings: 2, riseSamples: 5, settled: true, ssError: 1},
	}
	sugg := suggestPID(ts, ysp, defaultSuggestCfg())
	if len(sugg) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(sugg))
	}
	b := sugg[0]
	if b.factorP >= 1 || b.factorD >= 1 {
		t.Errorf("expected P/D trim (<1), got P=%.2f D=%.2f", b.factorP, b.factorD)
	}
	if b.factorI != 1 {
		t.Errorf("expected no I change for small steady error, got %.2f", b.factorI)
	}
}

// TestSuggestPID_AddsIOnSteadyError checks a persistent steady error raises I.
func TestSuggestPID_AddsIOnSteadyError(t *testing.T) {
	ysp := []float64{1000, 2000, 3000, 4000}
	ts := []transient{
		{rpm: 2000, overshoot: 5, crossings: 0, riseSamples: 8, settled: true, ssError: 40},
	}
	sugg := suggestPID(ts, ysp, defaultSuggestCfg())
	if len(sugg) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(sugg))
	}
	if sugg[0].factorI <= 1 {
		t.Errorf("expected I boost (>1), got %.2f", sugg[0].factorI)
	}
}
