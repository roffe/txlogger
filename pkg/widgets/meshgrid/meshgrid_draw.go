package meshgrid

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
)

// lineSegment indexes into the precomputed projected/color slices so we don't
// duplicate the Vertex data per edge.
type lineSegment struct {
	idx1, idx2 int
	x1, y1     int
	x2, y2     int
	depth      float64
	diagonal   bool
}

// drawMeshgridLines renders the meshgrid into a reusable RGBA buffer.
func (m *Meshgrid) drawMeshgridLines() *image.RGBA {
	w, h := int(m.size.Width), int(m.size.Height)
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}

	// Reuse the backing image across frames when the size hasn't changed,
	// otherwise allocate a fresh one.
	img := m.scratchImg
	if img == nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		m.scratchImg = img
	} else {
		clear(img.Pix)
	}

	// Vertices sit on cell corners, so the grid is one larger than the data
	// in each direction (one quad per table cell).
	vRows, vCols := m.rows+1, m.cols+1

	// Find min/max of the view-space Z for depth shading.
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for i := 0; i < vRows; i++ {
		row := m.vertices[i]
		for j := 0; j < vCols; j++ {
			z := row[j].Z
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
		}
	}
	zRange := maxZ - minZ
	if zRange == 0 {
		zRange = 1
	}

	// Precompute screen-space projection and color for each vertex once.
	n := vRows * vCols
	if cap(m.scratchProjX) < n {
		m.scratchProjX = make([]int, n)
		m.scratchProjY = make([]int, n)
		m.scratchColors = make([]color.RGBA, n)
	}
	projX := m.scratchProjX[:n]
	projY := m.scratchProjY[:n]
	vertCol := m.scratchColors[:n]

	cx := float64(m.size.Width) * 0.5
	cy := float64(m.size.Height) * 0.5
	for i := 0; i < vRows; i++ {
		row := m.vertices[i]
		base := i * vCols
		for j := 0; j < vCols; j++ {
			v := row[j]
			idx := base + j
			projX[idx] = int(cx + v.X)
			projY[idx] = int(cy + v.Y)
			depth := (v.Z - minZ) / zRange
			vertCol[idx] = m.getColorWithDepth(v.V, depth)
		}
	}

	mode := m.renderMode

	if mode != RenderModeWireframe {
		m.drawSurface(img, projX, projY, vertCol, mode == RenderModeSolidWireframe)
		m.drawAxisScales(img)
		return img
	}

	// Collect line segments using cached projections.
	segs := m.scratchLines[:0]
	for i := 0; i < vRows; i++ {
		for j := 0; j < vCols; j++ {
			idx := i*vCols + j
			x1, y1 := projX[idx], projY[idx]
			// neighbors: (+1,0) down, (0,+1) right, (+1,-1) diagonal
			tryAddSeg := func(ni, nj int) {
				if ni >= vRows || nj < 0 || nj >= vCols {
					return
				}
				nidx := ni*vCols + nj
				x2, y2 := projX[nidx], projY[nidx]
				dx, dy := x2-x1, y2-y1
				if dx*dx+dy*dy < 4 {
					return
				}
				segs = append(segs, lineSegment{
					idx1:     idx,
					idx2:     nidx,
					x1:       x1,
					y1:       y1,
					x2:       x2,
					y2:       y2,
					depth:    (m.vertices[i][j].Z + m.vertices[ni][nj].Z) * 0.5,
					diagonal: x1 != x2 && y1 != y2,
				})
			}
			tryAddSeg(i+1, j)
			tryAddSeg(i, j+1)
			tryAddSeg(i+1, j-1)
		}
	}
	m.scratchLines = segs

	// Back-to-front painter's order.
	slices.SortFunc(segs, func(a, b lineSegment) int {
		switch {
		case a.depth < b.depth:
			return -1
		case a.depth > b.depth:
			return 1
		default:
			return 0
		}
	})

	for _, s := range segs {
		c1 := vertCol[s.idx1]
		c2 := vertCol[s.idx2]
		if s.diagonal {
			c1 = fadeColor(c1, 0.7)
			c2 = fadeColor(c2, 0.7)
		}
		drawBresenhamLine(img, s.x1, s.y1, s.x2, s.y2, c1, c2)
	}

	m.drawAxisScales(img)

	return img
}

