package widgets

import (
	"image"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
	m.showCrosshair = true
	m.Refresh()
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

// Update MouseMoved to track position for crosshair
func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	// Store current mouse position for crosshair
	m.mousePosition = image.Point{X: int(event.Position.X), Y: int(event.Position.Y)}
	m.showCrosshair = true

	dx := float64(event.Position.X - m.lastMouseX)
	dy := float64(event.Position.Y - m.lastMouseY)

	// Scale factors to make movements less sensitive
	const (
		rotationScale = 0.5 // Reduce rotation speed
		rollScale     = 0.3 // Reduce roll speed
	)

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			m.rotateMeshgrid(0, dy*rotationScale, dx*rotationScale)
		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			roll := (dx + dy) * rollScale
			m.rotateMeshgrid(-roll, 0, 0)
		}
	}

	m.lastMouseX = event.Position.X
	m.lastMouseY = event.Position.Y

	// Refresh to update crosshair
	m.Refresh()
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

func (m *Meshgrid) findValueAtPosition(pos image.Point) (float64, bool) {
	// Find the closest vertices to the mouse position
	minDist := math.MaxFloat64
	var closestVertices []struct {
		value   float64
		dist    float64
		screenX int
		screenY int
	}

	// Check all vertices
	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			screenX, screenY := m.project(m.vertices[i][j])
			dx := float64(screenX - pos.X)
			dy := float64(screenY - pos.Y)
			dist := math.Sqrt(dx*dx + dy*dy)

			// Keep track of the closest vertices
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

	// If we found some close vertices
	if len(closestVertices) > 0 {
		// Weight the values by inverse distance
		var weightedSum, weightSum float64
		for _, v := range closestVertices {
			weight := 1.0 / (v.dist + 0.0001) // Add small constant to avoid division by zero
			weightedSum += v.value * weight
			weightSum += weight
		}

		if weightSum > 0 {
			return weightedSum / weightSum, true
		}
	}

	return 0, false
}
