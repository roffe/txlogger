package hbar

import (
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type HBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	lines       []*canvas.Line

	cfg   *widgets.GaugeConfig
	value float64
	size  fyne.Size

	layoutValues struct {
		middle       float32
		titleX       float32
		displayTextX float32
	}

	fillCache    []color.RGBA
	strokeCache  []color.RGBA
	cacheMinInt  int
	cacheSpan    float64
	cacheSpanInv float64

	// Hot-path skips
	lastColorIdx   int
	lastDisplayInt int
	hasLastDisplay bool
}

func New(cfg *widgets.GaugeConfig) *HBar {
	s := &HBar{
		cfg:          cfg,
		lastColorIdx: -1,
	}

	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}

	s.cacheSpan = s.cfg.Max - s.cfg.Min
	if s.cacheSpan <= 0 {
		s.cacheSpan = 1
	}
	s.cacheSpanInv = 1.0 / s.cacheSpan

	minInt := int(math.Floor(s.cfg.Min))
	maxInt := int(math.Ceil(s.cfg.Max))
	count := maxInt - minInt + 1
	if count < 1 {
		count = 1
	}

	s.cacheMinInt = minInt
	s.fillCache = make([]color.RGBA, count)
	s.strokeCache = make([]color.RGBA, count)

	for v := minInt; v <= maxInt; v++ {
		fill, stroke := s.getColorForValue(float64(v))
		idx := v - minInt
		s.fillCache[idx] = fill
		s.strokeCache[idx] = stroke
	}

	s.ExtendBaseWidget(s)
	return s
}

func (s *HBar) GetConfig() *widgets.GaugeConfig {
	return s.cfg
}

func (s *HBar) clampNorm(v float64) float32 {
	n := (v - s.cfg.Min) * s.cacheSpanInv
	if n < 0 {
		n = 0
	} else if n > 1 {
		n = 1
	}
	return float32(n)
}

func (s *HBar) cacheIndexForValue(v float64) int {
	idx := int(v) - s.cacheMinInt
	if idx < 0 {
		idx = 0
	} else if idx >= len(s.fillCache) {
		idx = len(s.fillCache) - 1
	}
	return idx
}

func (s *HBar) SetValue(value float64) {
	if value == s.value {
		return
	}
	s.value = value

	idx := s.cacheIndexForValue(value)
	if idx != s.lastColorIdx {
		s.bar.FillColor = s.fillCache[idx]
		s.bar.StrokeColor = s.strokeCache[idx]
		s.lastColorIdx = idx
	}

	norm := s.clampNorm(value)
	barWidth := norm * float32(s.size.Width)
	s.bar.Resize(fyne.Size{Width: barWidth, Height: s.size.Height})

	iv := int(value)
	if !s.hasLastDisplay || iv != s.lastDisplayInt {
		s.lastDisplayInt = iv
		s.hasLastDisplay = true
		s.displayText.Text = strconv.Itoa(iv)
		s.displayText.Refresh()
	}
}

func (s *HBar) SetValue2(value float64) {
	s.SetValue(value)
}

func (s *HBar) Value() float64 {
	return s.value
}

func (s *HBar) CreateRenderer() fyne.WidgetRenderer {
	s.face = &canvas.Rectangle{
		StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},
		FillColor:   color.RGBA{0x00, 0x00, 0x00, 0x00},
		StrokeWidth: 3,
	}

	s.bar = &canvas.Rectangle{
		FillColor:   s.fillCache[0],
		StrokeColor: s.strokeCache[0],
	}

	s.titleText = &canvas.Text{
		Text:     s.cfg.Title,
		Color:    color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF},
		TextSize: 25,
	}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{
		Text:     "0",
		Color:    color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF},
		TextSize: 25,
	}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	s.layoutValues.titleX = -s.titleText.Size().Width * 0.5
	s.layoutValues.displayTextX = -s.displayText.Size().Width * 0.5

	maxSteps := s.cfg.Steps + 1
	s.lines = make([]*canvas.Line, maxSteps)

	for i := 0; i < int(maxSteps); i++ {
		t := float64(i) / float64(maxSteps-1)
		ci := int(t * float64(len(s.strokeCache)-1))
		if ci < 0 {
			ci = 0
		} else if ci >= len(s.strokeCache) {
			ci = len(s.strokeCache) - 1
		}
		strokeColor := s.strokeCache[ci]
		line := &canvas.Line{
			StrokeColor: strokeColor,
			StrokeWidth: 2,
		}
		s.lines[i] = line
	}

	return &HBarRenderer{s}
}

