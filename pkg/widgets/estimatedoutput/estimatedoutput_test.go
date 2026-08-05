package estimatedoutput

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
)

func TestInterpTable(t *testing.T) {
	table := []int{10, 20, 30, 40}
	xaxis := []int{0, 100}
	yaxis := []int{1000, 2000}
	tests := []struct {
		name   string
		yv, xv int
		want   float64
	}{
		{"center", 1500, 50, 25},
		{"clamp high both", 3000, 200, 40},
		{"below axis start", 500, -50, 10},
		{"on lower corner", 1000, 0, 10},
		{"x only", 1000, 50, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interpTable(table, xaxis, yaxis, tt.yv, tt.xv); got != tt.want {
				t.Fatalf("interpTable(%d, %d) = %v, want %v", tt.yv, tt.xv, got, tt.want)
			}
		})
	}
	// 1-D lookup: xaxis {0}, x 0
	if got := interpTable([]int{100, 200}, []int{0}, []int{1000, 2000}, 1500, 0); got != 150 {
		t.Fatalf("1-D interp = %v, want 150", got)
	}
}

func TestT5PressureToTorque(t *testing.T) {
	stock := t5TorqueTables[0]
	tests := []struct {
		bar  float64
		want float64
	}{
		{1.0, 300},  // exact breakpoint
		{0.5, 225},  // between 0.4 and 0.6
		{0.0, 110},  // linear extrapolation below the first breakpoint
		{-0.5, -15}, // idle vacuum keeps extrapolating
		{2.5, 460},  // clamps flat above the last breakpoint
	}
	for _, tt := range tests {
		if got := t5PressureToTorque(tt.bar, stock); math.Abs(got-tt.want) > 1e-9 {
			t.Fatalf("t5PressureToTorque(%v) = %v, want %v", tt.bar, got, tt.want)
		}
	}
}

func sym(t *testing.T, name string, vals ...int) *symbol.Symbol {
	t.Helper()
	data := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.BigEndian.PutUint16(data[i*2:], uint16(v))
	}
	s := &symbol.Symbol{Name: name, Length: uint16(len(data))}
	if err := s.SetData(data); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestT7Calculate runs the full T7 chain on a synthetic binary where the
// fuel cut limiter (1200 mg/c) is the binding WOT limit.
func TestT7Calculate(t *testing.T) {
	fw := symbol.NewCollection(
		// 2 rpm points x 2 pedal points, WOT column requests 1500 mg/c
		sym(t, "PedalMapCal.m_RequestMap", 0, 0, 1500, 1500),
		sym(t, "PedalMapCal.n_EngineMap", 1000, 4000),
		sym(t, "PedalMapCal.X_PedalMap", 0, 100),
		// airmass = torque * 3.1 at both rpm rows
		sym(t, "TorqueCal.m_AirTorqMap", 0, 3100, 0, 3100),
		sym(t, "TorqueCal.M_EngXSP", 0, 1000),
		sym(t, "TorqueCal.n_EngYSP", 1000, 4000),
		sym(t, "BstKnkCal.MaxAirmass", 2000, 2000),
		sym(t, "BstKnkCal.OffsetXSP", 0),
		sym(t, "BstKnkCal.n_EngYSP", 1000, 4000),
		sym(t, "TorqueCal.M_NominalMap", 0, 1000, 0, 1000),
		sym(t, "TorqueCal.m_AirXSP", 0, 3100),
		sym(t, "FCutCal.m_AirInletLimit", 1200),
		sym(t, "TorqueCal.M_EngMaxTab", 400, 400),
		sym(t, "TorqueCal.M_EngMaxAutTab", 350, 350),
		sym(t, "TorqueCal.M_ManGearLim", 500, 500, 500, 500, 500, 500),
		sym(t, "InjCorrCal.InjectorConst", 875),
		sym(t, "InjCorrCal.BattCorrTab", 200, 400),
		sym(t, "InjCorrCal.BattCorrSP", 90, 140),
		sym(t, "BFuelCal.Map", 100, 100, 100, 100),
		sym(t, "BFuelCal.AirXSP", 0, 2000),
		sym(t, "BFuelCal.RpmYSP", 1000, 4000),
	)
	opts := map[string]float64{"fwlimit": 1, "gear": 3, "ambient": 100}
	res, err := t7{}.Calculate(fw, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RPM) != 2 || res.RPM[1] != 4000 {
		t.Fatalf("unexpected RPM axis %v", res.RPM)
	}
	// torque limiter caps at 400 Nm -> 1240 mg/c, fuel cut caps at 1200
	for i, lim := range res.Limiters {
		if lim != limFuelCut {
			t.Fatalf("limiter[%d] = %q, want %q", i, lim, limFuelCut)
		}
	}
	// 1200 mg/c / 3.1 = 387 Nm; power = 387*4000/7024 = 220 metric hp
	// (integer division, 7024 = PS constant like the original suites)
	tq := res.Curves[1].Values
	if tq[0] != 387 || tq[1] != 387 {
		t.Fatalf("torque = %v, want [387 387]", tq)
	}
	pwr := res.Curves[0].Values
	if pwr[0] != 55 || pwr[1] != 220 {
		t.Fatalf("power = %v, want [55 220]", pwr)
	}
}

