package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

type HBar struct {
	face      *canvas.Rectangle
	bar       *canvas.Rectangle
	titleText *canvas.Text
	bars      []*canvas.Line

	cfg *HBarConfig

	value float64

	canvas fyne.CanvasObject
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
	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}
	s.canvas = s.render()
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
	bar.Objects = append(bar.Objects, s.bar, s.titleText)
	bar.Layout = s
	return bar
}

func (s *HBar) Layout(_ []fyne.CanvasObject, space fyne.Size) {
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

func (s *HBar) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return s.cfg.Minsize
}

func (s *HBar) Content() fyne.CanvasObject {
	return s.canvas
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

	size := s.canvas.Size()

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
	s.bar.Refresh()
}

func (s *HBar) Value() float64 {
	return s.value
}
