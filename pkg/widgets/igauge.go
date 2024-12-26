package widgets

import (
	"fyne.io/fyne/v2"
)

type IGauge interface {
	fyne.Widget
	SetValue(float64)
	SetValue2(float64)
	GetConfig() GaugeConfig
}
