package widgets

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type Vertex struct {
	*canvas.Line
	X, Y, Z float64
}

type Meshgrid struct {
	widget.BaseWidget

	values []float64

	rows, cols         int
	zmin, zmax, zrange float64
	depth              float64

	vertices [][]Vertex
	zValues  [][]float64

	center         Vertex
	cameraPosition Vertex

	lastMouseX, lastMouseY float32
	isDragging             bool

	image     *canvas.Image
	container *fyne.Container
	size      fyne.Size

	cellWidth  float32
	cellHeight float32

	ax, ay, az float64
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

		cameraPosition: Vertex{
			X: 0,
			Y: 0,
		},

		rows: rows,
		cols: cols,

		//size:    fyne.NewSize(float32(width), float32(height)),
		//depth:   depth,
		//spacing: spacing,
	}
	m.ExtendBaseWidget(m)

	// Set up the cell size based on the space available and desired spacing
	m.cellWidth = float32(30)
	m.cellHeight = float32(30)

	// Set depth based on spacing, too, for consistency
	m.depth = float64(40) * float64(rows) // You might want to scale this according to your needs

	// Initialize camera position to center the mesh on screen
	m.cameraPosition = Vertex{
		X: -60,
		Y: -10,
	}
	m.ax = 45
	m.ay = 3
	m.az = 0

	m.createVertices(500, 400)
	m.rotateMeshgrid(m.ax, m.ay, m.az)

	// Generate the initial image and set it to the window content.
	m.image = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 0, 0)))
	m.image.FillMode = canvas.ImageFillOriginal
	m.image.ScaleMode = canvas.ImageScaleFastest
	m.container = container.NewBorder(
		nil,
		container.NewGridWithColumns(4,
			widget.NewButton("<", func() {
				m.rotateMeshgrid(0, -1, 0)
				m.Refresh()
			}),
			widget.NewButton("^", func() {
				m.rotateMeshgrid(1, 0, 0)
				m.Refresh()
			}),
			widget.NewButton("v", func() {
				m.rotateMeshgrid(-1, 0, 0)
				m.Refresh()
			}),
			widget.NewButton(">", func() {
				m.rotateMeshgrid(0, 1, 0)
				m.Refresh()
			}),
		),
		nil,
		nil,
		m.image,
	)

	//m.rotateMeshgrid(29, 35.264, 0)

	return m
}

func (m *Meshgrid) RotateMeshgrid(ax, ay, az float64) {
	m.rotateMeshgrid(ax, ay, az)
	m.Refresh()
}

