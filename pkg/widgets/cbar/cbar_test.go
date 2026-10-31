package cbar

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
)

// render builds a CBar from cfg, creates its renderer and lays it out at size.
func render(t *testing.T, cfg *widgets.GaugeConfig, size fyne.Size) (*CBar, fyne.WidgetRenderer) {
	t.Helper()
	test.NewTempApp(t)
	c := New(cfg)
	r := test.TempWidgetRenderer(t, c)
	c.Resize(size)
	return c, r
}

func cfg100(classic bool) *widgets.GaugeConfig {
	return &widgets.GaugeConfig{Min: 0, Max: 100, Center: 50, Steps: 10, Classic: classic}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

func checkRect(t *testing.T, what string, r *canvas.Rectangle, x, w, h float32) {
	t.Helper()
	if p, s := r.Position(), r.Size(); !near(p.X, x) || p.Y != 0 || !near(s.Width, w) || !near(s.Height, h) {
		t.Errorf("%s at %v size %v, want x=%v y=0 size %vx%v", what, p, s, x, w, h)
	}
}

// New fills in the defaults a zero config leaves out and keeps the values the
// caller set; the renderer reports cfg.MinSize.
func TestNewDefaults(t *testing.T) {
	test.NewTempApp(t)
	for _, tc := range []struct {
		name    string
		in, out widgets.GaugeConfig
	}{
		{"zero", widgets.GaugeConfig{},
			widgets.GaugeConfig{MinSize: fyne.NewSize(50, 50), Steps: 10, DisplayString: "%.0f", DisplayTextSize: 25}},
		{"negative steps", widgets.GaugeConfig{Steps: -3},
			widgets.GaugeConfig{MinSize: fyne.NewSize(50, 50), Steps: 10, DisplayString: "%.0f", DisplayTextSize: 25}},
		{"set", widgets.GaugeConfig{MinSize: fyne.NewSize(120, 30), Steps: 4, DisplayString: "%.2f", DisplayTextSize: 12},
			widgets.GaugeConfig{MinSize: fyne.NewSize(120, 30), Steps: 4, DisplayString: "%.2f", DisplayTextSize: 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.in
			c := New(&cfg)
			if c.GetConfig() != &cfg {
				t.Fatal("GetConfig does not return the config New was given")
			}
			if cfg != tc.out {
				t.Errorf("config after New = %+v, want %+v", cfg, tc.out)
			}
			if got := test.TempWidgetRenderer(t, c).MinSize(); got != tc.out.MinSize {
				t.Errorf("MinSize = %v, want %v", got, tc.out.MinSize)
			}
			if len(c.bars) != tc.out.Steps+1 {
				t.Errorf("%d ticks, want %d", len(c.bars), tc.out.Steps+1)
			}
		})
	}
}

// Until the first sample the readout shows zero in the configured format.
func TestInitialReadout(t *testing.T) {
	for _, tc := range []struct{ format, want string }{
		{"", "0"},
		{"%.2f", "0.00"},
		{"λ %.2f", "λ 0.00"},
		{"%.1f%%", "0.0%"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			cfg := cfg100(false)
			cfg.DisplayString = tc.format
			c, _ := render(t, cfg, fyne.NewSize(200, 40))
			if c.displayText.Text != tc.want {
				t.Errorf("readout %q, want %q", c.displayText.Text, tc.want)
			}
		})
	}
}

// SetValue formats the readout with DisplayString: plain "%.Nf" formats and
// formats with extra text both work.
func TestReadoutFormat(t *testing.T) {
	for _, tc := range []struct {
		format string
		value  float64
		want   string
	}{
		{"", 42.7, "43"},
		{"%.2f", 0.956, "0.96"},
		{"λ %.2f", 0.95, "λ 0.95"},
		{"%.2f%%", -3.14159, "-3.14%"},
		{"%5.1f", 7.25, "  7.2"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			cfg := &widgets.GaugeConfig{Min: -100, Max: 100, DisplayString: tc.format}
			c, _ := render(t, cfg, fyne.NewSize(200, 40))
			c.SetValue(tc.value)
			if c.displayText.Text != tc.want {
				t.Errorf("SetValue(%v) readout %q, want %q", tc.value, c.displayText.Text, tc.want)
			}
		})
	}
}

