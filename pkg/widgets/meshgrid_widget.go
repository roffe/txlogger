package widgets

import (
	"fmt"
	"image"
	"image/color"
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

	lastMouseX, lastMouseY float32
	isDragging             bool

	image     *canvas.Image
	container *fyne.Container
	size      fyne.Size

	cellWidth  float32
	cellHeight float32

	cx, cy, cz float64
	ax, ay, az float64
	sx, sy, sz float64
	px, py     float64
}

// NewMeshgrid creates a new Meshgrid given width, height, depth and spacing.
func NewMeshgrid(values []float64, cols, rows int) (*Meshgrid, error) {
	// Check if the provided values slice has the correct number of elements
	if len(values) != cols*rows {
		return nil, fmt.Errorf("the number of Z values does not match the meshgrid dimensions")
	}
	// Find min and max Z values for normalization
	minZ, maxZ, rangeZ := findMinMaxRange(values)

	m := &Meshgrid{
		values: values,

		zmin:   minZ,
		zmax:   maxZ,
		zrange: rangeZ,

		rows: rows,
		cols: cols,

		// Set up the cell size based on the space available and desired spacing
		cellWidth:  32,
		cellHeight: 32,
		depth:      400,

		cx: 0, // Center of the meshgrid X
		cy: 0, // Center of the meshgrid Y
		cz: 0, // Center of the meshgrid Z

		ax: 69, // Rotation angles X
		ay: 15, // Rotation angles Y
		az: 0,  // Rotation angles Z

		sx: .2, // Scale factors X
		sy: .2, // Scale factors Y
		sz: .2, // Scale factors Z

		px: 60, // Camera position X
		py: 50, // Camera position Y

		size: fyne.NewSize(200, 200),
	}

	if cols == 1 {
		m.ax = 90
		m.ay = 90
		m.az = 0
		m.px = 0
		m.py = 40
	}

	m.ExtendBaseWidget(m)

	m.createVertices(400, 400)

	// Generate the initial image and set it to the window content.
	m.image = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 0, 0)))
	m.image.FillMode = canvas.ImageFillOriginal
	m.image.ScaleMode = canvas.ImageScaleFastest
	m.container = container.NewStack(m.image)

	return m, nil
}

func (m *Meshgrid) createVertices(width, height float32) {
	var vertices [][]Vertex
	valueIndex := 0
	for i := m.rows; i > 0; i-- {
		var row []Vertex
		for j := 0; j < m.cols; j++ {
			// Calculate the x and y coordinates based on the current row and column
			x := -float64(width)*.5 + float64(j)*float64(m.cellWidth)
			y := -float64(height)*.5 + float64(i)*float64(m.cellHeight)
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
	//	log.Println("Scaling meshgrid", m.sx, m.sy, m.sz)
	cz := m.depth * .5
	xmax := 0.0
	ymax := 0.0
	for i, row := range m.vertices {
		for j, vertex := range row {
			// Translate point to origin (or cx, cy, cz for scaling about a point)
			x, y, z := vertex.Ox-m.cx, vertex.Oy-m.cy, vertex.Oz-cz
			// Apply scaling & Translate point back

			nx := x*m.sx + m.cx
			ny := y*m.sy + m.cy
			nz := z*m.sz + cz

			m.vertices[i][j].X = nx
			m.vertices[i][j].Y = ny
			m.vertices[i][j].Z = nz
			if nx > xmax {
				xmax = nx
			}
			if ny > ymax {
				ymax = ny
			}
		}
	}
	m.cx = xmax * .5
	m.cy = ymax * .5
	m.cz = m.depth * .5
	// m.cx = m.vertices[len(m.vertices)-1][m.cols-1].X / 2
	// m.cy = m.vertices[len(m.vertices)-1][m.cols-1].Y / 2
}

func (m *Meshgrid) rotateMeshgrid(ax, ay, az float64) {
	// Convert angles to radians
	ax = ax * piDiv180
	ay = ay * piDiv180
	az = az * piDiv180

	// Calculate sine and cosine of the angles
	sinAx, cosAx := math.Sin(ax), math.Cos(ax)
	sinAy, cosAy := math.Sin(ay), math.Cos(ay)
	sinAz, cosAz := math.Sin(az), math.Cos(az)

	cz := m.depth * 0.5 // This assumes your z-values range symmetrically around zero.

	// Iterate over each vertex and apply rotation
	for i := range m.vertices {
		for j := range m.vertices[i] {
			// Translate point to origin for rotation
			x, y, z := m.vertices[i][j].X-m.cx, m.vertices[i][j].Y-m.cy, m.vertices[i][j].Z-cz

			// Rotate around x-axis
			newY, newZ := cosAx*y-sinAx*z, sinAx*y+cosAx*z
			// Rotate around y-axis
			newX, newZ := cosAy*x+sinAy*newZ, -sinAy*x+cosAy*newZ
			// Rotate around z-axis
			newX, newY = cosAz*newX-sinAz*newY, sinAz*newX+cosAz*newY

			// Translate point back from origin after rotation
			m.vertices[i][j].X = newX + m.cx
			m.vertices[i][j].Y = newY + m.cy
			m.vertices[i][j].Z = newZ + cz
		}
	}
}

func (m *Meshgrid) SetFloat64(idx int, value float64) {
	m.values[idx] = value
	m.zmin, m.zmax, m.zrange = findMinMaxRange(m.values)
	m.vertices[idx/m.cols][idx%m.cols].Z = ((value - m.zmin) / m.zrange) * m.depth // Normalize to [0, 1]
	m.createVertices(400, 400)
	m.Refresh()
}

func (m *Meshgrid) SetMin(min float64) {
	m.zmin = min
	m.zrange = m.zmax - m.zmin
}

func (m *Meshgrid) SetMax(max float64) {
	m.zmax = max
	m.zrange = m.zmax - m.zmin
}

func (m *Meshgrid) LoadFloat64s(floats []float64) {
	m.values = floats
	if len(floats) == 0 {
		return
	}
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
	centerX := float64(m.size.Width * .5)
	centerY := float64(m.size.Height * .5)
	screenX := centerX + v.X - m.px
	screenY := centerY + v.Y - m.py
	return int(screenX), int(screenY)
}

func (m *Meshgrid) Refresh() {
	m.scaleMeshgrid()
	m.rotateMeshgrid(m.ax, m.ay, m.az)
	m.image.Image = m.drawMeshgridLines()
	m.image.Refresh()
}

func (m *Meshgrid) Layout(size fyne.Size) {
	m.size = size
	//m.size.Width = size.Width
	m.container.Resize(size)
	m.Refresh()

	//m.image.Resize(size)
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
	return fyne.NewSize(250, 150)
}

func (m *meshgridRenderer) Refresh() {

}

func (m *meshgridRenderer) Destroy() {
}

func (m *meshgridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{m.meshgrid.container}
}

func (m *Meshgrid) drawMeshgridLines() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, int(m.size.Width), int(m.size.Height)))
	for i := m.rows - 1; i >= 0; i-- {
		for j := m.cols - 1; j >= 0; j-- {
			vertice := m.vertices[i][j]
			value := m.values[i*m.cols+j]
			x1, y1 := m.project(vertice)
			lineColor := m.getColorInterpolation(value)

			// Enhance line color based on value (assuming higher values are closer or more significant)
			enhancedLineColor := enhanceLineColor(lineColor, value)

			// Neighboring vertices to connect
			//			neighbors := []struct{ di, dj int }{{1, 0}, {0, 1}, {1, 1}, {1, -1}}
			neighbors := []struct{ di, dj int }{{1, 0}, {0, 1}, {1, -1}}
			for _, n := range neighbors {
				ni, nj := i+n.di, j+n.dj
				if ni < m.rows && nj >= 0 && nj < m.cols {
					neighborVertice := m.vertices[ni][nj]
					neighborValue := m.values[ni*m.cols+nj]
					x2, y2 := m.project(neighborVertice)
					neighborColor := m.getColorInterpolation(neighborValue)
					enhancedNeighborColor := enhanceLineColor(neighborColor, neighborValue)
					// Draw line with interpolated color between current and neighbor
					m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 0, 0, enhancedLineColor, enhancedNeighborColor)
				}
			}
		}
	}
	return img
}

