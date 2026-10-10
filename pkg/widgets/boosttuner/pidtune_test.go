package boosttuner

import (
	"math"
	"reflect"
	"testing"
)

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

// detectTransients only starts on a large positive error with the loop engaged,
// drops events shorter than minLen, and ends events at a disengage, a gap or
// the end of the log.
func TestDetectTransients_Boundaries(t *testing.T) {
	nan := math.NaN()
	cases := []struct {
		name  string
		trace [][3]float64
		want  []transient
	}{
		{"open loop", [][3]float64{{3000, 150, 1}, {3000, 150, 1}, {3000, 150, 1}, {3000, 150, 1}, {3000, 150, 1}}, nil},
		{"too short", [][3]float64{{3000, 150, 0}, {3000, 100, 0}, {3000, 80, 0}, {3000, 60, 0}, {3000, 0, 1}}, nil},
		{"never settles", [][3]float64{
			{3000, 150, 0}, {3100, 120, 0}, {3200, 100, 0}, {3300, 80, 0}, {3400, 60, 0}, {3500, 50, 0},
		}, []transient{{rpm: 3250, riseSamples: 6}}},
		{"gap splits events", [][3]float64{
			{3000, 150, 0}, {3000, 100, 0}, {3000, 50, 0}, {3000, 10, 0}, {3000, 5, 0},
			{3000, nan, 0},
			{5000, 200, 0}, {5000, 150, 0}, {5000, 120, 0}, {5000, 110, 0}, {5000, 100, 0},
		}, []transient{
			{rpm: 3000, riseSamples: 3, settled: true, ssError: 7.5},
			{rpm: 5000, riseSamples: 5},
		}},
	}
	for _, c := range cases {
		got := detectTransients(buildTrace(c.trace), defaultTransientCfg())
		if len(got) != len(c.want) {
			t.Errorf("%s: got %d transients %+v, want %d", c.name, len(got), got, len(c.want))
			continue
		}
		for i := range got {
			g, w := got[i], c.want[i]
			if math.Abs(g.rpm-w.rpm) > 1e-9 || g.riseSamples != w.riseSamples || g.settled != w.settled ||
				math.Abs(g.ssError-w.ssError) > 1e-9 || g.overshoot != w.overshoot || g.crossings != w.crossings {
				t.Errorf("%s[%d] = %+v, want %+v", c.name, i, g, w)
			}
		}
	}
}

// suggestPID adds P for slow rise, trims P and D for overshoot or oscillation,
// adds I for steady error of either sign, averages per band, omits bands with
// nothing to change and sorts by rpm.
func TestSuggestPID_Rules(t *testing.T) {
	ysp := []float64{1000, 3000, 5000}
	cfg := defaultSuggestCfg()

	got := suggestPID([]transient{
		{rpm: 5100, riseSamples: 30},                              // slow rise
		{rpm: 900, overshoot: 80, riseSamples: 4, ssError: -30},   // trim + I
		{rpm: 3000, overshoot: 100, crossings: 1, riseSamples: 5}, // averages to
		{rpm: 2900, overshoot: 0, crossings: 2, riseSamples: 5},   // 50 / 1.5: no change
	}, ysp, cfg)

	want := []bandSuggestion{
		{row: 0, rpm: 1000, factorP: 0.85, factorI: 1.15, factorD: 0.85,
			reason: "overshoot 80 mg/c, 0.0 crossings; steady error -30 mg/c", events: 1},
		{row: 2, rpm: 5000, factorP: 1.15, factorI: 1, factorD: 1,
			reason: "slow rise (30 samples)", events: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions =\n%+v\nwant\n%+v", got, want)
	}

	if got := suggestPID(nil, ysp, cfg); len(got) != 0 {
		t.Fatalf("no transients gave %+v", got)
	}
}

// factorFor returns the factor for P, I or D and 1 for anything else.
func TestBandSuggestion_FactorFor(t *testing.T) {
	b := bandSuggestion{factorP: 0.5, factorI: 2, factorD: 3}
	for name, want := range map[string]float64{"P": 0.5, "I": 2, "D": 3, "X": 1, "": 1} {
		if got := b.factorFor(name); got != want {
			t.Errorf("factorFor(%q) = %v, want %v", name, got, want)
		}
	}
}

// signOf returns -1, 0 or 1.
func TestSignOf(t *testing.T) {
	for v, want := range map[float64]float64{-3: -1, 0: 0, 0.1: 1} {
		if got := signOf(v); got != want {
			t.Errorf("signOf(%v) = %v, want %v", v, got, want)
		}
	}
}
