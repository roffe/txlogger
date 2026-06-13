package meshgrid

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

// Vertex is a corner of the mesh. Values are cell-centered (one per table
// cell) while vertices sit on cell corners, so the vertex grid is one larger
// than the data grid in each direction and V holds the average of the
// adjacent cell values.
type Vertex struct {
	Ox, Oy, Oz float64 // Original coordinates
	X, Y, Z    float64 // Transformed coordinates for rendering
	V          float64 // Data value at this corner (average of adjacent cells)
}

var _ fyne.Widget = (*Meshgrid)(nil)

// renderBackend selects how the mesh is drawn. The GPU shader is the
// default; TXLOGGER_MESH_RENDERER=poly|image selects the older paths.
type RenderBackend int

const (
	// backendShader ray-casts the whole mesh in one fragment shader
	// (meshgrid_shader.go); per-frame CPU cost is a uniform update.
	BackendShader RenderBackend = iota
	// backendPolygons draws one canvas.ArbitraryPolygon per cell
	// (meshgrid_poly.go).
	BackendPolygons
	// backendImage rasterizes on the CPU into a canvas.Image
	// (meshgrid_draw.go).
	BackendImage
)

func backendFromEnv() RenderBackend {
	switch os.Getenv("TXLOGGER_MESH_RENDERER") {
	case "poly":
		return BackendPolygons
	case "image":
		return BackendImage
	default:
		return BackendShader
	}
}

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

	backend RenderBackend

	// Shader backend (meshgrid_shader.go): the whole mesh in one object.
	shader *canvas.Shader

	// Polygon backend (meshgrid_poly.go): one reusable
	// canvas.ArbitraryPolygon per cell instead of rasterizing into an image.
	polys       []*canvas.ArbitraryPolygon
	polyObjects []fyne.CanvasObject
	scratchFX   []float32
	scratchFY   []float32

	// Axis-indicator overlay shared by the shader and polygon backends (the
	// image backend draws its own into the raster).
	axisLines  [3]*canvas.Line
	axisLabels [3]*canvas.Text

	renderMode RenderMode

	lastMouseX, lastMouseY float32

	image *canvas.Image
	size  fyne.Size

	cellWidth  float32
	cellHeight float32

	rotationMatrix Matrix3x3
	scale          float64

	// fitted is set once the widget has received its first real size and the
	// mesh has been auto-scaled to fit. Subsequent resizes scale relative to
	// the size change so the user's manual zoom is preserved.
	fitted bool

	cameraRotation Matrix3x3  // Camera's rotation matrix
	cameraPosition [3]float64 // Camera's position in world space
	mousePosition  image.Point

	// Live tracking marker (fractional cell indices), mirroring the
	// mapviewer crosshair. Hidden until SetCursor is first called. The marker
	// is a canvas primitive overlaid on the mesh image so moving it only
	// repaints the scene instead of re-rasterizing the whole mesh.
	cursorX, cursorY float64
	showCursor       bool
	cursor           *canvas.Circle

	xlabel, ylabel, zlabel string

	refreshPending bool

	colorMode colors.ColorBlindMode

	dragging bool

	OnMouseDown func()
}

// Marker colors match the mapviewer crosshair so the two tracking
// indicators read as the same thing.
var (
	cursorFillColor = color.RGBA{R: 165, G: 55, B: 253, A: 255}
	cursorRingColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

const cursorRadius = 6

// NewMeshgrid creates a new Meshgrid given width, height, depth and spacing.
func NewMeshgrid(xlabel, ylabel, zlabel string, values []float64, cols, rows int, colorBlindMode colors.ColorBlindMode, backend RenderBackend) (*Meshgrid, error) {
	cols = max(1, cols)
	rows = max(1, rows)
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

		backend: backend,
	}

	m.createVertices()

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

	m.cursor = &canvas.Circle{
		// Position2 sets the bounds directly; Resize would trigger a canvas
		// refresh before the widget is even shown.
		Position2:   fyne.NewPos(cursorRadius*2, cursorRadius*2),
		FillColor:   cursorFillColor,
		StrokeColor: cursorRingColor,
		StrokeWidth: 2,
		Hidden:      true,
	}

	m.initAxisObjects()
	switch m.backend {
	case BackendShader:
		m.initShader()
	case BackendPolygons:
		m.initPolygons()
	}

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
		m.updateShaderColormap()
	}
	m.refresh()
}

