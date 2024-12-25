package dualdial

import (
	"bytes"
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

type DualDial struct {
	widget.BaseWidget

	cfg widgets.GaugeConfig

	titleText     *canvas.Text
	displayString string

	value  float64
	value2 float64

	needle  *canvas.Line
	needle2 *canvas.Line

	pips []*canvas.Line
	//numbers     []*canvas.Text
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
	buf1, buf2                 bytes.Buffer
}

func New(cfg widgets.GaugeConfig) *DualDial {
	s := &DualDial{
		cfg:           cfg,
		steps:         10,
		displayString: "%.0f",
		minsize:       fyne.NewSize(100, 100),
	}
	s.ExtendBaseWidget(s)
	if cfg.Steps > 0 {
		s.steps = float64(cfg.Steps)
	}

	if cfg.DisplayString != "" {
		s.displayString = cfg.DisplayString
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

	fac := float64(0xA5) / s.steps
	for i := 0; i < int(s.steps+1); i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		s.pips = append(s.pips, pip)
	}

	totalRange := s.cfg.Max - s.cfg.Min
	s.needleRotConst = common.Pi15 / (s.steps * (totalRange / s.steps))
	s.lineRotConst = common.Pi15 / s.steps

	return s
}

func (c *DualDial) GetConfig() widgets.GaugeConfig {
	return c.cfg
}

func (c *DualDial) rotateNeedle(hand *canvas.Line, facePosition float64) {
	// Normalize the value to start from 0 regardless of minimum value
	normalizedPosition := facePosition - c.cfg.Min
	if normalizedPosition < 0 {
		normalizedPosition = 0
	}
	c.rotate(hand, c.needleRotConst*normalizedPosition-common.Pi43, c.needleOffset, c.needleLength)
}

func (c *DualDial) rotateLines(hand *canvas.Line, facePosition float64, offset, length float32) {
	c.rotate(hand, c.lineRotConst*facePosition-common.Pi43, offset, length)
}

func (c *DualDial) rotate(hand *canvas.Line, rotation float64, offset, length float32) {
	sinRotation := float32(math.Sin(rotation))
	cosRotation := float32(math.Cos(rotation))
	x2 := length * sinRotation
	y2 := -length * cosRotation
	offX := offset * sinRotation
	offY := -offset * cosRotation
	midOffX := c.middle.X + offX
	midY := c.middle.Y + offY
	hand.Position1 = fyne.NewPos(midOffX, midY)
	hand.Position2 = fyne.NewPos(midOffX+x2, midY+y2)
	hand.Refresh()
}

func (c *DualDial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value
	c.rotateNeedle(c.needle, value)
	c.buf1.Reset()
	fmt.Fprintf(&c.buf1, c.displayString, value)
	c.displayText.Text = c.buf1.String()
	c.displayText.Refresh()
}

func (c *DualDial) SetValue2(value float64) {
	if value == c.value2 {
		return
	}
	c.value2 = value
	c.rotateNeedle(c.needle2, value)
	c.buf2.Reset()
	fmt.Fprintf(&c.buf2, c.displayString, value)
	c.displayText2.Text = c.buf2.String()
	c.displayText2.Refresh()
}

func (c *DualDial) CreateRenderer() fyne.WidgetRenderer {
	return &DualDialRenderer{c}
}

type DualDialRenderer struct {
	*DualDial
}

func (c *DualDialRenderer) Layout(space fyne.Size) {
	if c.size == space {
		return
	}
	c.size = space
	//	log.Println("dual_dial.Layout", dr.d.title, space.Width, space.Height)

	c.diameter = fyne.Min(space.Width, space.Height)
	c.radius = c.diameter * common.OneHalf
	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14

	// Pre-calculate stroke sizes
	stroke := c.diameter * common.OneSixthieth
	midStroke := c.diameter * common.OneEighthieth
	smallStroke := c.diameter * common.OneTwohundredth

	c.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)

	// Pre-calculate sizes and positions that are used multiple times
	size := fyne.NewSize(c.diameter, c.diameter)

	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	// Text and element sizing
	c.titleText.TextSize = c.radius * common.OneFourth
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter*common.OneFourth)))
	c.titleText.Refresh()

	// Calculate the size of the center component directly
	center := c.radius * common.OneFourth

	c.center.Move(c.middle.SubtractXY(center*common.OneHalf, center*common.OneHalf))
	c.center.Resize(fyne.NewSize(center, center))

	coverY := c.middle.Y + c.radius*common.OneSeventh*5
	c.cover.Move(fyne.NewPos(0, coverY))
	c.cover.Resize(fyne.NewSize(space.Width, (space.Height-coverY)+1))

	sixthDiameter := c.diameter * common.OneSixth

	c.displayText.TextSize = c.radius * common.OneHalf
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Move(topleft.AddXY(0, sixthDiameter))
	c.displayText.Resize(size)

	c.displayText2.TextSize = c.radius * common.OneFourth
	c.displayText2.Text = fmt.Sprintf(c.displayString, c.value2)
	c.displayText2.Move(topleft.AddXY(0, -sixthDiameter))
	c.displayText2.Resize(size)

	// Needle and face styling
	c.needle.StrokeWidth = stroke
	c.needle2.StrokeWidth = stroke
	c.rotateNeedle(c.needle, c.value)
	c.rotateNeedle(c.needle2, c.value2)

	//c.face.StrokeWidth = smallStroke
	c.face.Move(topleft)
	c.face.Resize(size)

	fourthRadius := c.radius * common.OneFourth
	eightRadius := c.radius * common.OneEight

	// Optimize pip rotation and styling
	radius43 := c.radius * common.OneFourth * 3
	radius87 := c.radius * common.OneEight * 7

	for i, p := range c.pips {
		if i%2 == 0 {
			p.StrokeWidth = max(2.0, midStroke)
			c.rotateLines(p, float64(i), radius43, fourthRadius-1)
		} else {
			p.StrokeWidth = max(2.0, smallStroke)
			c.rotateLines(p, float64(i), radius87, eightRadius-1)
		}
	}
}

func (c *DualDialRenderer) MinSize() fyne.Size {
	return c.minsize
}

func (c *DualDialRenderer) Refresh() {
}

func (c *DualDialRenderer) Destroy() {
}

func (c *DualDial) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{}
	for _, p := range c.pips {
		objs = append(objs, p)
	}
	objs = append(objs, c.face, c.cover, c.titleText, c.center, c.needle2, c.needle, c.displayText, c.displayText2)
	return objs
}
