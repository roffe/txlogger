package meshgrid

import (
	"fmt"
	"image"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

type Vertex struct {
	Ox, Oy, Oz float64 // Original coordinates
	X, Y, Z    float64 // Transformed coordinates for rendering
}

var _ fyne.Widget = (*Meshgrid)(nil)

type Meshgrid struct {
	widget.BaseWidget

	values []float64

	rows, cols         int
	zmin, zmax, zrange float64
	depth              float64

	vertices [][]Vertex

	lastMouseX, lastMouseY float32

	image *canvas.Image
	size  fyne.Size

	cellWidth  float32
	cellHeight float32

	rotationMatrix Matrix3x3
	scale          float64

	cameraRotation Matrix3x3  // Camera's rotation matrix
	cameraPosition [3]float64 // Camera's position in world space
	mousePosition  image.Point

	xlabel, ylabel, zlabel string

	refreshPending bool

	colorMode colors.ColorBlindMode
}

// NewMeshgrid creates a new Meshgrid given width, height, depth and spacing.
func NewMeshgrid(xlabel, ylabel, zlabel string, values []float64, cols, rows int, colorBlindMode colors.ColorBlindMode) (*Meshgrid, error) {
	// Check if the provided values slice has the correct number of elements
	if len(values) != max(1, cols)*max(1, rows) {
		return nil, fmt.Errorf("the number of Z values does not match the meshgrid dimensions")
	}
	// Find min and max Z values for normalization
	minZ, maxZ, rangeZ := findMinMaxRange(values)

	m := &Meshgrid{
		values: values,
		zmin:   minZ,
		zmax:   maxZ,
		zrange: rangeZ,
		rows:   rows,
		cols:   cols,
		// Set up the cell size based on the space available and desired spacing
		cellWidth:  32,
		cellHeight: 32,
		depth:      400,
		size:       fyne.NewSize(200, 200),
		scale:      1,

		rotationMatrix: NewMatrix3x3(),
		cameraRotation: NewMatrix3x3(),
		cameraPosition: [3]float64{0, 0, 0},

		xlabel: xlabel,
		ylabel: ylabel,
		zlabel: zlabel,

		colorMode: colorBlindMode,
	}

	m.createVertices(fyne.Max(float32(m.cols), 1), fyne.Max(float32(m.rows), 1))

	m.scaleMeshgrid(0.3)

	if cols == 1 {
		m.rotateMeshgrid(0, 90, 0)
	} else {
		m.rotateMeshgrid(60, 0, -30)
	}

	m.ExtendBaseWidget(m)

	// Generate the initial image and set it to the window content.
	m.image = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 0, 0)))
	m.image.FillMode = canvas.ImageFillOriginal
	m.image.ScaleMode = canvas.ImageScaleFastest

	return m, nil
}