// The bar grows from the middle toward the value: left and green below
// Center, right and red above it, a 6px amber sliver on it. Values past
// Min/Max are clamped, readout included.
func TestBarGeometry(t *testing.T) {
	c, _ := render(t, cfg100(false), fyne.NewSize(200, 40)) // 2px per unit
	for _, tc := range []struct {
		value   float64
		x, w    float32
		color   color.NRGBA
		readout string
	}{
		{50, 97, 6, c.colorCenter, "50"},
		{25, 50, 50, c.colorLow, "25"},
		{75, 100, 50, c.colorHigh, "75"},
		{0, 0, 100, c.colorLow, "0"},
		{100, 100, 100, c.colorHigh, "100"},
		{-20, 0, 100, c.colorLow, "0"},
		{140, 100, 100, c.colorHigh, "100"},
	} {
		c.SetValue(tc.value)
		checkRect(t, "bar", c.bar, tc.x, tc.w, 40)
		if c.bar.FillColor != tc.color {
			t.Errorf("SetValue(%v): bar color %v, want %v", tc.value, c.bar.FillColor, tc.color)
		}
		if c.displayText.Text != tc.readout {
			t.Errorf("SetValue(%v): readout %q, want %q", tc.value, c.displayText.Text, tc.readout)
		}
	}
}

// A sample sent before the first render is what the new renderer draws.
func TestSetValueBeforeRender(t *testing.T) {
	test.NewTempApp(t)
	c := New(cfg100(false))
	c.SetValue(73.4)
	test.TempWidgetRenderer(t, c)
	c.Resize(fyne.NewSize(200, 40))
	if c.displayText.Text != "73" {
		t.Errorf("readout %q, want 73", c.displayText.Text)
	}
	checkRect(t, "bar", c.bar, 100, 46.8, 40)
	if c.bar.FillColor != c.colorHigh {
		t.Errorf("bar color %v, want %v", c.bar.FillColor, c.colorHigh)
	}
}

