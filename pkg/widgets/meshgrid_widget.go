package widgets

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Vertex struct {
	Ox, Oy, Oz float64 // Original coordinates
	X, Y, Z    float64 // Transformed coordinates for rendering
}

type Meshgrid struct {
	widget.BaseWidget

	values []float64

	rows, cols         int
	zmin, zmax, zrange float64
	depth              float64

	vertices [][]Vertex

	center Vertex
	//cameraPosition Vertex

	lastMouseX, lastMouseY float32
	isDragging             bool

	image     *canvas.Image
	container *fyne.Container
	size      fyne.Size

	cellWidth  float32
	cellHeight float32

	sx, sy, sz float64
	ax, ay, az float64
	px, py     float64
}

// NewMeshgrid creates a new Meshgrid given width, height, depth and spacing.
func NewMeshgrid(values []float64, cols, rows int) *Meshgrid {
	// Check if the provided values slice has the correct number of elements
	if len(values) != cols*rows {
		panic("The number of Z values does not match the meshgrid dimensions.")
	}
	// Find min and max Z values for normalization
	minZ, maxZ, rangeZ := findMinMaxRange(values)

	m := &Meshgrid{
		values: values,

		zmin:   minZ,
		zmax:   maxZ,
		zrange: rangeZ,

		center: Vertex{X: 0, Y: 0, Z: 0},

		rows: rows,
		cols: cols,

		// Set up the cell size based on the space available and desired spacing
		cellWidth:  30,
		cellHeight: 30,
		depth:      20 * 20,
		ax:         69,
		ay:         15,
		az:         0,
		sx:         .4,
		sy:         .4,
		sz:         .4,
	}
	m.ExtendBaseWidget(m)

	m.createVertices(400, 400)

	// Generate the initial image and set it to the window content.
	m.image = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 0, 0)))
	m.image.FillMode = canvas.ImageFillOriginal
	m.image.ScaleMode = canvas.ImageScaleFastest
	m.container = container.NewStack(m.image)

	return m
}

func (m *Meshgrid) createVertices(width, height float32) {
	var vertices [][]Vertex
	valueIndex := 0
	for i := m.rows; i > 0; i-- {
		var row []Vertex
		for j := 0; j < m.cols; j++ {
			// Calculate the x and y coordinates based on the current row and column
			x := -float64(width)/2 + float64(j)*float64(m.cellWidth)
			y := -float64(height)/2 + float64(i)*float64(m.cellHeight)
			z := ((m.values[valueIndex] - m.zmin) / m.zrange) * m.depth // Normalize to [0, 1]
			//zValues = append(zValues, m.values[valueIndex])
			row = append(row, Vertex{
				Ox: x,
				Oy: y,
				Oz: z,
				X:  x,
				Y:  y,
				Z:  z,
			})
			valueIndex++
		}
		vertices = append(vertices, row)
	}
	m.vertices = vertices
}

func (m *Meshgrid) RotateMeshgrid(ax, ay, az float64) {
	m.ax = ax
	m.ay = ay
	m.az = az
	m.Refresh()
}

func (m *Meshgrid) scaleMeshgrid() {
	log.Println("Scaling meshgrid", m.sx, m.sy, m.sz)
	cx := 0.0
	cy := 0.0
	cz := m.depth / 2
	for i, row := range m.vertices {
		for j, vertex := range row {
			// Translate point to origin (or cx, cy, cz for scaling about a point)
			x, y, z := vertex.Ox-cx, vertex.Oy-cy, vertex.Oz-cz
			// Apply scaling & Translate point back
			m.vertices[i][j].X = x*m.sx + cx
			m.vertices[i][j].Y = y*m.sy + cy
			m.vertices[i][j].Z = z*m.sz + cz
		}
	}
}

