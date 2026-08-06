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

// peakDecay is how far the hold markers bleed back toward center per sample.
// Exponential, so it needs no timer — it rides the sample stream itself.
// ponytail: rate is per-sample, ~1.5 s to fall back at 30 Hz. Make it time-based
// if log rates ever vary enough for that to feel wrong.
const peakDecay = 0.02

type CBar struct {
	widget.BaseWidget
	face        *canvas.Rectangle
	bar         *canvas.Rectangle
	titleText   *canvas.Text
	displayText *canvas.Text
	bars        []*canvas.Line

	// Peak-hold markers: latch the furthest excursion either side of center so a
	// value jittering at 10 Hz still shows how far it actually went.
	peakLo, peakHi       *canvas.Rectangle
	peakLoVal, peakHiVal float64

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

	// Bar colors: below center / above center / on center
	colorLow, colorHigh, colorCenter color.NRGBA

	// Cache layout calculations
	middleHeight     float32
	heightOneThird   float32
	heightOneSeventh float32
	stepFactor       float32

	// Fast float formatting
	fmtPrec int
	buf     []byte

	// Cached monospace glyph width for the current display TextSize
	charWidth float32
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
		peakLoVal:  cfg.Center,
		peakHiVal:  cfg.Center,
		valueRange: cfg.Max - cfg.Min,
		fmtPrec:    -1,
		// Modern bars are solid; the classic ones let the ticks show through
		colorLow:    color.NRGBA{0x26, 0xCC, 0x00, 0xFF},
		colorHigh:   color.NRGBA{0xD0, 0x32, 0x2A, 0xFF},
		colorCenter: color.NRGBA{0xFC, 0xBA, 0x03, 0xFF},
	}
	if cfg.Classic {
		s.colorLow = color.NRGBA{0x26, 0xCC, 0x00, 0x80}
		s.colorHigh = color.NRGBA{0xA5, 0x00, 0x00, 0x80}
		s.colorCenter = color.NRGBA{0xFC, 0xBA, 0x03, 0x80}
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
	textColor := widgets.TextPrimary
	if s.cfg.Classic {
		textColor = color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
		// Thin outline around the bar, drawn on top of it
		s.face = &canvas.Rectangle{
			StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},
			FillColor:   color.RGBA{0x00, 0x00, 0x00, 0x00},
			StrokeWidth: 3,
		}
	} else {
		// Modern: a dark rounded track the deviation bar fills from the center
		s.face = &canvas.Rectangle{FillColor: widgets.TrackColor}
	}

	s.bar = &canvas.Rectangle{
		FillColor: s.colorCenter,
	}

	// NRGBA, not RGBA: partial alpha with full channels is invalid premultiplied
	// and renders black on the software rasterizer.
	peakColor := color.NRGBA{0xFF, 0xFF, 0xFF, 0xE0}
	if s.cfg.Classic {
		peakColor = color.NRGBA{0xFF, 0xFF, 0xFF, 0xA0}
	}
	s.peakLo = &canvas.Rectangle{FillColor: peakColor}
	s.peakHi = &canvas.Rectangle{FillColor: peakColor}

	s.titleText = &canvas.Text{
		Text:      s.cfg.Title,
		Color:     textColor,
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
		Color:     textColor,
		TextSize:  float32(s.cfg.DisplayTextSize),
		TextStyle: fyne.TextStyle{Monospace: true},
		Alignment: fyne.TextAlignLeading,
	}

	for i := 0; i <= s.cfg.Steps; i++ {
		tick := widgets.TickMajor
		switch {
		case s.cfg.Classic:
			tick = color.RGBA{0x00, 0xE5, 0x00, 0xFF}
		case i%2 != 0:
			tick = widgets.TickMinor
		}
		line := &canvas.Line{
			StrokeColor: tick,
			StrokeWidth: 2,
		}
		s.bars = append(s.bars, line)
	}
}

