package dial

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

type Dial struct {
	widget.BaseWidget
	displayString string

	cfg *widgets.GaugeConfig

	value float64

	needle *canvas.Line

	pips      []*canvas.Line
	pipLabels []*canvas.Text

	face        *canvas.Arc
	valueArc    *canvas.Arc
	center      *canvas.Circle
	displayText *canvas.Text
	titleText   *canvas.Text

	size    fyne.Size
	minsize fyne.Size

	diameter                   float32
	radius                     float32
	middle                     fyne.Position
	needleOffset, needleLength float32
	needleRotConst             float64 // = common.Pi15/(Max-Min)
	lineRotConst               float64 // = common.Pi15/steps
	invRange                   float64 // = 1/(Max-Min), for value-arc fraction

	// Precomputed trig for pips: angle_i = lineRotConst*float64(i) - common.Pi43
	pipSin []float32
	pipCos []float32

	// Fast float formatting
	fmtPrec   int // precision extracted from displayString like "%.0f", "%.1f", defaults to -1
	gaugePrec int // precision extracted from GaugeTextString like "%.0f", "%.1f", defaults to -1
	buf       []byte

	// Label sizing cache (avoids MinSize calls every layout)
	maxLabelChars int     // longest label length at construction
	labelBoxW     float32 // computed each layout from TextSize and maxLabelChars
	labelBoxH     float32 // computed each layout from TextSize
}

func New(cfg *widgets.GaugeConfig) *Dial {
	c := &Dial{
		cfg:           cfg,
		displayString: "%.0f",
		minsize:       fyne.NewSize(100, 100),
		fmtPrec:       -1,
	}
	c.ExtendBaseWidget(c)

	if cfg.DisplayString != "" {
		c.displayString = cfg.DisplayString
		if n := common.ParseFixedPrec(c.displayString); n >= 0 {
			c.fmtPrec = n
		}
	}
	if cfg.GaugeTextString != "" {
		if n := common.ParseFixedPrec(cfg.GaugeTextString); n >= 0 {
			c.gaugePrec = n
		}
	}
	if cfg.GaugeFactor == 0 {
		cfg.GaugeFactor = 1.0
	}
	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		c.minsize = cfg.MinSize
	}

	steps := float64(cfg.Steps)
	totalRange := c.cfg.Max - c.cfg.Min
	if totalRange <= 0 {
		totalRange = 1
	}

	if cfg.Classic {
		c.face = canvas.NewArc(-135.73, 135.8, 0.985, color.RGBA{0x80, 0x80, 0x80, 0xFF})
		c.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
		c.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 3}
	} else {
		// Track ring: thick, dark, rounded ends. The value arc fills it as the
		// value rises — no needle or center hub in the modern style.
		c.face = canvas.NewArc(widgets.DialStartDeg, widgets.DialEndDeg, widgets.DialRingCutout, color.RGBA{R: 0x2E, G: 0x2E, B: 0x35, A: 0xFF})
		c.valueArc = canvas.NewArc(widgets.DialStartDeg, widgets.DialStartDeg, widgets.DialRingCutout, widgets.ZoneColor(0))
	}

	c.titleText = &canvas.Text{Text: c.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	c.titleText.TextStyle.Monospace = true
	c.titleText.Alignment = fyne.TextAlignCenter

	displayColor := color.RGBA{R: 0xF5, G: 0xF5, B: 0xF7, A: 0xFF}
	if cfg.Classic {
		displayColor = color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}
	}
	c.displayText = &canvas.Text{Text: "0", Color: displayColor, TextSize: 52}
	c.displayText.Alignment = fyne.TextAlignCenter

	// Build pips + labels once; also track the longest label length.
	// Modern ticks are neutral — the value arc carries the green→yellow→red zone
	// color. Classic keeps the gradient on the pips themselves.
	majorTick := color.RGBA{R: 0x8A, G: 0x8A, B: 0x92, A: 0xFF}
	minorTick := color.RGBA{R: 0x55, G: 0x55, B: 0x5C, A: 0xFF}
	for i := 0; i < c.cfg.Steps+1; i++ {
		var col color.RGBA
		switch {
		case cfg.Classic:
			col = widgets.ZoneColor(float64(i) / steps)
		case i%2 == 0:
			col = majorTick
		default:
			col = minorTick
		}
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		c.pips = append(c.pips, pip)

		if i%2 == 0 {
			val := c.cfg.Min + (float64(i)/float64(c.cfg.Steps))*(c.cfg.Max-c.cfg.Min)*c.cfg.GaugeFactor
			txt := strconv.FormatFloat(val, 'f', c.gaugePrec, 64)

			lbl := &canvas.Text{
				Text:      txt,
				Color:     color.RGBA{0xE0, 0xE0, 0xE0, 0xFF},
				Alignment: fyne.TextAlignCenter,
			}
			if n := len(txt); n > c.maxLabelChars {
				c.maxLabelChars = n
			}
			c.pipLabels = append(c.pipLabels, lbl)
		} else {
			c.pipLabels = append(c.pipLabels, nil)
		}
	}

	// Constants
	c.needleRotConst = common.Pi15 / totalRange
	c.lineRotConst = common.Pi15 / steps
	c.invRange = 1 / totalRange

	// Precompute pip sin/cos (size-independent)
	c.pipSin = make([]float32, c.cfg.Steps+1)
	c.pipCos = make([]float32, c.cfg.Steps+1)
	for i := 0; i <= c.cfg.Steps; i++ {
		ang := c.lineRotConst*float64(i) - common.Pi43
		s, co := math.Sincos(ang)
		c.pipSin[i] = float32(s)
		c.pipCos[i] = float32(co)
	}

	return c
}

