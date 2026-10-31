package ledicon

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

var (
	green = color.RGBA{0x00, 0xFF, 0x00, 0xFF}
	grey  = color.RGBA{0x80, 0x80, 0x80, 0xFF}
)

// A new LED is off: grey, with the given label, and green as its on colour.
func TestNewIsOff(t *testing.T) {
	test.NewTempApp(t)
	w := New("CAN")
	if w.ColorOn != green || w.ColorOff != grey {
		t.Errorf("colours on %v off %v", w.ColorOn, w.ColorOff)
	}
	if w.ledicon.FillColor != grey {
		t.Errorf("initial fill %v, want off colour", w.ledicon.FillColor)
	}
	if w.label.Text != "CAN" || w.Text != "CAN" {
		t.Errorf("label %q text %q, want CAN", w.label.Text, w.Text)
	}
}

// On, Off and SetState switch the fill between the on and off colours,
// using whatever colours are configured at the time of the switch.
func TestStateSwitchesColour(t *testing.T) {
	test.NewTempApp(t)
	w := New("Ignition")
	w.ColorOn = color.RGBA{0xFF, 0x00, 0x00, 0xFF}

	steps := []struct {
		name string
		do   func()
		want color.Color
	}{
		{"On", w.On, w.ColorOn},
		{"On again", w.On, w.ColorOn},
		{"Off", w.Off, grey},
		{"SetState(true)", func() { w.SetState(true) }, w.ColorOn},
		{"SetState(false)", func() { w.SetState(false) }, grey},
	}
	for _, s := range steps {
		s.do()
		if w.ledicon.FillColor != s.want {
			t.Errorf("%s: fill %v, want %v", s.name, w.ledicon.FillColor, s.want)
		}
	}
}

// Setting the state it already has leaves the fill alone, even if the
// configured colour changed in between.
func TestSameStateIsNoop(t *testing.T) {
	test.NewTempApp(t)
	w := New("x")
	w.On()
	w.ColorOn = color.RGBA{0x01, 0x02, 0x03, 0xFF}
	w.On()
	if w.ledicon.FillColor != green {
		t.Errorf("fill %v, want unchanged %v", w.ledicon.FillColor, green)
	}
}

// The renderer places a 20px LED left of the label, sizes itself to the
// label plus the LED, and Refresh picks up a changed Text.
func TestRenderer(t *testing.T) {
	test.NewTempApp(t)
	w := New("Engine stopped")
	r := test.TempWidgetRenderer(t, w)
	objs := r.Objects()
	if len(objs) != 2 || objs[0] != w.ledicon || objs[1] != w.label {
		t.Fatalf("objects %v, want [led label]", objs)
	}
	if got, want := r.MinSize(), fyne.NewSize(20+w.label.MinSize().Width, 34); got != want {
		t.Errorf("MinSize %v, want %v", got, want)
	}

	r.Layout(fyne.NewSize(200, 34))
	if p, s := w.ledicon.Position(), w.ledicon.Size(); p != fyne.NewPos(0, 8) || s != fyne.NewSize(20, 20) {
		t.Errorf("led at %v size %v", p, s)
	}
	if p, s := w.label.Position(), w.label.Size(); p != fyne.NewPos(20, 0) || s != fyne.NewSize(180, 32) {
		t.Errorf("label at %v size %v", p, s)
	}

	w.Text = "Engine running"
	w.Refresh()
	if w.label.Text != "Engine running" {
		t.Errorf("label %q after Refresh, want the new Text", w.label.Text)
	}
}
