package widgets

import (
	"log"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (mv *MapViewer) MouseIn(_ *desktop.MouseEvent) {
}

func (mv *MapViewer) MouseOut() {
}

func (mv *MapViewer) MouseMoved(event *desktop.MouseEvent) {
	if mv.moving {
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		mv.selectedX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.SelectedY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		newY := (float32(mv.numRows-mv.SelectedY-1) * cellHeight)
		newX := (float32(mv.selectedX) * cellWidth)
		mv.cursor.Move(fyne.NewPos(newX, newY))
	}
	if mv.selecting {
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
	}
}

func (mv *MapViewer) MouseDown(event *desktop.MouseEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(mv); c != nil {
		c.Focus(mv)
	}

	if event.Button == desktop.MouseButtonPrimary && event.Modifier == 0 {
		//mv.moving = true
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		mv.selectedX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.SelectedY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		newY := (float32(mv.numRows-mv.SelectedY-1) * cellHeight)
		newX := (float32(mv.selectedX) * cellWidth)
		mv.cursor.Move(fyne.NewPos(newX, newY))
		mv.cursor.Resize(fyne.NewSize(cellWidth, cellHeight))
		mv.selectedCells = []int{mv.SelectedY*mv.numColumns + mv.selectedX}
	}

	if event.Button == desktop.MouseButtonPrimary /*&& event.Modifier == fyne.KeyModifierShift*/ {
		mv.selecting = true
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

		if mv.moving {
			mv.moving = false
			cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
			cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
			mv.selectedX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
			mv.SelectedY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
			//mv.cursor.Move(fyne.NewPos(float32(mv.selectedX)*cellWidth, float32(mv.numRows-1-mv.SelectedY)*cellHeight))
			index := mv.SelectedY*mv.numColumns + mv.selectedX
			if index < 0 || index >= len(mv.zData) {
				log.Printf("Index out of range: %d", index)
				return
			}
			value := mv.zData[index]
			log.Printf("Value: %d in cell %dx%d", value, mv.selectedX, mv.SelectedY)
		}

	}
}
