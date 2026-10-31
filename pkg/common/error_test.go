package common

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// collect walks a rendered object tree through containers and the cached
// widget renderers.
func collect(o fyne.CanvasObject, out []fyne.CanvasObject) []fyne.CanvasObject {
	out = append(out, o)
	switch c := o.(type) {
	case *fyne.Container:
		for _, child := range c.Objects {
			out = collect(child, out)
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(c).Objects() {
			out = collect(child, out)
		}
	}
	return out
}

type dialogParts struct {
	title, message *widget.Label
	button         *widget.Button
	image          *canvas.Image
}

func findParts(o fyne.CanvasObject, title, message string) dialogParts {
	var p dialogParts
	for _, obj := range collect(o, nil) {
		switch v := obj.(type) {
		case *widget.Label:
			switch v.Text {
			case title:
				p.title = v
			case message:
				p.message = v
			}
		case *widget.Button:
			p.button = v
		case *canvas.Image:
			p.image = v
		}
	}
	return p
}

// firstWindow starts a test app and returns its first window, the one
// ShowError attaches to (the test driver always creates one).
func firstWindow(t *testing.T) fyne.Window {
	t.Helper()
	app := test.NewTempApp(t)
	w := app.Driver().AllWindows()[0]
	w.SetContent(widget.NewLabel("main"))
	w.Resize(fyne.NewSize(800, 600))
	return w
}

// ShowError opens a dialog on the first app window with the title, the
// centered error text, the zoned-out image and an OK button that closes it.
func TestShowError(t *testing.T) {
	w := firstWindow(t)

	ShowError("Dang it!", errors.New("adapter not found"))

	overlay := w.Canvas().Overlays().Top()
	if overlay == nil {
		t.Fatal("no dialog shown")
	}
	p := findParts(overlay, "Dang it!", "adapter not found")
	if p.title == nil {
		t.Error("title label missing")
	}
	if p.message == nil {
		t.Fatal("error message label missing")
	}
	if p.message.Alignment != fyne.TextAlignCenter {
		t.Errorf("message alignment = %v, want center", p.message.Alignment)
	}
	if p.image == nil || p.image.Resource == nil || p.image.Resource.Name() != "zonedout.png" {
		t.Errorf("zoned-out image missing: %+v", p.image)
	}
	if p.button == nil || p.button.Text != "OK" {
		t.Fatalf("OK button missing: %+v", p.button)
	}

	test.Tap(p.button)
	if top := w.Canvas().Overlays().Top(); top != nil {
		t.Fatalf("dialog still shown after OK: %v", top)
	}
}

// Two error dialogs can be open at once (MainWindow.Error posts each error on
// its own). Each dialog's image stays laid out for that dialog, so closing a
// wider dialog leaves the narrower one beneath it intact.
func TestShowErrorStackedDialogsKeepOwnImageLayout(t *testing.T) {
	w := firstWindow(t)

	ShowError("first", errors.New("short"))
	first := findParts(w.Canvas().Overlays().Top(), "first", "short")
	long := strings.Repeat("a considerably longer error message ", 4)
	ShowError("second", errors.New(long))
	second := findParts(w.Canvas().Overlays().Top(), "second", long)
	if first.image == nil || first.message == nil || second.button == nil {
		t.Fatal("dialog parts missing")
	}

	test.Tap(second.button)
	if got, want := first.image.Size().Width, first.message.Size().Width; got != want {
		t.Fatalf("first dialog image is %v wide in a %v wide dialog", got, want)
	}
	if first.image == second.image {
		t.Fatal("both dialogs hold the same *canvas.Image")
	}
}
