package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/roffe/txlogger/pkg/layout"
)

func NewGrid(cols, rows int) fyne.CanvasObject {
	con := container.NewStack()
	var xlines []*canvas.Line
	for i := 0; i < cols; i++ {
		line := canvas.NewLine(color.Black)
		xlines = append(xlines, line)
		con.Add(line)
	}
	var ylines []*canvas.Line
	for i := 0; i < rows; i++ {
		line := canvas.NewLine(color.Black)
		ylines = append(ylines, line)
		con.Add(line)
	}
	g := &layout.Grid{
		Xlines: xlines,
		Ylines: ylines,
	}
	con.Layout = g
	return con
}
