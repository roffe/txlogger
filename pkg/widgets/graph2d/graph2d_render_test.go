package graph2d

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/colors"
)

// battery correction table from the T7Suite screenshot the widget mimics
func testGraph(t testing.TB) *Graph {
	t.Helper()
	return New(&Config{
		AxisData:       []float64{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Values:         []float64{4590, 3605, 2785, 2145, 1755, 1495, 1295, 1145, 1025, 925, 845, 765},
		AxisPrecision:  0,
		ValuePrecision: 0,
		AxisLabel:      "Battery voltage",
		ColorblindMode: colors.ModeNormal,
	})
}

func TestRender(t *testing.T) {
	test.NewApp()
	g := testGraph(t)
	w := test.NewWindow(g)
	defer w.Close()
	w.Resize(fyne.NewSize(640, 420))

	g.SetCursor(3.5)
	g.SetValues(700, 4700, []float64{4700, 3605, 2785, 2145, 1755, 1495, 1295, 1145, 1025, 925, 845, 700})

	img := w.Canvas().Capture()
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("captured empty image")
	}

	if os.Getenv("GRAPH2D_DUMP") != "" {
		f, err := os.Create("/tmp/graph2d.png")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

// many cells in a small window exercise the callout stagger/skip and the
// x label thinning
func TestRenderCrowded(t *testing.T) {
	test.NewApp()
	values := make([]float64, 32)
	for i := range values {
		values[i] = 1000 + 500*float64(i%5)
	}
	g := New(&Config{
		Values:         values,
		ValuePrecision: 0,
		ColorblindMode: colors.ModeNormal,
	})
	w := test.NewWindow(g)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))

	img := w.Canvas().Capture()
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("captured empty image")
	}

	if os.Getenv("GRAPH2D_DUMP") != "" {
		f, err := os.Create("/tmp/graph2d_crowded.png")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

// a mismatched axis must fall back to index labels and a flat line of equal
// values must not divide by zero
func TestRenderDegenerate(t *testing.T) {
	test.NewApp()
	g := New(&Config{
		Values:         []float64{42, 42, 42, 42},
		ColorblindMode: colors.ModeNormal,
	})
	w := test.NewWindow(g)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 200))
	if img := w.Canvas().Capture(); img.Bounds().Dx() == 0 {
		t.Fatal("captured empty image")
	}
}
