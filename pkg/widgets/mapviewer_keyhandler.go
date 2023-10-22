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
	shifted := false
	if key.Name == "LeftShift" {
		shifted = true
	}
	log.Println("TypedKey", key.Name, shifted, key.Physical.ScanCode)
	var refresh, updateCursor bool
	switch key.Name {
	case fyne.KeyPageUp:
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += int((mv.zCorrFac * 10) * (1.0 / mv.zCorrFac))
		}
		refresh = true
	case fyne.KeyPageDown:
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= int((mv.zCorrFac * 10) * (1.0 / mv.zCorrFac))
		}
		refresh = true
	case "+":
		//mv.zData[index]++
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += int(mv.zCorrFac * (1.0 / mv.zCorrFac))
		}
		refresh = true
	case "-":
		//mv.zData[index]--
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= int(mv.zCorrFac * (1.0 / mv.zCorrFac))
		}
		refresh = true
	case "Up":
		mv.SelectedY++
		if mv.SelectedY >= mv.numRows {
			mv.SelectedY = mv.numRows - 1
		}
		updateCursor = true
	case "Down":
		mv.SelectedY--
		if mv.SelectedY < 0 {
			mv.SelectedY = 0
		}
		updateCursor = true
	case "Left":
		mv.selectedX--
		if mv.selectedX < 0 {
			mv.selectedX = 0
		}
		updateCursor = true
	case "Right":
		mv.selectedX++
		if mv.selectedX >= mv.numColumns {
			mv.selectedX = mv.numColumns - 1
		}
		updateCursor = true
	}
	index := mv.SelectedY*mv.numColumns + mv.selectedX

	if updateCursor {
		sz := mv.innerView.Size()
		xPosFactor := float32(mv.selectedX)
		yPosFactor := float32(float64(mv.numRows-1) - float64(mv.SelectedY))
		xPos := xPosFactor * (sz.Width / float32(mv.numColumns))
		yPos := yPosFactor * (sz.Height / float32(mv.numRows))
		mv.selectedCells = []int{index}
		mv.cursor.Resize(fyne.NewSize(sz.Width/float32(mv.numColumns), sz.Height/float32(mv.numRows)))
		mv.cursor.Move(fyne.NewPos(xPos, yPos))
	}

	if refresh {
		for _, textIndex := range mv.selectedCells {
			prec := 2
			if mv.zCorrFac == 1 {
				prec = 0
			}
			t := mv.textValues[textIndex]
			t.Text = strconv.FormatFloat(float64(mv.zData[index])*mv.zCorrFac, 'f', prec, 64)
			t.Refresh()
		}
		mv.Refresh()
	}
}
