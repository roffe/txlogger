package mapviewer

import (
	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/colors"
)

type Config struct {
	Name string

	XData []float64
	YData []float64
	ZData []float64

	XPrecision int
	YPrecision int
	ZPrecision int

	XLabel string
	YLabel string
	ZLabel string

	LoadFileFunc func()
	SaveFileFunc func([]float64)
	LoadECUFunc  func()
	SaveECUFunc  func([]float64)
	OnUpdateCell func(idx int, value []float64)
	OnMouseDown  func()

	MeshView              bool
	Editable              bool
	CursorFollowCrosshair bool

	ColorblindMode colors.ColorBlindMode

	Buttons []*MapViewerButton
}

type MapViewerButton struct {
	Label    string
	Icon     fyne.Resource
	OnTapped func()
}
