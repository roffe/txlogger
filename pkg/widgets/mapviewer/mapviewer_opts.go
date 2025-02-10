package mapviewer

import (
	symbol "github.com/roffe/ecusymbol"
)

type MapViewerOption func(*MapViewer) error

type LoadFunc func()
type SaveFunc func([]float64)
type UpdateFunc func(idx int, value []float64)

func WithButtons(buttons bool) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.opts.buttonsEnabled = buttons
		return nil
	}
}

func WithXFrom(xFrom string) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.xFrom = xFrom
		return nil
	}
}

func WithYFrom(yFrom string) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.yFrom = yFrom
		return nil
	}
}

func WithXData(xData []float64) MapViewerOption {
	//	log.Println("WithXData", xData)
	return func(mv *MapViewer) error {
		mv.numColumns = len(xData)
		mv.xData = xData
		return nil
	}
}

func WithYData(yData []float64) MapViewerOption {
	//	log.Println("WithYData", yData)
	return func(mv *MapViewer) error {
		mv.numRows = len(yData)
		mv.yData = yData
		return nil
	}
}

func WithZData(zData []float64) MapViewerOption {
	//	log.Println("WithZData", zData)
	return func(mv *MapViewer) error {
		mv.numData = len(zData)
		mv.zData = zData
		return nil
	}
}

func WithAxisLabels(x, y, z string) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.xAxisLabel = x
		mv.yAxisLabel = y
		mv.zAxisLabel = z
		return nil
	}
}

func WithXPrecision(precision int) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.xPrecision = precision
		return nil
	}
}

func WithYPrecision(precision int) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.yPrecision = precision
		return nil
	}
}

func WithZPrecision(precision int) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.zPrecision = precision
		return nil
	}
}

func WithSaveFileFunc(saveFileFunc SaveFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.funcs.saveFileFunc = saveFileFunc
		return nil
	}
}

func WithLoadECUFunc(loadFunc LoadFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.funcs.loadECUFunc = loadFunc
		return nil
	}
}

func WithSaveECUFunc(saveECUFunc SaveFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.funcs.saveECUFunc = saveECUFunc
		return nil
	}
}

func WithUpdateECUFunc(updateFunc UpdateFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.funcs.updateECUFunc = updateFunc
		return nil
	}
}

func WithSymbol(symbol *symbol.Symbol) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.symbol = symbol
		return nil
	}
}

func WithMeshView(meshView bool) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.opts.meshView = meshView
		return nil
	}
}

func WithEditable(editable bool) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.opts.editable = editable
		return nil
	}
}

func WithFollowCrosshair(enabled bool) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.opts.cursorFollowCrosshair = enabled
		return nil
	}
}
