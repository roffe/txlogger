package hbar

import (
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
)

// render builds an HBar from cfg, creates its renderer and lays it out at size.
func render(t *testing.T, cfg *widgets.GaugeConfig, size fyne.Size) (*HBar, fyne.WidgetRenderer) {
	t.Helper()
	test.NewTempApp(t)
	b := New(cfg)
	r := test.TempWidgetRenderer(t, b)
	b.Resize(size)
	return b, r
}

func cfg100(classic bool) *widgets.GaugeConfig {
	return &widgets.GaugeConfig{Min: 0, Max: 100, Steps: 10, Classic: classic, MinSize: fyne.NewSize(100, 50)}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// checkBar verifies the bar fills norm of the width from the left inside the
// band [barY, barY+barH].
func checkBar(t *testing.T, b *HBar, w, barY, barH, norm float32) {
	t.Helper()
	p, s := b.bar.Position(), b.bar.Size()
	if p != fyne.NewPos(0, barY) || !near(s.Width, norm*w) || !near(s.Height, barH) {
		t.Errorf("bar at %v size %v, want (0,%v) size %vx%v", p, s, barY, norm*w, barH)
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
			b := New(cfg)
			if b.GetConfig() != cfg {
				t.Fatal("GetConfig does not return the config New was given")
			}
			if cfg.Steps != tc.wantSteps {
				t.Errorf("Steps = %d, want %d", cfg.Steps, tc.wantSteps)
			}
			if len(b.fillCache) != tc.wantLen || len(b.strokeCache) != tc.wantLen || b.cacheMinInt != tc.wantFirst {
				t.Errorf("caches %d/%d from %d, want %d from %d",
					len(b.fillCache), len(b.strokeCache), b.cacheMinInt, tc.wantLen, tc.wantFirst)
			}
			if n := b.clampNorm(tc.min - 100); n != 0 {
				t.Errorf("clampNorm below Min = %v, want 0", n)
			}
			// Every value maps to a valid cache slot.
			for _, v := range []float64{tc.min - 1000, tc.min, tc.max, tc.max + 1000} {
				if i := b.cacheIndexForValue(v); i < 0 || i >= len(b.fillCache) {
					t.Errorf("cacheIndexForValue(%v) = %d, outside 0..%d", v, i, len(b.fillCache)-1)
				}
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
			b, _ := render(t, cfg, fyne.NewSize(300, 60))
			// 0.5 picks the color for 0; a first sample of 0 repeats the initial value
			for i, val := range []float64{0.5, 50, 100} {
				b.SetValue(val)
				if b.bar.FillColor != tc.fill[i] || b.bar.StrokeColor != tc.stroke[i] {
					t.Errorf("value %v: fill %v stroke %v, want %v %v",
						val, b.bar.FillColor, b.bar.StrokeColor, tc.fill[i], tc.stroke[i])
				}
			}
			// Out of range clamps to the end colors.
			for _, c := range []struct {
				val  float64
				want color.RGBA
			}{{-40, tc.stroke[0]}, {170, tc.stroke[2]}} {
				if _, stroke := b.getColorForValue(c.val); stroke != c.want {
					t.Errorf("getColorForValue(%v) stroke %v, want %v", c.val, stroke, c.want)
				}
			}
		})
	}
}

