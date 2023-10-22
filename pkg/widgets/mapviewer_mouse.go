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
		mv.curX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.curY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		newY := (float32(mv.numRows-mv.curY-1) * cellHeight)
		newX := (float32(mv.curX) * cellWidth)
		mv.cursor.Move(fyne.NewPos(newX, newY))
	}
	if mv.selecting {
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		ncurX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		ncurY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		difX := int(math.Abs(float64(ncurX - mv.curX)))
		difY := int(math.Abs(float64(ncurY - mv.curY)))
		// Calculate top-left corner of the selection
		topLeftX := float32(min(mv.curX, ncurX)) * cellWidth
		topLeftY := float32(mv.numRows-1-max(mv.curY, ncurY)) * cellHeight
		mv.cursor.Resize(fyne.NewSize(float32(difX+1)*cellWidth, float32(difY+1)*cellHeight))
		mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))
	}
}

func (mv *MapViewer) MouseDown(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary && event.Modifier == 0 {
		//mv.moving = true
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		mv.curX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.curY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		newY := (float32(mv.numRows-mv.curY-1) * cellHeight)
		newX := (float32(mv.curX) * cellWidth)
		mv.cursor.Move(fyne.NewPos(newX, newY))
		mv.cursor.Resize(fyne.NewSize(cellWidth, cellHeight))
		mv.selectedCells = []int{mv.curY*mv.numColumns + mv.curX}
	}

	if event.Button == desktop.MouseButtonPrimary /*&& event.Modifier == fyne.KeyModifierShift*/ {
		mv.selecting = true
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		ncurX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		ncurY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		difX := int(math.Abs(float64(ncurX - mv.curX)))
		difY := int(math.Abs(float64(ncurY - mv.curY)))
		// Calculate top-left corner of the selection
		topLeftX := float32(min(mv.curX, ncurX)) * cellWidth
		topLeftY := float32(mv.numRows-1-max(mv.curY, ncurY)) * cellHeight
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
			ncurX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
			ncurY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
			difX := int(math.Abs(float64(ncurX - mv.curX)))
			difY := int(math.Abs(float64(ncurY - mv.curY)))
			// Calculate top-left corner of the selection
			topLeftX := float32(min(mv.curX, ncurX)) * cellWidth
			topLeftY := float32(mv.numRows-1-max(mv.curY, ncurY)) * cellHeight
			mv.cursor.Resize(fyne.NewSize(float32(difX+1)*cellWidth, float32(difY+1)*cellHeight))
			mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))

			// Calculate top-left and bottom-right corners of the selection
			topLeftX1 := int(min(mv.curX, ncurX))
			topLeftY1 := int(min(mv.curY, ncurY))
			bottomRightX := max(mv.curX, ncurX)
			bottomRightY := max(mv.curY, ncurY)
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
			mv.curX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
			mv.curY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
			//mv.cursor.Move(fyne.NewPos(float32(mv.curX)*cellWidth, float32(mv.numRows-1-mv.curY)*cellHeight))
			index := mv.curY*mv.numColumns + mv.curX
			if index < 0 || index >= len(mv.zData) {
				log.Printf("Index out of range: %d", index)
				return
			}
			value := mv.zData[index]
			log.Printf("Value: %d in cell %dx%d", value, mv.curX, mv.curY)
		}

	}
}
