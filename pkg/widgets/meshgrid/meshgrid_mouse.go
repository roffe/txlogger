package meshgrid

import (
	"image"
	"math"

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
	rotationScale = 0.5
	rollScale     = 0.3
	panScale      = 0.5
)

func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	m.mousePosition = image.Point{X: int(event.Position.X), Y: int(event.Position.Y)}

	dx := float64(event.Position.X - m.lastMouseX)
	dy := float64(event.Position.Y - m.lastMouseY)

	if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
		// Primary button: vertical drag (dy) controls tilting toward/away (pitch)
		// Horizontal drag (dx) controls rotation around vertical axis (yaw)
		m.rotateMeshgrid(-dy*rotationScale, dx*rotationScale, 0)
		m.refresh()
	} else if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
		// Secondary button: control roll rotation
		roll := (dx + dy) * rollScale
		m.rotateMeshgrid(0, 0, roll)
		m.refresh()
	} else if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
		// Tertiary button (middle): control panning
		// Swap left-right direction by negating dx
		m.panMeshgrid(-dx*panScale, dy*panScale)
		m.refresh()
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
	m.refresh()
}

func (m *Meshgrid) findValueAtPosition(pos image.Point) (float64, bool) {
	minDist := math.MaxFloat64
	var closestVertices []struct {
		value   float64
		dist    float64
		screenX int
		screenY int
	}

	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			screenX, screenY := m.project(m.vertices[i][j])
			dx := float64(screenX - pos.X)
			dy := float64(screenY - pos.Y)
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist < minDist {
				minDist = dist
				value := m.values[i*m.cols+j]
				closestVertices = append(closestVertices[:0], struct {
					value   float64
					dist    float64
					screenX int
					screenY int
				}{value, dist, screenX, screenY})
			} else if math.Abs(dist-minDist) < 1.0 {
				value := m.values[i*m.cols+j]
				closestVertices = append(closestVertices, struct {
					value   float64
					dist    float64
					screenX int
					screenY int
				}{value, dist, screenX, screenY})
			}
		}
	}

	if len(closestVertices) > 0 {
		var weightedSum, weightSum float64
		for _, v := range closestVertices {
			weight := 1.0 / (v.dist + 0.0001)
			weightedSum += v.value * weight
			weightSum += weight
		}

		if weightSum > 0 {
			return weightedSum / weightSum, true
		}
	}

	return 0, false
}

// New method to handle panning in camera space
func (m *Meshgrid) panMeshgrid(dx, dy float64) {
	// Convert screen-space movement to camera-space movement
	// For this, we need to use the camera's right and up vectors
	rightVector := m.cameraRotation.MultiplyVector([3]float64{1, 0, 0})
	upVector := m.cameraRotation.MultiplyVector([3]float64{0, -1, 0}) // Negative because screen Y is down

	// Scale the movement
	for i := range rightVector {
		rightVector[i] *= dx
		upVector[i] *= dy
	}

	// Update camera position by moving along right and up vectors
	for i := range m.cameraPosition {
		m.cameraPosition[i] += rightVector[i] + upVector[i]
	}

	// Update all vertex positions based on the new camera
	m.updateVertexPositions()
}