// Modern puts a text row on top, title on the left and readout on the right,
// with the track below it; the bar fills the track from the left, clamped at
// Min/Max, and the readout shows the integer part of the value.
func TestModernLayout(t *testing.T) {
	const w, h = 300, 60
	cfg := cfg100(false)
	cfg.Title = "PWM"
	b, _ := render(t, cfg, fyne.NewSize(w, h))
	if b.titleText.TextSize != 25 || b.displayText.TextSize != 25 {
		t.Errorf("text sizes %v/%v, want 25", b.titleText.TextSize, b.displayText.TextSize)
	}
	if b.titleText.Alignment != fyne.TextAlignLeading || b.displayText.Alignment != fyne.TextAlignTrailing {
		t.Errorf("alignments title %v readout %v, want leading/trailing", b.titleText.Alignment, b.displayText.Alignment)
	}
	th := b.displayText.MinSize().Height
	barY, barH := th, h-th
	for _, txt := range []fyne.CanvasObject{b.titleText, b.displayText} {
		if p, s := txt.Position(), txt.Size(); p != (fyne.Position{}) || s != fyne.NewSize(w, th) {
			t.Errorf("text row at %v size %v, want origin %vx%v", p, s, w, th)
		}
	}
	if p, s := b.face.Position(), b.face.Size(); p != fyne.NewPos(0, barY) || s != fyne.NewSize(w, barH) {
		t.Errorf("track at %v size %v, want (0,%v) %vx%v", p, s, barY, w, barH)
	}
	if r := barH * widgets.BarCornerFrac; b.face.CornerRadius != r || b.bar.CornerRadius != r {
		t.Errorf("corner radius %v/%v, want %v", b.face.CornerRadius, b.bar.CornerRadius, r)
	}
	if b.titleText.Text != "PWM" || b.titleText.Color != widgets.TextPrimary {
		t.Errorf("title %q color %v, want PWM %v", b.titleText.Text, b.titleText.Color, widgets.TextPrimary)
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
		{150, 1, "150"},
	} {
		b.SetValue(tc.value)
		checkBar(t, b, w, barY, barH, tc.norm)
		if b.displayText.Text != tc.readout {
			t.Errorf("SetValue(%v): readout %q, want %q", tc.value, b.displayText.Text, tc.readout)
		}
		if got := b.Value(); got != tc.value {
			t.Errorf("Value() = %v, want %v", got, tc.value)
		}
	}

	// Short bars shrink the text row with the height.
	b.Resize(fyne.NewSize(w, 40))
	if want := float32(40 * 0.42); !near(b.displayText.TextSize, want) {
		t.Errorf("text size %v at 40px tall, want %v", b.displayText.TextSize, want)
	}
}

// Classic draws the bar over the full height under a thin outline, the
// readout centered in the bar and the title along its bottom edge.
func TestClassicLayout(t *testing.T) {
	const w, h = 300, 60
	b, _ := render(t, cfg100(true), fyne.NewSize(w, h))
	if p, s := b.face.Position(), b.face.Size(); p != fyne.NewPos(-2, 0) || s != fyne.NewSize(w+3, h+1) {
		t.Errorf("outline at %v size %v, want (-2,0) %vx%v", p, s, w+3, h+1)
	}
	if b.face.StrokeWidth != 3 || b.face.StrokeColor != (color.RGBA{0x80, 0x80, 0x80, 0xFF}) {
		t.Errorf("outline stroke %v width %v", b.face.StrokeColor, b.face.StrokeWidth)
	}
	tm, dm := b.titleText.MinSize(), b.displayText.MinSize()
	if p, s := b.titleText.Position(), b.titleText.Size(); p != fyne.NewPos(0, h-tm.Height) || s != fyne.NewSize(w, tm.Height) {
		t.Errorf("title at %v size %v, want (0,%v) %vx%v", p, s, h-tm.Height, w, tm.Height)
	}
	if p, s := b.displayText.Position(), b.displayText.Size(); p != fyne.NewPos(0, h/2-dm.Height/2) || s != fyne.NewSize(w, dm.Height) {
		t.Errorf("readout at %v size %v, want (0,%v) %vx%v", p, s, h/2-dm.Height/2, w, dm.Height)
	}
	if b.titleText.Alignment != fyne.TextAlignCenter || b.displayText.Alignment != fyne.TextAlignCenter {
		t.Errorf("alignments %v/%v, want centered", b.titleText.Alignment, b.displayText.Alignment)
	}
	want := color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
	if b.titleText.Color != want || b.displayText.Color != want {
		t.Errorf("text colors %v/%v, want %v", b.titleText.Color, b.displayText.Color, want)
	}
	b.SetValue(60)
	checkBar(t, b, w, 0, h, 0.6)
}

// Modern stacks track, bar, ticks, title, readout; classic puts the bar and
// outline above the ticks. The renderer reports cfg.MinSize.
func TestObjectOrder(t *testing.T) {
	for _, classic := range []bool{false, true} {
		b, r := render(t, cfg100(classic), fyne.NewSize(300, 60))
		var want []fyne.CanvasObject
		if !classic {
			want = append(want, b.face, b.bar)
		}
		for _, l := range b.lines {
			want = append(want, l)
		}
		if classic {
			want = append(want, b.bar, b.face)
		}
		want = append(want, b.titleText, b.displayText)
		objs := r.Objects()
		if len(objs) != len(want) {
			t.Fatalf("classic=%v: %d objects, want %d", classic, len(objs), len(want))
		}
		for i := range want {
			if objs[i] != want[i] {
				t.Errorf("classic=%v: object %d is %T, want %T", classic, i, objs[i], want[i])
			}
		}
		if got := r.MinSize(); got != fyne.NewSize(100, 50) {
			t.Errorf("MinSize = %v, want 100x50", got)
		}
		if !classic && b.face.FillColor != widgets.TrackColor {
			t.Errorf("track color %v, want %v", b.face.FillColor, widgets.TrackColor)
		}
	}
}

