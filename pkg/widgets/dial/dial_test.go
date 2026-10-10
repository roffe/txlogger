package dial

import (
	"image/color"
	"math"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
)

// render builds a Dial from cfg, creates its renderer and lays it out at size.
func render(t *testing.T, cfg *widgets.GaugeConfig, size fyne.Size) (*Dial, fyne.WidgetRenderer) {
	t.Helper()
	test.NewTempApp(t)
	d := New(cfg)
	r := test.TempWidgetRenderer(t, d)
	d.Resize(size)
	return d, r
}

func cfg100(classic bool) *widgets.GaugeConfig {
	return &widgets.GaugeConfig{Min: 0, Max: 100, Steps: 10, Classic: classic}
}

// polar returns p's distance from mid and its bearing in degrees: 0 points
// straight up, positive turns clockwise.
func polar(mid, p fyne.Position) (radius, deg float64) {
	dx, dy := float64(p.X-mid.X), float64(p.Y-mid.Y)
	return math.Hypot(dx, dy), math.Atan2(dx, -dy) * 180 / math.Pi
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// checkPolar verifies p lies at radius and bearing deg around mid.
func checkPolar(t *testing.T, what string, mid, p fyne.Position, radius, deg float64) {
	t.Helper()
	r, a := polar(mid, p)
	da := math.Mod(a-deg+540, 360) - 180
	if !near(r, radius) || math.Abs(da) > 0.01 {
		t.Errorf("%s at r=%.3f %.3f°, want r=%.3f %.3f°", what, r, a, radius, deg)
	}
}

// checkNeedle verifies the classic needle points at deg: its tip 0.99r out,
// its tail 0.15r behind the hub.
func checkNeedle(t *testing.T, d *Dial, deg float64) {
	t.Helper()
	r := float64(d.radius)
	checkPolar(t, "needle tail", d.middle, d.needle.Position1, 0.15*r, deg+180)
	checkPolar(t, "needle tip", d.middle, d.needle.Position2, 0.99*r, deg)
}

// New fills in the display format, the gauge factor and a 100x100 minimum
// size; a MinSize is only used when both dimensions are set.
func TestNewDefaults(t *testing.T) {
	test.NewTempApp(t)
	for _, tc := range []struct {
		name       string
		in         widgets.GaugeConfig
		wantMin    fyne.Size
		wantFactor float64
	}{
		{"zero", widgets.GaugeConfig{Max: 100, Steps: 10}, fyne.NewSize(100, 100), 1},
		{"width only", widgets.GaugeConfig{Max: 100, Steps: 10, MinSize: fyne.NewSize(150, 0)}, fyne.NewSize(100, 100), 1},
		{"both set", widgets.GaugeConfig{Max: 100, Steps: 10, MinSize: fyne.NewSize(150, 120), GaugeFactor: 0.1}, fyne.NewSize(150, 120), 0.1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.in
			d := New(&cfg)
			if d.GetConfig() != &cfg {
				t.Fatal("GetConfig does not return the config New was given")
			}
			if got := test.TempWidgetRenderer(t, d).MinSize(); got != tc.wantMin {
				t.Errorf("MinSize = %v, want %v", got, tc.wantMin)
			}
			if cfg.GaugeFactor != tc.wantFactor {
				t.Errorf("GaugeFactor = %v, want %v", cfg.GaugeFactor, tc.wantFactor)
			}
			if d.displayString != "%.0f" || d.displayText.Text != "0" {
				t.Errorf("format %q readout %q, want %%.0f and 0", d.displayString, d.displayText.Text)
			}
		})
	}
}

