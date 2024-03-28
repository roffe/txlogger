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

type DualDialConfig struct {
	Title string
	Min   float64
	Max   float64
	Steps int
	//Factor        float64
	DisplayString string    // default "%.0f"
	MinSize       fyne.Size // default 260x260
}

type DualDial struct {
	widget.BaseWidget

	title         string
	titleText     *canvas.Text
	displayString string

	min, max float64

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

	container *fyne.Container

	minsize fyne.Size

	diameter                   float32
	radius                     float32
	middle                     fyne.Position
	needleOffset, needleLength float32
	needleRotConst             float64
	lineRotConst               float64
}

func NewDualDial(cfg DualDialConfig) *DualDial {
	s := &DualDial{
		title:         cfg.Title,
		min:           cfg.Min,
		max:           cfg.Max,
		steps:         30,
		displayString: "%.0f",
		minsize:       fyne.NewSize(260, 260),
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

	s.factor = s.max / s.steps
	s.needleRotConst = pi15 / (s.steps * s.factor)
	s.lineRotConst = pi15 / s.steps

	s.face = &canvas.Circle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, StrokeWidth: 2}
	s.cover = &canvas.Rectangle{FillColor: theme.BackgroundColor()}
	s.center = &canvas.Circle{FillColor: color.RGBA{R: 0x01, G: 0x0B, B: 0x13, A: 0xFF}}
	s.needle = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF}, StrokeWidth: 4}
	s.needle2 = &canvas.Line{StrokeColor: color.RGBA{R: 0xFF, G: 0x40, B: 0, A: 0xa0}, StrokeWidth: 3}

	s.titleText = &canvas.Text{Text: s.title, Color: color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}, TextSize: 25}
	s.titleText.TextStyle.Monospace = true
	s.titleText.Alignment = fyne.TextAlignCenter

	s.displayText = &canvas.Text{Text: "0", Color: color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}, TextSize: 52}
	s.displayText.TextStyle.Monospace = true
	s.displayText.Alignment = fyne.TextAlignCenter

	s.displayText2 = &canvas.Text{Text: "0", Color: color.RGBA{R: 0xff, G: 0x0, B: 0, A: 0xFF}, TextSize: 35}
	s.displayText2.TextStyle.Monospace = true
	s.displayText2.Alignment = fyne.TextAlignCenter

	dial := container.NewWithoutLayout(s.face, s.cover, s.titleText)
	fac := float64(0xA5) / s.steps
	for i := 0; i < int(s.steps+1); i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		dial.Add(pip)
		s.pips = append(s.pips, pip)
	}

	dial.Objects = append(dial.Objects, s.center, s.needle2, s.needle, s.displayText, s.displayText2)
	s.container = dial
	return s
}

func (c *DualDial) rotateNeedle(hand *canvas.Line, facePosition float64) {
	if facePosition < 0 {
		facePosition = 0
	}
	c.rotate(hand, c.needleRotConst*facePosition-pi43, c.needleOffset, c.needleLength)
}

func (c *DualDial) rotateLines(hand *canvas.Line, facePosition float64, offset, length float32) {
	c.rotate(hand, c.lineRotConst*facePosition-pi43, offset, length)
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
	c.value = value
	c.rotateNeedle(c.needle, value)
	c.displayText.Text = fmt.Sprintf(c.displayString, value)
	c.displayText.Refresh()
}

func (c *DualDial) SetValue2(value float64) {
	c.value2 = value
	c.rotateNeedle(c.needle2, value)
	c.displayText2.Text = fmt.Sprintf(c.displayString, value)
	c.displayText2.Refresh()
}

func (c *DualDial) CreateRenderer() fyne.WidgetRenderer {
	return &DualDialRenderer{
		d: c,
	}
}

type DualDialRenderer struct {
	d    *DualDial
	size fyne.Size
}

func (dr *DualDialRenderer) Layout(space fyne.Size) {
	if dr.size.Width == space.Width && dr.size.Height == space.Height {
		return
	}
	dr.size = space
	log.Println("dual_dial.Layout", dr.d.title, space.Width, space.Height)
	c := dr.d
	c.container.Resize(space)
	c.diameter = fyne.Min(space.Width, space.Height)
	c.radius = c.diameter / 2
	c.needleOffset = -c.radius * .15
	c.needleLength = c.radius * 1.14

	// Pre-calculate stroke sizes
	stroke := c.diameter / 60
	midStroke := c.diameter / 80
	smallStroke := c.diameter / 300

	c.middle = fyne.NewPos(space.Width/2, space.Height/2)

	// Pre-calculate sizes and positions that are used multiple times
	size := fyne.NewSize(c.diameter, c.diameter)

	topleft := fyne.NewPos(c.middle.X-c.radius, c.middle.Y-c.radius)

	// Text and element sizing
	c.titleText.TextSize = c.radius / 3
	c.titleText.Move(c.middle.Add(fyne.NewPos(0, c.diameter/4)))

	// Calculate the size of the center component directly
	center := c.radius / 4

	c.center.Move(c.middle.SubtractXY(center/2, center/2))
	c.center.Resize(fyne.NewSize(center, center))

	coverHeight := size.Height / 6
	c.cover.Move(fyne.NewPos(0, c.middle.Y+c.radius/7*5))
	c.cover.Resize(fyne.NewSize(space.Width, coverHeight))

	sixthDiameter := c.diameter / 6

	c.displayText.TextSize = c.radius / 2
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Move(topleft.AddXY(0, sixthDiameter))
	c.displayText.Resize(size)

	c.displayText2.TextSize = c.radius / 4
	c.displayText2.Text = fmt.Sprintf(c.displayString, c.value2)
	c.displayText2.Move(topleft.AddXY(0, -sixthDiameter))
	c.displayText2.Resize(size)

	// Needle and face styling
	c.needle.StrokeWidth = stroke
	c.rotateNeedle(c.needle, c.value)
	c.rotateNeedle(c.needle2, c.value2)

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

func (dr *DualDialRenderer) MinSize() fyne.Size {
	return dr.d.minsize
}

func (dr *DualDialRenderer) Refresh() {
}

func (dr *DualDialRenderer) Destroy() {
}

func (dr *DualDialRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{dr.d.container}
}
