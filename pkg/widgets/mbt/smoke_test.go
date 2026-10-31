package mbt

import (
	"os"
	"testing"

	"fyne.io/fyne/v2/test"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
)

// TestWidgetSmoke builds and renders the widget against a real binary, so
// a broken renderer or a bad axis assumption fails here instead of in the
// app. Point it at a T7 bin: BIN=/path/to/file.bin go test ./pkg/widgets/mbt/
func TestWidgetSmoke(t *testing.T) {
	path := os.Getenv("BIN")
	if path == "" {
		t.Skip("set BIN=/path/to/a/t7.bin to run")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	_, fw, err := symbol.Load(path, data, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	app := test.NewApp()
	defer app.Quit()

	w := New(&Config{
		GetFW:      func() symbol.FirmwareFile { return fw },
		Colorblind: colors.ModeNormal,
	})
	win := test.NewWindow(w)
	defer win.Close()
	win.Resize(w.MinSize())
	t.Log(w.status.Text)
	t.Log(w.pointInfo.Text)
	w.fit()
	t.Log(w.status.Text)
	for _, v := range views {
		w.view.SetSelected(v)
	}
	t.Log("views ok")
}
