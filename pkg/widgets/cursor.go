package widgets

import (
	"image/color"

	"fyne.io/fyne/v2/canvas"
)

func NewTracker() *canvas.Rectangle {
	tracker := canvas.NewRectangle(color.RGBA{0, 0, 0, 0})
	tracker.FillColor = color.Transparent
	tracker.StrokeColor = color.RGBA{0xfc, 0x4a, 0xaa, 255}
	tracker.StrokeWidth = 4
	return tracker
}

func NewCursor() *canvas.Circle {
	cursor := canvas.NewCircle(color.RGBA{0, 0, 0, 0})
	cursor.FillColor = color.Transparent
	cursor.StrokeColor = color.RGBA{0x0c, 0x4a, 0xaa, 255}
	cursor.StrokeWidth = 4
	return cursor
}
