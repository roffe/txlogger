package layout

import (
	"fyne.io/fyne/v2"
)

type Grid struct {
	Cols, Rows  int
	Text        bool
	MinimumSize fyne.Size

	lastSize fyne.Size
}

func (g *Grid) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if size == g.lastSize {
		return
	}
	g.lastSize = size

	cellWidth := size.Width / float32(g.Cols)
	cellHeight := size.Height / float32(g.Rows)

	for i := 0; i < g.Rows; i++ {
		for j := 0; j < g.Cols; j++ {
			idx := (i * g.Cols) + j
			if idx >= len(objects) {
				return
			}
			obj := objects[idx]
			xPosition := float32(j) * cellWidth
			// Adjust y-position to start from the top of the grid and invert the y-axis
			yPosition := size.Height - (float32(i+1) * cellHeight)
			if g.Text {
				// Center the object within its grid cell if it's text
				obj.Move(fyne.NewPos(
					xPosition+(cellWidth*.5)-(obj.MinSize().Width*.5),
					yPosition+(cellHeight*.5)-(obj.MinSize().Height*.5),
				))
			} else {
				// Position the object at the top-left corner of its grid cell, adjusted for inverted y-axis
				obj.Move(fyne.NewPos(xPosition, yPosition))
				obj.Resize(fyne.NewSize(cellWidth, cellHeight))
			}
		}
	}
}

func (g *Grid) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return g.MinimumSize
}
