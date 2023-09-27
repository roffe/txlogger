package widgets

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

type CBar struct {
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	bars        []*canvas.Line

	cfg *CBarConfig

	value float64

	canvas fyne.CanvasObject
}

type CBarConfig struct {
	Title            string
	DisplayString    string // default "%.0f"
	Min, Max, Center float64
	Steps            int
	Minsize          fyne.Size
	TextAtBottom     bool
}

func NewCBar(cfg *CBarConfig) *CBar {
	s := &CBar{
		cfg: cfg,
	}
	s.value = cfg.Center
	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}
	if s.cfg.DisplayString == "" {
		s.cfg.DisplayString = "%.0f"
	}
	s.canvas = s.render()
	return s
}

func (s *CBar) render() *fyne.Container {
	s.face = &canvas.Rectangle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, FillColor: color.RGBA{0x00, 0x00, 0x00, 0x00}, StrokeWidth: 3}
	s.bar = &canvas.Rectangle{FillColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80}}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: fmt.Sprintf(s.cfg.DisplayString, 0.00), Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 40}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignLeading

	bar := container.NewWithoutLayout(s.face)
	for i := 0; i < int(s.cfg.Steps+1); i++ {
		line := &canvas.Line{StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF}, StrokeWidth: 2}
		s.bars = append(s.bars, line)
		bar.Add(line)
	}
	bar.Objects = append(bar.Objects, s.bar, s.titleText, s.displayText)
	bar.Layout = s
	return bar
}

func (s *CBar) Layout(_ []fyne.CanvasObject, space fyne.Size) {
	diameter := space.Width
	height := space.Height
	middle := height / 2
	widthFactor := float32(diameter) / float32(s.cfg.Steps)

	s.face.Resize(space)

	y := float32(-s.bar.Size().Height * .47)
	if s.cfg.TextAtBottom {
		y = float32(s.bar.Size().Height * 0.85)
	}

	s.titleText.Move(fyne.NewPos(diameter/2-s.titleText.Size().Width/2, height-30))
	s.displayText.TextSize = min((diameter/2)/6, 40)
	s.displayText.Move(fyne.NewPos(diameter/2-s.displayText.MinSize().Width/2, y))

	for i, line := range s.bars {
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(float32(i)*widthFactor, middle-height/3)
			line.Position2 = fyne.NewPos(float32(i)*widthFactor, middle+height/3)
			continue
		}
		line.Position1 = fyne.NewPos(float32(i)*widthFactor, middle-height/7)
		line.Position2 = fyne.NewPos(float32(i)*widthFactor, middle+height/7)
	}

	s.refresh(space)
}

func (s *CBar) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return s.cfg.Minsize
}

func (s *CBar) Content() fyne.CanvasObject {
	return s.canvas
}

func (s *CBar) SetValue(value float64) {
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
	s.refresh(s.canvas.Size())
}

func (s *CBar) refresh(size fyne.Size) {
	rangeWidth := s.cfg.Max - s.cfg.Min
	center := size.Width / 2
	widthFactor := float32(size.Width) / float32(rangeWidth)

	s.displayText.Text = fmt.Sprintf(s.cfg.DisplayString, s.value)
	s.displayText.Refresh()

	var newBarPos fyne.Position
	var newBarSize fyne.Size

	if s.value < s.cfg.Center {
		s.bar.FillColor = color.RGBA{0x26, 0xcc, 0x00, 0x80}
		barWidth := float32(s.cfg.Center - s.value)
		barPosition := center - float32(s.cfg.Center-s.value)*widthFactor
		newBarPos = fyne.NewPos(barPosition, size.Height/8)
		newBarSize = fyne.NewSize(barWidth*widthFactor, size.Height-(size.Height/8*2))
	} else if s.value > s.cfg.Center {
		s.bar.FillColor = color.RGBA{0xA5, 0x00, 0x00, 0x80}
		barWidth := float32(s.value - s.cfg.Center)
		barPosition := center
		newBarPos = fyne.NewPos(barPosition, size.Height/8)
		newBarSize = fyne.NewSize(barWidth*widthFactor, size.Height-(size.Height/8*2))
	} else {
		s.bar.FillColor = color.RGBA{252, 186, 3, 0x80}
		barPosition := center - 3
		newBarPos = fyne.NewPos(barPosition, size.Height/8)
		newBarSize = fyne.NewSize(6, size.Height-(size.Height/8*2))
	}
	s.bar.Move(newBarPos)
	s.bar.Resize(newBarSize)
	s.bar.Refresh()

}

func (s *CBar) Value() float64 {
	return s.value
}