// Modern draws a track ring with a zone-colored value arc and neutral ticks;
// classic draws a gray rim, gradient pips, a hub and a needle.
func TestStyles(t *testing.T) {
	t.Run("modern", func(t *testing.T) {
		d, r := render(t, cfg100(false), fyne.NewSize(200, 200))
		if d.needle != nil || d.center != nil {
			t.Error("modern dial has a needle or hub")
		}
		checkArc(t, "track", d.face, widgets.DialStartDeg, widgets.DialEndDeg, widgets.DialRingCutout, widgets.TrackColor)
		checkArc(t, "value arc", d.valueArc, widgets.DialStartDeg, widgets.DialStartDeg, widgets.DialRingCutout, widgets.ZoneColor(0))
		if d.displayText.Color != widgets.TextPrimary {
			t.Errorf("readout color %v, want %v", d.displayText.Color, widgets.TextPrimary)
		}
		for i, p := range d.pips {
			var want color.Color = widgets.TickMajor
			if i%2 != 0 {
				want = widgets.TickMinor
			}
			if p.StrokeColor != want {
				t.Errorf("pip %d color %v, want %v", i, p.StrokeColor, want)
			}
		}
		want := append([]fyne.CanvasObject{d.face, d.valueArc}, objects(d)...)
		want = append(want, d.titleText, d.displayText)
		checkObjects(t, r.Objects(), want)
	})
	t.Run("classic", func(t *testing.T) {
		d, r := render(t, cfg100(true), fyne.NewSize(200, 200))
		if d.valueArc != nil {
			t.Error("classic dial has a value arc")
		}
		checkArc(t, "rim", d.face, -135.73, 135.8, 0.985, color.RGBA{0x80, 0x80, 0x80, 0xFF})
		if d.needle.StrokeColor != (color.RGBA{0xFF, 0x67, 0, 0xFF}) {
			t.Errorf("needle color %v", d.needle.StrokeColor)
		}
		if d.center.FillColor != (color.RGBA{0x01, 0x0B, 0x13, 0xFF}) {
			t.Errorf("hub color %v", d.center.FillColor)
		}
		if d.displayText.Color != (color.RGBA{0x2c, 0xfc, 0x03, 0xFF}) {
			t.Errorf("readout color %v", d.displayText.Color)
		}
		for i, p := range d.pips {
			if want := widgets.ZoneColor(float64(i) / 10); p.StrokeColor != want {
				t.Errorf("pip %d color %v, want %v", i, p.StrokeColor, want)
			}
		}
		want := append(objects(d), d.face, d.titleText, d.center, d.needle, d.displayText)
		checkObjects(t, r.Objects(), want)
	})
}

// objects lists the pips followed by the labels, as the renderer stacks them.
func objects(d *Dial) []fyne.CanvasObject {
	var objs []fyne.CanvasObject
	for _, p := range d.pips {
		objs = append(objs, p)
	}
	for _, l := range d.pipLabels {
		if l != nil {
			objs = append(objs, l)
		}
	}
	return objs
}

func checkObjects(t *testing.T, got, want []fyne.CanvasObject) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d objects, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("object %d is %T, want %T", i, got[i], want[i])
		}
	}
}

func checkArc(t *testing.T, what string, a *canvas.Arc, start, end, cutout float32, c color.Color) {
	t.Helper()
	if a.StartAngle != start || a.EndAngle != end || a.CutoutRatio != cutout || a.FillColor != c {
		t.Errorf("%s: %v..%v° cutout %v color %v, want %v..%v° cutout %v color %v",
			what, a.StartAngle, a.EndAngle, a.CutoutRatio, a.FillColor, start, end, cutout, c)
	}
}

// Every second pip carries a scale label: Min + i/Steps of the range, times
// GaugeFactor, with GaugeTextString's precision (whole numbers by default).
func TestPipLabels(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  widgets.GaugeConfig
		want []string
	}{
		{"0..100", widgets.GaugeConfig{Max: 100, Steps: 10},
			[]string{"0", "20", "40", "60", "80", "100"}},
		{"engine temp", widgets.GaugeConfig{Min: -20, Max: 140, Steps: 16},
			[]string{"-20", "0", "20", "40", "60", "80", "100", "120", "140"}},
		{"rpm x1000", widgets.GaugeConfig{Max: 8000, Steps: 16, GaugeFactor: 0.001},
			[]string{"0", "1", "2", "3", "4", "5", "6", "7", "8"}},
		{"speed x10", widgets.GaugeConfig{Max: 260, Steps: 26, GaugeFactor: 0.1, GaugeTextString: "%.0f"},
			[]string{"0", "2", "4", "6", "8", "10", "12", "14", "16", "18", "20", "22", "24", "26"}},
		{"one decimal", widgets.GaugeConfig{Max: 3, Steps: 30, GaugeTextString: "%.1f"},
			[]string{"0.0", "0.2", "0.4", "0.6", "0.8", "1.0", "1.2", "1.4", "1.6", "1.8", "2.0",
				"2.2", "2.4", "2.6", "2.8", "3.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test.NewTempApp(t)
			cfg := tc.cfg
			d := New(&cfg)
			if len(d.pips) != cfg.Steps+1 || len(d.pipLabels) != cfg.Steps+1 {
				t.Fatalf("%d pips, %d label slots, want %d", len(d.pips), len(d.pipLabels), cfg.Steps+1)
			}
			var got []string
			longest := 0
			for i, l := range d.pipLabels {
				if (l != nil) != (i%2 == 0) {
					t.Errorf("pip %d: label present %v, want only on even pips", i, l != nil)
				}
				if l != nil {
					got = append(got, l.Text)
					longest = max(longest, len(l.Text))
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("labels %q, want %q", got, tc.want)
			}
			if d.maxLabelChars != longest {
				t.Errorf("maxLabelChars %d, want %d", d.maxLabelChars, longest)
			}
		})
	}
}

