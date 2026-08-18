package camtiming

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/logfile"
)

// TestDocsScreenshots writes the pictures the webpage documentation uses.
// It is a no-op unless CAMTIMING_DOCS names an output directory:
//
//	CAMTIMING_DOCS=../../../../txlogger-webpage/static \
//	CAMTIMING_DOCS_LOG=~/logs/a-wot-pull.t7l go test ./pkg/widgets/camtiming/ -run Docs
//
// Without a log the VE shot is skipped rather than faked.
func TestDocsScreenshots(t *testing.T) {
	dir := os.Getenv("CAMTIMING_DOCS")
	if dir == "" {
		t.Skip("CAMTIMING_DOCS not set")
	}
	test.NewApp()
	w := New(&Config{ECU: "T7"})
	win := test.NewWindow(w)
	defer win.Close()
	win.Resize(fyne.NewSize(1100, 940))

	shoot := func(name string) {
		img := win.Canvas().Capture()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", filepath.Join(dir, name))
	}

	// stock B235R cams with a Catcams Sport-2 grind laid over them
	w.intake.compare.SetSelected(find("Catcams Sport-2", false).Label())
	w.exhaust.compare.SetSelected(find("Catcams Sport-2", true).Label())
	win.Canvas().Capture()
	test.MoveMouse(win.Canvas(), fyne.NewPos(845, 420))
	shoot("cam-timing.png")

	logPath := os.Getenv("CAMTIMING_DOCS_LOG")
	if logPath == "" {
		t.Log("CAMTIMING_DOCS_LOG not set, skipping the VE shot")
		return
	}
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lf, err := logfile.Open(logPath, f)
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	// the VE tab's panel is short, so it does not need the tall window the
	// timing tab does
	win.Resize(fyne.NewSize(1100, 660))
	w.tabs.SelectIndex(1)
	w.ve.disp.SetText(os.Getenv("CAMTIMING_DOCS_CC"))
	w.ve.samples = readSamples(lf, veSignalsByECU["T7"])
	w.ve.recalc()
	win.Canvas().Capture()
	test.MoveMouse(win.Canvas(), fyne.NewPos(700, 300))
	shoot("cam-timing-ve.png")
}
