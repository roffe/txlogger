package plotter

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
)

// var _ fyne.Focusable = (*Plotter)(nil)
// var _ fyne.Tappable = (*Plotter)(nil)
var (
	_ fyne.Draggable = (*Plotter)(nil)
	_ fyne.Widget    = (*Plotter)(nil)
)

// plotBackend selects how the plot is drawn. The GPU shader is the default;
// TXLOGGER_PLOT_RENDERER=image selects the CPU rasterizer, which is also the
// automatic fallback when the log does not fit the shader's texture layout.
type PlotBackend int

const (
	PlotBackendImage PlotBackend = iota
	PlotBackendShader
)

type Plotter struct {
	widget.BaseWidget

	backend PlotBackend
	shader  *canvas.Shader
	// plotObj is the canvas object showing the plot: shader on the GPU
	// backend, canvasImage on the image backend.
	plotObj fyne.CanvasObject

	cursor      *canvas.Line
	canvasImage *canvas.Image
	split       *container.Split
	overlayText *canvas.Text

	legendTexts []*TappableText
	legend      *fyne.Container
	zoom        *widget.Slider

	ts               []*TimeSeries
	plotStartPos     int
	cursorPos        int
	values           map[string][]float64
	valueOrder       []string
	dataPointsToShow int
	dataLength       int

	widthFactor          float32
	plotResolution       fyne.Size
	plotResolutionFactor float32

	imgBuffers [2]*image.RGBA
	imgIndex   int

	// textBuffer []byte

	size fyne.Size

	hilightLine int

	// laneMode stacks each enabled series in its own horizontal band instead
	// of overlaying them all on the full plot height. laneOf holds the band
	// index per series, filled by updateLanes.
	laneMode bool
	laneOf   []int

	// seekPos is the latest position requested via Seek, which is called from
	// the playback goroutine; the pending UI refresh reads it back out.
	seekMu         sync.Mutex
	seekPos        int
	refreshPending atomic.Bool

	OnDragged func(event *fyne.DragEvent)
	OnTapped  func(event *fyne.PointEvent)
}

type PlotterOpt func(*Plotter)

func WithPlotResolutionFactor(factor float32) PlotterOpt {
	return func(p *Plotter) {
		p.plotResolutionFactor = factor
	}
}

func WithOnDragged(f func(event *fyne.DragEvent)) PlotterOpt {
	return func(p *Plotter) {
		p.OnDragged = f
	}
}

func WithOrder(order []string) PlotterOpt {
	return func(p *Plotter) {
		p.valueOrder = order
	}
}

func WithRenderer(renderer PlotBackend) PlotterOpt {
	return func(p *Plotter) {
		p.backend = renderer
	}
}

