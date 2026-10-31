package boosttuner

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/browse"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// On Linux, widgets.SelectFiles shows the file dialog by re-executing the
// running binary as a child process and reading the chosen paths from its
// stdout. In a test that binary is this test binary, so TestMain answers as
// the dialog child whenever envDialogReply is set.
const (
	envDialogReply   = "TXL_BT_TEST_DIALOG_REPLY"   // printed on stdout as the child's answer
	envDialogRequest = "TXL_BT_TEST_DIALOG_REQUEST" // file the child copies its stdin to
)

func TestMain(m *testing.M) {
	if reply, ok := os.LookupEnv(envDialogReply); ok {
		req, _ := io.ReadAll(os.Stdin)
		if path := os.Getenv(envDialogRequest); path != "" {
			if err := os.WriteFile(path, req, 0o600); err != nil {
				os.Exit(99)
			}
		}
		os.Stdout.WriteString(reply)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// dialogRequest is what the dialog child receives on stdin.
type dialogRequest struct {
	Op      string
	Options browse.Options
}

// fakeDialog makes the next file dialog choose paths and returns a function
// that reads back the request the dialog child received.
func fakeDialog(t *testing.T, paths ...string) func() dialogRequest {
	t.Helper()
	reply, err := json.Marshal(struct{ Paths []string }{paths})
	if err != nil {
		t.Fatal(err)
	}
	reqFile := filepath.Join(t.TempDir(), "request.json")
	t.Setenv(envDialogReply, string(reply))
	t.Setenv(envDialogRequest, reqFile)
	// A -race child otherwise idles for a second before it exits.
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	return func() dialogRequest {
		t.Helper()
		b, err := os.ReadFile(reqFile)
		if err != nil {
			t.Fatalf("dialog child got no request: %v", err)
		}
		var req dialogRequest
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("dialog request %q: %v", b, err)
		}
		return req
	}
}

// notifyApp wraps the current test app so the test hears each time a
// function posted with fyne.Do has finished on the UI goroutine.
type notifyApp struct {
	fyne.App
	drv *notifyDriver
}

func (a *notifyApp) Driver() fyne.Driver { return a.drv }

type notifyDriver struct {
	fyne.Driver
	ran chan struct{}
}

func (d *notifyDriver) DoFromGoroutine(f func(), wait bool) {
	d.Driver.DoFromGoroutine(func() {
		f()
		select {
		case d.ran <- struct{}{}:
		default: // a wake-up is already pending
		}
	}, wait)
}

// startUI installs a test app whose fyne.Do runs on one UI goroutine (see
// uitest) and returns onUI, which runs f there and waits, plus waitUI, which
// blocks until cond, evaluated on the UI goroutine, holds and reports false if
// it never does. Functions run through onUI must not call t.Fatal.
func startUI(t *testing.T) (onUI func(func()), waitUI func(cond func() bool) bool) {
	t.Helper()
	uitest.Start(t)
	inner := fyne.CurrentApp()
	d := &notifyDriver{Driver: inner.Driver(), ran: make(chan struct{}, 1)}
	fyne.SetCurrentApp(&notifyApp{App: inner, drv: d})

	// The test's own functions go straight to the UI goroutine so they do not
	// count as wake-ups.
	onUI = func(f func()) { d.Driver.DoFromGoroutine(f, true) }
	waitUI = func(cond func() bool) bool {
		deadline := time.After(30 * time.Second)
		for {
			var ok bool
			onUI(func() { ok = cond() })
			if ok {
				return true
			}
			select {
			case <-d.ran:
			case <-deadline:
				return false
			}
		}
	}
	return onUI, waitUI
}

// lockedBuffer collects log output written from any goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// "Add log files" asks for the supported log types, parses every chosen file
// off the UI goroutine, merges the good ones in order, resolves the channel
// checklist, hides the progress modal and reports how many files failed.
func TestOpenLogDialog(t *testing.T) {
	logs := captureLog(t)
	onUI, waitUI := startUI(t)

	dir := t.TempDir()
	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	run1 := write("run1.csv", boostLog(repeat(sample{rpm: 3000}, 2)...))
	bad := write("bad.csv", "Time,A\n")
	run2 := write("run2.csv", csvLog([]string{"ActualIn.n_Engine", "Extra"}, []float64{4000, 1}, []float64{4100, 2}, []float64{4200, 3}))
	request := fakeDialog(t, run1, bad, run2)
	syms := fakeBinary(t, nil)

	var bt *BoostTuner
	var w fyne.Window
	onUI(func() {
		bt = New(Config{Symbols: syms})
		w = test.NewWindow(bt)
	})
	// Nothing else runs on the UI until the tap, so the tree can be read here.
	add := findButton(t, tab(t, bt, "Logs"), "Add log files")
	onUI(func() { test.Tap(add) })

	want := "2 file(s), 5 records:\nrun1.csv\nrun2.csv\nLoaded 2 file(s), 1 failed"
	var got string
	if !waitUI(func() bool { got = bt.logsLabel.Text; return got == want }) {
		t.Fatalf("log list = %q, want %q", got, want)
	}

	req := request()
	wantReq := dialogRequest{Op: "open_files", Options: browse.Options{
		Title:   "Open Log file",
		Filters: []browse.Filter{{Name: "Log file", Extensions: []string{"t5l", "t7l", "t8l", "csv", "bpl"}}},
	}}
	if !reflect.DeepEqual(req, wantReq) {
		t.Errorf("dialog request = %+v, want %+v", req, wantReq)
	}

	onUI(func() {
		defer w.Close()
		if bt.nrecords != 5 || !reflect.DeepEqual(bt.loadedFiles, []string{"run1.csv", "run2.csv"}) {
			t.Errorf("nrecords = %d, files = %v", bt.nrecords, bt.loadedFiles)
		}
		if rpm := bt.values["ActualIn.n_Engine"]; !reflect.DeepEqual(rpm, []float64{3000, 3000, 4000, 4100, 4200}) {
			t.Errorf("rpm = %v", rpm)
		}
		if extra := bt.values["Extra"]; len(extra) != 5 || !math.IsNaN(extra[0]) || !math.IsNaN(extra[1]) || extra[2] != 1 || extra[4] != 3 {
			t.Errorf("Extra = %v, want NaN padding then [1 2 3]", extra)
		}
		if bt.order[0] != "ActualIn.n_Engine" || len(bt.order) != len(boostCols)+1 {
			t.Errorf("order = %v", bt.order)
		}
		if m := bt.missingChannels(); len(m) != 0 {
			t.Errorf("missing = %v, want none", m)
		}
		for i, row := range bt.channelList.Objects {
			if icons := findAll[*widget.Icon](row, nil); len(icons) != 1 || icons[0].Resource.Name() != theme.ConfirmIcon().Name() {
				t.Errorf("checklist row %d icons = %v, want the confirm icon", i, icons)
			}
		}
		if top := w.Canvas().Overlays().Top(); top != nil {
			t.Errorf("progress modal still shown: %T", top)
		}
	})
	if !strings.Contains(logs.String(), "boosttuner: ") || !strings.Contains(logs.String(), "no valid records") {
		t.Errorf("parse failure not logged: %q", logs.String())
	}
}

// When every chosen file fails to parse, nothing is merged and the log list
// says so under the empty-state text.
func TestOpenLogDialog_AllFail(t *testing.T) {
	captureLog(t)
	onUI, waitUI := startUI(t)
	bad := filepath.Join(t.TempDir(), "bad.csv")
	if err := os.WriteFile(bad, []byte("Time,A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeDialog(t, bad)

	var bt *BoostTuner
	onUI(func() {
		bt = New(Config{})
		bt.openLogDialog()
	})
	want := "No log files loaded\nLoaded 0 file(s), 1 failed"
	var got string
	if !waitUI(func() bool { got = bt.logsLabel.Text; return got == want }) {
		t.Fatalf("log list = %q, want %q", got, want)
	}
	onUI(func() {
		if len(bt.values) != 0 || bt.nrecords != 0 || bt.loadedFiles != nil {
			t.Errorf("state after failed load: %d series, %d records, files %v", len(bt.values), bt.nrecords, bt.loadedFiles)
		}
	})
}
