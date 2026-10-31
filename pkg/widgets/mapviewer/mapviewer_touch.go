//go:build android || ios

package mapviewer

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// Touch screens send taps and drags but no mouse down or up, so these feed the
// desktop mouse handlers. Tapped is not used on desktop, a click would run it
// after MouseDown and redo the selection.

// Tapped selects the tapped cell.
func (mv *MapViewer) Tapped(ev *fyne.PointEvent) {
	mv.MouseDown(&desktop.MouseEvent{PointEvent: *ev, Button: desktop.MouseButtonPrimary})
	mv.MouseUp(&desktop.MouseEvent{PointEvent: *ev, Button: desktop.MouseButtonPrimary})
}

// TappedSecondary is a long press, it opens the cell menu like a right click.
func (mv *MapViewer) TappedSecondary(ev *fyne.PointEvent) {
	mv.MouseDown(&desktop.MouseEvent{PointEvent: *ev, Button: desktop.MouseButtonSecondary})
}

// Dragged selects the block between where the finger went down and where it
// is now.
func (mv *MapViewer) Dragged(ev *fyne.DragEvent) {
	if !mv.selecting {
		down := fyne.PointEvent{Position: ev.Position.Subtract(ev.Dragged)}
		mv.MouseDown(&desktop.MouseEvent{PointEvent: down, Button: desktop.MouseButtonPrimary})
		if !mv.selecting { // started on the axis labels
			return
		}
	}
	mv.MouseMoved(&desktop.MouseEvent{PointEvent: ev.PointEvent, Button: desktop.MouseButtonPrimary})
}

func (mv *MapViewer) DragEnd() {
	up := fyne.PointEvent{Position: mv.mousePos}
	mv.MouseUp(&desktop.MouseEvent{PointEvent: up, Button: desktop.MouseButtonPrimary})
}
