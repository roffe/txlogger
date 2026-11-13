package matrixbuilder

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
)

func TestSetAxes(t *testing.T) {
	test.NewTempApp(t)
	mb := New(meshgrid.RenderBackend(0))

	x, y := []float64{1, 2, 3}, []float64{10, 20, 30, 40, 50}
	if err := mb.SetAxes(x, y); err != nil {
		t.Fatal(err)
	}
	if mb.cols != 3 || mb.rows != 5 || len(mb.zData) != 15 || len(mb.xEntries) != 3 || len(mb.yEntries) != 5 {
		t.Fatalf("matrix not resized: %dx%d, %d cells, %d/%d entries", mb.cols, mb.rows, len(mb.zData), len(mb.xEntries), len(mb.yEntries))
	}
	x[0] = 99
	if !slices.Equal(mb.xAxis, []float64{1, 2, 3}) {
		t.Fatalf("x axis aliases the caller's slice: %v", mb.xAxis)
	}
	if got := mb.yEntries[4].Text; got != "50" {
		t.Fatalf("y entry = %q, want 50", got)
	}

	if err := mb.SetAxes(make([]float64, maxAxis+1), y); err == nil {
		t.Fatal("oversized axis accepted")
	}
	if mb.cols != 3 {
		t.Fatalf("rejected axes changed the matrix: %d cols", mb.cols)
	}

	// Rows beyond the window height must scroll instead of growing the widget.
	if err := mb.SetAxes(x, make([]float64, maxAxis)); err != nil {
		t.Fatal(err)
	}
	if h := mb.MinSize().Height; h > 300 {
		t.Fatalf("min height %v grows with the row count", h)
	}
}
