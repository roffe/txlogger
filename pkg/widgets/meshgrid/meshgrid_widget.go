package meshgrid

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
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

	// New camera state variables

}

// NewMeshgrid creates a new Meshgrid given width, height, depth and spacing.
func NewMeshgrid(xlabel, ylabel, zlabel string, values []float64, cols, rows int) (*Meshgrid, error) {
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

// New method to compute vertex positions based on camera state
func (m *Meshgrid) updateVertexPositions() {
	// Calculate the true center of the mesh using original coordinates
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

	// Compute the view matrix (inverse of camera transform)
	// For simplicity, we're just using the transpose of the rotation
	// since we're assuming orthographic projection
	viewMatrix := m.cameraRotation

	// Apply transformations to all vertices
	for i := range m.vertices {
		for j := range m.vertices[i] {
			// Scale the original coordinates
			vx := m.vertices[i][j].Ox * m.scale
			vy := m.vertices[i][j].Oy * m.scale
			vz := m.vertices[i][j].Oz * m.scale

			// Translate to mesh center
			vx -= centerX * m.scale
			vy -= centerY * m.scale
			vz -= centerZ * m.scale

			// Apply view matrix (camera rotation)
			viewVec := viewMatrix.MultiplyVector([3]float64{vx, vy, vz})

			// Apply camera position offset
			viewVec[0] -= m.cameraPosition[0]
			viewVec[1] -= m.cameraPosition[1]
			viewVec[2] -= m.cameraPosition[2]

			// Store final transformed coordinates
			m.vertices[i][j].X = viewVec[0] + centerX*m.scale
			m.vertices[i][j].Y = viewVec[1] + centerY*m.scale
			m.vertices[i][j].Z = viewVec[2] + centerZ*m.scale
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
	// Project vertex to screen coordinates with fixed camera at center
	centerX := float64(m.size.Width * .5)
	centerY := float64(m.size.Height * .5)
	screenX := centerX + v.X
	screenY := centerY + v.Y
	return int(screenX), int(screenY)
}

func (m *Meshgrid) refresh() {
	m.image.Image = m.drawMeshgridLines()
	m.image.Resize(m.size)
	m.image.Refresh()

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
	m.refresh()
}

func (m *meshgridRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 100)
}

func (m *meshgridRenderer) Refresh() {
}

func (m *meshgridRenderer) Destroy() {
}

func (m *meshgridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{m.image}
}

func (m *Meshgrid) drawMeshgridLines() *image.RGBA {
	//	log.Println("drawMeshgridLines")
	img := image.NewRGBA(image.Rect(0, 0, int(m.size.Width), int(m.size.Height)))

	// Find the min and max Z values after projection for depth scaling
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			if m.vertices[i][j].Z < minZ {
				minZ = m.vertices[i][j].Z
			}
			if m.vertices[i][j].Z > maxZ {
				maxZ = m.vertices[i][j].Z
			}
		}
	}
	zRange := maxZ - minZ

	for i := m.rows - 1; i >= 0; i-- {
		for j := m.cols - 1; j >= 0; j-- {
			vertex := m.vertices[i][j]
			value := m.values[i*m.cols+j]
			x1, y1 := m.project(vertex)

			// Calculate depth factor based on projected Z position
			depthFactor := (vertex.Z - minZ) / zRange
			baseColor := m.getColorInterpolation(value)
			enhancedColor := m.enhanceLineColor(baseColor, depthFactor)

			neighbors := []struct{ di, dj int }{{1, 0}, {0, 1}, {1, -1}}
			for _, n := range neighbors {
				ni, nj := i+n.di, j+n.dj
				if ni < m.rows && nj >= 0 && nj < m.cols {
					neighborVertex := m.vertices[ni][nj]
					neighborValue := m.values[ni*m.cols+nj]
					x2, y2 := m.project(neighborVertex)

					// Calculate neighbor depth factor
					neighborDepthFactor := (neighborVertex.Z - minZ) / zRange
					neighborBaseColor := m.getColorInterpolation(neighborValue)
					neighborEnhancedColor := m.enhanceLineColor(neighborBaseColor, neighborDepthFactor)
					/*
						m.drawLine(img,
							image.Point{x1, y1},
							image.Point{x2, y2},
							0, 0,
							enhancedColor,
							neighborEnhancedColor)
					*/

					// For diagonal lines, darken the colors and reduce thickness
					if n.di == 1 && n.dj == -1 {
						// Darken both colors by reducing their values
						enhancedColor = color.RGBA{
							R: uint8(float64(enhancedColor.R) * 0.7),
							G: uint8(float64(enhancedColor.G) * 0.7),
							B: uint8(float64(enhancedColor.B) * 0.7),
							A: enhancedColor.A,
						}
						neighborEnhancedColor = color.RGBA{
							R: uint8(float64(neighborEnhancedColor.R) * 0.7),
							G: uint8(float64(neighborEnhancedColor.G) * 0.7),
							B: uint8(float64(neighborEnhancedColor.B) * 0.7),
							A: neighborEnhancedColor.A - 90,
						}
						// Draw diagonal lines with -1 thickness (thinner than regular lines)
						m.drawLine(img,
							image.Point{x1, y1},
							image.Point{x2, y2},
							-1, 0, // Reduced thickness for diagonals
							enhancedColor,
							neighborEnhancedColor)
					} else {
						// Regular lines remain unchanged
						m.drawLine(img,
							image.Point{x1, y1},
							image.Point{x2, y2},
							0, 0,
							enhancedColor,
							neighborEnhancedColor)
					}

				}
			}
		}
	}

	/*
		// Draw horizontal and vertical lines
		crosshairSize := 10
		crosshairColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}

		// Get and display the value at cursor position
		if value, found := m.findValueAtPosition(m.mousePosition); found {
			valueText := fmt.Sprintf("%.3f", value)
			textColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}

			// Draw background box for better readability
			boxWidth := (len(valueText) * 7) + 3 // Approximate width based on font size
			boxHeight := 15
			for y := -3; y <= boxHeight; y++ {
				for x := 3; x <= boxWidth; x++ {
					if m.mousePosition.X+x >= 0 && m.mousePosition.X+x < int(m.size.Width) &&
						m.mousePosition.Y-boxHeight+y >= 0 && m.mousePosition.Y-boxHeight+y < int(m.size.Height) {
						img.Set(m.mousePosition.X+x, m.mousePosition.Y-boxHeight+y, color.RGBA{0, 0, 0, 192})
					}
				}
			}
			// Draw the value text
			m.drawText(img, valueText,
				m.mousePosition.X+3, m.mousePosition.Y-2-boxHeight+12,
				textColor)
		}

		// Horizontal line
		m.drawLine(img,
			image.Point{m.mousePosition.X - crosshairSize, m.mousePosition.Y},
			image.Point{m.mousePosition.X + crosshairSize, m.mousePosition.Y},
			0, 0, crosshairColor, crosshairColor)

		// Vertical line
		m.drawLine(img,
			image.Point{m.mousePosition.X, m.mousePosition.Y - crosshairSize},
			image.Point{m.mousePosition.X, m.mousePosition.Y + crosshairSize},
			0, 0, crosshairColor, crosshairColor)
	*/

	// Draw the axis indicator after the mesh
	m.drawAxisIndicator(img)

	return img
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

	var r, g, b float64

	// Three-part interpolation: green -> yellow -> red
	if t < 0.4 { // Green to Yellow transition (0-40%)
		r = lerp(0, 1, t/0.4) // Red increases
		g = 1.0               // Green stays max
		b = 0                 // Blue stays zero
	} else if t < 0.6 { // Hold Yellow (40-60%)
		r = 1.0
		g = 1.0
		b = 0
	} else { // Yellow to Red transition (60-100%)
		r = 1.0                     // Red stays max
		g = lerp(1, 0, (t-0.6)/0.4) // Green decreases
		b = 0                       // Blue stays zero
	}

	// Enhance saturation for yellow region
	if t > 0.35 && t < 0.65 {
		// Boost both red and green components slightly in yellow region
		boost := 1.2 // Increase brightness of yellow
		r = math.Min(1.0, r*boost)
		g = math.Min(1.0, g*boost)
	}

	// Convert from 0-1 range to 0-255 for color.RGBA
	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}