func TestT7MissingSymbols(t *testing.T) {
	fw := symbol.NewCollection()
	_, err := t7{}.Calculate(fw, map[string]float64{"gear": 3, "ambient": 100})
	if err == nil {
		t.Fatal("expected error for empty collection")
	}
}

// TestPullDetectionAndCurves: 5 s cruise, then an 8 s constant-acceleration
// pull (a = 2 m/s², 400 rpm/s), then cruise. With loss/drag/rolling zeroed,
// P = m*a*v exactly.
func TestPullDetectionAndCurves(t *testing.T) {
	var tt, v, rpm, air []float64
	add := func(ts, vs, rs float64) {
		tt = append(tt, ts)
		v = append(v, vs)
		rpm = append(rpm, rs)
		air = append(air, math.NaN())
	}
	for ts := 0.0; ts < 5; ts += 0.05 {
		add(ts, 20, 2000)
	}
	for ts := 0.0; ts <= 8; ts += 0.05 {
		add(5+ts, 20+2*ts, 2500+400*ts)
	}
	for ts := 0.05; ts < 3; ts += 0.05 {
		add(13+ts, 36, 5700)
	}
	pulls, err := findPulls(tt, v, rpm, air, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 {
		t.Fatalf("got %d pulls, want 1", len(pulls))
	}
	pull := pulls[0]
	if pull.rpm[0] > 2900 || pull.rpm[len(pull.rpm)-1] < 5300 {
		t.Fatalf("pull range %v-%v, want ~2500-5700", pull.rpm[0], pull.rpm[len(pull.rpm)-1])
	}
	prm := logParams{MassKg: 1500, LossPct: 0, Cd: 0, AreaM2: 0, Crr: 0}
	curves := pull.curves(prm, []float64{3000, 4000, 5000, 7000}, false, 1)
	if curves[0].Name != "Log power (hp)" || curves[1].Name != "Log torque (Nm)" {
		t.Fatalf("unexpected curve names %q %q", curves[0].Name, curves[1].Name)
	}
	// at 3000 rpm: t=1.25s into the pull, v=22.5 m/s, P=1500*2*22.5=67.5 kW
	// -> 214.9 Nm, 90.5 hp
	wantNm := 67500 * 60 / (2 * math.Pi * 3000)
	if got := curves[1].Values[0]; math.Abs(got-wantNm) > 2 {
		t.Fatalf("torque@3000 = %v, want ~%v", got, wantNm)
	}
	if got := curves[0].Values[0]; math.Abs(got-wantNm*3000/7024) > 1 {
		t.Fatalf("power@3000 = %v, want ~%v", got, wantNm*3000/7024)
	}
	// 7000 rpm is outside the pull -> NaN
	if !math.IsNaN(curves[0].Values[3]) {
		t.Fatalf("power@7000 = %v, want NaN", curves[0].Values[3])
	}
	// no airmass signal -> only the two dv/dt curves
	if len(curves) != 2 {
		t.Fatalf("got %d curves, want 2", len(curves))
	}
}

func TestPullDetectionRejectsCruise(t *testing.T) {
	var tt, v, rpm, air []float64
	for ts := 0.0; ts < 60; ts += 0.1 {
		tt = append(tt, ts)
		v = append(v, 25)
		rpm = append(rpm, 2500+10*math.Sin(ts))
		air = append(air, math.NaN())
	}
	if _, err := findPulls(tt, v, rpm, air, nil); err == nil {
		t.Fatal("expected no pull in a cruise log")
	}
}

// TestPullDetectionSplitsGearChange: a 2nd-gear pull, a fast shift with an
// rpm drop, then a longer 3rd-gear pull. The shift must not be bridged into
// one mixed-gear segment; the 3rd-gear pull wins on span.
func TestPullDetectionSplitsGearChange(t *testing.T) {
	var tt, v, rpm, air []float64
	add := func(ts, vs, rs float64) {
		tt = append(tt, ts)
		v = append(v, vs)
		rpm = append(rpm, rs)
		air = append(air, math.NaN())
	}
	for ts := 0.0; ts < 3; ts += 0.05 { // 2nd gear: 3500 -> 5600
		add(ts, 15+2.5*ts, 3500+700*ts)
	}
	for ts := 0.0; ts < 0.4; ts += 0.05 { // shift: rpm falls fast
		add(3+ts, 22.5, 5600-6000*ts)
	}
	for ts := 0.0; ts < 6; ts += 0.05 { // 3rd gear: 3200 -> 6200
		add(3.4+ts, 22.5+2*ts, 3200+500*ts)
	}
	pulls, err := findPulls(tt, v, rpm, air, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 2 {
		t.Fatalf("got %d pulls, want 2 (one per gear)", len(pulls))
	}
	pull := pulls[1]
	if pull.rpm[0] < 3100 || pull.rpm[0] > 3600 || pull.rpm[len(pull.rpm)-1] < 5900 {
		t.Fatalf("pull %v-%v rpm, want the 3rd gear pull ~3200-6200", pull.rpm[0], pull.rpm[len(pull.rpm)-1])
	}
	// neither pull may contain the shift's rpm drop
	for _, p := range pulls {
		for i := 1; i < len(p.rpm); i++ {
			if p.rpm[i] < p.rpm[i-1]-300 {
				t.Fatalf("pull contains a %v -> %v rpm drop", p.rpm[i-1], p.rpm[i])
			}
		}
	}
}

// TestAveragePullCurves: two identical-shape pulls at different accelerations
// (a=2 and a=4 over the same rpm range) must average to the a=3 power.
func TestAveragePullCurves(t *testing.T) {
	makePull := func(a float64) *logPull {
		var tt, v, rpm, air []float64
		for ts := 0.0; ts <= 8; ts += 0.05 {
			tt = append(tt, ts)
			v = append(v, 20+a*ts)
			rpm = append(rpm, 2500+400*ts)
			air = append(air, math.NaN())
		}
		pulls, err := findPulls(tt, v, rpm, air, nil)
		if err != nil || len(pulls) != 1 {
			t.Fatalf("findPulls: %v, %d pulls", err, len(pulls))
		}
		return pulls[0]
	}
	prm := logParams{MassKg: 1500, LossPct: 0, Cd: 0, AreaM2: 0, Crr: 0}
	axis := []float64{3000}
	avg := averagePullCurves([]*logPull{makePull(2), makePull(4)}, prm, axis, false, 1)
	c2 := makePull(2).curves(prm, axis, false, 1)
	c4 := makePull(4).curves(prm, axis, false, 1)
	want := (c2[1].Values[0] + c4[1].Values[0]) / 2
	if got := avg[1].Values[0]; math.Abs(got-want) > 0.1 {
		t.Fatalf("avg torque@3000 = %v, want %v", got, want)
	}
	if avg[0].Name != "Log power (hp)" || len(avg) != 2 {
		t.Fatalf("unexpected avg curves: %d, %q", len(avg), avg[0].Name)
	}
}

// TestPullDetectionThrottleGate: a brisk part-throttle segment (steep enough
// rpm slope to pass the slope-only gate) followed by a true WOT pull. With a
// throttle channel only the WOT segment may survive; without one, both do.
func TestPullDetectionThrottleGate(t *testing.T) {
	var tt, v, rpm, thr, air []float64
	add := func(ts, rs, th float64) {
		tt = append(tt, ts)
		v = append(v, rs/100) // arbitrary consistent speed
		rpm = append(rpm, rs)
		thr = append(thr, th)
		air = append(air, math.NaN())
	}
	for ts := 0.0; ts < 6; ts += 0.05 { // part throttle: 300 rpm/s, 1800 rpm span
		add(ts, 2000+300*ts, 50)
	}
	for ts := 0.0; ts < 2; ts += 0.05 { // lift, rpm settles lower
		add(6+ts, 3800-800*ts, 10)
	}
	for ts := 0.0; ts < 5; ts += 0.05 { // WOT: 400 rpm/s
		add(8+ts, 2400+400*ts, 98)
	}
	pulls, err := findPulls(tt, v, rpm, air, thr)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 {
		t.Fatalf("got %d pulls with throttle gate, want 1 (WOT only)", len(pulls))
	}
	if pulls[0].rpm[0] > 2600 {
		t.Fatalf("pull starts at %v rpm, want the WOT segment ~2400", pulls[0].rpm[0])
	}
	pulls, err = findPulls(tt, v, rpm, air, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 2 {
		t.Fatalf("got %d pulls without throttle, want 2", len(pulls))
	}
}

// TestPullDetectionTallGear: a 5th-gear WOT pull at ~170 rpm/s is under the
// slope-only threshold (250) but must be found when the throttle channel
// proves it is WOT.
func TestPullDetectionTallGear(t *testing.T) {
	var tt, v, rpm, thr, air []float64
	for ts := 0.0; ts < 8; ts += 0.05 { // 170 rpm/s, 1360 rpm span
		tt = append(tt, ts)
		v = append(v, 40+ts)
		rpm = append(rpm, 3000+170*ts)
		thr = append(thr, 97)
		air = append(air, math.NaN())
	}
	if _, err := findPulls(tt, v, rpm, air, nil); err == nil {
		t.Fatal("slope-only detection found a pull at 170 rpm/s; threshold change?")
	}
	pulls, err := findPulls(tt, v, rpm, air, thr)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || pulls[0].rpm[len(pulls[0].rpm)-1]-pulls[0].rpm[0] < 1000 {
		t.Fatalf("throttle-gated detection missed the tall-gear pull: %d pulls", len(pulls))
	}
}

type fakeLog struct{ recs []logfile.Record }

func (f *fakeLog) Get() logfile.Record           { return logfile.Record{} }
func (f *fakeLog) Next() logfile.Record          { return logfile.Record{} }
func (f *fakeLog) Prev() logfile.Record          { return logfile.Record{} }
func (f *fakeLog) Seek(int)                      {}
func (f *fakeLog) Pos() int                      { return 0 }
func (f *fakeLog) Len() int                      { return len(f.recs) }
func (f *fakeLog) RecordAt(i int) logfile.Record { return f.recs[i] }
func (f *fakeLog) Start() time.Time              { return time.Time{} }
func (f *fakeLog) End() time.Time                { return time.Time{} }
func (f *fakeLog) Close()                        {}

// TestSpeedAltValidation: In.v_Vehicle2 must only be preferred when alive.
// A dead rear ABS ring reads a constant 0 — using it yields v=0 everywhere
// and a confidently wrong ~0 hp curve.
func TestSpeedAltValidation(t *testing.T) {
	sig := logSignalsByECU["T7"]
	makeLog := func(alt func(kmh float64) float64) *fakeLog {
		f := &fakeLog{}
		for i := 0; i < 300; i++ { // 15 s @ 20 Hz WOT ramp, 2000-6800 rpm
			ts := float64(i) * 0.05
			kmh := 50 + 3*ts
			vals := map[string]float64{
				sig.rpm:      2000 + 320*ts,
				sig.speed:    kmh,
				sig.throttle: 98,
			}
			if alt != nil {
				vals[sig.speedAlt] = alt(kmh)
			}
			f.recs = append(f.recs, logfile.Record{
				Time:   time.Unix(0, int64(ts*float64(time.Second))),
				Values: vals,
			})
		}
		return f
	}
	// dead channel: constant 0 -> primary must be used, pull still found
	pulls, err := extractPulls(makeLog(func(float64) float64 { return 0 }), sig)
	if err != nil {
		t.Fatalf("dead v2 channel broke extraction: %v", err)
	}
	if pulls[0].v[0] < 10 {
		t.Fatalf("dead v2 channel was used: v[0] = %v m/s", pulls[0].v[0])
	}
	// alive channel, 10%% off the primary (still within 20%%): preferred
	pulls, err = extractPulls(makeLog(func(kmh float64) float64 { return kmh * 1.1 }), sig)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1.1 * 50 / 3.6; math.Abs(pulls[0].v[0]-want) > 1 {
		t.Fatalf("alive v2 channel not used: v[0] = %v m/s, want ~%v", pulls[0].v[0], want)
	}
}
