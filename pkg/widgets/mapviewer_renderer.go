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

func calculateOptimalTextSize(width, height float32, columns, rows int) float32 {
	cellWidth := float64(width / float32(columns))
	return max(min(float32(cellWidth/5), 21), 12)
}

func (vr *MapViewerRenderer) Layout(size fyne.Size) {
	vr.mv.content.Resize(size)
	sz := vr.mv.innerView.Size()

	// Calculate shared factors
	numColumnsFloat := float32(vr.mv.numColumns)
	numRowsFloat := float32(vr.mv.numRows)
	widthFactor := sz.Width / numColumnsFloat
	heightFactor := sz.Height / numRowsFloat

	// Position and resize crosshair
	vr.mv.crosshair.Resize(fyne.NewSize(widthFactor, heightFactor))
	vr.mv.crosshair.Move(
		fyne.NewPos(
			float32(vr.mv.xIdx)*widthFactor,
			float32(float64(vr.mv.numRows)-1-vr.mv.yIdx)*heightFactor,
		),
	)

	// Calculate text size
	textSize := calculateOptimalTextSize(sz.Width, sz.Height, vr.mv.numColumns, vr.mv.numRows)

	// Position and resize text values
	for i := vr.mv.numRows; i > 0; i-- {
		for j := 0; j < vr.mv.numColumns; j++ {
			iDx := (i * vr.mv.numColumns) - (vr.mv.numColumns - j)
			t := vr.mv.textValues[iDx]
			t.TextSize = textSize
			t.Resize(t.MinSize())
			t.Move(fyne.NewPos(
				(float32(j)*widthFactor)+(widthFactor/2)-(t.MinSize().Width/2),
				(sz.Height-float32(i)*heightFactor)+(heightFactor/2)-(t.MinSize().Height/2),
			))
			vr.mv.zDataRects[iDx].Resize(fyne.NewSize(widthFactor, heightFactor))
			vr.mv.zDataRects[iDx].Move(fyne.NewPos(
				float32(j)*widthFactor,
				(sz.Height - float32(i)*heightFactor),
			))
		}
	}

	// Update x and y axes
	for _, xb := range vr.mv.xAxisTexts {
		xb.TextSize = textSize
		xb.Resize(xb.MinSize())
		xb.Refresh()
	}
	for _, yb := range vr.mv.yAxisTexts {
		yb.TextSize = textSize
		yb.Resize(yb.MinSize())
		yb.Refresh()
	}

	vr.resizeCursor(sz, numColumnsFloat, numRowsFloat, widthFactor, heightFactor)
}

func (vr *MapViewerRenderer) resizeCursor(viewportSize fyne.Size, numColumnsFloat, numRowsFloat, widthFactor, heightFactor float32) {
	// Position and resize cursor
	if vr.mv.selectedX >= 0 {
		if len(vr.mv.selectedCells) > 1 {
			mv := vr.mv
			// if multiple cells are selected, we need to calculate the bounding box
			// of the selection area
			minX := mv.numColumns
			maxX := 0
			minY := mv.numRows
			maxY := 0
			for _, cell := range mv.selectedCells {
				x := cell % mv.numColumns
				y := cell / mv.numColumns
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
			widthFactor := viewportSize.Width / numColumnsFloat
			heightFactor := viewportSize.Height / numRowsFloat
			topLeftX := float32(minX) * widthFactor
			topLeftY := float32(mv.numRows-1-maxY) * heightFactor
			width := float32(maxX-minX+1) * widthFactor
			height := float32(maxY-minY+1) * heightFactor
			vr.mv.cursor.Resize(fyne.NewSize(width, height))
			vr.mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))

		} else {
			vr.mv.cursor.Resize(fyne.NewSize(widthFactor, heightFactor))
			vr.mv.cursor.Move(
				fyne.NewPos(
					float32(vr.mv.selectedX)*widthFactor,
					float32(vr.mv.numRows-1-vr.mv.SelectedY)*heightFactor,
				),
			)
		}
	}
}

func (vr *MapViewerRenderer) MinSize() fyne.Size {
	min := fyne.NewSize(float32(vr.mv.numColumns)*40, float32(vr.mv.numRows)*23)
	return min.Max(vr.mv.content.MinSize())
}

func (vr *MapViewerRenderer) Refresh() {
}

func (vr *MapViewerRenderer) Destroy() {
}

func (vr *MapViewerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{vr.mv.content}
}
