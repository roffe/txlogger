package widgets

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
)

type CBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	bars        []*canvas.Line

	cfg *CBarConfig

	value float64

	container *fyne.Container

	size        fyne.Size
	valueRange  float64
	widthFactor float32
	center      float32
	eightHeight float32
	barHeight   float32
}

type TextPosition int

const (
	TextAtTop TextPosition = iota
	TextAtBottom
	TextAtCenter
)

type CBarConfig struct {
	Title            string
	DisplayString    string // default "%.0f"
	DisplayTextSize  int
	Min, Max, Center float64
	Steps            int
	Minsize          fyne.Size
	TextPosition     TextPosition
}

func NewCBar(cfg *CBarConfig) *CBar {
	s := &CBar{
		cfg: cfg,
	}

	if s.cfg.Minsize.Width == 0 {
		s.cfg.Minsize.Width = 50
	}
	if s.cfg.Minsize.Height == 0 {
		s.cfg.Minsize.Height = 50
	}

	s.ExtendBaseWidget(s)
	s.value = cfg.Center
	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}
	if s.cfg.DisplayString == "" {
		s.cfg.DisplayString = "%.0f"
	}
	if s.cfg.DisplayTextSize == 0 {
		s.cfg.DisplayTextSize = 25
	}

	s.valueRange = s.cfg.Max - s.cfg.Min

	s.face = &canvas.Rectangle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF}, FillColor: color.RGBA{0x00, 0x00, 0x00, 0x00}, StrokeWidth: 3}
	s.bar = &canvas.Rectangle{FillColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80}}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: fmt.Sprintf(s.cfg.DisplayString, 0.00), Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: float32(s.cfg.DisplayTextSize)}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignLeading

	s.container = container.NewWithoutLayout()
	for i := 0; i < int(s.cfg.Steps+1); i++ {
		line := &canvas.Line{StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF}, StrokeWidth: 2}
		s.bars = append(s.bars, line)
		s.container.Add(line)
	}
	s.container.Objects = append(s.container.Objects, s.bar, s.face, s.titleText, s.displayText)
	return s
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
	s.refresh()
}

func (s *CBar) refresh() {
	s.displayText.Text = fmt.Sprintf(s.cfg.DisplayString, s.value)
	s.displayText.Refresh()
	if s.value < s.cfg.Center {
		s.bar.FillColor = color.RGBA{0x26, 0xcc, 0x00, 0x80}
		barWidth := float32(s.cfg.Center - s.value)
		barPosition := s.center - float32(s.cfg.Center-s.value)*s.widthFactor
		s.bar.Move(fyne.NewPos(barPosition, s.eightHeight))
		s.bar.Resize(fyne.NewSize(barWidth*s.widthFactor, s.barHeight))
	} else if s.value > s.cfg.Center {
		s.bar.FillColor = color.RGBA{0xA5, 0x00, 0x00, 0x80}
		barWidth := float32(s.value - s.cfg.Center)
		barPosition := s.center
		s.bar.Move(fyne.NewPos(barPosition, s.eightHeight))
		s.bar.Resize(fyne.NewSize(barWidth*s.widthFactor, s.barHeight))
	} else {
		s.bar.FillColor = color.RGBA{252, 186, 3, 0x80}
		barPosition := s.center - 3
		s.bar.Move(fyne.NewPos(barPosition, s.eightHeight))
		s.bar.Resize(fyne.NewSize(6, s.barHeight))
	}

	var y float32

	switch s.cfg.TextPosition {
	case TextAtTop:
		y = -s.bar.MinSize().Height - s.displayText.MinSize().Height
	case TextAtBottom:
		y = s.size.Height
	case TextAtCenter:
		//y = (s.bar.Size().Height - s.displayText.MinSize().Height) / 2

	}
	diameter := s.size.Width
	height := s.size.Height
	s.titleText.Move(fyne.NewPos(diameter*.5-s.titleText.MinSize().Width*.5, height-30))
	s.displayText.Move(fyne.NewPos(diameter*.5-s.displayText.MinSize().Width*.5, y))
}

func (s *CBar) CreateRenderer() fyne.WidgetRenderer {
	return &CBarRenderer{
		d: s,
	}
}

type CBarRenderer struct {
	d *CBar
}

func (dr *CBarRenderer) Layout(space fyne.Size) {
	if dr.d.size.Width == space.Width && dr.d.size.Height == space.Height {
		return
	}
	// log.Println("cbar.Layout", dr.d.displayText.Text, space.Width, space.Height)
	s := dr.d
	s.size = space
	s.container.Resize(space)
	s.eightHeight = s.size.Height * common.OneEight
	diameter := space.Width
	s.center = diameter * .5
	height := space.Height
	middle := height * .5
	stepFactor := float32(diameter) / float32(s.cfg.Steps)
	s.widthFactor = space.Width / float32(s.valueRange)
	s.barHeight = s.size.Height - (s.eightHeight * 2)
	s.face.Move(fyne.NewPos(-2, 0))
	s.face.Resize(space.AddWidthHeight(3, 0))

	for i, line := range s.bars {
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(float32(i)*stepFactor, middle-height*common.OneThird)
			line.Position2 = fyne.NewPos(float32(i)*stepFactor, middle+height*common.OneThird)
			continue
		}
		line.Position1 = fyne.NewPos(float32(i)*stepFactor, middle-height*common.OneSeventh)
		line.Position2 = fyne.NewPos(float32(i)*stepFactor, middle+height*common.OneSeventh)
	}
	s.refresh()
}

func (dr *CBarRenderer) MinSize() fyne.Size {
	return dr.d.cfg.Minsize
}

func (dr *CBarRenderer) Refresh() {
}

func (dr *CBarRenderer) Destroy() {
}

func (dr *CBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{dr.d.container}
}
