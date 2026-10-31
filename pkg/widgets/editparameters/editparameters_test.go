package editparameters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/ecu/t8"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

// fakeT8 is a Trionic 8 behind a virtual adapter answering the GMLAN
// services the parameter editor uses on 0x7E0/0x7E8: readDataByIdentifier
// (single and multi-frame answers), readDataByPacketIdentifier (UUDT answer
// on 0x5E8), securityAccess FD, writeDataByIdentifier (single and multi-frame
// requests), deviceControl and returnToNormalMode. Writes and device control
// need security access first, like the real ECU.
type fakeT8 struct {
	dids map[byte][]byte // readDataByIdentifier answers by DID
	e85  []byte          // $AA 01 7A answer frame on 0x5E8; nil = not supported
	nrc  map[uint16]byte // service<<8|first parameter -> forced negative response code

	mu       sync.Mutex
	bus      *gocan.Bus
	writes   map[byte][]byte // writeDataByIdentifier data by DID
	devCtl   [][]byte        // deviceControl requests, CPID first
	trace    []string        // "SSPP" per request handled: service and first parameter
	unlocked bool
	req      []byte
	reqLen   int
	pending  []gocan.Frame
}

var seed = []byte{0x12, 0x34}

func (s *fakeT8) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	s.writes = map[byte][]byte{}
	return nil
}

func (s *fakeT8) Close() error { return nil }

func (s *fakeT8) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.ID != 0x7E0 {
		return nil
	}
	d := f.Bytes()
	switch d[0] >> 4 {
	case 0: // single frame
		s.handle(d[1 : 1+int(d[0])])
	case 1: // first frame: let the rest come without waits
		s.reqLen = int(d[0]&0x0F)<<8 | int(d[1])
		s.req = append(s.req[:0], d[2:]...)
		s.bus.Deliver(gocan.NewFrame(0x7E8, []byte{0x30, 0x00, 0x00, 0, 0, 0, 0, 0}))
	case 2: // consecutive frame
		s.req = append(s.req, d[1:]...)
		if len(s.req) >= s.reqLen {
			s.handle(s.req[:s.reqLen])
		}
	case 3: // flow control: release the rest of our answer
		for _, p := range s.pending {
			s.bus.Deliver(p)
		}
		s.pending = nil
	}
	return nil
}

func (s *fakeT8) handle(msg []byte) {
	sid, sub := msg[0], byte(0)
	if len(msg) > 1 {
		sub = msg[1]
	}
	s.trace = append(s.trace, fmt.Sprintf("%02X%02X", sid, sub))
	if code, ok := s.nrc[uint16(sid)<<8|uint16(sub)]; ok {
		s.reply(0x7F, sid, code)
		return
	}
	switch sid {
	case gmlan.READ_DATA_BY_IDENTIFIER:
		data, ok := s.dids[sub]
		if !ok {
			s.reply(0x7F, sid, 0x31)
			return
		}
		s.reply(append([]byte{0x5A, sub}, data...)...)
	case gmlan.READ_DATA_BY_PACKET_IDENTIFIER:
		if s.e85 == nil {
			s.reply(0x7F, sid, 0x31)
			return
		}
		s.bus.Deliver(gocan.NewFrame(0x5E8, s.e85))
	case gmlan.RETURN_TO_NORMAL_MODE:
		s.reply(0x60)
	case gmlan.SECURITY_ACCESS:
		if sub == 0xFD {
			s.reply(0x67, sub, seed[0], seed[1])
			return
		}
		hi, lo := t8sec.CalculateAccessKey(seed, 0xFD)
		if msg[2] != hi || msg[3] != lo {
			s.reply(0x7F, sid, 0x35)
			return
		}
		s.unlocked = true
		s.reply(0x67, sub)
	case gmlan.WRITE_DATA_BY_IDENTIFIER:
		if !s.unlocked {
			s.reply(0x7F, sid, 0x33)
			return
		}
		s.writes[sub] = slices.Clone(msg[2:])
		s.reply(0x7B, sub)
	case gmlan.DEVICE_CONTROL:
		if !s.unlocked {
			s.reply(0x7F, sid, 0x33)
			return
		}
		s.devCtl = append(s.devCtl, slices.Clone(msg[1:]))
		s.reply(0xEE, sub)
	default:
		s.reply(0x7F, sid, 0x11)
	}
}

