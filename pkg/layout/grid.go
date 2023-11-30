package layout

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type Grid struct {
	Cols, Rows int
	Text       bool
}

func (g *Grid) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	widthFactor := size.Width / float32(g.Cols)
	heightFactor := size.Height / float32(g.Rows)
	for i := 0; i < g.Rows; i++ {
		for j := 0; j < g.Cols; j++ {
			idx := (i * g.Cols) + j
			if idx >= len(objects) {
				return
			}
			obj := objects[idx]
			xPosition := float32(j) * widthFactor
			// Adjust y-position to start from the top of the grid and invert the y-axis
			yPosition := size.Height - (float32(i+1) * heightFactor)
			obj.Resize(fyne.NewSize(widthFactor, heightFactor))
			if g.Text {
				// Center the object within its grid cell if it's text
				obj.Move(fyne.NewPos(
					xPosition+(widthFactor/2)-(obj.MinSize().Width/2),
					yPosition,
				))
			} else {
				// Position the object at the top-left corner of its grid cell, adjusted for inverted y-axis
				obj.Move(fyne.NewPos(xPosition, yPosition))
			}
		}
	}
}

func (g *Grid) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var width, height float32
	for _, o := range objects {
		if o.MinSize().Width > width {
			width += o.MinSize().Width
		}
		if o.MinSize().Height > height {
			height += o.MinSize().Height
		}
	}
	return fyne.NewSize(width+theme.Padding(), height+theme.Padding())
}