// enhanceLineColor modified to preserve yellow better
func (m *Meshgrid) enhanceLineColor(baseColor color.RGBA, depthFactor float64) color.RGBA {
	// Adjust brightness based on depth while preserving color ratios
	adjustedDepthFactor := math.Pow(depthFactor, 1.5)

	// Scale factor now has a higher minimum to prevent colors from getting too dark
	scaleFactor := 0.6 + (adjustedDepthFactor * 0.4)

	// Reduced atmospheric perspective effect to maintain color clarity
	coolTint := uint8((1 - adjustedDepthFactor) * 15)

	// Calculate the color components while preserving ratios
	r := uint8(float64(baseColor.R) * scaleFactor)
	g := uint8(float64(baseColor.G) * scaleFactor)
	b := uint8(math.Min(255, float64(baseColor.B)*scaleFactor+float64(coolTint)))

	// Special handling for yellow regions to preserve distinctness
	if baseColor.R > 200 && baseColor.G > 200 && baseColor.B < 50 {
		// Boost yellow while maintaining ratio
		boost := 1.1
		r = uint8(math.Min(255, float64(r)*boost))
		g = uint8(math.Min(255, float64(g)*boost))
	}

	return color.RGBA{
		R: r,
		G: g,
		B: b,
		A: baseColor.A,
	}
}
