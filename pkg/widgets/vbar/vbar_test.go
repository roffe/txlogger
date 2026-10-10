package vbar

import (
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
)

// render builds a VBar from cfg, creates its renderer and lays it out at size.
func render(t *testing.T, cfg *widgets.GaugeConfig, size fyne.Size) (*VBar, fyne.WidgetRenderer) {
	t.Helper()
	test.NewTempApp(t)
	v := New(cfg)
	r := test.TempWidgetRenderer(t, v)
	v.Resize(size)
	return v, r
}

func cfg100(classic bool) *widgets.GaugeConfig {
	return &widgets.GaugeConfig{Min: 0, Max: 100, Steps: 10, Classic: classic, MinSize: fyne.NewSize(50, 100)}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// checkBar verifies the bar fills norm of the band [barY, barY+barH] from the
// bottom up across the full width.
func checkBar(t *testing.T, v *VBar, w, barY, barH, norm float32) {
	t.Helper()
	h := norm * barH
	p, s := v.bar.Position(), v.bar.Size()
	if !near(s.Width, w) || !near(s.Height, h) || p.X != 0 || !near(p.Y, barY+barH-h) {
		t.Errorf("bar at %v size %v, want (0,%v) size %vx%v", p, s, barY+barH-h, w, h)
	}
}

// New defaults Steps to 10 and builds one color per integer value across
// floor(Min)..ceil(Max), keeping at least one for an empty or inverted range.
func TestNewColorCache(t *testing.T) {
	for _, tc := range []struct {
		name               string
		min, max           float64
		steps, wantSteps   int
		wantLen, wantFirst int
	}{
		{"0..100", 0, 100, 0, 10, 101, 0},
		{"fractional", -0.5, 10.2, 4, 4, 13, -1},
		{"empty", 5, 5, 0, 10, 1, 5},
		{"inverted", 10, 0, 0, 10, 1, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test.NewTempApp(t)
			cfg := &widgets.GaugeConfig{Min: tc.min, Max: tc.max, Steps: tc.steps}
			v := New(cfg)
			if v.GetConfig() != cfg {
				t.Fatal("GetConfig does not return the config New was given")
			}
			if cfg.Steps != tc.wantSteps {
				t.Errorf("Steps = %d, want %d", cfg.Steps, tc.wantSteps)
			}
			if len(v.fillCache) != tc.wantLen || len(v.strokeCache) != tc.wantLen || v.cacheMinInt != tc.wantFirst {
				t.Errorf("caches %d/%d from %d, want %d from %d",
					len(v.fillCache), len(v.strokeCache), v.cacheMinInt, tc.wantLen, tc.wantFirst)
			}
			if n := v.clampNorm(tc.max + 100); n != 1 && tc.max > tc.min {
				t.Errorf("clampNorm above Max = %v, want 1", n)
			}
			if n := v.clampNorm(tc.min - 100); n != 0 {
				t.Errorf("clampNorm below Min = %v, want 0", n)
			}
		})
	}
}

