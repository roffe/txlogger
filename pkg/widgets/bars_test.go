package widgets_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/hbar"
	"github.com/roffe/txlogger/pkg/widgets/vbar"
)

// In the modern style the first two objects are the track and the fill bar.
func trackAndBar(t *testing.T, w fyne.Widget, size fyne.Size) (track, bar *canvas.Rectangle) {
	t.Helper()
	win := test.NewWindow(w)
	win.SetPadded(false)
	win.Resize(size)
	objs := test.WidgetRenderer(w).Objects()
	track, ok := objs[0].(*canvas.Rectangle)
	if !ok {
		t.Fatalf("object 0 is %T, want the track rectangle", objs[0])
	}
	bar, ok = objs[1].(*canvas.Rectangle)
	if !ok {
		t.Fatalf("object 1 is %T, want the fill rectangle", objs[1])
	}
	return track, bar
}

func TestHBarFillStaysInTrack(t *testing.T) {
	test.NewApp()
	size := fyne.NewSize(200, 60)
	h := hbar.New(&widgets.GaugeConfig{Title: "IGN", Min: 0, Max: 100, Steps: 10, MinSize: size})
	track, bar := trackAndBar(t, h, size)

	for _, tc := range []struct {
		value float64
		width float32
	}{{-50, 0}, {0, 0}, {50, 100}, {100, 200}, {500, 200}} {
		h.SetValue(tc.value)
		if bar.Size().Width != tc.width {
			t.Errorf("value %v: bar width %v, want %v", tc.value, bar.Size().Width, tc.width)
		}
		if bar.Position().Y != track.Position().Y || bar.Size().Height != track.Size().Height {
			t.Errorf("value %v: bar %v/%v outside track %v/%v", tc.value,
				bar.Position(), bar.Size(), track.Position(), track.Size())
		}
	}
	if track.Position().Y+track.Size().Height != size.Height {
		t.Errorf("track %v/%v does not end at the widget bottom %v", track.Position(), track.Size(), size.Height)
	}
}

func TestVBarFillGrowsFromTrackBottom(t *testing.T) {
	test.NewApp()
	size := fyne.NewSize(60, 200)
	v := vbar.New(&widgets.GaugeConfig{Title: "TPS", Min: 0, Max: 100, Steps: 20, MinSize: size})
	track, bar := trackAndBar(t, v, size)
	trackBottom := track.Position().Y + track.Size().Height

	for _, value := range []float64{-50, 0, 50, 100, 500} {
		v.SetValue(value)
		if got := bar.Position().Y + bar.Size().Height; got != trackBottom {
			t.Errorf("value %v: bar bottom %v, want track bottom %v", value, got, trackBottom)
		}
		if bar.Size().Height > track.Size().Height || bar.Position().Y < track.Position().Y {
			t.Errorf("value %v: bar %v/%v outside track %v/%v", value,
				bar.Position(), bar.Size(), track.Position(), track.Size())
		}
	}
	if bar.Size().Height != track.Size().Height { // last value was over Max
		t.Errorf("bar height %v at full scale, want %v", bar.Size().Height, track.Size().Height)
	}
}

func TestCBarFillAnchoredToCenter(t *testing.T) {
	test.NewApp()
	size := fyne.NewSize(200, 40)
	c := cbar.New(&widgets.GaugeConfig{Min: -25, Center: 0, Max: 25, Steps: 20, MinSize: size})
	track, bar := trackAndBar(t, c, size)
	center := size.Width * 0.5

	for _, value := range []float64{-100, -25, -10, 10, 25, 100} {
		c.SetValue(value)
		left, right := bar.Position().X, bar.Position().X+bar.Size().Width
		if value < 0 && right != center {
			t.Errorf("value %v: bar right edge %v, want center %v", value, right, center)
		}
		if value > 0 && left != center {
			t.Errorf("value %v: bar left edge %v, want center %v", value, left, center)
		}
		if left < 0 || right > track.Size().Width {
			t.Errorf("value %v: bar spans %v..%v, outside track width %v", value, left, right, track.Size().Width)
		}
	}
}
