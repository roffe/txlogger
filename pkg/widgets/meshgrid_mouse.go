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
	dx := float64(event.Position.X - m.lastMouseX)
	dy := float64(event.Position.Y - m.lastMouseY)

	// Scale factors to make movements less sensitive
	const (
		rotationScale = 0.5 // Reduce rotation speed
		rollScale     = 0.3 // Reduce roll speed
	)

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			// Left mouse: Rotate mesh
			// Invert dx for more intuitive rotation (moving mouse right rotates mesh right)
			// dx controls rotation around Y axis, dy controls rotation around X axis
			m.rotateMeshgrid(0, -dy*rotationScale, -dx*rotationScale)
		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			// Right mouse: Roll mesh around Z axis
			roll := (dx + dy) * rollScale
			m.rotateMeshgrid(roll, 0, 0)
		}

		m.Refresh()
	}

	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y
}

// MouseDown is called when a mouse button is pressed
func (m *Meshgrid) MouseDown(event *desktop.MouseEvent) {
	if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary ||
		event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary ||
		event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
		m.isDragging = true
	}
	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y
}

// MouseUp is called when a mouse button is released
func (m *Meshgrid) MouseUp(event *desktop.MouseEvent) {
	m.isDragging = false
}

func (m *Meshgrid) Scrolled(event *fyne.ScrollEvent) {
	// Add smoother zoom with reduced sensitivity
	if event.Scrolled.DY > 0 {
		m.scaleMeshgrid(1.1) // Zoom in slightly less aggressively
	} else {
		m.scaleMeshgrid(0.9) // Zoom out slightly less aggressively
	}
	m.Refresh()
}
