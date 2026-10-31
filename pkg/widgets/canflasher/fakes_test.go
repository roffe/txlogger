package canflasher

import (
	"context"
	"errors"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t7"
	"github.com/roffe/txlogger/pkg/model"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

// The flasher is driven against fake ECU clients registered in the real ecu
// registry and a fake adapter registered in the real gocan registry, so every
// action runs its whole path: settings -> adapter -> bus -> ecu.New -> client.
const (
	fakeECU       = "Fake ECU"        // resets over CAN when done
	fakeManualECU = "Fake ECU manual" // ManualReset: the user resets it
	fakeEOLECU    = "Fake EOL ECU"    // also implements eolFlasher
	fakeAdapter   = "canflasher fake adapter"
)

var (
	curMu sync.Mutex
	cur   *rig // the running test's rig; tests in this package are not parallel
)

func current() *rig {
	curMu.Lock()
	defer curMu.Unlock()
	return cur
}

func init() {
	ecu.Register(&ecu.EcuInfo{Name: fakeECU, CANRate: 500, Filter: []uint32{0x7E8}, NewFunc: newFakeClient})
	ecu.Register(&ecu.EcuInfo{Name: fakeManualECU, CANRate: 615.384, Filter: []uint32{0x258, 0x266}, ManualReset: true, NewFunc: newFakeClient})
	ecu.Register(&ecu.EcuInfo{Name: fakeEOLECU, CANRate: 500, Filter: []uint32{0x258}, NewFunc: func(c *gocan.Bus, cfg *ecu.Config) ecu.Client {
		return &fakeEOLClient{newFakeClient(c, cfg).(*fakeClient)}
	}})
	if err := gocan.Register(gocan.AdapterInfo{Name: fakeAdapter, New: newFakeAdapter}); err != nil {
		panic(err)
	}
}

// rig is one flasher in a test window on a uitest UI goroutine, plus the
// record of everything the fakes were asked to do.
type rig struct {
	t    *testing.T
	onUI func(func())
	w    *CanFlasherWidget
	win  fyne.Window

	notes    chan *fyne.Notification
	finished chan struct{} // a worker closed its bus or failed to open it

	mu         sync.Mutex
	calls      []string
	adapterCfg *gocan.Config
	adapterGID string // goroutine the adapter was built on
	ecuCfg     *ecu.Config
	openErr    error            // returned by the adapter's Open
	errs       map[string]error // returned by the named client method
	hook       func(method string)
	info       []model.HeaderResult
	dump       []byte
	written    []byte // bin handed to Flash/Recover/EOLFlash
	pin        string
	eolParams  t7.EOLParams
}

// notifyApp records notifications instead of handing them to the OS.
type notifyApp struct {
	fyne.App
	notes chan *fyne.Notification
}

func (a *notifyApp) SendNotification(n *fyne.Notification) { a.notes <- n }

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		t:        t,
		onUI:     uitest.Start(t),
		notes:    make(chan *fyne.Notification, 8),
		finished: make(chan struct{}, 8),
		errs:     map[string]error{},
		dump:     []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04},
	}
	fyne.SetCurrentApp(&notifyApp{App: fyne.CurrentApp(), notes: r.notes})
	fyne.CurrentApp().Preferences().SetString("adapter", fakeAdapter)
	curMu.Lock()
	cur = r
	curMu.Unlock()
	t.Cleanup(func() {
		curMu.Lock()
		cur = nil
		curMu.Unlock()
	})
	r.onUI(func() {
		r.w = New(&Config{CSW: settings.New(&settings.Config{})})
		// the flasher shows its dialogs on the first window, the main
		// window in the app; in the test driver that is its default one
		r.win = fyne.CurrentApp().Driver().AllWindows()[0]
		r.win.SetContent(r.w)
		r.win.Resize(fyne.NewSize(800, 600))
	})
	return r
}

func (r *rig) record(call string) {
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
}

func (r *rig) callLog() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func (r *rig) setErr(method string, err error) {
	r.mu.Lock()
	r.errs[method] = err
	r.mu.Unlock()
}

// enter records a client call, runs the test's hook and returns the error
// the test set for it.
func (r *rig) enter(method string) error {
	r.mu.Lock()
	r.calls = append(r.calls, method)
	hook, err := r.hook, r.errs[method]
	r.mu.Unlock()
	if hook != nil {
		hook(method)
	}
	return err
}

func (r *rig) selectECU(name string) {
	r.onUI(func() { r.w.ecuSelect.SetSelected(name) })
}

// wait blocks until the running worker has closed its bus (or failed to open
// it) and the buttons are enabled again. Disable is queued before the bus is
// opened, so seeing the buttons enabled after that means Enable has run.
func (r *rig) wait() {
	r.t.Helper()
	select {
	case <-r.finished:
	case <-time.After(30 * time.Second):
		r.t.Fatal("worker never closed its bus")
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		var busy bool
		r.onUI(func() { busy = r.w.infoBTN.Disabled() })
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			r.t.Fatal("buttons never re-enabled")
		}
	}
}

var stamp = regexp.MustCompile(`^\d\d:\d\d:\d\d\.\d{3} - `)

// logLines returns the messages in the log pane without their timestamps.
func (r *rig) logLines() []string {
	var text string
	r.onUI(func() { text = r.w.logText.Text })
	var out []string
	for l := range strings.SplitSeq(strings.TrimSuffix(text, "\n"), "\n") {
		if l != "" {
			out = append(out, stamp.ReplaceAllString(l, ""))
		}
	}
	return out
}