func NewPlotter(values map[string][]float64, opts ...PlotterOpt) *Plotter {
	p := &Plotter{
		values:               values,
		dataPointsToShow:     0,
		legend:               container.NewVBox(),
		zoom:                 widget.NewSlider(1, 400),
		canvasImage:          canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 400, 200))),
		cursor:               canvas.NewLine(color.White),
		ts:                   make([]*TimeSeries, len(values)),
		plotResolutionFactor: 1.0,
		hilightLine:          -1,
	}
	p.ExtendBaseWidget(p)

	p.canvasImage.FillMode = canvas.ImageFillContain
	p.canvasImage.ScaleMode = canvas.ImageScaleFastest

	p.zoom.Orientation = widget.Vertical
	p.zoom.Value = 10
	p.zoom.OnChanged = p.onZoom

	for _, opt := range opts {
		opt(p)
	}

	if len(p.valueOrder) == 0 {
		for k := range values {
			p.valueOrder = append(p.valueOrder, k)
		}
		sort.Slice(p.valueOrder, func(i, j int) bool { return lessCaseInsensitive(p.valueOrder[i], p.valueOrder[j]) })
	}

	for n, k := range p.valueOrder {
		v, ok := values[k]
		if !ok {
			continue
		}
		valuesLenght := len(v)
		if valuesLenght > p.dataLength {
			p.dataLength = valuesLenght - 1
		}

		p.ts[n] = NewTimeSeries(k, values)

		onTapped := func(enabled bool) {
			p.ts[n].Enabled = enabled
			p.refreshImage(false)
		}

		onColorUpdate := func(col color.Color) {
			r, g, b, a := col.RGBA()
			p.ts[n].Color = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
			// log.Printf("\"%s\": {%d, %d, %d, %d},", k, uint8(r), uint8(g), uint8(b), uint8(a))
			p.refreshImage(false)
		}
		// var oldColor color.RGBA
		onHover := func(hover bool) {
			if hover {
				// oldColor = p.ts[n].Color
				// p.ts[n].Color = color.RGBA{255, 0, 0, 255}
				p.overlayText.Text = k
				p.overlayText.Color = p.ts[n].Color
				p.hilightLine = n
				p.legendTexts[n].text.TextStyle.Bold = true
				p.legendTexts[n].value.TextStyle.Bold = true
				p.refreshImage(false)
			} else {
				// p.ts[n].Color = oldColor
				p.legendTexts[n].text.TextStyle.Bold = false
				p.legendTexts[n].value.TextStyle.Bold = false
				p.overlayText.Text = ""
				p.hilightLine = -1
				p.refreshImage(false)
			}
		}

		labelText := NewTappableText(k, p.ts[n].Color, onTapped, onColorUpdate, onHover)

		//labelText.precission = symbol.GetPrecision(symbol.GetCorrectionfactor(k))
		//if k == "Lambda.External" {
		//	labelText.precission = 3
		//}

		p.legendTexts = append(p.legendTexts, labelText)
		p.legend.Add(labelText)

	}

	p.dataPointsToShow = min(p.dataLength, 250.0)

	p.plotObj = p.canvasImage
	if p.backend == PlotBackendShader && p.initShader() {
		p.plotObj = p.shader
	}

	leading := container.NewBorder(
		nil,
		nil,
		p.zoom,
		nil,
		container.New(&plotLayout{p: p}, p.plotObj),
	)
	p.split = container.NewHSplit(
		leading,
		container.NewBorder(
			nil,
			container.NewVBox(
				widget.NewCheck("Lanes", func(on bool) {
					p.laneMode = on
					p.refreshImage(false)
				}),
				widget.NewButton("Toggle visible", func() {
					for _, ts := range p.legendTexts {
						ts.Tapped(&fyne.PointEvent{})
					}
				}),
			),
			nil,
			nil,
			container.NewVScroll(p.legend),
		),
	)
	p.split.Offset = 0.90

	p.overlayText = canvas.NewText("", color.White)
	p.overlayText.TextSize = 25

	return p
}

func lessCaseInsensitive(s, t string) bool {
	for {
		if len(t) == 0 {
			return false
		}
		if len(s) == 0 {
			return true
		}
		c, sizec := utf8.DecodeRuneInString(s)
		d, sized := utf8.DecodeRuneInString(t)

		lowerc := unicode.ToLower(c)
		lowerd := unicode.ToLower(d)

		if lowerc < lowerd {
			return true
		}
		if lowerc > lowerd {
			return false
		}

		s = s[sizec:]
		t = t[sized:]
	}
}

func (p *Plotter) CreateRenderer() fyne.WidgetRenderer {
	return &plotterRenderer{PL: p}
}

// Seek records the new playback position and schedules a redraw on the UI
// goroutine. Calls are coalesced: while a refresh is pending, further Seeks
// only overwrite seekPos and the pending refresh renders the latest position.
// This bounds drawing to the rate the UI goroutine can keep up with instead of
// the log record rate, so a fast log can never back up the event queue.
func (p *Plotter) Seek(pos int) {
	p.seekMu.Lock()
	p.seekPos = pos
	p.seekMu.Unlock()

	if !p.refreshPending.CompareAndSwap(false, true) {
		return
	}
	fyne.Do(func() {
		// Clear the flag before reading seekPos: a Seek arriving mid-draw then
		// queues a new refresh rather than being dropped, so the final
		// position is always rendered.
		p.refreshPending.Store(false)
		p.seekMu.Lock()
		pos := p.seekPos
		p.seekMu.Unlock()
		p.seekTo(pos)
	})
}

