package estimatedoutput

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	tickTextSize    = 12
	readoutTextSize = 12
	valueTextSize   = 11
	markerRadius    = 4
	gridPool        = 16
	maxYTicks       = 8
)

// series colors: validated categorical palette, assigned by curve index so
// the same quantity keeps the same color across ECUs
var seriesColors = []color.RGBA{
	{0x2a, 0x78, 0xd6, 0xff}, // blue
	{0xeb, 0x68, 0x34, 0xff}, // orange
	{0x1b, 0xaf, 0x7a, 0xff}, // aqua
	{0xed, 0xa1, 0x00, 0xff}, // yellow
	{0xe8, 0x7b, 0xa4, 0xff}, // magenta
	{0x00, 0x83, 0x00, 0xff}, // green
	{0x4a, 0x3a, 0xa7, 0xff}, // violet
	{0xe3, 0x49, 0x48, 0xff}, // red
}

var (
	dynoPlotBg    = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
	dynoGridColor = color.RGBA{0xe1, 0xe0, 0xd9, 0xff}
	dynoInkColor  = color.RGBA{0x0b, 0x0b, 0x0b, 0xff}
	// same cursor color as graph2d/mapviewer so it reads as the same tool
	dynoCursorColor = color.NRGBA{165, 55, 253, 180}
)

type graphSeries struct {
	name    string
	col     color.RGBA
	values  []float64
	visible bool
}

var (
	_ fyne.Widget       = (*dynoGraph)(nil)
	_ desktop.Hoverable = (*dynoGraph)(nil)
)

// dynoGraph is a multi-series line graph over a shared RPM axis with a
// hover readout, in the style of graph2d.
type dynoGraph struct {
	widget.BaseWidget

	rpm      []float64
	series   []graphSeries
	hoverIdx int

	renderer *dynoRenderer
}

func newDynoGraph() *dynoGraph {
	g := &dynoGraph{hoverIdx: -1}
	g.ExtendBaseWidget(g)
	return g
}

func (g *dynoGraph) SetData(rpm []float64, series []graphSeries) {
	g.rpm = rpm
	g.series = series
	if g.renderer != nil {
		g.renderer.rebuild()
	}
	g.Refresh()
}

func (g *dynoGraph) SetSeriesVisible(name string, visible bool) {
	for i := range g.series {
		if g.series[i].name == name {
			g.series[i].visible = visible
		}
	}
	g.Refresh()
}

func (g *dynoGraph) MouseIn(e *desktop.MouseEvent) { g.MouseMoved(e) }

func (g *dynoGraph) MouseMoved(e *desktop.MouseEvent) {
	if g.renderer == nil || len(g.rpm) == 0 {
		return
	}
	if idx := g.renderer.indexAt(e.Position.X); idx != g.hoverIdx {
		g.hoverIdx = idx
		g.renderer.updateHover()
	}
}

func (g *dynoGraph) MouseOut() {
	g.hoverIdx = -1
	if g.renderer != nil {
		g.renderer.updateHover()
	}
}

func (g *dynoGraph) CreateRenderer() fyne.WidgetRenderer {
	r := &dynoRenderer{g: g}
	r.plotBg = &canvas.Rectangle{FillColor: dynoPlotBg}
	for i := 0; i < gridPool; i++ {
		line := &canvas.Line{StrokeColor: dynoGridColor, StrokeWidth: 1}
		line.Hide()
		r.gridLines = append(r.gridLines, line)
		label := &canvas.Text{TextSize: tickTextSize}
		label.Hide()
		r.yLabels = append(r.yLabels, label)
	}
	r.cursor = &canvas.Line{StrokeColor: dynoCursorColor, StrokeWidth: 3}
	r.cursor.Hide()
	r.readout = &canvas.Text{TextSize: readoutTextSize, Color: dynoInkColor}
	r.rebuild()
	g.renderer = r
	return r
}

var _ fyne.WidgetRenderer = (*dynoRenderer)(nil)

type dynoRenderer struct {
	g *dynoGraph

	plotBg    *canvas.Rectangle
	gridLines []*canvas.Line
	yLabels   []*canvas.Text

	lines       [][]*canvas.Line
	markers     [][]*canvas.Circle
	valueLabels [][]*canvas.Text
	xLabels     []*canvas.Text

	cursor  *canvas.Line
	readout *canvas.Text

	objects []fyne.CanvasObject
	size    fyne.Size

	plotLeft, plotTop, plotBottom float32
	xStep                         float32
}

