package meshgrid

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// T7Suite-style axis scales: instead of a small orientation gizmo in the
// corner, the X (column), Y (row) and Z (value) scales are drawn along three
// edges of the mesh's own bounding box, labeled with the table's real axis
// values. The geometry is computed once per refresh in original (untransformed)
// coordinates, projected with projectOriginal, and consumed either as canvas
// overlay objects (shader/polygon backends) or rasterized into the image
// (image backend). Both paths share computeAxisGeometry so the projection and
// the label thinning live in one place.

const (
	axisTextSize float32 = 11
	// axisCharW is a rough per-character width at axisTextSize, used only to
	// decide how many tick labels fit before they overlap.
	axisCharW = 7.0
	// axisEdgeOffset lifts the whole axis (line, ticks, labels) off the mesh
	// edge so it doesn't sit directly on the surface. The distances below are
	// measured outward from this shifted line.
	axisEdgeOffset = 8.0
	axisTickLen    = 7.0  // tick-mark length, px, drawn outward from the edge
	axisLabelPad   = 24.0 // outward distance of a value label's center from the lifted edge
	axisNameGap    = 16.0 // extra clearance of the axis-name label past the value labels
	// zDivisions is the number of intervals on the vertical value scale, so it
	// shows zDivisions+1 ticks from zmin to zmax.
	zDivisions = 5
)

// axisSeg is one screen-space line: a box edge or a tick mark.
type axisSeg struct {
	x1, y1, x2, y2 float32
	col            color.RGBA
}

// axisLabel is one screen-space text placed centered on (x, y).
type axisLabel struct {
	text string
	x, y float32
	col  color.RGBA
}

// initAxisObjects allocates the canvas pools for the overlay backends. The
// pools are sized to the worst case (no thinning) so the per-frame update only
// has to position the active entries and hide the rest; nothing here needs a
// driver, so it is safe from the constructor and from tests.
func (m *Meshgrid) initAxisObjects() {
	// Worst case: every column/row/Z tick visible, plus a small headroom for
	// the always-included last tick on each axis.
	maxTicks := m.cols + m.rows + (zDivisions + 1) + 3
	maxLines := 3 + maxTicks // three labeled edges + one tick mark each
	maxTexts := maxTicks + 3 // one value label each + three axis names

	m.axisLinePool = make([]*canvas.Line, maxLines)
	for i := range m.axisLinePool {
		m.axisLinePool[i] = &canvas.Line{StrokeWidth: 1, Hidden: true}
	}
	m.axisTextPool = make([]*canvas.Text, maxTexts)
	for i := range m.axisTextPool {
		t := canvas.NewText("", color.White)
		t.TextSize = axisTextSize
		t.Hidden = true
		m.axisTextPool[i] = t
	}
}

// updateAxisObjects drives the canvas pools from the current axis geometry:
// active entries get positioned and shown, the rest hidden.
func (m *Meshgrid) updateAxisObjects() {
	segs, labels := m.computeAxisGeometry()

	for i, l := range m.axisLinePool {
		if i < len(segs) {
			s := segs[i]
			l.StrokeColor = s.col
			l.Position1 = fyne.NewPos(s.x1, s.y1)
			l.Position2 = fyne.NewPos(s.x2, s.y2)
			if l.Hidden {
				l.Show()
			}
		} else if !l.Hidden {
			l.Hide()
		}
	}

	for i, t := range m.axisTextPool {
		if i < len(labels) {
			lb := labels[i]
			t.Text = lb.text
			t.Color = lb.col
			sz := t.MinSize()
			t.Resize(sz)
			// canvas.Text positions by its top-left; center it on the anchor.
			t.Move(fyne.NewPos(lb.x-sz.Width/2, lb.y-sz.Height/2))
			if t.Hidden {
				t.Show()
			}
		} else if !t.Hidden {
			t.Hide()
		}
	}
}

// drawAxisScales rasterizes the same axis geometry into the image backend's
// frame, drawing edges/ticks as Bresenham lines and labels with the bitmap font.
func (m *Meshgrid) drawAxisScales(img *image.RGBA) {
	segs, labels := m.computeAxisGeometry()
	for _, s := range segs {
		drawBresenhamLine(img, int(s.x1), int(s.y1), int(s.x2), int(s.y2), s.col, s.col)
	}
	for _, l := range labels {
		// basicfont.Face7x13 is 7px wide per glyph; the drawer anchors at the
		// baseline, so nudge so the label reads roughly centered on its anchor.
		w := len(l.text) * 7
		m.drawText(img, l.text, int(l.x)-w/2, int(l.y)+4, l.col)
	}
}

