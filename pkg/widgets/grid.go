package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewGrid(cols, rows int) *Grid {
	g := &Grid{
		cols: cols,
		rows: rows,
	}
	g.ExtendBaseWidget(g)
	return g
}

type Grid struct {
	widget.BaseWidget
	cols, rows int
}

func (g *Grid) CreateRenderer() fyne.WidgetRenderer {
	con := container.NewWithoutLayout()
	var xlines []*canvas.Line
	for i := 0; i < g.cols; i++ {
		line := canvas.NewLine(color.Black)
		line.StrokeWidth = 1
		xlines = append(xlines, line)
		con.Add(line)
	}
	var ylines []*canvas.Line
	for i := 0; i < g.rows; i++ {
		line := canvas.NewLine(color.Black)
		line.StrokeWidth = 1
		ylines = append(ylines, line)
		con.Add(line)
	}

	return &GridRenderer{
		g:         g,
		container: con,
		xlines:    xlines,
		ylines:    ylines,
	}
}

type GridRenderer struct {
	xlines    []*canvas.Line
	ylines    []*canvas.Line
	g         *Grid
	container *fyne.Container
}

func (gr *GridRenderer) Layout(size fyne.Size) {
	cw := size.Width / float32(gr.g.cols)  // Use cols from the Grid struct
	ch := size.Height / float32(gr.g.rows) // Use rows from the Grid struct

	for i, l := range gr.xlines {
		l.Position1 = fyne.NewPos((float32(i) * cw), 0)
		l.Position2 = fyne.NewPos((float32(i) * cw), size.Height)
		l.Refresh()
	}
	for i, l := range gr.ylines {
		l.Position1 = fyne.NewPos(0, (float32(i) * ch))
		l.Position2 = fyne.NewPos(size.Width, (float32(i) * ch))
		l.Refresh()
	}
}

func (gr *GridRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 100)
}

func (gr *GridRenderer) Refresh() {
}

func (gr *GridRenderer) Destroy() {
}

func (gr *GridRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{gr.container}
}
