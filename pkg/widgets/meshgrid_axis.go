package widgets

import (
	"fmt"
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
	// Define the position for the axis indicator (bottom-left corner)
	cornerOffset := 80.0   // Increased offset to accommodate labels
	indicatorScale := 80.0 // Slightly larger for better visibility

	// Create and transform axis indicator
	indicator := NewAxisIndicator(indicatorScale)

	// Project the indicator vertices using the current rotation matrix
	origin := Vertex{
		X: cornerOffset,
		Y: float64(m.size.Height) - cornerOffset,
		Z: 0,
	}

	// Transform axis endpoints using the current rotation matrix
	transformedX := m.rotationMatrix.MultiplyVector([3]float64{indicator.axisScale, 0, 0})
	transformedY := m.rotationMatrix.MultiplyVector([3]float64{0, indicator.axisScale, 0})
	transformedZ := m.rotationMatrix.MultiplyVector([3]float64{0, 0, indicator.axisScale})

	// Create end points for each axis
	xEnd := Vertex{
		X: origin.X + transformedX[0],
		Y: origin.Y + transformedX[1],
		Z: origin.Z + transformedX[2],
	}
	yEnd := Vertex{
		X: origin.X + transformedY[0],
		Y: origin.Y + transformedY[1],
		Z: origin.Z + transformedY[2],
	}
	zEnd := Vertex{
		X: origin.X + transformedZ[0],
		Y: origin.Y + transformedZ[1],
		Z: origin.Z + transformedZ[2],
	}

	// Draw the axes with different colors
	ox, oy := int(origin.X), int(origin.Y)

	// X axis (red) with ticks and labels
	ex, ey := int(xEnd.X), int(xEnd.Y)
	m.drawLine(img,
		image.Point{ox, oy},
		image.Point{ex, ey},
		0, 0,
		color.RGBA{R: 255, G: 0, B: 0, A: 255},
		color.RGBA{R: 255, G: 0, B: 0, A: 255})

	// Draw X axis label
	m.drawText(img, "mg/c",
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

	// Draw Y axis label
	m.drawText(img, "rpm",
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

	// Draw Z axis label
	m.drawText(img, "Fuel correction factor",
		int(ex+5), int(ey),
		color.RGBA{R: 0, G: 0, B: 255, A: 255})

	// Draw tick marks and values along each axis
	//m.drawAxisTicks(img, origin, xEnd, yEnd, zEnd)
}

// Add this new method to draw text on the image
func (m *Meshgrid) drawText(img *image.RGBA, text string, x, y int, col color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)},
	}
	d.DrawString(text)
}

// Add this new method to draw tick marks and values
func (m *Meshgrid) drawAxisTicks(img *image.RGBA, origin, xEnd, yEnd, zEnd Vertex) {
	// X axis ticks (mg/c values)
	xValues := []float64{150, 360, 600, 900, 1600}
	for _, val := range xValues {
		// Calculate tick position along X axis
		t := (val - 150) / (900 - 150) // normalize to 0-1
		tickX := origin.X + t*(xEnd.X-origin.X)
		tickY := origin.Y + t*(xEnd.Y-origin.Y)

		// Draw tick mark
		m.drawLine(img,
			image.Point{int(tickX), int(tickY)},
			image.Point{int(tickX), int(tickY - 5)},
			0, 0,
			color.RGBA{R: 255, G: 0, B: 0, A: 255},
			color.RGBA{R: 255, G: 0, B: 0, A: 255})

		// Draw value
		m.drawText(img, fmt.Sprintf("%.0f", val),
			int(tickX-10), int(tickY+15),
			color.RGBA{R: 255, G: 0, B: 0, A: 255})
	}

	// Y axis ticks (rpm)
	yValues := []float64{880, 2400, 3920, 5440}
	for _, val := range yValues {
		// Calculate tick position along Y axis
		t := (val - 880) / (5440 - 880) // normalize to 0-1
		tickX := origin.X + t*(yEnd.X-origin.X)
		tickY := origin.Y + t*(yEnd.Y-origin.Y)

		// Draw tick mark and value
		m.drawLine(img,
			image.Point{int(tickX), int(tickY)},
			image.Point{int(tickX), int(tickY - 5)},
			0, 0,
			color.RGBA{R: 0, G: 255, B: 0, A: 255},
			color.RGBA{R: 0, G: 255, B: 0, A: 255})

		m.drawText(img, fmt.Sprintf("%.0f", val),
			int(tickX-10), int(tickY+15),
			color.RGBA{R: 0, G: 255, B: 0, A: 255})
	}

	// Z axis ticks (correction factor values)
	zValues := []float64{0.9, 1.0, 1.1, 1.2}
	for _, val := range zValues {
		// Calculate tick position along Z axis
		t := (val - 0.9) / (1.2 - 0.9) // normalize to 0-1
		tickX := origin.X + t*(zEnd.X-origin.X)
		tickY := origin.Y + t*(zEnd.Y-origin.Y)

		// Draw tick mark and value
		m.drawLine(img,
			image.Point{int(tickX), int(tickY)},
			image.Point{int(tickX), int(tickY - 5)},
			0, 0,
			color.RGBA{R: 0, G: 0, B: 255, A: 255},
			color.RGBA{R: 0, G: 0, B: 255, A: 255})

		m.drawText(img, fmt.Sprintf("%.1f", val),
			int(tickX-10), int(tickY+15),
			color.RGBA{R: 0, G: 0, B: 255, A: 255})
	}
}