func (m *Meshgrid) rotateMeshgrid(ax, ay, az float64) {
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
			// Apply rotations around the origin
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

func (m *Meshgrid) scaleMeshgrid(sx, sy, sz float64) {
	// Assuming cx, cy, cz are the centers of the grid as in your rotation function
	// and represent the point around which you want to scale the meshgrid.

	// You might want to scale around the center or some other point,
	// in which case you would calculate or set cx, cy, cz to that point.
	cx := 0.0
	cy := 0.0
	cz := m.depth / 2 // As with the rotation, assuming z-values are symmetrical around zero.

	for i, row := range m.vertices {
		for j, vertex := range row {
			// Translate point to origin (or cx, cy, cz for scaling about a point)
			x, y, z := vertex.X-cx, vertex.Y-cy, vertex.Z-cz

			// Apply scaling
			newX, newY, newZ := x*sx, y*sy, z*sz

			// Translate point back after scaling
			m.vertices[i][j].X = newX + cx
			m.vertices[i][j].Y = newY + cy
			m.vertices[i][j].Z = newZ + cz
		}
	}
}

func (m *Meshgrid) createVertices(width, height float32) {
	var vertices [][]Vertex
	var newZvalues [][]float64
	valueIndex := 0
	for i := m.rows; i > 0; i-- {
		var row []Vertex
		var zValues []float64
		for j := 0; j < m.cols; j++ {
			// Calculate the x and y coordinates based on the current row and column
			x := -float64(width)/2 + float64(j)*float64(m.cellWidth)
			y := -float64(height)/2 + float64(i)*float64(m.cellHeight)
			normalizedZ := (m.values[valueIndex] - m.zmin) / m.zrange // Normalize to [0, 1]
			zValues = append(zValues, m.values[valueIndex])
			row = append(row, Vertex{X: x, Y: y, Z: normalizedZ * m.depth})
			valueIndex++
		}
		vertices = append(vertices, row)
		newZvalues = append(newZvalues, zValues) // Add the row of scaled Z values to the slice of slices.
	}
	m.vertices = vertices
	m.zValues = newZvalues
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

// Define a projection function that maps 3D vertices to 2D coordinates.
func (m *Meshgrid) project2(v Vertex) (int, int) {
	return int(v.X) + int(m.size.Width/2+float32(m.cameraPosition.X)), int(v.Y) + int(m.size.Height/2+float32(m.cameraPosition.Y))
}

func (m *Meshgrid) project(v Vertex) (int, int) {
	// Translate the vertex position by the center of the screen
	// and adjust by the camera position to get screen coordinates.
	centerX := m.size.Width / 2
	centerY := m.size.Height / 2
	screenX := centerX + float32(v.X) - float32(m.cameraPosition.X)
	screenY := centerY + float32(v.Y) - float32(m.cameraPosition.Y)
	return int(screenX), int(screenY)
}

func (m *Meshgrid) Refresh() {
	m.image.Image = m.generateImage()
	m.image.Refresh()
}

func (m *Meshgrid) Layout(size fyne.Size) {
	m.container.Resize(size)
	m.size = size
	//m.cellWidth = size.Width / float32(m.cols) * .8
	//m.cellHeight = size.Height / float32(m.rows) * .8
	//m.depth = float64(size.Height) * .8
	//m.createVertices(size.Width, size.Height)
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
	return fyne.NewSize(m.meshgrid.cellHeight*float32(m.meshgrid.cols), m.meshgrid.cellHeight*float32(m.meshgrid.rows))
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

			// Draw line to the right
			if j < m.cols-1 {
				nextVertexRight := m.vertices[i][j+1]
				x2, y2 := m.project(nextVertexRight)
				lineColor := m.getLineColor(i, j)
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 2, lineColor)
			}

			// Draw line downward
			if i < m.rows-1 {
				nextVertexDown := m.vertices[i+1][j]
				x2, y2 := m.project(nextVertexDown)
				lineColor := m.getLineColor(i, j)
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 3, lineColor)
			}

			// Draw diagonal line down-right
			if i < m.rows-1 && j < m.cols-1 {
				nextVertexDiagRight := m.vertices[i+1][j+1]
				nextVertexDiagRight.Z = nextVertexDiagRight.Z / 2
				x2, y2 := m.project(nextVertexDiagRight)
				lineColor := m.getLineColor(i, j)
				// Dim the color for diagonal lines
				lineColor.R = uint8(float64(lineColor.R) * 0.30)
				lineColor.G = uint8(float64(lineColor.G) * 0.30)
				lineColor.B = uint8(float64(lineColor.B) * 0.30)
				lineColor.A = 5 // Adjust the alpha if necessary
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, lineColor)
			}

			// Draw diagonal line down-left
			if j > 0 && i < m.rows-1 {
				nextVertexDiagLeft := m.vertices[i+1][j-1]
				nextVertexDiagLeft.Z = nextVertexDiagLeft.Z / 2
				x2, y2 := m.project(nextVertexDiagLeft)
				lineColor := m.getLineColor(i, j)
				// Dim the color for diagonal lines
				lineColor.R = uint8(float64(lineColor.R) * 0.20)
				lineColor.G = uint8(float64(lineColor.G) * 0.20)
				lineColor.B = uint8(float64(lineColor.B) * 0.20)
				lineColor.A = 5 // Adjust the alpha if necessary
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, lineColor)
			}

		}
	}
}

func (m *Meshgrid) drawMeshgridLines2(img *image.RGBA) {
	for i := len(m.vertices) - 1; i >= 0; i-- {
		row := m.vertices[i]
		for j := len(row) - 1; j >= 0; j-- {
			vertex := row[j]
			x1, y1 := m.project(vertex)
			if j < len(row)-1 {
				nextVertexRight := row[j+1]
				x2, y2 := m.project(nextVertexRight)
				lineColor := m.getLineColor(i, j)
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 3, lineColor)
			}
			if i < len(m.vertices)-1 && len(m.vertices[i+1]) > j {
				nextVertexDown := m.vertices[i+1][j]
				x2, y2 := m.project(nextVertexDown)
				lineColor := m.getLineColor(i, j)
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 3, lineColor)
			}
		}
	}
}

