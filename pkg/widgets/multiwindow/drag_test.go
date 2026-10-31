package multiwindow

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// Dragging a window past the viewport edge and back must leave it under the
// cursor: the clamp absorbs the overshoot instead of drifting the grab point.
func TestDragClampNoDrift(t *testing.T) {
	test.NewTempApp(t)

	m := NewMultipleWindows()
	m.LockViewport = true
	win := test.NewTempWindow(t, m)
	win.Resize(fyne.NewSize(800, 600))

	w := NewInnerWindow("a", widget.NewLabel("a"))
	m.Add(w, fyne.NewPos(100, 100))
	title := test.TempWidgetRenderer(t, w).(*innerWindowRenderer).title

	// Press at x=150, then drag left in steps, 190 units in total: 90 past the edge.
	x := float32(150)
	for _, to := range []float32{140, 50, -40} {
		title.Dragged(&fyne.DragEvent{
			PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(to, 110)},
			Dragged:    fyne.Delta{DX: to - x},
		})
		x = to
	}
	if got := w.Position().X; got != 0 {
		t.Fatalf("overshoot not clamped: x = %v, want 0", got)
	}

	title.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(150, 110)},
		Dragged:    fyne.Delta{DX: 150 - x},
	})
	title.DragEnd()
	if got := w.Position().X; got != 100 {
		t.Fatalf("window drifted off the cursor: x = %v, want 100", got)
	}
}
