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

func calculateOptimalTextSize(width, height float32, columns, rows int) float32 {

	// Calculate the width and height of a single cell
	cellWidth := float64(width / float32(columns))
	return max(min(float32(cellWidth/5), 21), 12)

	cellHeight := float64(height / float32(rows))
	// The optimal text size is the smallest of the two dimensions of the cell
	return float32(int(math.Min(cellWidth, cellHeight)))

}

func (vr *MapViewerRenderer) Layout(size fyne.Size) {
	vr.mv.content.Resize(size)
	sz := vr.mv.innerView.Size()
	// Calculate shared factors
	numColumnsFloat := float32(vr.mv.numColumns)
	numRowsFloat := float32(vr.mv.numRows)
	widthFactor := sz.Width / numColumnsFloat
	heightFactor := sz.Height / numRowsFloat

	// Use the more restrictive factor to determine text size

	// Calculate text size
	textSize := calculateOptimalTextSize(sz.Width, sz.Height, vr.mv.numColumns, vr.mv.numRows)

	// Position and resize text values
	for i := len(vr.mv.yData); i > 0; i-- {
		for j := 0; j < len(vr.mv.xData); j++ {
			t := vr.mv.textValues[(i*vr.mv.numColumns)-(vr.mv.numColumns-j)]
			t.TextSize = textSize
			t.Resize(t.MinSize())
			t.Move(fyne.NewPos(
				(float32(j)*widthFactor)+(widthFactor/2)-(t.MinSize().Width/2),
				(sz.Height-float32(i)*heightFactor)+(heightFactor/2)-(t.MinSize().Height/2),
			))
		}
	}

	// Update x and y axes
	for _, xb := range vr.mv.xAxis {
		xb.TextSize = textSize
		xb.Resize(xb.MinSize())
		xb.Refresh()
	}
	for _, yb := range vr.mv.yAxis {
		yb.TextSize = textSize
		yb.Resize(yb.MinSize())
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
			float32(vr.mv.selectedX)*widthFactor,
			float32(vr.mv.numRows-1-vr.mv.SelectedY)*heightFactor,
		),
	)
}

func (vr *MapViewerRenderer) MinSize() fyne.Size {
	s := vr.mv.content.MinSize()
	return fyne.NewSize(
		float32(int(s.Width)),
		float32(int(s.Height)),
	)
}

func (vr *MapViewerRenderer) Refresh() {
}

func (vr *MapViewerRenderer) Destroy() {
}

func (vr *MapViewerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.mv.content}
}
