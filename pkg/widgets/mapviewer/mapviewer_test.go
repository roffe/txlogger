package mapviewer

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestGridGeometry pins the overlays (crosshair, region lines, mouse) to the
// cells laid out by fyne's grid, and ZData row 0 to the bottom.
func TestGridGeometry(t *testing.T) {
	test.NewTempApp(t)
	const cols, rows = 4, 3
	mv, err := New(&Config{
		XData: []float64{0, 1, 2, 3},
		YData: []float64{0, 1, 2},
		ZData: make([]float64, cols*rows),
	})
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewTempWindow(t, mv)
	w.Resize(fyne.NewSize(437.3, 311.7))

	size := mv.innerView.Size()
	pw, ph := cellPitch(size.Width, cols), cellPitch(size.Height, rows)
	near := func(a, b float32) bool { return a-b < 0.01 && b-a < 0.01 }
	cell := mv.cellSize()
	for r := range rows {
		for c := range cols {
			rect := mv.zDataRects[r*cols+c]
			x, y := float32(c)*pw, float32(rows-1-r)*ph // row 0 is the bottom
			if p, s := rect.Position(), rect.Size(); !near(p.X, x) || !near(p.Y, y) || !near(s.Width, cell.Width) || !near(s.Height, cell.Height) {
				t.Fatalf("cell r%d c%d at %v %v, want %v,%v %v", r, c, p, s, x, y, cell)
			}
			if sel := mv.selectionRects[r*cols+c]; sel.Position() != rect.Position() {
				t.Fatalf("selection r%d c%d at %v, want %v", r, c, sel.Position(), rect.Position())
			}
		}
	}

	mv.setXY(2, 1)
	want, ch := mv.zDataRects[1*cols+2], mv.crosshair
	if !near(ch.Position().X, want.Position().X) || !near(ch.Position().Y, want.Position().Y) || !near(ch.Size().Width, want.Size().Width) || !near(ch.Size().Height, want.Size().Height) {
		t.Fatalf("crosshair at %v %v, want %v %v", ch.Position(), ch.Size(), want.Position(), want.Size())
	}

	// middle of cell r1 c2, in widget coordinates
	mid := mv.zDataRects[1*cols+2]
	pos := mv.innerView.Position().Add(mid.Position()).Add(fyne.NewPos(mid.Size().Width/2, mid.Size().Height/2))
	if x, y := mv.calculateSelectionBounds(pos); x != 2 || y != 1 {
		t.Fatalf("hit test got c%d r%d, want c2 r1", x, y)
	}
}
