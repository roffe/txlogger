package gearcalc

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// GEARCALC_DUMP=<path> writes the capture there (used for the webpage docs)
func TestRender(t *testing.T) {
	test.NewApp()
	w := New()
	win := test.NewWindow(w)
	defer win.Close()
	win.Resize(fyne.NewSize(900, 700))
	win.Canvas().Capture() // force the renderers into existence

	// fake a hover so the tooltip shows in the capture
	w.chart.hoverPos = fyne.NewPos(260, 160)
	w.chart.hoverRPM = 4000
	w.chart.renderer.updateHover()

	img := win.Canvas().Capture()
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("captured empty image")
	}

	if path := os.Getenv("GEARCALC_DUMP"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}