// Modern draws a rounded track under the bar with neutral ticks on top;
// classic draws the bar under a thin outline with green ticks and translucent
// bar colors.
func TestStyles(t *testing.T) {
	const w, h = 200, 40
	for _, tc := range []struct {
		classic              bool
		low, high, center    color.NRGBA
		majorTick, minorTick color.Color
		peak                 color.NRGBA
		text                 color.Color
	}{
		{false,
			color.NRGBA{0x26, 0xCC, 0x00, 0xFF}, color.NRGBA{0xD0, 0x32, 0x2A, 0xFF}, color.NRGBA{0xFC, 0xBA, 0x03, 0xFF},
			widgets.TickMajor, widgets.TickMinor, color.NRGBA{0xFF, 0xFF, 0xFF, 0xE0}, widgets.TextPrimary},
		{true,
			color.NRGBA{0x26, 0xCC, 0x00, 0x80}, color.NRGBA{0xA5, 0x00, 0x00, 0x80}, color.NRGBA{0xFC, 0xBA, 0x03, 0x80},
			color.RGBA{0x00, 0xE5, 0x00, 0xFF}, color.RGBA{0x00, 0xE5, 0x00, 0xFF}, color.NRGBA{0xFF, 0xFF, 0xFF, 0xA0},
			color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}},
	} {
		name := "modern"
		if tc.classic {
			name = "classic"
		}
		t.Run(name, func(t *testing.T) {
			cfg := cfg100(tc.classic)
			cfg.Title = "AFR"
			c, r := render(t, cfg, fyne.NewSize(w, h))
			if c.colorLow != tc.low || c.colorHigh != tc.high || c.colorCenter != tc.center {
				t.Errorf("bar colors low/high/center = %v/%v/%v, want %v/%v/%v",
					c.colorLow, c.colorHigh, c.colorCenter, tc.low, tc.high, tc.center)
			}

			objs := r.Objects()
			var want []fyne.CanvasObject
			if !tc.classic {
				want = append(want, c.face, c.bar)
			}
			for _, l := range c.bars {
				want = append(want, l)
			}
			if tc.classic {
				want = append(want, c.bar, c.face)
			}
			want = append(want, c.peakLo, c.peakHi, c.titleText, c.displayText)
			if len(objs) != len(want) {
				t.Fatalf("%d objects, want %d", len(objs), len(want))
			}
			for i := range want {
				if objs[i] != want[i] {
					t.Errorf("object %d is %T, want %T", i, objs[i], want[i])
				}
			}

			for i, l := range c.bars {
				wantTick := tc.majorTick
				if i%2 != 0 {
					wantTick = tc.minorTick
				}
				if l.StrokeColor != wantTick {
					t.Errorf("tick %d color %v, want %v", i, l.StrokeColor, wantTick)
				}
			}
			if c.peakLo.FillColor != tc.peak || c.peakHi.FillColor != tc.peak {
				t.Errorf("peak colors %v/%v, want %v", c.peakLo.FillColor, c.peakHi.FillColor, tc.peak)
			}
			if c.displayText.Color != tc.text || c.titleText.Color != tc.text {
				t.Errorf("text colors %v/%v, want %v", c.titleText.Color, c.displayText.Color, tc.text)
			}
			if c.titleText.Text != "AFR" {
				t.Errorf("title %q, want AFR", c.titleText.Text)
			}
			wantTitle := fyne.NewPos(w/2-c.titleText.MinSize().Width/2, h-30)
			if p := c.titleText.Position(); !near(p.X, wantTitle.X) || p.Y != wantTitle.Y {
				t.Errorf("title at %v, want %v", p, wantTitle)
			}

			if tc.classic {
				if c.face.StrokeWidth != 3 || c.face.StrokeColor != (color.RGBA{0x80, 0x80, 0x80, 0xFF}) {
					t.Errorf("classic outline stroke %v width %v", c.face.StrokeColor, c.face.StrokeWidth)
				}
				if p, s := c.face.Position(), c.face.Size(); p != fyne.NewPos(-2, 0) || s != fyne.NewSize(w+3, h) {
					t.Errorf("classic outline at %v size %v, want (-2,0) %vx%v", p, s, w+3, h)
				}
				if c.bar.CornerRadius != 0 {
					t.Errorf("classic bar corner radius %v, want 0", c.bar.CornerRadius)
				}
			} else {
				if c.face.FillColor != widgets.TrackColor {
					t.Errorf("track color %v, want %v", c.face.FillColor, widgets.TrackColor)
				}
				if p, s := c.face.Position(), c.face.Size(); p != (fyne.Position{}) || s != fyne.NewSize(w, h) {
					t.Errorf("track at %v size %v, want origin %vx%v", p, s, w, h)
				}
				radius := float32(h * widgets.BarCornerFrac)
				if c.face.CornerRadius != radius || c.bar.CornerRadius != radius {
					t.Errorf("corner radius track %v bar %v, want %v", c.face.CornerRadius, c.bar.CornerRadius, radius)
				}
			}

			c.SetValue(25)
			if c.bar.FillColor != tc.low {
				t.Errorf("below center: bar color %v, want %v", c.bar.FillColor, tc.low)
			}
			c.SetValue(75)
			if c.bar.FillColor != tc.high {
				t.Errorf("above center: bar color %v, want %v", c.bar.FillColor, tc.high)
			}
		})
	}
}

// Ticks are spread evenly across the width; even ticks span 2/3 of the
// height, odd ones 2/7, both centered vertically. A resize re-spreads them.
func TestTickLayout(t *testing.T) {
	c, _ := render(t, cfg100(false), fyne.NewSize(200, 60))
	check := func(w, h float32) {
		t.Helper()
		for i, l := range c.bars {
			x := float32(i) * w / 10
			half := h / 3
			if i%2 != 0 {
				half = h / 7
			}
			if !near(l.Position1.X, x) || !near(l.Position2.X, x) ||
				!near(l.Position1.Y, h/2-half) || !near(l.Position2.Y, h/2+half) {
				t.Errorf("%vx%v: tick %d from %v to %v, want x=%v y=%v..%v",
					w, h, i, l.Position1, l.Position2, x, h/2-half, h/2+half)
			}
		}
	}
	check(200, 60)
	c.Resize(fyne.NewSize(400, 90))
	check(400, 90)
}

