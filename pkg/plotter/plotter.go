package plotter

import (
	"image/color"
	"log"
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

	controls bool

	plotContainer *fyne.Container
	plotImages    []*canvas.Image

	texts  []*TappableText
	legend *fyne.Container
	cursor *canvas.Line
	zoom   *widget.Slider
	sel    *widget.Slider

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
}

type PlotterOpt func(*Plotter)

func WithControls(enabled bool) PlotterOpt {
	return func(p *Plotter) {
		p.controls = enabled
	}
}

func NewPlotter(values map[string][]float64, opts ...PlotterOpt) *Plotter {
	p := &Plotter{
		values:           values,
		dataPointsToShow: 0,
		controls:         true,
		semChan:          make(chan struct{}, runtime.NumCPU()),
	}

	for _, opt := range opts {
		opt(p)
	}

	p.ExtendBaseWidget(p)

	p.plotContainer = container.NewStack()
	p.legend = container.NewVBox()

	for k := range values {
		p.valueOrder = append(p.valueOrder, k)
	}
	sort.Strings(p.valueOrder)

	for _, k := range p.valueOrder {
		v, ok := values[k]
		if !ok {
			continue
		}
		dl := len(v)
		if dl > p.dataLength {
			p.dataLength = dl - 1
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
			//log.Println("onColorUpdate", col)
			r, g, b, a := col.RGBA()
			ts.Color = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
			cimg.Image = ts.Plot(p.values, p.start, p.dataPointsToShow, cimg.Image.Bounds().Dx(), cimg.Image.Bounds().Dy())
			cimg.Refresh()
		}

		text := NewTappableText(k, ts.Color, onTapped, onColorUpdate)
		text.SetTextSize(11)
		p.texts = append(p.texts, text)
		p.legend.Add(text)

	}
	maxZoom := min(float64(p.dataLength-25), 6000)

	p.dataPointsToShow = int(max(maxZoom/10, 250.0))

	p.sel = widget.NewSlider(0, float64(p.dataLength))
	p.sel.Max = float64(p.dataLength - p.dataPointsToShow)
	p.sel.OnChanged = p.onSelect

	p.zoom = widget.NewSlider(25, maxZoom)
	p.zoom.SetValue(float64(p.dataPointsToShow))
	p.zoom.Orientation = widget.Vertical
	p.zoom.OnChanged = p.onZoom

	p.cursor = canvas.NewLine(color.White)
	//p.cursor.Position1 = fyne.NewPos(0, 0)
	//p.cursor.Position2 = fyne.NewPos(0, 600)

	var br fyne.CanvasObject
	legend := layout.NewFixedWidth(
		180,
		p.legend,
	)

	split := container.NewHSplit(
		p.plotContainer,
		container.NewVScroll(legend),
	)
	split.Offset = 0.9

	if p.controls {
		br = container.NewBorder(
			nil,
			p.sel,
			p.zoom,
			nil,
			split,
		)
	} else {
		br = split
	}

	p.container = container.NewStack(
		br,
		p.cursor,
	)
	return p
}

func (p *Plotter) Next() {
	p.sel.SetValue(p.sel.Value + 1)
}

func (p *Plotter) SetValues(values map[string]float64) {
	for k, v := range values {
		if _, ok := p.values[k]; !ok {
			log.Println("Unknown value", k)
			continue
		}
		for i, name := range p.valueOrder {
			if name == k {
				if p.ts[i].Min > v {
					p.ts[i].Min = v
				}
				if p.ts[i].Max < v {
					p.ts[i].Max = v
				}
				continue
			}
		}
		p.values[k] = append(p.values[k], v)
	}
	p.dataLength++
	p.RefreshImages(p.plotContainer.Size())
}

func (p *Plotter) Seek(pos int) {
	pos++
	widthFactor := p.plotContainer.Size().Width / float32(p.dataPointsToShow)
	halfDataPointsToShow := p.dataPointsToShow / 2
	pp := float64(pos - halfDataPointsToShow)
	if pp < 0 {
		// if we are at the start of the data advance the cursor until we reach the middle of the screen
		p.cursor.Move(fyne.NewPos(p.zoom.Size().Width+float32(pos)*widthFactor, 0))
		p.sel.SetValue(pp)
	} else if pos > p.dataLength-halfDataPointsToShow {
		// if we are at the end of the data move the cursor to the end of the screen by calculating
		// position based on the data length and the width of the screen
		cursorPos := p.zoom.Size().Width + p.plotContainer.Size().Width - float32(p.dataLength-pos)*widthFactor
		p.cursor.Move(fyne.NewPos(cursorPos, 0))
		p.sel.Value = pp
		p.sel.Refresh()

	} else {
		// if we are in between the start and end of the data draw it in the middle of the screen
		p.cursor.Move(fyne.NewPos(p.plotContainer.Position().X+p.plotContainer.Size().Width/2, 0))
		p.sel.SetValue(pp)
	}
	p.updateLegend(float64(pos))
}

func (p *Plotter) onSelect(pos float64) {
	p.start = int(pos)
	p.RefreshImages(p.plotContainer.Size())
}

func (p *Plotter) updateLegend(pos float64) {
	for i, v := range p.valueOrder {
		valueIndex := min(p.dataLength, int(pos))
		obj := p.texts[i]
		obj.text.Text = v + ": " + strconv.FormatFloat(p.values[v][valueIndex], 'f', 2, 64)
		//obj.text.Refresh()
	}
	p.legend.Refresh()

}

func (p *Plotter) RefreshImages(size fyne.Size) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var wg sync.WaitGroup
	for n, obj := range p.plotImages {
		if !p.texts[n].enabled {
			continue
		}
		p.semChan <- struct{}{}
		wg.Add(1)
		go func(obj *canvas.Image) {
			defer wg.Done()
			defer func() {
				<-p.semChan
			}()
			obj.Image = p.ts[n].Plot(p.values, p.start, p.dataPointsToShow, int(size.Width/2), int(size.Height/2))
			obj.Refresh()
		}(obj)
	}
	//p.plotContainer.Refresh()
	wg.Wait()
	//	log.Println("RefreshImages took", time.Since(startx))
}

func (p *Plotter) CreateRenderer() fyne.WidgetRenderer {
	return &plotterRenderer{plotter: p}
}

type plotterRenderer struct {
	plotter *Plotter
	size    fyne.Size
}

func (r *plotterRenderer) MinSize() fyne.Size {
	return fyne.NewSize(500, 200)
}

func (r *plotterRenderer) Destroy() {}

func (r *plotterRenderer) Refresh() {}

func (r *plotterRenderer) Layout(size fyne.Size) {
	//if r.size.Width == size.Width && r.size.Height == size.Height {
	//	return
	//}
	r.size = size
	log.Println("plotter.Layout", size.Width, size.Height)
	r.plotter.container.Resize(size)
	pl := r.plotter.plotContainer.Size()
	r.plotter.cursor.Position2 = fyne.NewPos(r.plotter.cursor.Position1.X, pl.Height)
	r.plotter.cursor.Refresh()
	r.plotter.RefreshImages(pl)
}

func (r *plotterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.plotter.container}
}
