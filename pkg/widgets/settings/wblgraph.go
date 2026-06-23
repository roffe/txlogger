package settings

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
)

// --- graph view (native fyne primitives) -----------------------------------

var (
	bgColor    = color.NRGBA{R: 24, G: 24, B: 28, A: 255}
	gridColor  = color.NRGBA{R: 60, G: 60, B: 68, A: 255}
	axisColor  = color.NRGBA{R: 140, G: 140, B: 150, A: 255}
	lineColor  = color.NRGBA{R: 80, G: 200, B: 120, A: 255}
	pointColor = color.NRGBA{R: 240, G: 200, B: 60, A: 255}
	pointEdge  = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
)

const (
	graphMargin    = 16
	pointSize      = 12
	minGraphWidth  = 360
	minGraphHeight = 280

	yMin = 0
	yMax = 1023
	zMin = 0.0
	zMax = 1.5
)

type graphView struct {
	widget.BaseWidget
	editor *WBLEditor
	r      *graphRenderer
}

func newGraphView(editor *WBLEditor) *graphView {
	g := &graphView{editor: editor}
	g.ExtendBaseWidget(g)
	return g
}

func (g *graphView) CreateRenderer() fyne.WidgetRenderer {
	r := &graphRenderer{g: g}
	r.bg = canvas.NewRectangle(bgColor)
	g.r = r
	r.rebuild()
	return r
}

type graphRenderer struct {
	g    *graphView
	size fyne.Size

	bg        *canvas.Rectangle
	gridLines []*canvas.Line
	axes      []*canvas.Line
	dataLines []*canvas.Line
	points    []*draggablePoint

	// cached mapping from last layout (used by drag math)
	x0, y0, x1, y1 float32
	minYv, maxYv   int
	minZv, maxZv   float64

	objects []fyne.CanvasObject
}

func (r *graphRenderer) rebuild() {
	if len(r.gridLines) == 0 {
		for range 6 {
			l := canvas.NewLine(gridColor)
			l.StrokeWidth = 1
			r.gridLines = append(r.gridLines, l)
		}
	}
	if len(r.axes) == 0 {
		for range 2 {
			l := canvas.NewLine(axisColor)
			l.StrokeWidth = 1
			r.axes = append(r.axes, l)
		}
	}

	rows := r.g.editor.rows
	wantLines := 0
	if n := len(rows); n > 1 {
		wantLines = n - 1
	}
	for len(r.dataLines) < wantLines {
		l := canvas.NewLine(lineColor)
		l.StrokeWidth = 2
		r.dataLines = append(r.dataLines, l)
	}
	r.dataLines = r.dataLines[:wantLines]

	for len(r.points) < len(rows) {
		p := newDraggablePoint(r.g)
		r.points = append(r.points, p)
	}
	r.points = r.points[:len(rows)]
	for i, row := range rows {
		r.points[i].row = row
	}

	// dataLines/points may have changed length; force Objects() to rebuild.
	r.objects = nil
}

func (r *graphRenderer) Layout(size fyne.Size) {
	r.size = size
	r.bg.Resize(size)
	r.layoutGraph()
}