func enhanceLineColor(baseColor color.RGBA, value float64) color.RGBA {
	// Adjust the base color based on value to simulate depth; higher values are "closer"
	// This is a simple approach where we map the value to a brightness factor
	factor := 0.5 + (value * .5) // Assuming value is normalized between 0 and 1
	if factor > 1 {
		factor = 1
	}
	return color.RGBA{
		R: uint8(float64(baseColor.R) * factor),
		G: uint8(float64(baseColor.G) * factor),
		B: uint8(float64(baseColor.B) * factor),
		A: baseColor.A,
	}
}

func (m *Meshgrid) drawLine(img *image.RGBA, p1, p2 image.Point, startThickness, endThickness int, startColor, endColor color.RGBA) {
	dx := float64(p2.X - p1.X)
	dy := float64(p2.Y - p1.Y)
	length := math.Sqrt(dx*dx + dy*dy)

	for i := 0.0; i < length; i++ {
		t := i / length
		x := p1.X + int(t*dx)
		y := p1.Y + int(t*dy)

		// Interpolating thickness
		currentThickness := int(float64(startThickness)*(1-t) + float64(endThickness)*t)

		// Interpolating color
		currentColor := color.RGBA{
			R: uint8(float64(startColor.R)*(1-t) + float64(endColor.R)*t),
			G: uint8(float64(startColor.G)*(1-t) + float64(endColor.G)*t),
			B: uint8(float64(startColor.B)*(1-t) + float64(endColor.B)*t),
			A: uint8(float64(startColor.A)*(1-t) + float64(endColor.A)*t),
		}

		// Draw the current segment of the line
		drawCircle(img, image.Point{X: x, Y: y}, currentThickness, currentColor)
	}
}

// drawCircle draws a circle on img at point p with the given radius and color.
func drawCircle(img *image.RGBA, p image.Point, radius int, c color.RGBA) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(p.X+dx, p.Y+dy, c)
			}
		}
	}
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func (m *Meshgrid) getColorInterpolation(value float64) color.RGBA {
	t := (value - m.zmin) / (m.zmax - m.zmin)
	divider := .5
	var r, g, b float64
	if t < divider { // Green to Yellow interpolation
		r = lerp(0, 1, t/divider)
		g = 1
	} else { // Yellow to Red interpolation
		r = 1
		g = lerp(1, 0, (t-divider)/(1-divider))
	}
	b = 0
	// Convert from 0-1 range to 0-255 for color.RGBA
	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}
