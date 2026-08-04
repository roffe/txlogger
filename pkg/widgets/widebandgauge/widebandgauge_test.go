package widebandgauge

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
)

// Exactly one LED is at full brightness, and it is the one the value maps to;
// all others sit at their dim zone shade.
func TestOneSegmentLitAtTheRightIndex(t *testing.T) {
	test.NewApp()
	g := New(&widgets.GaugeConfig{Min: 0.5, Max: 1.5, Center: 1.0})
	win := test.NewWindow(g)
	win.SetPadded(false)
	win.Resize(fyne.NewSize(200, 200))

	if got := len(g.segments); got != ledsRich+ledsStoich+ledsLean {
		t.Fatalf("%d LEDs, want %d", got, ledsRich+ledsStoich+ledsLean)
	}

	for _, tc := range []struct {
		value float64
		want  int
	}{{0.5, 0}, {0.75, 7}, {1.0, 15}, {1.5, 29}, {-5, 0}, {99, 29}} {
		g.SetValue(tc.value)

		var litIdx []int
		for i, seg := range g.segments {
			switch seg.StrokeColor {
			case color.Color(zoneColor(i)):
				litIdx = append(litIdx, i)
			case color.Color(dim(zoneColor(i))):
			default:
				t.Fatalf("value %v: segment %d has color %v, neither lit nor dim", tc.value, i, seg.StrokeColor)
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

// Amber over the rich range, green around stoich, red over the lean range.
func TestZoneColorBands(t *testing.T) {
	for _, tc := range []struct {
		idx  int
		want color.RGBA
	}{
		{0, richAmber}, {ledsRich - 1, richAmber},
		{ledsRich, stoichGrn}, {ledsRich + ledsStoich - 1, stoichGrn},
		{ledsRich + ledsStoich, leanRed}, {ledsRich + ledsStoich + ledsLean - 1, leanRed},
	} {
		if got := zoneColor(tc.idx); got != tc.want {
			t.Errorf("zoneColor(%d) = %v, want %v", tc.idx, got, tc.want)
		}
		if dim(tc.want) == tc.want {
			t.Errorf("dim(%v) did not darken", tc.want)
		}
	}
}
