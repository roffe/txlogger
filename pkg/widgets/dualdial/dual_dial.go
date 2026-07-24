package dualdial

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

// Modern twin-ring geometry, as fractions of the dial radius: the outer,
// thinner ring shows the secondary value; the inner, thicker ring the primary.
const (
	innerScale  = 0.89 // outer radius of the inner (primary) ring
	innerCutout = 1 - 0.13/innerScale
	outerCutout = 0.92
)

type DualDial struct {
	widget.BaseWidget

	cfg *widgets.GaugeConfig

	titleText     *canvas.Text
	displayString string

	value  float64
	value2 float64

	needle  *canvas.Line
	needle2 *canvas.Line

	pips      []*canvas.Line
	pipLabels []*canvas.Text

	face      *canvas.Arc // classic rim, or modern inner (primary) track
	valueArc  *canvas.Arc // modern inner (primary) value arc
	face2     *canvas.Arc // modern outer (secondary) track
	valueArc2 *canvas.Arc // modern outer (secondary) value arc
	center    *canvas.Circle

	displayText  *canvas.Text
	displayText2 *canvas.Text

	steps  float64
	factor float64

	size    fyne.Size
	minsize fyne.Size

	diameter                   float32
	radius                     float32
	middle                     fyne.Position
	needleOffset, needleLength float32
	needleRotConst             float64
	lineRotConst               float64
	invRange                   float64 // = 1/(Max-Min), for value-arc fraction

	// cached sin/cos for pips (angle_i = lineRotConst*i - common.Pi43)
	pipSin []float32
	pipCos []float32

	// fast float formatting buffers
	fmtPrec   int
	gaugePrec int
	buf1      []byte
	buf2      []byte

	// Label sizing cache (avoid per-label MinSize on each layout)
	maxLabelChars int
	labelBoxW     float32
	labelBoxH     float32
}

