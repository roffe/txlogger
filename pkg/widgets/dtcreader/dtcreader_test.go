package dtcreader

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/dtc/wis"
	"github.com/roffe/txlogger/pkg/ecusim"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// recorder collects the log lines and errors the reader reports, from
// whichever goroutine reports them.
type recorder struct {
	mu   sync.Mutex
	logs []string
	errs []error
}

func (r *recorder) log(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, s)
}

func (r *recorder) err(e error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, e)
}

func (r *recorder) snapshot() ([]string, []error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.logs), slices.Clone(r.errs)
}

// onlyErr returns the single reported error, failing on none or several.
func (r *recorder) onlyErr(t *testing.T) error {
	t.Helper()
	_, errs := r.snapshot()
	if len(errs) != 1 {
		t.Fatalf("%d errors reported, want 1: %v", len(errs), errs)
	}
	return errs[0]
}

// t5Firmware is a T5 symbol table with three one-byte DTC flags in SRAM,
// plus symbols the DTC reader must skip: a two-byte "_error" one and an
// ordinary map.
func t5Firmware() symbol.FirmwareFile {
	return symbol.NewCollection(
		&symbol.Symbol{Name: "Hast_fel", Number: 1, SramOffset: 0x1002, Length: 1},
		&symbol.Symbol{Name: "Adapt_error", Number: 2, SramOffset: 0x1000, Length: 1},
		&symbol.Symbol{Name: "Cat_error", Number: 3, SramOffset: 0x1001, Length: 1},
		&symbol.Symbol{Name: "Wide_error", Number: 4, SramOffset: 0x1010, Length: 2},
		&symbol.Symbol{Name: "Rpm", Number: 5, SramOffset: 0x1020, Length: 2},
	)
}

// failingOpen is an adapter whose port cannot be opened.
type failingOpen struct{}

func (failingOpen) Open(context.Context, *gocan.Bus) error  { return errors.New("port busy") }
func (failingOpen) Send(context.Context, gocan.Frame) error { return nil }
func (failingOpen) Close() error                            { return nil }

// fixture is a rendered reader driven from a uitest UI goroutine.
type fixture struct {
	t    *testing.T
	onUI func(func())
	d    *DTCReader
	rec  *recorder
}

// newFixture renders a reader on a uitest UI goroutine, so the background
// read/clear goroutine and the UI run concurrently under -race.
func newFixture(t *testing.T, ecu string, fw symbol.FirmwareFile, dev gocan.Adapter, devErr error) *fixture {
	t.Helper()
	f := &fixture{t: t, onUI: uitest.Start(t), rec: &recorder{}}
	f.onUI(func() {
		f.d = New(
			func() symbol.FirmwareFile { return fw },
			func() string { return ecu },
			func() (gocan.Adapter, error) { return dev, devErr },
			f.rec.log, f.rec.err,
		)
		w := test.NewWindow(f.d)
		w.Resize(fyne.NewSize(500, 400))
	})
	return f
}

// tapAndWait taps a button and waits for the background job to re-enable
// the buttons.
func (f *fixture) tapAndWait(btn func() *widget.Button) {
	f.t.Helper()
	f.onUI(func() { test.Tap(btn()) })
	deadline := time.Now().Add(30 * time.Second)
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		var readOff, clearOff bool
		f.onUI(func() { readOff, clearOff = f.d.readBtn.Disabled(), f.d.clearBtn.Disabled() })
		if !readOff && !clearOff {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatal("buttons never re-enabled")
		}
		<-tick.C
	}
}

func (f *fixture) dtcs() []dtc.DTC {
	var out []dtc.DTC
	f.onUI(func() { out = slices.Clone(f.d.dtcs) })
	return out
}

func (f *fixture) setDTCs(d []dtc.DTC) {
	f.onUI(func() { f.d.dtcs = d; f.d.Refresh() })
}

func (f *fixture) read() *widget.Button  { return f.d.readBtn }
func (f *fixture) clear() *widget.Button { return f.d.clearBtn }

