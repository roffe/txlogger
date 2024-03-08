package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type HBar struct {
	widget.BaseWidget
	face      *canvas.Rectangle
	bar       *canvas.Rectangle
	titleText *canvas.Text
	bars      []*canvas.Line

	cfg *HBarConfig

	value float64

	container *fyne.Container
}

type HBarConfig struct {
	Title    string
	Min, Max float64
	Steps    int
	Minsize  fyne.Size
}

func NewHBar(cfg *HBarConfig) *HBar {
	s := &HBar{
		cfg: cfg,
	}
	s.ExtendBaseWidget(s)
	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}
	s.container = s.render()
	return s
}

func (s *HBar) render() *fyne.Container {
	s.face = &canvas.Rectangle{StrokeColor: theme.DisabledColor(), StrokeWidth: 3}
	s.bar = &canvas.Rectangle{FillColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80}}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter
	bar := container.NewWithoutLayout(s.face)
	for i := 0; i < int(s.cfg.Steps+1); i++ {
		line := &canvas.Line{StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF}, StrokeWidth: 2}
		s.bars = append(s.bars, line)
		bar.Add(line)
	}
	bar.Add(s.bar)
	bar.Add(s.titleText)
	//bar.Objects = append(bar.Objects, s.bar, s.titleText)
	return bar
}

func (s *HBar) SetValue(value float64) {
	if value == s.value {
		return
	}
	if value > s.cfg.Max {
		value = s.cfg.Max
	}
	if value < s.cfg.Min {
		value = s.cfg.Min
	}
	s.value = value

	size := s.container.Size()

	widthFactor := float32(size.Width) / float32(s.cfg.Max)
	//	log.Println("Height:", size.Height, "heightFacor", heightFactor, "sfac", s.factor)

	if value >= s.cfg.Max*.75 && value < s.cfg.Max*.9 {
		s.bar.FillColor = color.RGBA{R: 0xFF, G: 0x98, B: 0x00, A: 0x90}
	} else if value >= s.cfg.Max*.9 {
		s.bar.FillColor = color.RGBA{R: 0xF4, G: 0x43, B: 0x36, A: 0x90}
	} else {
		s.bar.FillColor = color.RGBA{0x2C, 0xA5, 0x00, 0x90}
	}
	s.bar.Move(fyne.NewPos(0, size.Height/8))
	s.bar.Resize(fyne.NewSize((float32(value) * widthFactor), size.Height-(size.Height/8*2)))
	//s.bar.Refresh()
}

func (s *HBar) Value() float64 {
	return s.value
}

func (s *HBar) CreateRenderer() fyne.WidgetRenderer {
	return &HBarRenderer{
		d: s,
	}
}

type HBarRenderer struct {
	d *HBar
}

func (dr *HBarRenderer) Layout(space fyne.Size) {
	dr.d.container.Resize(space)
	s := dr.d
	diameter := space.Width
	height := space.Height
	middle := height / 2
	widthFactor := float32(diameter) / float32(s.cfg.Steps)
	s.face.Resize(space)
	s.titleText.Move(fyne.NewPos(diameter/2-s.titleText.Size().Width/2, height-30))
	s.bar.Move(fyne.NewPos(space.Width-float32(s.value), 0))
	for i, line := range s.bars {
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(float32(i)*widthFactor, middle-height/3)
			line.Position2 = fyne.NewPos(float32(i)*widthFactor, middle+height/3)
			continue
		}
		line.Position1 = fyne.NewPos(float32(i)*widthFactor, middle-height/7)
		line.Position2 = fyne.NewPos(float32(i)*widthFactor, middle+height/7)
	}
	s.SetValue(s.value)
}

func (dr *HBarRenderer) MinSize() fyne.Size {
	return dr.d.cfg.Minsize
}

func (dr *HBarRenderer) Refresh() {
}

func (dr *HBarRenderer) Destroy() {
}

func (dr *HBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{dr.d.container}
}
