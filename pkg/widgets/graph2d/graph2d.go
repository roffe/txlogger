// Package graph2d provides a T7Suite style 2D graph for one dimensional
// maps. The map values are plotted as a line with one marker per cell,
// value callouts above the markers and the axis values along the bottom.
package graph2d

import (
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
)

const (
	tickTextSize    = 12
	calloutTextSize = 11

	markerRadius = 4

	padRight = 12
	axisGapX = 6 // gap between the y tick labels and the plot area

	calloutPadX = 4
	calloutPadY = 2

	// pool size for y gridlines/labels/bands, must fit maxYTicks plus the
	// extra ticks added when the range is extended to nice boundaries
	gridPool = 16

	maxYTicks = 8
)

var (
	plotBgColor      = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	bandColor        = color.RGBA{0xEF, 0xED, 0xD8, 0xFF}
	gridLineColor    = color.RGBA{0xC9, 0xC9, 0xC9, 0xFF}
	plotLineColor    = color.RGBA{0xD8, 0x40, 0x28, 0xFF}
	markerStrokeCol  = color.RGBA{0x8A, 0x22, 0x12, 0xFF}
	calloutBorderCol = color.RGBA{0x99, 0x99, 0x99, 0xFF}
	calloutTextColor = color.RGBA{0x00, 0x00, 0x00, 0xFF}
	// same color as the mapviewer crosshair so the live cursor is
	// recognizable; NRGBA since the value is not alpha-premultiplied
	cursorColor = color.NRGBA{165, 55, 253, 180}
)

var (
	_ fyne.Widget       = (*Graph)(nil)
	_ desktop.Mouseable = (*Graph)(nil)
)

type Config struct {
	AxisData       []float64 // axis value per cell, shown along the x axis
	Values         []float64
	AxisPrecision  int
	ValuePrecision int
	AxisLabel      string // optional axis description shown below the x axis
	ColorblindMode colors.ColorBlindMode
}

type Graph struct {
	widget.BaseWidget

	cfg *Config

	axis   []float64
	values []float64

	zMin, zMax float64

	colorMode colors.ColorBlindMode

	cursorIdx  float64
	showCursor bool

	OnMouseDown func()

	renderer *graphRenderer
}

func New(cfg *Config) *Graph {
	g := &Graph{
		cfg:       cfg,
		axis:      cfg.AxisData,
		values:    cfg.Values,
		colorMode: cfg.ColorblindMode,
	}
	if len(g.axis) != len(g.values) {
		g.axis = make([]float64, len(g.values))
		for i := range g.axis {
			g.axis[i] = float64(i)
		}
	}
	g.zMin, g.zMax = common.FindMinMaxFloat64(g.values)
	g.ExtendBaseWidget(g)
	return g
}

// SetValues updates the plotted values. The number of values must match the
// map dimensions the graph was created with.
func (g *Graph) SetValues(min, max float64, values []float64) {
	if len(values) != len(g.values) {
		return
	}
	g.values = values
	g.zMin = min
	g.zMax = max
	g.Refresh()
}

func (g *Graph) SetColorBlindMode(mode colors.ColorBlindMode) {
	if g.colorMode != mode {
		g.colorMode = mode
		g.Refresh()
	}
}

// SetCursor positions the live cursor at the (fractional) cell index,
// mirroring the crosshair in the map above.
func (g *Graph) SetCursor(idx float64) {
	if idx < 0 {
		idx = 0
	} else if max := float64(len(g.values) - 1); idx > max {
		idx = max
	}
	g.cursorIdx = idx
	g.showCursor = true
	if g.renderer != nil {
		g.renderer.positionCursor()
		g.renderer.cursor.Refresh()
	}
}

func (g *Graph) MouseDown(_ *desktop.MouseEvent) {
	if g.OnMouseDown != nil {
		g.OnMouseDown()
	}
}

func (g *Graph) MouseUp(_ *desktop.MouseEvent) {}

