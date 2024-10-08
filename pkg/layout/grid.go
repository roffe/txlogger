package layout

import (
	"fyne.io/fyne/v2"
)

type Grid struct {
	Cols, Rows   int
	Text         bool
	widthFactor  float32
	heightFactor float32
}

func (g *Grid) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	g.widthFactor = size.Width / float32(g.Cols)
	g.heightFactor = size.Height / float32(g.Rows)
	for i := 0; i < g.Rows; i++ {
		for j := 0; j < g.Cols; j++ {
			idx := (i * g.Cols) + j
			if idx >= len(objects) {
				return
			}
			obj := objects[idx]
			xPosition := float32(j) * g.widthFactor
			// Adjust y-position to start from the top of the grid and invert the y-axis
			yPosition := size.Height - (float32(i+1) * g.heightFactor)
			if g.Text {
				// Center the object within its grid cell if it's text
				obj.Move(fyne.NewPos(
					xPosition+(g.widthFactor*.5)-(obj.MinSize().Width*.5),
					yPosition,
				))
			} else {
				// Position the object at the top-left corner of its grid cell, adjusted for inverted y-axis
				obj.Move(fyne.NewPos(xPosition, yPosition))
			}
			obj.Resize(fyne.NewSize(g.widthFactor, g.heightFactor))
		}
	}
}

func (g *Grid) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(float32(g.Cols*20), float32(g.Rows*11))
}