// rebuild recreates the per-point canvas objects after the data shape
// changed and recomposes the z-ordered object list.
func (r *dynoRenderer) rebuild() {
	g := r.g
	n := len(g.rpm)

	r.lines = make([][]*canvas.Line, len(g.series))
	r.markers = make([][]*canvas.Circle, len(g.series))
	r.valueLabels = make([][]*canvas.Text, len(g.series))
	for si, s := range g.series {
		for i := 0; i < n-1; i++ {
			r.lines[si] = append(r.lines[si], &canvas.Line{StrokeColor: s.col, StrokeWidth: 2})
		}
		for i := 0; i < n; i++ {
			r.markers[si] = append(r.markers[si], &canvas.Circle{FillColor: s.col})
			r.valueLabels[si] = append(r.valueLabels[si], &canvas.Text{TextSize: valueTextSize, Color: dynoInkColor})
		}
	}
	r.xLabels = nil
	for i := 0; i < n; i++ {
		r.xLabels = append(r.xLabels, &canvas.Text{TextSize: tickTextSize})
	}

	r.objects = r.objects[:0]
	r.objects = append(r.objects, r.plotBg)
	for _, o := range r.gridLines {
		r.objects = append(r.objects, o)
	}
	r.objects = append(r.objects, r.cursor)
	for si := range r.lines {
		for _, o := range r.lines[si] {
			r.objects = append(r.objects, o)
		}
		for _, o := range r.markers[si] {
			r.objects = append(r.objects, o)
		}
	}
	for si := range r.valueLabels {
		for _, o := range r.valueLabels[si] {
			r.objects = append(r.objects, o)
		}
	}
	for _, o := range r.yLabels {
		r.objects = append(r.objects, o)
	}
	for _, o := range r.xLabels {
		r.objects = append(r.objects, o)
	}
	r.objects = append(r.objects, r.readout)
	r.size = fyne.Size{} // force relayout
}

func (r *dynoRenderer) MinSize() fyne.Size { return fyne.NewSize(420, 300) }

func (r *dynoRenderer) Destroy() {}

func (r *dynoRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *dynoRenderer) Layout(size fyne.Size) {
	if size == r.size {
		return
	}
	r.size = size
	r.relayout()
	r.refreshObjects()
}

func (r *dynoRenderer) Refresh() {
	r.relayout()
	r.refreshObjects()
}

func (r *dynoRenderer) refreshObjects() {
	for _, o := range r.objects {
		if o.Visible() {
			o.Refresh()
		}
	}
}

