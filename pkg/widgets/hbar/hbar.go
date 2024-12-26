package hbar

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type HBar struct {
	widget.BaseWidget
	face      *canvas.Rectangle
	barRect   *canvas.Rectangle
	titleText *canvas.Text
	bars      []*canvas.Line

	cfg widgets.GaugeConfig

	value float64

	widthFactor float32

	max75, max90 float64
	eightHeight  float32
	height       float32

	oldSize fyne.Size
}

func New(cfg widgets.GaugeConfig) *HBar {
	s := &HBar{
		cfg: cfg,
	}
	s.ExtendBaseWidget(s)
	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}
	s.max75 = s.cfg.Max * .75
	s.max90 = s.cfg.Max * .90

	s.render()
	return s
}

func (s *HBar) GetConfig() widgets.GaugeConfig {
	return s.cfg
}

func (s *HBar) render() {
	s.face = &canvas.Rectangle{StrokeColor: theme.Color(theme.ColorNameDisabled), StrokeWidth: 2}

	s.barRect = &canvas.Rectangle{FillColor: color.RGBA{0x2C, 0xA5, 0x00, 0x80}}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter
	for i := 0; i < int(s.cfg.Steps+1); i++ {
		line := &canvas.Line{StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF}, StrokeWidth: 2}
		s.bars = append(s.bars, line)
	}

}

func (s *HBar) SetValue(value float64) {
	s.value = value
	switch {
	case value >= s.max75 && value < s.max90:
		s.barRect.FillColor = color.RGBA{R: 0xFF, G: 0x98, B: 0x00, A: 0x90}
	case value >= s.max90:
		s.barRect.FillColor = color.RGBA{R: 0xF4, G: 0x43, B: 0x36, A: 0x90}
	default:
		s.barRect.FillColor = color.RGBA{0x2C, 0xA5, 0x00, 0x90}
	}

	s.barRect.Move(fyne.NewPos(0, s.eightHeight))
	s.barRect.Resize(fyne.NewSize((float32(value) * s.widthFactor), s.height))
}

func (s *HBar) SetValue2(value float64) {
	s.SetValue(value)
}

func (s *HBar) Value() float64 {
	return s.value
}

func (s *HBar) CreateRenderer() fyne.WidgetRenderer {
	return &HBarRenderer{s}
}

type HBarRenderer struct {
	*HBar
}

func (s *HBarRenderer) Layout(space fyne.Size) {
	if s.oldSize == space {
		return
	}
	s.oldSize = space

	diameter := space.Width
	height := space.Height
	middle := height * .5
	s.widthFactor = float32(diameter) / float32(s.cfg.Max)
	stepsFactor := float32(diameter) / float32(s.cfg.Steps)
	s.eightHeight = space.Height * common.OneEight
	s.height = space.Height - (s.eightHeight * 2)
	s.face.Resize(space)
	s.titleText.Move(fyne.NewPos(diameter*.5-s.titleText.Size().Width*.5, height-30))
	s.barRect.Move(fyne.NewPos(space.Width-float32(s.value), 0))
	for i, line := range s.bars {
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(float32(i)*stepsFactor, middle-height*common.OneThird)
			line.Position2 = fyne.NewPos(float32(i)*stepsFactor, middle+height*common.OneThird)
			continue
		}
		line.Position1 = fyne.NewPos(float32(i)*stepsFactor, middle-height*common.OneSeventh)
		line.Position2 = fyne.NewPos(float32(i)*stepsFactor, middle+height*common.OneSeventh)
	}
	s.SetValue(s.value)
}

func (s *HBarRenderer) MinSize() fyne.Size {
	return s.cfg.MinSize
}

func (s *HBarRenderer) Refresh() {
}

func (s *HBarRenderer) Destroy() {
}

func (s *HBarRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{s.face}
	for _, line := range s.bars {
		objs = append(objs, line)
	}
	objs = append(objs, s.barRect, s.titleText)
	return objs
}
