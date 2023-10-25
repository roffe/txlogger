package widgets

import (
	"image/color"

	"fyne.io/fyne/v2/canvas"
)

func NewRectangle(stroke_color color.RGBA, strokeWidth float32) *canvas.Rectangle {
	rectangle := canvas.NewRectangle(color.RGBA{0, 0, 0, 0})
	fil := stroke_color
	fil.A = 0x40
	rectangle.FillColor = fil
	rectangle.StrokeColor = stroke_color
	rectangle.StrokeWidth = strokeWidth
	return rectangle
}

func NewCircle(col color.Color, strokeWidth float32) *canvas.Circle {
	circle := canvas.NewCircle(color.RGBA{0, 0, 0, 0})
	circle.FillColor = color.Transparent
	circle.StrokeColor = col
	circle.StrokeWidth = strokeWidth
	return circle
}
