package meshgrid

import (
	"image"
	"image/color"
	"math"

	"github.com/roffe/txlogger/pkg/widgets"
)

// VertexPair represents a line segment between two vertices
type VertexPair struct {
	v1, v2     Vertex
	val1, val2 float64
	x1, y1     int
	x2, y2     int
	depth      float64
}

// drawMeshgridLines creates a new image with the meshgrid drawn
func (m *Meshgrid) drawMeshgridLines() *image.RGBA {
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
	if zRange == 0 {
		zRange = 1 // Prevent division by zero
	}

	// Sort vertices by Z value for proper depth
	// Instead of using a map-based z-buffer, we'll use a simpler approach:
	// 1. Collect all line segments
	// 2. Sort them by depth (back to front)
	// 3. Draw them in order

	// Capacity preallocation helps performance
	lineSegments := make([]VertexPair, 0, m.rows*m.cols*3)

	// Collect line segments with all necessary data pre-computed
	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			vertex := m.vertices[i][j]
			value := m.values[i*m.cols+j]
			x1, y1 := m.project(vertex)

			neighbors := []struct{ di, dj int }{{1, 0}, {0, 1}, {1, -1}}
			for _, n := range neighbors {
				ni, nj := i+n.di, j+n.dj
				if ni < m.rows && nj >= 0 && nj < m.cols {
					neighborVertex := m.vertices[ni][nj]
					neighborValue := m.values[ni*m.cols+nj]
					x2, y2 := m.project(neighborVertex)

					// Skip if line is too small to be visible
					dx, dy := x2-x1, y2-y1
					if dx*dx+dy*dy < 4 {
						continue
					}

					// Calculate average Z for sorting - farther objects have larger Z values
					depth := -(vertex.Z + neighborVertex.Z) / 2.0

					lineSegments = append(lineSegments, VertexPair{
						v1:    vertex,
						v2:    neighborVertex,
						val1:  value,
						val2:  neighborValue,
						x1:    x1,
						y1:    y1,
						x2:    x2,
						y2:    y2,
						depth: depth,
					})
				}
			}
		}
	}

	// Sort line segments by depth (back to front)
	// Using quick sort to improve performance
	quickSortLineSegments(lineSegments, 0, len(lineSegments)-1)

	// Draw lines from back to front
	for _, segment := range lineSegments {
		// Calculate depth factors
		depth1 := (segment.v1.Z - minZ) / zRange
		depth2 := (segment.v2.Z - minZ) / zRange

		// Calculate colors based on values and depth
		color1 := m.getColorWithDepth(segment.val1, depth1)
		color2 := m.getColorWithDepth(segment.val2, depth2)

		// Determine if this is a diagonal line
		isDiagonal := segment.x1 != segment.x2 && segment.y1 != segment.y2

		// Adjust colors and thickness for diagonals
		if isDiagonal {
			color1 = fadeColor(color1, 0.7)
			color2 = fadeColor(color2, 0.7)
		}

		// Draw the line using Bresenham's algorithm for speed
		drawBresenhamLine(img,
			segment.x1, segment.y1,
			segment.x2, segment.y2,
			color1, color2)
	}

	// Draw the axis indicator on top
	m.drawAxisIndicator(img)

	return img
}

// getColorWithDepth combines color interpolation and depth enhancement in one step
func (m *Meshgrid) getColorWithDepth(value, depthFactor float64) color.RGBA {
	// Get base color from value
	//baseColor := m.getColorInterpolation(value)
	baseColor := widgets.GetColorInterpolation(
		m.zmin,
		m.zmax,
		value,
		m.colorMode,
	)

	// Apply depth effects - simpler calculation for performance
	adjustedFactor := 0.6 + 0.4*depthFactor // Simplify power calculations

	// Add slight atmospheric haze (blue tint to distant objects)
	hazeFactor := (1 - depthFactor) * 15

	r := uint8(float64(baseColor.R) * adjustedFactor)
	g := uint8(float64(baseColor.G) * adjustedFactor)
	b := uint8(min(255, int(float64(baseColor.B)*adjustedFactor+hazeFactor)))

	// Special handling for yellow (simpler check)
	if baseColor.R > 200 && baseColor.G > 200 && baseColor.B < 50 {
		r = uint8(min(255, int(float64(r)*1.1)))
		g = uint8(min(255, int(float64(g)*1.1)))
	}

	return color.RGBA{
		R: r,
		G: g,
		B: b,
		A: baseColor.A,
	}
}

// Fade a color by a factor (used for diagonals)
func fadeColor(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * factor),
		G: uint8(float64(c.G) * factor),
		B: uint8(float64(c.B) * factor),
		A: uint8(float64(c.A) * factor),
	}
}

// Quick sort implementation for line segments (faster than bubble sort)
func quickSortLineSegments(segments []VertexPair, low, high int) {
	if low < high {
		pivotIndex := partitionLineSegments(segments, low, high)
		quickSortLineSegments(segments, low, pivotIndex-1)
		quickSortLineSegments(segments, pivotIndex+1, high)
	}
}

func partitionLineSegments(segments []VertexPair, low, high int) int {
	pivot := segments[high].depth
	i := low - 1

	for j := low; j < high; j++ {
		if segments[j].depth <= pivot {
			i++
			segments[i], segments[j] = segments[j], segments[i]
		}
	}

	segments[i+1], segments[high] = segments[high], segments[i+1]
	return i + 1
}

// Fast Bresenham line algorithm for drawing lines
func drawBresenhamLine(img *image.RGBA, x0, y0, x1, y1 int, color1, color2 color.RGBA) {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Check if line is completely out of bounds
	if (x0 < 0 && x1 < 0) || (x0 >= width && x1 >= width) ||
		(y0 < 0 && y1 < 0) || (y0 >= height && y1 >= height) {
		return
	}

	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy

	// Calculate total steps for color interpolation
	totalSteps := max(dx, -dy)
	step := 0

	for {
		// Only draw if point is within bounds
		if x0 >= 0 && x0 < width && y0 >= 0 && y0 < height {
			// Interpolate color based on position along the line
			t := 0.0
			if totalSteps > 0 {
				t = float64(step) / float64(totalSteps)
			}

			currentColor := interpolateColor(color1, color2, t)
			img.SetRGBA(x0, y0, currentColor)
		}

		if x0 == x1 && y0 == y1 {
			break
		}

		e2 := 2 * err
		if e2 >= dy {
			if x0 == x1 {
				break
			}
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			if y0 == y1 {
				break
			}
			err += dx
			y0 += sy
		}

		step++
	}
}

// Color interpolation
func interpolateColor(c1, c2 color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c1.R)*(1-t) + float64(c2.R)*t),
		G: uint8(float64(c1.G)*(1-t) + float64(c2.G)*t),
		B: uint8(float64(c1.B)*(1-t) + float64(c2.B)*t),
		A: uint8(float64(c1.A)*(1-t) + float64(c2.A)*t),
	}
}

// Helper functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