// The bar color follows the value: modern bars use the opaque green-yellow-red
// zone gradient, classic a translucent fill with an opaque stroke going green
// to red, and the blue-yellow scale replaces either for colorblind users.
func TestColorScales(t *testing.T) {
	for _, tc := range []struct {
		name    string
		classic bool
		scale   widgets.ColorScheme
		// fill/stroke at 0, 50, 100 on a 0..100 gauge
		fill, stroke [3]color.RGBA
	}{
		{"modern", false, widgets.TraditionalScale,
			[3]color.RGBA{{0, 255, 0, 255}, {255, 255, 0, 255}, {255, 0, 0, 255}},
			[3]color.RGBA{{0, 255, 0, 255}, {255, 255, 0, 255}, {255, 0, 0, 255}}},
		{"classic", true, widgets.TraditionalScale,
			[3]color.RGBA{{0, 0xA5, 0, 0x80}, {0x52, 0x52, 0, 0x80}, {0xA5, 0, 0, 0x80}},
			[3]color.RGBA{{0, 0xA5, 0, 0xFF}, {0x52, 0x52, 0, 0xFF}, {0xA5, 0, 0, 0xFF}}},
		{"classic blue-yellow", true, widgets.BlueYellowScale,
			[3]color.RGBA{{0, 0x77, 0xBB, 0x80}, {0xDD, 0xAA, 0x33, 0x80}, {0xCC, 0x33, 0x33, 0x80}},
			[3]color.RGBA{{0, 0x77, 0xBB, 0xFF}, {0xDD, 0xAA, 0x33, 0xFF}, {0xCC, 0x33, 0x33, 0xFF}}},
		{"modern blue-yellow", false, widgets.BlueYellowScale,
			[3]color.RGBA{{0, 0x77, 0xBB, 0xFF}, {0xDD, 0xAA, 0x33, 0xFF}, {0xCC, 0x33, 0x33, 0xFF}},
			[3]color.RGBA{{0, 0x77, 0xBB, 0xFF}, {0xDD, 0xAA, 0x33, 0xFF}, {0xCC, 0x33, 0x33, 0xFF}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := cfg100(tc.classic)
			cfg.ColorScale = tc.scale
			v, _ := render(t, cfg, fyne.NewSize(50, 300))
			// 0.5 picks the color for 0; a first sample of 0 repeats the initial value
			for i, val := range []float64{0.5, 50, 100} {
				v.SetValue(val)
				if v.bar.FillColor != tc.fill[i] || v.bar.StrokeColor != tc.stroke[i] {
					t.Errorf("value %v: fill %v stroke %v, want %v %v",
						val, v.bar.FillColor, v.bar.StrokeColor, tc.fill[i], tc.stroke[i])
				}
			}
			// Out of range clamps to the end colors.
			for _, c := range []struct {
				val  float64
				want color.RGBA
			}{{-40, tc.stroke[0]}, {170, tc.stroke[2]}} {
				if _, stroke := v.getColorForValue(c.val); stroke != c.want {
					t.Errorf("getColorForValue(%v) stroke %v, want %v", c.val, stroke, c.want)
				}
			}
		})
	}
}

// Modern lays out a title row on top, a value row at the bottom and the track
// between them; the bar fills the track from the bottom, clamped at Min/Max,
// and the readout shows the integer part of the value.
func TestModernLayout(t *testing.T) {
	const w, h = 50, 300
	cfg := cfg100(false)
	cfg.Title = "TPS"
	v, _ := render(t, cfg, fyne.NewSize(w, h))
	if v.titleText.TextSize != 25 || v.displayText.TextSize != 25 {
		t.Errorf("text sizes %v/%v, want 25", v.titleText.TextSize, v.displayText.TextSize)
	}
	th := v.displayText.MinSize().Height
	barY, barH := th, h-2*th
	if p, s := v.titleText.Position(), v.titleText.Size(); p != (fyne.Position{}) || s != fyne.NewSize(w, th) {
		t.Errorf("title at %v size %v, want origin %vx%v", p, s, w, th)
	}
	if p, s := v.displayText.Position(), v.displayText.Size(); p != fyne.NewPos(0, h-th) || s != fyne.NewSize(w, th) {
		t.Errorf("readout at %v size %v, want (0,%v) %vx%v", p, s, h-th, w, th)
	}
	if p, s := v.face.Position(), v.face.Size(); p != fyne.NewPos(0, barY) || s != fyne.NewSize(w, barH) {
		t.Errorf("track at %v size %v, want (0,%v) %vx%v", p, s, barY, w, barH)
	}
	if r := float32(w * widgets.BarCornerFrac); v.face.CornerRadius != r || v.bar.CornerRadius != r {
		t.Errorf("corner radius %v/%v, want %v", v.face.CornerRadius, v.bar.CornerRadius, r)
	}
	if v.titleText.Text != "TPS" {
		t.Errorf("title %q, want TPS", v.titleText.Text)
	}

	for _, tc := range []struct {
		value   float64
		norm    float32
		readout string
	}{
		{25, 0.25, "25"},
		{100, 1, "100"},
		{42.9, 0.429, "42"},
		{0, 0, "0"},
		{-10, 0, "-10"},
		{-3.7, 0, "-3"},
		{150, 1, "150"},
	} {
		v.SetValue(tc.value)
		checkBar(t, v, w, barY, barH, tc.norm)
		if v.displayText.Text != tc.readout {
			t.Errorf("SetValue(%v): readout %q, want %q", tc.value, v.displayText.Text, tc.readout)
		}
		if got := v.Value(); got != tc.value {
			t.Errorf("Value() = %v, want %v", got, tc.value)
		}
	}
}

