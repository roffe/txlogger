package estimatedoutput

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/roffe/txlogger/pkg/logfile"
)

// rampPull builds a constant-acceleration pull: v = 20 + a*t (m/s),
// rpm = 2500 + 400*t over 8 s at 20 Hz, with an optional airmass value.
func rampPull(t *testing.T, a, air float64) *logPull {
	t.Helper()
	return rampPullFrom(t, 2500, a, air)
}

// rampPullFrom is rampPull starting at rpm0.
func rampPullFrom(t *testing.T, rpm0, a, air float64) *logPull {
	t.Helper()
	var tt, v, rpm, am []float64
	for ts := 0.0; ts <= 8; ts += 0.05 {
		tt = append(tt, ts)
		v = append(v, 20+a*ts)
		rpm = append(rpm, rpm0+400*ts)
		am = append(am, air)
	}
	pulls, err := findPulls(tt, v, rpm, am, nil)
	if err != nil || len(pulls) != 1 {
		t.Fatalf("findPulls: %v, %d pulls", err, len(pulls))
	}
	return pulls[0]
}

// TestLogSignalsFor maps the ECU profile's rpm/speed/airmass/throttle
// names; ECUs without rpm or speed are not usable.
func TestLogSignalsFor(t *testing.T) {
	sig, ok := logSignalsFor("T7")
	if !ok || sig.rpm != "ActualIn.n_Engine" || sig.speed != "In.v_Vehicle" || sig.speedAlt != "In.v_Vehicle2" || sig.airmass != "MAF.m_AirInlet" || sig.throttle != "Out.X_AccPedal" {
		t.Fatalf("T7 signals = %+v, ok=%v", sig, ok)
	}
	sig, ok = logSignalsFor("T5")
	if !ok || sig.rpm != "Rpm" || sig.speed != "Bil_hast" || sig.speedAlt != "" || sig.airmass != "" || sig.throttle != "Medeltrot" {
		t.Fatalf("T5 signals = %+v, ok=%v", sig, ok)
	}
	if _, ok := logSignalsFor("NOPE"); ok {
		t.Fatal("unknown ECU reported usable signals")
	}
}

// TestPullInfo describes the rpm range and duration of a pull.
func TestPullInfo(t *testing.T) {
	p := &logPull{t: []float64{1.25, 3, 5.5}, rpm: []float64{2510.4, 4000, 5699.6}}
	if got := p.info(); got != "pull: 2510-5700 rpm, 4.2 s" {
		t.Fatalf("info = %q", got)
	}
}

// TestPullCurvesSanitizesParams: implausible mass and loss fall back to the
// defaults and negative drag/rolling inputs count as zero.
func TestPullCurvesSanitizesParams(t *testing.T) {
	p := rampPull(t, 2, math.NaN())
	axis := []float64{3000, 4000}
	clean := p.curves(logParams{MassKg: defaultLogParams.MassKg, LossPct: defaultLogParams.LossPct}, axis, false, 1)
	for _, bad := range []logParams{
		{MassKg: 0, LossPct: -1, Cd: -0.3, AreaM2: -2, Crr: -0.01},
		{MassKg: 100, LossPct: 90, Cd: -1},
	} {
		got := p.curves(bad, axis, false, 1)
		for i := range axis {
			if math.Abs(got[1].Values[i]-clean[1].Values[i]) > 1e-9 {
				t.Fatalf("params %+v: torque %v, want %v", bad, got[1].Values, clean[1].Values)
			}
		}
	}
}

