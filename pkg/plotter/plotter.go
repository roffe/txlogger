package plotter

import (
	"image"
	"image/color"
	"log"
	"sort"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

type PlotterControl interface {
	Seek(int)
}

type Plotter struct {
	widget.BaseWidget

	cursor      *canvas.Line
	canvasImage *canvas.Image
	container   *container.Split
	overlayText *canvas.Text
	canvas      fyne.CanvasObject
	//canvasImageContainer *fyne.Container

	texts  []*TappableText
	legend *fyne.Container
	zoom   *widget.Slider

	ts               []*TimeSeries
	plotStartPos     int
	cursorPos        int
	values           map[string][]float64
	valueOrder       []string
	dataPointsToShow int
	dataLength       int

	Logplayer bool

	widthFactor          float32
	plotResolution       fyne.Size
	plotResolutionFactor float32

	textBuffer []byte

	size fyne.Size

	mu sync.Mutex

	hilightLine int
}

type PlotterOpt func(*Plotter)

func WithPlotResolutionFactor(factor float32) PlotterOpt {
	return func(p *Plotter) {
		p.plotResolutionFactor = factor
	}
}

func WithOrder(order []string) PlotterOpt {
	return func(p *Plotter) {
		p.valueOrder = order
	}
}

func NewPlotter(values map[string][]float64, opts ...PlotterOpt) *Plotter {
	p := &Plotter{
		values:               values,
		dataPointsToShow:     0,
		legend:               container.NewVBox(),
		zoom:                 widget.NewSlider(1, 100),
		canvasImage:          canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 400, 200))),
		cursor:               canvas.NewLine(color.White),
		ts:                   make([]*TimeSeries, len(values)),
		plotResolutionFactor: 1.0,
	}
	p.ExtendBaseWidget(p)

	p.canvasImage.FillMode = canvas.ImageFillStretch
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
		sort.Strings(p.valueOrder)
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
			p.RefreshImage()
		}

		onColorUpdate := func(col color.Color) {
			r, g, b, a := col.RGBA()
			p.ts[n].Color = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
			// log.Printf("\"%s\": {%d, %d, %d, %d},", k, uint8(r), uint8(g), uint8(b), uint8(a))
			p.RefreshImage()
		}
		//var oldColor color.RGBA
		onHover := func(hover bool) {
			if hover {
				//oldColor = p.ts[n].Color
				//p.ts[n].Color = color.RGBA{255, 0, 0, 255}
				p.overlayText.Text = k
				p.overlayText.Color = p.ts[n].Color
				p.hilightLine = n
				p.texts[n].text.TextStyle.Bold = true
				p.RefreshImage()
			} else {
				//p.ts[n].Color = oldColor
				p.texts[n].text.TextStyle.Bold = false
				p.overlayText.Text = ""
				p.hilightLine = -1
				p.RefreshImage()
			}
		}

		legendLabel := NewTappableText(k, p.ts[n].Color, onTapped, onColorUpdate, onHover)
		legendLabel.SetTextSize(14)
		p.texts = append(p.texts, legendLabel)
		p.legend.Add(legendLabel)

	}

	p.dataPointsToShow = min(p.dataLength, 250.0)

	canvasImage := container.New(&testL{p: p}, p.canvasImage)

	leading := container.NewBorder(
		nil,
		nil,
		p.zoom,
		nil,
		canvasImage,
	)
	p.container = container.NewHSplit(leading, container.NewVScroll(p.legend))
	p.container.Offset = 0.83

	p.overlayText = canvas.NewText("", color.White)
	p.overlayText.TextSize = 20

	p.canvas = container.NewWithoutLayout(
		p.container,
		p.overlayText,
		p.cursor,
	)
	return p
}

type testL struct {
	p       *Plotter
	oldSize fyne.Size
}

func (t *testL) Layout(_ []fyne.CanvasObject, plotSize fyne.Size) {
	if t.oldSize == plotSize {
		return
	}
	t.oldSize = plotSize

	t.p.overlayText.Move(fyne.NewPos(t.p.zoom.Size().Width, 20))

	t.p.canvasImage.Resize(plotSize) // Calculate new plot dimensions
	t.p.plotResolution = fyne.NewSize(plotSize.Width*t.p.plotResolutionFactor, plotSize.Height*t.p.plotResolutionFactor)
	// Update width factor based on the new size
	t.p.widthFactor = plotSize.Width / float32(t.p.dataPointsToShow)
	// Refresh the image and cursor
	t.p.RefreshImage()
	t.p.updateCursor()
}

func (t *testL) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 100)
}

//func (p *Plotter) Resize(size fyne.Size) {
//	log.Println("Resize", size)
//	p.r.Layout(size)
//}

