package cbar

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type CBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	bars        []*canvas.Line

	cfg *widgets.GaugeConfig

	// Cached values
	value float64

	lastSize    fyne.Size
	valueRange  float64
	widthFactor float32
	center      float32

	barHeight float32
	barWidth  float32

	displayY float32

	// Cache layout calculations
	middleHeight     float32
	heightOneThird   float32
	heightOneSeventh float32
	stepFactor       float32

	// Fast float formatting
	fmtPrec int
	buf     []byte
}

func New(cfg *widgets.GaugeConfig) *CBar {
	if cfg.MinSize.Width <= 0 {
		cfg.MinSize.Width = 50
	}
	if cfg.MinSize.Height <= 0 {
		cfg.MinSize.Height = 50
	}
	if cfg.Steps <= 0 {
		cfg.Steps = 10
	}
	if cfg.DisplayString == "" {
		cfg.DisplayString = "%.0f"
	}
	if cfg.DisplayTextSize <= 0 {
		cfg.DisplayTextSize = 25
	}

	s := &CBar{
		cfg:        cfg,
		value:      cfg.Center,
		valueRange: cfg.Max - cfg.Min,
		fmtPrec:    -1,
	}
	if n := common.ParseFixedPrec(cfg.DisplayString); n >= 0 {
		s.fmtPrec = n
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *CBar) GetConfig() *widgets.GaugeConfig {
	return s.cfg
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

	s.buf = s.buf[:0]
	if s.fmtPrec >= 0 {
		s.buf = strconv.AppendFloat(s.buf, 0, 'f', s.fmtPrec, 64)
	} else {
		s.buf = common.AppendFormatFloat(s.buf, s.cfg.DisplayString, 0)
	}
	s.displayText = &canvas.Text{
		Text:      string(s.buf),
		Color:     color.RGBA{0xF0, 0xF0, 0xF0, 0xFF},
		TextSize:  float32(s.cfg.DisplayTextSize),
		TextStyle: fyne.TextStyle{Monospace: true},
		Alignment: fyne.TextAlignLeading,
	}

	for i := 0; i <= s.cfg.Steps; i++ {
		line := &canvas.Line{
			StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF},
			StrokeWidth: 2,
		}
		s.bars = append(s.bars, line)
	}
}

func (s *CBar) SetValue(value float64) {
	if value == s.value {
		return
	}
	s.value = max(s.cfg.Min, min(s.cfg.Max, value))

	barPosition := s.center
	var pxWidth float32
	switch {
	case s.value < s.cfg.Center:
		s.bar.FillColor = color.RGBA{0x26, 0xcc, 0x00, 0x80}
		s.barWidth = float32(s.cfg.Center - s.value)
		pxWidth = s.barWidth * s.widthFactor
		barPosition -= pxWidth
	case s.value > s.cfg.Center:
		s.bar.FillColor = color.RGBA{0xA5, 0x00, 0x00, 0x80}
		s.barWidth = float32(s.value - s.cfg.Center)
		pxWidth = s.barWidth * s.widthFactor
	default:
		s.bar.FillColor = color.RGBA{252, 186, 3, 0x80}
		barPosition -= 3
		s.barWidth = 6 / s.widthFactor
		pxWidth = 6
	}

	s.bar.Move(fyne.Position{X: barPosition, Y: 0})
	s.bar.Resize(fyne.Size{Width: pxWidth, Height: s.barHeight})

	s.buf = s.buf[:0]
	if s.fmtPrec >= 0 {
		s.buf = strconv.AppendFloat(s.buf, s.value, 'f', s.fmtPrec, 64)
	} else {
		s.buf = common.AppendFormatFloat(s.buf, s.cfg.DisplayString, s.value)
	}
	if !common.SameTextBytes(s.displayText.Text, s.buf) {
		s.displayText.Text = string(s.buf)
		s.displayText.Refresh()
		s.updateDisplayTextPosition()
	}
}