func (m *Meshgrid) rotateMeshgrid(ax, ay, az float64) {
	log.Println("Rotating meshgrid", m.ax, m.ay, m.az)
	ax = ax * math.Pi / 180
	ay = ay * math.Pi / 180
	az = az * math.Pi / 180
	sinAx, cosAx := math.Sin(ax), math.Cos(ax)
	sinAy, cosAy := math.Sin(ay), math.Cos(ay)
	sinAz, cosAz := math.Sin(az), math.Cos(az)

	cx := 0.0
	cy := 0.0
	cz := m.depth / 2 // This assumes your z-values range symmetrically around zero.

	for i, row := range m.vertices {
		for j, vertex := range row {
			// Translate point to origin for rotation
			x, y, z := vertex.X-cx, vertex.Y-cy, vertex.Z-cz
			// Rotate around x-axis
			newY, newZ := cosAx*y-sinAx*z, sinAx*y+cosAx*z
			// Rotate around y-axis
			newX, newZ := cosAy*x+sinAy*newZ, -sinAy*x+cosAy*newZ
			// Rotate around z-axis
			newX, newY = cosAz*newX-sinAz*newY, sinAz*newX+cosAz*newY
			// Translate point back from origin after rotation
			m.vertices[i][j].X = newX + cx
			m.vertices[i][j].Y = newY + cy
			m.vertices[i][j].Z = newZ + cz
		}
	}
}

func (m *Meshgrid) LoadFloat64s(floats []float64) {
	m.values = floats
	for i, f := range floats {
		normalizedZ := (f - m.zmin) / m.zrange // Normalize to [0, 1]
		m.vertices[i/m.cols][i%m.cols].Z = normalizedZ * m.depth
	}
	m.createVertices(400, 400)
	m.Refresh()
}

// returns the min, max and range across the data
func findMinMaxRange(values []float64) (float64, float64, float64) {
	minZ, maxZ := values[0], values[0]
	for _, v := range values {
		if v < minZ {
			minZ = v
		}
		if v > maxZ {
			maxZ = v
		}
	}
	return minZ, maxZ, maxZ - minZ
}

func (m *Meshgrid) project(v Vertex) (int, int) {
	// Translate the vertex position by the center of the screen
	// and adjust by the camera position to get screen coordinates.
	centerX := float64(m.size.Width / 2)
	centerY := float64(m.size.Height / 2)
	screenX := centerX + v.X - m.px
	screenY := centerY + v.Y - m.py
	return int(screenX), int(screenY)
}

func (m *Meshgrid) Refresh() {
	m.scaleMeshgrid()
	m.rotateMeshgrid(m.ax, m.ay, m.az)
	m.image.Image = m.generateImage()
	m.image.Refresh()
}

func (m *Meshgrid) Layout(size fyne.Size) {
	m.size = size
	m.container.Resize(size)
	m.Refresh()
	m.image.Resize(size)
}

func (m *Meshgrid) CreateRenderer() fyne.WidgetRenderer {
	return &meshgridRenderer{meshgrid: m}
}

type meshgridRenderer struct {
	meshgrid *Meshgrid
}

func (m *meshgridRenderer) Layout(size fyne.Size) {
	m.meshgrid.Layout(size)
}

func (m *meshgridRenderer) MinSize() fyne.Size {
	return fyne.NewSize(300, 200)
}

func (m *meshgridRenderer) Refresh() {

}

func (m *meshgridRenderer) Destroy() {
}

func (m *meshgridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{m.meshgrid.container}
}

func (m *Meshgrid) generateImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, int(m.size.Width), int(m.size.Height)))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Transparent}, image.Point{}, draw.Src)
	m.drawMeshgridLines(img)
	return img
}