// reply sends an answer on 0x7E8 as ISO-TP: one frame when it fits, else a
// first frame now and consecutive frames after the tester's flow control.
func (s *fakeT8) reply(p ...byte) {
	pad := func(b []byte) []byte { return append(b, make([]byte, 8-len(b))...) }
	if len(p) <= 7 {
		s.bus.Deliver(gocan.NewFrame(0x7E8, pad(append([]byte{byte(len(p))}, p...))))
		return
	}
	s.pending = nil
	seq := byte(0x21)
	for pos := 6; pos < len(p); pos += 7 {
		s.pending = append(s.pending, gocan.NewFrame(0x7E8, pad(append([]byte{seq}, p[pos:min(pos+7, len(p))]...))))
		seq = 0x20 | (seq+1)&0x0F
	}
	s.bus.Deliver(gocan.NewFrame(0x7E8, append([]byte{0x10 | byte(len(p)>>8), byte(len(p))}, p[:6]...)))
}

func (s *fakeT8) state() (map[byte][]byte, [][]byte, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes, s.devCtl, s.trace
}

const testVIN = "YS3FB45S131012345"

// newECU is a T8 with BioPower, convertible, US tank, OBD2, clutch start,
// SAI and high output set in PI 01, plus PI 01 bits and a byte the editor
// has no field for (0x82 / 0x81 / 0xAA), 52.05078125% oil quality, a
// 270 km/h top speed and 82% ethanol.
func newECU() *fakeT8 {
	return &fakeT8{
		dids: map[byte][]byte{
			0x01: {0xAF, 0xB5, 0xAA},
			0x25: {0x00, 0x00, 0x34, 0x0D},
			0x90: []byte(testVIN),
			0x02: {0x0A, 0x8C},
		},
		e85: []byte{0x7A, 0x00, 0x52, 0x00, 0x00, 0x00, 0x00, 0x00},
	}
}

// recorder collects what the editor reports.
type recorder struct {
	logs []string
	errs []error
}

func (r *recorder) log(s string) { r.logs = append(r.logs, s) }
func (r *recorder) err(e error)  { r.errs = append(r.errs, e) }

func newEditor(t *testing.T, ecu gocan.Adapter, adapterErr error) (*EditParameters, *recorder, *int) {
	t.Helper()
	test.NewTempApp(t)
	rec := &recorder{}
	opens := 0
	ep := NewEditParameters(func() (gocan.Adapter, error) {
		opens++
		return ecu, adapterErr
	}, rec.err, rec.log)
	return ep, rec, &opens
}

// findButton finds a button by its text through the widgets' existing
// renderers.
func findButton(o fyne.CanvasObject, text string) *widget.Button {
	switch v := o.(type) {
	case *widget.Button:
		if v.Text == text {
			return v
		}
	case *fyne.Container:
		for _, c := range v.Objects {
			if b := findButton(c, text); b != nil {
				return b
			}
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			if b := findButton(c, text); b != nil {
				return b
			}
		}
	}
	return nil
}

func form(ep *EditParameters) *widget.Form {
	return test.WidgetRenderer(ep).Objects()[0].(*widget.Form)
}

// item returns the form widget with the given label.
func item(t *testing.T, ep *EditParameters, label string) fyne.CanvasObject {
	t.Helper()
	for _, it := range form(ep).Items {
		if it.Text == label {
			return it.Widget
		}
	}
	t.Fatalf("no form item %q", label)
	return nil
}

type fields struct {
	vin, e85, topSpeed, oil, diag, tank string
	convertible, sai, highOutput, bio   bool
	clutch                              bool
}