// TestPullCurvesDragAndLoss: aero drag, rolling resistance and drivetrain
// loss add to the measured wheel power in closed form.
func TestPullCurvesDragAndLoss(t *testing.T) {
	p := rampPull(t, 2, math.NaN())
	prm := logParams{MassKg: 1500, LossPct: 20, Cd: 0.3, AreaM2: 2, Crr: 0.01}
	got := p.curves(prm, []float64{3000}, false, 1)
	// at 3000 rpm the window is symmetric around t=1.25 s: v=22.5 m/s, a=2.
	// Wheel power: 67500 W accelerating + 4186 W aero drag + 3311 W rolling
	// = 74997 W; / 0.8 for 20 % loss = 93746 W at the crank = 298.4 Nm
	if want := 298.4; math.Abs(got[1].Values[0]-want) > 0.5 {
		t.Fatalf("torque = %v, want ~%v", got[1].Values[0], want)
	}
	// 93.7 kW = 127.45 metric hp
	if want := 127.45; math.Abs(got[0].Values[0]-want) > 0.25 {
		t.Fatalf("power = %v, want ~%v", got[0].Values[0], want)
	}
}

// TestPullCurvesAirmass: with a logged airmass the hidden air torque/power
// curves use the estimator's airmass/3.1 model with E85, high-rpm roll-off
// and efficiency applied.
func TestPullCurvesAirmass(t *testing.T) {
	p := rampPull(t, 2, 1000)
	if !p.hasAir {
		t.Fatal("hasAir not set")
	}
	axis := []float64{3000, 5500}
	curves := p.curves(defaultLogParams, axis, true, 0.9)
	if len(curves) != 4 || curves[2].Name != "Log air power (hp)" || curves[3].Name != "Log air torque (Nm)" || !curves[2].Hidden || !curves[3].Hidden {
		t.Fatalf("unexpected curves %+v", curves)
	}
	// 1000 mg/c / 3.1 * 1.07 (E85) * 0.9 = 310.65 Nm; 5500 rpm also takes the
	// 0.95 high-rpm roll-off. Power is torque * rpm / 7024.
	wantTq := []float64{310.6451612903226, 295.11290322580646}
	wantHp := []float64{132.67874200896466, 231.0821423322801}
	for i, r := range axis {
		if math.Abs(curves[3].Values[i]-wantTq[i]) > 1e-9 {
			t.Errorf("air torque@%v = %v, want %v", r, curves[3].Values[i], wantTq[i])
		}
		if math.Abs(curves[2].Values[i]-wantHp[i]) > 1e-9 {
			t.Errorf("air power@%v = %v, want %v", r, curves[2].Values[i], wantHp[i])
		}
	}
}

// TestPullCurvesSparseWindow: an rpm point with fewer than three samples in
// its window stays NaN instead of fitting a slope through noise.
func TestPullCurvesSparseWindow(t *testing.T) {
	p := &logPull{
		t:   []float64{0, 1, 2, 3, 4},
		v:   []float64{10, 12, 14, 16, 18},
		rpm: []float64{3000, 3050, 3100, 4000, 4100},
		air: []float64{math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()},
	}
	// 4050 rpm has exactly two samples (4000, 4100) in its +-250 rpm window
	c := p.curves(logParams{MassKg: 1000}, []float64{3050, 4050}, false, 1)
	// three samples around 3050 rpm: a = 2 m/s², v = 12 m/s, so 24 kW and
	// 75.14 Nm at 3050 rpm
	if want := 75.142006; math.Abs(c[1].Values[0]-want) > 1e-5 {
		t.Fatalf("dense point = %v, want %v", c[1].Values[0], want)
	}
	if !math.IsNaN(c[1].Values[1]) || !math.IsNaN(c[0].Values[1]) {
		t.Fatalf("two-sample point = %v/%v, want NaN", c[0].Values[1], c[1].Values[1])
	}
}

// TestSlopeAndMean fits a least-squares slope; a zero time spread yields
// NaN acceleration.
func TestSlopeAndMean(t *testing.T) {
	a, m := slopeAndMean([]float64{0, 1, 2, 3}, []float64{1, 3, 5, 7})
	if a != 2 || m != 4 {
		t.Fatalf("slope/mean = %v/%v", a, m)
	}
	a, m = slopeAndMean([]float64{5, 5, 5}, []float64{1, 2, 3})
	if !math.IsNaN(a) || m != 2 {
		t.Fatalf("zero spread: %v/%v", a, m)
	}
}

