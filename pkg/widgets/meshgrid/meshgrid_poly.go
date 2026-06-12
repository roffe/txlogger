package meshgrid

import (
	"image/color"
	"math"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Polygon-based renderer experiment: instead of rasterizing the mesh into an
// RGBA image (and re-uploading that texture) every frame, each grid cell is a
// reusable canvas.ArbitraryPolygon that fyne's GL painter draws as a single
// SDF shader quad. A frame update only mutates points, colors and the
// painter's order of the object list — nothing is rasterized on the CPU.
//
// Trade-offs vs the image renderer:
//   - Flat shading: a polygon has one fill color, so the four corner colors
//     are averaged instead of Gouraud-interpolated across the quad.
//   - Wireframe mode uses the quad outlines (stroke), so the cell diagonals
//     are not drawn.
//   - fyne does not clip renderer objects to the widget bounds, so a zoomed
//     mesh can spill outside the widget area.

// polyPad expands each polygon's bounding box so the 1px stroke and the
// shader's antialiased edge aren't clipped at the object bounds (the painter
// clamps points to the object's own size).
const polyPad float32 = 1.5

// initPolygons creates the reusable cell polygons. Geometry and colors are
// filled in by updatePolygons; nothing here needs a driver, so it is safe to
// call from the constructor (and from tests without an app).
func (m *Meshgrid) initPolygons() {
	m.polys = make([]*canvas.ArbitraryPolygon, m.rows*m.cols)
	for i := range m.polys {
		m.polys[i] = &canvas.ArbitraryPolygon{Points: make([]fyne.Position, 4)}
	}
}

// updatePolygons projects the mesh and updates the reusable canvas objects:
// quad geometry, flat fill/stroke colors and the back-to-front painter's
// order of the renderer's object list.
func (m *Meshgrid) updatePolygons() {
	w, h := m.size.Width, m.size.Height
	if w <= 0 || h <= 0 || len(m.polys) == 0 {
		return
	}

	vRows, vCols := m.rows+1, m.cols+1

	// View-space depth range for the depth shading, same as the image path.
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

	// Per-vertex screen projection and color. Float projection keeps subpixel
	// precision, which the GPU edges make visible (the image path rounds to
	// whole pixels).
	n := vRows * vCols
	if cap(m.scratchFX) < n {
		m.scratchFX = make([]float32, n)
		m.scratchFY = make([]float32, n)
	}
	if cap(m.scratchColors) < n {
		m.scratchColors = make([]color.RGBA, n)
	}
	fxs := m.scratchFX[:n]
	fys := m.scratchFY[:n]
	vertCol := m.scratchColors[:n]

	cx := float64(w) * 0.5
	cy := float64(h) * 0.5
	for i := 0; i < vRows; i++ {
		row := m.vertices[i]
		base := i * vCols
		for j := 0; j < vCols; j++ {
			v := row[j]
			idx := base + j
			fxs[idx] = float32(cx + v.X)
			fys[idx] = float32(cy + v.Y)
			depth := (v.Z - minZ) / zRange
			vertCol[idx] = m.getColorWithDepth(v.V, depth)
		}
	}

	// Fixed light direction in view space, normalized once per frame.
	lx, ly, lz := 0.3, -0.5, 0.8
	il := 1 / math.Sqrt(lx*lx+ly*ly+lz*lz)
	lx, ly, lz = lx*il, ly*il, lz*il

	mode := m.renderMode

	quads := m.scratchQuads[:0]
	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			ai := i*vCols + j // top-left
			bi := ai + 1      // top-right
			di := ai + vCols  // bottom-left
			ci := di + 1      // bottom-right

			z := m.vertices[i][j].Z + m.vertices[i][j+1].Z + m.vertices[i+1][j].Z + m.vertices[i+1][j+1].Z
			quads = append(quads, quadRef{i: i, j: j, depth: z * 0.25})

			poly := m.polys[i*m.cols+j]

			ax, ay := fxs[ai], fys[ai]
			bx, by := fxs[bi], fys[bi]
			ccx, ccy := fxs[ci], fys[ci]
			dx, dy := fxs[di], fys[di]

			minx := min(min(ax, bx), min(ccx, dx))
			maxx := max(max(ax, bx), max(ccx, dx))
			miny := min(min(ay, by), min(ccy, dy))
			maxy := max(max(ay, by), max(ccy, dy))

			x0, y0 := ax-minx+polyPad, ay-miny+polyPad
			x1, y1 := bx-minx+polyPad, by-miny+polyPad
			x2, y2 := ccx-minx+polyPad, ccy-miny+polyPad
			x3, y3 := dx-minx+polyPad, dy-miny+polyPad

			// A quad folding over itself at the mesh silhouette projects to a
			// self-intersecting outline, which both fyne painters mangle (the
			// software stroker floods the whole bounding box). Swap a corner
			// pair to make the polygon simple again.
			if segmentsCross(x0, y0, x1, y1, x2, y2, x3, y3) {
				x1, y1, x2, y2 = x2, y2, x1, y1
			} else if segmentsCross(x1, y1, x2, y2, x3, y3, x0, y0) {
				x2, y2, x3, y3 = x3, y3, x2, y2
			}

			// Corners of an edge-on cell project onto each other, giving the
			// polygon zero-length edges. The GL shader normalize()s the edge
			// vectors, so those become NaN and flood the bounding box with
			// color. Drop collapsed corners (the quad degrades to a triangle);
			// under three distinct corners the cell is invisible anyway.
			pts := appendDistinct(poly.Points[:0], [4]fyne.Position{
				{X: x0, Y: y0}, {X: x1, Y: y1}, {X: x2, Y: y2}, {X: x3, Y: y3},
			})
			poly.Points = pts
			if len(pts) < 3 {
				if !poly.Hidden {
					poly.Hide()
				}
				continue
			}
			if poly.Hidden {
				poly.Show()
			}

			// Flat shade: average the four corner colors, then apply the same
			// Lambert term the image renderer uses per quad.
			shade := m.quadShade(i, j, lx, ly, lz)
			fill := avgQuadColor(vertCol[ai], vertCol[bi], vertCol[ci], vertCol[di], shade)

			switch mode {
			case RenderModeSolidWireframe:
				poly.FillColor = fill
				poly.StrokeColor = fadeColor(fill, surfaceEdgeFade)
				poly.StrokeWidth = 1
			case RenderModeSolid:
				poly.FillColor = fill
				poly.StrokeColor = color.Transparent
				poly.StrokeWidth = 0
			case RenderModeWireframe:
				poly.FillColor = color.Transparent
				poly.StrokeColor = fill
				poly.StrokeWidth = 1
			}

			// The painter clamps points to the object's own bounds, so size
			// the object to the quad's padded bbox and make points relative.
			poly.Move(fyne.NewPos(minx-polyPad, miny-polyPad))
			poly.Resize(fyne.NewSize(maxx-minx+2*polyPad, maxy-miny+2*polyPad))
		}
	}
	m.scratchQuads = quads

	// Back-to-front painter's order, applied by reordering the object list.
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

	objs := m.polyObjects[:0]
	for _, q := range quads {
		objs = append(objs, m.polys[q.i*m.cols+q.j])
	}
	m.updateAxisObjects()
	for i := range m.axisLines {
		objs = append(objs, m.axisLines[i], m.axisLabels[i])
	}
	objs = append(objs, m.cursor)
	m.polyObjects = objs
}