func current(t *testing.T, ep *EditParameters) fields {
	t.Helper()
	var f fields
	var err error
	get := func(s string, e error) string {
		if e != nil {
			err = e
		}
		return s
	}
	getb := func(b bool, e error) bool {
		if e != nil {
			err = e
		}
		return b
	}
	f.vin = get(ep.GetVIN())
	f.e85 = get(ep.GetE85Percent())
	f.topSpeed = get(ep.GetTopSpeed())
	f.oil = get(ep.GetOilQuality())
	f.diag = get(ep.GetDiagnosticType())
	f.tank = get(ep.GetTankType())
	f.convertible = getb(ep.GetConvertible())
	f.sai = getb(ep.GetSAI())
	f.highOutput = getb(ep.GetHighOutput())
	f.bio = getb(ep.GetBioPower())
	f.clutch = getb(ep.clutchStart.Get())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// Reading decodes PI 01 into the selects and checks and fills the oil
// quality (/256, two decimals), VIN, top speed (/10) and E85 entries, all
// shown in the form; afterwards the ECU is returned to normal mode.
func TestReadFieldsFromECU(t *testing.T) {
	ecu := newECU()
	ep, rec, _ := newEditor(t, ecu, nil)
	w := test.NewWindow(ep)
	w.Resize(fyne.NewSize(500, 600))

	test.Tap(findButton(form(ep), "Read fields from ECU"))

	if len(rec.errs) != 0 {
		t.Fatalf("errors: %v", rec.errs)
	}
	want := fields{
		vin: testVIN, e85: "82", topSpeed: "270", oil: "52.05", diag: "OBD2", tank: "US",
		convertible: true, sai: true, highOutput: true, bio: true, clutch: true,
	}
	if got := current(t, ep); got != want {
		t.Errorf("fields\n got %+v\nwant %+v", got, want)
	}
	if !slices.Equal(rec.logs, []string{"Reading parameters from ECU..."}) {
		t.Errorf("logs %q", rec.logs)
	}
	if !ep.hasBeenRead {
		t.Error("hasBeenRead not set after a successful read")
	}
	if got := item(t, ep, "VIN").(*widget.Entry).Text; got != testVIN {
		t.Errorf("VIN entry %q", got)
	}
	if got := item(t, ep, "Diagnostic Type").(*widget.Select).Selected; got != "OBD2" {
		t.Errorf("diagnostic select %q", got)
	}
	if !item(t, ep, "Clutch Start").(*widget.Check).Checked {
		t.Error("clutch start check not ticked")
	}
	_, _, trace := ecu.state()
	if wantTrace := []string{"1A01", "1A25", "1A90", "1A02", "AA01", "2000"}; !slices.Equal(trace, wantTrace) {
		t.Errorf("requests %v, want %v", trace, wantTrace)
	}
}

// Writing after a read unlocks the ECU, writes VIN, E85 (device control
// 0x18), top speed (x10), oil quality (x256) and PI 01 re-read from the ECU
// with the editor's bits applied and every other bit kept, then persists
// with device control 0x16. Each case's written PI 01 decodes back to the
// fields that were set.
func TestWriteFieldsToECU(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pi01   []byte
		diag   string
		tank   string
		on     bool
		wantPI []byte
	}{
		{"all off LOBD AWD keeps foreign bits", []byte{0xAF, 0xB5, 0xAA}, "LOBD", "AWD", false, []byte{0xFA, 0xCB, 0xAA}},
		{"all on EOBD EU", []byte{0x00, 0x4A}, "EOBD", "EU", true, []byte{0x55, 0x34}},
		{"all off None US", []byte{0x60, 0x2A}, "None", "US", false, []byte{0x08, 0x4A}},
		{"all on OBD2 US", []byte{0x18, 0x4A}, "OBD2", "US", true, []byte{0x2D, 0x34}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ecu := newECU()
			ecu.dids[0x01] = tc.pi01
			ep, rec, _ := newEditor(t, ecu, nil)
			w := test.NewWindow(ep)
			w.Resize(fyne.NewSize(500, 600))
			test.Tap(findButton(form(ep), "Read fields from ECU"))

			ep.SetVIN("YS3FB45S131099999")
			ep.SetE85Percent("10")
			ep.SetTopSpeed("240")
			ep.SetOilQuality("100")
			ep.SetDiagnosticType(tc.diag)
			ep.SetTankType(tc.tank)
			ep.SetConvertible(tc.on)
			ep.SetSAI(tc.on)
			ep.SetHighOutput(tc.on)
			ep.SetBioPower(tc.on)
			ep.SetClutchStart(tc.on)

			submit := findButton(form(ep), "Write fields to ECU")
			if submit.Disabled() {
				t.Fatal("write button disabled with valid fields")
			}
			test.Tap(submit)

			if len(rec.errs) != 0 {
				t.Fatalf("errors: %v", rec.errs)
			}
			writes, devCtl, trace := ecu.state()
			if got := string(writes[0x90]); got != "YS3FB45S131099999" {
				t.Errorf("VIN written %q", got)
			}
			if got := writes[0x02]; !slices.Equal(got, []byte{0x09, 0x60}) {
				t.Errorf("top speed written % X, want 09 60 (2400)", got)
			}
			if got := writes[0x25]; !slices.Equal(got, []byte{0x00, 0x00, 0x64, 0x00}) {
				t.Errorf("oil quality written % X, want 00 00 64 00 (100*256)", got)
			}
			if got := writes[0x01]; !slices.Equal(got, tc.wantPI) {
				t.Errorf("PI 01 written % X, want % X", got, tc.wantPI)
			}
			if want := [][]byte{{0x18, 0x00, 0x0A}, {0x16}}; !slices.EqualFunc(devCtl, want, slices.Equal[[]byte]) {
				t.Errorf("device control % X, want % X", devCtl, want)
			}
			wantTrace := []string{"27FD", "27FE", "3B90", "AE18", "3B02", "3B25", "1A01", "3B01", "AE16", "2000"}
			if got := trace[6:]; !slices.Equal(got, wantTrace) { // after the read
				t.Errorf("write requests %v, want %v", got, wantTrace)
			}

			dec, err := t8.DecodePI01(writes[0x01])
			if err != nil {
				t.Fatal(err)
			}
			want := t8.PI01Data{
				Convertible: tc.on, SAI: tc.on, HighOutput: tc.on, BioPower: tc.on, ClutchStart: tc.on,
				DiagnosticType: t8.DiagnosticTypeFromString(tc.diag), TankType: t8.TankTypeFromString(tc.tank),
			}
			if dec != want {
				t.Errorf("written PI 01 decodes to %v, want %v", dec, want)
			}
		})
	}
}

