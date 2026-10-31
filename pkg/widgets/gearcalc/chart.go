package gearcalc

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

const (
	rpmMin, rpmMax = 600, 8000
	tickTextSize   = 12
	yGridPool      = 16
	maxYTicks      = 8
)

// same categorical palette as estimatedoutput so charts read as one tool
var seriesColors = [6]color.RGBA{
	{0x2a, 0x78, 0xd6, 0xff}, // blue
	{0xeb, 0x68, 0x34, 0xff}, // orange
	{0x1b, 0xaf, 0x7a, 0xff}, // aqua
	{0xed, 0xa1, 0x00, 0xff}, // yellow
	{0xe8, 0x7b, 0xa4, 0xff}, // magenta
	{0x00, 0x83, 0x00, 0xff}, // green
}

var (
	chartPlotBg    = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
	chartGridColor = color.RGBA{0xe1, 0xe0, 0xd9, 0xff}
	// same cursor color as graph2d/estimatedoutput
	chartCursorColor = color.NRGBA{165, 55, 253, 180}
	// tooltip has its own solid background so fixed ink works in both themes
	tipBgColor     = color.RGBA{0xff, 0xff, 0xff, 0xff}
	tipBorderColor = color.RGBA{0x99, 0x99, 0x99, 0xff}
	tipInkColor    = color.RGBA{0x0b, 0x0b, 0x0b, 0xff}
)

// gearLine is one gear: speed is linear in rpm so a single line segment
// from rpmMin to rpmMax plots it exactly.
type gearLine struct {
	name string
	k    float64 // km/h per engine rpm
	col  color.Color
}

var _ desktop.Hoverable = (*chart)(nil)

type chart struct {
	widget.BaseWidget
	lines    []gearLine
	hoverRPM float64 // <0 when the pointer is outside the chart
	hoverPos fyne.Position
	renderer *chartRenderer
}

func newChart() *chart {
	c := &chart{hoverRPM: -1}
	c.ExtendBaseWidget(c)
	return c
}

func (c *chart) MouseIn(e *desktop.MouseEvent) { c.MouseMoved(e) }

func (c *chart) MouseMoved(e *desktop.MouseEvent) {
	if c.renderer == nil {
		return
	}
	c.hoverPos = e.Position
	c.hoverRPM = c.renderer.rpmAt(e.Position.X)
	c.renderer.updateHover()
}

func (c *chart) MouseOut() {
	c.hoverRPM = -1
	if c.renderer != nil {
		c.renderer.updateHover()
	}
}

func (c *chart) SetLines(lines []gearLine) {
	c.lines = lines
	c.Refresh()
}

func (c *chart) CreateRenderer() fyne.WidgetRenderer {
	r := &chartRenderer{c: c}
	r.plotBg = &canvas.Rectangle{FillColor: chartPlotBg}
	r.objects = append(r.objects, r.plotBg)
	for i := 0; i < yGridPool; i++ {
		line := &canvas.Line{StrokeColor: chartGridColor, StrokeWidth: 1}
		label := &canvas.Text{TextSize: tickTextSize}
		r.yGrid = append(r.yGrid, line)
		r.yLabels = append(r.yLabels, label)
		r.objects = append(r.objects, line, label)
	}
	for rpm := 1000; rpm <= rpmMax; rpm += 1000 {
		line := &canvas.Line{StrokeColor: chartGridColor, StrokeWidth: 1}
		label := &canvas.Text{Text: strconv.Itoa(rpm), TextSize: tickTextSize}
		r.xGrid = append(r.xGrid, line)
		r.xLabels = append(r.xLabels, label)
		r.objects = append(r.objects, line, label)
	}
	r.cursor = &canvas.Line{StrokeColor: chartCursorColor, StrokeWidth: 3}
	r.cursor.Hide()
	r.objects = append(r.objects, r.cursor)
	for i := 0; i < len(seriesColors); i++ {
		line := &canvas.Line{StrokeWidth: 2}
		label := &canvas.Text{TextSize: tickTextSize}
		r.gearLines = append(r.gearLines, line)
		r.gearLabels = append(r.gearLabels, label)
		r.objects = append(r.objects, line, label)
	}
	r.yTitle = &canvas.Text{Text: "km/h", TextSize: tickTextSize}
	r.xTitle = &canvas.Text{Text: "RPM", TextSize: tickTextSize}
	r.objects = append(r.objects, r.yTitle, r.xTitle)
	r.tipBg = &canvas.Rectangle{FillColor: tipBgColor, StrokeColor: tipBorderColor, StrokeWidth: 1}
	r.tipBg.Hide()
	r.objects = append(r.objects, r.tipBg)
	for i := 0; i < 1+len(seriesColors); i++ {
		t := &canvas.Text{TextSize: tickTextSize}
		t.Hide()
		r.tipTexts = append(r.tipTexts, t)
		r.objects = append(r.objects, t)
	}
	c.renderer = r
	return r
}

