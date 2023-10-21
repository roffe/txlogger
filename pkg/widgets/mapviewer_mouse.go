package widgets

import (
	"log"

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
		newY := float32(mv.numRows-mv.curY-1) * cellHeight
		newX := float32(mv.curX) * cellWidth
		mv.cursor.Move(fyne.NewPos(newX, newY))
	}
}

func (mv *MapViewer) MouseDown(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary && event.Modifier == 0 {
		mv.moving = true
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		mv.curX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.curY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		newY := float32(mv.numRows-mv.curY-1) * cellHeight
		newX := float32(mv.curX) * cellWidth
		mv.cursor.Move(fyne.NewPos(newX, newY))
	}

}

func (mv *MapViewer) MouseUp(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierShift {
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		ncurX := max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		ncurY := max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))
		difX := ncurX - mv.curX
		difY := ncurY - mv.curY
		mv.cursor.Resize(fyne.NewSize(float32(difX)*cellWidth, float32(difY)*cellHeight))
		return
	}

	if event.Button == desktop.MouseButtonPrimary && event.Modifier == 0 {
		mv.moving = false
		cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
		cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
		mv.curX = max(0, min(int(event.Position.X-mv.yAxisButtons.Size().Width)/int(cellWidth), mv.numColumns-1))
		mv.curY = max(0, min(mv.numRows-int(event.Position.Y-mv.xAxisButtons.Size().Height)/int(cellHeight)-1, mv.numRows-1))

		mv.cursor.Move(fyne.NewPos(float32(mv.curX)*cellWidth, float32(mv.numRows-1-mv.curY)*cellHeight))

		index := mv.curY*mv.numColumns + mv.curX
		if index < 0 || index >= len(mv.zData) {
			log.Printf("Index out of range: %d", index)
			return
		}
		value := mv.zData[index]
		log.Printf("Value: %d in cell %dx%d", value, mv.curX, mv.curY)
	}
}
