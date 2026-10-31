package icon

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

func newTestIcon(text string) *Icon {
	return New(&Config{
		Image:   canvas.NewImageFromResource(nil),
		Text:    text,
		Minsize: fyne.NewSize(120, 110),
	})
}

// New configures the image to keep its aspect ratio at the configured
// minimum size, and styles the overlay text as large green monospace.
func TestNewConfiguresImageAndText(t *testing.T) {
	test.NewTempApp(t)
	ic := newTestIcon("")
	img := ic.cfg.Image
	if img.FillMode != canvas.ImageFillContain {
		t.Errorf("FillMode %v, want ImageFillContain", img.FillMode)
	}
	if img.ScaleMode != canvas.ImageScaleFastest {
		t.Errorf("ScaleMode %v, want ImageScaleFastest", img.ScaleMode)
	}
	if got := img.MinSize(); got != fyne.NewSize(120, 110) {
		t.Errorf("image MinSize %v, want 120x110", got)
	}
	if ic.text.Color != (color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF}) {
		t.Errorf("text colour %v", ic.text.Color)
	}
	if ic.text.TextSize != 25 || !ic.text.TextStyle.Monospace {
		t.Errorf("text size %v monospace %v, want 25 monospace", ic.text.TextSize, ic.text.TextStyle.Monospace)
	}
}

// SetText shows the new text and records it in the config; setting the text
// already recorded is skipped (the dashboard sets it on every knock frame).
func TestSetText(t *testing.T) {
	test.NewTempApp(t)
	ic := newTestIcon("")
	ic.SetText("12")
	if ic.text.Text != "12" || ic.cfg.Text != "12" {
		t.Fatalf("text %q cfg %q, want 12", ic.text.Text, ic.cfg.Text)
	}
	ic.text.Text = "stale" // would be overwritten if the repeat were applied
	ic.SetText("12")
	if ic.text.Text != "stale" {
		t.Errorf("repeated SetText(%q) was applied again", "12")
	}
	ic.SetText("")
	if ic.text.Text != "" || ic.cfg.Text != "" {
		t.Fatalf("text %q cfg %q after clearing", ic.text.Text, ic.cfg.Text)
	}
}

// Config.Text is the icon's initial text: it is displayed from the start,
// and setting it again keeps it displayed.
func TestConfigTextIsDisplayed(t *testing.T) {
	test.NewTempApp(t)
	ic := newTestIcon("4")
	if ic.text.Text != "4" {
		t.Errorf("initial text %q, want Config.Text %q", ic.text.Text, "4")
	}
	ic.SetText("4")
	if ic.text.Text != "4" {
		t.Errorf("after SetText(%q) text is %q", "4", ic.text.Text)
	}
}

// The renderer draws the image then the text; the image is pinned to the
// top-left at its minimum size and the text sits in a 30px band at (14,87)
// spanning the widget width.
func TestRendererLayout(t *testing.T) {
	test.NewTempApp(t)
	ic := newTestIcon("")
	r := test.TempWidgetRenderer(t, ic)
	objs := r.Objects()
	if len(objs) != 2 || objs[0] != ic.cfg.Image || objs[1] != ic.text {
		t.Fatalf("objects %v, want [image text]", objs)
	}
	if got := r.MinSize(); got != fyne.NewSize(120, 110) {
		t.Errorf("MinSize %v, want 120x110", got)
	}
	r.Layout(fyne.NewSize(300, 200))
	if p, s := ic.cfg.Image.Position(), ic.cfg.Image.Size(); p != fyne.NewPos(0, 0) || s != fyne.NewSize(120, 110) {
		t.Errorf("image at %v size %v", p, s)
	}
	if p, s := ic.text.Position(), ic.text.Size(); p != fyne.NewPos(14, 87) || s != fyne.NewSize(300, 30) {
		t.Errorf("text at %v size %v", p, s)
	}
}
