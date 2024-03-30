package widgets

import (
	"fmt"
	"image/color"
	"log"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type DialConfig struct {
	Title         string
	Min           float64
	Max           float64
	Steps         int
	DisplayString string    // default "%.0f"
	MinSize       fyne.Size // default 260x260
}

type Dial struct {
	widget.BaseWidget
	title         string
	displayString string

	min, max float64
	steps    float64
	factor   float64
	value    float64

	needle      *canvas.Line
	pips        []*canvas.Line
	face        *canvas.Circle
	center      *canvas.Circle
	cover       *canvas.Rectangle
	displayText *canvas.Text
	titleText   *canvas.Text

	minsize fyne.Size

	container *fyne.Container

	diameter                   float32
	radius                     float32
	middle                     fyne.Position
	needleOffset, needleLength float32
	needleRotConst             float64
	lineRotConst               float64
}

func NewDial(cfg DialConfig) *Dial {
	c := &Dial{
		title:         cfg.Title,
		min:           cfg.Min,
		max:           cfg.Max,
		steps:         30,
		displayString: "%.0f",
		minsize:       fyne.NewSize(150, 150),
	}
	c.ExtendBaseWidget(c)
	if cfg.Steps > 0 {
		c.steps = float64(cfg.Steps)
	}

	if cfg.DisplayString != "" {
		c.displayString = cfg.DisplayString
	}

	if cfg.MinSize.Width > 0 && cfg.MinSize.Height > 0 {
		c.minsize = cfg.MinSize
	}

	c.factor = c.max / c.steps

	c.needleRotConst = pi15 / (c.steps * c.factor)
	c.lineRotConst = pi15 / c.steps

	c.face = &canvas.Circle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, StrokeWidth: 2}
	c.cover = &canvas.Rectangle{FillColor: theme.BackgroundColor()}
	c.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
	c.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 3}

	c.titleText = &canvas.Text{Text: c.title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	c.titleText.TextStyle.Monospace = true
	c.titleText.Alignment = fyne.TextAlignCenter

	c.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	c.displayText.TextStyle.Monospace = true
	c.displayText.Alignment = fyne.TextAlignCenter

	c.container = container.NewWithoutLayout(c.face, c.cover, c.titleText)
	fac := float64(0xA5) / c.steps
	for i := 0; i < int(c.steps+1); i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		c.pips = append(c.pips, pip)
		c.container.Add(pip)
	}
	c.container.Objects = append(c.container.Objects, c.center, c.needle, c.displayText)

	return c
}

func (c *Dial) rotateNeedle(hand *canvas.Line, facePosition float64, offset, length float32) {
	if facePosition < 0 {
		facePosition = 0
	}
	c.rotate(hand, c.needleRotConst*facePosition-pi43, offset, length)
}

func (c *Dial) rotateLines(hand *canvas.Line, facePosition float64, offset, length float32) {
	c.rotate(hand, c.lineRotConst*facePosition-pi43, offset, length)
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
	c.value = value
	c.rotateNeedle(c.needle, value, c.needleOffset, c.needleLength)
	c.displayText.Text = fmt.Sprintf(c.displayString, value)
	c.displayText.Refresh()
}

func (c *Dial) CreateRenderer() fyne.WidgetRenderer {
	return &DialRenderer{
		d: c,
	}
}

type DialRenderer struct {
	d    *Dial
	size fyne.Size
}

func (dr *DialRenderer) Layout(space fyne.Size) {
	if dr.size.Width == space.Width && dr.size.Height == space.Height {
		return
	}
	dr.size = space
	log.Println("dial.Layout", dr.d.title, space.Width, space.Height)
	dr.d.container.Resize(space)
	c := dr.d
	c.diameter = fyne.Min(space.Width, space.Height)
	c.radius = c.diameter * .5
	c.middle = fyne.NewPos(space.Width*.5, space.Height*.5)
	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14
	stroke := c.diameter / 60
	midStroke := c.diameter / 80
	smallStroke := c.diameter / 300

	size := fyne.NewSize(c.diameter, c.diameter)

	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	c.titleText.TextSize = c.radius / 3.5
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter/4)))
	c.titleText.Refresh()

	c.center.Move(c.middle.SubtractXY(c.center.Size().Width*.5, c.center.Size().Height*.5))
	c.center.Resize(fyne.NewSize(c.radius/4, c.radius/4))

	c.cover.Move(fyne.NewPos(0, c.middle.Y+c.radius/7*5))
	c.cover.Resize(fyne.NewSize(c.container.Size().Width, size.Height/6))

	c.displayText.TextSize = c.radius * .5
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Move(topleft.AddXY(0, c.diameter/6))
	c.displayText.Resize(size)

	c.needle.StrokeWidth = stroke
	c.rotateNeedle(c.needle, c.value, c.needleOffset, c.needleLength)

	c.face.StrokeWidth = smallStroke
	c.face.Move(topleft)
	c.face.Resize(size)

	fourthRadius := c.radius / 4
	eightRadius := c.radius / 8

	// Optimize pip rotation and styling
	radius43 := c.radius / 4 * 3
	radius87 := c.radius / 8 * 7

	for i, p := range c.pips {
		if i%2 == 0 {
			p.StrokeWidth = midStroke
			c.rotateLines(p, float64(i), radius43, fourthRadius)
		} else {
			p.StrokeWidth = smallStroke
			c.rotateLines(p, float64(i), radius87, eightRadius)
		}
	}
}

func (dr *DialRenderer) MinSize() fyne.Size {
	return dr.d.minsize
}

func (dr *DialRenderer) Refresh() {
}

func (dr *DialRenderer) Destroy() {
}

func (dr *DialRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{dr.d.container}
}
