package camtiming

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

// airFor is the airmass per combustion (mg) a 2290cc four swallows at the
// given VE, manifold pressure (bar) and inlet temperature (°C).
func airFor(ve, bar, iat float64) float64 {
	return ve / 100 * bar * 1e5 / (rGasAir * (iat + 273.15)) * (2290.0 / cylinders * 1e-6) * 1e6
}

// VE needs rpm, airmass, manifold pressure and inlet temperature from the
// ECU's profile; T5 logs no airmass and an unknown ECU logs nothing.
func TestVESignalsFor(t *testing.T) {
	sig, ok := veSignalsFor("T7")
	want := veSignals{"ActualIn.n_Engine", "MAF.m_AirInlet", "ActualIn.p_AirInlet", "ActualIn.T_AirInlet", "Out.X_AccPedal"}
	if !ok || sig != want {
		t.Errorf("T7 = %+v (%v), want %+v", sig, ok, want)
	}
	if sig, ok := veSignalsFor("T8"); !ok || sig.map_ != "In.p_AirInlet" || sig.pedal != "Out.X_AccPos" {
		t.Errorf("T8 = %+v (%v)", sig, ok)
	}
	for _, name := range []string{"T5", "", "no such ECU"} {
		if _, ok := veSignalsFor(name); ok {
			t.Errorf("veSignalsFor(%q) ok, want false", name)
		}
	}
}

// readSamples keeps the records that carry rpm, airmass, pressure and
// temperature with a running engine, treats a missing pedal as 0, and
// rescales a log whose pressures are in kPa to bar.
func TestReadSamples(t *testing.T) {
	sig := veSignals{"rpm", "air", "map", "iat", "pedal"}
	rows := func(mapVals ...float64) []map[string]float64 {
		var out []map[string]float64
		for i, p := range mapVals {
			out = append(out, map[string]float64{"rpm": 2000 + float64(i), "air": 500, "map": p, "iat": 30, "pedal": 90})
		}
		return out
	}
	open := func(rs []map[string]float64) logfile.Logfile {
		times := make([]time.Time, len(rs))
		for i := range times {
			times[i] = time.Unix(int64(i), 0)
		}
		return logfile.FromRows(times, rs)
	}

	t.Run("filters", func(t *testing.T) {
		rs := rows(1.1, 1.2, 1.3, 1.4, 1.5, 1.6)
		delete(rs[0], "iat")   // missing a channel
		rs[1]["rpm"] = 0       // engine not running
		rs[2]["air"] = 0       // no airmass
		delete(rs[3], "pedal") // pedal is optional
		got := readSamples(open(rs), sig)
		want := []veSample{
			{rpm: 2003, air: 500, bar: 1.4, iat: 30, pedal: 0},
			{rpm: 2004, air: 500, bar: 1.5, iat: 30, pedal: 90},
			{rpm: 2005, air: 500, bar: 1.6, iat: 30, pedal: 90},
		}
		if !slices.Equal(got, want) {
			t.Errorf("samples = %+v, want %+v", got, want)
		}
	})
	t.Run("kPa log", func(t *testing.T) {
		got := readSamples(open(rows(100, 180, 250)), sig)
		var bars []float64
		for _, s := range got {
			bars = append(bars, s.bar)
		}
		if !slices.Equal(bars, []float64{1, 1.8, 2.5}) {
			t.Errorf("pressures = %v, want them in bar", bars)
		}
	})
	t.Run("bar log", func(t *testing.T) {
		got := readSamples(open(rows(0.4, 2.5, 9.9)), sig)
		if got[1].bar != 2.5 || got[2].bar != 9.9 {
			t.Errorf("bar pressures rescaled: %+v", got)
		}
	})
	t.Run("nothing usable", func(t *testing.T) {
		if got := readSamples(open(rows()), sig); len(got) != 0 {
			t.Errorf("samples from an empty log: %+v", got)
		}
	})
}

// median sorts a copy and averages the middle pair of an even count.
func TestMedian(t *testing.T) {
	for _, tt := range []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{7}, 7},
		{[]float64{3, 1, 2}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
		{[]float64{100, 90, 130}, 100},
	} {
		in := slices.Clone(tt.in)
		if got := median(in); got != tt.want {
			t.Errorf("median(%v) = %v, want %v", tt.in, got, tt.want)
		}
		if !slices.Equal(in, tt.in) {
			t.Errorf("median reordered its input to %v", in)
		}
	}
}

// parse reads a decimal comma as a point and anything unparsable as 0.
func TestParse(t *testing.T) {
	e := numericentry.New("")
	for in, want := range map[string]float64{"2290": 2290, "1,5": 1.5, "-6.5": -6.5, "": 0, "abc": 0, "NaN": 0, "1,2,3": 0} {
		e.Text = in
		if got := parse(e); got != want {
			t.Errorf("parse(%q) = %v, want %v", in, got, want)
		}
	}
}