// A resize re-lays the bar for the value on screen and scales the readout
// with the bar height.
func TestResize(t *testing.T) {
	c, _ := render(t, cfg100(false), fyne.NewSize(200, 40))
	c.SetValue(75)
	if c.displayText.TextSize != 32 {
		t.Errorf("readout text size %v, want 32 (bar height - 8)", c.displayText.TextSize)
	}
	c.Resize(fyne.NewSize(400, 80))
	checkRect(t, "bar", c.bar, 200, 100, 80)
	if c.displayText.TextSize != 72 {
		t.Errorf("readout text size %v after resize, want 72", c.displayText.TextSize)
	}
	if c.bar.CornerRadius != 20 {
		t.Errorf("bar corner radius %v after resize, want 20", c.bar.CornerRadius)
	}
}

// The readout keeps its decimal point on the widget's horizontal center, or
// its middle when there is none; TextAtBottom puts it under the bar,
// TextAtTop above it with its bottom edge one pixel over the bar.
func TestReadoutPosition(t *testing.T) {
	const w, h = 300, 40
	for _, tc := range []struct {
		format string
		pos    widgets.TextPosition
		values []float64
	}{
		{"%.2f", widgets.TextAtBottom, []float64{1.23, -12.5, 99.99}},
		{"%.0f", widgets.TextAtTop, []float64{7, 42, -100}},
	} {
		t.Run(tc.format, func(t *testing.T) {
			cfg := &widgets.GaugeConfig{Min: -100, Max: 100, DisplayString: tc.format, TextPosition: tc.pos}
			c, _ := render(t, cfg, fyne.NewSize(w, h))
			wantY := float32(h - 2)
			if tc.pos == widgets.TextAtTop {
				wantY = -c.displayText.MinSize().Height - 1
			}
			for _, v := range tc.values {
				c.SetValue(v)
				txt := c.displayText
				measure := func(s string) float32 { return fyne.MeasureText(s, txt.TextSize, txt.TextStyle).Width }
				var mid float32
				if i := strings.IndexByte(txt.Text, '.'); i >= 0 {
					mid = txt.Position().X + measure(txt.Text[:i]) + measure(".")/2
				} else {
					mid = txt.Position().X + measure(txt.Text)/2
				}
				if math.Abs(float64(mid-w/2)) > 0.5 {
					t.Errorf("%q: anchor at x=%v, want %v", txt.Text, mid, w/2)
				}
				if txt.Position().Y != wantY {
					t.Errorf("%q: y=%v, want %v", txt.Text, txt.Position().Y, wantY)
				}
			}
		})
	}
}

// The peak markers latch the furthest excursion on each side of Center and
// bleed back toward it by 2% of the distance per sample; each is a thin
// full-height marker kept inside the widget.
func TestPeakMarkers(t *testing.T) {
	const w, h = 250, 40 // 2.5px per unit, center at 125
	c, _ := render(t, cfg100(false), fyne.NewSize(w, h))
	mw := float32(2) // max(2, w*0.008)
	at := func(v float64) float32 { return 125 + float32(v-50)*2.5 - mw/2 }

	c.SetValue(80)
	checkRect(t, "high peak", c.peakHi, at(80), mw, h)
	checkRect(t, "low peak", c.peakLo, at(50), mw, h)

	c.SetValue(20)
	checkRect(t, "low peak", c.peakLo, at(20), mw, h)
	hi := 50 + 30*0.98 // 2% of the 30 units back toward Center
	checkRect(t, "decayed high peak", c.peakHi, at(hi), mw, h)

	// Holding the value on Center keeps both markers bleeding back: after n
	// more samples each sits 0.98^n of its distance from Center.
	for range 100 {
		c.SetValue(50)
	}
	lo := 50 - 30*math.Pow(0.98, 100)
	hi = 50 + 30*math.Pow(0.98, 101)
	if math.Abs(c.peakLoVal-lo) > 1e-9 || math.Abs(c.peakHiVal-hi) > 1e-9 {
		t.Errorf("after 100 samples on center: peaks %v/%v, want %v/%v", c.peakLoVal, c.peakHiVal, lo, hi)
	}
	checkRect(t, "low peak", c.peakLo, at(lo), mw, h)

	// At the ends the markers stay inside the widget.
	c.SetValue(100)
	checkRect(t, "high peak at Max", c.peakHi, w-mw, mw, h)
	c.SetValue(0)
	checkRect(t, "low peak at Min", c.peakLo, 0, mw, h)

	// The marker width grows with the widget.
	c.Resize(fyne.NewSize(1000, h))
	if got := c.peakLo.Size().Width; got != 8 {
		t.Errorf("peak width %v at 1000px, want 8", got)
	}
}

