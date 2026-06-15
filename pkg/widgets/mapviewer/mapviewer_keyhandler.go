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
			mv.setCellText(cell, mv.cfg.ZData[cell])
		}
		mv.restoreValues = false
	}
}

func (mv *MapViewer) TypedShortcut(shortcut fyne.Shortcut) {
	switch shortcut.ShortcutName() {
	case "Copy":
		mv.copy()
	case "Paste":
		// Ctrl+V pastes at the current cursor/selection position rather than
		// the coordinates the data was originally copied from.
		mv.pasteHere()
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
		copyString.WriteString(fmt.Sprintf("%d:%d:%g:"+copyPasteSeparator, x, y, mv.cfg.ZData[cell]))
	}
	fyne.CurrentApp().Clipboard().SetContent(copyString.String())
}

type clipboardCell struct {
	x, y  int // storage coordinates (y already converted to storage row)
	value float64
}

// parseClipboardCells parses the internal copy format into cells with storage
// coordinates. The first cell is the anchor; its x carries the +20/+200 marker,
// which is stripped here.
func (mv *MapViewer) parseClipboardCells(cb string) []clipboardCell {
	split := strings.Split(cb, copyPasteSeparator)
	cells := make([]clipboardCell, 0, len(split))
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
		value, err := strconv.ParseFloat(pp[2], 64)
		if err != nil {
			log.Println(err)
			continue
		}
		cells = append(cells, clipboardCell{x: x, y: mv.numRows - 1 - y, value: value})
	}
	return cells
}

// applyPaste writes the parsed cells into ZData, offset by shiftX/shiftY. Cells
// that fall outside the map bounds are skipped. When bounds is non-nil, only
// destination cells present in the set are written, so the paste stays inside
// the current selection.
func (mv *MapViewer) applyPaste(cells []clipboardCell, shiftX, shiftY int, bounds map[int]struct{}) {
	changed := false
	for _, c := range cells {
		x := c.x + shiftX
		y := c.y + shiftY
		if x < 0 || x >= mv.numColumns || y < 0 || y >= mv.numRows {
			continue
		}
		index := y*mv.numColumns + x
		if index < 0 || index >= len(mv.cfg.ZData) {
			continue
		}
		if bounds != nil {
			if _, ok := bounds[index]; !ok {
				continue
			}
		}
		mv.cfg.ZData[index] = c.value
		changed = true
	}
	if changed {
		mv.Refresh()
	}
}

// paste writes the clipboard back to the same coordinates it was copied from.
func (mv *MapViewer) paste() {
	if !mv.cfg.Editable {
		return
	}
	cells := mv.parseClipboardCells(fyne.CurrentApp().Clipboard().Content())
	mv.applyPaste(cells, 0, 0, nil)
}

// pasteHere writes the clipboard with its anchor cell landing on the currently
// selected cell, so the block is pasted starting where the user right-clicked.
func (mv *MapViewer) pasteHere() {
	if !mv.cfg.Editable {
		return
	}
	cells := mv.parseClipboardCells(fyne.CurrentApp().Clipboard().Content())
	if len(cells) == 0 {
		return
	}
	// Anchor on the visual top-left of the copied block (smallest column,
	// topmost row). Data row 0 is drawn at the bottom, so the topmost row is the
	// largest storage y. This makes the block fill down and to the right from
	// the clicked cell, matching the on-screen orientation.
	minX, maxY := cells[0].x, cells[0].y
	for _, c := range cells[1:] {
		if c.x < minX {
			minX = c.x
		}
		if c.y > maxY {
			maxY = c.y
		}
	}
	// When more than a single cell is selected, confine the paste to that
	// selection so values can't spill outside the highlighted block.
	var bounds map[int]struct{}
	if len(mv.selectedCells) > 1 {
		bounds = make(map[int]struct{}, len(mv.selectedCells))
		for _, cell := range mv.selectedCells {
			bounds[cell] = struct{}{}
		}
	}
	mv.applyPaste(cells, mv.selectedX-minX, mv.SelectedY-maxY, bounds)
}

func (mv *MapViewer) smooth() {
	if len(mv.selectedCells) < 3 {
		return
	}

	values := make([]float64, len(mv.selectedCells))

	for i, idx := range mv.selectedCells {
		values[i] = mv.cfg.ZData[idx]
	}

	start := values[0]
	end := values[len(values)-1]

	// Calculate the step for interpolation for the internal elements
	step := float64(end-start) / float64(len(values)-1)

	// Interpolate internal values
	for i := 1; i < len(values)-1; i++ {
		values[i] = start + float64(i)*step
	}
	for i, idx := range mv.selectedCells {
		mv.cfg.ZData[idx] = values[i]
	}
	mv.updateCells()
	mv.Refresh()
}

// updateCursor collapses the selection to the single cell at selectedX/SelectedY
// and redraws the overlay highlight. Used by keyboard navigation and the
// crosshair-follow cursor. Pass goroutine=true when called off the main thread.
func (mv *MapViewer) updateCursor(goroutine bool) {
	cell := mv.SelectedY*mv.numColumns + mv.selectedX
	if len(mv.selectedCells) == 1 && mv.selectedCells[0] == cell {
		return
	}
	apply := func() {
		mv.clearSelectionVisual()
		mv.selectedCells = append(mv.selectedCells[:0], cell)
		mv.drawSelectionVisual()
	}
	if goroutine {
		fyne.Do(apply)
	} else {
		apply()
	}
}

type updateBlock struct {
	idx  int
	end  int
	data []float64
}

func (mv *MapViewer) updateCells() {
	if len(mv.selectedCells) == 0 {
		return
	}

	slices.Sort(mv.selectedCells)
	updates := []*updateBlock{
		{mv.selectedCells[0], mv.selectedCells[0], []float64{mv.cfg.ZData[mv.selectedCells[0]]}},
	}
	for _, cell := range mv.selectedCells[1:] {
		data := mv.cfg.ZData[cell]
		last := updates[len(updates)-1]
		if cell-1 == last.end {
			last.end = cell
			last.data = append(last.data, data)
		} else {
			updates = append(updates, &updateBlock{cell, cell, []float64{data}})
		}
	}

	if mv.shouldFullSync(updates) {
		mv.fullSync()
		return
	}

	mv.partialSync(updates)
}

func (mv *MapViewer) shouldFullSync(updates []*updateBlock) bool {
	var lenUpdates int
	for _, update := range updates {
		lenUpdates += len(update.data)
	}
	return float32(len(mv.selectedCells)) > float32(mv.numColumns*mv.numRows)*0.60 || lenUpdates > 127
}

func (mv *MapViewer) fullSync() {
	mv.cfg.SaveECUFunc(mv.cfg.ZData)
}

func (mv *MapViewer) partialSync(updates []*updateBlock) {
	for _, update := range updates {
		mv.cfg.OnUpdateCell(update.idx, update.data)
	}
}