// updateDisplayTextPosition positions the display text so that the decimal
// point stays at the widget's horizontal center. Falls back to centering by
// width when no "." is present in the text.
func (s *CBar) updateDisplayTextPosition() {
	text := s.displayText.Text
	if len(text) == 0 {
		return
	}
	minSize := s.displayText.MinSize()

	dotIdx := -1
	for i := 0; i < len(text); i++ {
		if text[i] == '.' {
			dotIdx = i
			break
		}
	}

	var x float32
	if dotIdx >= 0 {
		charWidth := minSize.Width / float32(len(text))
		x = s.lastSize.Width*0.5 - charWidth*(float32(dotIdx)+0.5)
	} else {
		x = s.lastSize.Width*0.5 - minSize.Width*0.5
	}

	s.displayText.Move(fyne.Position{X: x, Y: s.displayY})
}

func (s *CBar) SetValue2(value float64) {
	s.SetValue(value)
}

func (s *CBar) CreateRenderer() fyne.WidgetRenderer {
	// Initialize visual elements
	s.initializeVisualElements()
	return &CBarRenderer{s}
}

type CBarRenderer struct {
	*CBar
}

func (r *CBarRenderer) MinSize() fyne.Size {
	return r.cfg.MinSize
}

func (r *CBarRenderer) Refresh() {
}

func (r *CBarRenderer) Destroy() {
}

func (r *CBarRenderer) Layout(space fyne.Size) {
	if r.lastSize == space {
		return
	}

	r.lastSize = space

	// Cache frequently used calculations
	r.center = space.Width * 0.5
	r.middleHeight = space.Height * 0.5
	r.heightOneThird = space.Height * common.OneThird
	r.heightOneSeventh = space.Height * common.OneSeventh
	r.widthFactor = space.Width / float32(r.valueRange)
	r.barHeight = space.Height
	r.stepFactor = space.Width / float32(r.cfg.Steps)

	r.face.Move(fyne.NewPos(-2, 0))
	r.face.Resize(space.AddWidthHeight(3, 0))

	// Update bar positions
	for i, line := range r.bars {
		pos := float32(i) * r.stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(pos, r.middleHeight-r.heightOneThird)
			line.Position2 = fyne.NewPos(pos, r.middleHeight+r.heightOneThird)
		} else {
			line.Position1 = fyne.NewPos(pos, r.middleHeight-r.heightOneSeventh)
			line.Position2 = fyne.NewPos(pos, r.middleHeight+r.heightOneSeventh)
		}
	}

	barPosition := r.center
	switch {
	case r.value < r.cfg.Center:
		r.bar.FillColor = color.RGBA{0x26, 0xcc, 0x00, 0x80}
		r.barWidth = float32(r.cfg.Center - r.value)
		barPosition -= r.barWidth * r.widthFactor
	case r.value > r.cfg.Center:
		r.bar.FillColor = color.RGBA{0xA5, 0x00, 0x00, 0x80}
		r.barWidth = float32(r.value - r.cfg.Center)
	default:
		r.bar.FillColor = color.RGBA{252, 186, 3, 0x80}
		barPosition -= 3
		r.barWidth = 6 / r.widthFactor
	}

	r.bar.Move(fyne.Position{X: barPosition, Y: 0})
	r.bar.Resize(fyne.Size{Width: r.barWidth * r.widthFactor, Height: r.barHeight})

	r.displayText.TextSize = r.bar.Size().Height - 8

	var y float32
	switch r.cfg.TextPosition {
	case widgets.TextAtTop:
		y = -r.bar.MinSize().Height - r.displayText.MinSize().Height
	case widgets.TextAtBottom:
		y = r.lastSize.Height - 2
	}

	r.displayY = y

	titleX := r.lastSize.Width*0.5 - r.titleText.MinSize().Width*0.5
	r.titleText.Move(fyne.Position{X: titleX, Y: r.lastSize.Height - 30})
	r.updateDisplayTextPosition()
}

func (r *CBarRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{}
	for _, line := range r.bars {
		objs = append(objs, line)
	}

	objs = append(objs, r.bar, r.face, r.titleText, r.displayText)
	return objs
}
