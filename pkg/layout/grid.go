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
	if size == g.lastSize {
		return
	}
	g.lastSize = size

	// Calculate total padding needed for each dimension
	totalPaddingWidth := float32(g.Cols*2) * g.Padding
	totalPaddingHeight := float32(g.Rows*2) * g.Padding

	// Calculate available space after padding
	availableWidth := size.Width - totalPaddingWidth
	availableHeight := size.Height - totalPaddingHeight

	cellWidth := availableWidth / float32(g.Cols)
	cellHeight := availableHeight / float32(g.Rows)

	for i := 0; i < g.Rows; i++ {
		for j := 0; j < g.Cols; j++ {
			idx := (i * g.Cols) + j
			if idx >= len(objects) {
				return
			}
			obj := objects[idx]
			xPosition := float32(j)*cellWidth + float32(j*2+1)*g.Padding
			yPosition := size.Height - (float32(i+1) * cellHeight) - float32(i*2+1)*g.Padding
			obj.Move(fyne.NewPos(xPosition, yPosition))
			obj.Resize(fyne.NewSize(cellWidth, cellHeight))

		}
	}
}

func (g *Grid) MinSize(objects []fyne.CanvasObject) fyne.Size {

	w := objects[0].MinSize().Width + (2 * g.Padding)
	h := objects[0].MinSize().Height + (2 * g.Padding)

	return fyne.NewSize(w*float32(g.Cols), h*float32(g.Rows))

	//return fyne.NewSize(float32(g.Cols)*g.Padding+totalPaddingWidth, float32(g.Rows)*g.Padding+totalPaddingHeight)
}

// NewGrid creates a new Grid layout with the specified number of columns and rows
func NewGrid(cols, rows int, padding float32) *Grid {
	return &Grid{
		Cols:    cols,
		Rows:    rows,
		Padding: padding,
	}
}
