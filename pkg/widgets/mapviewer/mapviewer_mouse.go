package mapviewer

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Dragged is called when the user drags the window.
func (mv *MapViewer) Dragged(ev *fyne.DragEvent) {
	moveEvent := &desktop.MouseEvent{}
	moveEvent.Position = ev.Position
	if mv.selecting {
		moveEvent.Button = desktop.MouseButtonPrimary
	}
	mv.MouseMoved(moveEvent)
}

// DragEnd is called when the user stops dragging the window.
func (mv *MapViewer) DragEnd() {}

func (mv *MapViewer) MouseIn(_ *desktop.MouseEvent) {}
func (mv *MapViewer) MouseOut()                     {}

// MouseMoved is called when the mouse is moved over the map viewer.
func (mv *MapViewer) MouseMoved(event *desktop.MouseEvent) {
	// log.Println("MouseMoved", event)
	if !mv.selecting {
		return
	}
	if event.Button&desktop.MouseButtonPrimary != desktop.MouseButtonPrimary {
		mv.finalizeSelection(mv.mousePos)
		return
	}

	mv.mousePos = event.Position
	nx, ny := mv.calculateSelectionBounds(event.Position)
	// Only rebuild the selection when the drag crosses into a different cell.
	if nx == mv.dragCornerX && ny == mv.dragCornerY {
		return
	}
	mv.dragCornerX, mv.dragCornerY = nx, ny
	mv.selectRegion(nx, ny)
}

// MouseDown is called when a mouse button is pressed.
func (mv *MapViewer) MouseDown(event *desktop.MouseEvent) {
	// log.Println("MouseDown")
	mv.lastModifier = event.Modifier
	if mv.cfg.OnMouseDown != nil {
		mv.cfg.OnMouseDown()
	}

	if event.Position.Y > mv.xAxisLabelContainer.Size().Height+mv.innerView.Size().Height {
		return
	}

	mv.handleFocusAndInputBuffer()

	switch {
	case event.Button == desktop.MouseButtonPrimary && event.Modifier == 0:
		mv.handlePrimaryClick(event)

	case event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierShift:
		mv.handlePrimaryClickWithShift(event)

	case event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierControl:
		mv.handlePrimaryCtrlClick(event)

	case event.Button == desktop.MouseButtonSecondary && event.Modifier == 0:
		mv.handleSecondaryClick(event)
	}
}

// MouseUp is called when a mouse button is released.
func (mv *MapViewer) MouseUp(event *desktop.MouseEvent) {
	// log.Println("MouseUp", event)
	if event.Button == desktop.MouseButtonPrimary && mv.selecting {
		mv.finalizeSelection(event.Position)
	}
}

// handlePrimaryClick starts a fresh single-cell selection and begins a drag.
func (mv *MapViewer) handlePrimaryClick(event *desktop.MouseEvent) {
	mv.clearSelectionVisual()
	mv.selectedX, mv.SelectedY = mv.calculateSelectionBounds(event.Position)
	mv.dragCornerX, mv.dragCornerY = mv.selectedX, mv.SelectedY
	mv.selectedCells = append(mv.selectedCells[:0], mv.SelectedY*mv.numColumns+mv.selectedX)
	mv.drawSelectionVisual()
	mv.selecting = true
}

func (mv *MapViewer) handlePrimaryCtrlClick(event *desktop.MouseEvent) {
	mv.selectedX, mv.SelectedY = mv.calculateSelectionBounds(event.Position)
	newCell := mv.SelectedY*mv.numColumns + mv.selectedX

	// Toggle the clicked cell while keeping the rest of the selection.
	if index := slices.Index(mv.selectedCells, newCell); index != -1 {
		mv.selectedCells = append(mv.selectedCells[:index], mv.selectedCells[index+1:]...)
		mv.selectionRects[newCell].Hide()
	} else {
		mv.selectedCells = append(mv.selectedCells, newCell)
		mv.selectionRects[newCell].Show()
	}
	// Show()/Hide() only flip the Hidden flag, and canvas.Refresh on a rect
	// that has never been painted (a hidden overlay cell) is a no-op because the
	// object isn't in the canvas cache. Refresh the always-visible value rect for
	// this cell instead to dirty the canvas and force an immediate repaint that
	// draws the toggled highlight.
	canvas.Refresh(mv.zDataRects[newCell])
}

// handlePrimaryClickWithShift extends the selection from the current anchor to
// the clicked cell and begins a drag.
func (mv *MapViewer) handlePrimaryClickWithShift(event *desktop.MouseEvent) {
	nx, ny := mv.calculateSelectionBounds(event.Position)
	mv.dragCornerX, mv.dragCornerY = nx, ny
	mv.selectRegion(nx, ny)
	mv.selecting = true
}