func (c *Dial) GetConfig() *widgets.GaugeConfig { return c.cfg }

// rotate angle (in radians) without refreshing per-object
func (c *Dial) rotateNoRefresh(hand *canvas.Line, rotation float64, offset, length float32) {
	s, co := math.Sincos(rotation)
	c.applySinCos(hand, float32(s), float32(co), offset, length)
}

func (c *Dial) applySinCos(hand *canvas.Line, sinRot, cosRot float32, offset, length float32) {
	x2 := length * sinRot
	y2 := -length * cosRot
	offX := offset * sinRot
	offY := -offset * cosRot
	midxOffX := c.middle.X + offX
	midY := c.middle.Y + offY
	hand.Position1 = fyne.Position{X: midxOffX, Y: midY}
	hand.Position2 = fyne.Position{X: midxOffX + x2, Y: midY + y2}
}

func (c *Dial) rotateNeedleNoRefresh(hand *canvas.Line, facePosition float64, offset, length float32) {
	normalized := facePosition - c.cfg.Min
	if normalized < 0 {
		normalized = 0
	}
	c.rotateNoRefresh(hand, c.needleRotConst*normalized-common.Pi43, offset, length)
}

// syncValueArcNoRefresh recomputes the value arc sweep and color from the
// current value; reports whether the arc changed.
func (c *Dial) syncValueArcNoRefresh() bool {
	frac := (c.value - c.cfg.Min) * c.invRange
	frac = min(max(frac, 0), 1)
	end := float32(widgets.DialStartDeg + widgets.DialSweepDeg*frac)
	if end == c.valueArc.EndAngle {
		return false
	}
	c.valueArc.EndAngle = end
	c.valueArc.FillColor = widgets.ZoneColor(frac)
	return true
}

