package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
}

func (m *Meshgrid) MouseOut() {
}

/*
func (m *Meshgrid) Dragged(ev *fyne.DragEvent) {

	// Apply rotation
	//m.RotateMeshgrid(float64(ev.Dragged.DX), float64(ev.Dragged.DY))

	dx := -float64(ev.Dragged.DY)
	dy := float64(ev.Dragged.DX)

	log.Println("dx", dx, "dy", dy)

	m.rotateMeshgrid(dx, dy, 0)
	m.Refresh()

}

func (m *Meshgrid) DragEnd() {
}
*/

func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	dx := float64(event.Position.X - m.lastMouseX) // Change in X position
	dy := float64(event.Position.Y - m.lastMouseY) // Change in Y position

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			m.rotateMeshgrid(dy, -dx, 0)
		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			m.px -= dx
			m.py -= dy
		}
		if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
			m.rotateMeshgrid(0, 0, dx)
		}

		m.Refresh() // Update the rendered view after modifying rotation/translation
	}
	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y
}

// MouseDown is called when a mouse button is pressed over the map viewer.
func (m *Meshgrid) MouseDown(event *desktop.MouseEvent) {
	if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary ||
		event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary ||
		event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
		// When the primary button is pressed, start dragging
		m.isDragging = true
		m.lastMouseX = event.Position.X
		m.lastMouseY = event.Position.Y
	}

}

// MouseUp is called when a mouse button is released over the map viewer.
func (m *Meshgrid) MouseUp(event *desktop.MouseEvent) {
	// When any mouse button is released, stop dragging
	m.isDragging = false
}

func (m *Meshgrid) Scrolled(event *fyne.ScrollEvent) {
	if event.Scrolled.DY > 0 {
		m.scaleMeshgrid(1.03)
	} else {
		m.scaleMeshgrid(0.97)
	}
	m.Refresh()
}