// The first sample always draws, even when it equals Center; after that a
// repeated sample with the peaks at rest does not reach the canvas.
func TestRepeatedSampleDropped(t *testing.T) {
	c, _ := render(t, cfg100(false), fyne.NewSize(200, 40))
	c.SetValue(50)
	if c.displayText.Text != "50" {
		t.Fatalf("first sample on Center: readout %q, want 50", c.displayText.Text)
	}
	c.value = -1 // sentinel: apply overwrites it
	c.SetValue(50)
	if c.value != -1 {
		t.Error("repeated sample with resting peaks was redrawn")
	}
	c.SetValue(60)
	if c.value != 60 {
		t.Errorf("new sample: value on screen %v, want 60", c.value)
	}
}

// The bar is anchored at Center's position on the Min..Max scale, not at the
// widget's pixel midpoint. The gauge creator lets the user pick any Center for
// a CBar; at Max the bar ends at the right edge and at Min starts at the left
// edge.
func TestBarAnchoredAtCenterValue(t *testing.T) {
	cfg := &widgets.GaugeConfig{Min: 0, Max: 100, Center: 20, Steps: 10}
	c, _ := render(t, cfg, fyne.NewSize(200, 40))
	c.SetValue(100)
	if right := c.bar.Position().X + c.bar.Size().Width; !near(right, 200) {
		t.Errorf("bar at Max ends at x=%v, want 200", right)
	}
	c.SetValue(0)
	if left := c.bar.Position().X; !near(left, 0) {
		t.Errorf("bar at Min starts at x=%v, want 0", left)
	}
}

// The readout keeps its decimal point on the widget's center also with a
// multi-byte prefix such as the dashboard's "λ %.2f".
func TestReadoutCenteredWithUnicodePrefix(t *testing.T) {
	const w = 300
	cfg := &widgets.GaugeConfig{Min: 0.5, Max: 1.5, Center: 1, DisplayString: "λ %.2f"}
	c, _ := render(t, cfg, fyne.NewSize(w, 40))
	c.SetValue(0.95)
	txt := c.displayText
	measure := func(s string) float32 { return fyne.MeasureText(s, txt.TextSize, txt.TextStyle).Width }
	i := strings.IndexByte(txt.Text, '.')
	mid := txt.Position().X + measure(txt.Text[:i]) + measure(".")/2
	if math.Abs(float64(mid-w/2)) > 0.5 {
		t.Errorf("%q: decimal point at x=%v, want %v", txt.Text, mid, w/2)
	}
}

// A peak marker is the furthest excursion, so it never sits inside the
// current value; holding a value at the latched peak keeps the marker there
// instead of decaying it 2% inward on every other sample, and a marker
// decaying onto a held value stops at it.
func TestPeakHoldsAtSteadyValue(t *testing.T) {
	c, _ := render(t, cfg100(false), fyne.NewSize(200, 40))
	for i := range 4 {
		c.SetValue(80)
		if c.peakHiVal != 80 {
			t.Errorf("sample %d at 80: high peak %v, want 80", i, c.peakHiVal)
		}
	}

	c.SetValue(90)
	c.SetValue(10)
	for i := range 100 {
		c.SetValue(80)
		if c.peakHiVal < 80 {
			t.Fatalf("sample %d at 80 after 90: high peak %v, inside the value", i, c.peakHiVal)
		}
		if c.peakLoVal > 50 {
			t.Fatalf("sample %d at 80: low peak %v, past center", i, c.peakLoVal)
		}
	}
	if c.peakHiVal != 80 {
		t.Errorf("after 100 samples at 80: high peak %v, want 80", c.peakHiVal)
	}
}

// Fyne destroys the renderer of a widget hidden for a while and builds a new
// one when it shows again, laying it out at the unchanged size. The new
// renderer's canvas objects get laid out and it holds one set of ticks.
func TestRebuiltRendererIsLaidOut(t *testing.T) {
	c, _ := render(t, cfg100(false), fyne.NewSize(200, 40))
	c.SetValue(75)

	r := c.CreateRenderer()
	r.Layout(c.Size())
	checkRect(t, "bar after rebuild", c.bar, 100, 50, 40)
	if got, want := len(r.Objects()), c.cfg.Steps+1+6; got != want {
		t.Errorf("%d objects after rebuild, want %d", got, want)
	}
}
