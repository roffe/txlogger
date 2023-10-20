package widgets

import (
	"image/color"

	"fyne.io/fyne/v2/canvas"
)

func NewRectangle(col color.Color, strokeWidth float32) *canvas.Rectangle {
	Rectangle := canvas.NewRectangle(color.RGBA{0, 0, 0, 0})
	Rectangle.FillColor = color.Transparent
	Rectangle.StrokeColor = col
	Rectangle.StrokeWidth = strokeWidth
	return Rectangle
}

func NewCircle(col color.Color) *canvas.Circle {
	circle := canvas.NewCircle(color.RGBA{0, 0, 0, 0})
	circle.FillColor = color.Transparent
	circle.StrokeColor = col
	circle.StrokeWidth = 4
	return circle
}