// Read DTCS on a T5 reads every one-byte error flag of the loaded binary
// from the ECU's RAM and lists the non-zero ones, sorted by name.
func TestReadT5DTCs(t *testing.T) {
	sim := &ecusim.T5{}
	sim.Poke(0x1000, []byte{3, 0, 7})    // Adapt_error, Cat_error, Hast_fel
	sim.Poke(0x1010, []byte{0xFF, 0xFF}) // Wide_error: not a flag
	f := newFixture(t, "T5", t5Firmware(), sim, nil)

	f.tapAndWait(f.read)

	want := []dtc.DTC{
		{ECU: symbol.ECU_T5, Code: "Adapt_error", Status: 3},
		{ECU: symbol.ECU_T5, Code: "Hast_fel", Status: 7},
	}
	if got := f.dtcs(); !slices.Equal(got, want) {
		t.Fatalf("dtcs %+v, want %+v", got, want)
	}
	var n int
	f.onUI(func() { n = f.d.list.Length() })
	if n != 2 {
		t.Errorf("list shows %d rows, want 2", n)
	}
	logs, errs := f.rec.snapshot()
	if len(errs) != 0 {
		t.Errorf("errors: %v", errs)
	}
	if len(logs) < 2 || logs[0] != "Reading DTCs..." || logs[1] != "Connecting to device *ecusim.T5" {
		t.Errorf("logs %q", logs)
	}
}

// Clear DTCS on a T5 zeroes the set error flags in RAM and empties the list.
func TestClearT5DTCs(t *testing.T) {
	sim := &ecusim.T5{}
	sim.Poke(0x1000, []byte{3, 0, 7})
	f := newFixture(t, "T5", t5Firmware(), sim, nil)
	f.setDTCs([]dtc.DTC{{ECU: symbol.ECU_T5, Code: "Adapt_error", Status: 3}})

	f.tapAndWait(f.clear)

	if got := sim.Peek(0x1000, 3); !slices.Equal(got, []byte{0, 0, 0}) {
		t.Errorf("flags after clear % X, want zeroed", got)
	}
	if got := f.dtcs(); got != nil {
		t.Errorf("dtcs %+v after clear, want none", got)
	}
	logs, errs := f.rec.snapshot()
	if len(errs) != 0 {
		t.Errorf("errors: %v", errs)
	}
	if len(logs) == 0 || logs[0] != "Clearing DTCs..." {
		t.Errorf("logs %q", logs)
	}
}

// ECUs without DTC support report an error straight away and leave the
// buttons usable.
func TestUnsupportedECU(t *testing.T) {
	for _, tc := range []struct {
		name string
		btn  func(*fixture) *widget.Button
		want string
	}{
		{"read", (*fixture).read, "DTC reading not supported for ECU AW55"},
		{"clear", (*fixture).clear, "DTC clearing not supported for ECU AW55"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "AW55", nil, nil, errors.New("adapter must not be opened"))
			f.tapAndWait(func() *widget.Button { return tc.btn(f) })
			if err := f.rec.onlyErr(t); err.Error() != tc.want {
				t.Errorf("error %q, want %q", err, tc.want)
			}
		})
	}
}

// When the job fails the error is reported, the buttons come back and the
// listed DTCs are left as they were.
func TestJobFailures(t *testing.T) {
	old := []dtc.DTC{{ECU: symbol.ECU_T5, Code: "Cat_error", Status: 1}}
	for _, tc := range []struct {
		name   string
		btn    func(*fixture) *widget.Button
		fw     symbol.FirmwareFile
		dev    gocan.Adapter
		devErr error
		want   string
	}{
		{"read no adapter", (*fixture).read, t5Firmware(), nil, errors.New("no adapter selected"), "no adapter selected"},
		{"clear no adapter", (*fixture).clear, t5Firmware(), nil, errors.New("no adapter selected"), "no adapter selected"},
		{"read open fails", (*fixture).read, t5Firmware(), failingOpen{}, nil, "port busy"},
		{"read no binary", (*fixture).read, nil, &ecusim.T5{}, nil, "it is required to load a binary file to read DTCs"},
		{"clear no binary", (*fixture).clear, nil, &ecusim.T5{}, nil, "it is required to load a binary file to read DTCs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "T5", tc.fw, tc.dev, tc.devErr)
			f.setDTCs(old)
			f.tapAndWait(func() *widget.Button { return tc.btn(f) })
			if err := f.rec.onlyErr(t); err.Error() != tc.want {
				t.Errorf("error %q, want %q", err, tc.want)
			}
			if got := f.dtcs(); !slices.Equal(got, old) {
				t.Errorf("dtcs %+v, want unchanged %+v", got, old)
			}
		})
	}
}

