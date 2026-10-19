//go:build android || ios

package meshgrid

import "fyne.io/fyne/v2"

// Touch screens send drags instead of mouse moves. Desktop keeps rotating in
// MouseMoved: implementing Draggable there would route primary and middle
// button drags here and stop MouseMoved during them.
var _ fyne.Draggable = (*Meshgrid)(nil)

// Dragged orbits like a primary button drag on desktop.
func (m *Meshgrid) Dragged(ev *fyne.DragEvent) {
	m.orbit(-float64(ev.Dragged.DX)*rotationScale, -float64(ev.Dragged.DY)*rotationScale)
	m.throttledRefresh()
}

func (m *Meshgrid) DragEnd() {}