var _ fyne.WidgetRenderer = (*chartRenderer)(nil)

type chartRenderer struct {
	c *chart

	plotBg     *canvas.Rectangle
	yGrid      []*canvas.Line
	yLabels    []*canvas.Text
	xGrid      []*canvas.Line
	xLabels    []*canvas.Text
	gearLines  []*canvas.Line
	gearLabels []*canvas.Text
	yTitle     *canvas.Text
	xTitle     *canvas.Text
	cursor     *canvas.Line
	tipBg      *canvas.Rectangle
	tipTexts   []*canvas.Text

	objects []fyne.CanvasObject
	size    fyne.Size

	plotLeft, plotTop, plotBottom, plotW float32
}

func (r *chartRenderer) MinSize() fyne.Size           { return fyne.NewSize(380, 260) }
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
	r.relayout()
	for _, o := range r.objects {
		if o.Visible() {
			o.Refresh()
		}
	}
}

func (r *chartRenderer) relayout() {
	size := r.size
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	style := fyne.TextStyle{}
	fg := theme.Color(theme.ColorNameForeground)

	yMax := 1.0
	for _, l := range r.c.lines {
		yMax = math.Max(yMax, l.k*rpmMax)
	}
	step := niceStep(yMax, maxYTicks)
	yEnd := math.Ceil(yMax/step) * step
	ticks := int(math.Round(yEnd/step)) + 1
	if ticks > yGridPool {
		ticks = yGridPool
	}

	var maxYLabelW float32
	tickTexts := make([]string, ticks)
	for i := range tickTexts {
		tickTexts[i] = strconv.FormatFloat(float64(i)*step, 'f', 0, 64)
		if s := fyne.MeasureText(tickTexts[i], tickTextSize, style); s.Width > maxYLabelW {
			maxYLabelW = s.Width
		}
	}
	labelH := fyne.MeasureText("0", tickTextSize, style).Height
	gearLabelW := fyne.MeasureText("6th", tickTextSize, style).Width

	padLeft := maxYLabelW + 10
	padTop := labelH + 6      // room for the km/h title
	padBottom := 2*labelH + 8 // x tick labels plus the RPM title
	padRight := gearLabelW + 8
	plotW := size.Width - padLeft - padRight
	plotH := size.Height - padTop - padBottom
	if plotW < 10 || plotH < 10 {
		return
	}
	plotBottom := padTop + plotH

	xFor := func(rpm float64) float32 {
		return padLeft + float32((rpm-rpmMin)/(rpmMax-rpmMin))*plotW
	}
	yFor := func(v float64) float32 {
		return plotBottom - float32(v/yEnd)*plotH
	}

	r.plotBg.Move(fyne.NewPos(padLeft, padTop))
	r.plotBg.Resize(fyne.NewSize(plotW, plotH))

	for i := 0; i < yGridPool; i++ {
		if i >= ticks {
			r.yGrid[i].Hide()
			r.yLabels[i].Hide()
			continue
		}
		y := yFor(float64(i) * step)
		r.yGrid[i].Position1 = fyne.NewPos(padLeft, y)
		r.yGrid[i].Position2 = fyne.NewPos(padLeft+plotW, y)
		r.yGrid[i].Show()
		label := r.yLabels[i]
		label.Text = tickTexts[i]
		label.Color = fg
		s := fyne.MeasureText(label.Text, tickTextSize, style)
		label.Resize(s)
		label.Move(fyne.NewPos(padLeft-6-s.Width, y-s.Height/2))
		label.Show()
	}

	for i, line := range r.xGrid {
		rpm := float64((i + 1) * 1000)
		x := xFor(rpm)
		line.Position1 = fyne.NewPos(x, padTop)
		line.Position2 = fyne.NewPos(x, plotBottom)
		label := r.xLabels[i]
		label.Color = fg
		s := fyne.MeasureText(label.Text, tickTextSize, style)
		label.Resize(s)
		label.Move(fyne.NewPos(x-s.Width/2, plotBottom+2))
	}

	for i, line := range r.gearLines {
		if i >= len(r.c.lines) {
			line.Hide()
			r.gearLabels[i].Hide()
			continue
		}
		gl := r.c.lines[i]
		line.StrokeColor = gl.col
		line.Position1 = fyne.NewPos(xFor(rpmMin), yFor(gl.k*rpmMin))
		line.Position2 = fyne.NewPos(xFor(rpmMax), yFor(gl.k*rpmMax))
		line.Show()
		label := r.gearLabels[i]
		label.Text = gl.name
		label.Color = gl.col
		s := fyne.MeasureText(label.Text, tickTextSize, style)
		label.Resize(s)
		label.Move(fyne.NewPos(padLeft+plotW+4, yFor(gl.k*rpmMax)-s.Height/2))
		label.Show()
	}

	r.yTitle.Color = fg
	r.yTitle.Resize(fyne.MeasureText(r.yTitle.Text, tickTextSize, style))
	r.yTitle.Move(fyne.NewPos(padLeft, 0))
	r.xTitle.Color = fg
	s := fyne.MeasureText(r.xTitle.Text, tickTextSize, style)
	r.xTitle.Resize(s)
	r.xTitle.Move(fyne.NewPos(padLeft+plotW-s.Width, plotBottom+2+labelH))

	r.plotLeft = padLeft
	r.plotTop = padTop
	r.plotBottom = plotBottom
	r.plotW = plotW
	r.updateHover()
}

