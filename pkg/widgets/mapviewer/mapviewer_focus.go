package mapviewer

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/debug"
)

func (mw *MapViewer) Tapped(_ *fyne.PointEvent) {
	fyne.CurrentApp().Driver().CanvasForObject(mw).Focus(mw)
}

func (mw *MapViewer) TappedSecondary(_ *fyne.PointEvent) {
}

func (mw *MapViewer) FocusGained() {
	mw.focused = true
}
func (mw *MapViewer) FocusLost() {
	mw.focused = false
}
func (mw *MapViewer) Focused() bool {
	return mw.focused
}

func (mv *MapViewer) TypedRune(r rune) {
	// print out the rune
	//	log.Println("TypedRune", string(r))
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
				switch mv.zCorrFac {
				case 1:
					mv.zData[cell] = int(num)
				case 0.1:
					mv.zData[cell] = int(num * 10)
				case 0.01:
					mv.zData[cell] = int(num * 100)
				case 0.001:
					mv.zData[cell] = int(num * 1000)
				case oneHundredTwentyeighth:
					mv.zData[cell] = int(num * 128)
				case oneThousandTwentyfourth:
					mv.zData[cell] = int(num * 1024)
				case 0.00390625:
					mv.zData[cell] = int(num * 256)
				case 0.004:
					mv.zData[cell] = int(num * 250)
				default:
					fyne.LogError("unknown zCorrFac", fmt.Errorf("%f", mv.zCorrFac))
					debug.Log(fmt.Sprintf("%s unknown zCorrFac: %f", mv.symbol.Name, mv.zCorrFac))
				}
			} else {
				num, err := strconv.Atoi(mv.inputBuffer.String())
				if err == nil {
					mv.zData[cell] = num
				}
			}
		}
		mv.updateCells()
		mv.Refresh()
		mv.restoreValues = false
		mv.inputBuffer.Reset()
	case fyne.KeyPageUp, "S":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += int((mv.zCorrFac*10)*(1.0/mv.zCorrFac) + mv.zCorrOffset)
		}
		mv.updateCells()
		refresh = true
	case fyne.KeyPageDown, "X":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= int((mv.zCorrFac*10)*(1.0/mv.zCorrFac) + mv.zCorrOffset)
		}
		mv.updateCells()
		refresh = true
	case "+", "A":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += int(mv.zCorrFac*(1.0/mv.zCorrFac) + mv.zCorrOffset)
		}
		mv.updateCells()
		refresh = true
	case "-", "Z":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= int(mv.zCorrFac*(1.0/mv.zCorrFac) + mv.zCorrOffset)
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
		mv.updateCursor()
	}

	if refresh {
		mv.Refresh()
	}
}