// The modern value arc sweeps from -135° by the value's fraction of the range,
// clamped at Min/Max, and takes the zone color for that fraction.
func TestValueArc(t *testing.T) {
	for _, tc := range []struct {
		min, max, value float64
		end             float32
		color           color.RGBA
	}{
		{0, 100, 50, 0, color.RGBA{255, 255, 0, 255}},
		{0, 100, 25, -67.5, color.RGBA{127, 255, 0, 255}},
		{0, 100, 75, 67.5, color.RGBA{255, 127, 0, 255}},
		{0, 100, 100, 135, color.RGBA{255, 0, 0, 255}},
		{0, 100, -10, -135, color.RGBA{0, 255, 0, 255}},
		{0, 100, 150, 135, color.RGBA{255, 0, 0, 255}},
		{-20, 140, 60, 0, color.RGBA{255, 255, 0, 255}},
	} {
		cfg := &widgets.GaugeConfig{Min: tc.min, Max: tc.max, Steps: 10}
		d, _ := render(t, cfg, fyne.NewSize(200, 200))
		d.SetValue(tc.value)
		if d.valueArc.StartAngle != widgets.DialStartDeg || d.valueArc.EndAngle != tc.end || d.valueArc.FillColor != tc.color {
			t.Errorf("%v..%v value %v: arc %v..%v° %v, want -135..%v° %v", tc.min, tc.max, tc.value,
				d.valueArc.StartAngle, d.valueArc.EndAngle, d.valueArc.FillColor, tc.end, tc.color)
		}
	}
}

// The classic needle turns through 270° from -135° at Min to 135° at Max and
// stays at Min below it.
func TestNeedle(t *testing.T) {
	d, _ := render(t, &widgets.GaugeConfig{Min: -20, Max: 140, Steps: 16, Classic: true}, fyne.NewSize(200, 200))
	if !near(float64(d.needle.StrokeWidth), 200.0/60) {
		t.Errorf("needle width %v, want %v", d.needle.StrokeWidth, 200.0/60)
	}
	for _, tc := range []struct{ value, deg float64 }{
		{60, 0},
		{-20, -135},
		{140, 135},
		{20, -67.5},
		{-50, -135},
	} {
		d.SetValue(tc.value)
		checkNeedle(t, d, tc.deg)
	}
}

// The needle never wraps back onto the scale past Max: it points at Max or
// into the dead zone between 135° and 225°, so a value more than a third of
// the range past Max does not read as a low value.
func TestNeedleClampedAtMax(t *testing.T) {
	d, _ := render(t, &widgets.GaugeConfig{Max: 100, Steps: 10, Classic: true}, fyne.NewSize(200, 200))
	for _, v := range []float64{110, 140, 200} {
		d.SetValue(v)
		checkPastMax(t, d.middle, d.needle, v)
	}
}

// checkPastMax verifies an over-range needle points at Max (135°) or into the
// dead zone below the dial (135°..225°), never back onto the scale.
func checkPastMax(t *testing.T, mid fyne.Position, n *canvas.Line, v float64) {
	t.Helper()
	_, deg := polar(mid, n.Position2)
	if deg < 0 {
		deg += 360
	}
	if deg < 135-0.01 || deg > 225+0.01 {
		t.Errorf("value %v: needle at %.1f°, want 135°..225° (pinned at Max or in the dead zone)", v, deg)
	}
}

