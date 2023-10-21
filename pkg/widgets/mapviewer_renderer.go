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
	vr.mv.content.Resize(size)

	sz := vr.mv.innerView.Size()

	w := sz.Width / float32(vr.mv.numColumns)
	h := sz.Height / float32(vr.mv.numRows)

	for i := len(vr.mv.yData); i > 0; i-- {
		for j := 0; j < len(vr.mv.xData); j++ {
			t := vr.mv.textValues[(i*vr.mv.numColumns)-(vr.mv.numColumns-j)]
			t.TextSize = float32(int(h / 2.0))
			t.Move(fyne.NewPos(
				float32(j)*(sz.Width/float32(vr.mv.numColumns))+4,
				sz.Height-float32(i)*(sz.Height/float32(vr.mv.numRows)),
			))
			t.Resize(fyne.NewSize(w, h))
		}
	}

	for _, xb := range vr.mv.xAxis {
		xb.TextSize = float32(int(h / 2.0))
		xb.Refresh()
	}

	for _, yb := range vr.mv.yAxis {
		yb.TextSize = float32(int(h / 2.0))
		yb.Refresh()
	}

	vr.mv.crosshair.Resize(fyne.NewSize(w, h))
	vr.mv.crosshair.Move(
		fyne.NewPos(
			float32(vr.mv.xIdx)*(sz.Width/float32(vr.mv.numColumns)),
			float32(float64(vr.mv.numRows-1)-vr.mv.yIdx)*(sz.Height/float32(vr.mv.numRows)),
		),
	)

	vr.mv.cursor.Resize(fyne.NewSize(w, h))
	vr.mv.cursor.Move(
		fyne.NewPos(
			float32(vr.mv.curX)*(sz.Width/float32(vr.mv.numColumns)),
			float32(float64(vr.mv.numRows-1)-float64(vr.mv.curY))*(sz.Height/float32(vr.mv.numRows))+1,
		),
	)
}

func (vr *MapViewerRenderer) MinSize() fyne.Size {
	return vr.mv.content.MinSize()
}

func (vr *MapViewerRenderer) Refresh() {
}

func (vr *MapViewerRenderer) Destroy() {
}

func (vr *MapViewerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.mv.content}
}
