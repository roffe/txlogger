package mapviewer

import (
	"log"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

//func (mw *MapViewer) Tapped(_ *fyne.PointEvent) {
//	log.Println("Tapped")
//	fyne.CurrentApp().Driver().CanvasForObject(mw).Focus(mw)
//}
//
//func (mw *MapViewer) TappedSecondary(_ *fyne.PointEvent) {
//	log.Println("TappedSecondary")
//}

func (mw *MapViewer) FocusGained() {
	//log.Println("FocusGained")
	mw.focused = true
}
func (mw *MapViewer) FocusLost() {
	//log.Println("FocusLost")
	mw.focused = false
}
func (mw *MapViewer) Focused() bool {
	return mw.focused
}

func (mv *MapViewer) TypedRune(r rune) {
	// print out the rune
	// log.Println("TypedRune", string(r))
}

func (mv *MapViewer) TypedKey(key *fyne.KeyEvent) {
	if !mv.opts.editable {
		return
	}
	//	log.Println("TypedKey", key.Name, shifted, key.Physical.ScanCode)
	var refresh, updateCursor bool
	switch key.Name {
	case fyne.KeyEscape:
		mv.inputBuffer.Reset()
		mv.restoreSelectedValues()
	case fyne.KeyBackspace:
		if mv.inputBuffer.Len() > 0 {
			newStr := mv.inputBuffer.String()[:mv.inputBuffer.Len()-1]
			mv.inputBuffer.Reset()
			mv.inputBuffer.WriteString(newStr)
			for _, cell := range mv.selectedCells {
				mv.textValues[cell].Text = mv.inputBuffer.String()
				mv.textValues[cell].Refresh()
			}
		}

	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", ".", ",":
		if key.Name == "," {
			key.Name = "."
		}
		mv.restoreValues = true
		mv.inputBuffer.WriteString(string(key.Name))
		for _, cell := range mv.selectedCells {
			mv.textValues[cell].Text = mv.inputBuffer.String()
			mv.textValues[cell].Refresh()
		}

	case fyne.KeyEnter, fyne.KeyReturn:
		if mv.inputBuffer.Len() == 0 {
			return
		}
		for _, cell := range mv.selectedCells {
			buff := mv.inputBuffer.String()
			if strings.Contains(buff, ".") {
				num, err := strconv.ParseFloat(buff, 64)
				if err != nil {
					fyne.LogError("Error parsing float", err)
					return
				}
				mv.zData[cell] = num
				log.Println("Set", cell, num)
			} else {
				num, err := strconv.Atoi(mv.inputBuffer.String())
				if err == nil {
					mv.zData[cell] = float64(num)
				}
			}
		}
		mv.updateCells()
		mv.Refresh()
		mv.restoreValues = false
		mv.inputBuffer.Reset()
	case fyne.KeyPageUp, "S":
		base := 10.0
		if dr, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
			if dr.CurrentKeyModifiers()&fyne.KeyModifierShift != 0 {
				base = 100
			}
		}

		increment := base * math.Pow(10, -float64(mv.zPrecision))
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += increment
		}
		mv.updateCells()
		refresh = true
	case fyne.KeyPageDown, "X":
		base := 10.0
		if dr, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
			if dr.CurrentKeyModifiers()&fyne.KeyModifierShift != 0 {
				base = 100
			}
		}
		increment := base * math.Pow(10, -float64(mv.zPrecision))
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= increment
		}
		mv.updateCells()
		refresh = true
	case "+", "A":
		increment := math.Pow(10, -float64(mv.zPrecision))
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += increment
		}
		mv.updateCells()
		refresh = true
	case "-", "Z":
		increment := math.Pow(10, -float64(mv.zPrecision))
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= increment
		}
		mv.updateCells()
		refresh = true
	case "Up":
		mv.SelectedY++
		if mv.SelectedY >= mv.numRows {
			mv.SelectedY = mv.numRows - 1
		}
		updateCursor = true
		mv.restoreSelectedValues()
	case "Down":
		mv.SelectedY--
		if mv.SelectedY < 0 {
			mv.SelectedY = 0
		}
		updateCursor = true
		mv.restoreSelectedValues()
	case "Left":
		mv.selectedX--
		if mv.selectedX < 0 {
			mv.selectedX = 0
		}
		updateCursor = true
		mv.restoreSelectedValues()
	case "Right":
		mv.selectedX++
		if mv.selectedX >= mv.numColumns {
			mv.selectedX = mv.numColumns - 1
		}
		updateCursor = true
		mv.restoreSelectedValues()
	}

	if updateCursor {
		mv.updateCursor(false)
	}

	if refresh {
		mv.Refresh()
	}
}