// Writing before a successful read is refused without touching the adapter.
func TestWriteRequiresRead(t *testing.T) {
	ep, rec, opens := newEditor(t, newECU(), nil)
	ep.writeParameters()
	if len(rec.errs) != 1 || rec.errs[0].Error() != "You must read the parameters from the ECU before writing" {
		t.Fatalf("errors %v", rec.errs)
	}
	if *opens != 0 {
		t.Errorf("adapter requested %d times", *opens)
	}
}

// A read failing at any step reports the error, keeps the fields read so
// far and leaves writing locked.
func TestReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*fakeT8)
		want    string
		wantVIN string // VIN is read third; set only if the failure comes after it
	}{
		{"PI 01 rejected", func(e *fakeT8) { e.nrc = map[uint16]byte{0x1A01: 0x31} }, "ReadDataByIdentifier", ""},
		{"PI 01 empty", func(e *fakeT8) { e.dids[0x01] = []byte{0x00, 0x00} }, "data invalid", ""},
		{"oil quality rejected", func(e *fakeT8) { e.nrc = map[uint16]byte{0x1A25: 0x31} }, "ReadDataByIdentifier", ""},
		{"VIN rejected", func(e *fakeT8) { e.nrc = map[uint16]byte{0x1A90: 0x31} }, "ReadDataByIdentifier", ""},
		{"top speed rejected", func(e *fakeT8) { e.nrc = map[uint16]byte{0x1A02: 0x31} }, "GetTopSpeed[1]", testVIN},
		{"E85 not supported", func(e *fakeT8) { e.e85 = nil }, "ReadDataByPacketIdentifier", testVIN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ecu := newECU()
			tc.mutate(ecu)
			ep, rec, _ := newEditor(t, ecu, nil)
			ep.readParameters()
			if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), tc.want) {
				t.Fatalf("errors %v, want one containing %q", rec.errs, tc.want)
			}
			if vin, _ := ep.GetVIN(); vin != tc.wantVIN {
				t.Errorf("VIN %q, want %q", vin, tc.wantVIN)
			}
			if ep.hasBeenRead {
				t.Error("hasBeenRead set after a failed read")
			}
			if _, _, trace := ecu.state(); trace[len(trace)-1] != "2000" {
				t.Errorf("ECU not returned to normal mode: %v", trace)
			}
		})
	}
}