// A template label resolves back to its cam on the right side of the head
// only; misses give the zero cam, which draws no lobe.
func TestLookups(t *testing.T) {
	in := find("9170887", false)
	if c, ok := byLabel(in.Label(), false); !ok || c != in {
		t.Errorf("byLabel(%q) = %+v %v", in.Label(), c, ok)
	}
	if _, ok := byLabel(in.Label(), true); ok {
		t.Error("intake label resolved on the exhaust side")
	}
	if _, ok := byLabel("(none)", false); ok {
		t.Error("(none) resolved to a cam")
	}
	if c := find("9170887", true); c != (Cam{}) {
		t.Errorf("find on the wrong side = %+v", c)
	}
	if s := lobeSeries(Cam{}, "x", intakeColor, false); len(s.pts) != 0 {
		t.Errorf("zero cam drew %d points", len(s.pts))
	}
}

// valueAt interpolates inside the series and reports nothing outside it.
func TestValueAt(t *testing.T) {
	pts := []xy{{0, 0}, {10, 5}, {10, 8}, {20, 8}}
	for _, tt := range []struct {
		x    float64
		want float64
		ok   bool
	}{
		{-1, 0, false},
		{21, 0, false},
		{0, 0, true},
		{5, 2.5, true},
		{10, 5, true}, // first point at a repeated x
		{15, 8, true},
		{20, 8, true},
	} {
		got, ok := valueAt(pts, tt.x)
		if ok != tt.ok || got != tt.want {
			t.Errorf("valueAt(%v) = %v %v, want %v %v", tt.x, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := valueAt(nil, 0); ok {
		t.Error("value from an empty series")
	}
	if got, ok := valueAt([]xy{{1, 2}, {1, 4}}, 1); !ok || got != 4 { // a step reads its later point
		t.Errorf("vertical step = %v %v, want 4", got, ok)
	}
}

// niceStep picks a 1/2/5 multiple of a power of ten giving at most maxTicks
// ticks over the range.
func TestNiceStep(t *testing.T) {
	for _, tt := range []struct {
		rng      float64
		maxTicks int
		want     float64
	}{
		{0, 5, 1},
		{-10, 5, 1},
		{10, 1, 1},
		{100, 11, 10},
		{7, 5, 2},
		{0.33, 4, 0.2},
		{6000, 7, 1000},
		{540, 7, 100},
		{45, 4, 20},
	} {
		got := niceStep(tt.rng, tt.maxTicks)
		if math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("niceStep(%v, %d) = %v, want %v", tt.rng, tt.maxTicks, got, tt.want)
		}
		if tt.rng > 0 && tt.maxTicks >= 2 && tt.rng/got > float64(tt.maxTicks-1)+1e-9 {
			t.Errorf("niceStep(%v, %d) = %v gives more than %d ticks", tt.rng, tt.maxTicks, got, tt.maxTicks)
		}
	}
}

func newVEWidget(t *testing.T) (*Widget, fyne.Window) {
	t.Helper()
	test.NewTempApp(t)
	w := New(&Config{ECU: "T7"})
	win := test.NewWindow(w)
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(1000, 620))
	w.tabs.SelectIndex(1)
	win.Canvas().Capture()
	return w, win
}

func samplesAt(rpm float64, ves ...float64) []veSample {
	var out []veSample
	for _, ve := range ves {
		out = append(out, veSample{rpm: rpm, air: airFor(ve, 1.5, 30), bar: 1.5, iat: 30, pedal: 100})
	}
	return out
}

// recalc plots the median VE of every 250 rpm bin inside 1000–7000 rpm with
// at least three samples at or above the pedal floor, and reports the peak.
func TestVERecalc(t *testing.T) {
	w, _ := newVEWidget(t)
	v := w.ve
	var s []veSample
	s = append(s, samplesAt(2900, 90)...)
	s = append(s, samplesAt(3000, 130)...)
	s = append(s, samplesAt(3120, 100)...) // 3120 rounds into the 3000 bin
	peak := samplesAt(4000, 110, 112, 111)
	peak[0].pedal = 80 // exactly on the floor still counts
	s = append(s, peak...)
	s = append(s, samplesAt(5000, 120, 120)...)   // too few
	s = append(s, samplesAt(750, 50, 50, 50)...)  // below 1000 rpm
	s = append(s, samplesAt(7500, 80, 80, 80)...) // above 7000 rpm
	low := samplesAt(3000, 200, 200, 200)
	for i := range low {
		low[i].pedal = 40 // part throttle
	}
	s = append(s, low...)
	s = append(s, veSample{rpm: 3000, air: 500, bar: 0, iat: 30, pedal: 100}) // no VE without pressure
	v.samples = s
	v.recalc()

	if len(v.chart.series) != 1 {
		t.Fatalf("series = %+v", v.chart.series)
	}
	pts := v.chart.series[0].pts
	if len(pts) != 2 || pts[0].x != 3000 || pts[1].x != 4000 {
		t.Fatalf("points = %+v, want the 3000 and 4000 rpm bins", pts)
	}
	if math.Abs(pts[0].y-100) > 1e-6 || math.Abs(pts[1].y-111) > 1e-6 {
		t.Errorf("bin VE = %v / %v, want the medians 100 / 111", pts[0].y, pts[1].y)
	}
	if got, want := v.info.Text, "14 of 18 samples above 80% pedal — peak VE 111% @ 4000 rpm"; got != want {
		t.Errorf("info = %q, want %q", got, want)
	}

	v.minPedal.SetText("30") // the part throttle samples now count
	pts = v.chart.series[0].pts
	if math.Abs(pts[0].y-165) > 1e-6 { // median of 90 100 130 200 200 200
		t.Errorf("3000 rpm bin at 30%% pedal = %v, want 165", pts[0].y)
	}

	v.minPedal.SetText("80")
	v.disp.SetText("4580") // twice the displacement halves VE
	if got := v.chart.series[0].pts[1].y; math.Abs(got-55.5) > 1e-6 {
		t.Errorf("4000 rpm VE at 4580cc = %v, want 55.5", got)
	}

	v.disp.SetText("0")
	if v.chart.series != nil {
		t.Errorf("chart kept %d series with no displacement", len(v.chart.series))
	}
}

// With every sample under the pedal floor nothing is plotted and the info
// line says why.
func TestVERecalcNothingAbovePedal(t *testing.T) {
	w, _ := newVEWidget(t)
	v := w.ve
	v.samples = samplesAt(3000, 100, 100, 100, 100)
	for i := range v.samples {
		v.samples[i].pedal = 20
	}
	v.recalc()
	if v.chart.series != nil {
		t.Errorf("series = %+v, want none", v.chart.series)
	}
	if got, want := v.info.Text, "4 samples, none above 80% pedal with 3+ per 250 rpm bin"; got != want {
		t.Errorf("info = %q, want %q", got, want)
	}
}

func button(o fyne.CanvasObject, text string) *widget.Button {
	for _, c := range test.LaidOutObjects(o) {
		if b, ok := c.(*widget.Button); ok && b.Text == text {
			return b
		}
	}
	return nil
}

// Clear log drops the samples, the curve and the summary.
func TestVEClearLog(t *testing.T) {
	w, _ := newVEWidget(t)
	w.ve.samples = samplesAt(3000, 100, 100, 100)
	w.ve.recalc()
	b := button(w.ve.content, "Clear log")
	if b == nil {
		t.Fatal("no Clear log button")
	}
	test.Tap(b)
	if w.ve.samples != nil || w.ve.chart.series != nil {
		t.Errorf("after clear: %d samples, %d series", len(w.ve.samples), len(w.ve.chart.series))
	}
	if got := w.ve.info.Text; got != "Load a log to measure volumetric efficiency." {
		t.Errorf("info = %q", got)
	}
}

// Loading for an ECU that cannot measure VE explains instead of opening a
// file dialog; a nil config is a widget with no ECU, which cannot either.
func TestVELoadUnsupportedECU(t *testing.T) {
	test.NewTempApp(t)
	for cfg, want := range map[*Config]string{
		{ECU: "T5"}: `"T5" does not log all three`,
		nil:         `"" does not log all three`,
	} {
		w := New(cfg)
		w.ve.load()
		if got := w.ve.info.Text; !strings.Contains(got, want) {
			t.Errorf("info = %q, want it to contain %q", got, want)
		}
	}
}

// The pointer maps to the data x under it, snapped to the chart's step and
// NaN off the plot; leaving the chart hides the readout.
func TestChartPointer(t *testing.T) {
	w, _ := newVEWidget(t)
	c := w.ve.chart
	if !math.IsNaN(c.xAt(c.plotLeft - 1)) {
		t.Error("x left of the plot is not NaN")
	}
	if !math.IsNaN(c.xAt(c.plotLeft + c.plotW + 1)) {
		t.Error("x right of the plot is not NaN")
	}
	if got := c.xAt(c.plotLeft); got != veRPMMin {
		t.Errorf("left edge = %v, want %v", got, veRPMMin)
	}
	if got := c.xAt(c.plotLeft + c.plotW); got != veRPMMax {
		t.Errorf("right edge = %v, want %v", got, veRPMMax)
	}
	if got := c.xAt(c.plotLeft + c.plotW*0.333); math.Mod(got, 50) != 0 {
		t.Errorf("x = %v, not snapped to 50 rpm", got)
	}

	w.ve.samples = samplesAt(3000, 100, 100, 100)
	w.ve.samples = append(w.ve.samples, samplesAt(4000, 100, 100, 100)...)
	w.ve.recalc()
	mid := fyne.NewPos(c.plotLeft+c.plotW/2, c.plotTop+10)
	c.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: mid}})
	if math.IsNaN(c.hoverX) || !c.renderer.cursor.Visible() {
		t.Fatalf("hover at mid plot: x %v, cursor %v", c.hoverX, c.renderer.cursor.Visible())
	}
	for range 2 { // a second MouseOut finds nothing to hide
		c.MouseOut()
		if !math.IsNaN(c.hoverX) || c.renderer.cursor.Visible() || c.renderer.tipBg.Visible() {
			t.Errorf("after MouseOut: x %v, cursor %v, tip %v", c.hoverX, c.renderer.cursor.Visible(), c.renderer.tipBg.Visible())
		}
	}
}
