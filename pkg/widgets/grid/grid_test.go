package grid

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// New makes one line per column and per row, drawn in the theme background
// colour with a 2px stroke, and the renderer exposes exactly those lines.
func TestNewLines(t *testing.T) {
	test.NewTempApp(t)
	g := New(4, 3)
	objs := test.TempWidgetRenderer(t, g).Objects()
	if len(objs) != 7 {
		t.Fatalf("%d objects, want 7 (4 columns + 3 rows)", len(objs))
	}
	for i, o := range objs {
		l, ok := o.(*canvas.Line)
		if !ok {
			t.Fatalf("object %d is %T, want *canvas.Line", i, o)
		}
		if l != g.lines[i] {
			t.Errorf("object %d is not lines[%d]", i, i)
		}
		if l.StrokeWidth != 2 {
			t.Errorf("line %d stroke %v, want 2", i, l.StrokeWidth)
		}
		if l.StrokeColor != theme.Color(theme.ColorNameBackground) {
			t.Errorf("line %d colour %v, want theme background", i, l.StrokeColor)
		}
	}
}

// Layout splits the size into equal cells: vertical lines at every column
// start spanning the full height, horizontal lines at every row start
// spanning the full width.
func TestLayoutPositionsLines(t *testing.T) {
	test.NewTempApp(t)
	g := New(4, 3)
	r := test.TempWidgetRenderer(t, g)
	if got := r.MinSize(); got != fyne.NewSize(100, 100) {
		t.Errorf("MinSize %v, want 100x100", got)
	}
	r.Layout(fyne.NewSize(400, 300))

	type seg struct{ p1, p2 fyne.Position }
	want := []seg{
		{fyne.NewPos(0, 0), fyne.NewPos(0, 300)},
		{fyne.NewPos(100, 0), fyne.NewPos(100, 300)},
		{fyne.NewPos(200, 0), fyne.NewPos(200, 300)},
		{fyne.NewPos(300, 0), fyne.NewPos(300, 300)},
		{fyne.NewPos(0, 0), fyne.NewPos(400, 0)},
		{fyne.NewPos(0, 100), fyne.NewPos(400, 100)},
		{fyne.NewPos(0, 200), fyne.NewPos(400, 200)},
	}
	for i, w := range want {
		l := g.lines[i]
		if l.Position1 != w.p1 || l.Position2 != w.p2 {
			t.Errorf("line %d: %v-%v, want %v-%v", i, l.Position1, l.Position2, w.p1, w.p2)
		}
	}
}

// Layout skips the work when the size has not changed, and recomputes the
// lines once it does.
func TestLayoutCachesSize(t *testing.T) {
	test.NewTempApp(t)
	g := New(2, 2)
	r := test.TempWidgetRenderer(t, g)
	r.Layout(fyne.NewSize(200, 200))

	g.lines[1].Position1 = fyne.NewPos(-1, -1) // would be reset by a real layout pass
	r.Layout(fyne.NewSize(200, 200))
	if g.lines[1].Position1 != fyne.NewPos(-1, -1) {
		t.Errorf("same-size Layout recomputed lines: %v", g.lines[1].Position1)
	}

	r.Layout(fyne.NewSize(100, 50))
	if got, want := g.lines[1].Position1, fyne.NewPos(50, 0); got != want {
		t.Errorf("after resize line 1 starts at %v, want %v", got, want)
	}
	if got, want := g.lines[3].Position2, fyne.NewPos(100, 25); got != want {
		t.Errorf("after resize line 3 ends at %v, want %v", got, want)
	}
}
