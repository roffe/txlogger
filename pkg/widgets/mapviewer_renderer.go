package widgets

import (
	"math"

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

	// Calculate shared factors
	numColumnsFloat := float32(vr.mv.numColumns)
	numRowsFloat := float32(vr.mv.numRows)
	widthFactor := sz.Width / numColumnsFloat
	heightFactor := sz.Height / numRowsFloat

	// Calculate text size
	textSize := min(heightFactor/2, widthFactor/2)
	textSize = float32(math.Floor(float64(textSize)*100) / 100)

	// Position and resize text values
	for i := len(vr.mv.yData); i > 0; i-- {
		for j := 0; j < len(vr.mv.xData); j++ {
			t := vr.mv.textValues[(i*vr.mv.numColumns)-(vr.mv.numColumns-j)]
			t.TextSize = textSize
			t.Move(fyne.NewPos(
				float32(j)*widthFactor+4,
				sz.Height-float32(i)*heightFactor,
			))
			t.Resize(fyne.NewSize(widthFactor, heightFactor))
		}
	}

	// Update x and y axes
	for _, xb := range vr.mv.xAxis {
		xb.TextSize = textSize
		xb.Refresh()
	}
	for _, yb := range vr.mv.yAxis {
		yb.TextSize = textSize
		yb.Refresh()
	}

	// Position and resize crosshair
	vr.mv.crosshair.Resize(fyne.NewSize(widthFactor, heightFactor))
	vr.mv.crosshair.Move(
		fyne.NewPos(
			float32(vr.mv.xIdx)*widthFactor,
			float32(float64(vr.mv.numRows)-1-vr.mv.yIdx)*heightFactor,
		),
	)

	// Position and resize cursor
	vr.mv.cursor.Resize(fyne.NewSize(widthFactor, heightFactor))
	vr.mv.cursor.Move(
		fyne.NewPos(
			float32(vr.mv.curX)*widthFactor,
			float32(vr.mv.numRows-1-vr.mv.curY)*heightFactor,
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
