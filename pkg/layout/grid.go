package layout

import (
	"fyne.io/fyne/v2"
)

type Grid struct {
	Cols, Rows int
	Padding    float32 // Added padding field
	lastSize   fyne.Size
}

func (g *Grid) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Skip if size hasn't changed and avoid unnecessary calculations
	if size == g.lastSize {
		return
	}
	g.lastSize = size

	// Pre-calculate common values outside the loops
	padding2 := g.Padding * 2
	cellWidth := (size.Width - float32(g.Cols)*padding2) / float32(g.Cols)
	cellHeight := (size.Height - float32(g.Rows)*padding2) / float32(g.Rows)

	// Calculate base positions once
	baseY := size.Height - cellHeight - g.Padding

	for i, obj := range objects[:min(len(objects), g.Rows*g.Cols)] {
		row := i / g.Cols
		col := i % g.Cols

		obj.Move(fyne.NewPos(
			float32(col)*(cellWidth+padding2)+g.Padding,
			baseY-float32(row)*(cellHeight+padding2),
		))
		obj.Resize(fyne.Size{Width: cellWidth, Height: cellHeight})
	}
}

func (g *Grid) MinSize(objects []fyne.CanvasObject) fyne.Size {

	w := objects[0].MinSize().Width + (2 * g.Padding)
	h := objects[0].MinSize().Height + (2 * g.Padding)

	return fyne.Size{Width: w * float32(g.Cols), Height: h * float32(g.Rows)}

	//return fyne.NewSize(float32(g.Cols)*g.Padding+totalPaddingWidth, float32(g.Rows)*g.Padding+totalPaddingHeight)
}

// NewGrid creates a new Grid layout with the specified number of columns and rows
func NewGrid(cols, rows int, padding float32) *Grid {
	return &Grid{
		Cols:    max(cols, 1),
		Rows:    max(rows, 1),
		Padding: padding,
	}
}