// seekTo applies a seek on the UI goroutine: it recomputes the view window,
// updates the legend values, redraws the plot and refreshes the changed
// canvas objects.
func (p *Plotter) seekTo(pos int) {
	halfDataPointsToShow := int(float64(p.dataPointsToShow) * .5)
	offsetPosition := pos - halfDataPointsToShow
	if pos <= p.dataLength-halfDataPointsToShow {
		if offsetPosition < 0 {
			offsetPosition = 0
		}
		p.plotStartPos = min(offsetPosition, p.dataLength)
	}
	p.cursorPos = pos

	valueIndex := min(p.dataLength, p.cursorPos)
	for i, v := range p.valueOrder {
		obj := p.legendTexts[i]
		newValue := fmt.Sprintf("%.4g", p.values[v][valueIndex])
		// newValue := strconv.FormatFloat(p.values[v][valueIndex], 'f', obj.precission, 64)
		if obj.value.Text == newValue {
			continue
		}
		obj.SetValue(newValue)
	}

	if p.backend == PlotBackendShader {
		// The view window moved; the GPU re-renders from two uniforms.
		p.updateShaderView()
		p.layoutCursor()
		p.cursor.Refresh()
		p.shader.Refresh()
		return
	}
	p.drawImage()
	p.layoutCursor()
	p.cursor.Refresh()
	p.canvasImage.Refresh()
}

// drawImage renders the enabled time series into the (reused) plot buffer and
// points canvasImage at it. It does not trigger a Refresh.
func (p *Plotter) drawImage() {
	w, h := int(p.plotResolution.Width), int(p.plotResolution.Height)
	if w <= 0 || h <= 0 {
		return
	}

	// Ping-pong between the two buffers; reuse if the resolution is unchanged,
	// otherwise (re)allocate for the new size.
	p.imgIndex ^= 1
	img := p.imgBuffers[p.imgIndex]
	if img == nil || img.Rect.Dx() != w || img.Rect.Dy() != h {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		p.imgBuffers[p.imgIndex] = img
	} else {
		clear(img.Pix) // reset to transparent before redrawing
	}

	laneCount := p.updateLanes()
	target := func(n int) *image.RGBA {
		if laneCount == 0 {
			return img
		}
		return laneImage(img, p.laneOf[n], laneCount)
	}

	for n := range len(p.ts) {
		if !p.ts[n].Enabled {
			continue
		}
		if p.hilightLine == n {
			continue
		}
		p.ts[n].PlotImage(target(n), p.values, p.plotStartPos, p.dataPointsToShow, 1)
	}
	if p.hilightLine >= 0 && p.ts[p.hilightLine].Enabled {
		p.ts[p.hilightLine].PlotImage(target(p.hilightLine), p.values, p.plotStartPos, p.dataPointsToShow, 4)
		// write the text of the current value in the top left corner of the image
	}

	p.canvasImage.Image = img
}

// updateLanes gives every enabled series its own horizontal band and returns
// the number of bands. 0 means overlay mode: all series share the full height.
func (p *Plotter) updateLanes() int {
	if len(p.laneOf) != len(p.ts) {
		p.laneOf = make([]int, len(p.ts))
	}
	if !p.laneMode {
		return 0
	}
	n := 0
	for i, ts := range p.ts {
		p.laneOf[i] = n
		if ts != nil && ts.Enabled {
			n++
		}
	}
	return n
}

// laneImage returns band i of n as a sub-image sharing img's pixels, so
// PlotImage keeps drawing in 0-based coordinates and is clipped to the band
// for free.
func laneImage(img *image.RGBA, i, n int) *image.RGBA {
	h := img.Rect.Dy()
	y0 := img.Rect.Min.Y + i*h/n
	y1 := img.Rect.Min.Y + (i+1)*h/n
	return img.SubImage(image.Rect(img.Rect.Min.X, y0, img.Rect.Max.X, y1)).(*image.RGBA)
}

