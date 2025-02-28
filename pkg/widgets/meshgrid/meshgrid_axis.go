package meshgrid

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Add new struct for axis indicator
type AxisIndicator struct {
	origin    Vertex
	xAxis     Vertex
	yAxis     Vertex
	zAxis     Vertex
	axisScale float64
}

func NewAxisIndicator(scale float64) AxisIndicator {
	return AxisIndicator{
		origin:    Vertex{X: 0, Y: 0, Z: 0},
		xAxis:     Vertex{X: scale, Y: 0, Z: 0},
		yAxis:     Vertex{X: 0, Y: scale, Z: 0},
		zAxis:     Vertex{X: 0, Y: 0, Z: scale},
		axisScale: scale,
	}
}

func (m *Meshgrid) drawAxisIndicator(img *image.RGBA) {
	cornerOffset := 60.0
	indicatorScale := 60.0

	// Create the indicator at corner position
	origin := Vertex{
		X: cornerOffset,
		Y: float64(m.size.Height) - cornerOffset,
	}

	// Instead of using just the rotation matrix, we should use the same
	// camera transformation that's applied to the mesh vertices

	// Use the camera's view matrix (same as in updateVertexPositions)
	viewMatrix := m.cameraRotation

	// Transform axis endpoints using the camera's view matrix
	transformedX := viewMatrix.MultiplyVector([3]float64{indicatorScale, 0, 0})
	transformedY := viewMatrix.MultiplyVector([3]float64{0, -indicatorScale, 0}) // Negative Y scale
	transformedZ := viewMatrix.MultiplyVector([3]float64{0, 0, indicatorScale})

	// Calculate endpoints
	xEnd := Vertex{
		X: origin.X + transformedX[0],
		Y: origin.Y + transformedX[1],
	}
	yEnd := Vertex{
		X: origin.X + transformedY[0],
		Y: origin.Y + transformedY[1],
	}
	zEnd := Vertex{
		X: origin.X + transformedZ[0],
		Y: origin.Y + transformedZ[1],
	}

	// Draw the axes
	ox, oy := int(origin.X), int(origin.Y)

	// X axis (red)
	ex, ey := int(xEnd.X), int(xEnd.Y)
	m.drawLine(img,
		image.Point{ox, oy},
		image.Point{ex, ey},
		0, 0,
		color.RGBA{R: 255, G: 0, B: 0, A: 255},
		color.RGBA{R: 255, G: 0, B: 0, A: 255})

	m.drawText(img, m.xlabel,
		int(ex+5), int(ey),
		color.RGBA{R: 255, G: 0, B: 0, A: 255})

	// Y axis (green)
	ey = int(yEnd.Y)
	ex = int(yEnd.X)
	m.drawLine(img,
		image.Point{ox, oy},
		image.Point{ex, ey},
		0, 0,
		color.RGBA{R: 0, G: 255, B: 0, A: 255},
		color.RGBA{R: 0, G: 255, B: 0, A: 255})

	m.drawText(img, m.ylabel,
		int(ex+5), int(ey),
		color.RGBA{R: 0, G: 255, B: 0, A: 255})

	// Z axis (blue)
	ex = int(zEnd.X)
	ey = int(zEnd.Y)
	m.drawLine(img,
		image.Point{ox, oy},
		image.Point{ex, ey},
		0, 0,
		color.RGBA{R: 0, G: 0, B: 255, A: 255},
		color.RGBA{R: 0, G: 0, B: 255, A: 255})

	m.drawText(img, m.zlabel,
		int(ex+5), int(ey),
		color.RGBA{R: 0, G: 0, B: 255, A: 255})
}

func (m *Meshgrid) drawText(img *image.RGBA, text string, x, y int, col color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)},
	}
	d.DrawString(text)
}
