package widgets

import (
	"log"
	"strconv"

	"fyne.io/fyne/v2"
)

func (mv *MapViewer) TypedRune(r rune) {
	// print out the rune
	log.Println("TypedRune", string(r))
}

func (mv *MapViewer) TypedKey(key *fyne.KeyEvent) {
	log.Println("TypedKey", key.Name)
	index := mv.curY*mv.numColumns + mv.curX
	var refresh, updateCursor bool
	switch key.Name {
	case fyne.KeyPageUp:
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += 10
		}
		refresh = true
	case fyne.KeyPageDown:
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= 10
		}
		refresh = true
	case "+":
		//mv.zData[index]++
		for _, cell := range mv.selectedCells {
			mv.zData[cell]++
		}
		refresh = true
	case "-":
		//mv.zData[index]--
		for _, cell := range mv.selectedCells {
			mv.zData[cell]--
		}
		refresh = true
	case "Up":
		mv.curY++
		if mv.curY >= mv.numRows {
			mv.curY = mv.numRows - 1
		}
		updateCursor = true
	case "Down":
		mv.curY--
		if mv.curY < 0 {
			mv.curY = 0
		}
		updateCursor = true
	case "Left":
		mv.curX--
		if mv.curX < 0 {
			mv.curX = 0
		}
		updateCursor = true
	case "Right":
		mv.curX++
		if mv.curX >= mv.numColumns {
			mv.curX = mv.numColumns - 1
		}
		updateCursor = true
	}
	if updateCursor {
		sz := mv.innerView.Size()
		xPosFactor := float32(mv.curX)
		yPosFactor := float32(float64(mv.numRows-1) - float64(mv.curY))
		xPos := xPosFactor * (sz.Width / float32(mv.numColumns))
		yPos := yPosFactor * (sz.Height / float32(mv.numRows))
		mv.cursor.Move(fyne.NewPos(xPos, yPos))
	}
	if refresh {
		for _, textIndex := range mv.selectedCells {
			t := mv.textValues[textIndex]
			t.Text = strconv.FormatFloat(float64(mv.zData[index])*mv.corrFac, 'f', 2, 64)
			t.Refresh()
		}
		mv.Refresh()
	}
}
