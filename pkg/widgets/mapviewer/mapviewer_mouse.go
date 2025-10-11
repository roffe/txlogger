package mapviewer

import (
	"math"
	"slices"

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
	//log.Println("MouseMoved", event)
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
	//log.Println("MouseDown")
	mv.lastModifier = event.Modifier
	if mv.OnMouseDown != nil {
		mv.OnMouseDown()
	}

	if event.Position.Y > mv.xAxisLabelContainer.Size().Height+mv.innerView.Size().Height {
		return
	}

	if event.Modifier != fyne.KeyModifierControl {
		for _, rect := range mv.zDataRects {
			if rect.FillColor != rect.StrokeColor {
				rect.FillColor = rect.StrokeColor
				rect.Refresh()
			}
		}
	}

	mv.handleFocusAndInputBuffer()

	switch {
	case event.Button == desktop.MouseButtonPrimary && event.Modifier == 0:
		if mv.selectionRect.Hidden {
			mv.selectionRect.Resize(fyne.NewSize(mv.widthFactor, mv.heightFactor))
			mv.selectionRect.Show()
		}
		mv.handlePrimaryClick(event)

	case event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierShift:
		if mv.selectionRect.Hidden {
			mv.selectionRect.Resize(fyne.NewSize(mv.widthFactor, mv.heightFactor))
			mv.selectionRect.Show()
		}
		mv.handlePrimaryClickWithShift(event)

	case event.Button == desktop.MouseButtonPrimary && event.Modifier == fyne.KeyModifierControl:
		if mv.selectionRect.Hidden {
			mv.selectionRect.Resize(fyne.NewSize(mv.widthFactor, mv.heightFactor))
			mv.selectionRect.Show()
		}
		mv.handlePrimaryCtrlClick(event)

	case event.Button == desktop.MouseButtonSecondary && event.Modifier == 0:
		mv.handleSecondaryClick(event)
	}
}

// MouseUp is called when a mouse button is released.
func (mv *MapViewer) MouseUp(event *desktop.MouseEvent) {
	// log.Println("MouseUp", event)
	mv.selectionRect.Hide()
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

func (mv *MapViewer) handlePrimaryCtrlClick(event *desktop.MouseEvent) {
	mv.selectedX, mv.SelectedY = mv.calculateSelectionBounds(event.Position)

	cellWidth, cellHeight := mv.calculateCellDimensions()
	x := (float32(mv.selectedX) * cellWidth)
	y := (float32(mv.numRows-mv.SelectedY-1) * cellHeight)

	mv.updateCursorPositionAndSize(x, y, cellWidth, cellHeight)

	newCell := mv.SelectedY*mv.numColumns + mv.selectedX

	// Check if cell is already selected
	if index := slices.Index(mv.selectedCells, newCell); index != -1 {
		// Remove cell if already selected
		mv.selectedCells = append(mv.selectedCells[:index], mv.selectedCells[index+1:]...)
		mv.zDataRects[newCell].FillColor = mv.zDataRects[newCell].StrokeColor
	} else {
		// Add new cell
		mv.selectedCells = append(mv.selectedCells, newCell)
		mv.zDataRects[newCell].FillColor = theme.Color(theme.ColorNameForegroundOnPrimary)
	}
	mv.zDataRects[newCell].Refresh()
}

// handlePrimaryClickWithShift handles the primary click with shift action.
func (mv *MapViewer) handlePrimaryClickWithShift(event *desktop.MouseEvent) {
	nselectedX, nSelectedY := mv.calculateSelectionBounds(event.Position)
	mv.updateSelection(nselectedX, nSelectedY)
	mv.selecting = true
}

func (mv *MapViewer) handleSecondaryClick(event *desktop.MouseEvent) {
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
	//xAxisOffset := mv.yAxisLabelContainer.Size().Width
	//yAxisOffset := mv.xAxisLabelContainer.Size().Height

	// Adjust for inner view position relative to the parent container
	// This accounts for any extra padding or layout adjustments
	innerViewPos := mv.innerView.Position()

	nselectedX := max(0, min(int((eventPos.X-innerViewPos.X))/int(cellWidth), mv.numColumns-1))
	nSelectedY := max(0, min(mv.numRows-int((eventPos.Y-innerViewPos.Y))/int(cellHeight)-1, mv.numRows-1))

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
	mv.selectionRect.Resize(fyne.NewSize(width+2, height+1))
	mv.selectionRect.Move(fyne.NewPos(topLeftX-1, topLeftY))
}

// handleFocusAndInputBuffer focuses the MapViewer and clears the input buffer if necessary.
func (mv *MapViewer) handleFocusAndInputBuffer() {
	if mv.inputBuffer.Len() > 0 {
		mv.inputBuffer.Reset()
		mv.restoreSelectedValues()
	}
}

// finalizeSelection finalizes the selection process.
func (mv *MapViewer) finalizeSelection(eventPos fyne.Position) {
	// log.Println("finalizeSelection")
	mv.selecting = false

	nselectedX, nSelectedY := mv.calculateSelectionBounds(eventPos)
	mv.updateSelection(nselectedX, nSelectedY)

	// For Ctrl selections, we don't want to clear existing selections
	if mv.lastModifier != fyne.KeyModifierControl {
		mv.selectedCells = make([]int, 0)
	}

	topLeftX := min(mv.selectedX, nselectedX)
	bottomRightX := max(mv.selectedX, nselectedX)
	topLeftY := min(mv.SelectedY, nSelectedY)
	bottomRightY := max(mv.SelectedY, nSelectedY)

	for y := topLeftY; y <= bottomRightY; y++ {
		for x := topLeftX; x <= bottomRightX; x++ {
			zIndex := y*mv.numColumns + x
			if mv.lastModifier == fyne.KeyModifierControl {
				// For Ctrl, toggle selection
				if index := slices.Index(mv.selectedCells, zIndex); index != -1 {
					mv.selectedCells = append(mv.selectedCells[:index], mv.selectedCells[index+1:]...)
					mv.zDataRects[zIndex].FillColor = mv.zDataRects[zIndex].StrokeColor
				} else {
					mv.selectedCells = append(mv.selectedCells, zIndex)
					mv.zDataRects[zIndex].FillColor = theme.Color(theme.ColorNameForegroundOnPrimary)
				}
			} else {
				mv.selectedCells = append(mv.selectedCells, zIndex)
				mv.zDataRects[zIndex].FillColor = theme.Color(theme.ColorNameForegroundOnPrimary)
			}
			mv.zDataRects[zIndex].Refresh()
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

		if mv.cfg.Editable {
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