// failingOpen is an adapter whose port cannot be opened.
type failingOpen struct{}

func (failingOpen) Open(context.Context, *gocan.Bus) error  { return errors.New("port busy") }
func (failingOpen) Send(context.Context, gocan.Frame) error { return nil }
func (failingOpen) Close() error                            { return nil }

// Adapter problems are reported for both directions.
func TestAdapterFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		dev  gocan.Adapter
		err  error
		want string
	}{
		{"no adapter", nil, errors.New("no adapter selected"), "no adapter selected"},
		{"open fails", failingOpen{}, nil, "port busy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ep, rec, _ := newEditor(t, tc.dev, tc.err)
			ep.readParameters()
			ep.hasBeenRead = true
			ep.writeParameters()
			if len(rec.errs) != 2 || rec.errs[0].Error() != tc.want || rec.errs[1].Error() != tc.want {
				t.Errorf("errors %v, want %q from read and write", rec.errs, tc.want)
			}
		})
	}
}

// A write failing at any step reports the error with the step named and
// stops there, except for the VIN: a VIN the ECU rejects or one of the wrong
// length is reported and the remaining writes still go out. Every case ends
// with returnToNormalMode.
func TestWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*fakeT8, *EditParameters)
		want     string
		lastSent string // last request of the write before normal mode
	}{
		{"security access denied", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0x27FD: 0x22} }, "SecurityAccessRequestSeed", "27FD"},
		{"VIN rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0x3B90: 0x31} }, "Error setting VIN", "AE16"},
		{"VIN too short", func(_ *fakeT8, ep *EditParameters) { ep.SetVIN("YS3") }, "Error setting VIN: invalid vin length", "AE16"},
		{"E85 not a number", func(_ *fakeT8, ep *EditParameters) { ep.SetE85Percent("lots") }, "Error parsing E85 content", "3B90"},
		{"E85 rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0xAE18: 0x31} }, "Error setting E85 percent", "AE18"},
		{"top speed not a number", func(_ *fakeT8, ep *EditParameters) { ep.SetTopSpeed("fast") }, "Error parsing Top Speed", "AE18"},
		{"top speed rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0x3B02: 0x31} }, "Error setting Top Speed", "3B02"},
		{"oil quality not a number", func(_ *fakeT8, ep *EditParameters) { ep.SetOilQuality("good") }, "Error parsing oil quality", "3B02"},
		{"oil quality rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0x3B25: 0x31} }, "Error setting oil quality", "3B25"},
		{"PI 01 read rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0x1A01: 0x31} }, "Error reading PI01", "1A01"},
		{"PI 01 write rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0x3B01: 0x31} }, "Error writing PI 0x01", "3B01"},
		{"persist rejected", func(e *fakeT8, _ *EditParameters) { e.nrc = map[uint16]byte{0xAE16: 0x31} }, "Error performing device control 0x16", "AE16"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ecu := newECU()
			ep, rec, _ := newEditor(t, ecu, nil)
			ep.readParameters()
			if len(rec.errs) != 0 {
				t.Fatalf("read failed: %v", rec.errs)
			}
			_, _, readTrace := ecu.state()
			nRead := len(readTrace)
			tc.mutate(ecu, ep)

			ep.writeParameters()

			if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), tc.want) {
				t.Fatalf("errors %v, want one containing %q", rec.errs, tc.want)
			}
			_, _, trace := ecu.state()
			trace = trace[nRead:]
			if len(trace) < 2 || trace[len(trace)-1] != "2000" || trace[len(trace)-2] != tc.lastSent {
				t.Errorf("write requests %v, want ending %s then normal mode", trace, tc.lastSent)
			}
		})
	}
}

// The entry validators accept exactly 17 VIN characters, 0-85 % E85,
// 0-5000 km/h top speed and 0-100 % oil quality (decimals allowed for oil
// only).
func TestValidators(t *testing.T) {
	ep, _, _ := newEditor(t, nil, nil)
	test.NewWindow(ep)
	for _, tc := range []struct {
		label string
		input string
		ok    bool
	}{
		{"VIN", testVIN, true},
		{"VIN", testVIN[:16], false},
		{"VIN", testVIN + "X", false},
		{"E85%", "0", true},
		{"E85%", "85", true},
		{"E85%", "86", false},
		{"E85%", "-1", false},
		{"E85%", "8.5", false},
		{"E85%", "", false},
		{"Top Speed (km/h)", "0", true},
		{"Top Speed (km/h)", "5000", true},
		{"Top Speed (km/h)", "5001", false},
		{"Top Speed (km/h)", "-5", false},
		{"Top Speed (km/h)", "fast", false},
		{"Oil Quaility %", "52.05", true},
		{"Oil Quaility %", "100", true},
		{"Oil Quaility %", "100.01", false},
		{"Oil Quaility %", "-0.5", false},
		{"Oil Quaility %", "x", false},
	} {
		err := item(t, ep, tc.label).(*widget.Entry).Validator(tc.input)
		if (err == nil) != tc.ok {
			t.Errorf("%s %q: error %v, want ok=%v", tc.label, tc.input, err, tc.ok)
		}
	}
}

// The form widgets and the getters/setters share the same bindings: typing,
// selecting and ticking shows up in the getters, and setters show up in the
// widgets.
func TestFormBindings(t *testing.T) {
	ep, _, _ := newEditor(t, nil, nil)
	w := test.NewWindow(ep)
	w.Resize(fyne.NewSize(500, 600))

	test.Type(item(t, ep, "VIN").(*widget.Entry), testVIN)
	test.Type(item(t, ep, "E85%").(*widget.Entry), "25")
	item(t, ep, "Diagnostic Type").(*widget.Select).SetSelected("EOBD")
	item(t, ep, "Tank Type").(*widget.Select).SetSelected("AWD")
	for _, label := range []string{"Convertible", "SAI", "High Output", "BioPower", "Clutch Start"} {
		test.Tap(item(t, ep, label).(*widget.Check))
	}
	got := current(t, ep)
	want := fields{vin: testVIN, e85: "25", diag: "EOBD", tank: "AWD",
		convertible: true, sai: true, highOutput: true, bio: true, clutch: true}
	if got != want {
		t.Errorf("after editing the form\n got %+v\nwant %+v", got, want)
	}

	ep.SetTopSpeed("230")
	ep.SetOilQuality("75.5")
	ep.SetTankType("EU")
	ep.SetSAI(false)
	if s := item(t, ep, "Top Speed (km/h)").(*widget.Entry).Text; s != "230" {
		t.Errorf("top speed entry %q", s)
	}
	if s := item(t, ep, "Oil Quaility %").(*widget.Entry).Text; s != "75.5" {
		t.Errorf("oil entry %q", s)
	}
	if s := item(t, ep, "Tank Type").(*widget.Select).Selected; s != "EU" {
		t.Errorf("tank select %q", s)
	}
	if item(t, ep, "SAI").(*widget.Check).Checked {
		t.Error("SAI still ticked after SetSAI(false)")
	}

	pi, err := ep.GetPI01Data()
	if err != nil {
		t.Fatal(err)
	}
	wantPI := t8.PI01Data{Convertible: true, SAI: false, HighOutput: true, BioPower: true, ClutchStart: true,
		DiagnosticType: t8.DiagnosticTypeEOBD, TankType: t8.TankTypeEU}
	if pi != wantPI {
		t.Errorf("GetPI01Data %v, want %v", pi, wantPI)
	}
}

// setBit sets or clears one bit and leaves the others alone.
func TestSetBit(t *testing.T) {
	for _, tc := range []struct {
		in    byte
		pos   uint
		state bool
		want  byte
	}{
		{0x00, 0, true, 0x01},
		{0x01, 0, true, 0x01},
		{0xFF, 7, false, 0x7F},
		{0x7F, 7, false, 0x7F},
		{0xA5, 1, true, 0xA7},
		{0xA5, 2, false, 0xA1},
	} {
		if got := setBit(tc.in, tc.pos, tc.state); got != tc.want {
			t.Errorf("setBit(%02X, %d, %v) = %02X, want %02X", tc.in, tc.pos, tc.state, got, tc.want)
		}
	}
}

// openT8 opens a bus on the fake ECU (already unlocked) and wraps it in the
// editor's T8 client.
func openT8(t *testing.T, ecu *fakeT8) *T8 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cl, err := gocan.OpenAdapter(ctx, ecu)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	ecu.unlocked = true
	return &T8{gm: gmlan.New(cl, 0x7e0, 0x5e8, 0x7e8)}
}