// computeAxisGeometry builds the screen-space edges, tick marks and labels for
// the three axis scales. It reuses the scratch slices and returns them.
func (m *Meshgrid) computeAxisGeometry() ([]axisSeg, []axisLabel) {
	segs := m.scratchAxisSegs[:0]
	labels := m.scratchAxisLabels[:0]
	if m.size.Width <= 0 || m.size.Height <= 0 {
		m.scratchAxisSegs, m.scratchAxisLabels = segs, labels
		return segs, labels
	}

	cw, ch := float64(m.cellWidth), float64(m.cellHeight)
	xMax := float64(m.cols) * cw
	yMin, yMax := ch, float64(m.rows+1)*ch
	zTop := m.depth

	// The four floor corners (z=0). The front-most (largest screen Y) carries
	// the X and Y scales on its two outgoing floor edges. The vertical Z scale
	// goes on the most side-on of the remaining corners (the leftmost), so its
	// edge rides the silhouette in open space instead of being buried behind
	// the surface the way the back corner was.
	type pt struct{ ox, oy float64 }
	floor := [4]pt{{0, yMin}, {xMax, yMin}, {0, yMax}, {xMax, yMax}}
	frontIdx := 0
	frontY := float32(math.Inf(-1))
	for i, c := range floor {
		_, sy, _ := m.projectOriginal(c.ox, c.oy, 0)
		if sy > frontY {
			frontY, frontIdx = sy, i
		}
	}
	zIdx := -1
	zX := float32(math.Inf(1))
	for i, c := range floor {
		if i == frontIdx {
			continue
		}
		sx, _, _ := m.projectOriginal(c.ox, c.oy, 0)
		if sx < zX {
			zX, zIdx = sx, i
		}
	}
	front, zCorner := floor[frontIdx], floor[zIdx]

	// "Inside" reference: screen centroid of all eight box corners. Labels are
	// pushed outward from it so they sit outside the surface.
	var sumX, sumY float32
	for _, oz := range [2]float64{0, zTop} {
		for _, c := range floor {
			sx, sy, _ := m.projectOriginal(c.ox, c.oy, oz)
			sumX += sx
			sumY += sy
		}
	}
	inside := fyne.NewPos(sumX/8, sumY/8)

	xCol := color.RGBA{R: 255, G: 90, B: 90, A: 255}
	yCol := color.RGBA{R: 90, G: 220, B: 90, A: 255}
	zCol := color.RGBA{R: 120, G: 170, B: 255, A: 255}

	// X scale: front edge at constant Y, value per column at the cell center.
	nx := 0
	if len(m.xData) >= m.cols {
		nx = m.cols
	}
	segs, labels = m.appendAxis(segs, labels, inside, xCol,
		[3]float64{0, front.oy, 0}, [3]float64{xMax, front.oy, 0}, m.xlabel, nx,
		func(k int) [3]float64 { return [3]float64{(float64(k) + 0.5) * cw, front.oy, 0} },
		func(k int) string { return strconv.FormatFloat(m.xData[k], 'f', m.xPrec, 64) })

	// Y scale: side edge at constant X. Data row 0 sits at the high-Y (far)
	// end, so row k maps to Oy = (rows+0.5-k)*ch.
	ny := 0
	if len(m.yData) >= m.rows {
		ny = m.rows
	}
	segs, labels = m.appendAxis(segs, labels, inside, yCol,
		[3]float64{front.ox, yMin, 0}, [3]float64{front.ox, yMax, 0}, m.ylabel, ny,
		func(k int) [3]float64 { return [3]float64{front.ox, (float64(m.rows) + 0.5 - float64(k)) * ch, 0} },
		func(k int) string { return strconv.FormatFloat(m.yData[k], 'f', m.yPrec, 64) })

	// Z scale: vertical edge at the side corner, zmin..zmax mapped to 0..zTop.
	nz := 0
	if m.zrange > 0 && zTop > 0 {
		nz = zDivisions + 1
	}
	segs, labels = m.appendAxis(segs, labels, inside, zCol,
		[3]float64{zCorner.ox, zCorner.oy, 0}, [3]float64{zCorner.ox, zCorner.oy, zTop}, m.zlabel, nz,
		func(k int) [3]float64 { return [3]float64{zCorner.ox, zCorner.oy, float64(k) / zDivisions * zTop} },
		func(k int) string {
			return strconv.FormatFloat(m.zmin+float64(k)/zDivisions*m.zrange, 'f', m.zPrec, 64)
		})

	m.scratchAxisSegs, m.scratchAxisLabels = segs, labels
	return segs, labels
}