// unplugged is an adapter that reports its firmware when opened and dies
// with a fatal error on the first frame sent.
type unplugged struct{ bus *gocan.Bus }

var errUnplugged = errors.New("usb unplugged")

func (u *unplugged) Open(_ context.Context, bus *gocan.Bus) error {
	u.bus = bus
	bus.Emit(gocan.Event{Type: gocan.EventTypeInfo, Details: "firmware 1.2"})
	return nil
}

func (u *unplugged) Send(context.Context, gocan.Frame) error {
	u.bus.Fatal(errUnplugged)
	return errUnplugged
}

func (u *unplugged) Close() error { return nil }

// Adapter events, the final fatal one included, go to the log as they
// happen; the fatal failure aborts the read, which reports it and keeps the
// listed DTCs.
func TestAdapterEventsAreLogged(t *testing.T) {
	old := []dtc.DTC{{ECU: symbol.ECU_T5, Code: "Cat_error", Status: 1}}
	f := newFixture(t, "T5", t5Firmware(), &unplugged{}, nil)
	f.setDTCs(old)

	f.tapAndWait(f.read)

	logs, errs := f.rec.snapshot()
	for _, want := range []string{"[INFO] firmware 1.2", "[FATAL] usb unplugged"} {
		if !slices.Contains(logs, want) {
			t.Errorf("log lacks %q: %q", want, logs)
		}
	}
	if len(errs) != 1 || !errors.Is(errs[0], errUnplugged) {
		t.Errorf("errors %v, want one wrapping %v", errs, errUnplugged)
	}
	if got := f.dtcs(); !slices.Equal(got, old) {
		t.Errorf("dtcs %+v, want unchanged %+v", got, old)
	}
}

// urlApp records the URLs the reader asks the OS to open.
type urlApp struct {
	fyne.App
	err  error
	urls []*url.URL
}

func (a *urlApp) OpenURL(u *url.URL) error {
	a.urls = append(a.urls, u)
	return a.err
}

// newListFixture renders a reader in a window on the test goroutine (no
// background work involved) with a recording app and temp dir.
func newListFixture(t *testing.T) (*DTCReader, *urlApp, *recorder, fyne.Window) {
	t.Helper()
	app := &urlApp{App: test.NewTempApp(t)}
	fyne.SetCurrentApp(app)
	t.Setenv("TMPDIR", t.TempDir())
	rec := &recorder{}
	d := New(func() symbol.FirmwareFile { return nil }, func() string { return "T7" },
		func() (gocan.Adapter, error) { return nil, errors.New("unused") }, rec.log, rec.err)
	w := test.NewWindow(d)
	w.Resize(fyne.NewSize(600, 400))
	return d, app, rec, w
}

// row renders list row i into a fresh entry the way the list does.
func row(d *DTCReader, i int) *DTCEntry {
	e := d.list.CreateItem().(*DTCEntry)
	d.list.UpdateItem(i, e)
	return e
}

