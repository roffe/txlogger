package meshgrid

import (
	"fmt"
	"image"
	"image/color"
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

	// Cached mesh center of the original (untransformed) coordinates.
	centerX, centerY, centerZ float64

	// Scratch buffers reused between frames to avoid per-frame allocations.
	scratchImg    *image.RGBA
	scratchProjX  []int
	scratchProjY  []int
	scratchColors []color.RGBA
	scratchLines  []lineSegment
	scratchQuads  []quadRef

	renderMode RenderMode

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

	dragging bool

	OnMouseDown func()
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
		// T7Suite-style starting view: ~30° elevation (pitch 60° from
		// top-down) with the mesh spun 35° around its vertical axis. Starting
		// from identity, the RotZ term is a model-space turntable spin (the
		// same composition orbit() applies), not a camera roll.
		m.cameraRotation = RotationMatrixX(60).Multiply(m.cameraRotation).Multiply(RotationMatrixZ(-35))
		m.updateVertexPositions()
	}

	m.ExtendBaseWidget(m)

	// Generate the initial image and set it to the window content.
	m.image = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 0, 0)))
	m.image.FillMode = canvas.ImageFillOriginal
	m.image.ScaleMode = canvas.ImageScaleFastest

	return m, nil
}

// SetRenderMode switches between solid surface, solid+wireframe and pure
// wireframe rendering.
func (m *Meshgrid) SetRenderMode(mode RenderMode) {
	if m.renderMode != mode {
		m.renderMode = mode
		m.refresh()
	}
}

func (m *Meshgrid) RenderMode() RenderMode {
	return m.renderMode
}

// CycleRenderMode steps to the next render mode (surface → solid → wireframe).
func (m *Meshgrid) CycleRenderMode() {
	m.renderMode = (m.renderMode + 1) % renderModeCount
	m.refresh()
}

func (m *Meshgrid) SetColorBlindMode(mode colors.ColorBlindMode) {
	if m.colorMode != mode {
		m.colorMode = mode
	}
	m.refresh()
}

func (m *Meshgrid) createVertices(width, height float32) {
	// Guard against a zero range (e.g. all values identical / all zero) so we
	// produce a flat mesh at Z=0 instead of NaN from a div-by-zero.
	zrange := m.zrange
	if zrange == 0 {
		zrange = 1
	}

	vertices := make([][]Vertex, 0, m.rows)
	valueIndex := 0
	var sumX, sumY, sumZ float64
	var count int
	for i := m.rows; i > 0; i-- {
		row := make([]Vertex, 0, m.cols)
		for j := 0; j < m.cols; j++ {
			x := -float64(width)*.5 + float64(j)*float64(m.cellWidth)
			y := -float64(height)*.5 + float64(i)*float64(m.cellHeight)
			z := ((m.values[valueIndex] - m.zmin) / zrange) * m.depth
			row = append(row, Vertex{
				Ox: x,
				Oy: y,
				Oz: z,
				X:  x,
				Y:  y,
				Z:  z,
			})
			sumX += x
			sumY += y
			sumZ += z
			count++
			valueIndex++
		}
		vertices = append(vertices, row)
	}
	m.vertices = vertices

	if count > 0 {
		inv := 1.0 / float64(count)
		m.centerX = sumX * inv
		m.centerY = sumY * inv
		m.centerZ = sumZ * inv
	}
}

func (m *Meshgrid) scaleMeshgrid(factor float64) {
	m.scale = m.scale * factor
	m.updateVertexPositions()
}

// orbit performs a Fusion 360-style "turntable" orbit. Spin is applied around
// the mesh's own vertical axis — data Z, the height axis (right-multiplied so
// it rotates the model before the camera) — pitch around the camera-local X
// axis (left-multiplied). Composing the two this way prevents roll from
// sneaking in on diagonal drags.
func (m *Meshgrid) orbit(spinDelta, pitchDelta float64) {
	pitchRot := RotationMatrixX(pitchDelta)
	spinRot := RotationMatrixZ(spinDelta)
	m.cameraRotation = pitchRot.Multiply(m.cameraRotation).Multiply(spinRot)
	m.updateVertexPositions()
}

// rotateMeshgrid is the free 3DOF rotation, kept for the initial view setup
// (which wants a roll component) and for the RMB roll handler.
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
	// Original coordinates never change after createVertices, so reuse the
	// pre-computed mesh center instead of recomputing it every frame.
	cx, cy, cz := m.centerX, m.centerY, m.centerZ
	scale := m.scale
	r := m.cameraRotation
	camX, camY, camZ := m.cameraPosition[0], m.cameraPosition[1], m.cameraPosition[2]

	for i := range m.vertices {
		row := m.vertices[i]
		for j := range row {
			v := &row[j]
			vx := (v.Ox - cx) * scale
			vy := (v.Oy - cy) * scale
			vz := (v.Oz - cz) * scale

			v.X = r[0][0]*vx + r[0][1]*vy + r[0][2]*vz - camX
			v.Y = r[1][0]*vx + r[1][1]*vy + r[1][2]*vz - camY
			v.Z = r[2][0]*vx + r[2][1]*vy + r[2][2]*vz - camZ
		}
	}
}

// SetFloat64 updates a single cell value. The whole mesh is rebuilt since a
// new value can shift zmin/zmax and with it every vertex's normalized height.
func (m *Meshgrid) SetFloat64(idx int, value float64) {
	if idx < 0 || idx >= len(m.values) {
		return
	}
	m.values[idx] = value
	m.zmin, m.zmax, m.zrange = findMinMaxRange(m.values)
	m.createVertices(fyne.Max(float32(m.cols), 1), fyne.Max(float32(m.rows), 1))
	m.updateVertexPositions()
	m.refresh()
}

// Update LoadFloat64s to use the new vertex position update method
func (m *Meshgrid) LoadFloat64s(min, max float64, floats []float64) {
	if len(floats) != m.rows*m.cols {
		log.Printf("meshgrid: LoadFloat64s got %d values, want %d (%dx%d)", len(floats), m.rows*m.cols, m.cols, m.rows)
		return
	}
	m.zmin = min
	m.zmax = max
	m.zrange = m.zmax - m.zmin

	m.values = floats

	m.createVertices(fyne.Max(float32(m.cols), 1), fyne.Max(float32(m.rows), 1))
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
		// AfterFunc fires on a timer goroutine; hop back to the fyne thread.
		// refreshPending is then only ever touched on the fyne thread.
		fyne.Do(func() {
			m.refresh()
			m.refreshPending = false
		})
	})
}

func (m *Meshgrid) CreateRenderer() fyne.WidgetRenderer {
	return &meshgridRenderer{MG: m}
}

type meshgridRenderer struct {
	MG      *Meshgrid
	objects []fyne.CanvasObject
}

func (m *meshgridRenderer) Layout(size fyne.Size) {
	if size == m.MG.size {
		return
	}
	m.MG.size = size
	m.MG.image.Resize(size)
	m.MG.throttledRefresh()
}

func (m *meshgridRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 100)
}

func (m *meshgridRenderer) Refresh() {
	m.MG.refresh()
}

func (m *meshgridRenderer) Destroy() {
}

func (m *meshgridRenderer) Objects() []fyne.CanvasObject {
	if m.objects == nil {
		m.objects = []fyne.CanvasObject{m.MG.image}
	}
	return m.objects
}
