package widgets

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
)

// Shared dial styling for dial and dualdial widgets
const (
	DialStartDeg   = -135.0 // matches -common.Pi43 in radians
	DialEndDeg     = 135.0
	DialSweepDeg   = DialEndDeg - DialStartDeg
	DialRingCutout = 0.87 // ring thickness = 13% of radius
)

// ZoneColor maps a 0..1 fraction of the gauge range to green→yellow→red.
func ZoneColor(frac float64) color.RGBA {
	if frac < 0.5 {
		return color.RGBA{R: byte(255 * frac * 2), G: 255, B: 0, A: 255}
	}
	return color.RGBA{R: 255, G: byte(255 * (1 - (frac-0.5)*2)), B: 0, A: 255}
}

type Gauge interface {
	GetConfig() *GaugeConfig
}

type GaugeConfig struct {
	Type             string
	Title            string
	DisplayString    string // default "%.0f"
	GaugeTextString  string
	GaugeFactor      float64
	DisplayTextSize  int
	Min, Max, Center float64
	Steps            int
	MinSize          fyne.Size
	TextPosition     TextPosition
	ColorScale       ColorScheme
	Classic          bool // render dials in the old look: thin outline, colored pips, green readout

	SymbolName          string
	SymbolNameSecondary string
}

func (g GaugeConfig) String() string {
	return fmt.Sprintf("Title: %s, DisplayString: %s, DisplayTextSize: %d, Min: %f, Max: %f, Center: %f, Steps: %d, MinSize: %v, TextPosition: %v, ColorScale: %v",
		g.Title, g.DisplayString, g.DisplayTextSize, g.Min, g.Max, g.Center, g.Steps, g.MinSize, g.TextPosition, g.ColorScale)
}

// Config structs remain the same
type TextPosition int

const (
	TextAtTop TextPosition = iota
	TextAtBottom
	TextAtCenter
)