func (c *Dial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value

	// Update needle position (no immediate refresh)
	if c.needle != nil {
		c.rotateNeedleNoRefresh(c.needle, value, c.needleOffset, c.needleLength)
	}

	// Value arc is the indicator in the modern style; color shifts through the zones
	if c.valueArc != nil && c.syncValueArcNoRefresh() {
		canvas.Refresh(c.valueArc)
	}

	// Update text with minimal allocs; skip refresh if formatted output is unchanged
	c.buf = c.buf[:0]
	if c.fmtPrec >= 0 {
		c.buf = strconv.AppendFloat(c.buf, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf = common.AppendFormatFloat(c.buf, c.displayString, value)
	}
	textChanged := !common.SameTextBytes(c.displayText.Text, c.buf)
	if textChanged {
		c.displayText.Text = string(c.buf)
	}

	if c.needle != nil {
		canvas.Refresh(c.needle)
	}
	if textChanged {
		canvas.Refresh(c.displayText)
	}
}

func (c *Dial) CreateRenderer() fyne.WidgetRenderer {
	objs := make([]fyne.CanvasObject, 0, len(c.pips)+len(c.pipLabels)+6)
	if c.valueArc != nil {
		objs = append(objs, c.face, c.valueArc)
	}
	for _, v := range c.pips {
		objs = append(objs, v)
	}
	for _, t := range c.pipLabels {
		if t != nil {
			objs = append(objs, t)
		}
	}
	if c.cfg.Classic {
		// Classic z-order: the rim arc paints over the pip tips, as before
		objs = append(objs, c.face, c.titleText, c.center, c.needle, c.displayText)
	} else {
		objs = append(objs, c.titleText, c.displayText)
	}
	return &DialRenderer{d: c, objects: objs}
}

type DialRenderer struct {
	d       *Dial
	objects []fyne.CanvasObject
}

func (r *DialRenderer) Layout(space fyne.Size) {
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

	// Stroke sizes
	midStroke := c.diameter * common.OneEighthieth
	smallStroke := c.diameter * common.OneTwohundredth

	size := fyne.Size{Width: c.diameter, Height: c.diameter}
	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	// Title (no rounding needed)
	c.titleText.TextSize = float32(int(c.radius * common.OneFourth))
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))

	// Center element (classic only)
	if c.center != nil {
		center := c.radius * common.OneFourth
		c.center.Move(c.middle.SubtractXY(center*common.OneHalf, center*common.OneHalf))
		c.center.Resize(fyne.Size{Width: center, Height: center})
	}

	// Display text
	c.displayText.TextSize = float32(int(c.radius * common.OneThird))
	c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneFifth))
	c.displayText.Resize(size)

	// Needle (classic only)
	if c.needle != nil {
		c.needle.StrokeWidth = c.diameter * common.OneSixthieth
		c.rotateNeedleNoRefresh(c.needle, c.value, c.needleOffset, c.needleLength)
	}

	facePos := c.middle.SubtractXY(c.radius, c.radius)
	c.face.Move(facePos)
	c.face.Resize(size)
	if c.valueArc != nil {
		ringCorner := c.radius * (1 - widgets.DialRingCutout) * common.OneHalf
		c.face.CornerRadius = ringCorner
		c.valueArc.CornerRadius = ringCorner
		c.valueArc.Move(facePos)
		c.valueArc.Resize(size)
		// Layout may run before any SetValue; keep the arc in step with c.value
		c.syncValueArcNoRefresh()
	}

	// Pips: reuse precomputed sin/cos, scale with current radii.
	// Modern ticks live just inside the ring (ring spans 0.87r..1.0r);
	// classic ticks run out to the rim like before.
	var majorOffset, majorLen, minorOffset, minorLen float32
	if c.cfg.Classic {
		majorLen = c.radius*common.OneFourth - 2
		minorLen = c.radius*common.OneEight - 2
		majorOffset = c.radius * common.OneFourth * 3
		minorOffset = c.radius * common.OneEight * 7
	} else {
		tickOuter := c.radius * 0.85
		majorLen = c.radius * 0.13
		minorLen = c.radius * 0.06
		majorOffset = tickOuter - majorLen
		minorOffset = tickOuter - minorLen
	}

	// Label padding and cached box dims (avoid lbl.MinSize per label)
	labelPad := max(float32(6.0), c.radius*0.14)

	// Assume monospace, digits only: width ≈ chars * 0.62 * TextSize; height ≈ 1.15 * TextSize
	// This keeps alignment stable and removes per-label measuring.
	const charWidthFactor = 0.62
	const heightFactor = 1.15

	labelTextSize := c.radius * 0.10
	c.labelBoxW = float32(c.maxLabelChars) * float32(charWidthFactor) * labelTextSize
	c.labelBoxH = float32(heightFactor) * labelTextSize

	for i, p := range c.pips {
		if i%2 == 0 {
			p.StrokeWidth = max(2.0, midStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], majorOffset, majorLen)

			// Label for long pip
			lbl := c.pipLabels[i]
			if lbl != nil {
				lbl.TextSize = labelTextSize

				// Place label on the INSIDE of the gauge
				labelRadius := majorOffset - labelPad
				cx := c.middle.X + c.pipSin[i]*labelRadius
				cy := c.middle.Y - c.pipCos[i]*labelRadius

				boxW := c.labelBoxW
				boxH := c.labelBoxH
				lbl.Resize(fyne.NewSize(boxW, boxH))
				// lbl.Move(fyne.NewPos(cx-boxW/2, cy-boxH/2))
				lbl.Move(fyne.Position{X: cx - boxW/2, Y: cy - boxH/2})
			}
		} else {
			p.StrokeWidth = max(2.0, smallStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], minorOffset, minorLen)
		}
	}
}

func (r *DialRenderer) MinSize() fyne.Size { return r.d.minsize }
func (r *DialRenderer) Refresh()           {}
func (r *DialRenderer) Destroy()           {}

func (r *DialRenderer) Objects() []fyne.CanvasObject { return r.objects }