// TestWotMask: the WOT level is 85 % of the channel's 98th percentile;
// gaps count as WOT, and patchy or all-zero channels disable gating.
func TestWotMask(t *testing.T) {
	nan := math.NaN()
	thr := []float64{10, 50, 84, 85, 100, nan, 99, 100, 100, 100}
	got := wotMask(thr, len(thr))
	want := []bool{false, false, false, true, true, true, true, true, true, true}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wotMask = %v, want %v", got, want)
		}
	}
	if wotMask([]float64{nan, nan, nan, 100}, 4) != nil {
		t.Fatal("patchy channel must disable gating")
	}
	if wotMask([]float64{0, 0, 0, 0}, 4) != nil {
		t.Fatal("all-zero channel must disable gating")
	}
	if wotMask(nil, 10) != nil {
		t.Fatal("no channel must disable gating")
	}
}

// TestExtractPullsTooShort rejects logs with fewer than ten usable rows,
// naming the signals it looked for.
func TestExtractPullsTooShort(t *testing.T) {
	sig, _ := logSignalsFor("T5")
	var times []time.Time
	var rows []map[string]float64
	for i := 0; i < 9; i++ {
		times = append(times, time.Unix(int64(i), 0))
		rows = append(rows, map[string]float64{sig.rpm: 3000, sig.speed: 80})
	}
	// rows missing rpm or speed are skipped
	times = append(times, time.Unix(20, 0))
	rows = append(rows, map[string]float64{sig.speed: 80})
	_, err := extractPulls(logfile.FromRows(times, rows), sig)
	if err == nil || !strings.Contains(err.Error(), "no usable Rpm/Bil_hast samples") {
		t.Fatalf("err = %v", err)
	}
}

// TestExtractPullsAirmassAndThrottle: logged airmass reaches the pull, and
// the throttle channel gates detection; time is rebased to the first row.
func TestExtractPullsAirmassAndThrottle(t *testing.T) {
	sig, _ := logSignalsFor("T8")
	var times []time.Time
	var rows []map[string]float64
	add := func(ts, rpm, kmh, thr float64) {
		times = append(times, time.Unix(1000, 0).Add(time.Duration(ts*float64(time.Second))))
		rows = append(rows, map[string]float64{sig.rpm: rpm, sig.speed: kmh, sig.throttle: thr, sig.airmass: 900})
	}
	for ts := 0.0; ts < 3; ts += 0.05 { // cruise at part throttle
		add(ts, 2000, 60, 20)
	}
	for ts := 0.0; ts < 6; ts += 0.05 { // WOT pull
		add(3+ts, 2500+500*ts, 60+8*ts, 100)
	}
	pulls, err := extractPulls(logfile.FromRows(times, rows), sig)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || !pulls[0].hasAir || pulls[0].air[0] != 900 {
		t.Fatalf("pulls = %d, hasAir = %v", len(pulls), len(pulls) > 0 && pulls[0].hasAir)
	}
	if p := pulls[0]; p.t[0] < 2.9 || p.t[0] > 3.1 || p.rpm[0] > 2600 {
		t.Fatalf("pull starts at t=%v rpm=%v, want ~3 s / 2500 rpm", p.t[0], p.rpm[0])
	}
}

