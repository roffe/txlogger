package gauge

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/widgets"
)

// readouts returns the text of the gauge's readout (and, for DualDial, the
// secondary readout after it); every gauge draws its readouts last.
func readouts(r fyne.WidgetRenderer, n int) string {
	objs := r.Objects()
	s := ""
	for i := len(objs) - n; i < len(objs); i++ {
		if s != "" {
			s += "/"
		}
		s += objs[i].(*canvas.Text).Text
	}
	return s
}

// New builds the widget named by cfg.Type and subscribes it to the symbol's
// ebus topic (and the secondary symbol for DualDial); the returned cancel
// unsubscribes it.
func TestNewSubscribesByType(t *testing.T) {
	test.NewTempApp(t)
	for _, tc := range []struct {
		typ, wantType string
		readouts      int
		want          string
	}{
		{"Dial", "*dial.Dial", 1, "42"},
		{"DualDial", "*dualdial.DualDial", 2, "42/17"},
		{"VBar", "*vbar.VBar", 1, "42"},
		{"HBar", "*hbar.HBar", 1, "42"},
		{"CBar", "*cbar.CBar", 1, "42"},
		{"Wideband", "*widebandgauge.WidebandGauge", 1, "42"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			sym := "gauge_test." + tc.typ
			cfg := &widgets.GaugeConfig{
				Type: tc.typ, Max: 100, Center: 50, Steps: 10, DisplayString: "%.0f",
				SymbolName: sym, SymbolNameSecondary: sym + ".2",
			}
			obj, cancel, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(cancel)
			if got := fmt.Sprintf("%T", obj); got != tc.wantType {
				t.Fatalf("New built %s, want %s", got, tc.wantType)
			}
			if g, ok := obj.(widgets.Gauge); !ok || g.GetConfig() != cfg {
				t.Error("gauge does not report the config it was built from")
			}
			w := obj.(fyne.Widget)
			r := test.TempWidgetRenderer(t, w)
			w.Resize(fyne.NewSize(200, 200))

			ebus.Publish(sym, 42)
			ebus.Publish(sym+".2", 17)
			if got := readouts(r, tc.readouts); got != tc.want {
				t.Errorf("after publish: readout %q, want %q", got, tc.want)
			}

			cancel()
			ebus.Publish(sym, 7)
			ebus.Publish(sym+".2", 8)
			if got := readouts(r, tc.readouts); got != tc.want {
				t.Errorf("after cancel: readout %q, want %q", got, tc.want)
			}
		})
	}
}

// An unknown type is an error and builds nothing.
func TestNewUnknownType(t *testing.T) {
	obj, cancel, err := New(&widgets.GaugeConfig{Type: "Sundial"})
	if err == nil || obj != nil || cancel != nil {
		t.Errorf("New(Sundial) = %v, cancel set %v, err %v; want nil, false, error", obj, cancel != nil, err)
	}
}
