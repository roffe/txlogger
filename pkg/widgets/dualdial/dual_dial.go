package dualdial

import (
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
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

	pips   []*canvas.Line
	face   *canvas.Circle
	center *canvas.Circle
	cover  *canvas.Rectangle

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

	// cached sin/cos for pips (angle_i = lineRotConst*i - common.Pi43)
	pipSin []float32
	pipCos []float32

	// fast float formatting buffers
	fmtPrec int
	buf1    []byte
	buf2    []byte
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
		if n := parseFixedPrec(s.displayString); n >= 0 {
			s.fmtPrec = n
		}
	}
	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		s.minsize = cfg.MinSize
	}

	s.factor = s.cfg.Max / s.steps

	s.face = &canvas.Circle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0xFF}, StrokeWidth: 3}
	s.cover = &canvas.Rectangle{FillColor: theme.Color(theme.ColorNameBackground)}
	s.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
	s.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 2}
	s.needle2 = &canvas.Line{StrokeColor: color.RGBA{R: 249, G: 27, B: 2, A: 255}, StrokeWidth: 2}

	s.titleText = &canvas.Text{Text: s.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	s.displayText2 = &canvas.Text{Text: "0", Color: color.RGBA{R: 0xff, G: 0x0, B: 0, A: 0xFF}, TextSize: 35}
	s.displayText2.TextStyle.Monospace = true
	s.displayText2.Alignment = fyne.TextAlignCenter

	// pips (colors same as before)
	fac := float64(0xA5) / s.steps
	for i := 0; i < int(s.steps+1); i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		s.pips = append(s.pips, pip)
	}

	totalRange := s.cfg.Max - s.cfg.Min
	if totalRange <= 0 {
		totalRange = 1 // guard
	}
	s.needleRotConst = common.Pi15 / totalRange
	s.lineRotConst = common.Pi15 / s.steps

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

/*
func (c *DualDial) rotateLinesNoRefresh(hand *canvas.Line, stepIndex float64, offset, length float32) {
	s, co := math.Sincos(c.lineRotConst*stepIndex - common.Pi43)
	c.applySinCos(hand, float32(s), float32(co), offset, length)
}
*/

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

func (c *DualDial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value

	// needles & text without per-object Refresh
	c.rotateNeedleNoRefresh(c.needle, value, c.needleOffset, c.needleLength)
	c.buf1 = c.buf1[:0]
	if c.fmtPrec >= 0 {
		c.buf1 = strconv.AppendFloat(c.buf1, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf1 = appendFormatFloat(c.buf1, c.displayString, value)
	}
	c.displayText.Text = string(c.buf1)

	// single refresh for changed objects
	canvas.Refresh(c.needle)
	canvas.Refresh(c.displayText)
}

func (c *DualDial) SetValue2(value float64) {
	if value == c.value2 {
		return
	}
	c.value2 = value

	c.rotateNeedleNoRefresh(c.needle2, value, c.needleOffset, c.needleLength)
	c.buf2 = c.buf2[:0]
	if c.fmtPrec >= 0 {
		c.buf2 = strconv.AppendFloat(c.buf2, value, 'f', c.fmtPrec, 64)
	} else {
		c.buf2 = appendFormatFloat(c.buf2, c.displayString, value)
	}
	c.displayText2.Text = string(c.buf2)

	canvas.Refresh(c.needle2)
	canvas.Refresh(c.displayText2)
}

func (c *DualDial) CreateRenderer() fyne.WidgetRenderer { return &DualDialRenderer{DualDial: c} }

type DualDialRenderer struct {
	*DualDial
	objects []fyne.CanvasObject
}

func (c *DualDialRenderer) Layout(space fyne.Size) {
	if c.size == space {
		return
	}
	c.size = space

	c.diameter = fyne.Min(space.Width, space.Height)
	c.radius = c.diameter * common.OneHalf
	c.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)

	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14

	stroke := c.diameter * common.OneSixthieth
	midStroke := c.diameter * common.OneEighthieth
	smallStroke := c.diameter * common.OneTwohundredth

	size := fyne.Size{Width: c.diameter, Height: c.diameter}
	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	c.titleText.TextSize = c.radius * common.OneFourth
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))

	center := c.radius * common.OneFourth
	c.center.Move(c.middle.SubtractXY(center*common.OneHalf, center*common.OneHalf))
	c.center.Resize(fyne.Size{Width: center, Height: center})

	coverY := c.middle.Y + c.radius*common.OneSeventh*5
	c.cover.Move(fyne.NewPos(0, coverY))
	c.cover.Resize(fyne.Size{Width: space.Width, Height: (space.Height - coverY) + 1})

	sixthDiameter := c.diameter * common.OneSixth

	c.displayText.TextSize = c.radius * common.OneHalf
	c.displayText.Move(topleft.AddXY(0, sixthDiameter))
	c.displayText.Resize(size)

	c.displayText2.TextSize = c.radius * common.OneFourth
	c.displayText2.Move(topleft.AddXY(0, -sixthDiameter))
	c.displayText2.Resize(size)

	// Needles & face
	c.needle.StrokeWidth = stroke
	c.needle2.StrokeWidth = stroke
	c.rotateNeedleNoRefresh(c.needle, c.value, c.needleOffset, c.needleLength)
	c.rotateNeedleNoRefresh(c.needle2, c.value2, c.needleOffset, c.needleLength)

	c.face.Move(topleft)
	c.face.Resize(size)

	// Pips using precomputed trig scaled by current radius
	fourthRadius := c.radius * common.OneFourth
	eightRadius := c.radius * common.OneEight
	radius43 := c.radius * common.OneFourth * 3
	radius87 := c.radius * common.OneEight * 7

	for i, p := range c.pips {
		if i%2 == 0 {
			p.StrokeWidth = max(2.0, midStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], radius43, fourthRadius-1)
		} else {
			p.StrokeWidth = max(2.0, smallStroke)
			c.applySinCos(p, c.pipSin[i], c.pipCos[i], radius87, eightRadius-1)
		}
	}

	// Batch refresh at end
	for _, o := range c.Objects() {
		canvas.Refresh(o)
	}
}

func (c *DualDialRenderer) MinSize() fyne.Size { return c.minsize }
func (c *DualDialRenderer) Refresh()           {}
func (c *DualDialRenderer) Destroy()           {}

func (c *DualDialRenderer) Objects() []fyne.CanvasObject {
	if c.objects == nil {
		objs := make([]fyne.CanvasObject, 0, len(c.pips)+8)
		for _, v := range c.pips {
			objs = append(objs, v)
		}
		objs = append(objs, c.face, c.cover, c.titleText, c.center, c.needle2, c.needle, c.displayText, c.displayText2)
		c.objects = objs
	}
	return c.objects
}

// --- helpers ---

func parseFixedPrec(format string) int {
	if len(format) >= 4 && format[0] == '%' && format[1] == '.' && format[len(format)-1] == 'f' {
		n := 0
		has := false
		for i := 2; i < len(format)-1; i++ {
			ch := format[i]
			if ch < '0' || ch > '9' {
				return -1
			}
			has = true
			n = n*10 + int(ch-'0')
		}
		if has {
			return n
		}
	}
	return -1
}

func appendFormatFloat(dst []byte, format string, v float64) []byte {
	if n := parseFixedPrec(format); n >= 0 {
		return strconv.AppendFloat(dst, v, 'f', n, 64)
	}
	return strconv.AppendFloat(dst, v, 'f', 0, 64)
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
