package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

const (
	rotationSensitivity = 1 // Adjust as necessary
)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
}

func (m *Meshgrid) MouseOut() {
}

// MouseMoved is called when the mouse is moved over the map viewer.
func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	dx := float64(event.Position.X - m.lastMouseX) // Change in X position
	dy := float64(event.Position.Y - m.lastMouseY) // Change in Y position

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			m.ax += -dy * rotationSensitivity // Rotate around X-axis
			m.ay += dx * rotationSensitivity  // Rotate around Y-axis
			//m.rotateMeshgrid(ax, ay, 0)     // Assuming no rotation around Z-axis for simplicity
		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			m.ax += -dy * rotationSensitivity // Rotate around X-axis
			m.az += dx * rotationSensitivity  // Rotate around Z-axis
			//m.rotateMeshgrid(ax, 0, az)
		}
		if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
			m.px += -dx // Move along X-axis
			m.py += -dy // Move along Y-axis
		}

		if m.ax > 90 {
			m.ax = 90
		}
		if m.ax < -90 {
			m.ax = -90
		}

		if m.ay > 90 {
			m.ay = 90
		}
		if m.ay < -90 {
			m.ay = -90
		}

		if m.az > 90 {
			m.az = 90
		}
		if m.az < -90 {
			m.az = -90
		}

		m.Refresh()
	}
	// Update the last mouse position
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
		m.sx += .03
		m.sy += .03
		m.sz += .03
	} else {
		m.sx -= .03
		m.sy -= .03
		m.sz -= .03
	}
	m.Refresh()
}
