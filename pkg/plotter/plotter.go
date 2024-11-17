package plotter

import (
	"image"
	"image/color"
	"log"
	"sort"
	"strconv"

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
	container   *fyne.Container
	canvas      fyne.CanvasObject

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
		canvasImage:          canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 500, 200))),
		cursor:               canvas.NewLine(color.White),
		ts:                   make([]*TimeSeries, len(values)),
		plotResolutionFactor: 1.0,
	}

	p.canvasImage.FillMode = canvas.ImageFillOriginal
	p.canvasImage.ScaleMode = canvas.ImageScalePixels
	//p.canvasImage.Translucency = 0.2

	p.zoom.Orientation = widget.Vertical
	p.zoom.Value = 10
	p.zoom.OnChanged = p.onZoom

	for _, opt := range opts {
		opt(p)
	}

	p.ExtendBaseWidget(p)

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
			log.Printf("\"%s\": {%d, %d, %d, %d},", k, uint8(r), uint8(g), uint8(b), uint8(a))
			p.RefreshImage()
		}

		legendLabel := NewTappableText(k, p.ts[n].Color, onTapped, onColorUpdate)
		legendLabel.SetTextSize(11)
		p.texts = append(p.texts, legendLabel)
		p.legend.Add(legendLabel)

	}

	p.dataPointsToShow = min(p.dataLength, 250.0)

	p.container = container.NewBorder(
		nil,
		nil,
		p.zoom,
		container.NewVScroll(p.legend),
		p.canvasImage,
	)

	p.canvas = container.NewWithoutLayout(
		p.container,
		p.cursor,
	)
	return p
}

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

func (p *Plotter) updateCursor() {
	var x float32
	halfDataPointsToShow := int(float64(p.dataPointsToShow) * .5)
	if p.cursorPos >= p.dataLength-halfDataPointsToShow {
		// If cursor position is at or beyond the end of data
		x = float32(p.dataLength-p.cursorPos) * p.widthFactor
		x = p.canvasImage.Size().Width - x
	} else {
		// Calculate x position based on plotStartPos
		x = float32(p.cursorPos-max(p.plotStartPos, 0)) * p.widthFactor
	}

	xOffset := p.zoom.Size().Width + x
	p.cursor.Position1 = fyne.NewPos(xOffset, 0)
	p.cursor.Position2 = fyne.NewPos(xOffset+1, p.canvasImage.Size().Height)
	p.cursor.Refresh()
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
	}
	p.legend.Refresh()
}

func (p *Plotter) RefreshImage() {
	img := image.NewRGBA(image.Rect(0, 0, int(p.plotResolution.Width), int(p.plotResolution.Height)))

	for n := range len(p.ts) {
		if !p.ts[n].Enabled {
			continue
		}
		p.ts[n].PlotImage(img, p.values, p.plotStartPos, p.dataPointsToShow)
	}
	p.canvasImage.Image = img
	p.canvasImage.Refresh()
}

func (p *Plotter) CreateRenderer() fyne.WidgetRenderer {
	return &plotterRenderer{p: p}
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

func (ts *TimeSeries) PlotImage(img *image.RGBA, values map[string][]float64, start, numPoints int) {
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
		Bresenham(img, x0, y0, x1, y1, ts.Color)
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