func New(cfg *widgets.GaugeConfig) *DualDial {
	s := &DualDial{
		cfg:           cfg,
		steps:         10,
		displayString: "%.0f",
		minsize:       fyne.NewSize(100, 100),
		fmtPrec:       -1,
	}
	s.ExtendBaseWidget(s)

	if cfg.Steps > 0 {
		s.steps = float64(cfg.Steps)
	}
	if cfg.DisplayString != "" {
		s.displayString = cfg.DisplayString
		if n := common.ParseFixedPrec(s.displayString); n >= 0 {
			s.fmtPrec = n
		}
	}
	if cfg.GaugeTextString != "" {
		if n := common.ParseFixedPrec(cfg.GaugeTextString); n >= 0 {
			s.gaugePrec = n
		}
	}
	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		s.minsize = cfg.MinSize
	}

	s.factor = s.cfg.Max / s.steps

	if cfg.Classic {
		s.face = canvas.NewArc(-135.73, 135.8, 0.985, color.RGBA{0x80, 0x80, 0x80, 0xFF})
		s.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
		s.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 2}
		s.needle2 = &canvas.Line{StrokeColor: color.RGBA{R: 249, G: 27, B: 2, A: 255}, StrokeWidth: 2}
	} else {
		// Twin track rings, no needles or center hub: the inner thick ring fills
		// with the primary value (zone colored), the outer thin ring with the
		// secondary value (red, matching its readout).
		trackColor := color.RGBA{R: 0x2E, G: 0x2E, B: 0x35, A: 0xFF}
		s.face = canvas.NewArc(widgets.DialStartDeg, widgets.DialEndDeg, innerCutout, trackColor)
		s.valueArc = canvas.NewArc(widgets.DialStartDeg, widgets.DialStartDeg, innerCutout, widgets.ZoneColor(0))
		s.face2 = canvas.NewArc(widgets.DialStartDeg, widgets.DialEndDeg, outerCutout, trackColor)
		s.valueArc2 = canvas.NewArc(widgets.DialStartDeg, widgets.DialStartDeg, outerCutout, color.RGBA{R: 249, G: 27, B: 2, A: 255})
	}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	displayColor := color.RGBA{R: 0xF5, G: 0xF5, B: 0xF7, A: 0xFF}
	if cfg.Classic {
		displayColor = color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}
	}
	s.displayText = &canvas.Text{Text: "0", Color: displayColor, TextSize: 52}
	s.displayText.Alignment = fyne.TextAlignCenter

	s.displayText2 = &canvas.Text{Text: "0", Color: color.RGBA{R: 0xff, G: 0x0, B: 0, A: 0xFF}, TextSize: 35}
	s.displayText2.Alignment = fyne.TextAlignCenter

	// Modern ticks are neutral — the value arc carries the green→yellow→red zone
	// color. Classic keeps the gradient on the pips themselves.
	majorTick := color.RGBA{R: 0x8A, G: 0x8A, B: 0x92, A: 0xFF}
	minorTick := color.RGBA{R: 0x55, G: 0x55, B: 0x5C, A: 0xFF}
	for i := 0; i <= int(s.steps); i++ {
		var col color.RGBA
		switch {
		case cfg.Classic:
			col = widgets.ZoneColor(float64(i) / s.steps)
		case i%2 == 0:
			col = majorTick
		default:
			col = minorTick
		}
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		s.pips = append(s.pips, pip)

		if i%2 == 0 {
			val := s.cfg.Min + (float64(i)/float64(s.cfg.Steps))*(s.cfg.Max-s.cfg.Min)
			txt := strconv.FormatFloat(val, 'f', s.gaugePrec, 64)
			lbl := &canvas.Text{
				Text:      txt,
				Color:     color.RGBA{0xE0, 0xE0, 0xE0, 0xFF},
				Alignment: fyne.TextAlignCenter,
			}
			// lbl.TextStyle.Monospace = true
			if n := len(txt); n > s.maxLabelChars {
				s.maxLabelChars = n
			}
			s.pipLabels = append(s.pipLabels, lbl)
		} else {
			s.pipLabels = append(s.pipLabels, nil)
		}
	}

	totalRange := s.cfg.Max - s.cfg.Min
	if totalRange <= 0 {
		totalRange = 1
	}
	s.needleRotConst = common.Pi15 / totalRange
	s.lineRotConst = common.Pi15 / s.steps
	s.invRange = 1 / totalRange

	// precompute pip trig (size independent)
	s.pipSin = make([]float32, int(s.steps)+1)
	s.pipCos = make([]float32, int(s.steps)+1)
	for i := 0; i <= int(s.steps); i++ {
		ang := s.lineRotConst*float64(i) - common.Pi43
		sinA, cosA := math.Sincos(ang)
		s.pipSin[i] = float32(sinA)
		s.pipCos[i] = float32(cosA)
	}

	return s
}

func (c *DualDial) GetConfig() *widgets.GaugeConfig { return c.cfg }

func (c *DualDial) rotateNeedleNoRefresh(hand *canvas.Line, facePosition float64, offset, length float32) {
	normalized := facePosition - c.cfg.Min
	if normalized < 0 {
		normalized = 0
	}
	s, co := math.Sincos(c.needleRotConst*normalized - common.Pi43)
	c.applySinCos(hand, float32(s), float32(co), offset, length)
}

func (c *DualDial) applySinCos(hand *canvas.Line, sinRot, cosRot float32, offset, length float32) {
	x2 := length * sinRot
	y2 := -length * cosRot
	offX := offset * sinRot
	offY := -offset * cosRot
	midxOffX := c.middle.X + offX
	midY := c.middle.Y + offY
	hand.Position1 = fyne.Position{X: midxOffX, Y: midY}
	hand.Position2 = fyne.Position{X: midxOffX + x2, Y: midY + y2}
}

// syncArcNoRefresh recomputes a value arc's sweep from value; zone also updates
// the fill color through the green→yellow→red gradient. Reports whether the
// arc changed.
func (c *DualDial) syncArcNoRefresh(arc *canvas.Arc, value float64, zone bool) bool {
	frac := (value - c.cfg.Min) * c.invRange
	frac = min(max(frac, 0), 1)
	end := float32(widgets.DialStartDeg + widgets.DialSweepDeg*frac)
	if end == arc.EndAngle {
		return false
	}
	arc.EndAngle = end
	if zone {
		arc.FillColor = widgets.ZoneColor(frac)
	}
	return true
}