func (p *Plotter) Seek(pos int) {
	halfDataPointsToShow := int(float64(p.dataPointsToShow) * .5)
	offsetPosition := float64(pos - halfDataPointsToShow)
	if pos <= p.dataLength-halfDataPointsToShow {
		if offsetPosition < 0 {
			offsetPosition = 0
		}
		p.plotStartPos = min(int(offsetPosition), p.dataLength)
	}
	p.cursorPos = pos
	p.updateLegend()
	p.updateCursor()
	p.RefreshImage()
}

func (p *Plotter) updateLegend() {
	for i, v := range p.valueOrder {
		valueIndex := min(p.dataLength, p.cursorPos)
		obj := p.texts[i]
		p.textBuffer = p.textBuffer[:0]
		p.textBuffer = append(p.textBuffer, v+" "...)
		p.textBuffer = strconv.AppendFloat(p.textBuffer, p.values[v][valueIndex], 'f', 2, 64)
		obj.text.Text = string(p.textBuffer)
		//obj.text.Text = v + ": " + strconv.FormatFloat(p.values[v][valueIndex], 'f', 2, 64)
		p.texts[i].Refresh()
	}

}

func (p *Plotter) RefreshImage() {
	p.mu.Lock()
	defer p.mu.Unlock()
	//log.Println("RefreshImage", p.plotResolution.Width, p.plotResolution.Height)
	img := image.NewRGBA(image.Rect(0, 0, int(p.plotResolution.Width), int(p.plotResolution.Height)))
	for n := range len(p.ts) {
		if !p.ts[n].Enabled {
			continue
		}
		if p.hilightLine == n {
			continue
		}
		p.ts[n].PlotImage(img, p.values, p.plotStartPos, p.dataPointsToShow, 1)
	}
	if p.hilightLine >= 0 && p.ts[p.hilightLine].Enabled {
		p.ts[p.hilightLine].PlotImage(img, p.values, p.plotStartPos, p.dataPointsToShow, 4)
		// write the text of the current value in the top left corner of the image
	}

	p.canvasImage.Image = img
	p.canvasImage.Refresh()
}

func (p *Plotter) Size() fyne.Size {
	return p.container.Size()
}

func (p *Plotter) Resize(size fyne.Size) {
	if p.size == size {
		return
	}
	p.size = size
	// Resize the container first
	p.container.Resize(size)
}

func (p *Plotter) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.canvas)
	//p.r = &plotterRenderer{p: p}
	//return p.r

}

type TimeSeries struct {
	Name       string
	Min        float64
	Max        float64
	valueRange float64
	Color      color.RGBA
	Enabled    bool
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

	switch name {
	case "m_Request", "MAF.m_AirInlet", "AirMassMast.m_Request":
		ts.Min, _ = findMinMaxFloat64(data)
		ts.Max = 2200
	case "ActualIn.p_AirInlet", "In.p_AirInlet", "ActualIn.p_AirBefThrottle", "In.p_AirBefThrottle":
		ts.Min = -1.0
		ts.Max = 3.0
	default:
		ts.Min, ts.Max = findMinMaxFloat64(data)
	}
	ts.valueRange = ts.Max - ts.Min

	return ts
}

func (ts *TimeSeries) PlotImage(img *image.RGBA, values map[string][]float64, start, numPoints, thickness int) {
	dl := len(values[ts.Name]) - 1
	startN, endN := min(max(start, 0), dl), min(start+numPoints, dl)
	s := img.Bounds().Size()
	w := s.X
	h := s.Y

	//log.Println("Plotting", ts.Name, "from", start, "to", numPoints, "width", w, "height", h)
	hh := h - 1
	dataLen := endN - startN
	heightFactor := float64(hh) / ts.valueRange
	widthFactor := float64(w) / float64(dataLen)

	// start at 1 since we need to draw a line from the previous point
	data := values[ts.Name][startN:endN]
	dle := dataLen - 1

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

func findMinMaxFloat64(data []float64) (float64, float64) {
	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// Updated cursor positioning method
func (p *Plotter) updateCursor() {
	var x float32
	halfDataPointsToShow := int(float64(p.dataPointsToShow) * .5)
	plotSize := p.canvasImage.Size()

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
	xOffset = min32(xOffset, plotSize.Width+p.zoom.Size().Width)
	xOffset = max32(xOffset, p.zoom.Size().Width)

	p.cursor.Position1 = fyne.NewPos(xOffset, 0)
	p.cursor.Position2 = fyne.NewPos(xOffset+1, plotSize.Height)
	p.cursor.Refresh()
}

// Helper functions
func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