func (r *dynoRenderer) relayout() {
	g := r.g
	n := len(g.rpm)
	size := r.size
	if n == 0 || size.Width <= 0 || size.Height <= 0 {
		return
	}
	style := fyne.TextStyle{}

	// y range over visible series, anchored at 0
	yMin, yMax := 0.0, 1.0
	first := true
	for _, s := range g.series {
		if !s.visible {
			continue
		}
		for _, v := range s.values {
			if math.IsNaN(v) {
				continue
			}
			if first {
				yMax = v
				first = false
			}
			yMin = math.Min(yMin, v)
			yMax = math.Max(yMax, v)
		}
	}
	if yMax <= yMin {
		yMax = yMin + 1
	}

	xLabelH := fyne.MeasureText("0", tickTextSize, style).Height
	padTop := fyne.MeasureText("0", readoutTextSize, style).Height + 8
	padBottom := xLabelH + 8
	plotH := size.Height - padTop - padBottom
	if plotH < 24 {
		plotH = 24
	}
	plotTop := padTop
	plotBottom := plotTop + plotH

	maxTicks := int(plotH / 36)
	if maxTicks < 2 {
		maxTicks = 2
	} else if maxTicks > maxYTicks {
		maxTicks = maxYTicks
	}
	step := niceStep(yMax-yMin, maxTicks)
	yStart := math.Floor(yMin/step) * step
	yEnd := math.Ceil(yMax/step) * step
	if yEnd-yStart < step {
		yEnd = yStart + step
	}
	ticks := int(math.Round((yEnd-yStart)/step)) + 1
	if ticks > len(r.gridLines) {
		ticks = len(r.gridLines)
	}
	decimals := 0
	if e := math.Floor(math.Log10(step)); e < 0 {
		decimals = int(-e)
	}
	tickTexts := make([]string, ticks)
	var maxYLabelW float32
	for i := range tickTexts {
		tickTexts[i] = strconv.FormatFloat(yStart+float64(i)*step, 'f', decimals, 64)
		if s := fyne.MeasureText(tickTexts[i], tickTextSize, style); s.Width > maxYLabelW {
			maxYLabelW = s.Width
		}
	}
	padLeft := maxYLabelW + 10
	plotW := size.Width - padLeft - 12
	if plotW < 10 {
		plotW = 10
	}

	scale := plotH / float32(yEnd-yStart)
	yFor := func(v float64) float32 {
		return plotBottom - float32(v-yStart)*scale
	}
	labelColor := theme.Color(theme.ColorNameForeground)

	r.plotBg.Move(fyne.NewPos(padLeft, plotTop))
	r.plotBg.Resize(fyne.NewSize(plotW, plotH))

	for i := 0; i < len(r.gridLines); i++ {
		if i >= ticks {
			r.gridLines[i].Hide()
			r.yLabels[i].Hide()
			continue
		}
		y := yFor(yStart + float64(i)*step)
		line := r.gridLines[i]
		line.Position1 = fyne.NewPos(padLeft, y)
		line.Position2 = fyne.NewPos(padLeft+plotW, y)
		line.Show()

		label := r.yLabels[i]
		label.Text = tickTexts[i]
		label.Color = labelColor
		s := fyne.MeasureText(label.Text, tickTextSize, style)
		label.Resize(s)
		label.Move(fyne.NewPos(padLeft-6-s.Width, y-s.Height/2))
		label.Show()
	}

	xStep := plotW / float32(n)
	cx := func(i int) float32 {
		return padLeft + (float32(i)+0.5)*xStep
	}

	for si, s := range g.series {
		for i := 0; i < n && i < len(s.values); i++ {
			m := r.markers[si][i]
			if !s.visible || math.IsNaN(s.values[i]) {
				m.Hide()
			} else {
				y := yFor(s.values[i])
				m.Position1 = fyne.NewPos(cx(i)-markerRadius, y-markerRadius)
				m.Position2 = fyne.NewPos(cx(i)+markerRadius, y+markerRadius)
				m.Show()
			}
			if i >= n-1 || i+1 >= len(s.values) {
				continue
			}
			l := r.lines[si][i]
			if !s.visible || math.IsNaN(s.values[i]) || math.IsNaN(s.values[i+1]) {
				l.Hide()
			} else {
				l.Position1 = fyne.NewPos(cx(i), yFor(s.values[i]))
				l.Position2 = fyne.NewPos(cx(i+1), yFor(s.values[i+1]))
				l.Show()
			}
		}
	}

	// value callouts on each point: prefer above the marker, dodge below on
	// collision, hide when even that overlaps (hover still shows the value);
	// columns thin out when they get too narrow for the widest label
	valueTexts := make([][]string, len(g.series))
	var maxValW float32
	for si, s := range g.series {
		if !s.visible {
			continue
		}
		valueTexts[si] = make([]string, len(s.values))
		for i, v := range s.values {
			if math.IsNaN(v) {
				continue
			}
			valueTexts[si][i] = strconv.FormatFloat(v, 'f', 0, 64)
			if w := fyne.MeasureText(valueTexts[si][i], valueTextSize, style).Width; w > maxValW {
				maxValW = w
			}
		}
	}
	valueSkip := 1
	if maxValW+4 > xStep {
		valueSkip = int(math.Ceil(float64((maxValW + 4) / xStep)))
	}
	type rect struct{ x1, y1, x2, y2 float32 }
	overlapsAny := func(placed []rect, c rect) bool {
		for _, p := range placed {
			if c.x1 < p.x2 && p.x1 < c.x2 && c.y1 < p.y2 && p.y1 < c.y2 {
				return true
			}
		}
		return false
	}
	for i := 0; i < n; i++ {
		var placed []rect
		for si, s := range g.series {
			label := r.valueLabels[si][i]
			if !s.visible || i >= len(s.values) || i%valueSkip != 0 || math.IsNaN(s.values[i]) {
				label.Hide()
				continue
			}
			sz := fyne.MeasureText(valueTexts[si][i], valueTextSize, style)
			x := cx(i) - sz.Width/2
			if x < padLeft+1 {
				x = padLeft + 1
			} else if x+sz.Width > padLeft+plotW-1 {
				x = padLeft + plotW - 1 - sz.Width
			}
			y := yFor(s.values[i])
			top := y - markerRadius - 2 - sz.Height
			c := rect{x, top, x + sz.Width, top + sz.Height}
			if top < plotTop || overlapsAny(placed, c) {
				top = y + markerRadius + 2
				c = rect{x, top, x + sz.Width, top + sz.Height}
			}
			if top+sz.Height > plotBottom || overlapsAny(placed, c) {
				label.Hide()
				continue
			}
			placed = append(placed, c)
			label.Text = valueTexts[si][i]
			label.Resize(sz)
			label.Move(fyne.NewPos(x, top))
			label.Show()
		}
	}

	xLabelTexts := make([]string, n)
	var maxXLabelW float32
	for i, v := range g.rpm {
		xLabelTexts[i] = strconv.FormatFloat(v, 'f', 0, 64)
		if s := fyne.MeasureText(xLabelTexts[i], tickTextSize, style); s.Width > maxXLabelW {
			maxXLabelW = s.Width
		}
	}
	labelSkip := 1
	if maxXLabelW+6 > xStep {
		labelSkip = int(math.Ceil(float64((maxXLabelW + 6) / xStep)))
	}
	for i := 0; i < n; i++ {
		label := r.xLabels[i]
		if i%labelSkip != 0 {
			label.Hide()
			continue
		}
		label.Text = xLabelTexts[i]
		label.Color = labelColor
		s := fyne.MeasureText(label.Text, tickTextSize, style)
		label.Resize(s)
		x := cx(i) - s.Width/2
		if x < 0 {
			x = 0
		} else if x+s.Width > size.Width {
			x = size.Width - s.Width
		}
		label.Move(fyne.NewPos(x, plotBottom+4))
		label.Show()
	}

	r.readout.Move(fyne.NewPos(padLeft, 2))

	r.plotLeft = padLeft
	r.plotTop = plotTop
	r.plotBottom = plotBottom
	r.xStep = xStep
	r.updateHover()
}

