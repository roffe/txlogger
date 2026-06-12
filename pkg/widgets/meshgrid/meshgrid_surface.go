package meshgrid

import (
	"image"
	"image/color"
	"math"
	"slices"
)

// RenderMode selects how the mesh is drawn.
type RenderMode int

const (
	// RenderModeSolidWireframe draws the filled surface with grid lines on top.
	RenderModeSolidWireframe RenderMode = iota
	// RenderModeSolid draws only the filled surface.
	RenderModeSolid
	// RenderModeWireframe draws the classic line mesh.
	RenderModeWireframe

	renderModeCount
)

// quadRef identifies one grid cell for the painter's-order surface pass.
type quadRef struct {
	i, j  int
	depth float64
}

// Grid lines drawn on top of the filled surface are dimmed so they read as a
// grid instead of blending into the identically colored fill.
const surfaceEdgeFade = 0.45

// drawSurface fills each grid cell with two Gouraud-shaded triangles,
// back-to-front. A fixed directional light flat-shades each quad so the
// surface relief stays visible even where the value color barely changes.
// When edges is true the cell outline is drawn right after its fill, which
// keeps lines on hidden faces correctly occluded by nearer quads.
func (m *Meshgrid) drawSurface(img *image.RGBA, projX, projY []int, vertCol []color.RGBA, edges bool) {
	// One quad per data cell; the corner-vertex grid is (rows+1) x (cols+1).
	quads := m.scratchQuads[:0]
	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			z := m.vertices[i][j].Z + m.vertices[i][j+1].Z + m.vertices[i+1][j].Z + m.vertices[i+1][j+1].Z
			quads = append(quads, quadRef{i: i, j: j, depth: z * 0.25})
		}
	}
	m.scratchQuads = quads

	// Back-to-front: larger view-space Z is nearer the viewer (the depth
	// shading brightens large Z), so draw small-Z quads first.
	slices.SortFunc(quads, func(a, b quadRef) int {
		switch {
		case a.depth < b.depth:
			return -1
		case a.depth > b.depth:
			return 1
		default:
			return 0
		}
	})

	// Fixed light direction in view space, normalized once per frame.
	lx, ly, lz := 0.3, -0.5, 0.8
	il := 1 / math.Sqrt(lx*lx+ly*ly+lz*lz)
	lx, ly, lz = lx*il, ly*il, lz*il

	vCols := m.cols + 1
	for _, q := range quads {
		ai := q.i*vCols + q.j // top-left
		bi := ai + 1          // top-right
		di := ai + vCols      // bottom-left
		ci := di + 1          // bottom-right

		shade := m.quadShade(q.i, q.j, lx, ly, lz)
		ca := fadeColor(vertCol[ai], shade)
		cb := fadeColor(vertCol[bi], shade)
		cc := fadeColor(vertCol[ci], shade)
		cd := fadeColor(vertCol[di], shade)

		fillTriangle(img, projX[ai], projY[ai], projX[bi], projY[bi], projX[ci], projY[ci], ca, cb, cc)
		fillTriangle(img, projX[ai], projY[ai], projX[ci], projY[ci], projX[di], projY[di], ca, cc, cd)

		if edges {
			ea := fadeColor(vertCol[ai], surfaceEdgeFade)
			eb := fadeColor(vertCol[bi], surfaceEdgeFade)
			ec := fadeColor(vertCol[ci], surfaceEdgeFade)
			ed := fadeColor(vertCol[di], surfaceEdgeFade)
			drawBresenhamLine(img, projX[ai], projY[ai], projX[bi], projY[bi], ea, eb)
			drawBresenhamLine(img, projX[bi], projY[bi], projX[ci], projY[ci], eb, ec)
			drawBresenhamLine(img, projX[ci], projY[ci], projX[di], projY[di], ec, ed)
			drawBresenhamLine(img, projX[di], projY[di], projX[ai], projY[ai], ed, ea)
		}
	}
}