// The readout is formatted with DisplayString; a repeated value is dropped,
// and a new value that formats the same moves the arc but keeps the text.
func TestReadout(t *testing.T) {
	for _, tc := range []struct {
		format string
		value  float64
		want   string
	}{
		{"", 12.345, "12"},
		{"%.1f", 12.345, "12.3"},
		{"%.1f bar", 1.27, "1.3 bar"},
		{"%5.1f", 7.3, "  7.3"},
	} {
		cfg := cfg100(false)
		cfg.DisplayString = tc.format
		d, _ := render(t, cfg, fyne.NewSize(200, 200))
		d.SetValue(tc.value)
		if d.displayText.Text != tc.want {
			t.Errorf("format %q value %v: readout %q, want %q", tc.format, tc.value, d.displayText.Text, tc.want)
		}
	}

	cfg := cfg100(false)
	cfg.DisplayString = "%.1f"
	d, _ := render(t, cfg, fyne.NewSize(200, 200))
	d.SetValue(50)
	d.value = -1 // sentinel: apply overwrites it
	d.SetValue(50)
	if d.value != -1 {
		t.Error("repeated value was redrawn")
	}
	d.SetValue(50.04)
	if d.displayText.Text != "50.0" {
		t.Errorf("readout %q, want 50.0", d.displayText.Text)
	}
	if d.value != 50.04 || d.valueArc.EndAngle <= 0 {
		t.Errorf("value %v arc end %v, want 50.04 and past 0°", d.value, d.valueArc.EndAngle)
	}
}

// Layout centers a square dial in the space: ring, title below the middle,
// readout over the middle (modern) or lower down (classic), and pips and
// labels on their radii at -135°..135°.
func TestLayout(t *testing.T) {
	const w, h = 300, 200 // diameter 200, radius 100, middle (150,100)
	mid := fyne.NewPos(150, 100)
	for _, tc := range []struct {
		classic                bool
		majorIn, minorIn, tipR float64
		displayY               float32
	}{
		{false, 72, 79, 85, 0},
		{true, 75, 87.5, 98, 40},
	} {
		d, _ := render(t, cfg100(tc.classic), fyne.NewSize(w, h))
		if d.middle != mid || d.radius != 100 {
			t.Fatalf("classic=%v: middle %v radius %v, want %v 100", tc.classic, d.middle, d.radius, mid)
		}
		if d.titleText.TextSize != 25 || d.titleText.Position() != fyne.NewPos(150, 150) {
			t.Errorf("classic=%v: title size %v at %v, want 25 at (150,150)", tc.classic, d.titleText.TextSize, d.titleText.Position())
		}
		if d.displayText.TextSize != 33 || d.displayText.Position() != fyne.NewPos(50, tc.displayY) || d.displayText.Size() != fyne.NewSize(200, 200) {
			t.Errorf("classic=%v: readout size %v at %v box %v, want 33 at (50,%v) box 200x200",
				tc.classic, d.displayText.TextSize, d.displayText.Position(), d.displayText.Size(), tc.displayY)
		}
		if d.face.Position() != fyne.NewPos(50, 0) || d.face.Size() != fyne.NewSize(200, 200) {
			t.Errorf("classic=%v: ring at %v size %v, want (50,0) 200x200", tc.classic, d.face.Position(), d.face.Size())
		}
		if tc.classic {
			if d.center.Position() != fyne.NewPos(137.5, 87.5) || d.center.Size() != fyne.NewSize(25, 25) {
				t.Errorf("hub at %v size %v, want (137.5,87.5) 25x25", d.center.Position(), d.center.Size())
			}
		} else {
			corner := float32(100 * (1 - widgets.DialRingCutout) / 2)
			if d.face.CornerRadius != corner || d.valueArc.CornerRadius != corner {
				t.Errorf("ring corners %v/%v, want %v", d.face.CornerRadius, d.valueArc.CornerRadius, corner)
			}
			if d.valueArc.Position() != d.face.Position() || d.valueArc.Size() != d.face.Size() {
				t.Errorf("value arc at %v size %v, want on the ring", d.valueArc.Position(), d.valueArc.Size())
			}
		}

		labelBox := fyne.NewSize(3*0.62*10, 1.15*10) // "100" at text size r/10
		for i, p := range d.pips {
			deg := -135 + 270*float64(i)/10
			in, width := tc.minorIn, float32(2)
			if i%2 == 0 {
				in, width = tc.majorIn, 2.5
			}
			checkPolar(t, "pip inner end", mid, p.Position1, in, deg)
			checkPolar(t, "pip outer end", mid, p.Position2, tc.tipR, deg)
			if p.StrokeWidth != width {
				t.Errorf("classic=%v pip %d width %v, want %v", tc.classic, i, p.StrokeWidth, width)
			}
			if l := d.pipLabels[i]; l != nil {
				if l.TextSize != 10 || !near(float64(l.Size().Width), float64(labelBox.Width)) || !near(float64(l.Size().Height), float64(labelBox.Height)) {
					t.Errorf("label %d size %v box %v, want 10 and %v", i, l.TextSize, l.Size(), labelBox)
				}
				c := l.Position().Add(fyne.NewPos(l.Size().Width/2, l.Size().Height/2))
				checkPolar(t, "label center", mid, c, tc.majorIn-14, deg)
			}
		}
	}
}

