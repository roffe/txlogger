package combinedlogplayer

import (
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// ignSym is a dashboard metric (T7/T8 ignition advance) shown as "Ign: x".
const ignSym = "Out.fi_Ignition"

func testLog() logfile.Logfile {
	t0 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.Local)
	return logfile.FromRows(
		[]time.Time{t0, t0.Add(250 * time.Millisecond), t0.Add(500 * time.Millisecond)},
		[]map[string]float64{{ignSym: 12.5}, {ignSym: 14}, {ignSym: 16.25}},
	)
}

// dashTexts collects the text of every canvas.Text the dashboard renders.
// UI goroutine only.
func dashTexts(o fyne.CanvasObject) []string {
	var out []string
	switch v := o.(type) {
	case *canvas.Text:
		out = append(out, v.Text)
	case *fyne.Container:
		for _, c := range v.Objects {
			out = append(out, dashTexts(c)...)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			out = append(out, dashTexts(c)...)
		}
	}
	return out
}

// waitTexts polls the dashboard on the UI goroutine until it shows every
// wanted text, failing after a deadline.
func waitTexts(t *testing.T, onUI func(func()), cp *Widget, want ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	var got []string
	for {
		onUI(func() { got = dashTexts(cp.db) })
		missing := false
		for _, w := range want {
			if !slices.Contains(got, w) {
				missing = true
			}
		}
		if !missing {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("dashboard never showed %q; texts: %q", want, got)
		}
		<-tick.C
	}
}

// startPlayer builds the combined player on lf in a window on the UI
// goroutine, which also starts the log player's playback goroutine.
func startPlayer(t *testing.T, lf logfile.Logfile) (*Widget, func(func())) {
	t.Helper()
	onUI := uitest.Start(t)
	var cp *Widget
	onUI(func() {
		cp = New(&CombinedLogplayerConfig{
			Logfile: lf,
			DBcfg:   &dashboard.Config{Logplayer: true},
		})
		w := test.NewWindow(cp)
		w.Resize(fyne.NewSize(1000, 800))
	})
	t.Cleanup(func() { onUI(cp.Close) })
	return cp, onUI
}

// Stepping the log player with the keyboard feeds each record's values and
// timestamp to the dashboard through the shared bus.
func TestKeysDriveDashboard(t *testing.T) {
	cp, onUI := startPlayer(t, testLog())
	key := func(name fyne.KeyName) {
		onUI(func() { cp.TypedKey(&fyne.KeyEvent{Name: name}) })
	}

	key(fyne.KeyRight)
	waitTexts(t, onUI, cp, "Ign: 12.5", "10:00:00.00")
	key(fyne.KeyRight)
	waitTexts(t, onUI, cp, "Ign: 14.0", "10:00:00.25")
	key(fyne.KeyLeft)
	waitTexts(t, onUI, cp, "Ign: 12.5", "10:00:00.00")
}

// Without an air demand formatter the dashboard is given one that reports
// "Undefined"; a supplied formatter is kept.
func TestAirDemFormatter(t *testing.T) {
	test.NewTempApp(t)

	cfg := &dashboard.Config{}
	cp := New(&CombinedLogplayerConfig{Logfile: testLog(), DBcfg: cfg})
	t.Cleanup(cp.Close)
	if cfg.AirDemToString == nil {
		t.Fatal("AirDemToString left nil")
	}
	if got := cfg.AirDemToString(10); got != "Undefined" {
		t.Errorf("default AirDemToString(10) = %q, want Undefined", got)
	}

	own := func(float64) string { return "mine" }
	cfg2 := &dashboard.Config{AirDemToString: own}
	cp2 := New(&CombinedLogplayerConfig{Logfile: testLog(), DBcfg: cfg2})
	t.Cleanup(cp2.Close)
	if got := cfg2.AirDemToString(10); got != "mine" {
		t.Errorf("supplied AirDemToString replaced, got %q", got)
	}
}

// Close closes the log player, which releases the log, and is safe to call
// twice; keys, runes and focus changes arriving afterwards are ignored
// rather than sent to the stopped playback goroutine (which would panic on
// its closed control channel).
func TestCloseStopsPlayer(t *testing.T) {
	lf := testLog()
	cp, onUI := startPlayer(t, lf)
	onUI(func() { cp.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight}) })
	waitTexts(t, onUI, cp, "Ign: 12.5")

	onUI(func() {
		cp.Close()
		cp.Close()
		cp.TypedRune('i')
		cp.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
		cp.FocusGained()
		cp.FocusLost()
	})
	if n := lf.Len(); n != 0 {
		t.Errorf("log still holds %d records after Close, want it released", n)
	}
	var got []string
	onUI(func() { got = dashTexts(cp.db) })
	if !slices.Contains(got, "Ign: 12.5") {
		t.Errorf("dashboard lost its last values on Close: %q", got)
	}
}
