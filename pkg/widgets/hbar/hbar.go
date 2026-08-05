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

	// Vertical band the bar occupies; modern reserves a text row above it
	barY, barH float32

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

	if !cfg.Classic {
		// Modern bars are solid; the translucent classic fill washes out on the dark track
		copy(s.fillCache, s.strokeCache)
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
	barWidth := norm * s.size.Width
	s.bar.Resize(fyne.Size{Width: barWidth, Height: s.barH})

	iv := int(value)
	if !s.hasLastDisplay || iv != s.lastDisplayInt {
		s.lastDisplayInt = iv
		s.hasLastDisplay = true
		s.displayText.Text = strconv.Itoa(iv)
		s.displayText.Refresh()
	}
}

func (s *HBar) Value() float64 {
	return s.value
}

func (s *HBar) CreateRenderer() fyne.WidgetRenderer {
	textColor := widgets.TextPrimary
	if s.cfg.Classic {
		textColor = color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}
		// Thin outline around the bar, drawn on top of it
		s.face = &canvas.Rectangle{
			StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},
			FillColor:   color.RGBA{0x00, 0x00, 0x00, 0x00},
			StrokeWidth: 3,
		}
	} else {
		// Modern: a dark rounded track the value bar fills
		s.face = &canvas.Rectangle{FillColor: widgets.TrackColor}
	}

	s.bar = &canvas.Rectangle{
		FillColor:   s.fillCache[0],
		StrokeColor: s.strokeCache[0],
	}

	s.titleText = &canvas.Text{
		Text:     s.cfg.Title,
		Color:    textColor,
		TextSize: 25,
	}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{
		Text:     "0",
		Color:    textColor,
		TextSize: 25,
	}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	if !s.cfg.Classic {
		// Modern splits the labels to opposite ends of the bar
		s.titleText.Alignment = fyne.TextAlignLeading
		s.displayText.Alignment = fyne.TextAlignTrailing
	}

	s.layoutValues.titleX = -s.titleText.Size().Width * 0.5
	s.layoutValues.displayTextX = -s.displayText.Size().Width * 0.5

	maxSteps := s.cfg.Steps + 1
	s.lines = make([]*canvas.Line, maxSteps)

	for i := 0; i < int(maxSteps); i++ {
		// Modern ticks are neutral, the bar carries the color. Classic tints them.
		strokeColor := widgets.TickMinor
		if i%2 == 0 {
			strokeColor = widgets.TickMajor
		}
		if s.cfg.Classic {
			t := float64(i) / float64(maxSteps-1)
			ci := int(t * float64(len(s.strokeCache)-1))
			if ci < 0 {
				ci = 0
			} else if ci >= len(s.strokeCache) {
				ci = len(s.strokeCache) - 1
			}
			strokeColor = s.strokeCache[ci]
		}
		line := &canvas.Line{
			StrokeColor: strokeColor,
			StrokeWidth: 2,
		}
		s.lines[i] = line
	}

	return &HBarRenderer{HBar: s}
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

	if !s.cfg.Classic {
		// Modern bars share the bright dial zone gradient: green -> yellow -> red
		c := widgets.ZoneColor(ratio)
		return c, c
	}

	// default green -> red
	rr := uint8(0xA5 * ratio)
	gg := uint8(0xA5 * (1 - ratio))
	return color.RGBA{R: rr, G: gg, B: 0, A: 0x80},
		color.RGBA{R: rr, G: gg, B: 0, A: 0xFF}
}

type HBarRenderer struct {
	HBar    *HBar
	objects []fyne.CanvasObject
}

func (r *HBarRenderer) MinSize() fyne.Size {
	return r.HBar.cfg.MinSize
}

func (r *HBarRenderer) Refresh() {
	// no-op, state is pushed directly on SetValue/Layout
}

func (r *HBarRenderer) Destroy() {
	// no-op
}