func (m *Meshgrid) SetColorBlindMode(mode colors.ColorBlindMode) {
	if m.colorMode != mode {
		m.colorMode = mode
	}
	m.refresh()
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

func (m *Meshgrid) scaleMeshgrid(factor float64) {
	m.scale = m.scale * factor
	m.updateVertexPositions()
}

// Replace rotateMeshgrid with this camera-centric approach
func (m *Meshgrid) rotateMeshgrid(pitchDelta, yawDelta, rollDelta float64) {
	// Create rotation matrices for each axis
	rotX := RotationMatrixX(pitchDelta) // Pitch (around X axis)
	rotY := RotationMatrixY(yawDelta)   // Yaw (around Y axis)
	rotZ := RotationMatrixZ(rollDelta)  // Roll (around Z axis)

	// Combine the new rotations
	deltaRotation := rotX.Multiply(rotY).Multiply(rotZ)

	// Update the camera rotation
	// For camera-relative rotations, we multiply the delta rotation first
	m.cameraRotation = deltaRotation.Multiply(m.cameraRotation)

	// Update all vertex positions based on the new camera
	m.updateVertexPositions()
}

func (m *Meshgrid) updateVertexPositions() {
	// Calculate mesh center from original coordinates
	var sumX, sumY, sumZ float64
	var count int
	for i := range m.vertices {
		for j := range m.vertices[i] {
			sumX += m.vertices[i][j].Ox
			sumY += m.vertices[i][j].Oy
			sumZ += m.vertices[i][j].Oz
			count++
		}
	}
	centerX := sumX / float64(count)
	centerY := sumY / float64(count)
	centerZ := sumZ / float64(count)

	viewMatrix := m.cameraRotation

	for i := range m.vertices {
		for j := range m.vertices[i] {
			// Scale and translate so mesh is centered at origin
			vx := (m.vertices[i][j].Ox - centerX) * m.scale
			vy := (m.vertices[i][j].Oy - centerY) * m.scale
			vz := (m.vertices[i][j].Oz - centerZ) * m.scale

			// Rotate
			viewVec := viewMatrix.MultiplyVector([3]float64{vx, vy, vz})

			// Apply camera offset
			viewVec[0] -= m.cameraPosition[0]
			viewVec[1] -= m.cameraPosition[1]
			viewVec[2] -= m.cameraPosition[2]

			// Store in mesh-local coordinates (already centered)
			m.vertices[i][j].X = viewVec[0]
			m.vertices[i][j].Y = viewVec[1]
			m.vertices[i][j].Z = viewVec[2]
		}
	}
}

func (m *Meshgrid) SetFloat64(idx int, value float64) {
	log.Println("SetFloat64", idx, value)
	m.values[idx] = value
	m.zmin, m.zmax, m.zrange = findMinMaxRange(m.values)
	m.vertices[idx/m.cols][idx%m.cols].Z = ((value - m.zmin) / m.zrange) * m.depth // Normalize to [0, 1]
	m.refresh()                                                                    // Refresh without recreating all vertices
}

func (m *Meshgrid) SetFloat642(idx int, value float64) {
	m.values[idx] = value
	m.zmin, m.zmax, m.zrange = findMinMaxRange(m.values)
	m.vertices[idx/m.cols][idx%m.cols].Z = ((value - m.zmin) / m.zrange) * m.depth // Normalize to [0, 1]
	m.createVertices(fyne.Min(float32(m.cols), 1), fyne.Min(float32(m.rows), 1))
	m.refresh()
}

// Update LoadFloat64s to use the new vertex position update method
func (m *Meshgrid) LoadFloat64s(min, max float64, floats []float64) {
	m.zmin = min
	m.zmax = max
	m.zrange = m.zmax - m.zmin

	m.values = floats
	if len(floats) == 0 {
		return
	}

	// Reset vertices with new values
	m.createVertices(fyne.Min(float32(m.cols), 1), fyne.Min(float32(m.rows), 1))

	// Update vertex positions based on current camera state
	m.updateVertexPositions()

	m.refresh()
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
	centerX := float64(m.size.Width) * 0.5
	centerY := float64(m.size.Height) * 0.5
	screenX := centerX + v.X
	screenY := centerY + v.Y
	return int(screenX), int(screenY)
}

func (m *Meshgrid) Refresh() {
	m.refresh()
}

func (m *Meshgrid) refresh() {
	m.image.Image = m.drawMeshgridLines()
	m.image.Resize(m.size)
	m.image.Refresh()
}

func (m *Meshgrid) throttledRefresh() {
	if m.refreshPending {
		return
	}
	m.refreshPending = true
	time.AfterFunc(10*time.Millisecond, func() { // ~100fps
		m.refresh()
		m.refreshPending = false
	})
}
func (m *Meshgrid) CreateRenderer() fyne.WidgetRenderer {
	return &meshgridRenderer{m}
}

type meshgridRenderer struct {
	*Meshgrid
}

func (m *meshgridRenderer) Layout(size fyne.Size) {
	if size == m.size {
		return
	}
	m.size = size
	m.throttledRefresh()
}

func (m *meshgridRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 100)
}

func (m *meshgridRenderer) Refresh() {
	m.Meshgrid.refresh()
}

func (m *meshgridRenderer) Destroy() {
}

func (m *meshgridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{m.image}
}