// Classic draws the bar over the full height under a thin outline, the title
// at the top and the readout at the bottom, both centered on the middle.
func TestClassicLayout(t *testing.T) {
	const w, h = 60, 300
	v, _ := render(t, cfg100(true), fyne.NewSize(w, h))
	if p, s := v.face.Position(), v.face.Size(); p != fyne.NewPos(0, -2) || s != fyne.NewSize(w, h+3) {
		t.Errorf("outline at %v size %v, want (0,-2) %vx%v", p, s, w, h+3)
	}
	if v.face.StrokeWidth != 3 || v.face.StrokeColor != (color.RGBA{0x80, 0x80, 0x80, 0xFF}) {
		t.Errorf("outline stroke %v width %v", v.face.StrokeColor, v.face.StrokeWidth)
	}
	if p := v.titleText.Position(); p != fyne.NewPos(w/2, 0) {
		t.Errorf("title at %v, want (%v,0)", p, w/2)
	}
	if p := v.displayText.Position(); p != fyne.NewPos(w/2, h-v.displayText.MinSize().Height) {
		t.Errorf("readout at %v, want (%v,%v)", p, w/2, h-v.displayText.MinSize().Height)
	}
	want := color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
	if v.titleText.Color != want || v.displayText.Color != want {
		t.Errorf("text colors %v/%v, want %v", v.titleText.Color, v.displayText.Color, want)
	}
	v.SetValue(60)
	checkBar(t, v, w, 0, h, 0.6)
}

// Modern stacks track, bar, ticks, title, readout; classic puts the bar and
// outline above the ticks. The renderer reports cfg.MinSize.
func TestObjectOrder(t *testing.T) {
	for _, classic := range []bool{false, true} {
		v, r := render(t, cfg100(classic), fyne.NewSize(50, 300))
		var want []fyne.CanvasObject
		if !classic {
			want = append(want, v.face, v.bar)
		}
		for _, l := range v.lines {
			want = append(want, l)
		}
		if classic {
			want = append(want, v.bar, v.face)
		}
		want = append(want, v.titleText, v.displayText)
		objs := r.Objects()
		if len(objs) != len(want) {
			t.Fatalf("classic=%v: %d objects, want %d", classic, len(objs), len(want))
		}
		for i := range want {
			if objs[i] != want[i] {
				t.Errorf("classic=%v: object %d is %T, want %T", classic, i, objs[i], want[i])
			}
		}
		if got := r.MinSize(); got != fyne.NewSize(50, 100) {
			t.Errorf("MinSize = %v, want 50x100", got)
		}
		if !classic && v.face.FillColor != widgets.TrackColor {
			t.Errorf("track color %v, want %v", v.face.FillColor, widgets.TrackColor)
		}
	}
}

// Steps+1 ticks run down the bar band, even ones 2/3 of the width and odd ones
// 2/7, centered. Modern ticks alternate major/minor gray; classic ones carry
// the value gradient with Max at the top.
func TestTicks(t *testing.T) {
	for _, classic := range []bool{false, true} {
		const w, h = 60, 300
		v, _ := render(t, cfg100(classic), fyne.NewSize(w, h))
		if len(v.lines) != 11 {
			t.Fatalf("%d ticks, want 11", len(v.lines))
		}
		barY, barH := v.barY, v.barH
		for i, l := range v.lines {
			y := barY + float32(i)*barH/10
			half := float32(w) / 3
			if i%2 != 0 {
				half = float32(w) / 7
			}
			if !near(l.Position1.Y, y) || !near(l.Position2.Y, y) ||
				!near(l.Position1.X, w/2-half) || !near(l.Position2.X, w/2+half) {
				t.Errorf("classic=%v tick %d from %v to %v, want y=%v x=%v..%v",
					classic, i, l.Position1, l.Position2, y, w/2-half, w/2+half)
			}
			var want color.Color = widgets.TickMajor
			switch {
			case classic:
				want = v.strokeCache[(10-i)*10]
			case i%2 != 0:
				want = widgets.TickMinor
			}
			if l.StrokeColor != want {
				t.Errorf("classic=%v tick %d color %v, want %v", classic, i, l.StrokeColor, want)
			}
		}
	}
}

