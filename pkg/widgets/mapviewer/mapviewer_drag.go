//go:build !android && !ios

package mapviewer

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
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
