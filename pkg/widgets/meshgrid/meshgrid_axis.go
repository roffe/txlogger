package meshgrid

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// initAxisObjects creates the axis-indicator overlay (lines and labels) used
// by the shader and polygon backends; the image backend instead draws the
// indicator into its raster (drawAxisIndicator below).
func (m *Meshgrid) initAxisObjects() {
	axisColors := [3]color.RGBA{
		{R: 255, A: 255}, // X red
		{G: 255, A: 255}, // Y green
		{B: 255, A: 255}, // Z blue
	}
	labels := [3]string{m.xlabel, m.ylabel, m.zlabel}
	for i := range m.axisLines {
		m.axisLines[i] = &canvas.Line{StrokeColor: axisColors[i], StrokeWidth: 1}
		t := canvas.NewText(labels[i], axisColors[i])
		t.TextSize = 11
		m.axisLabels[i] = t
	}
}

// updateAxisObjects mirrors drawAxisIndicator with canvas primitives.
func (m *Meshgrid) updateAxisObjects() {
	const indicatorScale = 60.0
	origin := fyne.NewPos(60, m.size.Height-m.size.Height/4)

	r := m.cameraRotation
	ends := [3][3]float64{
		r.MultiplyVector([3]float64{indicatorScale, 0, 0}),
		r.MultiplyVector([3]float64{0, -indicatorScale, 0}),
		r.MultiplyVector([3]float64{0, 0, indicatorScale}),
	}
	for i, e := range ends {
		end := fyne.NewPos(origin.X+float32(e[0]), origin.Y+float32(e[1]))
		m.axisLines[i].Position1 = origin
		m.axisLines[i].Position2 = end
		// Text positions by its top-left; the image path draws at the
		// baseline, so nudge up to roughly match.
		m.axisLabels[i].Move(fyne.NewPos(end.X+5, end.Y-8))
	}
}

func (m *Meshgrid) drawAxisIndicator(img *image.RGBA) {
	cornerOffset := 60.0
	indicatorScale := 60.0

	// Create the indicator at corner position
	origin := Vertex{
		X: cornerOffset,
		Y: float64(m.size.Height) - float64(m.size.Height/4),
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
	drawBresenhamLine(img, ox, oy, ex, ey, color.RGBA{R: 255, G: 0, B: 0, A: 255}, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	m.drawText(img, m.xlabel,
		int(ex+5), int(ey),
		color.RGBA{R: 255, G: 0, B: 0, A: 255})

	// Y axis (green)
	ey = int(yEnd.Y)
	ex = int(yEnd.X)
	drawBresenhamLine(img, ox, oy, ex, ey, color.RGBA{R: 0, G: 255, B: 0, A: 255}, color.RGBA{R: 0, G: 255, B: 0, A: 255})

	m.drawText(img, m.ylabel,
		int(ex+5), int(ey),
		color.RGBA{R: 0, G: 255, B: 0, A: 255})

	// Z axis (blue)
	ex = int(zEnd.X)
	ey = int(zEnd.Y)
	drawBresenhamLine(img, ox, oy, ex, ey, color.RGBA{R: 0, G: 0, B: 255, A: 255}, color.RGBA{R: 0, G: 0, B: 255, A: 255})
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