// The RPM limiter is DID 0x29, a big-endian rpm value.
func TestRPMLimiter(t *testing.T) {
	ecu := newECU()
	ecu.dids[0x29] = []byte{0x1B, 0x58}
	c := openT8(t, ecu)
	ctx := context.Background()

	got, err := c.GetRPMLimiter(ctx)
	if err != nil || got != 7000 {
		t.Fatalf("GetRPMLimiter = %d, %v; want 7000", got, err)
	}
	if err := c.SetRPMLimit(ctx, 6500); err != nil {
		t.Fatal(err)
	}
	if w, _, _ := ecu.state(); !slices.Equal(w[0x29], []byte{0x19, 0x64}) {
		t.Errorf("RPM limit written % X, want 19 64", w[0x29])
	}
}

// A VIN of the wrong length is refused before anything is sent.
func TestSetVehicleVINLength(t *testing.T) {
	ecu := newECU()
	c := openT8(t, ecu)
	for _, vin := range []string{"", testVIN[:16], testVIN + "1"} {
		if err := c.SetVehicleVIN(context.Background(), vin); err == nil || err.Error() != "invalid vin length" {
			t.Errorf("SetVehicleVIN(%q) error %v", vin, err)
		}
	}
	if _, _, trace := ecu.state(); len(trace) != 0 {
		t.Errorf("requests sent: %v", trace)
	}
}