func (p *Plotter) refreshImage(goroutine bool) {
	if p.backend == PlotBackendShader {
		// Legend toggles/recolors, hover and zoom all funnel through here;
		// the metadata texture is 4xN so rebuilding it unconditionally is
		// cheap, and the painter uploads it only because it is a new image.
		p.updateShaderMeta()
		p.updateShaderView()
		if goroutine {
			fyne.Do(p.shader.Refresh)
		} else {
			p.shader.Refresh()
		}
		return
	}
	p.drawImage()
	if goroutine {
		fyne.Do(p.canvasImage.Refresh)
	} else {
		p.canvasImage.Refresh()
	}
}

type TimeSeries struct {
	Name       string
	Min        float64
	Max        float64
	valueRange float64
	Color      color.RGBA
	Enabled    bool
	// Auto reports that the series has no known display range and should be
	// auto-ranged from its data by the caller (used by the live plotter).
	Auto bool
}

// defaultRange returns the fixed display range for the well-known symbols. ok is
// false for symbols without a preset range; the caller derives one from the data.
func defaultRange(name string) (min, max float64, ok bool) {
	switch name {
	case "Out.X_AccPedal", "Out.X_AccPos":
		return 0, 100, true
	case "ActualIn.T_Engine", "ActualIn.T_AirInlet":
		return -20, 120, true
	case "m_Request", "MAF.m_AirInlet", "AirMassMast.m_Request", "MAF.m_AirFromp_AirInlet":
		return 0, 2200, true
	case "ActualIn.p_AirInlet", "In.p_AirInlet", "ActualIn.p_AirBefThrottle", "In.p_AirBefThrottle":
		return -1.0, 3.0, true
	case "DisplProt.LambdaScanner", "Lambda.ADScanner", "LambdaScan.LambdaScanner", "LambdaScan.LambdaScanner2", "Lambda.External":
		return 0.5, 1.5, true
	case "IgnProt.fi_Offset":
		return -30, 10, true
	case "Lambda.LambdaInt":
		return -25, 25, true
	case "ECMStat.p_Diff":
		return -1, 2, true
	case "P_medel", "Max_tryck", "Regl_tryck":
		return -1, 3, true
	}
	return 0, 0, false
}

func NewTimeSeries(name string, values map[string][]float64) *TimeSeries {
	ts := &TimeSeries{
		Name:    name,
		Color:   colors.GetColor(name),
		Enabled: true,
	}

	data, ok := values[name]
	if !ok {
		log.Println("Time series", name, "not found")
		return ts
	}

	if min, max, known := defaultRange(name); known {
		ts.Min, ts.Max = min, max
	} else {
		ts.Min, ts.Max = common.FindMinMaxFloat64(data)
	}

	ts.valueRange = ts.Max - ts.Min

	return ts
}

// NewSeries builds a series with no data yet, for live plotting. Well-known
// symbols get their fixed display range; the rest are flagged Auto so the
// caller can range them from the live data via SetRange.
func NewSeries(name string) *TimeSeries {
	ts := &TimeSeries{
		Name:    name,
		Color:   colors.GetColor(name),
		Enabled: true,
	}
	if min, max, known := defaultRange(name); known {
		ts.SetRange(min, max)
	} else {
		ts.Auto = true
		ts.SetRange(0, 1)
	}
	return ts
}

// SetRange updates the display range used by PlotImage. valueRange is kept in
// sync so callers outside this package can re-range a series (e.g. auto-ranging
// a live signal each refresh).
func (ts *TimeSeries) SetRange(min, max float64) {
	ts.Min, ts.Max = min, max
	ts.valueRange = max - min
}

