package vbar

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

type VBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	lines       []*canvas.Line

	cfg   *widgets.GaugeConfig
	value float64
	size  fyne.Size

	// Vertical band the bar occupies; modern reserves text rows above and below
	barY, barH float32

	layoutValues struct {
		middle       float32
		titleX       float32
		displayTextX float32
	}

	fillCache    []color.RGBA
	strokeCache  []color.RGBA
	cacheMinInt  int // first value represented in caches
	cacheSpan    float64
	cacheSpanInv float64

	// Hot-path skips
	lastColorIdx   int
	lastDisplayInt int
	hasLastDisplay bool
}

func New(cfg *widgets.GaugeConfig) *VBar {
	s := &VBar{
		cfg:          cfg,
		lastColorIdx: -1,
	}

	if s.cfg.Steps == 0 {
		s.cfg.Steps = 10
	}

	// Precompute span info
	s.cacheSpan = s.cfg.Max - s.cfg.Min
	if s.cacheSpan <= 0 {
		// prevent div/0 and nonsense config
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

func (s *VBar) GetConfig() *widgets.GaugeConfig {
	return s.cfg
}

func (s *VBar) clampNorm(v float64) float32 {
	// normalize v in [cfg.Min..cfg.Max] to [0..1]
	n := (v - s.cfg.Min) * s.cacheSpanInv
	if n < 0 {
		n = 0
	} else if n > 1 {
		n = 1
	}
	return float32(n)
}

func (s *VBar) cacheIndexForValue(v float64) int {
	idx := int(v) - s.cacheMinInt
	if idx < 0 {
		idx = 0
	} else if idx >= len(s.fillCache) {
		idx = len(s.fillCache) - 1
	}
	return idx
}

func (s *VBar) SetValue(value float64) {
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
	barHeight := norm * s.barH
	s.bar.Resize(fyne.Size{Width: s.size.Width, Height: barHeight})
	s.bar.Move(fyne.Position{X: 0, Y: s.barY + s.barH - barHeight})

	iv := int(value)
	if !s.hasLastDisplay || iv != s.lastDisplayInt {
		s.lastDisplayInt = iv
		s.hasLastDisplay = true
		s.displayText.Text = strconv.Itoa(iv)
		s.displayText.Refresh()
	}
}

func (s *VBar) Value() float64 {
	return s.value
}

func (s *VBar) CreateRenderer() fyne.WidgetRenderer {
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

	// default bar rect (will be resized in Layout/SetValue)
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

	// Pre-calc text offsets so we can center without re-measuring every layout
	s.layoutValues.titleX = -s.titleText.Size().Width * 0.5
	s.layoutValues.displayTextX = -s.displayText.Size().Width * 0.5

	maxSteps := s.cfg.Steps + 1
	s.lines = make([]*canvas.Line, maxSteps)

	// Build tick lines. Modern ticks are neutral, the bar carries the color.
	// Classic tints them with the gradient across the full range (Min..Max).
	for i := maxSteps; i > 0; i-- {
		strokeColor := widgets.TickMinor
		if (maxSteps-i)%2 == 0 {
			strokeColor = widgets.TickMajor
		}
		if s.cfg.Classic {
			t := float64(i-1) / float64(maxSteps-1) // 0..1
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
		s.lines[maxSteps-i] = line
	}

	return &VBarRenderer{VBar: s}
}

// getColorForValue returns fill & stroke color for an arbitrary gauge value.
// It maps the value to a [0..1] ratio across cfg.Min..cfg.Max.
func (s *VBar) getColorForValue(value float64) (fillColor, strokeColor color.RGBA) {
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

type VBarRenderer struct {
	*VBar
	objects []fyne.CanvasObject
}

func (r *VBarRenderer) MinSize() fyne.Size {
	return r.cfg.MinSize
}

func (r *VBarRenderer) Refresh() {
	// no-op, state is pushed directly on SetValue/Layout
}

func (r *VBarRenderer) Destroy() {
	// no-op
}

func (r *VBarRenderer) Layout(space fyne.Size) {
	if r.size == space {
		return
	}
	r.size = space

	// cache middle X for centering text, tick lines
	middle := space.Width * 0.5
	r.layoutValues.middle = middle

	if r.cfg.Classic {
		r.barY, r.barH = 0, space.Height

		r.face.Move(fyne.Position{X: 0, Y: -2})
		r.face.Resize(space.AddWidthHeight(0, 3))

		// Text layout (title above bar, displayText inside bar near the bottom)
		r.titleText.Move(fyne.Position{
			X: middle + r.layoutValues.titleX,
			Y: 0,
		})
		r.displayText.Move(fyne.Position{
			X: middle + r.layoutValues.displayTextX,
			Y: space.Height - r.displayText.MinSize().Height,
		})
	} else {
		// Modern: title row on top, value row at the bottom, track between them,
		// so neither label sits on the colored fill
		textSize := min(float32(25), space.Width*0.5)
		r.titleText.TextSize = textSize
		r.displayText.TextSize = textSize

		textHeight := r.displayText.MinSize().Height
		box := fyne.Size{Width: space.Width, Height: textHeight}
		r.titleText.Resize(box)
		r.titleText.Move(fyne.Position{})
		r.displayText.Resize(box)
		r.displayText.Move(fyne.Position{X: 0, Y: space.Height - textHeight})

		r.barY = textHeight
		r.barH = max(0, space.Height-textHeight*2)

		radius := space.Width * widgets.BarCornerFrac
		r.face.CornerRadius = radius
		r.bar.CornerRadius = radius
		r.face.Move(fyne.Position{X: 0, Y: r.barY})
		r.face.Resize(fyne.Size{Width: space.Width, Height: r.barH})
	}

	// Tick lines layout, spread across the bar band
	stepFactor := r.barH / float32(r.cfg.Steps)
	oneThird := space.Width * common.OneThird
	oneSeventh := space.Width * common.OneSeventh

	for i, line := range r.lines {
		y := r.barY + float32(i)*stepFactor
		if i%2 == 0 {
			line.Position1 = fyne.Position{X: middle - oneThird, Y: y}
			line.Position2 = fyne.Position{X: middle + oneThird, Y: y}
		} else {
			line.Position1 = fyne.Position{X: middle - oneSeventh, Y: y}
			line.Position2 = fyne.Position{X: middle + oneSeventh, Y: y}
		}
	}

	// Recompute bar geometry for current value using new size
	barHeight := r.clampNorm(r.value) * r.barH
	r.bar.Resize(fyne.Size{Width: space.Width, Height: barHeight})
	r.bar.Move(fyne.Position{X: 0, Y: r.barY + r.barH - barHeight})
}

func (r *VBarRenderer) Objects() []fyne.CanvasObject {
	if r.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(r.lines)+4)
		if !r.cfg.Classic {
			// Modern: the track sits under the bar, ticks on top of both
			objs = append(objs, r.face, r.bar)
		}
		for _, line := range r.lines {
			objs = append(objs, line)
		}
		if r.cfg.Classic {
			objs = append(objs, r.bar, r.face)
		}
		objs = append(objs, r.titleText, r.displayText)
		r.objects = objs
	}
	return r.objects
}