func (r *rig) wantLog(msgs ...string) {
	r.t.Helper()
	got := r.logLines()
	for _, m := range msgs {
		if !slices.Contains(got, m) {
			r.t.Errorf("log %q has no line %q", got, m)
		}
	}
}

func (r *rig) wantCalls(want ...string) {
	r.t.Helper()
	if got := r.callLog(); !slices.Equal(got, want) {
		r.t.Errorf("calls = %q, want %q", got, want)
	}
}

// tap taps a widget on the UI goroutine.
func (r *rig) tap(o fyne.Tappable) { r.onUI(func() { test.Tap(o) }) }

// tapDialog taps the button labelled text on the dialog on screen.
func (r *rig) tapDialog(text string) {
	r.t.Helper()
	var found bool
	r.onUI(func() {
		for _, o := range dialogObjects(r.win) {
			if b, ok := o.(*widget.Button); ok && b.Text == text && b.Visible() {
				test.Tap(b)
				found = true
				return
			}
		}
	})
	if !found {
		r.t.Fatalf("no %q button on screen; dialog text %q", text, r.dialogText())
	}
}

// dialogText is every label on the dialog on screen, "" when none is shown.
func (r *rig) dialogText() string {
	var parts []string
	r.onUI(func() {
		for _, o := range dialogObjects(r.win) {
			switch o := o.(type) {
			case *widget.Label:
				parts = append(parts, o.Text)
			case *canvas.Text:
				parts = append(parts, o.Text)
			}
		}
	})
	return strings.Join(parts, "|")
}

func dialogObjects(win fyne.Window) []fyne.CanvasObject {
	top := win.Canvas().Overlays().Top()
	if top == nil {
		return nil
	}
	return test.LaidOutObjects(top)
}

// goid is the running goroutine's id, from its stack header.
func goid() string {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	return strings.Fields(string(b))[1]
}

// fakeClient answers the flasher's calls from the rig and drives the config
// callbacks the real clients use for progress and messages.
type fakeClient struct {
	r   *rig
	cfg *ecu.Config
}

func newFakeClient(_ *gocan.Bus, cfg *ecu.Config) ecu.Client {
	r := current()
	r.mu.Lock()
	r.ecuCfg = cfg
	r.mu.Unlock()
	return &fakeClient{r: r, cfg: cfg}
}

func (c *fakeClient) Info(context.Context) ([]model.HeaderResult, error) {
	err := c.r.enter("Info")
	c.r.mu.Lock()
	defer c.r.mu.Unlock()
	return c.r.info, err
}

func (c *fakeClient) DumpECU(context.Context) ([]byte, error) {
	if err := c.r.enter("DumpECU"); err != nil {
		return nil, err
	}
	c.cfg.OnMessage("dumping")
	c.r.mu.Lock()
	bin := slices.Clone(c.r.dump)
	c.r.mu.Unlock()
	c.cfg.OnProgress(float64(len(bin)))
	return bin, nil
}

func (c *fakeClient) write(method string, bin []byte) error {
	if err := c.r.enter(method); err != nil {
		return err
	}
	c.r.mu.Lock()
	c.r.written = slices.Clone(bin)
	c.r.mu.Unlock()
	return nil
}

func (c *fakeClient) FlashECU(_ context.Context, bin []byte) error {
	return c.write("FlashECU", bin)
}

func (c *fakeClient) RecoverECU(_ context.Context, bin []byte) error {
	return c.write("RecoverECU", bin)
}

func (c *fakeClient) MarryECU(_ context.Context, pin string) error {
	if err := c.r.enter("MarryECU"); err != nil {
		return err
	}
	c.r.mu.Lock()
	c.r.pin = pin
	c.r.mu.Unlock()
	return nil
}

func (c *fakeClient) ResetECU(context.Context) error { return c.r.enter("ResetECU") }

// The flasher never reads DTCs, prints info or erases on its own.
func (c *fakeClient) ReadDTC(context.Context) ([]dtc.DTC, error) { return nil, errUnused }
func (c *fakeClient) PrintECUInfo(context.Context) error         { return errUnused }
func (c *fakeClient) EraseECU(context.Context) error             { return errUnused }

var errUnused = errors.New("not used by the flasher")

type fakeEOLClient struct{ *fakeClient }

func (c *fakeEOLClient) EOLFlash(_ context.Context, bin []byte, p t7.EOLParams) error {
	if err := c.write("EOLFlash", bin); err != nil {
		return err
	}
	c.r.mu.Lock()
	c.r.eolParams = p
	c.r.mu.Unlock()
	return nil
}

// fakeAdapter is the CAN interface the settings hand out; it records how it
// was configured and emits one event when opened.
type fakeAdapterDev struct{ r *rig }

func newFakeAdapter(cfg gocan.Config) (gocan.Adapter, error) {
	r := current()
	r.mu.Lock()
	r.adapterCfg = &cfg
	r.adapterGID = goid()
	r.mu.Unlock()
	return &fakeAdapterDev{r: r}, nil
}

func (a *fakeAdapterDev) Open(_ context.Context, bus *gocan.Bus) error {
	a.r.record("open")
	a.r.mu.Lock()
	err := a.r.openErr
	a.r.mu.Unlock()
	if err != nil {
		a.r.finished <- struct{}{}
		return err
	}
	bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "fake adapter up"})
	return nil
}

func (a *fakeAdapterDev) Send(context.Context, gocan.Frame) error { return nil }

func (a *fakeAdapterDev) Close() error {
	a.r.record("close")
	a.r.finished <- struct{}{}
	return nil
}
