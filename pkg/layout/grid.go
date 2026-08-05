package layout

import (
	"math"

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

	// Snap cell boundaries to whole units so every gap between cells sits at
	// the same subpixel offset and rasterizes identically; the ±1px slack from
	// rounding goes into the cell sizes instead of the gaps.
	// ponytail: snaps in fyne units; snap to device pixels via canvas scale if
	// fractional HiDPI scales still shimmer.
	colEdge := func(c int) float32 {
		return float32(math.Round(float64(size.Width) * float64(c) / float64(g.Cols)))
	}
	rowEdge := func(r int) float32 {
		return float32(math.Round(float64(size.Height) * float64(r) / float64(g.Rows)))
	}

	for i, obj := range objects[:min(len(objects), g.Rows*g.Cols)] {
		row := i / g.Cols // row 0 is the bottom row
		col := i % g.Cols

		x0, x1 := colEdge(col), colEdge(col+1)
		y0, y1 := rowEdge(row), rowEdge(row+1) // measured from the bottom

		obj.Move(fyne.NewPos(x0+g.Padding, size.Height-y1+g.Padding))
		obj.Resize(fyne.Size{
			Width:  x1 - x0 - 2*g.Padding,
			Height: y1 - y0 - 2*g.Padding,
		})
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