// Each row shows the code's title with its name, from the built-in DTC
// table or else the first WIS hit, and a description from the same
// sources (WIS blank lines collapsed). The info button is enabled only
// when WIS has a document for the code.
func TestListRows(t *testing.T) {
	d, _, _, _ := newListFixture(t)
	codes := []dtc.DTC{
		{ECU: symbol.ECU_T7, Code: "P1230", Status: 0x48},                    // table name and description
		{ECU: symbol.ECU_T7, Code: "P0444", Status: 0x48},                    // WIS title and symptom
		{ECU: symbol.ECU_T5, Code: "Adapt_error", Status: 2},                 // table description, no WIS for T5
		{ECU: symbol.ECU_T8, Code: "U9999 00", FailureType: 0},               // unknown everywhere
		{ECU: symbol.ECU_T8, Code: "B0449 00", FailureType: 0, Status: 0x08}, // single WIS hit
	}
	d.dtcs = codes
	d.Refresh()

	tests := []struct {
		title, desc string
		info        bool
	}{
		{codes[0].Title() + " - Throttle Position Sensor 1 and 2 Circuit. Sum Out of Range",
			"(Faulty signal from throttle disc position sensor)", true},
		{codes[1].Title() + " - EVAP Purge Valve Circuit. Open / Short to Ground.",
			"CHECK ENGINE.\nPossible smell of petrol or misfiring.", true},
		{"Adapt_error: 2", "Adaption error counter", false},
		{codes[3].Title(), "No description available.", false},
		{codes[4].Title() + " - Blower Fan. Out Of Range (fan blocked)",
			"Poor cabin heating\nHeater does not start.", true},
	}
	if n := d.list.Length(); n != len(tests) {
		t.Fatalf("list length %d, want %d", n, len(tests))
	}
	for i, tc := range tests {
		e := row(d, i)
		if e.title.Text != tc.title {
			t.Errorf("row %d title %q, want %q", i, e.title.Text, tc.title)
		}
		if e.description.Text != tc.desc {
			t.Errorf("row %d description %q, want %q", i, e.description.Text, tc.desc)
		}
		if e.info.Disabled() == tc.info {
			t.Errorf("row %d info enabled %v, want %v", i, !e.info.Disabled(), tc.info)
		}
	}
}

// A row reused for a code without WIS documents loses the previous code's
// info handler.
func TestRowReuseClearsInfo(t *testing.T) {
	d, app, _, _ := newListFixture(t)
	d.dtcs = []dtc.DTC{
		{ECU: symbol.ECU_T8, Code: "B0449 00"},
		{ECU: symbol.ECU_T8, Code: "U9999 00"},
	}
	e := row(d, 0)
	d.list.UpdateItem(1, e)
	if !e.info.Disabled() {
		t.Fatal("info button still enabled for a code without WIS documents")
	}
	test.Tap(e.info)
	if len(app.urls) != 0 {
		t.Errorf("disabled info button opened %v", app.urls)
	}
}