func (m *Meshgrid) drawMeshgridLines(img *image.RGBA) {
	for i := m.rows - 1; i >= 0; i-- {
		for j := m.cols - 1; j >= 0; j-- {
			vertex := m.vertices[i][j]
			x1, y1 := m.project(vertex)
			lineColor := m.getColorInterpolation(m.values[i*m.cols+j])

			// Draw line to the right
			if j < m.cols-1 {
				nextVertexRight := m.vertices[i][j+1]
				x2, y2 := m.project(nextVertexRight)
				//lineColor := m.getLineColor(i, j)
				endColor := m.getColorInterpolation(m.values[i*m.cols+j+1])
				if y1 > y2 {
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 2, endColor, lineColor)
				} else {
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 2, lineColor, endColor)
				}
			}

			// Draw line downward
			if i < m.rows-1 {
				nextVertexDown := m.vertices[i+1][j]
				x2, y2 := m.project(nextVertexDown)
				endColor := m.getColorInterpolation(m.values[(i+1)*m.cols+j])
				if y1 > y2 {
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 2, endColor, lineColor)
				} else {
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 2, lineColor, endColor)
				}
			}
			// // Draw diagonal line down-right
			// if i < m.rows-1 && j < m.cols-1 {
			// 	tempVertex := m.vertices[i+1][j+1]
			// 	x2, y2 := m.project(tempVertex)
			// 	endColor := m.getColorInterpolation(m.values[(i+1)*m.cols+j+1])
			// 	// Dim the color for diagonal lines
			// 	dimmedLineColor := color.RGBA{
			// 		R: uint8(float64(lineColor.R) * 0.30),
			// 		G: uint8(float64(lineColor.G) * 0.30),
			// 		B: uint8(float64(lineColor.B) * 0.30),
			// 		A: 255, // Use a fully opaque alpha
			// 	}
			// 	if y1 > y2 {
			// 		m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, endColor, dimmedLineColor)
			// 	} else {
			// 		m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, dimmedLineColor, endColor)
			// 	}
			// }

			// Draw diagonal line down-left
			if j > 0 && i < m.rows-1 {
				tempVertex := m.vertices[i+1][j-1]
				x2, y2 := m.project(tempVertex)
				endColor := m.getColorInterpolation(m.values[(i+1)*m.cols+j-1])
				// Dim the color for diagonal lines
				dimmedLineColor := color.RGBA{
					R: uint8(float64(lineColor.R) * 0.20),
					G: uint8(float64(lineColor.G) * 0.20),
					B: uint8(float64(lineColor.B) * 0.20),
					A: 255, // Use a fully opaque alpha
				}
				if y1 > y2 {
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, endColor, dimmedLineColor)
				} else {
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, dimmedLineColor, endColor)
				}
			}

		}
	}
}

func (m *Meshgrid) drawLine(img *image.RGBA, p1, p2 image.Point, width int, startColor, endColor color.RGBA) {
	if width <= 0 {
		width = 1
	}

	steep := abs(p2.Y-p1.Y) > abs(p2.X-p1.X)

	// Swap points if necessary to ensure we always iterate from p1 to p2
	if steep {
		p1.X, p1.Y = p1.Y, p1.X
		p2.X, p2.Y = p2.Y, p2.X
	}
	if p1.X > p2.X {
		p1.X, p2.X = p2.X, p1.X
		p1.Y, p2.Y = p2.Y, p1.Y
	}

	dx := p2.X - p1.X
	dy := abs(p2.Y - p1.Y)
	err := dx / 2
	var ystep int
	if p1.Y < p2.Y {
		ystep = 1
	} else {
		ystep = -1
	}

	lineLength := max(dx, dy)

	interpolateColor := func(step, total int) color.RGBA {
		fraction := float64(step) / float64(total)
		return color.RGBA{
			R: uint8(float64(endColor.R)*fraction + float64(startColor.R)*(1-fraction)),
			G: uint8(float64(endColor.G)*fraction + float64(startColor.G)*(1-fraction)),
			B: uint8(float64(endColor.B)*fraction + float64(startColor.B)*(1-fraction)),
			A: uint8(float64(endColor.A)*fraction + float64(startColor.A)*(1-fraction)),
		}
	}

	var lineStep int
	for x := p1.X; x <= p2.X; x++ {
		interpColor := interpolateColor(lineStep, lineLength)

		if steep {
			for w := -width / 2; w <= width/2; w++ {
				img.Set(p1.Y+w, x, interpColor)
			}
		} else {
			for w := -width / 2; w <= width/2; w++ {
				img.Set(x, p1.Y+w, interpColor)
			}
		}

		err -= dy
		if err < 0 {
			p1.Y += ystep
			err += dx
		}

		if steep {
			lineStep += abs(ystep)
		} else {
			lineStep++
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func (m *Meshgrid) getColorInterpolation(value float64) color.RGBA {
	t := (value - m.zmin) / (m.zmax - m.zmin)
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	yellow := color.RGBA{R: 255, G: 255, B: 0, A: 255}
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	var result color.RGBA
	if t < 0.5 {
		factor := 2 * t
		result.R = uint8(lerp(float64(green.R), float64(yellow.R), factor))
		result.G = uint8(lerp(float64(green.G), float64(yellow.G), factor))
		result.B = uint8(lerp(float64(green.B), float64(yellow.B), factor))
		result.A = 255
	} else {
		factor := 2 * (t - 0.5)
		result.R = uint8(lerp(float64(yellow.R), float64(red.R), factor))
		result.G = uint8(lerp(float64(yellow.G), float64(red.G), factor))
		result.B = uint8(lerp(float64(yellow.B), float64(red.B), factor))
		result.A = 255
	}

	return result
}
