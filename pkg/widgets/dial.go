package widgets

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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

	canvas fyne.CanvasObject

	minsize fyne.Size

	//rect *canvas.Rectangle
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

	c.canvas = c.render()
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

func (c *Dial) render() *fyne.Container {
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

	//c.rect = &canvas.Rectangle{StrokeColor: color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, StrokeWidth: 3}

	dial := container.NewWithoutLayout(c.face, c.cover, c.titleText)
	fac := float64(0xA5) / c.steps
	for i := 0; i < int(c.steps+1); i++ {
		col := color.RGBA{byte(float64(i) * fac), 0x00, 0x00, 0xFF}
		col.G = 0xA5 - col.R
		//pip := &canvas.Line{StrokeColor: color.RGBA{0x00, 0xE5, 0x00, 0xFF}, StrokeWidth: 2}
		pip := &canvas.Line{StrokeColor: col, StrokeWidth: 2}
		/*
			if i == 0 {
				pip.StrokeColor = color.RGBA{0x00, 0xE5, 0x00, 0xFF}
				// } else if float64(i)*c.factor >= c.max-(c.max/4*2) && float64(i)*c.factor < c.max-(c.max/5) {
			} else if float64(i)*c.factor >= c.max*.6 && float64(i)*c.factor < c.max*.8 {
				pip.StrokeColor = theme.WarningColor()
			} else if float64(i)*c.factor >= c.max*.8 {
				pip.StrokeColor = theme.ErrorColor()
			}
		*/

		//num := canvas.NewText(fmt.Sprintf("%0.f", float64(i)*c.factor), color.RGBA{0x00, 0xE5, 0x00, 0xFF})
		//num.TextSize = 30
		//c.numbers = append(c.numbers, num)
		//dial.Add(num)

		dial.Add(pip)
		c.pips = append(c.pips, pip)
	}

	dial.Objects = append(dial.Objects, c.center, c.needle, c.displayText)
	dial.Layout = c

	return dial
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
	//if offset > 0 {
	offX += offset * float32(math.Sin(rotation))
	offY += -offset * float32(math.Cos(rotation))
	//}

	hand.Position1 = fyne.NewPos(middle.X+offX, middle.Y+offY)
	hand.Position2 = fyne.NewPos(middle.X+offX+x2, middle.Y+offY+y2)
}

func (c *Dial) Layout(_ []fyne.CanvasObject, space fyne.Size) {
	diameter := fyne.Min(space.Width, space.Height)
	//diameter := space.Height
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

	//c.rect.Resize(size)
	//c.rect.Move(fyne.NewPos(0, 0))

	c.face.Resize(size)
	c.face.Move(topleft)

	c.center.Resize(fyne.NewSize(radius/4, radius/4))
	c.center.Move(middle.SubtractXY(c.center.Size().Width/2, c.center.Size().Height/2))

	c.cover.Resize(fyne.NewSize(c.canvas.Size().Width, size.Height/6))
	c.cover.Move(fyne.NewPos(0, middle.Y+radius/7*5))

	c.displayText.TextSize = radius / 2
	c.displayText.Text = fmt.Sprintf(c.displayString, c.value)
	c.displayText.Resize(size)
	c.displayText.Move(topleft.AddXY(0, diameter/6))

	c.needle.StrokeWidth = stroke

	c.rotateNeedle(c.needle, middle, c.value, -radius*.15, radius*1.13)

	c.face.StrokeWidth = smallStroke
	c.face.Refresh()

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

func (c *Dial) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return c.minsize
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

	size := c.canvas.Size()

	diameter := fyne.Min(size.Width, size.Height)
	middle := fyne.NewPos(size.Width/2, size.Height/2)
	radius := diameter / 2

	//c.rotateNeedle(c.needle, middle, value, radius*.15, radius*.85)
	c.rotateNeedle(c.needle, middle, value, -radius*.15, radius*1.13)

	c.displayText.Text = fmt.Sprintf(c.displayString, value)
	c.displayText.Refresh()
}

func (c *Dial) Content() fyne.CanvasObject {
	return c.canvas
}

func (c *Dial) Value() float64 {
	return c.value
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
