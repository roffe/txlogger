package plotter

import (
	"image/color"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/layout"
)

type Plotter struct {
	widget.BaseWidget

	plotContainer *fyne.Container
	plotImages    []*canvas.Image

	texts  []*TappableText
	legend *fyne.Container
	zoom   *widget.Slider

	ts               []TimeSeries
	start            int
	values           map[string][]float64
	valueOrder       []string
	dataPointsToShow int
	dataLength       int

	container *fyne.Container

	Logplayer bool

	mu      sync.Mutex
	semChan chan struct{}

	plotMiddle  float32
	widthFactor float32
}

type PlotterOpt func(*Plotter)

func NewPlotter(values map[string][]float64, opts ...PlotterOpt) *Plotter {
	p := &Plotter{
		values:           values,
		dataPointsToShow: 0,
		semChan:          make(chan struct{}, runtime.NumCPU()),
		plotContainer:    container.NewStack(),
		legend:           container.NewVBox(),
		zoom:             widget.NewSlider(1, 100),
	}

	p.zoom.Orientation = widget.Vertical
	p.zoom.Value = 10
	p.zoom.OnChanged = p.onZoom

	for _, opt := range opts {
		opt(p)
	}

	p.ExtendBaseWidget(p)

	for k := range values {
		p.valueOrder = append(p.valueOrder, k)
	}
	sort.Strings(p.valueOrder)

	for _, k := range p.valueOrder {
		v, ok := values[k]
		if !ok {
			continue
		}
		valuesLenght := len(v)
		if valuesLenght > p.dataLength {
			p.dataLength = valuesLenght - 1
		}
		ts := NewTimeSeries(k, values)

		p.ts = append(p.ts, ts)

		cimg := canvas.NewImageFromImage(ts.Plot(values, 0, 0, 0, 0))
		cimg.FillMode = canvas.ImageFillOriginal
		cimg.ScaleMode = canvas.ImageScaleFastest
		p.plotContainer.Add(cimg)
		p.plotImages = append(p.plotImages, cimg)

		onTapped := func(enable bool) {
			if !enable {
				cimg.Hide()
				return
			}
			cimg.Image = ts.Plot(p.values, p.start, p.dataPointsToShow, cimg.Image.Bounds().Dx(), cimg.Image.Bounds().Dy())
			cimg.Refresh()
			cimg.Show()
		}

		onColorUpdate := func(col color.Color) {
			r, g, b, a := col.RGBA()
			ts.Color = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
			cimg.Image = ts.Plot(p.values, p.start, p.dataPointsToShow, cimg.Image.Bounds().Dx(), cimg.Image.Bounds().Dy())
			cimg.Refresh()
		}

		legendLabel := NewTappableText(k, ts.Color, onTapped, onColorUpdate)
		legendLabel.SetTextSize(11)
		p.texts = append(p.texts, legendLabel)
		p.legend.Add(legendLabel)

	}

	p.dataPointsToShow = min(p.dataLength, 250.0)

	legend := layout.NewFixedWidth(
		180,
		p.legend,
	)

	p.container = container.NewBorder(
		nil,
		nil,
		p.zoom,
		legend,
		p.plotContainer,
	)
	return p
}

func (p *Plotter) Seek(pos int) {
	halfDataPointsToShow := p.dataPointsToShow / 2
	offsetPosition := float64(pos - halfDataPointsToShow)
	if offsetPosition > 0 && pos < p.dataLength-halfDataPointsToShow {
		p.start = int(offsetPosition)
		p.RefreshImages(p.plotContainer.Size())
	}
	p.updateLegend(float64(pos + 1))
}

func (p *Plotter) updateLegend(pos float64) {
	for i, v := range p.valueOrder {
		valueIndex := min(p.dataLength, int(pos))
		obj := p.texts[i]
		obj.text.Text = v + ": " + strconv.FormatFloat(p.values[v][valueIndex], 'f', 2, 64)
	}
	p.legend.Refresh()

}

func (p *Plotter) RefreshImages(size fyne.Size) {
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
}

func (p *Plotter) CreateRenderer() fyne.WidgetRenderer {
	return &plotterRenderer{p: p}
}
