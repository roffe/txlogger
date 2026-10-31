package ipc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/windows"
)

// home is the fake home directory for the whole test binary. MainWindow.Log
// and MainWindow.Error write through pkg/debug to <home>/txlogger/txlogger.log,
// opened once per process, so HOME must point here before the first window
// is built.
var home string

func TestMain(m *testing.M) {
	var err error
	home, err = os.MkdirTemp("", "txlogger-ipc-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, "txlogger"), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

var (
	mwOnce sync.Once
	mwApp  fyne.App
	mw     *windows.MainWindow
	uiDrv  *uiDriver
)

// uiApp is the test app with fyne.Do routed through uiDriver.
type uiApp struct {
	fyne.App
	drv *uiDriver
}

func (a *uiApp) Driver() fyne.Driver { return a.drv }

// uiDriver queues fyne.Do functions for the test goroutine to run with runUI,
// the way the GLFW driver hands them to the main goroutine. The stock test
// driver runs them inline on the caller, so the dialog MainWindow.Error shows
// through fyne.Do from a goroutine of its own would be laid out while the
// test goroutine builds the next test's widgets.
type uiDriver struct {
	fyne.Driver
	queue chan func()
}

func (d *uiDriver) DoFromGoroutine(f func(), wait bool) {
	if wait {
		f()
		return
	}
	d.queue <- f
}

// runUI runs the fyne.Do functions queued so far.
func runUI() {
	for {
		select {
		case f := <-uiDrv.queue:
			f()
		default:
			return
		}
	}
}

// mainWindow returns one MainWindow on an in-memory test app, shared by every
// test and never closed: MainWindow.Error shows its dialog on the app's first
// window, which must still exist when runUI gets to it.
func mainWindow(t *testing.T) (fyne.App, *windows.MainWindow) {
	t.Helper()
	mwOnce.Do(func() {
		a := test.NewApp()
		uiDrv = &uiDriver{Driver: a.Driver(), queue: make(chan func(), 64)}
		mwApp = &uiApp{App: a, drv: uiDrv}
		fyne.SetCurrentApp(mwApp)
		mw = windows.NewMainWindow(mwApp)
	})
	runUI()
	return mwApp, mw
}

// debugLogSince returns what was appended to the debug log after offset.
func debugLogSince(t *testing.T, offset int64) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "txlogger", "txlogger.log"))
	if err != nil {
		if os.IsNotExist(err) && offset == 0 {
			return ""
		}
		t.Fatal(err)
	}
	if int64(len(b)) < offset {
		t.Fatalf("debug log shrank from %d to %d bytes", offset, len(b))
	}
	return string(b[offset:])
}

func debugLogSize() int64 {
	st, err := os.Stat(filepath.Join(home, "txlogger", "txlogger.log"))
	if err != nil {
		return 0
	}
	return st.Size()
}

// writeCSV writes a two-row txlogger CSV log under a unique name, so the
// shared MainWindow never already has a log player of that name open.
func writeCSV(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "run-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("Time,ActualIn.n_Engine\n" +
		"\"2026-10-10 12:00:00,000\",850\n" +
		"\"2026-10-10 12:00:00,050\",900\n"); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// An "open" request for an existing .csv loads it into a log player and adds
// it to the recent files list, with no reply sent back.
func TestOpenHandlerLoadsCSV(t *testing.T) {
	app, w := mainWindow(t)
	path := writeCSV(t)
	off := debugLogSize()

	if reply := CreateIPCRouter(w)["open"](path); reply != nil {
		t.Errorf("open reply = %v, want nil", reply)
	}
	if recent := app.Preferences().StringList("recentFiles"); len(recent) == 0 || recent[0] != path {
		t.Errorf("recent files = %v, want %s first", recent, path)
	}
	if log := debugLogSince(t, off); !strings.Contains(log, "loaded log file "+path) {
		t.Errorf("debug log has no load line for %s:\n%s", path, log)
	}
}

// The plain show request (empty filename) and names that are neither .bin
// nor .csv only focus the window: nothing is loaded and nothing is logged.
func TestOpenHandlerShowOnly(t *testing.T) {
	app, w := mainWindow(t)
	r := CreateIPCRouter(w)
	other := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", other, "run.t7l"} {
		before := app.Preferences().StringList("recentFiles")
		off := debugLogSize()
		if reply := r["open"](name); reply != nil {
			t.Errorf("open(%q) reply = %v, want nil", name, reply)
		}
		if log := debugLogSince(t, off); log != "" {
			t.Errorf("open(%q) logged:\n%s", name, log)
		}
		if after := app.Preferences().StringList("recentFiles"); strings.Join(after, "\n") != strings.Join(before, "\n") {
			t.Errorf("open(%q) changed recent files from %v to %v", name, before, after)
		}
	}
}

// A .csv that cannot be opened is reported once, with the os.Open error; the
// handler must not go on to parse the nil file as a log.
func TestOpenHandlerMissingCSVSingleError(t *testing.T) {
	_, w := mainWindow(t)
	path := filepath.Join(t.TempDir(), "gone.csv")
	off := debugLogSize()
	CreateIPCRouter(w)["open"](path)
	runUI()
	log := debugLogSince(t, off)
	if !strings.Contains(log, "open "+path+": no such file or directory") {
		t.Errorf("debug log does not report the open error for %s:\n%s", path, log)
	}
	if strings.Contains(log, "failed to open log file") {
		t.Errorf("a missing csv also tried to parse the log:\n%s", log)
	}
}

// A .bin that cannot be loaded is reported to the user, as every other
// LoadSymbolsFromFile caller does.
func TestOpenHandlerBadBinReported(t *testing.T) {
	_, w := mainWindow(t)
	path := filepath.Join(t.TempDir(), "gone.bin")
	off := debugLogSize()
	CreateIPCRouter(w)["open"](path)
	runUI()
	if log := debugLogSince(t, off); !strings.Contains(log, "error reading file") {
		t.Errorf("debug log does not report the failed .bin load:\n%s", log)
	}
}