// renderedDoc returns the file a file:// URL points at and its content.
func renderedDoc(t *testing.T, u *url.URL) string {
	t.Helper()
	if u.Scheme != "file" {
		t.Fatalf("URL scheme %q, want file", u.Scheme)
	}
	if dir := filepath.Dir(u.Path); dir != os.Getenv("TMPDIR") {
		t.Errorf("document written to %s, want the temp dir %s", dir, os.Getenv("TMPDIR"))
	}
	b, err := os.ReadFile(u.Path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// With one WIS hit the info button renders that document to a temp HTML
// file and opens it in the browser.
func TestInfoOpensSingleDocument(t *testing.T) {
	d, app, rec, _ := newListFixture(t)
	d.dtcs = []dtc.DTC{{ECU: symbol.ECU_T8, Code: "B0449 00"}}
	e := row(d, 0)

	test.Tap(e.info)

	if len(app.urls) != 1 {
		t.Fatalf("opened %d URLs, want 1", len(app.urls))
	}
	want, err := wis.Render(wis.Search("B0449 00", "9440")[0].Doc)
	if err != nil {
		t.Fatal(err)
	}
	if got := renderedDoc(t, app.urls[0]); got != string(want) {
		t.Errorf("document content differs from wis.Render (%d vs %d bytes)", len(got), len(want))
	}
	logs, errs := rec.snapshot()
	if len(errs) != 0 {
		t.Errorf("errors: %v", errs)
	}
	if !slices.Contains(logs, "Opening WIS document for B0449 00 in browser") {
		t.Errorf("logs %q", logs)
	}
}

// menuLabel is the visible text of a popup menu item.
func menuLabel(item fyne.CanvasObject) string {
	var parts []string
	for _, o := range test.LaidOutObjects(item) {
		if txt, ok := o.(*canvas.Text); ok && txt.Text != "" {
			parts = append(parts, txt.Text)
		}
	}
	return strings.Join(parts, "")
}

// With several WIS hits the info button pops up a menu of model, years,
// system and code; picking an entry opens that entry's document.
func TestInfoMenuForSeveralDocuments(t *testing.T) {
	d, app, rec, w := newListFixture(t)
	d.dtcs = []dtc.DTC{{ECU: symbol.ECU_T7, Code: "P0444", Status: 0x48}}
	e := row(d, 0)
	results := wis.Search("P0444", "9400", "9600")
	if len(results) < 2 {
		t.Fatalf("WIS archive has %d hits for P0444, the test needs several", len(results))
	}

	test.Tap(e.info)

	menu, ok := w.Canvas().Focused().(*widget.PopUpMenu)
	if !ok {
		t.Fatalf("focused object %T, want the WIS popup menu", w.Canvas().Focused())
	}
	if len(menu.Items) != len(results) {
		t.Fatalf("%d menu items, want one per WIS hit (%d)", len(menu.Items), len(results))
	}
	for i, r := range results {
		want := wis.ModelName(r.Model) + " " + r.Year + " · " + r.System + " · " + r.Code
		if got := menuLabel(menu.Items[i]); got != want {
			t.Errorf("item %d %q, want %q", i, got, want)
		}
	}
	if !strings.HasPrefix(menuLabel(menu.Items[0]), "9-3 (9400) ") {
		t.Errorf("first item %q, want the 9-3 (9400) documents first", menuLabel(menu.Items[0]))
	}
	if len(app.urls) != 0 {
		t.Fatalf("opened %v before an entry was picked", app.urls)
	}

	pick := len(results) - 1
	test.Tap(menu.Items[pick].(fyne.Tappable))

	if len(app.urls) != 1 {
		t.Fatalf("opened %d URLs, want 1", len(app.urls))
	}
	want, err := wis.Render(results[pick].Doc)
	if err != nil {
		t.Fatal(err)
	}
	if got := renderedDoc(t, app.urls[0]); got != string(want) {
		t.Errorf("opened a different document than entry %d's %s", pick, results[pick].Doc)
	}
	if _, errs := rec.snapshot(); len(errs) != 0 {
		t.Errorf("errors: %v", errs)
	}
}

// Failures rendering, saving or opening the document are reported instead
// of opening anything.
func TestOpenWISDocFailures(t *testing.T) {
	good := wis.Search("B0449 00", "9440")[0]

	t.Run("unknown document", func(t *testing.T) {
		d, app, rec, _ := newListFixture(t)
		d.openWISDoc(wis.Result{Entry: wis.Entry{Code: "X", Doc: "9440/nope.htm"}})
		if err := rec.onlyErr(t); !strings.HasPrefix(err.Error(), "wis: ") {
			t.Errorf("error %q, want a wis error", err)
		}
		if len(app.urls) != 0 {
			t.Errorf("opened %v", app.urls)
		}
	})

	t.Run("temp dir missing", func(t *testing.T) {
		d, app, rec, _ := newListFixture(t)
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
		d.openWISDoc(good)
		if err := rec.onlyErr(t); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("error %v, want not-exist", err)
		}
		if len(app.urls) != 0 {
			t.Errorf("opened %v", app.urls)
		}
	})

	t.Run("browser fails", func(t *testing.T) {
		d, app, rec, _ := newListFixture(t)
		app.err = errors.New("no browser")
		d.openWISDoc(good)
		if err := rec.onlyErr(t); err.Error() != "no browser" {
			t.Errorf("error %v, want the OpenURL error", err)
		}
	})
}

// The entry's info button starts disabled; a handler enables it and runs
// on tap, and a nil handler disables it again.
func TestDTCEntryInfoButton(t *testing.T) {
	test.NewTempApp(t)
	e := NewDTCEntry()
	if !e.info.Disabled() {
		t.Fatal("info button enabled without a handler")
	}
	if e.description.Text != "No description available." {
		t.Errorf("default description %q", e.description.Text)
	}
	taps := 0
	e.SetOnTapped(func() { taps++ })
	if e.info.Disabled() {
		t.Fatal("info button disabled with a handler")
	}
	test.Tap(e.info)
	if taps != 1 {
		t.Errorf("handler ran %d times, want 1", taps)
	}
	e.SetOnTapped(nil)
	if !e.info.Disabled() {
		t.Error("info button enabled after clearing the handler")
	}
	e.SetTitle("P0100")
	e.SetDescription("desc")
	if e.title.Text != "P0100" || e.description.Text != "desc" {
		t.Errorf("title %q description %q", e.title.Text, e.description.Text)
	}
}