func (ts *TimeSeries) PlotImage(img *image.RGBA, values map[string][]float64, start, numPoints, thickness int) {
	dl := len(values[ts.Name]) - 1
	startN, endN := min(max(start, 0), dl), min(start+numPoints, dl)
	s := img.Bounds().Size()
	w := s.X
	h := s.Y

	// log.Println("Plotting", ts.Name, "from", start, "to", numPoints, "width", w, "height", h)
	hh := h - 1
	dataLen := endN - startN
	heightFactor := float64(hh) / ts.valueRange
	widthFactor := float64(w) / float64(dataLen)

	data := values[ts.Name][startN:endN]

	if dataLen > w {
		ts.plotImageDecimated(img, data, thickness)
		return
	}

	dle := dataLen - 1

	// start at 1 since we need to draw a line from the previous point
	for x := 1; x < dataLen; x++ {
		fx := float64(x)
		x0 := int(((fx - 1) * widthFactor))
		y0 := int(float64(hh) - (data[x-1]-ts.Min)*heightFactor)
		x1 := (int(fx * widthFactor))
		if x == dle {
			x1 = w
		}
		y1 := int(float64(hh) - (data[x]-ts.Min)*heightFactor)
		BresenhamThick(img, x0, y0, x1, y1, thickness, ts.Color)
	}
}

// plotImageDecimated renders the series when there are more visible points
// than pixel columns. Connecting consecutive points with Bresenham lines would
// overdraw each column once per point landing on it; instead each column gets
// a single vertical run spanning the min/max of its points, capping the work
// at O(width) regardless of how far out the view is zoomed.
func (ts *TimeSeries) plotImageDecimated(img *image.RGBA, data []float64, thickness int) {
	pix := img.Pix
	stride := img.Stride
	s := img.Bounds().Size()
	w := s.X
	h := s.Y
	hh := h - 1
	heightFactor := float64(hh) / ts.valueRange
	dataLen := len(data)

	halfThick := 0
	if thickness > 1 {
		halfThick = thickness / 2
	}

	lo := 0
	for x := 0; x < w; x++ {
		hi := (x + 1) * dataLen / w
		// Re-scan the previous column's last point so adjacent runs always
		// overlap vertically and the plot stays gap-free.
		scanFrom := max(lo-1, 0)
		minV, maxV := data[scanFrom], data[scanFrom]
		for _, v := range data[scanFrom+1 : hi] {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
		y0 := int(float64(hh) - (maxV-ts.Min)*heightFactor)
		y1 := int(float64(hh) - (minV-ts.Min)*heightFactor)
		for t := -halfThick; t <= halfThick; t++ {
			fillVRun(pix, stride, w, h, x+t, y0-halfThick, y1+halfThick, ts.Color)
		}
		lo = hi
	}
}

// layoutCursor recomputes the cursor line position for the current view. It
// does not trigger a Refresh.
func (p *Plotter) layoutCursor() {
	var x float32
	halfDataPointsToShow := int(float64(p.dataPointsToShow) * .5)
	plotSize := p.plotObj.Size()

	if p.cursorPos >= p.dataLength-halfDataPointsToShow {
		// Handle cursor position near the end of data
		x = float32(p.dataLength-p.cursorPos) * p.widthFactor
		x = plotSize.Width - x
	} else {
		// Calculate x position based on current view
		x = float32(p.cursorPos-max(p.plotStartPos, 0)) * p.widthFactor
	}

	// Account for zoom slider width and ensure cursor stays within plot bounds
	xOffset := p.zoom.Size().Width + x
	xOffset = min(xOffset, plotSize.Width+p.zoom.Size().Width)
	xOffset = max(xOffset, p.zoom.Size().Width)

	p.cursor.Position1 = fyne.NewPos(xOffset, 0)
	p.cursor.Position2 = fyne.NewPos(xOffset+1, plotSize.Height)
}

// Updated cursor positioning method
func (p *Plotter) updateCursor(goroutine bool) {
	p.layoutCursor()
	if goroutine {
		fyne.Do(p.cursor.Refresh)
	} else {
		p.cursor.Refresh()
	}
}
