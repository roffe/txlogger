package widgets

import (
	"log"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (mv *MapViewer) MouseIn(_ *desktop.MouseEvent) {
}

func (mv *MapViewer) MouseOut() {
}

// MouseMoved is called when the mouse is moved over the map viewer.
func (mv *MapViewer) MouseMoved(event *desktop.MouseEvent) {
	if !mv.selecting {
		return
	}
	if event.Button&desktop.MouseButtonPrimary != desktop.MouseButtonPrimary {
		mv.finalizeSelection(mv.mousePos)
		return
	}

	mv.mousePos = event.Position
	nselectedX, nSelectedY := mv.calculateSelectionBounds(event.Position)
	mv.updateSelection(nselectedX, nSelectedY)
}

// MouseDown is called when a mouse button is pressed.
func (mv *MapViewer) MouseDown(event *desktop.MouseEvent) {
	if event.Position.Y > mv.xAxisLabelContainer.Size().Height+mv.innerView.Size().Height {
		return
	}
	// Handle focusing and input buffer reset
	mv.handleFocusAndInputBuffer()

	//if event.Position.Y > mv.innerView.Size().Height {
	//	return
	//}

	switch {
	case event.Button == desktop.MouseButtonPrimary && event.Modifier == 0:
		mv.handlePrimaryClick(event)

	case event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierShift:
		mv.handlePrimaryClickWithShift(event)

	case event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierControl:
		log.Println("Control")

	case event.Button == desktop.MouseButtonSecondary && event.Modifier == 0:
		mv.handleSecondaryClick(event)
	}
}

// MouseUp is called when a mouse button is released.
func (mv *MapViewer) MouseUp(event *desktop.MouseEvent) {

	if event.Button == desktop.MouseButtonPrimary && mv.selecting {
		mv.finalizeSelection(event.Position)
	}
}

// handlePrimaryClick handles the primary click action.
func (mv *MapViewer) handlePrimaryClick(event *desktop.MouseEvent) {
	mv.selectedX, mv.SelectedY = mv.calculateSelectionBounds(event.Position)

	cellWidth, cellHeight := mv.calculateCellDimensions()
	x := (float32(mv.selectedX) * cellWidth)
	y := (float32(mv.numRows-mv.SelectedY-1) * cellHeight)

	mv.updateCursorPositionAndSize(x, y, cellWidth, cellHeight)
	mv.selectedCells = []int{mv.SelectedY*mv.numColumns + mv.selectedX}
	mv.selecting = true
}

// handlePrimaryClickWithShift handles the primary click with shift action.
func (mv *MapViewer) handlePrimaryClickWithShift(event *desktop.MouseEvent) {
	nselectedX, nSelectedY := mv.calculateSelectionBounds(event.Position)
	mv.updateSelection(nselectedX, nSelectedY)
	mv.selecting = true
}

func (mv *MapViewer) handleSecondaryClick(event *desktop.MouseEvent) {
	mv.showPopupMenu(event.Position)
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
	xAxisOffset := mv.yAxisLabelContainer.Size().Width + (theme.Padding() * 2)
	yAxisOffset := mv.xAxisLabelContainer.Size().Height + (theme.Padding() * 2)

	nselectedX := max(0, min(int(eventPos.X-xAxisOffset)/int(cellWidth), mv.numColumns-1))
	nSelectedY := max(0, min(mv.numRows-int(eventPos.Y-yAxisOffset)/int(cellHeight)-1, mv.numRows-1))

	return nselectedX, nSelectedY
}

// updateSelection updates the selection based on the new cursor position.
func (mv *MapViewer) updateSelection(nselectedX, nSelectedY int) {
	cellWidth, cellHeight := mv.calculateCellDimensions()
	difX := int(math.Abs(float64(nselectedX - mv.selectedX)))
	difY := int(math.Abs(float64(nSelectedY - mv.SelectedY)))

	topLeftX := float32(min(mv.selectedX, nselectedX)) * cellWidth
	topLeftY := float32(mv.numRows-1-max(mv.SelectedY, nSelectedY)) * cellHeight

	mv.updateCursorPositionAndSize(topLeftX, topLeftY, float32(difX+1)*cellWidth, float32(difY+1)*cellHeight)
}

// updateCursorPositionAndSize updates the cursor's position and size on the screen.
func (mv *MapViewer) updateCursorPositionAndSize(topLeftX, topLeftY, width, height float32) {
	mv.cursor.Resize(fyne.NewSize(width+2, height+1))
	mv.cursor.Move(fyne.NewPos(topLeftX-1, topLeftY-1))
}

// handleFocusAndInputBuffer focuses the MapViewer and clears the input buffer if necessary.
func (mv *MapViewer) handleFocusAndInputBuffer() {
	//if c := fyne.CurrentApp().Driver().CanvasForObject(mv); c != nil {
	//	c.Focus(mv)
	//}
	if mv.inputBuffer.Len() > 0 {
		mv.inputBuffer.Reset()
		mv.restoreSelectedValues()
	}
}

// finalizeSelection finalizes the selection process.
func (mv *MapViewer) finalizeSelection(eventPos fyne.Position) {
	mv.selecting = false

	nselectedX, nSelectedY := mv.calculateSelectionBounds(eventPos)
	mv.updateSelection(nselectedX, nSelectedY)

	// Calculate and store selected cells
	topLeftX := min(mv.selectedX, nselectedX)
	bottomRightX := max(mv.selectedX, nselectedX)
	topLeftY := min(mv.SelectedY, nSelectedY)
	bottomRightY := max(mv.SelectedY, nSelectedY)

	mv.selectedCells = make([]int, 0)
	for y := topLeftY; y <= bottomRightY; y++ {
		for x := topLeftX; x <= bottomRightX; x++ {
			zIndex := y*mv.numColumns + x
			mv.selectedCells = append(mv.selectedCells, zIndex)
		}
	}
}

func (mv *MapViewer) showPopupMenu(pos fyne.Position) {
	if mv.popup == nil {
		menu := fyne.NewMenu("Opts",
			fyne.NewMenuItem("Copy", func() {
				mv.copy()
			}),
		)

		if mv.editable {
			menu.Items = append(menu.Items,
				fyne.NewMenuItem("Paste", func() {
					mv.paste()
				}),
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
