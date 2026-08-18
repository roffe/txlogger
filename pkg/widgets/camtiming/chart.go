package camtiming

import (
	"fmt"
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// chart is the small XY line plot both tabs draw with: the timing diagram
// puts valve lift against crank angle, the VE tab puts volumetric
// efficiency against rpm. ponytail: one chart, two callers — no plotting
// library, no zoom, no hover. Add those if a third caller shows up.

const (
	tickTextSize = 12
	maxYTicks    = 7
	tipLines     = 8 // header plus the most series either chart draws
)

var (
	plotBg       = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
	gridColor    = color.RGBA{0xe1, 0xe0, 0xd9, 0xff}
	intakeColor  = color.RGBA{0x2a, 0x78, 0xd6, 0xff}
	exhaustColor = color.RGBA{0xeb, 0x68, 0x34, 0xff}
	veColor      = color.RGBA{0x1b, 0xaf, 0x7a, 0xff}
	refColor     = color.RGBA{0x88, 0x88, 0x88, 0xff}
	markerColor  = color.RGBA{0xa5, 0x37, 0xfd, 0xff}
	bandColor    = color.NRGBA{0x2a, 0x78, 0xd6, 0x28}
	cursorColor  = color.NRGBA{165, 55, 253, 180}
	// the plot background and the tooltip are a fixed near-white in both
	// themes, so the ink on top of them has to be fixed too
	inkColor       = color.RGBA{0x0b, 0x0b, 0x0b, 0xff}
	tipBgColor     = color.RGBA{0xff, 0xff, 0xff, 0xff}
	tipBorderColor = color.RGBA{0x99, 0x99, 0x99, 0xff}
)

type xy struct{ x, y float64 }

type series struct {
	name string
	col  color.RGBA
	pts  []xy
	thin bool // reference/compare series: drawn hairline
}

// marker is a labelled vertical line, used for the cam centerlines.
type marker struct {
	x     float64
	col   color.RGBA
	label string
}

// band is a shaded x range, used for the valve overlap.
type band struct {
	x0, x1 float64
	label  string
}

type chart struct {
	widget.BaseWidget

	xTitle, yTitle string
	xMin, xMax     float64
	xStep          float64
	yMax           float64 // 0 = scale to the data
	yFloat         bool    // let the y axis start above 0 to fill the plot
	xUnit, yUnit   string  // for the hover readout
	yDec           int     // decimals in the hover readout
	xSnap          float64 // hovered x is rounded to this so it sits still

	dirty    bool    // series changed shape: the object pool must be rebuilt
	hoverX   float64 // NaN when the pointer is outside the plot
	hoverPos fyne.Position

	// plot geometry, written by whichever renderer last laid out. It
	// lives on the widget rather than the renderer so a pointer move can
	// be answered without reaching for a renderer that may not exist yet.
	plotLeft, plotTop, plotBottom, plotW float32

	series   []series
	markers  []marker
	bands    []band
	renderer *chartRenderer
}

func newChart(xTitle, yTitle string, xMin, xMax, xStep float64) *chart {
	c := &chart{xTitle: xTitle, yTitle: yTitle, xMin: xMin, xMax: xMax, xStep: xStep,
		xSnap: 1, hoverX: math.NaN()}
	c.ExtendBaseWidget(c)
	return c
}

var _ desktop.Hoverable = (*chart)(nil)

func (c *chart) MouseIn(e *desktop.MouseEvent) { c.MouseMoved(e) }

// ponytail: a pointer move refreshes the whole chart rather than poking
// the tooltip objects directly. It is a few hundred canvas objects on a
// chart that redraws nothing else, and going through Refresh is what
// guarantees the readout reaches the renderer the canvas is painting.
func (c *chart) MouseMoved(e *desktop.MouseEvent) {
	c.hoverPos = e.Position
	c.hoverX = c.xAt(e.Position.X)
	c.Refresh()
}

func (c *chart) MouseOut() {
	if math.IsNaN(c.hoverX) {
		return
	}
	c.hoverX = math.NaN()
	c.Refresh()
}

// xAt maps a pointer x to the data x under it, snapped so the readout
// does not jitter, and NaN outside the plot.
func (c *chart) xAt(px float32) float64 {
	if c.plotW <= 0 || px < c.plotLeft || px > c.plotLeft+c.plotW {
		return math.NaN()
	}
	x := c.xMin + float64((px-c.plotLeft)/c.plotW)*(c.xMax-c.xMin)
	if c.xSnap > 0 {
		x = math.Round(x/c.xSnap) * c.xSnap
	}
	return math.Min(math.Max(x, c.xMin), c.xMax)
}

func (c *chart) set(s []series, m []marker, b []band) {
	c.series, c.markers, c.bands = s, m, b
	c.dirty = true
	c.Refresh()
}

func (c *chart) CreateRenderer() fyne.WidgetRenderer {
	r := &chartRenderer{c: c}
	r.plotBg = &canvas.Rectangle{FillColor: plotBg}
	r.xTitleTxt = &canvas.Text{Text: c.xTitle, TextSize: tickTextSize}
	r.yTitleTxt = &canvas.Text{Text: c.yTitle, TextSize: tickTextSize}
	r.cursor = &canvas.Line{StrokeColor: cursorColor, StrokeWidth: 3}
	r.cursor.Hide()
	r.tipBg = &canvas.Rectangle{FillColor: tipBgColor, StrokeColor: tipBorderColor, StrokeWidth: 1}
	r.tipBg.Hide()
	for i := 0; i < tipLines; i++ {
		t := &canvas.Text{TextSize: tickTextSize, Color: inkColor}
		t.Hide()
		r.tipTexts = append(r.tipTexts, t)
	}
	r.rebuild()
	c.renderer = r
	return r
}

var _ fyne.WidgetRenderer = (*chartRenderer)(nil)

type chartRenderer struct {
	c *chart

	plotBg               *canvas.Rectangle
	bandRects            []*canvas.Rectangle
	bandLabels           []*canvas.Text
	grid                 []*canvas.Line
	gridLabels           []*canvas.Text
	markerLines          []*canvas.Line
	markerLabels         []*canvas.Text
	lines                [][]*canvas.Line
	names                []*canvas.Text
	xTitleTxt, yTitleTxt *canvas.Text

	cursor   *canvas.Line
	tipBg    *canvas.Rectangle
	tipTexts []*canvas.Text

	objects []fyne.CanvasObject
	size    fyne.Size
}

// rebuild recreates the per-point objects after the data shape changed.
func (r *chartRenderer) rebuild() {
	c := r.c

	r.bandRects = nil
	r.bandLabels = nil
	for range c.bands {
		r.bandRects = append(r.bandRects, &canvas.Rectangle{FillColor: bandColor})
		r.bandLabels = append(r.bandLabels, &canvas.Text{TextSize: tickTextSize, Color: inkColor})
	}
	r.grid = nil
	r.gridLabels = nil
	xTicks := int((c.xMax-c.xMin)/c.xStep) + 1
	for i := 0; i < xTicks+maxYTicks; i++ {
		r.grid = append(r.grid, &canvas.Line{StrokeColor: gridColor, StrokeWidth: 1})
		r.gridLabels = append(r.gridLabels, &canvas.Text{TextSize: tickTextSize})
	}
	r.markerLines = nil
	r.markerLabels = nil
	for range c.markers {
		r.markerLines = append(r.markerLines, &canvas.Line{StrokeWidth: 1})
		r.markerLabels = append(r.markerLabels, &canvas.Text{TextSize: tickTextSize})
	}
	r.lines = make([][]*canvas.Line, len(c.series))
	r.names = nil
	for si, s := range c.series {
		w := float32(2)
		if s.thin {
			w = 1
		}
		for i := 1; i < len(s.pts); i++ {
			r.lines[si] = append(r.lines[si], &canvas.Line{StrokeColor: s.col, StrokeWidth: w})
		}
		r.names = append(r.names, &canvas.Text{Text: s.name, TextSize: tickTextSize, Color: s.col})
	}

	r.objects = append(r.objects[:0], r.plotBg)
	for i := range r.bandRects {
		r.objects = append(r.objects, r.bandRects[i])
	}
	for i := range r.grid {
		r.objects = append(r.objects, r.grid[i], r.gridLabels[i])
	}
	for i := range r.markerLines {
		r.objects = append(r.objects, r.markerLines[i], r.markerLabels[i])
	}
	for si := range r.lines {
		for _, l := range r.lines[si] {
			r.objects = append(r.objects, l)
		}
	}
	for i := range r.names {
		r.objects = append(r.objects, r.names[i])
	}
	for i := range r.bandLabels {
		r.objects = append(r.objects, r.bandLabels[i])
	}
	r.objects = append(r.objects, r.xTitleTxt, r.yTitleTxt, r.cursor, r.tipBg)
	for _, t := range r.tipTexts {
		r.objects = append(r.objects, t)
	}
	// the objects are brand new and unpositioned: place them now rather
	// than waiting for a Layout that only comes when the widget resizes
	r.relayout()
}

func (r *chartRenderer) MinSize() fyne.Size           { return fyne.NewSize(420, 260) }
func (r *chartRenderer) Destroy()                     {}
func (r *chartRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chartRenderer) Layout(size fyne.Size) {
	if size == r.size {
		return
	}
	r.size = size
	r.relayout()
}

func (r *chartRenderer) Refresh() {
	if r.c.dirty {
		r.c.dirty = false
		r.rebuild() // rebuild lays out as well
	} else {
		r.relayout()
	}
	for _, o := range r.objects {
		if o.Visible() {
			o.Refresh()
		}
	}
}

func (r *chartRenderer) relayout() {
	c := r.c
	size := r.size
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	style := fyne.TextStyle{}
	fg := theme.Color(theme.ColorNameForeground)

	yMax, yMin := c.yMax, 0.0
	if yMax <= 0 {
		yMin = math.Inf(1)
		for _, s := range c.series {
			for _, p := range s.pts {
				yMax = math.Max(yMax, p.y)
				yMin = math.Min(yMin, p.y)
			}
		}
		if math.IsInf(yMin, 1) {
			yMin = 0
		}
	}
	if !c.yFloat {
		yMin = 0
	}
	if yMax <= yMin {
		yMax = yMin + 1
	}
	yStep := niceStep(yMax-yMin, maxYTicks)
	yStart := math.Floor(yMin/yStep) * yStep
	yEnd := math.Ceil(yMax/yStep) * yStep
	yTicks := int(math.Round((yEnd-yStart)/yStep)) + 1
	if yTicks > maxYTicks {
		yTicks = maxYTicks
	}
	dec := 0
	if yStep < 1 {
		dec = 1
	}

	labelH := fyne.MeasureText("0", tickTextSize, style).Height
	var maxYLabelW float32
	yTexts := make([]string, yTicks)
	for i := range yTexts {
		yTexts[i] = strconv.FormatFloat(yStart+float64(i)*yStep, 'f', dec, 64)
		if w := fyne.MeasureText(yTexts[i], tickTextSize, style).Width; w > maxYLabelW {
			maxYLabelW = w
		}
	}

	padLeft := maxYLabelW + 10
	padTop := labelH + 6
	padBottom := 2*labelH + 8
	padRight := float32(10)
	plotW := size.Width - padLeft - padRight
	plotH := size.Height - padTop - padBottom
	if plotW < 10 || plotH < 10 {
		return
	}
	plotBottom := padTop + plotH

	xFor := func(v float64) float32 {
		return padLeft + float32((v-c.xMin)/(c.xMax-c.xMin))*plotW
	}
	yFor := func(v float64) float32 {
		return plotBottom - float32((v-yStart)/(yEnd-yStart))*plotH
	}
	clampX := func(x float32) float32 {
		return float32(math.Max(float64(padLeft), math.Min(float64(padLeft+plotW), float64(x))))
	}
	place := func(t *canvas.Text, x, y float32, center bool) {
		s := fyne.MeasureText(t.Text, tickTextSize, style)
		t.Resize(s)
		if center {
			x -= s.Width / 2
		}
		t.Move(fyne.NewPos(x, y))
	}

	r.plotBg.Move(fyne.NewPos(padLeft, padTop))
	r.plotBg.Resize(fyne.NewSize(plotW, plotH))

	for i, b := range c.bands {
		x0, x1 := clampX(xFor(b.x0)), clampX(xFor(b.x1))
		rect := r.bandRects[i]
		if x1 <= x0 {
			rect.Hide()
			r.bandLabels[i].Hide()
			continue
		}
		rect.Move(fyne.NewPos(x0, padTop))
		rect.Resize(fyne.NewSize(x1-x0, plotH))
		rect.Show()
		lbl := r.bandLabels[i]
		lbl.Text = b.label
		place(lbl, (x0+x1)/2, plotBottom-labelH-2, true)
		lbl.Show()
	}

	gi := 0
	next := func() (*canvas.Line, *canvas.Text) {
		l, t := r.grid[gi], r.gridLabels[gi]
		gi++
		return l, t
	}
	for v := c.xMin; v <= c.xMax+0.001 && gi < len(r.grid); v += c.xStep {
		l, lbl := next()
		x := xFor(v)
		l.Position1 = fyne.NewPos(x, padTop)
		l.Position2 = fyne.NewPos(x, plotBottom)
		l.Show()
		lbl.Text = strconv.FormatFloat(v, 'f', 0, 64)
		lbl.Color = fg
		place(lbl, x, plotBottom+2, true)
		lbl.Show()
	}
	for i := 0; i < yTicks && gi < len(r.grid); i++ {
		l, lbl := next()
		y := yFor(yStart + float64(i)*yStep)
		l.Position1 = fyne.NewPos(padLeft, y)
		l.Position2 = fyne.NewPos(padLeft+plotW, y)
		l.Show()
		lbl.Text = yTexts[i]
		lbl.Color = fg
		s := fyne.MeasureText(lbl.Text, tickTextSize, style)
		lbl.Resize(s)
		lbl.Move(fyne.NewPos(padLeft-6-s.Width, y-s.Height/2))
		lbl.Show()
	}
	for ; gi < len(r.grid); gi++ {
		r.grid[gi].Hide()
		r.gridLabels[gi].Hide()
	}

	for i, m := range c.markers {
		x := xFor(m.x)
		l := r.markerLines[i]
		l.StrokeColor = m.col
		l.Position1 = fyne.NewPos(x, padTop)
		l.Position2 = fyne.NewPos(x, plotBottom)
		lbl := r.markerLabels[i]
		lbl.Text = m.label
		lbl.Color = m.col
		place(lbl, x, padTop+2, true)
	}

	for si, s := range c.series {
		var peak xy
		for i := 1; i < len(s.pts); i++ {
			l := r.lines[si][i-1]
			l.Position1 = fyne.NewPos(xFor(s.pts[i-1].x), yFor(s.pts[i-1].y))
			l.Position2 = fyne.NewPos(xFor(s.pts[i].x), yFor(s.pts[i].y))
			if s.pts[i].y > peak.y {
				peak = s.pts[i]
			}
		}
		lbl := r.names[si]
		if len(s.pts) == 0 {
			lbl.Hide()
			continue
		}
		// above the peak, unless that is where the marker labels live
		y := yFor(peak.y) - labelH - 2
		if y < padTop+labelH+2 {
			y = yFor(peak.y) + 4
		}
		place(lbl, clampX(xFor(peak.x)), y, true)
		lbl.Show()
	}

	r.xTitleTxt.Color = fg
	s := fyne.MeasureText(r.xTitleTxt.Text, tickTextSize, style)
	r.xTitleTxt.Resize(s)
	r.xTitleTxt.Move(fyne.NewPos(padLeft+plotW-s.Width, plotBottom+2+labelH))
	r.yTitleTxt.Color = fg
	place(r.yTitleTxt, padLeft, 0, false)

	c.plotLeft, c.plotTop, c.plotBottom, c.plotW = padLeft, padTop, plotBottom, plotW
	r.updateHover()
}

// valueAt interpolates a series at x. Outside the series it reports no
// value: on the timing diagram that means the valve is shut, on the VE
// curve it means the log never got there.
func valueAt(pts []xy, x float64) (float64, bool) {
	if len(pts) == 0 || x < pts[0].x || x > pts[len(pts)-1].x {
		return 0, false
	}
	for i := 1; i < len(pts); i++ {
		if x <= pts[i].x {
			p0, p1 := pts[i-1], pts[i]
			if p1.x == p0.x {
				return p1.y, true
			}
			return p0.y + (x-p0.x)/(p1.x-p0.x)*(p1.y-p0.y), true
		}
	}
	return pts[len(pts)-1].y, true
}

func (r *chartRenderer) updateHover() {
	c := r.c
	hide := func() {
		r.cursor.Hide()
		r.tipBg.Hide()
		r.cursor.Refresh()
		r.tipBg.Refresh()
		for _, t := range r.tipTexts {
			if t.Visible() {
				t.Hide()
				t.Refresh()
			}
		}
	}
	if math.IsNaN(c.hoverX) || len(c.series) == 0 || c.plotW <= 0 {
		hide()
		return
	}

	x := c.plotLeft + float32((c.hoverX-c.xMin)/(c.xMax-c.xMin))*c.plotW
	r.cursor.Position1 = fyne.NewPos(x, c.plotTop)
	r.cursor.Position2 = fyne.NewPos(x, c.plotBottom)
	r.cursor.Show()
	r.cursor.Refresh()

	sep := " "
	if c.xUnit == "°" { // degrees read wrong with a space before them
		sep = ""
	}
	texts := []string{fmt.Sprintf("%.0f%s%s", c.hoverX, sep, c.xUnit)}
	cols := []color.Color{inkColor}
	for _, s := range c.series {
		if len(texts) >= tipLines {
			break
		}
		val := "—"
		if v, ok := valueAt(s.pts, c.hoverX); ok {
			val = strconv.FormatFloat(v, 'f', c.yDec, 64) + " " + c.yUnit
		}
		texts = append(texts, s.name+"  "+val)
		cols = append(cols, s.col)
	}

	style := fyne.TextStyle{}
	const pad = 6
	lineH := fyne.MeasureText("0", tickTextSize, style).Height + 2
	var maxW float32
	for _, t := range texts {
		if w := fyne.MeasureText(t, tickTextSize, style).Width; w > maxW {
			maxW = w
		}
	}
	boxW, boxH := maxW+2*pad, float32(len(texts))*lineH+2*pad

	// beside the pointer, flipped or clamped to stay inside the widget
	pos := c.hoverPos.Add(fyne.NewPos(14, 10))
	if pos.X+boxW > r.size.Width {
		pos.X = c.hoverPos.X - boxW - 6
	}
	if pos.Y+boxH > r.size.Height {
		pos.Y = r.size.Height - boxH
	}
	pos.X = float32(math.Max(0, float64(pos.X)))
	pos.Y = float32(math.Max(0, float64(pos.Y)))

	r.tipBg.Move(pos)
	r.tipBg.Resize(fyne.NewSize(boxW, boxH))
	r.tipBg.Show()
	r.tipBg.Refresh()
	for i, t := range r.tipTexts {
		if i >= len(texts) {
			t.Hide()
			t.Refresh()
			continue
		}
		t.Text = texts[i]
		t.Color = cols[i]
		t.Resize(fyne.MeasureText(t.Text, tickTextSize, style))
		t.Move(pos.Add(fyne.NewPos(pad, pad+float32(i)*lineH)))
		t.Show()
		t.Refresh()
	}
}

// niceStep picks a 1/2/5·10ⁿ grid step giving at most maxTicks ticks.
func niceStep(rng float64, maxTicks int) float64 {
	if rng <= 0 || maxTicks < 2 {
		return 1
	}
	raw := rng / float64(maxTicks-1)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if raw <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}