func (g *Graph) CreateRenderer() fyne.WidgetRenderer {
	n := len(g.values)
	r := &graphRenderer{g: g}

	r.plotBg = &canvas.Rectangle{FillColor: plotBgColor}

	for i := 0; i < gridPool; i++ {
		band := &canvas.Rectangle{FillColor: bandColor}
		band.Hide()
		r.bands = append(r.bands, band)

		line := &canvas.Line{StrokeColor: gridLineColor, StrokeWidth: 1}
		line.Hide()
		r.gridLines = append(r.gridLines, line)

		label := &canvas.Text{TextSize: tickTextSize}
		label.Hide()
		r.yLabels = append(r.yLabels, label)
	}

	for i := 0; i < n-1; i++ {
		r.segments = append(r.segments, &canvas.Line{StrokeColor: plotLineColor, StrokeWidth: 2})
	}

	r.cursor = &canvas.Line{StrokeColor: cursorColor, StrokeWidth: 3}
	r.cursor.Hide()

	for i := 0; i < n; i++ {
		r.markers = append(r.markers, &canvas.Circle{StrokeColor: markerStrokeCol, StrokeWidth: 1.5})
		r.xLabels = append(r.xLabels, &canvas.Text{TextSize: tickTextSize})
		r.calloutBoxes = append(r.calloutBoxes, &canvas.Rectangle{
			FillColor:    plotBgColor,
			StrokeColor:  calloutBorderCol,
			StrokeWidth:  1,
			CornerRadius: 2,
		})
		r.calloutTexts = append(r.calloutTexts, &canvas.Text{
			TextSize:  calloutTextSize,
			Color:     calloutTextColor,
			Alignment: fyne.TextAlignCenter,
		})
	}

	if g.cfg.AxisLabel != "" {
		r.axisLabel = &canvas.Text{Text: g.cfg.AxisLabel, TextSize: tickTextSize}
	}

	// z-order: background, bands, gridlines, cursor, line, markers,
	// callouts, labels
	r.objects = append(r.objects, r.plotBg)
	for _, o := range r.bands {
		r.objects = append(r.objects, o)
	}
	for _, o := range r.gridLines {
		r.objects = append(r.objects, o)
	}
	r.objects = append(r.objects, r.cursor)
	for _, o := range r.segments {
		r.objects = append(r.objects, o)
	}
	for _, o := range r.markers {
		r.objects = append(r.objects, o)
	}
	for i := 0; i < n; i++ {
		r.objects = append(r.objects, r.calloutBoxes[i], r.calloutTexts[i])
	}
	for _, o := range r.yLabels {
		r.objects = append(r.objects, o)
	}
	for _, o := range r.xLabels {
		r.objects = append(r.objects, o)
	}
	if r.axisLabel != nil {
		r.objects = append(r.objects, r.axisLabel)
	}

	g.renderer = r
	return r
}

var _ fyne.WidgetRenderer = (*graphRenderer)(nil)

type graphRenderer struct {
	g *Graph

	plotBg *canvas.Rectangle

	bands     []*canvas.Rectangle
	gridLines []*canvas.Line
	yLabels   []*canvas.Text

	segments []*canvas.Line
	markers  []*canvas.Circle

	calloutBoxes []*canvas.Rectangle
	calloutTexts []*canvas.Text

	xLabels   []*canvas.Text
	axisLabel *canvas.Text

	cursor *canvas.Line

	objects []fyne.CanvasObject

	size fyne.Size

	// plot geometry, kept so the live cursor can be moved without a relayout
	plotTop, plotBottom float32
	plotLeft            float32
	xStep               float32
}

func (r *graphRenderer) Layout(size fyne.Size) {
	if size == r.size {
		return
	}
	r.size = size
	r.relayout()
	// the tick labels can change content with the available size so a plain
	// reposition is not enough
	r.refreshObjects()
}

func (r *graphRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 250)
}

func (r *graphRenderer) Refresh() {
	r.relayout()
	r.refreshObjects()
}

func (r *graphRenderer) Destroy() {}

func (r *graphRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *graphRenderer) refreshObjects() {
	for _, o := range r.objects {
		if !o.Visible() {
			continue
		}
		o.Refresh()
	}
}