// minEdgeSq is the squared minimum polygon edge length below which two
// corners count as collapsed onto each other. The GL painter rounds every
// point to whole device pixels before the shader sees it, so corners must
// stay far enough apart that they can't land on the same pixel after
// rounding: distance d keeps the larger coordinate delta ≥ d/√2, which
// survives rounding while d·pixScale > √2 — at d=2 that covers any
// pixScale ≥ 0.75.
const minEdgeSq float32 = 4

// appendDistinct appends the corners to dst, skipping ones that collapse
// onto the previously kept corner (including last-onto-first wrap-around),
// so every edge of the resulting polygon has a usable direction vector.
func appendDistinct(dst []fyne.Position, corners [4]fyne.Position) []fyne.Position {
	for _, p := range corners {
		if len(dst) > 0 {
			last := dst[len(dst)-1]
			dx, dy := p.X-last.X, p.Y-last.Y
			if dx*dx+dy*dy < minEdgeSq {
				continue
			}
		}
		dst = append(dst, p)
	}
	for len(dst) >= 2 {
		first, last := dst[0], dst[len(dst)-1]
		dx, dy := first.X-last.X, first.Y-last.Y
		if dx*dx+dy*dy >= minEdgeSq {
			break
		}
		dst = dst[:len(dst)-1]
	}
	return dst
}

// segmentsCross reports whether segments ab and cd properly cross (shared or
// collinear endpoints don't count, which is fine for untwisting quads).
func segmentsCross(ax, ay, bx, by, cx, cy, dx, dy float32) bool {
	orient := func(px, py, qx, qy, rx, ry float32) float32 {
		return (qx-px)*(ry-py) - (qy-py)*(rx-px)
	}
	d1 := orient(ax, ay, bx, by, cx, cy)
	d2 := orient(ax, ay, bx, by, dx, dy)
	d3 := orient(cx, cy, dx, dy, ax, ay)
	d4 := orient(cx, cy, dx, dy, bx, by)
	return (d1 > 0) != (d2 > 0) && (d3 > 0) != (d4 > 0)
}

// avgQuadColor averages the four corner colors and applies the flat Lambert
// shade. shade is <= 1 so the components can't overflow.
func avgQuadColor(a, b, c, d color.RGBA, shade float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64((int(a.R)+int(b.R)+int(c.R)+int(d.R))>>2) * shade),
		G: uint8(float64((int(a.G)+int(b.G)+int(c.G)+int(d.G))>>2) * shade),
		B: uint8(float64((int(a.B)+int(b.B)+int(c.B)+int(d.B))>>2) * shade),
		A: 255,
	}
}
