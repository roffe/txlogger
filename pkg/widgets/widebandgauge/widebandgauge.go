// Package widebandgauge renders an aftermarket-style wideband lambda display:
// a ring of segments around the top, a scale printed inside them and a large
// digital readout in the middle. One segment lights at a time to show the
// current reading.
package widebandgauge

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

// Segment ring geometry, as fractions of the gauge radius. The track band is
// the dial's ring exactly, so the two gauges are interchangeable on the face.
const (
	arcOuter = 1.0
	// Same outer edge as the dial ring, grown inwards by a third so the
	// segments have room to read as individual bars.
	band     = (arcOuter - widgets.DialRingCutout) * 4 / 3
	arcInner = arcOuter - band

	segInner  = arcInner + band*0.12 // segments sit inside the track with a hairline margin
	segOuter  = arcOuter - band*0.12
	segFill   = 0.65 // of the available arc per segment; the rest is the gap
	maxLabels = 8

	// Fixed LED layout, rich end first: amber over the rich range, green
	// around stoich, red over the lean range.
	ledsRich   = 8
	ledsStoich = 14
	ledsLean   = 8

	// Degrees of band the rounded arc end curves away at each tip:
	// corner radius (half band thickness) as an angle at the band's mid radius.
	arcEndPad = (arcOuter - arcInner) / (arcOuter + arcInner) * (180 / math.Pi)
)

// The track arc, labels and the readout follow the shared modern gauge
// palette so the widget sits next to dial/dualdial without standing out;
// every LED keeps its zone color, dimmed when unlit so the rich/stoich/lean
// bands stay visible.
var (
	labelGray = color.RGBA{0xE0, 0xE0, 0xE0, 0xFF} // same as dial scale labels
	richAmber = color.RGBA{0xFF, 0xA0, 0x00, 0xFF}
	stoichGrn = color.RGBA{0x2E, 0xE0, 0x40, 0xFF}
	leanRed   = color.RGBA{0xE8, 0x2A, 0x1E, 0xFF}
)

type WidebandGauge struct {
	widget.BaseWidget

	cfg   *widgets.GaugeConfig
	value float64
	lit   int // index of the currently lit segment

	arc         *canvas.Arc // background track behind the segments, dial-style
	segments    []*canvas.Line
	labels      []*canvas.Text // nil where a segment carries no label
	titleText   *canvas.Text
	displayText *canvas.Text

	// Precomputed per-segment trig, size independent
	segSin, segCos []float32

	size    fyne.Size
	middle  fyne.Position
	radius  float32
	invSpan float64 // 1/(Max-Min)

	fmtPrec int
	buf     []byte
}

func New(cfg *widgets.GaugeConfig) *WidebandGauge {
	if cfg.Min >= cfg.Max {
		cfg.Min, cfg.Max = 0.5, 1.5
	}
	if cfg.Center <= cfg.Min || cfg.Center >= cfg.Max {
		cfg.Center = (cfg.Min + cfg.Max) * 0.5
	}
	// Fixed 30-LED layout; the segment count is not configurable.
	cfg.Steps = ledsRich + ledsStoich + ledsLean - 1
	if cfg.DisplayString == "" {
		cfg.DisplayString = "%.2f"
	}
	if cfg.MinSize.Width <= 0 || cfg.MinSize.Height <= 0 {
		cfg.MinSize = fyne.NewSize(150, 150)
	}
	if cfg.GaugeFactor == 0 {
		cfg.GaugeFactor = 1
	}

	g := &WidebandGauge{
		cfg:     cfg,
		value:   cfg.Center,
		invSpan: 1 / (cfg.Max - cfg.Min),
		fmtPrec: -1,
	}
	if n := common.ParseFixedPrec(cfg.DisplayString); n >= 0 {
		g.fmtPrec = n
	}
	// Scale labels default to one decimal — a lambda scale reads 0.8/0.9/1.0.
	labelPrec := 1
	if n := common.ParseFixedPrec(cfg.GaugeTextString); n >= 0 {
		labelPrec = n
	}

	// Track ring like dial/dualdial. The ends are widened by half a segment
	// plus the corner rounding so the outermost segments sit on full band.
	endPad := float32(widgets.DialSweepDeg/float64(cfg.Steps)*segFill*0.5 + arcEndPad)
	g.arc = canvas.NewArc(widgets.DialStartDeg-endPad, widgets.DialEndDeg+endPad, arcInner/arcOuter, widgets.TrackColor)

	// Same title placement as dial/dualdial, so "λ" reads the same everywhere.
	g.titleText = &canvas.Text{
		Text:      cfg.Title,
		Color:     widgets.TextPrimary,
		TextStyle: fyne.TextStyle{Monospace: true},
		Alignment: fyne.TextAlignCenter,
	}

	g.displayText = &canvas.Text{
		Text:      "0",
		Color:     widgets.TextPrimary,
		TextStyle: fyne.TextStyle{Monospace: true},
		Alignment: fyne.TextAlignCenter,
	}

	// Label every Nth segment so the scale stays readable at any segment count.
	// Never denser than every second segment — adjacent labels crowd and, on a
	// fine scale, round to the same text.
	every := max(2, (cfg.Steps+1)/maxLabels)
	steps := float64(cfg.Steps)
	for i := 0; i <= cfg.Steps; i++ {
		g.segments = append(g.segments, &canvas.Line{StrokeColor: dim(zoneColor(i))})

		var lbl *canvas.Text
		if i%every == 0 {
			val := (cfg.Min + (float64(i)/steps)*(cfg.Max-cfg.Min)) * cfg.GaugeFactor
			lbl = &canvas.Text{
				Text:      strconv.FormatFloat(val, 'f', labelPrec, 64),
				Color:     labelGray,
				Alignment: fyne.TextAlignCenter,
			}
		}
		g.labels = append(g.labels, lbl)

		ang := common.Pi15*float64(i)/steps - common.Pi43
		s, c := math.Sincos(ang)
		g.segSin = append(g.segSin, float32(s))
		g.segCos = append(g.segCos, float32(c))
	}

	g.lit = g.segmentFor(g.value)
	g.segments[g.lit].StrokeColor = zoneColor(g.lit)

	g.ExtendBaseWidget(g)
	return g
}