// appendAxis appends one labeled axis: the edge line from p0 to p1 (original
// coords) lifted off the mesh by axisEdgeOffset, the axis name centered on the
// middle of that edge, and a thinned set of tick marks plus value labels at the
// original-space points returned by pointAt(k), k in [0,n). Everything is
// offset along one outward edge normal so the ticks stay parallel and the whole
// scale sits clear of the surface. The name rides the middle so it doesn't
// collide with the corner tick values.
func (m *Meshgrid) appendAxis(segs []axisSeg, labels []axisLabel, inside fyne.Position, col color.RGBA,
	p0, p1 [3]float64, name string, n int, pointAt func(int) [3]float64, valueAt func(int) string,
) ([]axisSeg, []axisLabel) {
	sx0, sy0, _ := m.projectOriginal(p0[0], p0[1], p0[2])
	sx1, sy1, _ := m.projectOriginal(p1[0], p1[1], p1[2])

	// One outward normal for the whole edge keeps the ticks parallel. The mesh
	// transform is affine, so the tick points stay collinear with the edge and
	// land on the offset line after the same shift.
	nx, ny := edgeOutwardNormal(sx0, sy0, sx1, sy1, inside)
	ox, oy := nx*axisEdgeOffset, ny*axisEdgeOffset

	ex0, ey0 := sx0+ox, sy0+oy
	ex1, ey1 := sx1+ox, sy1+oy
	segs = append(segs, axisSeg{ex0, ey0, ex1, ey1, col})

	if name != "" {
		// Sit the name on the edge midpoint, just past the value-label band so
		// it never overlaps the corner ticks (and tracks axisLabelPad changes).
		mx, my := (sx0+sx1)*0.5, (sy0+sy1)*0.5
		nameDist := float32(axisEdgeOffset + axisLabelPad + axisNameGap)
		labels = append(labels, axisLabel{name, mx + nx*nameDist, my + ny*nameDist, col})
	}
	if n <= 0 {
		return segs, labels
	}

	// Thin labels to the count that fits along the projected edge length.
	L := math.Hypot(float64(ex1-ex0), float64(ey1-ey0))
	maxChars := 1
	for k := 0; k < n; k++ {
		if c := len(valueAt(k)); c > maxChars {
			maxChars = c
		}
	}
	step := axisLabelStep(n, L, maxChars)

	appendTick := func(k int) {
		p := pointAt(k)
		sx, sy, _ := m.projectOriginal(p[0], p[1], p[2])
		bx, by := sx+ox, sy+oy // tick base sits on the lifted edge line
		segs = append(segs, axisSeg{bx, by, bx + nx*axisTickLen, by + ny*axisTickLen, col})
		labels = append(labels, axisLabel{valueAt(k), bx + nx*axisLabelPad, by + ny*axisLabelPad, col})
	}
	for k := 0; k < n; k += step {
		appendTick(k)
	}
	// Always label the last tick so the axis' full extent is annotated.
	if last := n - 1; last%step != 0 {
		appendTick(last)
	}
	return segs, labels
}

// edgeOutwardNormal returns the unit screen-space normal of edge (s0->s1) that
// points away from the inside reference, used to lift the axis and its ticks
// outward in one consistent direction.
func edgeOutwardNormal(sx0, sy0, sx1, sy1 float32, inside fyne.Position) (float32, float32) {
	dx, dy := float64(sx1-sx0), float64(sy1-sy0)
	L := math.Hypot(dx, dy)
	if L < 1e-6 {
		return outward(sx0, sy0, inside) // degenerate edge: fall back to radial
	}
	nx, ny := -dy/L, dx/L
	mx, my := float64(sx0+sx1)*0.5, float64(sy0+sy1)*0.5
	if nx*(mx-float64(inside.X))+ny*(my-float64(inside.Y)) < 0 {
		nx, ny = -nx, -ny
	}
	return float32(nx), float32(ny)
}

// axisLabelStep returns the index stride that keeps drawn labels at least one
// label-width apart along a projected edge of screen length L.
func axisLabelStep(n int, L float64, maxChars int) int {
	if n <= 1 || L <= 0 {
		return 1
	}
	minSpacing := float64(maxChars)*axisCharW + 8
	fit := int(L / minSpacing)
	if fit < 1 {
		fit = 1
	}
	step := (n + fit - 1) / fit
	if step < 1 {
		step = 1
	}
	return step
}

// outward returns the unit screen-space direction from the inside reference
// toward (px, py), i.e. the direction to push a label so it clears the surface.
func outward(px, py float32, inside fyne.Position) (float32, float32) {
	dx, dy := float64(px-inside.X), float64(py-inside.Y)
	d := math.Hypot(dx, dy)
	if d < 1e-3 {
		return 0, 1
	}
	return float32(dx / d), float32(dy / d)
}

func (m *Meshgrid) drawText(img *image.RGBA, text string, x, y int, col color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)},
	}
	d.DrawString(text)
}
