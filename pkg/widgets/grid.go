package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Grid struct {
	widget.BaseWidget
	cols, rows int

	lines []*canvas.Line

	content *fyne.Container

	lastSize fyne.Size
}

func NewGrid(cols, rows int) *Grid {
	g := &Grid{
		cols: cols,
		rows: rows,
	}
	g.ExtendBaseWidget(g)

	g.content = container.NewWithoutLayout()
	totalLines := cols + rows
	g.lines = make([]*canvas.Line, totalLines)

	for i := 0; i < totalLines; i++ {
		g.lines[i] = canvas.NewLine(color.Black)
		g.lines[i].StrokeWidth = 1
		g.content.Add(g.lines[i])
	}

	return g
}

func (g *Grid) Resize(size fyne.Size) {
	if size == g.lastSize {
		return
	}
	g.lastSize = size

	cellWidth := size.Width / float32(g.cols)
	cellHeight := size.Height / float32(g.rows)

	for i := 0; i < g.cols; i++ {
		l := g.lines[i]
		x := float32(i) * cellWidth
		l.Position1 = fyne.NewPos(x, 0)
		l.Position2 = fyne.NewPos(x, size.Height)
		l.Refresh()
	}

	offset := g.cols
	for i := 0; i < g.rows; i++ {
		l := g.lines[offset+i]
		y := float32(i) * cellHeight
		l.Position1 = fyne.NewPos(0, y)
		l.Position2 = fyne.NewPos(size.Width, y)
		l.Refresh()
	}
}

func (g *Grid) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.content)
}
