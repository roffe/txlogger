package estimatedoutput

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/browse"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// pickFile makes the file dialogs answer path and returns the options they
// are shown with.
func pickFile(t *testing.T, path string) <-chan browse.Options {
	t.Helper()
	asked := make(chan browse.Options, 1)
	old := widgets.FileDialog
	widgets.FileDialog = func(_ string, o browse.Options) ([]string, error) {
		asked <- o
		return []string{path}, nil
	}
	t.Cleanup(func() { widgets.FileDialog = old })
	return asked
}

// writeLog writes data to a file called name in a temp dir and returns its
// path.
func writeLog(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// pullCSV is a T7 CSV log: 3 s of cruising, then a 6 s pull from 2500 to
// 5500 rpm.
func pullCSV() string {
	sig, _ := logSignalsFor("T7")
	var b strings.Builder
	fmt.Fprintf(&b, "Time,%s,%s\n", sig.rpm, sig.speed)
	t0 := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	for i := range 180 {
		ts := float64(i) * 0.05
		rpm, kmh := 2000.0, 60.0
		if ts >= 3 {
			rpm, kmh = 2500+500*(ts-3), 60+8*(ts-3)
		}
		fmt.Fprintf(&b, "%q,%g,%g\n", t0.Add(time.Duration(i)*50*time.Millisecond).Format(logfile.ISONICO), rpm, kmh)
	}
	return b.String()
}

// loadPicked opens a T7 estimator on the UI goroutine, taps Load log and
// waits for the log info line to report the outcome.
func loadPicked(t *testing.T, path string) (w *Widget, onUI func(func()), o browse.Options, info string) {
	t.Helper()
	onUI = uitest.Start(t)
	asked := pickFile(t, path)
	fw := t7Syms(t).fw()
	onUI(func() {
		w = New(&Config{ECU: "T7", GetFW: func() symbol.FirmwareFile { return fw }})
		walk(w.content, func(c fyne.CanvasObject) {
			if b, ok := c.(*widget.Button); ok && b.Text == "Load log" {
				test.Tap(b)
			}
		})
	})
	select {
	case o = <-asked:
	case <-time.After(10 * time.Second):
		t.Fatal("Load log showed no file dialog")
	}
	deadline := time.Now().Add(10 * time.Second)
	for info == "" {
		if time.Now().After(deadline) {
			t.Fatal("log info still empty after 10s")
		}
		time.Sleep(time.Millisecond)
		onUI(func() { info = w.logInfo.Text })
	}
	return w, onUI, o, info
}

// Load log asks for a log file of a supported type, lists the pulls it finds
// and adds the log curves to the graph.
func TestWidgetLoadLog(t *testing.T) {
	w, onUI, o, info := loadPicked(t, writeLog(t, "pull.csv", pullCSV()))
	if want := []string{"t5l", "t7l", "t8l", "csv", "bpl"}; o.Title != "Open Log file" || len(o.Filters) != 1 || !slices.Equal(o.Filters[0].Extensions, want) {
		t.Errorf("dialog options = %+v, want title \"Open Log file\" and extensions %v", o, want)
	}
	if info != "1 pull(s) found" {
		t.Fatalf("log info = %q, want \"1 pull(s) found\"", info)
	}
	var opts []string
	var selected string
	var legend []string
	onUI(func() {
		opts, selected, legend = w.pullSel.Options, w.pullSel.Selected, legendNames(w)
	})
	if len(opts) != 1 || !strings.HasPrefix(opts[0], "Pull 1: ") || selected != opts[0] {
		t.Errorf("pull selector = %q (selected %q), want one pull, selected", opts, selected)
	}
	if !slices.Contains(legend, "Log torque (Nm)") || !slices.Contains(legend, "Log power (hp)") {
		t.Errorf("legend = %q, want the log curves added", legend)
	}
}

// A picked file that holds no log records reports the parse error and loads
// no pulls.
func TestWidgetLoadLogError(t *testing.T) {
	w, onUI, _, info := loadPicked(t, writeLog(t, "empty.csv", "Time,ActualIn.n_Engine\n"))
	if info != "no valid records in log" {
		t.Fatalf("log info = %q, want the parse error", info)
	}
	var pulls int
	onUI(func() { pulls = len(w.pulls) })
	if pulls != 0 {
		t.Errorf("%d pulls loaded from an empty log", pulls)
	}
}
