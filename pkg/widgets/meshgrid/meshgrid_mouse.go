package meshgrid

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

var _ desktop.Hoverable = (*Meshgrid)(nil)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
}

func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	m.mousePosition = image.Point{X: int(event.Position.X), Y: int(event.Position.Y)}
	dx := float64(event.Position.X - m.lastMouseX)
	dy := float64(event.Position.Y - m.lastMouseY)
	if m.dragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			m.rotateMeshgrid(-dy*rotationScale, dx*rotationScale, 0)
			m.throttledRefresh()
		} else if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			roll := (dx + dy) * rollScale
			m.rotateMeshgrid(0, 0, roll)
			m.throttledRefresh()
		} else if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
			m.panMeshgrid(dx*panScale, dy*panScale)
			m.throttledRefresh()
		}
	}
	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y
}

// Add this method to clear crosshair when mouse leaves
func (m *Meshgrid) MouseOut() {
	if m.dragging {
		m.dragging = false
	}
}

const (
	rotationScale = 0.6
	rollScale     = 0.4
	panScale      = 0.8
)

var _ desktop.Mouseable = (*Meshgrid)(nil)

func (m *Meshgrid) MouseDown(event *desktop.MouseEvent) {
	if f := m.OnMouseDown; f != nil {
		f()
	}
	m.dragging = true
}

func (m *Meshgrid) MouseUp(event *desktop.MouseEvent) {
	m.dragging = false
}

var _ fyne.Scrollable = (*Meshgrid)(nil)

func (m *Meshgrid) Scrolled(event *fyne.ScrollEvent) {
	if event.Scrolled.DY > 0 {
		m.scaleMeshgrid(1.1)
	} else {
		m.scaleMeshgrid(0.9)
	}
	m.throttledRefresh()
}

func (m *Meshgrid) panMeshgrid(dx, dy float64) {
	m.cameraPosition[0] -= dx * panScale // X axis (left/right)
	m.cameraPosition[1] -= dy * panScale // Y axis (up/down)
	m.updateVertexPositions()
}
