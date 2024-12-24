package widgets

import "fyne.io/fyne/v2"

type GaugeConfig struct {
	Title            string
	DisplayString    string // default "%.0f"
	DisplayTextSize  int
	Min, Max, Center float64
	Steps            int
	MinSize          fyne.Size
	TextPosition     TextPosition
	ColorScale       ColorScheme
}

// Config structs remain the same
type TextPosition int

const (
	TextAtTop TextPosition = iota
	TextAtBottom
	TextAtCenter
)