// The E85 packet (DPID 0x7A) carries the ethanol percentage in its third
// byte; the bytes after it are other data. TrionicCANLib reads byte 2 and
// documents the reply 7A 00 52 04 00 16 DC 00 as 82 %.
func TestGetE85PercentReadsOneByte(t *testing.T) {
	ecu := newECU()
	ecu.e85 = []byte{0x7A, 0x00, 0x52, 0x04, 0x00, 0x16, 0xDC, 0x00}
	c := openT8(t, ecu)
	got, err := c.GetE85Percent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != 82 {
		t.Errorf("GetE85Percent = %v, want 82", got)
	}
}

// An answer too short to hold the value is reported as an error, not a
// crash or a value read from beyond the frame.
func TestShortAnswersAreErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*T8) error
		set  func(*fakeT8)
	}{
		{"oil quality", func(c *T8) error { _, err := c.GetOilQuality(context.Background()); return err },
			func(e *fakeT8) { e.dids[0x25] = []byte{0x34, 0x0D} }},
		{"E85", func(c *T8) error { _, err := c.GetE85Percent(context.Background()); return err },
			func(e *fakeT8) { e.e85 = []byte{0x7A, 0x00} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ecu := newECU()
			tc.set(ecu)
			c := openT8(t, ecu)
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked: %v", r)
					}
				}()
				err = tc.call(c)
			}()
			if err == nil {
				t.Error("no error for a short answer")
			}
		})
	}
}