// TestAveragePullCurvesSkipsUncovered: a point only one pull covers takes
// that pull's value; a point no pull covers stays NaN.
func TestAveragePullCurvesSkipsUncovered(t *testing.T) {
	low := rampPullFrom(t, 2500, 2, math.NaN())  // 2500-5700 rpm
	high := rampPullFrom(t, 5000, 4, math.NaN()) // 5000-8200 rpm
	axis := []float64{3000, 7500, 9000}
	avg := averagePullCurves([]*logPull{low, high}, defaultLogParams, axis, false, 1)
	lowC := low.curves(defaultLogParams, axis, false, 1)
	highC := high.curves(defaultLogParams, axis, false, 1)
	if math.IsNaN(lowC[1].Values[0]) || avg[1].Values[0] != lowC[1].Values[0] {
		t.Fatalf("3000 rpm (low pull only) average = %v, want %v", avg[1].Values[0], lowC[1].Values[0])
	}
	if math.IsNaN(highC[1].Values[1]) || avg[1].Values[1] != highC[1].Values[1] {
		t.Fatalf("7500 rpm (high pull only) average = %v, want %v", avg[1].Values[1], highC[1].Values[1])
	}
	if !math.IsNaN(avg[1].Values[2]) {
		t.Fatalf("uncovered point = %v, want NaN", avg[1].Values[2])
	}
	if got := averagePullCurves(nil, defaultLogParams, axis, false, 1); len(got) != 0 {
		t.Fatalf("no pulls -> %d curves", len(got))
	}
}

// TestExtractPullsIgnoresDisagreeingSpeedAlt: a live undriven-wheel channel
// that reads 40 % below the primary speed (outside the +-20 % agreement
// band) is not trusted; the primary speed is used.
func TestExtractPullsIgnoresDisagreeingSpeedAlt(t *testing.T) {
	sig, _ := logSignalsFor("T7")
	var times []time.Time
	var rows []map[string]float64
	for i := 0; i < 200; i++ { // 10 s WOT ramp at 20 Hz, 2000-5200 rpm
		ts := float64(i) * 0.05
		kmh := 50 + 3*ts
		times = append(times, time.Unix(0, int64(i)*int64(50*time.Millisecond)))
		rows = append(rows, map[string]float64{sig.rpm: 2000 + 320*ts, sig.speed: kmh, sig.speedAlt: kmh * 0.6, sig.throttle: 98})
	}
	pulls, err := extractPulls(logfile.FromRows(times, rows), sig)
	if err != nil {
		t.Fatal(err)
	}
	// the pull starts at the first row: 50 km/h on the primary channel
	if want := 50 / 3.6; math.Abs(pulls[0].v[0]-want) > 0.01 {
		t.Fatalf("v[0] = %v m/s, want %v (primary speed)", pulls[0].v[0], want)
	}
}

// pullLog is a 20 Hz rpm trace built from (duration, rpm/s) segments, with
// speed rising alongside and no throttle or airmass channel.
func pullLog(rpm0 float64, segs ...[2]float64) (t, v, rpm, air []float64) {
	r := rpm0
	i := 0
	for _, s := range segs {
		for n := 0; n < int(math.Round(s[0]/0.05)); n++ {
			t = append(t, float64(i)*0.05)
			v = append(v, 20+float64(i)*0.05)
			rpm = append(rpm, r)
			air = append(air, math.NaN())
			r += s[1] * 0.05
			i++
		}
	}
	return
}

// TestFindPullsSplitRules: a short stall in the rpm rise (0.3 s) is bridged
// into one pull, a long one (1 s) ends the pull, and an rpm drop (a fast
// gear change) always splits, even when it is shorter than the 0.5 s
// dropout tolerance.
func TestFindPullsSplitRules(t *testing.T) {
	tests := []struct {
		name string
		segs [][2]float64
		want int
	}{
		{"short stall bridged", [][2]float64{{4, 400}, {0.3, 0}, {4, 400}}, 1},
		{"long stall splits", [][2]float64{{4, 400}, {1, 0}, {4, 400}}, 2},
		{"fast shift splits", [][2]float64{{4, 400}, {0.1, -15000}, {4, 400}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, v, rpm, air := pullLog(2500, tt.segs...)
			pulls, err := findPulls(ts, v, rpm, air, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(pulls) != tt.want {
				for _, p := range pulls {
					t.Log(p.info())
				}
				t.Fatalf("got %d pulls, want %d", len(pulls), tt.want)
			}
		})
	}
}