// cursorScreenPosition projects the tracking-marker cell position set by
// SetCursor onto the screen so the marker rides the surface the shader draws.
func (m *Meshgrid) cursorScreenPosition() (float32, float32) {
	if m.dataVertexMode() {
		// The shader's vertices are the cell values themselves, so the marker
		// rides the triangulated data surface: project the cell-centered data
		// point at the (fractional) cursor, with the same Ox/Oy convention the
		// axis uses. SetCursor clamps the indices to the data grid.
		cw, ch := float64(m.cellWidth), float64(m.cellHeight)
		ox := (m.cursorX + 0.5) * cw
		oy := (float64(m.rows) + 0.5 - m.cursorY) * ch
		zr := m.zrange
		if zr == 0 {
			zr = 1
		}
		oz := (m.sampleValue(m.cursorX, m.cursorY) - m.zmin) / zr * m.depth
		sx, sy, _ := m.projectOriginal(ox, oy, oz)
		return sx, sy
	}

	// Corner-averaged fallback: MapViewer indices are cell-centered while mesh
	// vertices sit on cell corners; +0.5 lands the marker mid-cell on the corner
	// grid. The camera transform is linear, so bilinearly interpolating the
	// transformed corners lands on the same point as transforming the
	// interpolated one.
	sx := m.cursorX + 0.5
	sy := m.cursorY + 0.5

	x0 := int(sx)
	y0 := int(sy)
	x1 := min(x0+1, m.cols)
	y1 := min(y0+1, m.rows)
	fx := sx - float64(x0)
	fy := sy - float64(y0)

	v00 := m.vertices[y0][x0]
	v01 := m.vertices[y0][x1]
	v10 := m.vertices[y1][x0]
	v11 := m.vertices[y1][x1]

	vx := (1-fy)*((1-fx)*v00.X+fx*v01.X) + fy*((1-fx)*v10.X+fx*v11.X)
	vy := (1-fy)*((1-fx)*v00.Y+fx*v01.Y) + fy*((1-fx)*v10.Y+fx*v11.Y)

	return float32(float64(m.size.Width)*0.5 + vx), float32(float64(m.size.Height)*0.5 + vy)
}

// sampleValue bilinearly interpolates the cell values at the fractional cell
// index (fx = column, fy = row), clamped to the data grid.
func (m *Meshgrid) sampleValue(fx, fy float64) float64 {
	cx0 := min(max(int(math.Floor(fx)), 0), m.cols-1)
	cy0 := min(max(int(math.Floor(fy)), 0), m.rows-1)
	cx1 := min(cx0+1, m.cols-1)
	cy1 := min(cy0+1, m.rows-1)
	tx := fx - float64(cx0)
	if tx < 0 {
		tx = 0
	} else if tx > 1 {
		tx = 1
	}
	ty := fy - float64(cy0)
	if ty < 0 {
		ty = 0
	} else if ty > 1 {
		ty = 1
	}
	v00 := m.values[cy0*m.cols+cx0]
	v01 := m.values[cy0*m.cols+cx1]
	v10 := m.values[cy1*m.cols+cx0]
	v11 := m.values[cy1*m.cols+cx1]
	return (1-ty)*((1-tx)*v00+tx*v01) + ty*((1-tx)*v10+tx*v11)
}

// getColorWithDepth combines color interpolation and depth enhancement in one step
func (m *Meshgrid) getColorWithDepth(value, depthFactor float64) color.RGBA {
	// Get base color from value
	// baseColor := m.getColorInterpolation(value)
	baseColor := colors.GetColorInterpolation(
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

// Fade a color by a factor (used for diagonals). Alpha is left untouched:
// the buffer uses straight alpha, so dimming RGB and A together would fade
// the line twice over once composited.
func fadeColor(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * factor),
		G: uint8(float64(c.G) * factor),
		B: uint8(float64(c.B) * factor),
		A: c.A,
	}
}

