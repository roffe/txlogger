package hbar

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type band int8

const (
	bandNormal band = iota
	band75
	band90
)

var (
	colNormal = color.RGBA{0x2C, 0xA5, 0x00, 0x90}
	col75     = color.RGBA{0xFF, 0x98, 0x00, 0x90}
	col90     = color.RGBA{0xF4, 0x43, 0x36, 0x90}
	colTicks  = color.RGBA{0x00, 0xE5, 0x00, 0xFF}
	colTitle  = color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
)

type HBar struct {
	widget.BaseWidget
	face      *canvas.Rectangle
	barRect   *canvas.Rectangle
	titleText *canvas.Text
	bars      []*canvas.Line

	cfg *widgets.GaugeConfig

	value float64

	widthFactor float32

	max75, max90 float64
	eightHeight  float32
	height       float32

	// perf caches
	oldSize      fyne.Size
	lastBarWidth float32
	lastBarY     float32
	curBand      band
	layoutDirty  bool
	pixelEpsilon float32
}

func New(cfg *widgets.GaugeConfig) *HBar {
	s := &HBar{
		cfg:          cfg,
		pixelEpsilon: 0.5, // ignore sub-pixel changes to cut churn
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

func (s *HBar) GetConfig() *widgets.GaugeConfig {
	return s.cfg
}

func (s *HBar) render() {
	s.face = &canvas.Rectangle{StrokeColor: theme.Color(theme.ColorNameDisabled), StrokeWidth: 2}

	s.barRect = &canvas.Rectangle{FillColor: colNormal}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: colTitle, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	// create tick lines once; their positions get set on size change only
	s.bars = s.bars[:0]
	for i := 0; i < int(s.cfg.Steps+1); i++ {
		line := &canvas.Line{StrokeColor: colTicks, StrokeWidth: 2}
		s.bars = append(s.bars, line)
	}
}

func (s *HBar) bandFor(v float64) band {
	switch {
	case v >= s.max90:
		return band90
	case v >= s.max75:
		return band75
	default:
		return bandNormal
	}
}

func (s *HBar) applyBand(b band) {
	if b == s.curBand {
		return
	}
	s.curBand = b
	switch b {
	case band90:
		s.barRect.FillColor = col90
	case band75:
		s.barRect.FillColor = col75
	default:
		s.barRect.FillColor = colNormal
	}
}

func (s *HBar) SetValue(value float64) {
	// clamp to [0, Max]
	if value < 0 {
		value = 0
	} else if value > s.cfg.Max {
		value = s.cfg.Max
	}
	s.value = value

	// color only when band changes
	s.applyBand(s.bandFor(value))

	// compute desired width in pixels
	w := float32(value) * s.widthFactor

	// if layout changed, we must move Y even if width is same
	if s.layoutDirty {
		s.barRect.Move(fyne.NewPos(0, s.eightHeight))
		s.lastBarY = s.eightHeight
		s.layoutDirty = false
	}

	// skip no-op width changes (within epsilon)
	if float32(math.Abs(float64(w-s.lastBarWidth))) < s.pixelEpsilon {
		return
	}

	// resize width only (height is already set in Layout)
	s.barRect.Resize(fyne.NewSize(w, s.height))
	s.lastBarWidth = w
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

	// Precompute geometry once per size change
	s.widthFactor = float32(diameter) / float32(s.cfg.Max)
	stepsFactor := float32(diameter) / float32(s.cfg.Steps)
	s.eightHeight = space.Height * common.OneEight
	s.height = space.Height - (s.eightHeight * 2)

	// mark layout dirty so SetValue repositions Y
	s.layoutDirty = true

	// face & title
	s.face.Resize(space)
	s.titleText.Move(fyne.NewPos(diameter*.5-s.titleText.Size().Width*.5, height-30))

	// tick lines
	for i, line := range s.bars {
		x := float32(i) * stepsFactor
		if i%2 == 0 {
			line.Position1 = fyne.NewPos(x, middle-height*common.OneThird)
			line.Position2 = fyne.NewPos(x, middle+height*common.OneThird)
		} else {
			line.Position1 = fyne.NewPos(x, middle-height*common.OneSeventh)
			line.Position2 = fyne.NewPos(x, middle+height*common.OneSeventh)
		}
	}

	// size & place the bar according to current value
	// (also updates color band if needed)
	s.SetValue(s.value)
}

func (s *HBarRenderer) MinSize() fyne.Size {
	return s.cfg.MinSize
}

func (s *HBarRenderer) Refresh() {
	// Intentionally empty: we do targeted updates in SetValue/Layout.
}

func (s *HBarRenderer) Destroy() {}

func (s *HBarRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{s.face}
	for _, line := range s.bars {
		objs = append(objs, line)
	}
	objs = append(objs, s.barRect, s.titleText)
	return objs
}
