package widebandgauge

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
)

// Exactly one segment is lit, and it is the one the value maps to.
func TestOneSegmentLitAtTheRightIndex(t *testing.T) {
	test.NewApp()
	g := New(&widgets.GaugeConfig{Min: 0.5, Max: 1.5, Center: 1.0, Steps: 40})
	win := test.NewWindow(g)
	win.SetPadded(false)
	win.Resize(fyne.NewSize(200, 200))

	for _, tc := range []struct {
		value float64
		want  int
	}{{0.5, 0}, {0.75, 10}, {1.0, 20}, {1.5, 40}, {-5, 0}, {99, 40}} {
		g.SetValue(tc.value)

		var litIdx []int
		for i, seg := range g.segments {
			if seg.StrokeColor != color.Color(segDark) {
				litIdx = append(litIdx, i)
			}
		}
		if len(litIdx) != 1 {
			t.Fatalf("value %v: %d segments lit (%v), want exactly 1", tc.value, len(litIdx), litIdx)
		}
		if litIdx[0] != tc.want {
			t.Errorf("value %v: segment %d lit, want %d", tc.value, litIdx[0], tc.want)
		}
	}
}

// Green at stoich, warmer going rich, redder going lean.
func TestLitColorSplitsRichAndLean(t *testing.T) {
	g := New(&widgets.GaugeConfig{Min: 0.5, Max: 1.5, Center: 1.0, Steps: 40})

	if got := g.litColor(1.0); got != stoichGrn {
		t.Errorf("at stoich got %v, want %v", got, stoichGrn)
	}
	if got := g.litColor(0.5); got != richAmber {
		t.Errorf("full rich got %v, want %v", got, richAmber)
	}
	if got := g.litColor(1.5); got != leanRed {
		t.Errorf("full lean got %v, want %v", got, leanRed)
	}
	// Equal deviation either side must not look the same: both warm up away from
	// green, but lean drops toward red far faster than rich drops toward amber.
	rich, lean := g.litColor(0.8), g.litColor(1.2)
	if rich.G >= stoichGrn.G || lean.G >= stoichGrn.G {
		t.Errorf("mid rich %v / mid lean %v did not fade away from green", rich, lean)
	}
	if rich.G <= lean.G {
		t.Errorf("rich %v is not warmer-but-greener than lean %v", rich, lean)
	}
}
