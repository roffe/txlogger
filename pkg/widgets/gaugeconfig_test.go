package widgets_test

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/widgets"
)

// ZoneColor runs green at the bottom of the range through yellow at the middle
// to red at the top, always opaque.
func TestZoneColor(t *testing.T) {
	for _, tc := range []struct {
		frac float64
		want color.RGBA
	}{
		{0, color.RGBA{R: 0, G: 255, B: 0, A: 255}},
		{0.25, color.RGBA{R: 127, G: 255, B: 0, A: 255}},
		{0.4999, color.RGBA{R: 254, G: 255, B: 0, A: 255}},
		{0.5, color.RGBA{R: 255, G: 255, B: 0, A: 255}},
		{0.75, color.RGBA{R: 255, G: 127, B: 0, A: 255}},
		{1, color.RGBA{R: 255, G: 0, B: 0, A: 255}},
	} {
		if got := widgets.ZoneColor(tc.frac); got != tc.want {
			t.Errorf("ZoneColor(%v) = %v, want %v", tc.frac, got, tc.want)
		}
	}
}

// String lists the display settings of a gauge config for debug output.
func TestGaugeConfigString(t *testing.T) {
	cfg := widgets.GaugeConfig{
		Type:            "Dial",
		Title:           "Boost",
		DisplayString:   "%.2f",
		DisplayTextSize: 20,
		Min:             -1,
		Max:             2.5,
		Center:          1,
		Steps:           10,
		MinSize:         fyne.NewSize(100, 50),
		TextPosition:    widgets.TextAtBottom,
		ColorScale:      widgets.BlueYellowScale,
	}
	want := "Title: Boost, DisplayString: %.2f, DisplayTextSize: 20, Min: -1.000000, Max: 2.500000, Center: 1.000000, Steps: 10, MinSize: {100 50}, TextPosition: 1, ColorScale: 1"
	if got := cfg.String(); got != want {
		t.Errorf("String() =\n%q\nwant\n%q", got, want)
	}
}