// selectRegion replaces the current selection with the rectangular block
// between the anchor cell (selectedX/SelectedY) and the given corner, updating
// the overlay highlight live.
func (mv *MapViewer) selectRegion(cornerX, cornerY int) {
	minX, maxX := min(mv.selectedX, cornerX), max(mv.selectedX, cornerX)
	minY, maxY := min(mv.SelectedY, cornerY), max(mv.SelectedY, cornerY)

	mv.clearSelectionVisual()
	mv.selectedCells = mv.selectedCells[:0]
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			mv.selectedCells = append(mv.selectedCells, y*mv.numColumns+x)
		}
	}
	mv.drawSelectionVisual()
}

func (mv *MapViewer) handleSecondaryClick(event *desktop.MouseEvent) {
	x, y := mv.calculateSelectionBounds(event.Position)
	clickedCell := y*mv.numColumns + x

	// The clicked cell becomes the anchor for "Paste here" regardless.
	mv.selectedX, mv.SelectedY = x, y

	// Right-clicking inside the current selection keeps it (so Copy/Smooth act
	// on the whole block). Right-clicking outside it, or with no selection,
	// selects just that cell.
	if !slices.Contains(mv.selectedCells, clickedCell) {
		mv.clearSelectionVisual()
		mv.selectedCells = []int{clickedCell}
		mv.drawSelectionVisual()
	}

	mv.showPopupMenu(event.AbsolutePosition)
}

// calculateCellDimensions calculates and returns the width and height of a cell.
func (mv *MapViewer) calculateCellDimensions() (float32, float32) {
	cellWidth := mv.innerView.Size().Width / float32(mv.numColumns)
	cellHeight := mv.innerView.Size().Height / float32(mv.numRows)
	return cellWidth, cellHeight
}

// calculateSelectionBounds computes the bounding box of the selection area.
func (mv *MapViewer) calculateSelectionBounds(eventPos fyne.Position) (int, int) {
	cellWidth, cellHeight := mv.calculateCellDimensions()
	// xAxisOffset := mv.yAxisLabelContainer.Size().Width
	// yAxisOffset := mv.xAxisLabelContainer.Size().Height

	// Adjust for inner view position relative to the parent container
	// This accounts for any extra padding or layout adjustments
	innerViewPos := mv.innerView.Position()

	nselectedX := max(0, min(int((eventPos.X-innerViewPos.X))/int(cellWidth), mv.numColumns-1))
	nSelectedY := max(0, min(mv.numRows-int((eventPos.Y-innerViewPos.Y))/int(cellHeight)-1, mv.numRows-1))

	return nselectedX, nSelectedY
}

// handleFocusAndInputBuffer focuses the MapViewer and clears the input buffer if necessary.
func (mv *MapViewer) handleFocusAndInputBuffer() {
	if mv.inputBuffer.Len() > 0 {
		mv.inputBuffer.Reset()
		mv.restoreSelectedValues()
	}
}

// finalizeSelection ends a drag. The selection was already built live during
// MouseMoved, so this just snaps to the release position and stops selecting.
func (mv *MapViewer) finalizeSelection(eventPos fyne.Position) {
	// log.Println("finalizeSelection")
	mv.selecting = false
	nx, ny := mv.calculateSelectionBounds(eventPos)
	mv.selectRegion(nx, ny)
}

func (mv *MapViewer) showPopupMenu(pos fyne.Position) {
	if mv.popup == nil {
		menu := fyne.NewMenu("Opts",
			fyne.NewMenuItem("Copy", func() {
				mv.copy()
			}),
		)
		if mv.cfg.Editable {

			pasteMenu := fyne.NewMenuItem("Paste", nil)
			pasteMenu.ChildMenu = fyne.NewMenu("Paste Options",
				fyne.NewMenuItem("At original position", func() {
					mv.paste()
				}),
				fyne.NewMenuItem("At currently selected location", func() {
					mv.pasteHere()
				}),
			)

			menu.Items = append(menu.Items,
				pasteMenu,
				fyne.NewMenuItem("Smooth", func() {
					mv.smooth()
				}),
			)
		}
		popupMenu := widget.NewPopUpMenu(menu,
			fyne.CurrentApp().Driver().CanvasForObject(mv),
		)

		popupMenu.ShowAtPosition(pos)
		mv.popup = popupMenu
		return
	}
	mv.popup.ShowAtPosition(pos)
}
