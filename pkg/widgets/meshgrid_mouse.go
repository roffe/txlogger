package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

const (
	rotationSensitivity = .4 // Adjust as necessary
)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
}

func (m *Meshgrid) MouseOut() {
}

func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	dx := float64(event.Position.X - m.lastMouseX) // Change in X position
	dy := float64(event.Position.Y - m.lastMouseY) // Change in Y position

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			// Adjust rotation direction based on current orientation
			rotationFactorX := 1.0
			rotationFactorY := 1.0
			//if m.ay > 90 && m.ay <= 270 {
			//	rotationFactorX = -1.0
			//	rotationFactorY = -1.0
			//}
			// Rotate around X-axis relative to the current rotation
			m.ax += -dy * rotationSensitivity * rotationFactorX
			// Rotate around Y-axis relative to the current rotation
			m.ay += dx * rotationSensitivity * rotationFactorY
		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			// Rotate around Z-axis relative to the current rotation
			m.az += dx * rotationSensitivity
		}
		if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
			// Move along X-axis relative to the current rotation
			m.px -= dx
			// Move along Y-axis relative to the current rotation
			m.py -= dy
		}

		// Clamping angles to prevent flipping
		// Note: Consider removing these clamps or adjusting them based on your requirements

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

		m.Refresh()
	}
	// Update the last mouse position
	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y
}

func (m *Meshgrid) MouseMoved2(event *desktop.MouseEvent) {
	dx := float64(event.Position.X - m.lastMouseX) // Change in X position
	dy := float64(event.Position.Y - m.lastMouseY) // Change in Y position

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			// Adjust rotation direction based on current orientation
			rotationFactorX := 1.0
			rotationFactorY := 1.0
			if m.ay > 90 && m.ay <= 270 {
				rotationFactorX = -1.0
				rotationFactorY = -1.0
			}
			m.ax += -dy * rotationSensitivity * rotationFactorX // Rotate around X-axis
			m.ay += dx * rotationSensitivity * rotationFactorY  // Rotate around Y-axis
		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			m.az += dx * rotationSensitivity // Rotate around Z-axis
		}
		if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
			m.px += -dx // Move along X-axis
			m.py += -dy // Move along Y-axis
		}

		// Clamping angles to prevent flipping
		// Note: Consider removing these clamps or adjusting them based on your requirements

		if m.ax > 90 {
			m.ax = 90
		}
		if m.ax < -90 {
			m.ax = -90
		}

		if m.ay > 70 {
			m.ay = 70
		}
		if m.ay < -70 {
			m.ay = -70
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
