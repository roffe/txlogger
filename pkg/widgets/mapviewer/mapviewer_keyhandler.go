package mapviewer

import (
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
)

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
	fyne.CurrentApp().Clipboard().SetContent(copyString.String())
}

func (mv *MapViewer) paste() {
	if !mv.editable {
		return
	}
	cb := fyne.CurrentApp().Clipboard().Content()
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
			mv.updateECUFunc(index, []int{mv.zData[index]})
		}
	}
	if len(split) >= 30 {
		mv.saveECUFunc(mv.zData)
	}
	mv.Refresh()
}

func (mv *MapViewer) smooth() {
	// Not enough elements to interpolate
	if len(mv.selectedCells) < 3 {
		return
	}

	values := make([]int, len(mv.selectedCells))

	for i, idx := range mv.selectedCells {
		values[i] = mv.zData[idx]
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
	mv.updateCells()
	mv.Refresh()
}

func (mv *MapViewer) updateCursor() {
	mv.selectedCells = []int{mv.SelectedY*mv.numColumns + mv.selectedX}
	xPosFactor := float32(mv.selectedX)
	yPosFactor := float32(float64(mv.numRows-1) - float64(mv.SelectedY))
	xPos := xPosFactor * mv.widthFactor
	yPos := yPosFactor * mv.heightFactor
	mv.cursor.Resize(fyne.NewSize(mv.widthFactor+1, mv.heightFactor+1))
	mv.cursor.Move(fyne.NewPos(xPos-1, yPos-1))
}

type updateBlock struct {
	idx  int
	end  int
	data []int
}

func (mv *MapViewer) updateCells() {
	if len(mv.selectedCells) == 0 {
		return
	}

	slices.Sort(mv.selectedCells)
	updates := []updateBlock{{mv.selectedCells[0], mv.selectedCells[0], []int{mv.zData[mv.selectedCells[0]]}}}
	for _, cell := range mv.selectedCells[1:] {
		data := mv.zData[cell]
		last := &updates[len(updates)-1]
		if cell-1 == last.end {
			last.end = cell
			last.data = append(last.data, data)
		} else {
			updates = append(updates, updateBlock{cell, cell, []int{data}})
		}
	}

	if mv.shouldFullSync(updates) {
		mv.fullSync()
		return
	}

	mv.partialSync(updates)
}

func (mv *MapViewer) shouldFullSync(updates []updateBlock) bool {
	var lenUpdates int
	for _, update := range updates {
		lenUpdates += len(update.data)
	}
	return float32(len(mv.selectedCells)) > float32(mv.numColumns*mv.numRows)*0.60 || lenUpdates > 127
}

func (mv *MapViewer) fullSync() {
	mv.saveECUFunc(mv.zData)
}

func (mv *MapViewer) partialSync(updates []updateBlock) {
	for _, update := range updates {
		mv.updateECUFunc(update.idx, update.data)
	}
}