func (r *HBarRenderer) Layout(space fyne.Size) {
	if r.HBar.size == space {
		return
	}
	r.HBar.size = space

	stepFactor := space.Width / float32(r.HBar.cfg.Steps)

	if r.HBar.cfg.Classic {
		r.HBar.barY, r.HBar.barH = 0, space.Height

		r.HBar.face.Move(fyne.Position{X: -2, Y: 0})
		r.HBar.face.Resize(space.AddWidthHeight(3, 1))

		// Title centered horizontally, just below bar
		titleMinSize := r.HBar.titleText.MinSize()
		r.HBar.titleText.Resize(fyne.Size{Width: space.Width, Height: titleMinSize.Height})
		r.HBar.titleText.Move(fyne.Position{
			X: 0,
			Y: space.Height - titleMinSize.Height,
		})

		// Display text in the middle, centered vertically
		displayMinSize := r.HBar.displayText.MinSize()
		r.HBar.displayText.Resize(fyne.Size{Width: space.Width, Height: displayMinSize.Height})
		r.HBar.displayText.Move(fyne.Position{
			X: 0,
			Y: space.Height*0.5 - displayMinSize.Height*0.5,
		})
	} else {
		// Modern: a text row on top (title left, value right), the bar band below
		// it, so the readout never sits on the colored fill
		textSize := min(float32(25), space.Height*0.42)
		r.HBar.titleText.TextSize = textSize
		r.HBar.displayText.TextSize = textSize

		textHeight := r.HBar.displayText.MinSize().Height
		box := fyne.Size{Width: space.Width, Height: textHeight}
		r.HBar.titleText.Resize(box)
		r.HBar.titleText.Move(fyne.Position{})
		r.HBar.displayText.Resize(box)
		r.HBar.displayText.Move(fyne.Position{})

		r.HBar.barY = textHeight
		r.HBar.barH = space.Height - textHeight

		radius := r.HBar.barH * widgets.BarCornerFrac
		r.HBar.face.CornerRadius = radius
		r.HBar.bar.CornerRadius = radius
		r.HBar.face.Move(fyne.Position{X: 0, Y: r.HBar.barY})
		r.HBar.face.Resize(fyne.Size{Width: space.Width, Height: r.HBar.barH})
	}

	// Tick lines layout (vertical lines across the bar band)
	middle := r.HBar.barY + r.HBar.barH*0.5
	r.HBar.layoutValues.middle = middle
	oneThird := r.HBar.barH * common.OneThird
	oneSeventh := r.HBar.barH * common.OneSeventh

	for i, line := range r.HBar.lines {
		x := float32(i) * stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.Position{X: x, Y: middle - oneThird}
			line.Position2 = fyne.Position{X: x, Y: middle + oneThird}
		} else {
			line.Position1 = fyne.Position{X: x, Y: middle - oneSeventh}
			line.Position2 = fyne.Position{X: x, Y: middle + oneSeventh}
		}
	}

	// Bar X is fixed; set position once here so SetValue only resizes.
	r.HBar.bar.Move(fyne.Position{X: 0, Y: r.HBar.barY})

	// Recompute bar geometry for current value using new size
	norm := r.HBar.clampNorm(r.HBar.value)
	r.HBar.bar.Resize(fyne.Size{Width: norm * space.Width, Height: r.HBar.barH})
}

func (r *HBarRenderer) Objects() []fyne.CanvasObject {
	if r.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(r.HBar.lines)+4)
		if !r.HBar.cfg.Classic {
			// Modern: the track sits under the bar, ticks on top of both
			objs = append(objs, r.HBar.face, r.HBar.bar)
		}
		for _, line := range r.HBar.lines {
			objs = append(objs, line)
		}
		if r.HBar.cfg.Classic {
			objs = append(objs, r.HBar.bar, r.HBar.face)
		}
		objs = append(objs, r.HBar.titleText, r.HBar.displayText)
		r.objects = objs
	}
	return r.objects
}
