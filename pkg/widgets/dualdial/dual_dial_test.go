package dualdial

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

// render builds a DualDial from cfg, creates its renderer and lays it out at size.
func render(t *testing.T, cfg *widgets.GaugeConfig, size fyne.Size) (*DualDial, fyne.WidgetRenderer) {
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

var secondaryRed = color.RGBA{R: 249, G: 27, B: 2, A: 255}

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

// checkNeedle verifies a classic needle points at deg: its tip 0.99r out, its
// tail 0.15r behind the hub.
func checkNeedle(t *testing.T, d *DualDial, n *canvas.Line, deg float64) {
	t.Helper()
	r := float64(d.radius)
	checkPolar(t, "needle tail", d.middle, n.Position1, 0.15*r, deg+180)
	checkPolar(t, "needle tip", d.middle, n.Position2, 0.99*r, deg)
}

func checkArc(t *testing.T, what string, a *canvas.Arc, start, end, cutout float32, c color.Color) {
	t.Helper()
	if a.StartAngle != start || a.EndAngle != end || !near(float64(a.CutoutRatio), float64(cutout)) || a.FillColor != c {
		t.Errorf("%s: %v..%v° cutout %v color %v, want %v..%v° cutout %v color %v",
			what, a.StartAngle, a.EndAngle, a.CutoutRatio, a.FillColor, start, end, cutout, c)
	}
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

// pipsAndLabels lists the pips followed by the labels, as the renderer stacks them.
func pipsAndLabels(d *DualDial) []fyne.CanvasObject {
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

// New defaults to 10 steps, a "%.0f" readout and a 100x100 minimum size; a
// MinSize is only used when both dimensions are set.
func TestNewDefaults(t *testing.T) {
	test.NewTempApp(t)
	for _, tc := range []struct {
		name      string
		in        widgets.GaugeConfig
		wantSteps int
		wantMin   fyne.Size
	}{
		{"zero", widgets.GaugeConfig{Max: 100}, 10, fyne.NewSize(100, 100)},
		{"width only", widgets.GaugeConfig{Max: 100, Steps: 4, MinSize: fyne.NewSize(150, 0)}, 4, fyne.NewSize(100, 100)},
		{"both set", widgets.GaugeConfig{Max: 100, Steps: 20, MinSize: fyne.NewSize(150, 120)}, 20, fyne.NewSize(150, 120)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.in
			d := New(&cfg)
			if d.GetConfig() != &cfg {
				t.Fatal("GetConfig does not return the config New was given")
			}
			if len(d.pips) != tc.wantSteps+1 {
				t.Errorf("%d pips, want %d", len(d.pips), tc.wantSteps+1)
			}
			if got := test.TempWidgetRenderer(t, d).MinSize(); got != tc.wantMin {
				t.Errorf("MinSize = %v, want %v", got, tc.wantMin)
			}
			if d.displayString != "%.0f" || d.displayText.Text != "0" || d.displayText2.Text != "0" {
				t.Errorf("format %q readouts %q/%q, want %%.0f and 0/0", d.displayString, d.displayText.Text, d.displayText2.Text)
			}
		})
	}
}

// Modern draws two track rings: the inner, thicker one fills zone-colored
// with the primary value, the outer, thinner one red with the secondary.
// Classic draws a gray rim, gradient pips, a hub and two needles.
func TestStyles(t *testing.T) {
	t.Run("modern", func(t *testing.T) {
		d, r := render(t, cfg100(false), fyne.NewSize(200, 200))
		if d.needle != nil || d.needle2 != nil || d.center != nil {
			t.Error("modern dual dial has needles or a hub")
		}
		checkArc(t, "inner track", d.face, -135, 135, innerCutout, widgets.TrackColor)
		checkArc(t, "inner value arc", d.valueArc, -135, -135, innerCutout, widgets.ZoneColor(0))
		checkArc(t, "outer track", d.face2, -135, 135, outerCutout, widgets.TrackColor)
		checkArc(t, "outer value arc", d.valueArc2, -135, -135, outerCutout, secondaryRed)
		if d.displayText.Color != widgets.TextPrimary || d.displayText2.Color != (color.RGBA{0xFF, 0, 0, 0xFF}) {
			t.Errorf("readout colors %v/%v", d.displayText.Color, d.displayText2.Color)
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
		want := append([]fyne.CanvasObject{d.face, d.valueArc, d.face2, d.valueArc2}, pipsAndLabels(d)...)
		want = append(want, d.titleText, d.displayText, d.displayText2)
		checkObjects(t, r.Objects(), want)
	})
	t.Run("classic", func(t *testing.T) {
		d, r := render(t, cfg100(true), fyne.NewSize(200, 200))
		if d.valueArc != nil || d.valueArc2 != nil || d.face2 != nil {
			t.Error("classic dual dial has rings")
		}
		checkArc(t, "rim", d.face, -135.73, 135.8, 0.985, color.RGBA{0x80, 0x80, 0x80, 0xFF})
		if d.needle.StrokeColor != (color.RGBA{0xFF, 0x67, 0, 0xFF}) || d.needle2.StrokeColor != secondaryRed {
			t.Errorf("needle colors %v/%v", d.needle.StrokeColor, d.needle2.StrokeColor)
		}
		if d.displayText.Color != (color.RGBA{0x2c, 0xfc, 0x03, 0xFF}) {
			t.Errorf("readout color %v", d.displayText.Color)
		}
		for i, p := range d.pips {
			if want := widgets.ZoneColor(float64(i) / 10); p.StrokeColor != want {
				t.Errorf("pip %d color %v, want %v", i, p.StrokeColor, want)
			}
		}
		want := append(pipsAndLabels(d), d.face, d.titleText, d.center, d.needle2, d.needle, d.displayText, d.displayText2)
		checkObjects(t, r.Objects(), want)
	})
}

// Every second pip carries a scale label at Min + i/Steps of the range, with
// GaugeTextString's precision (whole numbers by default).
func TestPipLabels(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  widgets.GaugeConfig
		want []string
	}{
		{"air mass", widgets.GaugeConfig{Max: 2500, Steps: 20},
			[]string{"0", "250", "500", "750", "1000", "1250", "1500", "1750", "2000", "2250", "2500"}},
		{"pressure", widgets.GaugeConfig{Max: 3, Steps: 30, GaugeTextString: "%.1f"},
			[]string{"0.0", "0.2", "0.4", "0.6", "0.8", "1.0", "1.2", "1.4", "1.6", "1.8", "2.0",
				"2.2", "2.4", "2.6", "2.8", "3.0"}},
		{"offset", widgets.GaugeConfig{Min: -1, Max: 1, Steps: 4, GaugeTextString: "%.1f"},
			[]string{"-1.0", "0.0", "1.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test.NewTempApp(t)
			cfg := tc.cfg
			d := New(&cfg)
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

// SetValue drives the inner arc (zone colored) and the primary readout,
// SetValue2 the outer arc (always red) and the secondary readout; both sweep
// from -135° by the value's fraction of the range, clamped at Min/Max.
func TestValueArcs(t *testing.T) {
	d, _ := render(t, cfg100(false), fyne.NewSize(200, 200))
	for _, tc := range []struct {
		v1, v2     float64
		end1, end2 float32
		zone       color.RGBA
	}{
		{50, 100, 0, 135, color.RGBA{255, 255, 0, 255}},
		{25, 75, -67.5, 67.5, color.RGBA{127, 255, 0, 255}},
		{-10, 150, -135, 135, color.RGBA{0, 255, 0, 255}},
		{150, -10, 135, -135, color.RGBA{255, 0, 0, 255}},
	} {
		d.SetValue(tc.v1)
		d.SetValue2(tc.v2)
		checkArc(t, "inner value arc", d.valueArc, -135, tc.end1, innerCutout, tc.zone)
		checkArc(t, "outer value arc", d.valueArc2, -135, tc.end2, outerCutout, secondaryRed)
	}
	if d.displayText.Text != "150" || d.displayText2.Text != "-10" {
		t.Errorf("readouts %q/%q, want 150/-10", d.displayText.Text, d.displayText2.Text)
	}
}

// Each classic needle follows its own value through 270° and stays at Min
// below it.
func TestNeedles(t *testing.T) {
	d, _ := render(t, cfg100(true), fyne.NewSize(200, 200))
	if !near(float64(d.needle.StrokeWidth), 200.0/60) || d.needle2.StrokeWidth != d.needle.StrokeWidth {
		t.Errorf("needle widths %v/%v, want %v", d.needle.StrokeWidth, d.needle2.StrokeWidth, 200.0/60)
	}
	d.SetValue(50)
	checkNeedle(t, d, d.needle, 0)
	checkNeedle(t, d, d.needle2, -135) // untouched, at the zero value
	d.SetValue2(100)
	checkNeedle(t, d, d.needle, 0)
	checkNeedle(t, d, d.needle2, 135)
	d.SetValue(-30)
	d.SetValue2(25)
	checkNeedle(t, d, d.needle, -135)
	checkNeedle(t, d, d.needle2, -67.5)
}

// Neither needle wraps back onto the scale past Max: each points at Max or
// into the dead zone between 135° and 225°, so a value more than a third of
// the range past Max does not read as a low value.
func TestNeedlesClampedAtMax(t *testing.T) {
	d, _ := render(t, cfg100(true), fyne.NewSize(200, 200))
	d.SetValue(140)
	d.SetValue2(200)
	for _, n := range []struct {
		line *canvas.Line
		v    float64
	}{{d.needle, 140}, {d.needle2, 200}} {
		_, deg := polar(d.middle, n.line.Position2)
		if deg < 0 {
			deg += 360
		}
		if deg < 135-0.01 || deg > 225+0.01 {
			t.Errorf("value %v: needle at %.1f°, want 135°..225° (pinned at Max or in the dead zone)", n.v, deg)
		}
	}
}

// Both readouts use DisplayString; a repeated value is dropped, and a new
// value that formats the same keeps the text.
func TestReadouts(t *testing.T) {
	for _, tc := range []struct {
		format      string
		v1, v2      float64
		want, want2 string
	}{
		{"", 12.7, 3.2, "13", "3"},
		{"%.2f", 1.234, 0.5, "1.23", "0.50"},
		{"%.1f kPa", 101.33, 99.96, "101.3 kPa", "100.0 kPa"},
	} {
		cfg := cfg100(false)
		cfg.DisplayString = tc.format
		d, _ := render(t, cfg, fyne.NewSize(200, 200))
		d.SetValue(tc.v1)
		d.SetValue2(tc.v2)
		if d.displayText.Text != tc.want || d.displayText2.Text != tc.want2 {
			t.Errorf("format %q: readouts %q/%q, want %q/%q", tc.format, d.displayText.Text, d.displayText2.Text, tc.want, tc.want2)
		}
	}

	cfg := cfg100(false)
	cfg.DisplayString = "%.1f"
	d, _ := render(t, cfg, fyne.NewSize(200, 200))
	d.SetValue(40)
	d.SetValue2(60)
	d.value, d.value2 = -1, -1 // sentinels: apply/apply2 overwrite them
	d.SetValue(40)
	d.SetValue2(60)
	if d.value != -1 || d.value2 != -1 {
		t.Errorf("repeated values redrawn: %v/%v", d.value, d.value2)
	}
	d.SetValue(40.01)
	d.SetValue2(60.01)
	if d.displayText.Text != "40.0" || d.displayText2.Text != "60.0" || d.value != 40.01 || d.value2 != 60.01 {
		t.Errorf("readouts %q/%q values %v/%v, want 40.0/60.0 and 40.01/60.01",
			d.displayText.Text, d.displayText2.Text, d.value, d.value2)
	}
}

// Layout centers a square dial: title below the middle, the primary readout
// over the middle and the secondary above it, rings (modern) or rim and hub
// (classic) on the dial square, and pips and labels on their radii.
func TestLayout(t *testing.T) {
	const w, h = 300, 200 // diameter 200, radius 100, middle (150,100)
	mid := fyne.NewPos(150, 100)
	for _, tc := range []struct {
		classic                bool
		majorIn, minorIn, tipR float64
		textSize               float32
		y1, y2                 float32
	}{
		{false, 62, 68, 74, 25, 20, -200.0 / 12},
		{true, 75, 87.5, 99, 100.0 / 3, 40, -200.0 / 6},
	} {
		d, _ := render(t, cfg100(tc.classic), fyne.NewSize(w, h))
		if d.middle != mid || d.radius != 100 {
			t.Fatalf("classic=%v: middle %v radius %v, want %v 100", tc.classic, d.middle, d.radius, mid)
		}
		if d.titleText.TextSize != 25 || d.titleText.Position() != fyne.NewPos(150, 150) {
			t.Errorf("classic=%v: title size %v at %v, want 25 at (150,150)", tc.classic, d.titleText.TextSize, d.titleText.Position())
		}
		for _, x := range []struct {
			txt *canvas.Text
			y   float32
		}{{d.displayText, tc.y1}, {d.displayText2, tc.y2}} {
			p := x.txt.Position()
			if !near(float64(x.txt.TextSize), float64(tc.textSize)) || p.X != 50 || !near(float64(p.Y), float64(x.y)) || x.txt.Size() != fyne.NewSize(200, 200) {
				t.Errorf("classic=%v: readout %q size %v at %v box %v, want %v at (50,%v) box 200x200",
					tc.classic, x.txt.Text, x.txt.TextSize, p, x.txt.Size(), tc.textSize, x.y)
			}
		}
		if tc.classic {
			if d.face.Position() != fyne.NewPos(50, 0) || d.face.Size() != fyne.NewSize(200, 200) {
				t.Errorf("rim at %v size %v, want (50,0) 200x200", d.face.Position(), d.face.Size())
			}
			if d.center.Position() != fyne.NewPos(137.5, 87.5) || d.center.Size() != fyne.NewSize(25, 25) {
				t.Errorf("hub at %v size %v, want (137.5,87.5) 25x25", d.center.Position(), d.center.Size())
			}
		} else {
			for _, a := range []*canvas.Arc{d.face, d.valueArc} {
				if a.Position() != fyne.NewPos(61, 11) || a.Size() != fyne.NewSize(178, 178) || !near(float64(a.CornerRadius), 6.5) {
					t.Errorf("inner ring at %v size %v corner %v, want (61,11) 178x178 6.5", a.Position(), a.Size(), a.CornerRadius)
				}
			}
			for _, a := range []*canvas.Arc{d.face2, d.valueArc2} {
				if a.Position() != fyne.NewPos(50, 0) || a.Size() != fyne.NewSize(200, 200) || !near(float64(a.CornerRadius), 4) {
					t.Errorf("outer ring at %v size %v corner %v, want (50,0) 200x200 4", a.Position(), a.Size(), a.CornerRadius)
				}
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

// Values set before the first layout are drawn once the dial is laid out.
func TestSetValueBeforeLayout(t *testing.T) {
	test.NewTempApp(t)
	classic := New(cfg100(true))
	modern := New(cfg100(false))
	for _, d := range []*DualDial{classic, modern} {
		d.SetValue(100)
		d.SetValue2(50)
		test.TempWidgetRenderer(t, d)
		d.Resize(fyne.NewSize(200, 200))
	}
	checkNeedle(t, classic, classic.needle, 135)
	checkNeedle(t, classic, classic.needle2, 0)
	if modern.valueArc.EndAngle != 135 || modern.valueArc2.EndAngle != 0 {
		t.Errorf("arcs end at %v°/%v°, want 135/0", modern.valueArc.EndAngle, modern.valueArc2.EndAngle)
	}
	if modern.displayText.Text != "100" || modern.displayText2.Text != "50" {
		t.Errorf("readouts %q/%q, want 100/50", modern.displayText.Text, modern.displayText2.Text)
	}
}

// The dual dial builds its canvas objects once in New, so a renderer rebuilt
// after Fyne drops a hidden widget's renderer reuses them, already laid out.
func TestRebuiltRendererReusesObjects(t *testing.T) {
	d, r := render(t, cfg100(true), fyne.NewSize(200, 200))
	d.SetValue(75)
	r2 := d.CreateRenderer()
	r2.Layout(d.Size())
	checkObjects(t, r2.Objects(), r.Objects())
	checkNeedle(t, d, d.needle, 67.5)
}

// With Steps 0 New defaults the pip count to 10 and labels the scale from
// that same count.
func TestZeroStepsLabels(t *testing.T) {
	test.NewTempApp(t)
	d := New(&widgets.GaugeConfig{Max: 100})
	var got []string
	for _, l := range d.pipLabels {
		if l != nil {
			got = append(got, l.Text)
		}
	}
	if want := []string{"0", "20", "40", "60", "80", "100"}; !slices.Equal(got, want) {
		t.Errorf("labels %q, want %q", got, want)
	}
}

// A range with Max at or below Min does not divide by zero: each value arc is
// empty below Min and full above it.
func TestDegenerateRange(t *testing.T) {
	d, _ := render(t, &widgets.GaugeConfig{Min: 50, Max: 50, Steps: 10}, fyne.NewSize(200, 200))
	d.SetValue(60)
	d.SetValue2(60)
	if d.valueArc.EndAngle != 135 || d.valueArc2.EndAngle != 135 {
		t.Errorf("Min=Max=50 value 60: arcs end at %v°/%v°, want 135/135", d.valueArc.EndAngle, d.valueArc2.EndAngle)
	}
	d.SetValue(40)
	d.SetValue2(40)
	if d.valueArc.EndAngle != -135 || d.valueArc2.EndAngle != -135 {
		t.Errorf("Min=Max=50 value 40: arcs end at %v°/%v°, want -135/-135", d.valueArc.EndAngle, d.valueArc2.EndAngle)
	}
}