func (r *dynoRenderer) indexAt(x float32) int {
	if r.xStep <= 0 {
		return -1
	}
	idx := int((x - r.plotLeft) / r.xStep)
	if idx < 0 || x < r.plotLeft {
		idx = 0
	}
	if idx > len(r.g.rpm)-1 {
		idx = len(r.g.rpm) - 1
	}
	return idx
}

func (r *dynoRenderer) updateHover() {
	idx := r.g.hoverIdx
	if idx < 0 || idx >= len(r.g.rpm) {
		r.cursor.Hide()
		r.readout.Text = ""
		r.cursor.Refresh()
		r.readout.Refresh()
		return
	}
	x := r.plotLeft + (float32(idx)+0.5)*r.xStep
	r.cursor.Position1 = fyne.NewPos(x, r.plotTop)
	r.cursor.Position2 = fyne.NewPos(x, r.plotBottom)
	r.cursor.Show()

	var sb strings.Builder
	fmt.Fprintf(&sb, "%.0f rpm", r.g.rpm[idx])
	for _, s := range r.g.series {
		if !s.visible || idx >= len(s.values) || math.IsNaN(s.values[idx]) {
			continue
		}
		fmt.Fprintf(&sb, "   %s %.0f", s.name, s.values[idx])
	}
	r.readout.Text = sb.String()
	r.readout.Color = theme.Color(theme.ColorNameForeground)
	r.cursor.Refresh()
	r.readout.Refresh()
}

// niceStep returns a 1/2/5*10^n step so the range is covered by at most
// maxTicks intervals (same helper as graph2d).
func niceStep(rng float64, maxTicks int) float64 {
	if maxTicks < 1 {
		maxTicks = 1
	}
	raw := rng / float64(maxTicks)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch norm := raw / mag; {
	case norm <= 1:
		return mag
	case norm <= 2:
		return 2 * mag
	case norm <= 5:
		return 5 * mag
	default:
		return 10 * mag
	}
}