// Fast Bresenham with clipping + direct Pix writes + fixed-point color interpolation.
func drawBresenhamLine(img *image.RGBA, x0, y0, x1, y1 int, c1, c2 color.RGBA) {
	r := img.Rect
	if !clipCohenSutherland(&x0, &y0, &x1, &y1, r.Min.X, r.Min.Y, r.Max.X-1, r.Max.Y-1) {
		return // fully outside
	}

	// Translate to image origin once so the pixel loop indexes directly.
	x0 -= r.Min.X
	x1 -= r.Min.X
	y0 -= r.Min.Y
	y1 -= r.Min.Y
	stride := img.Stride
	pix := img.Pix

	// Bresenham setup
	dx := common.Abs(x1 - x0)
	dy := -common.Abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy

	// Steps for color interpolation
	total := dx
	if -dy > total {
		total = -dy
	}
	if total == 0 {
		setPix(pix, stride, x0, y0, c1)
		return
	}

	// 16.16 fixed-point accumulators for R,G,B,A
	// acc starts at c1, step = (c2-c1)/total
	accR := int(c1.R) << 16
	accG := int(c1.G) << 16
	accB := int(c1.B) << 16
	accA := int(c1.A) << 16
	stepR := ((int(c2.R) - int(c1.R)) << 16) / total
	stepG := ((int(c2.G) - int(c1.G)) << 16) / total
	stepB := ((int(c2.B) - int(c1.B)) << 16) / total
	stepA := ((int(c2.A) - int(c1.A)) << 16) / total

	// Draw
	for i := 0; ; i++ {
		setPixRGBAFixed(pix, stride, x0, y0, accR, accG, accB, accA)

		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := err << 1
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}

		// increment color
		accR += stepR
		accG += stepG
		accB += stepB
		accA += stepA
	}
}

func setPix(pix []uint8, stride, x, y int, c color.RGBA) {
	i := y*stride + x*4
	pix[i+0] = c.R
	pix[i+1] = c.G
	pix[i+2] = c.B
	pix[i+3] = c.A
}

func setPixRGBAFixed(pix []uint8, stride, x, y int, r, g, b, a int) {
	i := y*stride + x*4
	pix[i+0] = byte(r >> 16)
	pix[i+1] = byte(g >> 16)
	pix[i+2] = byte(b >> 16)
	pix[i+3] = byte(a >> 16)
}

// Cohen–Sutherland line clipping (inclusive bounds).
const (
	codeInside = 0
	codeLeft   = 1
	codeRight  = 2
	codeBottom = 4
	codeTop    = 8
)

func outCode(x, y, xmin, ymin, xmax, ymax int) int {
	code := codeInside
	if x < xmin {
		code |= codeLeft
	} else if x > xmax {
		code |= codeRight
	}
	if y < ymin {
		code |= codeBottom
	} else if y > ymax {
		code |= codeTop
	}
	return code
}

func clipCohenSutherland(x0, y0, x1, y1 *int, xmin, ymin, xmax, ymax int) bool {
	x0i, y0i, x1i, y1i := *x0, *y0, *x1, *y1
	for {
		c0 := outCode(x0i, y0i, xmin, ymin, xmax, ymax)
		c1 := outCode(x1i, y1i, xmin, ymin, xmax, ymax)
		if (c0 | c1) == 0 {
			// both inside
			break
		}
		if (c0 & c1) != 0 {
			// fully outside
			return false
		}
		// pick an endpoint outside
		var cx int
		if c0 != 0 {
			cx = c0
		} else {
			cx = c1
		}

		var x, y int
		if (cx & codeTop) != 0 {
			x = x0i + (x1i-x0i)*(ymax-y0i)/(y1i-y0i)
			y = ymax
		} else if (cx & codeBottom) != 0 {
			x = x0i + (x1i-x0i)*(ymin-y0i)/(y1i-y0i)
			y = ymin
		} else if (cx & codeRight) != 0 {
			y = y0i + (y1i-y0i)*(xmax-x0i)/(x1i-x0i)
			x = xmax
		} else { // left
			y = y0i + (y1i-y0i)*(xmin-x0i)/(x1i-x0i)
			x = xmin
		}

		if cx == c0 {
			x0i, y0i = x, y
		} else {
			x1i, y1i = x, y
		}
	}
	*x0, *y0, *x1, *y1 = x0i, y0i, x1i, y1i
	return true
}