// applyBar positions, sizes and colors the deviation bar from the current value.
func (s *CBar) applyBar() {
	barPosition := s.center
	var pxWidth float32
	switch {
	case s.value < s.cfg.Center:
		s.bar.FillColor = s.colorLow
		s.barWidth = float32(s.cfg.Center - s.value)
		pxWidth = s.barWidth * s.widthFactor
		barPosition -= pxWidth
	case s.value > s.cfg.Center:
		s.bar.FillColor = s.colorHigh
		s.barWidth = float32(s.value - s.cfg.Center)
		pxWidth = s.barWidth * s.widthFactor
	default:
		s.bar.FillColor = s.colorCenter
		barPosition -= 3
		s.barWidth = 6 / s.widthFactor
		pxWidth = 6
	}

	s.bar.Move(fyne.Position{X: barPosition, Y: 0})
	s.bar.Resize(fyne.Size{Width: pxWidth, Height: s.barHeight})
	s.applyPeaks()
}

// syncPeaks latches the current value into whichever marker it exceeds and
// decays both back toward center. Reports whether either moved.
func (s *CBar) syncPeaks() bool {
	lo, hi := s.peakLoVal, s.peakHiVal
	if s.value < lo {
		lo = s.value
	} else {
		lo += (s.cfg.Center - lo) * peakDecay
	}
	if s.value > hi {
		hi = s.value
	} else {
		hi += (s.cfg.Center - hi) * peakDecay
	}
	if lo == s.peakLoVal && hi == s.peakHiVal {
		return false
	}
	s.peakLoVal, s.peakHiVal = lo, hi
	return true
}

func (s *CBar) applyPeaks() {
	width := max(float32(2), s.lastSize.Width*0.008)
	for _, p := range [2]struct {
		rect *canvas.Rectangle
		val  float64
	}{{s.peakLo, s.peakLoVal}, {s.peakHi, s.peakHiVal}} {
		x := s.center + float32(p.val-s.cfg.Center)*s.widthFactor - width*0.5
		x = min(max(x, 0), max(s.lastSize.Width-width, 0))
		p.rect.Move(fyne.Position{X: x, Y: 0})
		p.rect.Resize(fyne.Size{Width: width, Height: s.barHeight})
	}
}

func (s *CBar) SetValue(value float64) {
	value = max(s.cfg.Min, min(s.cfg.Max, value))
	changed := value != s.value
	s.value = value

	// Peaks keep decaying even when the value is unchanged, so a latched
	// excursion never freezes on the face.
	if !s.syncPeaks() && !changed {
		return
	}
	s.applyBar()
	if !changed {
		return
	}

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

	dotIdx := -1
	for i := 0; i < len(text); i++ {
		if text[i] == '.' {
			dotIdx = i
			break
		}
	}

	var x float32
	if dotIdx >= 0 {
		x = s.lastSize.Width*0.5 - s.charWidth*(float32(dotIdx)+0.5)
	} else {
		x = s.lastSize.Width*0.5 - s.charWidth*float32(len(text))*0.5
	}

	s.displayText.Move(fyne.Position{X: x, Y: s.displayY})
}

func (s *CBar) CreateRenderer() fyne.WidgetRenderer {
	// Initialize visual elements
	s.initializeVisualElements()
	return &CBarRenderer{CBar: s}
}

type CBarRenderer struct {
	*CBar
	objects []fyne.CanvasObject
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

	if r.cfg.Classic {
		r.face.Move(fyne.NewPos(-2, 0))
		r.face.Resize(space.AddWidthHeight(3, 0))
	} else {
		radius := space.Height * widgets.BarCornerFrac
		r.face.CornerRadius = radius
		r.bar.CornerRadius = radius
		r.face.Move(fyne.Position{})
		r.face.Resize(space)
	}

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

	r.applyBar()

	r.displayText.TextSize = r.bar.Size().Height - 8
	r.charWidth = fyne.MeasureText("0", r.displayText.TextSize, r.displayText.TextStyle).Width

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
	if r.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(r.bars)+4)
		if !r.cfg.Classic {
			// Modern: the track sits under the bar, ticks on top of both
			objs = append(objs, r.face, r.bar)
		}
		for _, line := range r.bars {
			objs = append(objs, line)
		}
		if r.cfg.Classic {
			objs = append(objs, r.bar, r.face)
		}
		objs = append(objs, r.peakLo, r.peakHi, r.titleText, r.displayText)
		r.objects = objs
	}
	return r.objects
}
