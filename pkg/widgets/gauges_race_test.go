package widgets_test

import (
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/dial"
	"github.com/roffe/txlogger/pkg/widgets/dualdial"
	"github.com/roffe/txlogger/pkg/widgets/hbar"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
	"github.com/roffe/txlogger/pkg/widgets/vbar"
	"github.com/roffe/txlogger/pkg/widgets/widebandgauge"
)

type gauge interface {
	fyne.Widget
	SetValue(float64)
}

// Publishing goroutines feed every gauge while the UI goroutine applies,
// renders and resizes them: -race flags any field both sides touch, and every
// readout must end up showing the last value published.
func TestGaugesRace(t *testing.T) {
	onUI := uitest.Start(t)

	// The modern set renders before publishing, which pays the one-off cost of
	// a first render up front; the classic set renders mid-stream.
	var all []gauge
	grid := container.NewGridWithColumns(6)
	var win fyne.Window
	onUI(func() {
		for _, classic := range []bool{false, true} {
			cfg := func() *widgets.GaugeConfig {
				return &widgets.GaugeConfig{Max: 100, Center: 50, Steps: 10, DisplayString: "%.0f", Classic: classic}
			}
			all = append(all, dial.New(cfg()), dualdial.New(cfg()), vbar.New(cfg()),
				hbar.New(cfg()), cbar.New(cfg()), widebandgauge.New(cfg()))
		}
		for _, w := range all[:6] {
			grid.Add(w)
		}
		win = test.NewWindow(grid)
		win.Resize(fyne.NewSize(600, 100))
	})

	publish := func(v, v2 float64) {
		for _, w := range all {
			w.SetValue(v)
			if dd, ok := w.(*dualdial.DualDial); ok {
				dd.SetValue2(v2)
			}
		}
	}
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 300 {
				v := float64((i*7+g*31)%120) - 10 // strays past Min and Max too
				publish(v, 100-v)
			}
		})
	}
	onUI(func() {
		for _, w := range all[6:] {
			grid.Add(w)
		}
	})
	for i := range 5 {
		onUI(func() { win.Resize(fyne.NewSize(600+float32(i)*60, 200+float32(i)*30)) })
	}
	wg.Wait()

	publish(42, 17)
	onUI(func() { // queued behind the final updates
		for _, w := range all {
			// Every gauge draws its readout last; DualDial its secondary after it.
			objs := test.WidgetRenderer(w).Objects()
			got, want := objs[len(objs)-1].(*canvas.Text).Text, "42"
			if _, ok := w.(*dualdial.DualDial); ok {
				got, want = objs[len(objs)-2].(*canvas.Text).Text+"/"+got, "42/17"
			}
			if got != want {
				t.Errorf("%T shows %q, want %q", w, got, want)
			}
		}
	})
}
