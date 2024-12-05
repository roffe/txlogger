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

	// Cached values
	value       float64
	container   *fyne.Container
	lastSize    fyne.Size
	valueRange  float64
	widthFactor float32
	center      float32
	eightHeight float32
	barHeight   float32

	// Cache layout calculations
	middleHeight     float32
	heightOneThird   float32
	heightOneSeventh float32
	stepFactor       float32
}

// Config structs remain the same
type TextPosition int

const (
	TextAtTop TextPosition = iota
	TextAtBottom
	TextAtCenter
)

type CBarConfig struct {
	Title            string
	DisplayString    string
	DisplayTextSize  int
	Min, Max, Center float64
	Steps            int
	Minsize          fyne.Size
	TextPosition     TextPosition
}

func NewCBar(cfg *CBarConfig) *CBar {
	if cfg.Minsize.Width == 0 {
		cfg.Minsize.Width = 50
	}
	if cfg.Minsize.Height == 0 {
		cfg.Minsize.Height = 50
	}
	if cfg.Steps == 0 {
		cfg.Steps = 10
	}
	if cfg.DisplayString == "" {
		cfg.DisplayString = "%.0f"
	}
	if cfg.DisplayTextSize == 0 {
		cfg.DisplayTextSize = 25
	}

	s := &CBar{
		cfg:        cfg,
		value:      cfg.Center,
		valueRange: cfg.Max - cfg.Min,
	}
	s.ExtendBaseWidget(s)

	// Initialize visual elements
	s.initializeVisualElements()

	return s
}

func (s *CBar) initializeVisualElements() {
	s.face = &canvas.Rectangle{
		StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},
		FillColor:   color.RGBA{0x00, 0x00, 0x00, 0x00},
		StrokeWidth: 3,
	}

	s.bar = &canvas.Rectangle{
		FillColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80},
	}

	s.titleText = &canvas.Text{
		Text:      s.cfg.Title,
		Color:     color.RGBA{0xF0, 0xF0, 0xF0, 0xFF},
		TextSize:  25,
		TextStyle: fyne.TextStyle{Monospace: true},
		Alignment: fyne.TextAlignCenter,
	}

	s.displayText = &canvas.Text{
		Text:      fmt.Sprintf(s.cfg.DisplayString, 0.00),
		Color:     color.RGBA{0xF0, 0xF0, 0xF0, 0xFF},
		TextSize:  float32(s.cfg.DisplayTextSize),
		TextStyle: fyne.TextStyle{Monospace: true},
		Alignment: fyne.TextAlignLeading,
	}

	s.container = container.NewWithoutLayout()
	s.initializeBars()
	s.container.Objects = append(s.container.Objects, s.bar, s.face, s.titleText, s.displayText)
}

func (s *CBar) initializeBars() {
	for i := 0; i <= s.cfg.Steps; i++ {
		line := &canvas.Line{
			StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF},
			StrokeWidth: 2,
		}
		s.bars = append(s.bars, line)
		s.container.Add(line)
	}
}

func (s *CBar) SetValue(value float64) {
	if value == s.value {
		return
	}

	// Clamp value between min and max
	if value > s.cfg.Max {
		value = s.cfg.Max
	} else if value < s.cfg.Min {
		value = s.cfg.Min
	}

	s.value = value
	s.refresh()
}

func (s *CBar) refresh() {
	s.displayText.Text = fmt.Sprintf(s.cfg.DisplayString, s.value)
	s.displayText.Refresh()

	barPosition := s.center
	var barWidth float32

	switch {
	case s.value < s.cfg.Center:
		s.bar.FillColor = color.RGBA{0x26, 0xcc, 0x00, 0x80}
		barWidth = float32(s.cfg.Center - s.value)
		barPosition -= barWidth * s.widthFactor
	case s.value > s.cfg.Center:
		s.bar.FillColor = color.RGBA{0xA5, 0x00, 0x00, 0x80}
		barWidth = float32(s.value - s.cfg.Center)
	default:
		s.bar.FillColor = color.RGBA{252, 186, 3, 0x80}
		barPosition -= 3
		barWidth = 6 / s.widthFactor
	}

	s.bar.Move(fyne.NewPos(barPosition, s.eightHeight))
	s.bar.Resize(fyne.NewSize(barWidth*s.widthFactor, s.barHeight))

	// Calculate text position
	var y float32
	switch s.cfg.TextPosition {
	case TextAtTop:
		y = -s.bar.MinSize().Height - s.displayText.MinSize().Height
	case TextAtBottom:
		y = s.lastSize.Height
	}

	titleX := s.lastSize.Width*0.5 - s.titleText.MinSize().Width*0.5
	displayX := s.lastSize.Width*0.5 - s.displayText.MinSize().Width*0.5

	s.titleText.Move(fyne.NewPos(titleX, s.lastSize.Height-30))
	s.displayText.Move(fyne.NewPos(displayX, y))
}

func (s *CBar) Size() fyne.Size {
	return s.container.Size()
}

func (s *CBar) Resize(space fyne.Size) {
	if s.lastSize == space {
		return
	}

	s.lastSize = space
	s.container.Resize(space)

	// Cache frequently used calculations
	s.eightHeight = space.Height * common.OneEight
	s.center = space.Width * 0.5
	s.middleHeight = space.Height * 0.5
	s.heightOneThird = space.Height * common.OneThird
	s.heightOneSeventh = space.Height * common.OneSeventh
	s.widthFactor = space.Width / float32(s.valueRange)
	s.barHeight = space.Height - (s.eightHeight * 2)
	s.stepFactor = space.Width / float32(s.cfg.Steps)

	s.face.Move(fyne.NewPos(-2, 0))
	s.face.Resize(space.AddWidthHeight(3, 0))

	// Update bar positions
	for i, line := range s.bars {
		pos := float32(i) * s.stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(pos, s.middleHeight-s.heightOneThird)
			line.Position2 = fyne.NewPos(pos, s.middleHeight+s.heightOneThird)
		} else {
			line.Position1 = fyne.NewPos(pos, s.middleHeight-s.heightOneSeventh)
			line.Position2 = fyne.NewPos(pos, s.middleHeight+s.heightOneSeventh)
		}
	}

	s.refresh()
}

func (s *CBar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.container)
}
