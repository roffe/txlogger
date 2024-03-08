package widgets

import (
	"fmt"
	"image/color"
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

	s.face = &canvas.Circle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, StrokeWidth: 2}
	s.cover = &canvas.Rectangle{FillColor: theme.BackgroundColor()}
	s.center = &canvas.Circle{FillColor: color.Black}
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

func (c *DualDial) rotateNeedle(hand *canvas.Line, middle fyne.Position, facePosition float64, offset, length float32) {
	if facePosition < 0 {
		facePosition = 0
	}
	rotation := math.Pi*1.5/(c.steps*c.factor)*facePosition - math.Pi/4*3
	c.rotate(hand, middle, rotation, offset, length)
	hand.Refresh()
}

func (c *DualDial) rotatePips(hand *canvas.Line, middle fyne.Position, facePosition float64, offset, length float32) {
	rotation := math.Pi*1.5/c.steps*facePosition - math.Pi/4*3
	c.rotate(hand, middle, rotation, offset, length)
}

func (c *DualDial) rotate(hand *canvas.Line, middle fyne.Position, rotation float64, offset, length float32) {
	sinRotation := float32(math.Sin(rotation))
	cosRotation := float32(math.Cos(rotation))

	x2 := length * sinRotation
	y2 := -length * cosRotation

	offX := offset * sinRotation
	offY := -offset * cosRotation

	hand.Position1 = fyne.NewPos(middle.X+offX, middle.Y+offY)
	hand.Position2 = fyne.NewPos(middle.X+offX+x2, middle.Y+offY+y2)
}

func (c *DualDial) Value() float64 {
	return c.value
}

func (c *DualDial) Value2() float64 {
	return c.value2
}

func (c *DualDial) SetValue(value float64) {
	if value == c.value {
		return
	}
	if value > c.max {
		value = c.max
	}
	if value < c.min {
		value = c.min
	}
	c.value = value

	size := c.container.Size()

	diameter := fyne.Min(size.Width, size.Height)
	middle := fyne.NewPos(size.Width/2, size.Height/2)
	radius := diameter / 2

	//c.rotateNeedle(c.needle, middle, value, radius*.15, radius*.85)
	c.rotateNeedle(c.needle, middle, value, -radius*.15, radius*1.14)

	c.displayText.Text = fmt.Sprintf(c.displayString, value)
	c.displayText.Refresh()
}

func (c *DualDial) SetValue2(value float64) {
	if value == c.value2 {
		return
	}
	if value > c.max {
		value = c.max
	}
	if value < c.min {
		value = c.min
	}
	c.value2 = value

	size := c.container.Size()

	diameter := fyne.Min(size.Width, size.Height)
	middle := fyne.NewPos(size.Width/2, size.Height/2)
	radius := diameter / 2

	//c.rotateNeedle(c.needle, middle, value, radius*.15, radius*.85)
	c.rotateNeedle(c.needle2, middle, value, -radius*.15, radius*1.14)

	c.displayText2.Text = fmt.Sprintf(c.displayString, value)
	c.displayText2.Refresh()
}

func (c *DualDial) CreateRenderer() fyne.WidgetRenderer {
	return &DualDialRenderer{
		d: c,
	}
}

type DualDialRenderer struct {
	d *DualDial
}

func (dr *DualDialRenderer) Layout(space fyne.Size) {
	dr.d.container.Resize(space)
	c := dr.d
	diameter := fyne.Min(space.Width, space.Height)
	radius := diameter / 2

	// Pre-calculate stroke sizes
	stroke := diameter / 60
	midStroke := diameter / 80
	smallStroke := diameter / 300

	// Pre-calculate sizes and positions that are used multiple times
	size := fyne.NewSize(diameter, diameter)
	halfWidth := space.Width / 2
	halfHeight := space.Height / 2
	middle := fyne.NewPos(halfWidth, halfHeight)
	topleft := fyne.NewPos(middle.X-radius, middle.Y-radius)

	// Text and element sizing
	c.titleText.TextSize = radius / 3
	c.titleText.Move(middle.Add(fyne.NewPos(0, diameter/4)))

	// Calculate the size of the center component directly
	centerWidth := radius / 4
	centerHeight := radius / 4
	c.center.Move(middle.SubtractXY(centerWidth/2, centerHeight/2))
	c.center.Resize(fyne.NewSize(centerWidth, centerHeight))

	coverHeight := size.Height / 6
	c.cover.Move(fyne.NewPos(0, middle.Y+radius/7*5))
	c.cover.Resize(fyne.NewSize(space.Width, coverHeight))

	c.displayText.TextSize = radius / 2
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Move(topleft.AddXY(0, diameter/6))
	c.displayText.Resize(size)

	c.displayText2.TextSize = radius / 4
	c.displayText2.Text = fmt.Sprintf(c.displayString, c.value2)
	c.displayText2.Move(topleft.AddXY(0, -diameter/6))
	c.displayText2.Resize(size)

	// Needle and face styling
	c.needle.StrokeWidth = stroke
	c.rotateNeedle(c.needle, middle, c.value, -radius*.15, radius*1.14)
	c.rotateNeedle(c.needle2, middle, c.value2, -radius*.15, radius*1.14)

	c.face.StrokeWidth = smallStroke
	c.face.Move(topleft)
	c.face.Resize(size)

	// Optimize pip rotation and styling
	for i, p := range c.pips {
		outerRadius, innerRadius := radius/4*3, radius/4 // default for even i
		p.StrokeWidth = midStroke
		if i%2 != 0 {
			outerRadius, innerRadius = radius/8*7, radius/8
			p.StrokeWidth = smallStroke
		}
		c.rotatePips(p, middle, float64(i), outerRadius, innerRadius)
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
