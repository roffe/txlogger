package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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
	g := &Grid{
		Xlines: xlines,
		Ylines: ylines,
	}
	con.Layout = g
	return con
}

type Grid struct {
	Xlines []*canvas.Line
	Ylines []*canvas.Line
}

func (g *Grid) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	cw := (size.Width) / float32(len(g.Xlines))
	ch := (size.Height) / float32(len(g.Ylines))

	stroke := float32(8)

	for i, l := range g.Xlines {
		l.Move(fyne.NewPos((float32(i)*cw)-stroke/2, 0))
		l.Resize(fyne.NewSize(stroke, size.Height))
	}
	for i, l := range g.Ylines {
		l.Move(fyne.NewPos(0, (float32(i)*ch)-stroke/2))
		l.Resize(fyne.NewSize(size.Width, stroke))
	}
}

func (g *Grid) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(100, 100)
}