func (g *WidebandGauge) GetConfig() *widgets.GaugeConfig { return g.cfg }

func (g *WidebandGauge) segmentFor(value float64) int {
	frac := (value - g.cfg.Min) * g.invSpan
	idx := int(math.Round(min(max(frac, 0), 1) * float64(g.cfg.Steps)))
	return min(max(idx, 0), g.cfg.Steps)
}

// zoneColor is an LED's fixed color by position on the scale, rich end first.
func zoneColor(i int) color.RGBA {
	switch {
	case i < ledsRich:
		return richAmber
	case i < ledsRich+ledsStoich:
		return stoichGrn
	default:
		return leanRed
	}
}

// dim is the unlit shade of an LED: dark, but still tinted its zone color.
func dim(c color.RGBA) color.RGBA {
	return color.RGBA{c.R / 5, c.G / 5, c.B / 5, 0xFF}
}

func (g *WidebandGauge) SetValue(value float64) {
	if value == g.value {
		return
	}
	g.value = value

	if idx := g.segmentFor(value); idx != g.lit {
		g.segments[g.lit].StrokeColor = dim(zoneColor(g.lit))
		canvas.Refresh(g.segments[g.lit])
		g.lit = idx
		g.segments[g.lit].StrokeColor = zoneColor(g.lit)
		canvas.Refresh(g.segments[g.lit])
	}

	g.buf = g.buf[:0]
	if g.fmtPrec >= 0 {
		g.buf = strconv.AppendFloat(g.buf, value, 'f', g.fmtPrec, 64)
	} else {
		g.buf = common.AppendFormatFloat(g.buf, g.cfg.DisplayString, value)
	}
	if !common.SameTextBytes(g.displayText.Text, g.buf) {
		g.displayText.Text = string(g.buf)
		canvas.Refresh(g.displayText)
	}
}

func (g *WidebandGauge) CreateRenderer() fyne.WidgetRenderer {
	objs := make([]fyne.CanvasObject, 0, len(g.segments)+len(g.labels)+3)
	objs = append(objs, g.arc)
	for _, s := range g.segments {
		objs = append(objs, s)
	}
	for _, l := range g.labels {
		if l != nil {
			objs = append(objs, l)
		}
	}
	objs = append(objs, g.titleText, g.displayText)
	return &renderer{g: g, objects: objs}
}

type renderer struct {
	g       *WidebandGauge
	objects []fyne.CanvasObject
}

func (r *renderer) Layout(space fyne.Size) {
	g := r.g
	if g.size == space {
		return
	}
	g.size = space

	diameter := min(space.Width, space.Height)
	g.radius = diameter * common.OneHalf
	g.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)

	// Track arc: rounded ends like the dial ring (corner radius = half band thickness)
	arcR := g.radius * arcOuter
	g.arc.CornerRadius = (arcOuter - arcInner) * g.radius * common.OneHalf
	g.arc.Move(fyne.Position{X: g.middle.X - arcR, Y: g.middle.Y - arcR})
	g.arc.Resize(fyne.NewSize(arcR*2, arcR*2))

	inner, outer := g.radius*segInner, g.radius*segOuter
	// Segment thickness fills most of the arc it owns, leaving a visible gap.
	midR := (inner + outer) * common.OneHalf
	stroke := max(float32(2), float32(common.Pi15)*midR/float32(g.cfg.Steps)*segFill)

	// Scale labels sit where the dial's do (dial: tick base 0.72r, pad 0.14r)
	labelSize := g.radius * 0.10
	labelRadius := g.radius*0.72 - max(float32(6), g.radius*0.14)
	labelBoxW, labelBoxH := labelSize*3, labelSize*1.3

	for i, seg := range g.segments {
		sin, cos := g.segSin[i], g.segCos[i]
		seg.StrokeWidth = stroke
		seg.Position1 = fyne.Position{X: g.middle.X + sin*inner, Y: g.middle.Y - cos*inner}
		seg.Position2 = fyne.Position{X: g.middle.X + sin*outer, Y: g.middle.Y - cos*outer}

		if lbl := g.labels[i]; lbl != nil {
			lbl.TextSize = labelSize
			lbl.Resize(fyne.NewSize(labelBoxW, labelBoxH))
			lbl.Move(fyne.Position{
				X: g.middle.X + sin*labelRadius - labelBoxW*common.OneHalf,
				Y: g.middle.Y - cos*labelRadius - labelBoxH*common.OneHalf,
			})
		}
	}

	g.titleText.TextSize = float32(int(g.radius * common.OneFourth))
	g.titleText.Move(g.middle.Add(fyne.NewPos(0, diameter*common.OneFourth)))

	// Readout owns the middle; fyne centers the text inside the box.
	g.displayText.TextSize = float32(int(g.radius * common.OneThird))
	g.displayText.Move(fyne.Position{X: g.middle.X - g.radius, Y: g.middle.Y - g.radius})
	g.displayText.Resize(fyne.NewSize(diameter, diameter))
}

func (r *renderer) MinSize() fyne.Size           { return r.g.cfg.MinSize }
func (r *renderer) Refresh()                     {}
func (r *renderer) Destroy()                     {}
func (r *renderer) Objects() []fyne.CanvasObject { return r.objects }