// Steps+1 ticks run across the bar band, even ones 2/3 of its height and odd
// ones 2/7, centered on it. Modern ticks alternate major/minor gray; classic
// ones carry the value gradient with Min on the left.
func TestTicks(t *testing.T) {
	for _, classic := range []bool{false, true} {
		const w, h = 300, 60
		b, _ := render(t, cfg100(classic), fyne.NewSize(w, h))
		if len(b.lines) != 11 {
			t.Fatalf("%d ticks, want 11", len(b.lines))
		}
		mid := b.barY + b.barH/2
		for i, l := range b.lines {
			x := float32(i) * w / 10
			half := b.barH / 3
			if i%2 != 0 {
				half = b.barH / 7
			}
			if !near(l.Position1.X, x) || !near(l.Position2.X, x) ||
				!near(l.Position1.Y, mid-half) || !near(l.Position2.Y, mid+half) {
				t.Errorf("classic=%v tick %d from %v to %v, want x=%v y=%v..%v",
					classic, i, l.Position1, l.Position2, x, mid-half, mid+half)
			}
			var want color.Color = widgets.TickMajor
			switch {
			case classic:
				want = b.strokeCache[i*10]
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
	const w, h = 300, 60
	b, _ := render(t, cfg100(false), fyne.NewSize(w, h))
	b.SetValue(42.2)
	b.value = -1 // sentinel: apply overwrites it
	b.SetValue(42.2)
	if b.value != -1 {
		t.Error("repeated value was redrawn")
	}
	b.SetValue(42.7)
	checkBar(t, b, w, b.barY, b.barH, 0.427)
	if b.displayText.Text != "42" || b.bar.FillColor != b.fillCache[42] {
		t.Errorf("readout %q color %v, want 42 and %v", b.displayText.Text, b.bar.FillColor, b.fillCache[42])
	}
	b.SetValue(43.1)
	if b.displayText.Text != "43" || b.bar.FillColor != b.fillCache[43] {
		t.Errorf("readout %q color %v, want 43 and %v", b.displayText.Text, b.bar.FillColor, b.fillCache[43])
	}
}

// A value set before the first render is what the new renderer draws, and a
// resize re-lays the bar for the value on screen.
func TestSetValueBeforeRenderAndResize(t *testing.T) {
	test.NewTempApp(t)
	b := New(cfg100(false))
	b.SetValue(75)
	if b.Value() != 75 {
		t.Errorf("Value() = %v, want 75", b.Value())
	}
	test.TempWidgetRenderer(t, b)
	b.Resize(fyne.NewSize(300, 60))
	if b.displayText.Text != "75" {
		t.Errorf("readout %q, want 75", b.displayText.Text)
	}
	if b.bar.FillColor != b.fillCache[75] {
		t.Errorf("bar color %v, want %v", b.bar.FillColor, b.fillCache[75])
	}
	checkBar(t, b, 300, b.barY, b.barH, 0.75)

	b.Resize(fyne.NewSize(600, 80))
	checkBar(t, b, 600, b.barY, 80-b.barY, 0.75)
}

// Fyne destroys the renderer of a widget hidden for a while and builds a new
// one when it shows again, laying it out at the unchanged size. The new
// renderer's canvas objects get laid out.
func TestRebuiltRendererIsLaidOut(t *testing.T) {
	b, _ := render(t, cfg100(false), fyne.NewSize(300, 60))
	b.SetValue(75)
	barY, barH := b.barY, b.barH

	r := b.CreateRenderer()
	r.Layout(b.Size())
	checkBar(t, b, 300, barY, barH, 0.75)
	if s := b.face.Size(); s != fyne.NewSize(300, barH) {
		t.Errorf("track size %v after rebuild, want 300x%v", s, barH)
	}
}

// The gauge creator's Steps field accepts "-"; New defaults a negative Steps
// like 0, so CreateRenderer never sizes the tick table from it.
func TestNegativeStepsDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HBar with Steps -2 panicked: %v", r)
		}
	}()
	cfg := cfg100(false)
	cfg.Steps = -2
	render(t, cfg, fyne.NewSize(100, 100))
}