// A value set before the first layout is drawn once the dial is laid out.
func TestSetValueBeforeLayout(t *testing.T) {
	test.NewTempApp(t)
	classic := New(cfg100(true))
	modern := New(cfg100(false))
	classic.SetValue(100)
	modern.SetValue(100)
	test.TempWidgetRenderer(t, classic)
	test.TempWidgetRenderer(t, modern)
	classic.Resize(fyne.NewSize(200, 200))
	modern.Resize(fyne.NewSize(200, 200))
	checkNeedle(t, classic, 135)
	if modern.valueArc.EndAngle != 135 {
		t.Errorf("value arc ends at %v°, want 135", modern.valueArc.EndAngle)
	}
	if classic.displayText.Text != "100" || modern.displayText.Text != "100" {
		t.Errorf("readouts %q/%q, want 100", classic.displayText.Text, modern.displayText.Text)
	}
}

// A resize recenters the dial and turns the needle on the new radius.
func TestResize(t *testing.T) {
	d, _ := render(t, cfg100(true), fyne.NewSize(200, 200))
	d.SetValue(50)
	d.Resize(fyne.NewSize(400, 600))
	if d.middle != fyne.NewPos(200, 300) || d.radius != 200 {
		t.Errorf("middle %v radius %v, want (200,300) 200", d.middle, d.radius)
	}
	checkNeedle(t, d, 0)
}

// The dial builds its canvas objects once in New, so a renderer rebuilt after
// Fyne drops a hidden widget's renderer reuses them, already laid out.
func TestRebuiltRendererReusesObjects(t *testing.T) {
	d, r := render(t, cfg100(true), fyne.NewSize(200, 200))
	d.SetValue(75)
	r2 := d.CreateRenderer()
	r2.Layout(d.Size())
	checkObjects(t, r2.Objects(), r.Objects())
	checkNeedle(t, d, 67.5)
}

// Like every other gauge, New defaults Steps, so a config with Steps 0
// (possible from the gauge creator or a saved layout) gets a finite scale
// with no NaN pip or "NaN" label.
func TestZeroStepsHasFiniteScale(t *testing.T) {
	d, _ := render(t, &widgets.GaugeConfig{Max: 100}, fyne.NewSize(200, 200))
	for i, p := range d.pips {
		for _, v := range []float32{p.Position1.X, p.Position1.Y, p.Position2.X, p.Position2.Y} {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Errorf("pip %d at %v-%v", i, p.Position1, p.Position2)
				break
			}
		}
	}
	for i, l := range d.pipLabels {
		if l != nil && (l.Text == "NaN" || l.Text == "+Inf") {
			t.Errorf("label %d reads %q", i, l.Text)
		}
	}
}

// A range with Max at or below Min does not divide by zero: the value arc is
// empty below Min and full above it, and the classic needle rests at -135° on
// Min.
func TestDegenerateRange(t *testing.T) {
	d, _ := render(t, &widgets.GaugeConfig{Min: 50, Max: 50, Steps: 10}, fyne.NewSize(200, 200))
	for _, tc := range []struct {
		value float64
		end   float32
	}{{60, 135}, {40, -135}} {
		d.SetValue(tc.value)
		if d.valueArc.EndAngle != tc.end {
			t.Errorf("Min=Max=50 value %v: arc ends at %v°, want %v°", tc.value, d.valueArc.EndAngle, tc.end)
		}
	}
	c, _ := render(t, &widgets.GaugeConfig{Min: 50, Max: 50, Steps: 10, Classic: true}, fyne.NewSize(200, 200))
	c.SetValue(50)
	checkNeedle(t, c, -135)
}

// The gauge creator's Steps field accepts "-"; a Steps of -2 or less gets the
// default instead of sizing the pip table from it.
func TestNegativeStepsDoesNotPanic(t *testing.T) {
	test.NewTempApp(t)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Dial with Steps -2 panicked: %v", r)
		}
	}()
	d := New(&widgets.GaugeConfig{Max: 100, Steps: -2})
	test.TempWidgetRenderer(t, d)
	d.Resize(fyne.NewSize(200, 200))
}
