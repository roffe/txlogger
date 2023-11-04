package widgets

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (mv *MapViewer) MouseIn(_ *desktop.MouseEvent) {
}

func (mv *MapViewer) MouseOut() {
}

func (mv *MapViewer) MouseMoved(event *desktop.MouseEvent) {
	if mv.selecting {
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		nselectedX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		nSelectedY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		difX := int(math.Abs(float64(nselectedX - mv.selectedX)))
		difY := int(math.Abs(float64(nSelectedY - mv.SelectedY)))
		topLeftX := float32(min(mv.selectedX, nselectedX)) * cellWidth
		topLeftY := float32(mv.numRows-1-max(mv.SelectedY, nSelectedY)) * cellHeight
		mv.cursor.Resize(fyne.NewSize(float32(difX+1)*cellWidth, float32(difY+1)*cellHeight))
		mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))
	}
}

func (mv *MapViewer) MouseDown(event *desktop.MouseEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(mv); c != nil {
		c.Focus(mv)
	}

	if mv.inputBuffer.Len() > 0 {
		mv.inputBuffer.Reset()
		mv.restoreSelectedValues()
	}

	cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
	cellHeight := mv.innerView.Size().Height / float32(mv.numRows)

	if event.Button == desktop.MouseButtonPrimary && event.Modifier == 0 {
		mv.selectedX = max(0, min(int(event.Position.X-mv.yAxisTexts[0].Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.SelectedY = max(0, min(mv.numRows-int(event.Position.Y-mv.yAxisTexts[0].Size().Height)/int(cellHeight)-1, mv.numRows-1))
		x := (float32(mv.selectedX) * cellWidth)
		y := (float32(mv.numRows-mv.SelectedY-1) * cellHeight)
		mv.cursor.Move(fyne.NewPos(x, y))
		mv.cursor.Resize(fyne.NewSize(cellWidth, cellHeight))
		mv.selectedCells = []int{mv.SelectedY*mv.numColumns + mv.selectedX}
		mv.selecting = true
		return
	}

	if event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierShift {
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		nselectedX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		nSelectedY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		difX := int(math.Abs(float64(nselectedX - mv.selectedX)))
		difY := int(math.Abs(float64(nSelectedY - mv.SelectedY)))
		topLeftX := float32(min(mv.selectedX, nselectedX)) * cellWidth
		topLeftY := float32(mv.numRows-1-max(mv.SelectedY, nSelectedY)) * cellHeight
		mv.cursor.Resize(fyne.NewSize(float32(difX+1)*cellWidth, float32(difY+1)*cellHeight))
		mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))
		mv.selecting = true
		return
	}

}

func (mv *MapViewer) MouseUp(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary {
		if mv.selecting {
			mv.selecting = false
			cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
			cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
			nselectedX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
			nSelectedY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
			difX := int(math.Abs(float64(nselectedX - mv.selectedX)))
			difY := int(math.Abs(float64(nSelectedY - mv.SelectedY)))

			// Calculate top-left corner of the selection
			topLeftX := float32(min(mv.selectedX, nselectedX)) * cellWidth
			topLeftY := float32(mv.numRows-1-max(mv.SelectedY, nSelectedY)) * cellHeight
			mv.cursor.Resize(fyne.NewSize(float32(difX+1)*cellWidth, float32(difY+1)*cellHeight))
			mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))

			// Calculate top-left and bottom-right corners of the selection
			topLeftX1 := int(min(mv.selectedX, nselectedX))
			topLeftY1 := int(min(mv.SelectedY, nSelectedY))
			bottomRightX := max(mv.selectedX, nselectedX)
			bottomRightY := max(mv.SelectedY, nSelectedY)
			// Print out all the zIndex positions the selection would have
			mv.selectedCells = make([]int, 0)
			for y := topLeftY1; y <= bottomRightY; y++ {
				for x := topLeftX1; x <= bottomRightX; x++ {
					zIndex := y*mv.numColumns + x
					mv.selectedCells = append(mv.selectedCells, zIndex)
				}
			}
		}
	}
}
