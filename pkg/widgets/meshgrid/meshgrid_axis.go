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
	// axisShallowSin is the sine of the screen angle between the X and Y
	// floor edges below which their label bands start printing into the same
	// screen band (fully collinear when the floor is viewed edge-on).
	axisShallowSin = 0.12
	// axisBandLift is how far the shorter of two colliding floor axes is
	// lifted along its outward normal at full collinearity: past the other
	// axis' value-label and name rows, so the two scales stack readably
	// instead of overprinting.
	axisBandLift = 56.0
	// axisMinEdgePx is the projected edge length below which tick marks and
	// value labels are skipped: an edge foreshortened almost to a point (the
	// Z scale in a top-down view) stacks all its labels onto one spot.
	axisMinEdgePx = 24.0
	// axisCornerHysteresis is how many pixels better a new corner pick must
	// be before it replaces the previous frame's; without the margin a
	// near-tie flips the winner with every pixel of drag.
	axisCornerHysteresis = 24.0
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
			// Only write the color when it changed: assigning a color.RGBA
			// to the color.Color field boxes it on the heap every frame.
			if c, ok := l.StrokeColor.(color.RGBA); !ok || c != s.col {
				l.StrokeColor = s.col
			}
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
			if c, ok := t.Color.(color.RGBA); !ok || c != lb.col {
				t.Color = lb.col
			}
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
	var floorX, floorY [4]float32
	var floorMeanX float32
	for i, c := range floor {
		sx, sy, _ := m.projectOriginal(c.ox, c.oy, 0)
		floorX[i], floorY[i] = sx, sy
		floorMeanX += sx * 0.25
		if sy > frontY {
			frontY, frontIdx = sy, i
		}
	}
	// Both corner picks get hysteresis: near a tie the winner otherwise flips
	// with every pixel of drag, bouncing the scales from one edge to another.
	// The previous frame's corner is kept until the new winner clearly beats it.
	if p := m.frontCornerIdx; p >= 0 && p != frontIdx && floorY[p] > frontY-axisCornerHysteresis {
		frontIdx = p
	}
	m.frontCornerIdx = frontIdx

	// The Z scale should ride the silhouette, so among the remaining corners
	// pick the one whose projected X sits farthest from the floor's screen
	// center. Simply taking the leftmost could pick an interior corner when a
	// shallow view squeezes the floor corners toward one line, running the
	// scale straight through the middle of the surface.
	zIdx := -1
	zDist := float32(-1)
	for i := range floor {
		if i == frontIdx {
			continue
		}
		d := floorX[i] - floorMeanX
		if d < 0 {
			d = -d
		}
		if d > zDist {
			zDist, zIdx = d, i
		}
	}
	if p := m.zCornerIdx; p >= 0 && p != frontIdx && p != zIdx {
		d := floorX[p] - floorMeanX
		if d < 0 {
			d = -d
		}
		if d > zDist-axisCornerHysteresis {
			zIdx = p
		}
	}
	m.zCornerIdx = zIdx
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

	// With the floor viewed at a shallow angle, the X and Y edges project
	// toward the same screen line and their label bands overprint. Lift the
	// shorter-projected axis outward past the other's rows so the two scales
	// stack; the lift fades back to zero as the viewing angle opens up.
	liftX, liftY := float32(0), float32(0)
	{
		x0, y0, _ := m.projectOriginal(0, front.oy, 0)
		x1, y1, _ := m.projectOriginal(xMax, front.oy, 0)
		u0, v0, _ := m.projectOriginal(front.ox, yMin, 0)
		u1, v1, _ := m.projectOriginal(front.ox, yMax, 0)
		xdx, xdy := float64(x1-x0), float64(y1-y0)
		ydx, ydy := float64(u1-u0), float64(v1-v0)
		xL, yL := math.Hypot(xdx, xdy), math.Hypot(ydx, ydy)
		if xL > 0 && yL > 0 {
			if s := math.Abs(xdx*ydy-xdy*ydx) / (xL * yL); s < axisShallowSin {
				lift := float32((1 - s/axisShallowSin) * axisBandLift)
				if xL < yL {
					liftX = lift
				} else {
					liftY = lift
				}
			}
		}
	}

	// X scale: front edge at constant Y, value per column at the cell center.
	nx := 0
	if len(m.xData) >= m.cols {
		nx = m.cols
		if len(m.xLabels) != nx {
			m.xLabels = formatAxisLabels(m.xData[:nx], m.xPrec)
		}
	}
	segs, labels = m.appendAxis(segs, labels, inside, xCol, liftX,
		[3]float64{0, front.oy, 0}, [3]float64{xMax, front.oy, 0}, m.xlabel, nx,
		func(k int) [3]float64 { return [3]float64{(float64(k) + 0.5) * cw, front.oy, 0} },
		m.xLabels)

	// Y scale: side edge at constant X. Data row 0 sits at the high-Y (far)
	// end, so row k maps to Oy = (rows+0.5-k)*ch.
	ny := 0
	if len(m.yData) >= m.rows {
		ny = m.rows
		if len(m.yLabels) != ny {
			m.yLabels = formatAxisLabels(m.yData[:ny], m.yPrec)
		}
	}
	segs, labels = m.appendAxis(segs, labels, inside, yCol, liftY,
		[3]float64{front.ox, yMin, 0}, [3]float64{front.ox, yMax, 0}, m.ylabel, ny,
		func(k int) [3]float64 { return [3]float64{front.ox, (float64(m.rows) + 0.5 - float64(k)) * ch, 0} },
		m.yLabels)

	// Z scale: vertical edge at the side corner, zmin..zmax mapped to 0..zTop.
	nz := 0
	if m.zrange > 0 && zTop > 0 {
		nz = zDivisions + 1
		if len(m.zLabels) != nz || m.zLabelMin != m.zmin || m.zLabelRange != m.zrange {
			m.zLabels = m.zLabels[:0]
			for k := 0; k < nz; k++ {
				m.zLabels = append(m.zLabels, strconv.FormatFloat(m.zmin+float64(k)/zDivisions*m.zrange, 'f', m.zPrec, 64))
			}
			m.zLabelMin, m.zLabelRange = m.zmin, m.zrange
		}
	}
	segs, labels = m.appendAxis(segs, labels, inside, zCol, 0,
		[3]float64{zCorner.ox, zCorner.oy, 0}, [3]float64{zCorner.ox, zCorner.oy, zTop}, m.zlabel, nz,
		func(k int) [3]float64 { return [3]float64{zCorner.ox, zCorner.oy, float64(k) / zDivisions * zTop} },
		m.zLabels)

	m.scratchAxisSegs, m.scratchAxisLabels = segs, labels
	return segs, labels
}

