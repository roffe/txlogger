package widgets

import (
	"fyne.io/fyne/v2"
)

func (mv *MapViewer) CreateRenderer() fyne.WidgetRenderer {
	return &MapViewerRenderer{
		mv: mv,
	}
}

type MapViewerRenderer struct {
	mv *MapViewer
}

func (vr *MapViewerRenderer) Layout(size fyne.Size) {
	vr.mv.resize(size)
}

func (vr *MapViewerRenderer) MinSize() fyne.Size {
	wm := maxf32(80, float32(vr.mv.numColumns)*40)
	hm := maxf32(50, float32(vr.mv.numRows)*25)
	min := fyne.NewSize(wm, hm)
	return min.Max(vr.mv.content.MinSize())
}

func (vr *MapViewerRenderer) Refresh() {
}

func (vr *MapViewerRenderer) Destroy() {
}

func (vr *MapViewerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.mv.content}
}

func maxf32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