func (r *graphRenderer) layoutGraph() {
	w := r.size.Width
	h := r.size.Height
	if w <= 0 || h <= 0 {
		return
	}

	r.x0 = float32(graphMargin)
	r.y0 = float32(graphMargin)
	r.x1 = w - graphMargin
	r.y1 = h - graphMargin

	for i := range 3 {
		gy := r.y0 + (r.y1-r.y0)*float32(i+1)/4
		gh := r.gridLines[i]
		gh.Position1 = fyne.NewPos(r.x0, gy)
		gh.Position2 = fyne.NewPos(r.x1, gy)
		gh.Refresh()

		gx := r.x0 + (r.x1-r.x0)*float32(i+1)/4
		gv := r.gridLines[3+i]
		gv.Position1 = fyne.NewPos(gx, r.y0)
		gv.Position2 = fyne.NewPos(gx, r.y1)
		gv.Refresh()
	}

	r.axes[0].Position1 = fyne.NewPos(r.x0, r.y1)
	r.axes[0].Position2 = fyne.NewPos(r.x1, r.y1)
	r.axes[0].Refresh()
	r.axes[1].Position1 = fyne.NewPos(r.x0, r.y0)
	r.axes[1].Position2 = fyne.NewPos(r.x0, r.y1)
	r.axes[1].Refresh()

	rows := r.g.editor.rows
	if len(rows) == 0 {
		r.minYv, r.maxYv = yMin, yMax
		r.minZv, r.maxZv = zMin, zMax
		return
	}

	minY, maxY := rows[0].y, rows[0].y
	minZ, maxZ := rows[0].z, rows[0].z
	for _, row := range rows {
		if row.y < minY {
			minY = row.y
		}
		if row.y > maxY {
			maxY = row.y
		}
		if row.z < minZ {
			minZ = row.z
		}
		if row.z > maxZ {
			maxZ = row.z
		}
	}
	// Enforce a minimum visible span so dragging stays responsive when
	// points are clustered and so the graph doesn't degenerate to a point.
	const minYSpan, minZSpan = 50, 0.2
	if maxY-minY < minYSpan {
		mid := (maxY + minY) / 2
		minY = common.Clamp(mid-minYSpan/2, yMin, yMax-minYSpan)
		maxY = minY + minYSpan
	}
	if maxZ-minZ < minZSpan {
		mid := (maxZ + minZ) / 2
		minZ = common.Clamp(mid-minZSpan/2, zMin, zMax-minZSpan)
		maxZ = minZ + minZSpan
	}
	r.minYv, r.maxYv = minY, maxY
	r.minZv, r.maxZv = minZ, maxZ

	type pt struct{ x, y float32 }
	pts := make([]pt, len(rows))
	for i, row := range rows {
		fx := float32(row.y-minY) / float32(maxY-minY)
		fy := float32((row.z - minZ) / (maxZ - minZ))
		pts[i].x = r.x0 + fx*(r.x1-r.x0)
		pts[i].y = r.y1 - fy*(r.y1-r.y0)
	}

	for i := 1; i < len(pts); i++ {
		l := r.dataLines[i-1]
		l.Position1 = fyne.NewPos(pts[i-1].x, pts[i-1].y)
		l.Position2 = fyne.NewPos(pts[i].x, pts[i].y)
		l.Refresh()
	}

	half := float32(pointSize) / 2
	for i, p := range pts {
		dp := r.points[i]
		dp.Resize(fyne.NewSize(pointSize, pointSize))
		dp.Move(fyne.NewPos(p.x-half, p.y-half))
		dp.Refresh()
	}
}

func (r *graphRenderer) Refresh() {
	r.bg.FillColor = bgColor
	r.bg.Refresh()
	r.rebuild()
	r.layoutGraph()
	canvas.Refresh(r.g)
}

func (r *graphRenderer) Objects() []fyne.CanvasObject {
	if r.objects == nil {
		r.objects = make([]fyne.CanvasObject, 0, 1+len(r.gridLines)+len(r.axes)+len(r.dataLines)+len(r.points))
		r.objects = append(r.objects, r.bg)
		for _, l := range r.gridLines {
			r.objects = append(r.objects, l)
		}
		for _, l := range r.axes {
			r.objects = append(r.objects, l)
		}
		for _, l := range r.dataLines {
			r.objects = append(r.objects, l)
		}
		for _, p := range r.points {
			r.objects = append(r.objects, p)
		}
	}
	return r.objects
}

func (r *graphRenderer) MinSize() fyne.Size {
	return fyne.NewSize(minGraphWidth, minGraphHeight)
}

func (r *graphRenderer) Destroy() {}

// --- draggable point -------------------------------------------------------

type draggablePoint struct {
	widget.BaseWidget
	g    *graphView
	row  *mapRow
	accY float64 // fractional accumulator for integer y axis
}

func newDraggablePoint(g *graphView) *draggablePoint {
	p := &draggablePoint{g: g}
	p.ExtendBaseWidget(p)
	return p
}

func (p *draggablePoint) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(pointColor)
	rect.StrokeColor = pointEdge
	rect.StrokeWidth = 1
	return widget.NewSimpleRenderer(rect)
}

func (p *draggablePoint) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (p *draggablePoint) Dragged(e *fyne.DragEvent) {
	r := p.g.r
	if r == nil || p.row == nil {
		return
	}
	w := r.x1 - r.x0
	h := r.y1 - r.y0
	if w <= 0 || h <= 0 {
		return
	}

	// pixel delta → value delta using last layout's data range
	dY := float64(e.Dragged.DX) / float64(w) * float64(r.maxYv-r.minYv)
	dZ := -float64(e.Dragged.DY) / float64(h) * (r.maxZv - r.minZv)

	p.accY += dY
	step := int(p.accY)
	p.accY -= float64(step)

	p.row.y = common.Clamp(p.row.y+step, yMin, yMax)
	p.row.z = common.Clamp(p.row.z+dZ, zMin, zMax)

	p.g.editor.updateRowEntries(p.row)
	p.g.Refresh()
}

func (p *draggablePoint) DragEnd() {
	p.accY = 0
	p.g.editor.save()
}