func (c *DualDial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value

	if c.needle != nil {
		c.rotateNeedleNoRefresh(c.needle, value, c.needleOffset, c.needleLength)
	}

	// Value arc is the primary indicator in the modern style
	if c.valueArc != nil && c.syncArcNoRefresh(c.valueArc, value, true) {
		canvas.Refresh(c.valueArc)
	}

	c.buf1 = c.buf1[:0]
	if c.fmtPrec >= 0 {
		c.buf1 = strconv.AppendFloat(c.buf1, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf1 = common.AppendFormatFloat(c.buf1, c.displayString, value)
	}
	textChanged := !common.SameTextBytes(c.displayText.Text, c.buf1)
	if textChanged {
		c.displayText.Text = string(c.buf1)
	}

	if c.needle != nil {
		canvas.Refresh(c.needle)
	}
	if textChanged {
		canvas.Refresh(c.displayText)
	}
}

func (c *DualDial) SetValue2(value float64) {
	if value == c.value2 {
		return
	}
	c.value2 = value

	if c.needle2 != nil {
		c.rotateNeedleNoRefresh(c.needle2, value, c.needleOffset, c.needleLength)
	}

	// Outer arc is the secondary indicator in the modern style
	if c.valueArc2 != nil && c.syncArcNoRefresh(c.valueArc2, value, false) {
		canvas.Refresh(c.valueArc2)
	}

	c.buf2 = c.buf2[:0]
	if c.fmtPrec >= 0 {
		c.buf2 = strconv.AppendFloat(c.buf2, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf2 = common.AppendFormatFloat(c.buf2, c.displayString, value)
	}
	textChanged := !common.SameTextBytes(c.displayText2.Text, c.buf2)
	if textChanged {
		c.displayText2.Text = string(c.buf2)
	}

	if c.needle2 != nil {
		canvas.Refresh(c.needle2)
	}
	if textChanged {
		canvas.Refresh(c.displayText2)
	}
}

func (c *DualDial) CreateRenderer() fyne.WidgetRenderer {
	objs := make([]fyne.CanvasObject, 0, len(c.pips)+len(c.pipLabels)+9)
	if c.valueArc != nil {
		objs = append(objs, c.face, c.valueArc, c.face2, c.valueArc2)
	}
	for _, v := range c.pips {
		objs = append(objs, v)
	}
	for _, v := range c.pipLabels {
		if v != nil {
			objs = append(objs, v)
		}
	}
	if c.cfg.Classic {
		// Classic z-order: the rim arc paints over the pip tips, as before
		objs = append(objs, c.face, c.titleText, c.center, c.needle2, c.needle, c.displayText, c.displayText2)
	} else {
		objs = append(objs, c.titleText, c.displayText, c.displayText2)
	}
	return &DualDialRenderer{d: c, objects: objs}
}

type DualDialRenderer struct {
	d       *DualDial
	objects []fyne.CanvasObject
}

func (r *DualDialRenderer) Layout(space fyne.Size) {
	c := r.d
	if c.size == space {
		return
	}
	c.size = space

	c.diameter = min(space.Width, space.Height)
	c.radius = c.diameter * common.OneHalf
	c.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)

	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14

	midStroke := c.diameter * common.OneEighthieth
	smallStroke := c.diameter * common.OneTwohundredth

	size := fyne.Size{Width: c.diameter, Height: c.diameter}
	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	// Title & display text sizes (no math.Round needed)
	c.titleText.TextSize = c.radius * common.OneFourth
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))

	// Center element (classic only)
	if c.center != nil {
		center := c.radius * common.OneFourth
		c.center.Move(c.middle.SubtractXY(center*common.OneHalf, center*common.OneHalf))
		c.center.Resize(fyne.Size{Width: center, Height: center})
	}

	sixthDiameter := c.diameter * common.OneSixth

	c.displayText.TextSize = c.radius * common.OneThird
	c.displayText2.TextSize = c.radius * common.OneThird

	if c.cfg.Classic {
		c.displayText.TextSize = c.radius * common.OneThird
		c.displayText2.TextSize = c.radius * common.OneThird
		c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneFifth))
		c.displayText2.Move(topleft.AddXY(0, -sixthDiameter))
	} else {
		c.displayText.TextSize = c.radius * common.OneFourth
		c.displayText2.TextSize = c.radius * common.OneFourth
		c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneTenth))
		c.displayText2.Move(topleft.AddXY(0, -c.diameter*common.OneTwelfth))
	}

	c.displayText.Resize(size)
	c.displayText2.Resize(size)

	// Needles (classic only)
	if c.needle != nil {
		needleStroke := c.diameter * common.OneSixthieth
		c.needle.StrokeWidth = needleStroke
		c.needle2.StrokeWidth = needleStroke
		c.rotateNeedleNoRefresh(c.needle, c.value, c.needleOffset, c.needleLength)
		c.rotateNeedleNoRefresh(c.needle2, c.value2, c.needleOffset, c.needleLength)
	}

	if c.valueArc != nil {
		// Inner (primary) ring
		innerR := c.radius * innerScale
		innerPos := c.middle.SubtractXY(innerR, innerR)
		innerSize := fyne.Size{Width: innerR * 2, Height: innerR * 2}
		innerCorner := innerR * (1 - innerCutout) * common.OneHalf
		c.face.CornerRadius = innerCorner
		c.face.Move(innerPos)
		c.face.Resize(innerSize)
		c.valueArc.CornerRadius = innerCorner
		c.valueArc.Move(innerPos)
		c.valueArc.Resize(innerSize)

		// Outer (secondary) ring
		outerPos := c.middle.SubtractXY(c.radius, c.radius)
		outerCorner := c.radius * (1 - outerCutout) * common.OneHalf
		c.face2.CornerRadius = outerCorner
		c.face2.Move(outerPos)
		c.face2.Resize(size)
		c.valueArc2.CornerRadius = outerCorner
		c.valueArc2.Move(outerPos)
		c.valueArc2.Resize(size)

		// Layout may run before any SetValue; keep the arcs in step with the values
		c.syncArcNoRefresh(c.valueArc, c.value, true)
		c.syncArcNoRefresh(c.valueArc2, c.value2, false)
	} else {
		c.face.Move(c.middle.SubtractXY(c.radius, c.radius))
		c.face.Resize(size)
	}

	// Pips using precomputed trig scaled by current radius.
	// Modern ticks live just inside the inner ring (rings span 0.76r..1.0r);
	// classic ticks run out to the rim like before.
	var majorOffset, majorLen, minorOffset, minorLen float32
	if c.cfg.Classic {
		majorLen = c.radius*common.OneFourth - 1
		minorLen = c.radius*common.OneEight - 1
		majorOffset = c.radius * common.OneFourth * 3
		minorOffset = c.radius * common.OneEight * 7
	} else {
		tickOuter := c.radius * 0.74
		majorLen = c.radius * 0.12
		minorLen = c.radius * 0.06
		majorOffset = tickOuter - majorLen
		minorOffset = tickOuter - minorLen
	}

	// Label padding and cached box dims (avoid lbl.MinSize per label)
	labelPad := max(float32(6.0), c.radius*0.14)
	const charWidthFactor = 0.62
	const heightFactor = 1.15
	labelTextSize := c.radius * 0.10
	c.labelBoxW = float32(c.maxLabelChars) * float32(charWidthFactor) * labelTextSize
	c.labelBoxH = float32(heightFactor) * labelTextSize

	for i, p := range c.pips {
		if i%2 == 0 {
			p.StrokeWidth = max(2.0, midStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], majorOffset, majorLen)
			// Label for long pip (uniform box; no MinSize)
			lbl := c.pipLabels[i]
			if lbl != nil {
				lbl.TextSize = labelTextSize
				// place inside the gauge slightly inward from long pip inner end
				labelRadius := majorOffset - labelPad
				cx := c.middle.X + c.pipSin[i]*labelRadius
				cy := c.middle.Y - c.pipCos[i]*labelRadius
				boxW := c.labelBoxW
				boxH := c.labelBoxH
				lbl.Resize(fyne.NewSize(boxW, boxH))
				lbl.Move(fyne.NewPos(cx-boxW/2, cy-boxH/2))
			}
		} else {
			p.StrokeWidth = max(2.0, smallStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], minorOffset, minorLen)
		}
	}
}

func (r *DualDialRenderer) MinSize() fyne.Size           { return r.d.minsize }
func (r *DualDialRenderer) Refresh()                     {}
func (r *DualDialRenderer) Destroy()                     {}
func (r *DualDialRenderer) Objects() []fyne.CanvasObject { return r.objects }
