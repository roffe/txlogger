package widgets

import (
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/symbol"
)

type MapViewerOption func(*MapViewer) error

type LoadFunc func()
type SaveFunc func([]int)
type UpdateFunc func(idx int, value []int)

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

func WithXData(xData []int) MapViewerOption {
	//	log.Println("WithXData", xData)
	return func(mv *MapViewer) error {
		mv.numColumns = len(xData)
		mv.xData = xData
		return nil
	}
}

func WithYData(yData []int) MapViewerOption {
	//	log.Println("WithYData", yData)
	return func(mv *MapViewer) error {
		mv.numRows = len(yData)
		mv.yData = yData
		return nil
	}
}

func WithZData(zData []int) MapViewerOption {
	//	log.Println("WithZData", zData)
	return func(mv *MapViewer) error {
		mv.numData = len(zData)
		mv.zData = zData
		return nil
	}
}

func WithXCorrFac(xCorrFac float64) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.xCorrFac = xCorrFac
		return nil
	}
}

func WithYCorrFac(yCorrFac float64) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.yCorrFac = yCorrFac
		return nil
	}
}

func WithZCorrFac(zCorrFac float64) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.zCorrFac = zCorrFac
		return nil
	}
}

func WithInterPolFunc(ipf interpolate.InterPolFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.ipf = ipf
		return nil
	}
}

func WithSaveFileFunc(saveFileFunc SaveFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.saveFileFunc = saveFileFunc
		return nil
	}
}

func WithLoadECUFunc(loadFunc LoadFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.loadECUFunc = loadFunc
		return nil
	}
}

func WithSaveECUFunc(saveECUFunc SaveFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.saveECUFunc = saveECUFunc
		return nil
	}
}

func WithUpdateECUFunc(updateFunc UpdateFunc) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.updateECUFunc = updateFunc
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
		mv.meshView = meshView
		return nil
	}
}

func WithEditable(editable bool) MapViewerOption {
	return func(mv *MapViewer) error {
		mv.editable = editable
		return nil
	}
}