// rpmAt maps an x coordinate to the rpm under the pointer, snapped to
// 100 rpm so the readout is stable while the mouse moves.
func (r *chartRenderer) rpmAt(x float32) float64 {
	if r.plotW <= 0 {
		return -1
	}
	rpm := rpmMin + float64((x-r.plotLeft)/r.plotW)*(rpmMax-rpmMin)
	rpm = math.Round(rpm/100) * 100
	return math.Min(math.Max(rpm, rpmMin), rpmMax)
}

func (r *chartRenderer) updateHover() {
	rpm := r.c.hoverRPM
	if rpm < 0 || len(r.c.lines) == 0 {
		r.cursor.Hide()
		r.tipBg.Hide()
		for _, t := range r.tipTexts {
			t.Hide()
		}
		r.cursor.Refresh()
		r.tipBg.Refresh()
		return
	}
	x := r.plotLeft + float32((rpm-rpmMin)/(rpmMax-rpmMin))*r.plotW
	r.cursor.Position1 = fyne.NewPos(x, r.plotTop)
	r.cursor.Position2 = fyne.NewPos(x, r.plotBottom)
	r.cursor.Show()

	style := fyne.TextStyle{}
	texts := []string{fmt.Sprintf("%.0f rpm", rpm)}
	cols := []color.Color{tipInkColor}
	for _, l := range r.c.lines {
		texts = append(texts, fmt.Sprintf("%s  %.1f km/h", l.name, l.k*rpm))
		cols = append(cols, l.col)
	}

	const pad = 6
	lineH := fyne.MeasureText("0", tickTextSize, style).Height + 2
	var maxW float32
	for _, t := range texts {
		if w := fyne.MeasureText(t, tickTextSize, style).Width; w > maxW {
			maxW = w
		}
	}
	boxW := maxW + 2*pad
	boxH := float32(len(texts))*lineH + 2*pad

	// place beside the pointer, flip/clamp to stay inside the widget
	pos := r.c.hoverPos.Add(fyne.NewPos(14, 10))
	if pos.X+boxW > r.size.Width {
		pos.X = r.c.hoverPos.X - boxW - 6
	}
	if pos.Y+boxH > r.size.Height {
		pos.Y = r.size.Height - boxH
	}
	if pos.X < 0 {
		pos.X = 0
	}
	if pos.Y < 0 {
		pos.Y = 0
	}

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
	r.cursor.Refresh()
}

// niceStep returns a 1/2/5*10^n step so the range is covered by at most
// maxTicks intervals (same helper as graph2d/estimatedoutput).
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
