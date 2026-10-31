package camtiming

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// On Linux the file dialogs run in a re-executed copy of the program, marked
// by FP=1, which answers with JSON on stdout. Here that copy is the test
// binary, and TestMain answers with the path the test put in envPick.
const envPick = "CAMTIMING_TEST_PICK"

func TestMain(m *testing.M) {
	if os.Getenv("FP") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"Paths": []string{os.Getenv(envPick)}})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// loadLog runs the Load log button with the dialog answering path and
// returns the VE tab's info line once the load has been applied.
func loadLog(t *testing.T, ecuName, path string) (*Widget, func(func()), string) {
	t.Helper()
	t.Setenv(envPick, path)
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	onUI := uitest.Start(t)
	var w *Widget
	var start string
	onUI(func() {
		w = New(&Config{ECU: ecuName})
		win := test.NewWindow(w)
		win.Resize(fyne.NewSize(1000, 620))
		w.tabs.SelectIndex(1)
		start = w.ve.info.Text
		test.Tap(button(w.ve.content, "Load log"))
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		var info string
		onUI(func() { info = w.ve.info.Text })
		if info != start {
			return w, onUI, info
		}
		if time.Now().After(deadline) {
			t.Fatalf("info line never changed from %q", start)
		}
	}
}

func writeLog(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A picked T7 csv log is read into samples and plotted.
func TestVELoad(t *testing.T) {
	var b strings.Builder
	b.WriteString("Time,ActualIn.n_Engine,MAF.m_AirInlet,ActualIn.p_AirInlet,ActualIn.T_AirInlet,Out.X_AccPedal\n")
	for i, rpm := range []float64{3000, 3000, 3000, 4000, 4000, 4000} {
		fmt.Fprintf(&b, "\"2026-10-10 12:00:0%d,000\",%v,%v,1.5,30,100\n", i, rpm, airFor(100, 1.5, 30))
	}
	w, onUI, info := loadLog(t, "T7", writeLog(t, "pull.csv", b.String()))
	if want := "6 of 6 samples above 80% pedal — peak VE 100% @ 3000 rpm"; info != want {
		t.Errorf("info = %q, want %q", info, want)
	}
	onUI(func() {
		if len(w.ve.samples) != 6 || len(w.ve.chart.series) != 1 || len(w.ve.chart.series[0].pts) != 2 {
			t.Errorf("%d samples, series %+v", len(w.ve.samples), w.ve.chart.series)
		}
	})
}

// A log without the VE channels, or one that cannot be read, leaves no
// samples and says what is wrong.
func TestVELoadFailures(t *testing.T) {
	for _, tt := range []struct {
		name, file, content, want string
	}{
		{"missing channels", "other.csv", "Time,Rpm\n\"2026-10-10 12:00:00,000\",3000\n",
			"no usable records: the log needs ActualIn.n_Engine, MAF.m_AirInlet, ActualIn.p_AirInlet and ActualIn.T_AirInlet"},
		{"no records", "empty.csv", "Time,ActualIn.n_Engine\n", "no valid records in log"},
		{"unknown type", "notes.txt", "hello", "Unsupported filetype"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w, onUI, info := loadLog(t, "T7", writeLog(t, tt.file, tt.content))
			if info != tt.want {
				t.Errorf("info = %q, want %q", info, tt.want)
			}
			onUI(func() {
				if len(w.ve.samples) != 0 || w.ve.chart.series != nil {
					t.Errorf("%d samples, %d series after a failed load", len(w.ve.samples), len(w.ve.chart.series))
				}
			})
		})
	}
}
