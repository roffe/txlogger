package meshgrid

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

//func (m *Meshgrid) Cursor() desktop.Cursor {
//	return desktop.CrosshairCursor
//}

var _ desktop.Hoverable = (*Meshgrid)(nil)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
}

// Add this method to clear crosshair when mouse leaves
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

const (
	rotationScale = 0.6
	rollScale     = 0.4
	panScale      = 0.8
)

func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	m.mousePosition = image.Point{X: int(event.Position.X), Y: int(event.Position.Y)}

	dx := float64(event.Position.X - m.lastMouseX)
	dy := float64(event.Position.Y - m.lastMouseY)

	if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
		// Primary button: vertical drag (dy) controls tilting toward/away (pitch)
		// Horizontal drag (dx) controls rotation around vertical axis (yaw)
		m.rotateMeshgrid(-dy*rotationScale, dx*rotationScale, 0)
		m.throttledRefresh()
	} else if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
		// Secondary button: control roll rotation
		roll := (dx + dy) * rollScale
		m.rotateMeshgrid(0, 0, roll)
		m.throttledRefresh()
	} else if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
		// Tertiary button (middle): control panning
		// Swap left-right direction by negating dx
		m.panMeshgrid(dx*panScale, dy*panScale)
		m.throttledRefresh()
	}

	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y
}

func (m *Meshgrid) MouseDown(event *desktop.MouseEvent) {

}

// MouseUp is called when a mouse button is released
func (m *Meshgrid) MouseUp(event *desktop.MouseEvent) {
}

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