// quadShade computes a flat Lambert term for the cell at (i,j) from its
// view-space normal (cross product of the diagonals, which is robust for
// non-planar quads). The absolute dot product is used since the surface is
// single-sided and may be viewed from below.
func (m *Meshgrid) quadShade(i, j int, lx, ly, lz float64) float64 {
	a := m.vertices[i][j]
	b := m.vertices[i][j+1]
	c := m.vertices[i+1][j+1]
	d := m.vertices[i+1][j]

	ux, uy, uz := c.X-a.X, c.Y-a.Y, c.Z-a.Z
	vx, vy, vz := d.X-b.X, d.Y-b.Y, d.Z-b.Z

	nx := uy*vz - uz*vy
	ny := uz*vx - ux*vz
	nz := ux*vy - uy*vx

	nl := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if nl == 0 {
		return 1
	}
	dot := (nx*lx + ny*ly + nz*lz) / nl
	if dot < 0 {
		dot = -dot
	}
	return 0.6 + 0.4*dot
}

// fillTriangle rasterizes a triangle with per-vertex (Gouraud) color
// interpolation using incremental integer edge functions, clipped to the
// image bounds via the bounding box.
func fillTriangle(img *image.RGBA, x0, y0, x1, y1, x2, y2 int, c0, c1, c2 color.RGBA) {
	area := (x1-x0)*(y2-y0) - (y1-y0)*(x2-x0)
	if area == 0 {
		return
	}
	if area < 0 {
		x1, y1, x2, y2 = x2, y2, x1, y1
		c1, c2 = c2, c1
		area = -area
	}

	r := img.Rect
	minX := max(min(x0, min(x1, x2)), r.Min.X)
	maxX := min(max(x0, max(x1, x2)), r.Max.X-1)
	minY := max(min(y0, min(y1, y2)), r.Min.Y)
	maxY := min(max(y0, max(y1, y2)), r.Max.Y-1)
	if minX > maxX || minY > maxY {
		return
	}

	// Edge function values at (minX, minY); stepping +1 in x adds the A term,
	// +1 in y adds the B term.
	ef := func(ax, ay, bx, by, px, py int) int {
		return (bx-ax)*(py-ay) - (by-ay)*(px-ax)
	}
	w0Row := ef(x1, y1, x2, y2, minX, minY)
	w1Row := ef(x2, y2, x0, y0, minX, minY)
	w2Row := ef(x0, y0, x1, y1, minX, minY)
	a0, b0 := y1-y2, x2-x1
	a1, b1 := y2-y0, x0-x2
	a2, b2 := y0-y1, x1-x0

	invArea := 1.0 / float64(area)
	r0, g0, bl0 := float64(c0.R), float64(c0.G), float64(c0.B)
	r1, g1, bl1 := float64(c1.R), float64(c1.G), float64(c1.B)
	r2, g2, bl2 := float64(c2.R), float64(c2.G), float64(c2.B)

	stride := img.Stride
	pix := img.Pix
	for y := minY; y <= maxY; y++ {
		w0, w1, w2 := w0Row, w1Row, w2Row
		idx := (y-r.Min.Y)*stride + (minX-r.Min.X)*4
		for x := minX; x <= maxX; x++ {
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				fw0 := float64(w0) * invArea
				fw1 := float64(w1) * invArea
				fw2 := float64(w2) * invArea
				pix[idx+0] = uint8(fw0*r0 + fw1*r1 + fw2*r2)
				pix[idx+1] = uint8(fw0*g0 + fw1*g1 + fw2*g2)
				pix[idx+2] = uint8(fw0*bl0 + fw1*bl1 + fw2*bl2)
				pix[idx+3] = 255
			}
			w0 += a0
			w1 += a1
			w2 += a2
			idx += 4
		}
		w0Row += b0
		w1Row += b1
		w2Row += b2
	}
}