// A repeat of the last value is dropped before it reaches the canvas; a
// change inside the same integer moves the bar but keeps color and readout.
func TestSetValueSkips(t *testing.T) {
	const w, h = 50, 300
	v, _ := render(t, cfg100(false), fyne.NewSize(w, h))
	v.SetValue(42.2)
	v.value = -1 // sentinel: apply overwrites it
	v.SetValue(42.2)
	if v.value != -1 {
		t.Error("repeated value was redrawn")
	}
	v.SetValue(42.7)
	checkBar(t, v, w, v.barY, v.barH, 0.427)
	if v.displayText.Text != "42" || v.bar.FillColor != v.fillCache[42] {
		t.Errorf("readout %q color %v, want 42 and %v", v.displayText.Text, v.bar.FillColor, v.fillCache[42])
	}
	v.SetValue(43.1)
	if v.displayText.Text != "43" || v.bar.FillColor != v.fillCache[43] {
		t.Errorf("readout %q color %v, want 43 and %v", v.displayText.Text, v.bar.FillColor, v.fillCache[43])
	}
}

// A value set before the first render is what the new renderer draws.
func TestSetValueBeforeRender(t *testing.T) {
	test.NewTempApp(t)
	v := New(cfg100(false))
	v.SetValue(75)
	if v.Value() != 75 {
		t.Errorf("Value() = %v, want 75", v.Value())
	}
	test.TempWidgetRenderer(t, v)
	v.Resize(fyne.NewSize(50, 300))
	if v.displayText.Text != "75" {
		t.Errorf("readout %q, want 75", v.displayText.Text)
	}
	if v.bar.FillColor != v.fillCache[75] {
		t.Errorf("bar color %v, want %v", v.bar.FillColor, v.fillCache[75])
	}
	checkBar(t, v, 50, v.barY, v.barH, 0.75)
}

// A resize re-lays the bar for the value on screen; narrow bars shrink the
// text and a bar too short for both text rows gets an empty track.
func TestResize(t *testing.T) {
	v, _ := render(t, cfg100(false), fyne.NewSize(50, 300))
	v.SetValue(60)

	v.Resize(fyne.NewSize(80, 400))
	th := v.displayText.MinSize().Height
	checkBar(t, v, 80, th, 400-2*th, 0.6)
	if v.face.CornerRadius != 20 {
		t.Errorf("corner radius %v, want 20", v.face.CornerRadius)
	}

	v.Resize(fyne.NewSize(20, 400))
	if v.displayText.TextSize != 10 || v.titleText.TextSize != 10 {
		t.Errorf("text size %v/%v at 20px wide, want 10", v.titleText.TextSize, v.displayText.TextSize)
	}

	v.Resize(fyne.NewSize(50, 20))
	if v.barH != 0 || v.bar.Size().Height != 0 || v.face.Size().Height != 0 {
		t.Errorf("20px tall: band %v, bar %v, track %v, want all 0", v.barH, v.bar.Size(), v.face.Size())
	}
}

// Fyne destroys the renderer of a widget hidden for a while and builds a new
// one when it shows again, laying it out at the unchanged size. The new
// renderer's canvas objects get laid out.
func TestRebuiltRendererIsLaidOut(t *testing.T) {
	v, _ := render(t, cfg100(false), fyne.NewSize(50, 300))
	v.SetValue(75)
	barY, barH := v.barY, v.barH

	r := v.CreateRenderer()
	r.Layout(v.Size())
	checkBar(t, v, 50, barY, barH, 0.75)
	if s := v.face.Size(); s != fyne.NewSize(50, barH) {
		t.Errorf("track size %v after rebuild, want 50x%v", s, barH)
	}
	for i, l := range v.lines {
		if l.Position1 == l.Position2 {
			t.Errorf("tick %d not laid out after rebuild", i)
			break
		}
	}
}

// The gauge creator's Steps field accepts "-"; New defaults a negative Steps
// like 0, so CreateRenderer never sizes the tick table from it.
func TestNegativeStepsDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("VBar with Steps -2 panicked: %v", r)
		}
	}()
	cfg := cfg100(false)
	cfg.Steps = -2
	render(t, cfg, fyne.NewSize(100, 100))
}