// createVertices builds the corner-vertex grid: one quad per table cell, so
// the mesh shows exactly cols x rows cells like the map above. The grid is
// (rows+1) x (cols+1); each corner takes the average of the 1-4 cell values
// touching it.
func (m *Meshgrid) createVertices() {
	// Guard against a zero range (e.g. all values identical / all zero) so we
	// produce a flat mesh at Z=0 instead of NaN from a div-by-zero.
	zrange := m.zrange
	if zrange == 0 {
		zrange = 1
	}

	vRows, vCols := m.rows+1, m.cols+1
	vertices := make([][]Vertex, 0, vRows)
	var sumX, sumY, sumZ float64
	var count int
	for i := 0; i < vRows; i++ {
		row := make([]Vertex, 0, vCols)
		for j := 0; j < vCols; j++ {
			value := m.cornerValue(i, j)
			x := float64(j) * float64(m.cellWidth)
			// Data row 0 is the bottom row in the map; keep it at the high-Y
			// end of the mesh like before, so the orientation is unchanged.
			y := float64(vRows-i) * float64(m.cellHeight)
			z := ((value - m.zmin) / zrange) * m.depth
			row = append(row, Vertex{
				Ox: x,
				Oy: y,
				Oz: z,
				X:  x,
				Y:  y,
				Z:  z,
				V:  value,
			})
			sumX += x
			sumY += y
			sumZ += z
			count++
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

// cornerValue averages the cell values adjacent to corner (vi, vj): four in
// the interior, two along edges and one at the outer corners.
func (m *Meshgrid) cornerValue(vi, vj int) float64 {
	r0 := max(vi-1, 0)
	r1 := min(vi, m.rows-1)
	c0 := max(vj-1, 0)
	c1 := min(vj, m.cols-1)

	var sum float64
	var n int
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			sum += m.values[r*m.cols+c]
			n++
		}
	}
	return sum / float64(n)
}

func (m *Meshgrid) scaleMeshgrid(factor float64) {
	m.scale = m.scale * factor
	m.updateVertexPositions()
}

// fitMargin leaves a border around the mesh so it isn't drawn flush to the
// widget edges (and so the axis indicator, which extends past the mesh, has
// some room).
const fitMargin = 0.85

// projectedBounds returns the min/max of the mesh's current projected
// (orthographic) screen positions. Both reflect the current scale, rotation
// and pan.
func (m *Meshgrid) projectedBounds() (minX, maxX, minY, maxY float64) {
	minX, maxX = math.Inf(1), math.Inf(-1)
	minY, maxY = math.Inf(1), math.Inf(-1)
	for i := range m.vertices {
		row := m.vertices[i]
		for j := range row {
			v := &row[j]
			if v.X < minX {
				minX = v.X
			}
			if v.X > maxX {
				maxX = v.X
			}
			if v.Y < minY {
				minY = v.Y
			}
			if v.Y > maxY {
				maxY = v.Y
			}
		}
	}
	return
}

// projectedExtent returns the width and height, in logical pixels, of the
// mesh's current projected bounding box. It reflects the current scale and
// rotation; panning shifts every vertex equally so the extent is
// pan-invariant.
func (m *Meshgrid) projectedExtent() (w, h float64) {
	minX, maxX, minY, maxY := m.projectedBounds()
	if math.IsInf(minX, 0) || math.IsInf(minY, 0) {
		return 0, 0
	}
	return maxX - minX, maxY - minY
}

// centerInView pans the camera so the mesh's projected bounding box is
// centered in the widget. Used on the initial fit only; the camera offset it
// produces scales with the mesh on later resizes, so the centering (and any
// user pan applied on top) is preserved.
func (m *Meshgrid) centerInView() {
	minX, maxX, minY, maxY := m.projectedBounds()
	if math.IsInf(minX, 0) || math.IsInf(minY, 0) {
		return
	}
	// v.{X,Y} = base - cam, so adding the current box center to the camera
	// moves the box center to the view origin (which maps to screen center).
	m.cameraPosition[0] += (minX + maxX) / 2
	m.cameraPosition[1] += (minY + maxY) / 2
	m.updateVertexPositions()
}

// fitScaleForSize returns the m.scale value that makes the mesh's projected
// bounding box fill the given widget size (minus fitMargin) at the current
// rotation. The bounding box is linear in m.scale, so it is normalized to a
// unit scale first.
func (m *Meshgrid) fitScaleForSize(size fyne.Size) float64 {
	w, h := m.projectedExtent()
	if w <= 0 || h <= 0 || m.scale == 0 {
		return m.scale
	}
	wPer := w / m.scale
	hPer := h / m.scale
	sx := float64(size.Width) * fitMargin / wPer
	sy := float64(size.Height) * fitMargin / hPer
	return math.Min(sx, sy)
}

// adaptZoom keeps the mesh sized to the widget across layout changes. The
// first real layout fits the mesh to the view and centers it; later resizes
// scale the mesh (and the camera pan) by the ratio of the new fit-scale to
// the old. This grows and shrinks the mesh with the window while preserving
// whatever zoom and pan the user has applied — it adjusts the existing scale
// and pan multiplicatively rather than resetting them.
func (m *Meshgrid) adaptZoom(oldSize, newSize fyne.Size) {
	if newSize.Width <= 0 || newSize.Height <= 0 {
		return
	}
	if !m.fitted {
		m.scale = m.fitScaleForSize(newSize)
		m.fitted = true
		m.updateVertexPositions()
		m.centerInView()
		return
	}
	if oldSize.Width <= 0 || oldSize.Height <= 0 {
		return
	}
	oldFit := m.fitScaleForSize(oldSize)
	if oldFit == 0 {
		return
	}
	ratio := m.fitScaleForSize(newSize) / oldFit
	if ratio <= 0 || math.IsInf(ratio, 0) || math.IsNaN(ratio) {
		return
	}
	m.scale *= ratio
	// Pan lives in the same scaled view space, so scale it alongside the mesh
	// to keep the centering and any user pan in the same relative spot.
	m.cameraPosition[0] *= ratio
	m.cameraPosition[1] *= ratio
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
	m.createVertices()
	m.updateVertexPositions()
	m.updateShaderData()
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

	m.createVertices()
	m.updateVertexPositions()
	m.updateShaderData()
	m.refresh()
}

// SetCursor positions the tracking marker at the (fractional) cell index,
// mirroring the crosshair in the map above. The marker rides on the mesh
// surface, interpolated between the four surrounding vertices.
func (m *Meshgrid) SetCursor(xIdx, yIdx float64) {
	if xIdx < 0 {
		xIdx = 0
	} else if max := float64(m.cols - 1); xIdx > max {
		xIdx = max
	}
	if yIdx < 0 {
		yIdx = 0
	} else if max := float64(m.rows - 1); yIdx > max {
		yIdx = max
	}
	if m.showCursor && xIdx == m.cursorX && yIdx == m.cursorY {
		return
	}
	m.cursorX = xIdx
	m.cursorY = yIdx
	m.showCursor = true
	m.moveCursor()
}

// moveCursor repositions the overlay marker on the projected surface point.
// Moving a canvas primitive only repaints the scene from cached textures,
// so cursor updates don't re-rasterize the mesh.
func (m *Meshgrid) moveCursor() {
	if !m.showCursor {
		return
	}
	px, py := m.cursorScreenPosition()
	m.cursor.Move(fyne.NewPos(px-cursorRadius, py-cursorRadius))
	if m.cursor.Hidden {
		m.cursor.Show()
	}
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
	switch m.backend {
	case BackendShader:
		// All per-frame state lives in shader uniforms; the GPU re-renders
		// from them on the next paint. Only the overlays move on the CPU.
		m.updateShaderUniforms()
		m.updateAxisObjects()
		m.moveCursor()
		canvas.Refresh(m)
	case BackendPolygons:
		// Geometry/color updates on the reusable polygons; the canvas.Refresh
		// marks the scene dirty so color-only changes repaint too.
		m.updatePolygons()
		m.moveCursor()
		canvas.Refresh(m)
	default:
		m.image.Image = m.drawMeshgridLines()
		m.image.Resize(m.size)
		m.image.Refresh()
		// Rotation, scale, resize and data changes all move the projected
		// surface point under the marker.
		m.moveCursor()
	}
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
	if m.backend != BackendImage {
		// Text measuring needs a driver, so the labels can't be sized in the
		// constructor (tests build widgets without an app).
		for _, t := range m.axisLabels {
			t.Resize(t.MinSize())
		}
	}
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
	oldSize := m.MG.size
	m.MG.size = size
	// Auto-fit on the first real size and scale with the window thereafter,
	// preserving any zoom the user has applied.
	m.MG.adaptZoom(oldSize, size)
	switch m.MG.backend {
	case BackendShader:
		m.MG.shader.Resize(size)
	case BackendImage:
		m.MG.image.Resize(size)
	}
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
	switch m.MG.backend {
	case BackendShader:
		if m.objects == nil {
			m.MG.updateAxisObjects()
			objs := []fyne.CanvasObject{m.MG.shader}
			for i := range m.MG.axisLines {
				objs = append(objs, m.MG.axisLines[i], m.MG.axisLabels[i])
			}
			m.objects = append(objs, m.MG.cursor)
		}
		return m.objects
	case BackendPolygons:
		// updatePolygons rebuilds the list in painter's order every frame;
		// populate it here for the first paint.
		if len(m.MG.polyObjects) == 0 {
			m.MG.updatePolygons()
		}
		return m.MG.polyObjects
	default:
		if m.objects == nil {
			m.objects = []fyne.CanvasObject{m.MG.image, m.MG.cursor}
		}
		return m.objects
	}
}
