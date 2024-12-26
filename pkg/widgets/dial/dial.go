package dial

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

type Dial struct {
	widget.BaseWidget
	displayString string

	cfg widgets.GaugeConfig

	factor float64
	value  float64
	//highestObserved float64

	needle *canvas.Line
	//highestObservedMarker *canvas.Line
	//lastHighestObserved   time.Time

	pips        []*canvas.Line
	face        *canvas.Circle
	center      *canvas.Circle
	cover       *canvas.Rectangle
	displayText *canvas.Text
	titleText   *canvas.Text

	size    fyne.Size
	minsize fyne.Size

	diameter                   float32
	radius                     float32
	middle                     fyne.Position
	needleOffset, needleLength float32
	needleRotConst             float64
	lineRotConst               float64

	buf bytes.Buffer
}

func New(cfg widgets.GaugeConfig) *Dial {
	c := &Dial{
		cfg:           cfg,
		displayString: "%.0f",
		minsize:       fyne.NewSize(100, 100),
	}
	c.ExtendBaseWidget(c)

	if cfg.DisplayString != "" {
		c.displayString = cfg.DisplayString
	}

	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		c.minsize = cfg.MinSize
	}

	steps := float64(cfg.Steps)

	c.factor = c.cfg.Max / steps

	c.face = &canvas.Circle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 255}, StrokeWidth: 3}
	c.cover = &canvas.Rectangle{FillColor: theme.Color(theme.ColorNameBackground)}
	c.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
	c.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 3}
	//c.highestObservedMarker = &canvas.Line{StrokeColor: color.RGBA{R: 216, G: 250, B: 8, A: 0xFF}, StrokeWidth: 4}

	c.titleText = &canvas.Text{Text: c.cfg.Title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	c.titleText.TextStyle.Monospace = true
	c.titleText.Alignment = fyne.TextAlignCenter

	c.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	c.displayText.TextStyle.Monospace = true
	c.displayText.Alignment = fyne.TextAlignCenter

	fac := float64(0xA5) / float64(c.cfg.Steps)
	for i := 0; i < c.cfg.Steps+1; i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		c.pips = append(c.pips, pip)

	}

	totalRange := c.cfg.Max - c.cfg.Min
	c.needleRotConst = common.Pi15 / (steps * (totalRange / steps))
	c.lineRotConst = common.Pi15 / steps

	return c
}

func (c *Dial) GetConfig() widgets.GaugeConfig {
	return c.cfg
}

func (c *Dial) rotateNeedle(hand *canvas.Line, facePosition float64, offset, length float32) {
	// Normalize the value to start from 0 regardless of minimum value
	normalizedPosition := facePosition - c.cfg.Min
	if normalizedPosition < 0 {
		normalizedPosition = 0
	}
	c.rotate(hand, c.needleRotConst*normalizedPosition-common.Pi43, offset, length)
}

func (c *Dial) rotateLines(hand *canvas.Line, facePosition float64, offset, length float32) {
	c.rotate(hand, c.lineRotConst*facePosition-common.Pi43, offset, length)
}

func (c *Dial) rotate(hand *canvas.Line, rotation float64, offset, length float32) {
	sinRotation := float32(math.Sin(rotation))
	cosRotation := float32(math.Cos(rotation))
	x2 := length * sinRotation
	y2 := -length * cosRotation
	offX := offset * sinRotation
	offY := -offset * cosRotation
	midxOffX := c.middle.X + offX
	midY := c.middle.Y + offY

	hand.Position1 = fyne.NewPos(midxOffX, midY)
	hand.Position2 = fyne.NewPos(midxOffX+x2, midY+y2)
	hand.Refresh()
}

func (c *Dial) SetValue(value float64) {
	if value == c.value {
		return
	}
	c.value = value
	c.rotateNeedle(c.needle, value, c.needleOffset, c.needleLength)
	c.buf.Reset()
	fmt.Fprintf(&c.buf, c.displayString, value)
	c.displayText.Text = c.buf.String()
	c.displayText.Refresh()
	/*
		if value > c.highestObserved || time.Since(c.lastHighestObserved) > 10*time.Second {
			c.highestObserved = value
			c.rotateNeedle(c.highestObservedMarker, c.highestObserved, c.radius, c.eightRadius*0.5)
			c.lastHighestObserved = time.Now()
		}
	*/
}

func (c *Dial) SetValue2(value float64) {
	c.SetValue(value)
}

func (c *Dial) CreateRenderer() fyne.WidgetRenderer {
	return &DialRenderer{Dial: c}
}

type DialRenderer struct {
	*Dial
}

func (c *DialRenderer) Layout(space fyne.Size) {
	if c.size == space {
		return
	}
	c.size = space
	//log.Println("dial.Layout", dr.d.title, space.Width, space.Height)
	//dr.d.container.Resize(space)

	c.diameter = fyne.Min(space.Width, space.Height)
	c.radius = c.diameter * common.OneHalf
	c.middle = fyne.NewPos(space.Width*common.OneHalf, space.Height*common.OneHalf)
	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14

	// Pre-calculate stroke sizes
	stroke := c.diameter * common.OneSixthieth
	midStroke := c.diameter * common.OneEighthieth
	smallStroke := c.diameter * common.OneTwohundredth

	size := fyne.NewSize(c.diameter, c.diameter)

	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

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

	c.displayText.TextSize = c.radius * common.OneHalf
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Move(topleft.AddXY(0, c.diameter*common.OneSixth))
	c.displayText.Resize(size)

	c.needle.StrokeWidth = stroke
	c.rotateNeedle(c.needle, c.value, c.needleOffset, c.needleLength)

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
	//c.rotateNeedle(c.highestObservedMarker, c.highestObserved, c.radius, c.eightRadius*0.5)
}

func (c *DialRenderer) MinSize() fyne.Size {
	return c.minsize
}

func (c *DialRenderer) Refresh() {
}

func (c *DialRenderer) Destroy() {
}

func (c *DialRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{}
	for _, v := range c.pips {
		objs = append(objs, v)
	}
	objs = append(objs, c.face, c.cover, c.titleText, c.center /*, c.highestObservedMarker*/, c.needle, c.displayText)
	return objs
}
