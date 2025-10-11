package mapviewer

import (
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets"
)

type LoadFunc func()
type SaveFunc func([]float64)
type UpdateFunc func(idx int, value []float64)

type Config struct {
	Symbol *symbol.Symbol

	XFrom string
	YFrom string

	XData []float64
	YData []float64
	ZData []float64

	XAxisLabel string
	YAxisLabel string
	ZAxisLabel string

	XPrecision int
	YPrecision int
	ZPrecision int

	SaveFileFunc  SaveFunc
	LoadECUFunc   LoadFunc
	SaveECUFunc   SaveFunc
	UpdateECUFunc UpdateFunc

	MeshView              bool
	Buttons               bool
	Editable              bool
	CursorFollowCrosshair bool
	ColorblindMode        widgets.ColorBlindMode
}
