package widgets

import (
	"image/color"

	"fyne.io/fyne/v2/canvas"
)

func NewRectangle(strokeColor color.RGBA, strokeWidth float32) *canvas.Rectangle {
	return &canvas.Rectangle{
		FillColor:   color.RGBA{0, 0, 0, 0},
		StrokeColor: strokeColor,
		StrokeWidth: strokeWidth,
	}
}

func NewCrosshair(strokeColor color.RGBA, strokeWidth float32) *canvas.Rectangle {
	return &canvas.Rectangle{
		FillColor:   strokeColor,
		StrokeColor: strokeColor,
		StrokeWidth: strokeWidth,
	}
}

func NewCircle(col color.Color, strokeWidth float32) *canvas.Circle {
	circle := canvas.NewCircle(color.RGBA{0, 0, 0, 0})
	circle.FillColor = color.Transparent
	circle.StrokeColor = col
	circle.StrokeWidth = strokeWidth
	return circle
}
