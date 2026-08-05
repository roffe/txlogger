package grid

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type Grid struct {
	widget.BaseWidget
	cols, rows int

	lines []*canvas.Line

	lastSize fyne.Size
}

func New(cols, rows int) *Grid {
	g := &Grid{
		cols: cols,
		rows: rows,
	}
	g.ExtendBaseWidget(g)

	totalLines := cols + rows

	g.lines = make([]*canvas.Line, totalLines)
	for i := 0; i < totalLines; i++ {
		g.lines[i] = canvas.NewLine(theme.Color(theme.ColorNameBackground))
		g.lines[i].StrokeWidth = 2
	}

	return g
}

func (g *Grid) CreateRenderer() fyne.WidgetRenderer {
	return &gridRenderer{G: g}
}

type gridRenderer struct {
	G       *Grid
	objects []fyne.CanvasObject
}

func (g *gridRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 100)
}

func (g *gridRenderer) Layout(size fyne.Size) {
	if size == g.G.lastSize {
		return
	}
	g.G.lastSize = size

	cellWidth := size.Width / float32(g.G.cols)
	cellHeight := size.Height / float32(g.G.rows)

	// update vertical lines
	for i := 0; i < g.G.cols; i++ {
		l := g.G.lines[i]
		x := float32(i) * cellWidth
		l.Position1 = fyne.NewPos(x, 0)
		l.Position2 = fyne.NewPos(x, size.Height)
		l.Refresh()
	}

	// update horizontal lines
	offset := g.G.cols
	for i := 0; i < g.G.rows; i++ {
		l := g.G.lines[offset+i]
		y := float32(i) * cellHeight
		l.Position1 = fyne.NewPos(0, y)
		l.Position2 = fyne.NewPos(size.Width, y)
		l.Refresh()
	}
}

func (g *gridRenderer) Refresh() {
}

func (g *gridRenderer) Destroy() {
}

func (g *gridRenderer) Objects() []fyne.CanvasObject {
	if g.objects == nil {
		g.objects = make([]fyne.CanvasObject, len(g.G.lines))
		for i, l := range g.G.lines {
			g.objects[i] = l
		}
	}
	return g.objects
}