// getColorForValue returns fill & stroke color for an arbitrary gauge value.
// It maps the value to a [0..1] ratio across cfg.Min..cfg.Max.
func (s *HBar) getColorForValue(value float64) (fillColor, strokeColor color.RGBA) {
	span := s.cfg.Max - s.cfg.Min
	if span <= 0 {
		span = 1
	}
	ratio := (value - s.cfg.Min) / span
	if ratio < 0 {
		ratio = 0
	} else if ratio > 1 {
		ratio = 1
	}

	if s.cfg.ColorScale == widgets.BlueYellowScale {
		if ratio < 0.5 {
			blueRatio := 1 - (ratio * 2)
			r := uint8(0xDD * ratio * 2)
			g := uint8(0x77 + (0x33 * ratio * 2))
			b := uint8(0xBB * blueRatio)
			return color.RGBA{R: r, G: g, B: b, A: 0x80},
				color.RGBA{R: r, G: g, B: b, A: 0xFF}
		}
		redRatio := (ratio - 0.5) * 2
		r := uint8(0xDD - (0x11 * redRatio))
		g := uint8(0xAA - (0x77 * redRatio))
		return color.RGBA{R: r, G: g, B: 0x33, A: 0x80},
			color.RGBA{R: r, G: g, B: 0x33, A: 0xFF}
	}

	// default green -> red
	rr := uint8(0xA5 * ratio)
	gg := uint8(0xA5 * (1 - ratio))
	return color.RGBA{R: rr, G: gg, B: 0, A: 0x80},
		color.RGBA{R: rr, G: gg, B: 0, A: 0xFF}
}

type HBarRenderer struct {
	*HBar
}

func (r *HBarRenderer) MinSize() fyne.Size {
	return r.cfg.MinSize
}

func (r *HBarRenderer) Refresh() {
	// no-op, state is pushed directly on SetValue/Layout
}

func (r *HBarRenderer) Destroy() {
	// no-op
}

func (r *HBarRenderer) Layout(space fyne.Size) {
	if r.size == space {
		return
	}
	r.size = space

	r.layoutValues.middle = space.Height * 0.5

	stepFactor := float32(space.Width) / float32(r.cfg.Steps)

	// Face layout
	r.face.Move(fyne.Position{X: -2, Y: 0})
	r.face.Resize(space.AddWidthHeight(3, 1))

	// Title centered horizontally, just below bar
	titleMinSize := r.titleText.MinSize()
	r.titleText.Resize(fyne.Size{Width: space.Width, Height: titleMinSize.Height})
	r.titleText.Move(fyne.Position{
		X: 0,
		Y: space.Height - titleMinSize.Height,
	})

	// Display text in the middle, centered vertically
	displayMinSize := r.displayText.MinSize()
	r.displayText.Resize(fyne.Size{Width: space.Width, Height: displayMinSize.Height})
	r.displayText.Move(fyne.Position{
		X: 0,
		Y: r.layoutValues.middle - displayMinSize.Height*0.5,
	})

	// Tick lines layout (vertical lines across width)
	oneThird := space.Height * common.OneThird
	oneSeventh := space.Height * common.OneSeventh
	middle := r.layoutValues.middle

	for i, line := range r.lines {
		x := float32(i) * stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.Position{X: x, Y: middle - oneThird}
			line.Position2 = fyne.Position{X: x, Y: middle + oneThird}
		} else {
			line.Position1 = fyne.Position{X: x, Y: middle - oneSeventh}
			line.Position2 = fyne.Position{X: x, Y: middle + oneSeventh}
		}
	}

	// Bar position is fixed at origin; set once here so SetValue can skip it.
	r.bar.Move(fyne.Position{X: 0, Y: 0})

	// Recompute bar geometry for current value using new size
	norm := r.clampNorm(r.value)
	barWidth := norm * float32(r.size.Width)
	r.bar.Resize(fyne.Size{Width: barWidth, Height: r.size.Height})
}

func (r *HBarRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.lines)+4)
	for _, line := range r.lines {
		objs = append(objs, line)
	}
	objs = append(objs, r.bar, r.face, r.titleText, r.displayText)
	return objs
}
