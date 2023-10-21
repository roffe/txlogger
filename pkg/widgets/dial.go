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
}

func NewDial(cfg DialConfig) *Dial {
	c := &Dial{
		title:         cfg.Title,
		min:           cfg.Min,
		max:           cfg.Max,
		steps:         30,
		displayString: "%.0f",
		//minsize:       fyne.NewSize(150, 150),
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
	c.face = &canvas.Circle{StrokeColor: color.RGBA{0x80, 0x80, 0x80, 0x80}, StrokeWidth: 2}
	c.cover = &canvas.Rectangle{FillColor: theme.BackgroundColor()}
	c.center = &canvas.Circle{FillColor: color.Black}
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

	//listener := make(chan fyne.Settings)
	//fyne.CurrentApp().Settings().AddChangeListener(listener)
	//go func() {
	//	for {
	//		settings := <-listener
	//		s.applyTheme(settings)
	//	}
	//}()
	return c
}

func (c *Dial) rotateNeedle(hand *canvas.Line, middle fyne.Position, facePosition float64, offset, length float32) {
	if facePosition < 0 {
		facePosition = 0
	}
	rotation := math.Pi*1.5/(c.steps*c.factor)*facePosition - math.Pi/4*3
	c.rotate(hand, middle, facePosition, rotation, offset, length)
	hand.Refresh()
}

func (c *Dial) rotatePips(hand *canvas.Line, middle fyne.Position, facePosition float64, offset, length float32) {
	rotation := math.Pi*1.5/c.steps*facePosition - math.Pi/4*3
	c.rotate(hand, middle, facePosition, rotation, offset, length)
}

func (c *Dial) rotate(hand *canvas.Line, middle fyne.Position, facePosition, rotation float64, offset, length float32) {
	x2 := length * float32(math.Sin(rotation))
	y2 := -length * float32(math.Cos(rotation))

	offX := float32(0)
	offY := float32(0)
	offX += offset * float32(math.Sin(rotation))
	offY += -offset * float32(math.Cos(rotation))

	hand.Position1 = fyne.NewPos(middle.X+offX, middle.Y+offY)
	hand.Position2 = fyne.NewPos(middle.X+offX+x2, middle.Y+offY+y2)
}

func (c *Dial) GetValue() float64 {
	return c.value
}

func (c *Dial) SetValue(value float64) {
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

	c.rotateNeedle(c.needle, middle, value, -radius*.15, radius*1.13)

	c.displayText.Text = fmt.Sprintf(c.displayString, value)
	c.displayText.Refresh()
}

/*
func (c *Dial) AnimateValue(value float64, dur time.Duration) {
	if value > c.max {
		value = c.max
	}

	if value < c.min {
		value = c.min
	}
	if c.animating {
		return
	}
	c.value = value

	//sz := c.canvas.Size()
	//diameter := fyne.Min(sz.Width, sz.Height)
	//middle := fyne.NewPos(sz.Width/2, sz.Height/2)
	//radius := diameter / 2

	start := value
	fyne.NewAnimation(dur, func(v float32) {
		val := start + (value-start)*float64(v)
		c.setPosition(val, c.canvas.Size())
		//log.Println(val)
		//c.displayText.Text = fmt.Sprintf(c.displayString, val)
		//c.rotateNeedle(c.needle, middle, val, radius*.15, radius*.8)
		c.needle.Refresh()
		c.displayText.Refresh()
		if v == 1 {
			c.animating = false
		}
	}).Start()
}
*/

/*
func (c *Dial) applyTheme(_ fyne.Settings) {
	c.face.StrokeColor = theme.DisabledColor()
	c.needle.StrokeColor = theme.ErrorColor()
	c.speed.Color = theme.ForegroundColor()
	c.cover.FillColor = theme.BackgroundColor()

	for i, pip := range c.pips {
		if i == 0 {
			pip.StrokeColor = theme.ForegroundColor()
		} else if i >= 100 && i < 110 {
			pip.StrokeColor = theme.WarningColor()
		} else if i >= 110 {
			pip.StrokeColor = theme.ErrorColor()
		} else {
			pip.StrokeColor = theme.DisabledColor()
		}
	}
}
*/

func (c *Dial) CreateRenderer() fyne.WidgetRenderer {
	return &DialRenderer{
		d: c,
	}
}

type DialRenderer struct {
	d *Dial
}

func (dr *DialRenderer) Layout(space fyne.Size) {
	dr.d.container.Resize(space)
	c := dr.d
	diameter := fyne.Min(space.Width, space.Height)

	radius := diameter / 2
	stroke := diameter / 60
	midStroke := diameter / 80
	smallStroke := diameter / 300

	size := fyne.NewSize(diameter, diameter)
	middle := fyne.NewPos(space.Width/2, space.Height/2)
	topleft := fyne.NewPos(middle.X-radius, middle.Y-radius)

	c.titleText.TextSize = radius / 3
	c.titleText.Move(middle.Add(fyne.NewPos(0, diameter/4)))
	c.titleText.Refresh()

	c.center.Resize(fyne.NewSize(radius/4, radius/4))
	c.center.Move(middle.SubtractXY(c.center.Size().Width/2, c.center.Size().Height/2))

	c.cover.Move(fyne.NewPos(0, middle.Y+radius/7*5))
	c.cover.Resize(fyne.NewSize(c.container.Size().Width, size.Height/6))

	c.displayText.TextSize = radius / 2
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Resize(size)
	c.displayText.Move(topleft.AddXY(0, diameter/6))

	c.needle.StrokeWidth = stroke

	c.rotateNeedle(c.needle, middle, c.value, -radius*.15, radius*1.13)

	c.face.StrokeWidth = smallStroke
	c.face.Move(topleft)
	c.face.Resize(size)

	for i, p := range c.pips {
		if i%2 == 0 {
			c.rotatePips(p, middle, float64(i), radius/4*3, radius/4)
			p.StrokeWidth = midStroke
		} else {
			c.rotatePips(p, middle, float64(i), radius/8*7, radius/8)
			p.StrokeWidth = smallStroke
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
