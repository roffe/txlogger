package widgets

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

type DialLayout struct {
	needle *canvas.Line
	pips   [121]*canvas.Line
	face   *canvas.Circle
	cover  *canvas.Rectangle
	speed  *canvas.Text

	content *fyne.Container

	canvas fyne.CanvasObject
	stop   bool
	value  float64
}

/*
func (c *DialLayout) Destroy() {
}

func (c *DialLayout) Hide() {
	c.content.Hide()
}

func (c *DialLayout) MinSize() fyne.Size {
	return c.content.MinSize()
}

func (c *DialLayout) Move(pos fyne.Position) {
	c.content.Move(pos)
}

func (c *DialLayout) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{c.content}
}

func (c *DialLayout) Refresh() {
	c.content.Refresh()
}
*/

func (c *DialLayout) rotate(hand *canvas.Line, middle fyne.Position, facePosition float64, offset, length float32) {
	rotation := math.Pi*1.5/120*facePosition - math.Pi/4*3
	x2 := length * float32(math.Sin(rotation))
	y2 := -length * float32(math.Cos(rotation))

	offX := float32(0)
	offY := float32(0)
	if offset > 0 {
		offX += offset * float32(math.Sin(rotation))
		offY += -offset * float32(math.Cos(rotation))
	}

	hand.Position1 = fyne.NewPos(middle.X+offX, middle.Y+offY)
	hand.Position2 = fyne.NewPos(middle.X+offX+x2, middle.Y+offY+y2)
}

func (c *DialLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	c.setPosition(c.value, size)
}

func (c *DialLayout) setPosition(v float64, space fyne.Size) {
	c.value = v
	diameter := fyne.Min(space.Width, space.Height)
	radius := diameter / 2
	stroke := diameter / 50
	midStroke := diameter / 80
	smallStroke := diameter / 200

	size := fyne.NewSize(diameter, diameter)
	middle := fyne.NewPos(space.Width/2, space.Height/2)
	topleft := fyne.NewPos(middle.X-radius, middle.Y-radius)

	c.face.Move(topleft)
	c.face.Resize(size)
	c.cover.Move(fyne.NewPos(0, middle.Y+radius/7*5))
	c.cover.Resize(fyne.NewSize(size.Width, size.Height/6))
	c.speed.Move(topleft)
	c.speed.Resize(size)
	c.speed.TextSize = size.Height / 3
	c.speed.Text = fmt.Sprintf("%d", int(v))

	c.needle.StrokeWidth = stroke
	c.rotate(c.needle, middle, v, radius*.2, radius*.75)
	c.face.StrokeWidth = smallStroke

	for i, p := range c.pips {
		if i%10 == 0 {
			c.rotate(p, middle, float64(i), radius/4*3, radius/4)
			p.StrokeWidth = midStroke
		} else {
			c.rotate(p, middle, float64(i), radius/8*7, radius/8)
			p.StrokeWidth = smallStroke
		}
	}
}

func (c *DialLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(150, 150)
}

func (c *DialLayout) render() *fyne.Container {
	c.face = &canvas.Circle{StrokeColor: theme.DisabledColor(), StrokeWidth: 1}
	c.cover = &canvas.Rectangle{FillColor: theme.BackgroundColor()}
	c.needle = &canvas.Line{StrokeColor: theme.ErrorColor(), StrokeWidth: 7}
	c.speed = &canvas.Text{Text: "60", Color: theme.ForegroundColor(), TextSize: 52}
	c.speed.TextStyle.Monospace = true
	c.speed.Alignment = fyne.TextAlignCenter

	dial := container.NewWithoutLayout(c.face, c.cover)
	for i := range c.pips {
		pip := &canvas.Line{StrokeColor: theme.DisabledColor(), StrokeWidth: 1}
		if i == 0 {
			pip.StrokeColor = theme.ForegroundColor()
		} else if i >= 100 && i < 110 {
			pip.StrokeColor = theme.WarningColor()
		} else if i >= 110 {
			pip.StrokeColor = theme.ErrorColor()
		}
		dial.Add(pip)
		c.pips[i] = pip
	}
	dial.Objects = append(dial.Objects, c.needle, c.speed)
	dial.Layout = c

	c.canvas = dial
	return dial
}

func (c *DialLayout) animate(co fyne.CanvasObject) {
	tick := time.NewTicker(time.Millisecond * 1500)
	go func() {
		for !c.stop {
			start := c.value
			stop := 10 + rand.Float64()*98
			fyne.NewAnimation(time.Millisecond*1500, func(v float32) {
				val := start + (stop-start)*float64(v)
				c.setPosition(val, co.Size())
				c.needle.Refresh()
				c.speed.Refresh()
			}).Start()
			<-tick.C
		}
	}()
}

func (c *DialLayout) SetValue(value float64) {
	go func() {

		start := c.value
		stop := 10 + value*98
		fyne.NewAnimation(time.Millisecond*1, func(v float32) {
			val := start + (stop-start)*float64(v)
			c.setPosition(val, c.content.Size())
			c.needle.Refresh()
			c.speed.Refresh()
		}).Start()

	}()
}

func (c *DialLayout) applyTheme(_ fyne.Settings) {
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

// speedo loads a speedo example window for the specified app context
func NewGauge() (fyne.CanvasObject, func(float64)) {
	s := &DialLayout{}
	s.content = s.render()

	//content := s.render()
	//go s.animate(content)

	listener := make(chan fyne.Settings)
	fyne.CurrentApp().Settings().AddChangeListener(listener)
	go func() {
		for {
			settings := <-listener
			s.applyTheme(settings)
		}
	}()

	return s.content, func(value float64) {
		s.SetValue(value)
	}
}
