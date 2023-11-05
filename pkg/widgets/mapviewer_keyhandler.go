package widgets

import (
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
)

func (mv *MapViewer) TypedRune(r rune) {
	// print out the rune
	log.Println("TypedRune", string(r))
}

func (mv *MapViewer) restoreSelectedValues() {
	if mv.restoreValues {
		mv.inputBuffer.Reset()
		for _, cell := range mv.selectedCells {
			mv.SetCellText(cell, mv.zData[cell])
		}
		mv.restoreValues = false
	}
}

func (mv *MapViewer) TypedShortcut(shortcut fyne.Shortcut) {
	log.Println(shortcut.ShortcutName())
	switch shortcut.ShortcutName() {
	case "Copy":
		mv.copy()
	case "Paste":
		mv.paste()
	}

}

const (
	copyPasteSeparator = "~"
)

func (mv *MapViewer) copy() {
	var copyString strings.Builder
	slices.Sort(mv.selectedCells)
	for i, cell := range mv.selectedCells {
		x := cell % mv.numColumns // Integer division gives the row
		y := cell / mv.numColumns // Modulus gives the columny
		y = (mv.numRows - 1) - y
		if i == 0 {
			if x < 10 {
				x += 20
			} else if x >= 10 {
				x += 200
			}
		}
		copyString.WriteString(fmt.Sprintf("%d:%d:%d:"+copyPasteSeparator, x, y, mv.zData[cell]))
	}
	fyne.CurrentApp().Driver().AllWindows()[0].Clipboard().SetContent(copyString.String())
}

func (mv *MapViewer) paste() {
	cb := fyne.CurrentApp().Driver().AllWindows()[0].Clipboard().Content()
	split := strings.Split(cb, copyPasteSeparator)
	for i, part := range split {
		if len(part) < 3 {
			continue
		}
		pp := strings.Split(part, ":")

		if len(pp) < 3 {
			continue
		}

		x, err := strconv.Atoi(pp[0])
		if err != nil {
			log.Println(err)
			continue
		}

		if i == 0 && x >= 200 {
			x -= 200
		} else if i == 0 && x >= 20 {
			x -= 20
		}

		y, err := strconv.Atoi(pp[1])
		if err != nil {
			log.Println(err)
			continue
		}
		value, err := strconv.Atoi(pp[2])
		if err != nil {
			log.Println(err)
			continue
		}
		y = mv.numRows - 1 - y

		index := y*mv.numColumns + x
		if index < 0 || index >= len(mv.zData) {
			log.Printf("Index out of range: %d", index)
			continue
		}
		mv.zData[index] = value
		if len(split) < 30 {
			mv.updateFunc(index, []int{mv.zData[index]})
		}
	}
	if len(split) >= 30 {
		mv.saveFunc(mv.zData)
	}
	mv.Refresh()
}

func (mv *MapViewer) smooth() {
	values := make([]int, len(mv.selectedCells))

	for i, idx := range mv.selectedCells {
		values[i] = mv.zData[idx]
	}

	if len(values) <= 2 {
		// Not enough elements to interpolate
		return
	}

	start := values[0]
	end := values[len(values)-1]

	// Calculate the step for interpolation for the internal elements
	step := float64(end-start) / float64(len(values)-1)

	// Interpolate internal values
	for i := 1; i < len(values)-1; i++ {
		values[i] = start + int(float64(i)*step+0.5) // Adding 0.5 for rounding to nearest integer
	}

	for i, idx := range mv.selectedCells {
		mv.zData[idx] = values[i]
	}
	mv.Refresh()
}

func (mv *MapViewer) TypedKey(key *fyne.KeyEvent) {
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
				default:
					fyne.LogError("Unknown zCorrFac", fmt.Errorf("%f", mv.zCorrFac))
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
			mv.zData[cell] += int((mv.zCorrFac * 10) * (1.0 / mv.zCorrFac))
		}
		mv.updateCells()
		refresh = true
	case fyne.KeyPageDown, "X":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= int((mv.zCorrFac * 10) * (1.0 / mv.zCorrFac))
		}
		mv.updateCells()
		refresh = true
	case "+", "A":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] += int(mv.zCorrFac * (1.0 / mv.zCorrFac))
		}
		mv.updateCells()
		refresh = true
	case "-", "Z":
		for _, cell := range mv.selectedCells {
			mv.zData[cell] -= int(mv.zCorrFac * (1.0 / mv.zCorrFac))
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
		mv.Refresh()
	}
}

type updateBlock struct {
	idx  int
	end  int
	data []int
}

func (mv *MapViewer) updateCells() {
	var updates []updateBlock

	slices.Sort(mv.selectedCells)

	for _, cell := range mv.selectedCells {
		data := mv.zData[cell]
		// if first, add the first entry
		if len(updates) == 0 {
			updates = append(updates, updateBlock{cell, cell, []int{data}})
			continue
		}

		last := updates[len(updates)-1]

		if cell-1 == last.end {
			last.end = cell
			last.data = append(last.data, data)
			updates[len(updates)-1] = last
			continue
		}
		updates = append(updates, updateBlock{cell, cell, []int{data}})
	}

	// Bigger than 60% of the map, sync the whole thing
	if float32(len(mv.selectedCells)) > float32(mv.numColumns*mv.numRows)*.60 || len(updates) > 127 {
		if mv.saveFunc != nil {
			mv.saveFunc(mv.zData)
			return
		}
	}

	// Do partial updates
	for _, update := range updates {
		//		log.Println("update", update.idx, update.end, update.data)
		if mv.updateFunc != nil {
			mv.updateFunc(update.idx, update.data)
		}
	}
}
