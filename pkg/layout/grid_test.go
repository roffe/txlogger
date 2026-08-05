package layout

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

func TestGridGapsConstant(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	const cols, rows = 18, 16
	g := NewGrid(cols, rows, 1.32)
	objs := make([]fyne.CanvasObject, cols*rows)
	for i := range objs {
		objs[i] = canvas.NewRectangle(nil)
	}
	near := func(a, b float32) bool { return a-b < 0.001 && b-a < 0.001 }
	// awkward sizes that don't divide evenly
	for _, size := range []fyne.Size{{Width: 733.7, Height: 461.3}, {Width: 641, Height: 500.5}} {
		g.lastSize = fyne.Size{}
		g.Layout(objs, size)
		for i := 0; i < cols-1; i++ {
			a, b := objs[i], objs[i+1]
			gap := b.Position().X - (a.Position().X + a.Size().Width)
			if want := 2 * g.Padding; !near(gap, want) {
				t.Fatalf("size %v col %d: horizontal gap %v, want %v", size, i, gap, want)
			}
		}
		for r := 0; r < rows-1; r++ {
			upper, lower := objs[(r+1)*cols], objs[r*cols] // row 0 is bottom
			gap := lower.Position().Y - (upper.Position().Y + upper.Size().Height)
			if want := 2 * g.Padding; !near(gap, want) {
				t.Fatalf("size %v row %d: vertical gap %v, want %v", size, r, gap, want)
			}
		}
	}
}
