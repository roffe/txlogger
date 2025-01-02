package widgets

import (
	"fmt"

	"fyne.io/fyne/v2"
)

type Gauge interface {
	GetConfig() *GaugeConfig
}

type GaugeConfig struct {
	Type             string
	Title            string
	DisplayString    string // default "%.0f"
	DisplayTextSize  int
	Min, Max, Center float64
	Steps            int
	MinSize          fyne.Size
	TextPosition     TextPosition
	ColorScale       ColorScheme

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