func (m *Meshgrid) drawMeshgridLines3(img *image.RGBA) {
	for i := len(m.vertices) - 1; i >= 0; i-- {
		row := m.vertices[i]
		//for i, row := range m.Vertices {
		for j := len(row) - 1; j >= 0; j-- {
			vertex := row[j]
			//for j, vertex := range row {
			x1, y1 := m.project(vertex)
			if j < len(row)-1 {
				nextVertexRight := row[j+1]
				x2, y2 := m.project(nextVertexRight)
				lineColor := m.getLineColor(i, j)
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 3, lineColor)
			}
			if i < len(m.vertices)-1 {
				nextVertexDown := m.vertices[i+1][j]
				x2, y2 := m.project(nextVertexDown)
				lineColor := m.getLineColor(i, j)
				m.drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 3, lineColor)
			}

			// mitten
			// if i < len(m.Vertices)-1 && j < len(row)-1 {
			// 	nextRow := m.Vertices[i+1]
			// 	nextVertexRight := nextRow[j+1]
			// 	x2, y2 := m.project(nextVertexRight)
			// 	lineColor := m.GetLineColor(i, j)
			// 	lineColor.R = uint8(float64(lineColor.R) * 0.25)
			// 	lineColor.G = uint8(float64(lineColor.G) * 0.25)
			// 	lineColor.B = uint8(float64(lineColor.B) * 0.25)
			// 	lineColor.A = 5
			// 	drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, lineColor)
			// }

			// if j > 0 && i < len(m.Vertices)-1 {
			// 	nextRow := m.Vertices[i+1]
			// 	nextVertexRight := nextRow[j-1]
			// 	x2, y2 := m.project(nextVertexRight)
			// 	lineColor := m.GetLineColor(i, j)
			// 	lineColor.R = uint8(float64(lineColor.R) * 0.15)
			// 	lineColor.G = uint8(float64(lineColor.G) * 0.15)
			// 	lineColor.B = uint8(float64(lineColor.B) * 0.15)
			// 	lineColor.A = 5
			// 	drawLine(img, image.Point{x1, y1}, image.Point{x2, y2}, 1, lineColor)
			// }
		}
	}
}

func (m *Meshgrid) drawLine(img *image.RGBA, p1, p2 image.Point, width int, clr color.RGBA) {
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

	// Determine the length of the line for interpolation purposes
	lineLength := max(dx, dy)

	// Function to interpolate color based on the current step and total line length
	interpolateColor := func(step, total int) color.RGBA {
		fraction := float64(step) / float64(total)
		scale := 1 - fraction*(1-0.40)
		return color.RGBA{
			R: uint8(float64(clr.R) * scale),
			G: uint8(float64(clr.G) * scale),
			B: uint8(float64(clr.B) * scale),
			A: clr.A,
		}
	}

	var lineStep int
	for x := p1.X; x <= p2.X; x++ {
		// Get the interpolated color based on the position
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

		// Increase the line step based on which direction is being interpolated
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

func (m *Meshgrid) getLineColor(i, j int) color.RGBA {
	// originalZ := m.zValues[i][j]
	// if originalZ < m.zmin {
	// return color.RGBA{R: 0, G: 255, B: 0, A: 255}
	// }
	// return m.getColorInterpolation(originalZ)
	return m.getColorInterpolation(m.zValues[i][j])
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

const (
	rotationSensitivity = 1 // Adjust as necessary
)

func (m *Meshgrid) MouseIn(_ *desktop.MouseEvent) {
}

func (m *Meshgrid) MouseOut() {
}

// MouseMoved is called when the mouse is moved over the map viewer.
func (m *Meshgrid) MouseMoved(event *desktop.MouseEvent) {
	dx := float64(event.Position.X - m.lastMouseX) // Change in X position
	dy := float64(event.Position.Y - m.lastMouseY) // Change in Y position

	if m.isDragging {
		if event.Button&desktop.MouseButtonPrimary == desktop.MouseButtonPrimary {
			ax := -dy * rotationSensitivity // Rotate around X-axis
			ay := dx * rotationSensitivity  // Rotate around Y-axis
			m.ax += ax
			m.ay += ay
			m.rotateMeshgrid(float64(ax), float64(ay), 0) // Assuming no rotation around Z-axis for simplicity

		}
		if event.Button&desktop.MouseButtonSecondary == desktop.MouseButtonSecondary {
			ax := -dy * rotationSensitivity // Rotate around X-axis
			az := dx * rotationSensitivity  // Rotate around Z-axis
			m.ax += ax
			m.az += az
			m.rotateMeshgrid(float64(ax), 0, float64(az))
		}
		if event.Button&desktop.MouseButtonTertiary == desktop.MouseButtonTertiary {
			m.cameraPosition.X += -float64(dx) // Move along X-axis
			m.cameraPosition.Y += -float64(dy) // Move along Y-axis
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
	var scaleFactor float64
	if event.Scrolled.DY > 0 {
		scaleFactor = 1.05
	} else {
		scaleFactor = 0.95
	}

	m.scaleMeshgrid(scaleFactor, scaleFactor, scaleFactor)

	// Refresh the meshgrid to redraw at the new scale and rotation
	m.Refresh()

	// Resize the image to fit the new size of the meshgrid
	m.image.Resize(m.size)
}
