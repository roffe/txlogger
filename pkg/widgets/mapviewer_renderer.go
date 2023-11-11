package widgets

import (
	"log"

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
	min := fyne.NewSize(float32(vr.mv.numColumns)*40, float32(vr.mv.numRows)*23)
	return min.Max(vr.mv.content.MinSize())
}

func (vr *MapViewerRenderer) Refresh() {
	log.Println("Refresh")
}

func (vr *MapViewerRenderer) Destroy() {
}

func (vr *MapViewerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.mv.content}
}
