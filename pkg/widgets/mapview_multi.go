package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/interpolate"
)

type MapViewerMulti struct {
	widget.BaseWidget
	mvs  []*MapViewer
	view *container.Split
}

func NewMapViewerMulti(typ symbol.ECUType, symbols symbol.SymbolCollection, mapNames ...string) (*MapViewerMulti, error) {
	mvm := &MapViewerMulti{}
	mvm.ExtendBaseWidget(mvm)

	mvm.mvs = make([]*MapViewer, len(mapNames))
	for i, m := range mapNames {
		axis := symbol.GetInfo(typ, m)
		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			return nil, err
		}
		mv, err := NewMapViewer(
			WithXData(xData),
			WithYData(yData),
			WithZData(zData),
			WithXCorrFac(xCorrFac),
			WithYCorrFac(yCorrFac),
			WithZCorrFac(zCorrFac),
			WithXFrom(axis.XFrom),
			WithYFrom(axis.YFrom),
			WithInterPolFunc(interpolate.Interpolate),
			WithButtons(false),
		)
		if err != nil {
			return nil, err
		}
		mvm.mvs[i] = mv
	}

	columns := container.NewGridWithColumns(len(mapNames) - 1)

	for i := 1; i < len(mapNames); i++ {
		columns.Add(
			container.NewBorder(widget.NewLabel(mapNames[i]), nil, nil, nil, mvm.mvs[i]),
		)
	}

	mvm.view = container.NewVSplit(
		columns,
		container.NewBorder(widget.NewLabel(mapNames[0]), nil, nil, nil, mvm.mvs[0]),
	)
	mvm.view.SetOffset(0.2)

	return mvm, nil
}

func (mvm *MapViewerMulti) Children() []*MapViewer {
	return mvm.mvs
}

func (mvm *MapViewerMulti) SetValue(name string, value float64) {
	for _, r := range mvm.mvs {
		r.SetValue(name, value)
	}

}

func (mvm *MapViewerMulti) CreateRenderer() fyne.WidgetRenderer {
	return &MapViewerMultiRenderer{
		mvm: mvm,
	}
}

type MapViewerMultiRenderer struct {
	mvm *MapViewerMulti
}

func (mvmr *MapViewerMultiRenderer) Layout(size fyne.Size) {
	mvmr.mvm.view.Resize(size)
}

func (mvmr *MapViewerMultiRenderer) MinSize() fyne.Size {
	return mvmr.mvm.view.MinSize()
}

func (mvmr *MapViewerMultiRenderer) Refresh() {
}

func (mvmr *MapViewerMultiRenderer) Destroy() {
}

func (mvmr *MapViewerMultiRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{mvmr.mvm.view}
}
