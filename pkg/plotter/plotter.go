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

func (p *Plotter) updateCursor2() {
	x := float32(p.cursorPos-max(p.plotStartPos, 0)) * p.widthFactor
	xOffset := p.zoom.Size().Width + x
	p.cursor.Position1 = fyne.NewPos(xOffset, 0)
	p.cursor.Position2 = fyne.NewPos(xOffset+2, p.canvasImage.Size().Height)
	p.cursor.Refresh()
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
		obj.text.Text = v + ": " + strconv.FormatFloat(p.values[v][valueIndex], 'f', 2, 64)
	}
	p.legend.Refresh()
}

/* func (p *Plotter) RefreshImages(size fyne.Size) {
	p.mu.Lock()
	defer p.mu.Unlock()
	//startx := time.Now()
	var wg sync.WaitGroup
	for n, obj := range p.plotImages {
		if !p.texts[n].enabled {
			continue
		}
		p.semChan <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				<-p.semChan
			}()
			obj.Image = p.ts[n].Plot(p.values, p.start, p.dataPointsToShow, int(size.Width/2), int(size.Height/2))
		}()
	}
	wg.Wait()
	p.plotContainer.Refresh()
	//log.Println("RefreshImages took", time.Since(startx))
} */

func (p *Plotter) RefreshImage() {
	//p.mu.Lock()
	//defer p.mu.Unlock()
	//startx := time.Now()
	img := image.NewRGBA(image.Rect(0, 0, int(p.plotResolution.Width), int(p.plotResolution.Height)))
	for n := range len(p.ts) {
		if !p.ts[n].Enabled {
			continue
		}
		p.ts[n].PlotImage(img, p.values, p.plotStartPos, p.dataPointsToShow)
		//p.ts[n].PlotImage(p.canvasImage.Image.(*image.RGBA), p.values, p.startPos, p.dataPointsToShow)
	}
	p.canvasImage.Image = img
	p.canvasImage.Refresh()
	//log.Println("RefreshImage took", time.Since(startx).Nanoseconds())
}

func (p *Plotter) CreateRenderer() fyne.WidgetRenderer {
	return &plotterRenderer{p: p}
}