// appendAxis appends one labeled axis: the edge line from p0 to p1 (original
// coords) lifted off the mesh by axisEdgeOffset plus the caller's extra lift,
// the axis name centered on the middle of that edge, and a thinned set of
// tick marks plus the value labels vals[k] at the original-space points
// returned by pointAt(k), k in [0,n). Everything is offset along one outward
// edge normal so the ticks stay parallel and the whole scale sits clear of
// the surface. The name rides the middle so it doesn't collide with the
// corner tick values.
func (m *Meshgrid) appendAxis(segs []axisSeg, labels []axisLabel, inside fyne.Position, col color.RGBA, lift float32,
	p0, p1 [3]float64, name string, n int, pointAt func(int) [3]float64, vals []string,
) ([]axisSeg, []axisLabel) {
	sx0, sy0, _ := m.projectOriginal(p0[0], p0[1], p0[2])
	sx1, sy1, _ := m.projectOriginal(p1[0], p1[1], p1[2])

	// One outward normal for the whole edge keeps the ticks parallel. The mesh
	// transform is affine, so the tick points stay collinear with the edge and
	// land on the offset line after the same shift.
	nx, ny := edgeOutwardNormal(sx0, sy0, sx1, sy1, inside)
	ox, oy := nx*(axisEdgeOffset+lift), ny*(axisEdgeOffset+lift)

	ex0, ey0 := sx0+ox, sy0+oy
	ex1, ey1 := sx1+ox, sy1+oy
	segs = append(segs, axisSeg{ex0, ey0, ex1, ey1, col})

	if name != "" {
		// Sit the name on the edge midpoint, just past the value-label band so
		// it never overlaps the corner ticks (and tracks axisLabelPad changes).
		// The lift moves the whole scale, so the name must ride along or it
		// ends up on the wrong side of its own axis line.
		mx, my := (sx0+sx1)*0.5, (sy0+sy1)*0.5
		nameDist := lift + float32(axisEdgeOffset+axisLabelPad+axisNameGap)
		labels = append(labels, axisLabel{name, mx + nx*nameDist, my + ny*nameDist, col})
	}
	if n <= 0 {
		return segs, labels
	}

	// Thin labels to the count that fits along the projected edge length.
	L := math.Hypot(float64(ex1-ex0), float64(ey1-ey0))
	if L < axisMinEdgePx {
		// Foreshortened almost to a point: every tick label would land on
		// the same spot. Keep the edge and name, skip the ticks.
		return segs, labels
	}
	maxChars := 1
	for k := 0; k < n; k++ {
		if c := len(vals[k]); c > maxChars {
			maxChars = c
		}
	}
	minSpacing := float64(maxChars)*axisCharW + 8
	step := axisLabelStep(n, L, minSpacing)

	appendTick := func(k int) {
		p := pointAt(k)
		sx, sy, _ := m.projectOriginal(p[0], p[1], p[2])
		bx, by := sx+ox, sy+oy // tick base sits on the lifted edge line
		segs = append(segs, axisSeg{bx, by, bx + nx*axisTickLen, by + ny*axisTickLen, col})
		labels = append(labels, axisLabel{vals[k], bx + nx*axisLabelPad, by + ny*axisLabelPad, col})
	}

	// The last tick is always labeled so the axis' full extent is annotated,
	// but with a thinning step > 1 it can land under a label width from the
	// final stepped tick and overprint it ("70006500"). Drop that stepped
	// neighbor instead of the extent label when they'd crowd.
	last := n - 1
	prev := last / step * step
	skipPrev := last != prev && float64(last-prev)*(L/float64(n)) < minSpacing
	for k := 0; k < n; k += step {
		if k == prev && skipPrev {
			continue
		}
		appendTick(k)
	}
	if last != prev {
		appendTick(last)
	}
	return segs, labels
}

// formatAxisLabels formats one tick label per value. Called once per axis:
// the tick values and precision are fixed at construction, so the strings are
// cached instead of re-formatted on every rotation frame.
func formatAxisLabels(data []float64, prec int) []string {
	out := make([]string, len(data))
	for i, v := range data {
		out[i] = strconv.FormatFloat(v, 'f', prec, 64)
	}
	return out
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

// axisLabelStep returns the index stride that keeps drawn labels at least
// minSpacing pixels apart along a projected edge of screen length L.
func axisLabelStep(n int, L, minSpacing float64) int {
	if n <= 1 || L <= 0 {
		return 1
	}
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