func (r *graphRenderer) relayout() {
	g := r.g
	n := len(g.values)
	size := r.size
	if n == 0 || size.Width <= 0 || size.Height <= 0 {
		return
	}

	style := fyne.TextStyle{}

	// measure the value callouts so headroom can be reserved above the plot
	calloutWidths := make([]float32, n)
	var maxCalloutW, calloutH float32
	for i, v := range g.values {
		text := strconv.FormatFloat(v, 'f', g.cfg.ValuePrecision, 64)
		r.calloutTexts[i].Text = text
		s := fyne.MeasureText(text, calloutTextSize, style)
		calloutWidths[i] = s.Width + 2*calloutPadX
		if calloutWidths[i] > maxCalloutW {
			maxCalloutW = calloutWidths[i]
		}
		if h := s.Height + 2*calloutPadY; h > calloutH {
			calloutH = h
		}
	}

	padTop := 2*(calloutH+2) + markerRadius + 4

	xLabelH := fyne.MeasureText("0", tickTextSize, style).Height
	padBottom := xLabelH + 8
	if r.axisLabel != nil {
		padBottom += xLabelH + 2
	}

	plotH := size.Height - padTop - padBottom
	if plotH < 24 {
		plotH = 24
	}
	plotTop := padTop
	plotBottom := plotTop + plotH

	// y scale with "nice" tick steps, extended to tick boundaries
	maxTicks := int(plotH / 36)
	if maxTicks < 2 {
		maxTicks = 2
	} else if maxTicks > maxYTicks {
		maxTicks = maxYTicks
	}
	rng := g.zMax - g.zMin
	if rng <= 0 {
		rng = math.Abs(g.zMax)
		if rng == 0 {
			rng = 1
		}
	}
	step := niceStep(rng, maxTicks)
	yStart := math.Floor(g.zMin/step) * step
	yEnd := math.Ceil(g.zMax/step) * step
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
	padLeft := maxYLabelW + axisGapX + 4
	plotW := size.Width - padLeft - padRight
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
			r.bands[i].Hide()
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
		label.Move(fyne.NewPos(padLeft-axisGapX-s.Width, y-s.Height/2))
		label.Show()

		// alternating band between this gridline and the one above it
		band := r.bands[i]
		if i+1 < ticks && i%2 == 0 {
			yAbove := yFor(yStart + float64(i+1)*step)
			band.Move(fyne.NewPos(padLeft, yAbove))
			band.Resize(fyne.NewSize(plotW, y-yAbove))
			band.Show()
		} else {
			band.Hide()
		}
	}

	xStep := plotW / float32(n)
	cx := func(i int) float32 {
		return padLeft + (float32(i)+0.5)*xStep
	}

	cys := make([]float32, n)
	for i, v := range g.values {
		cys[i] = yFor(v)
	}

	for i := 0; i < n; i++ {
		marker := r.markers[i]
		marker.FillColor = colors.GetColorInterpolation(g.zMin, g.zMax, g.values[i], g.colorMode)
		marker.Position1 = fyne.NewPos(cx(i)-markerRadius, cys[i]-markerRadius)
		marker.Position2 = fyne.NewPos(cx(i)+markerRadius, cys[i]+markerRadius)
	}

	for i := 0; i < n-1; i++ {
		segment := r.segments[i]
		segment.Position1 = fyne.NewPos(cx(i), cys[i])
		segment.Position2 = fyne.NewPos(cx(i+1), cys[i+1])
	}

	// stagger the callouts on two levels when they would overlap and skip
	// some of them when even that is not enough
	levels := 1
	if maxCalloutW+4 > xStep {
		levels = 2
	}
	skip := 1
	if needed := maxCalloutW + 4; needed > xStep*float32(levels) {
		skip = int(math.Ceil(float64(needed / (xStep * float32(levels)))))
	}
	shown := 0
	for i := 0; i < n; i++ {
		box, text := r.calloutBoxes[i], r.calloutTexts[i]
		if i%skip != 0 {
			box.Hide()
			text.Hide()
			continue
		}
		level := 0
		if levels == 2 {
			level = shown % 2
		}
		shown++

		w := calloutWidths[i]
		bottom := cys[i] - markerRadius - 3 - float32(level)*(calloutH+2)
		x := cx(i) - w/2
		if x < padLeft+1 {
			x = padLeft + 1
		}
		if x+w > padLeft+plotW-1 {
			x = padLeft + plotW - 1 - w
		}
		box.Move(fyne.NewPos(x, bottom-calloutH))
		box.Resize(fyne.NewSize(w, calloutH))
		box.Show()
		text.Resize(fyne.NewSize(w, calloutH-2*calloutPadY))
		text.Move(fyne.NewPos(x, bottom-calloutH+calloutPadY))
		text.Show()
	}

	xLabelTexts := make([]string, n)
	var maxXLabelW float32
	for i, v := range g.axis {
		xLabelTexts[i] = strconv.FormatFloat(v, 'f', g.cfg.AxisPrecision, 64)
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

	if r.axisLabel != nil {
		r.axisLabel.Color = labelColor
		s := fyne.MeasureText(r.axisLabel.Text, tickTextSize, style)
		r.axisLabel.Resize(s)
		r.axisLabel.Move(fyne.NewPos(padLeft+(plotW-s.Width)/2, plotBottom+4+xLabelH+2))
	}

	r.plotTop = plotTop
	r.plotBottom = plotBottom
	r.plotLeft = padLeft
	r.xStep = xStep
	r.positionCursor()
}

func (r *graphRenderer) positionCursor() {
	if !r.g.showCursor {
		return
	}
	x := r.plotLeft + (float32(r.g.cursorIdx)+0.5)*r.xStep
	r.cursor.Position1 = fyne.NewPos(x, r.plotTop)
	r.cursor.Position2 = fyne.NewPos(x, r.plotBottom)
	if r.cursor.Hidden {
		r.cursor.Show()
	}
}

// niceStep returns a 1/2/5*10^n step so that the range is covered by at most
// maxTicks intervals.
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
