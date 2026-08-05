package mapviewer

import (
	"image/color"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
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

	MeshView     bool
	MeshRenderer meshgrid.RenderBackend

	Editable              bool
	CursorFollowCrosshair bool

	ColorblindMode colors.ColorBlindMode

	// RegionBorder marks cells (same flat row-major order as ZData) that should
	// be drawn with a contrasting border, e.g. to outline the closed-loop fuel
	// area. nil or wrong length = no border drawn.
	RegionBorder []bool
	// RegionBorderColor is the border colour; zero value falls back to a default.
	RegionBorderColor color.RGBA

	Buttons []*MapViewerButton
}

type MapViewerButton struct {
	Label    string
	Icon     fyne.Resource
	OnTapped func()
}
