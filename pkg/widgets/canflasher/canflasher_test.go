package canflasher

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t7"
	"github.com/roffe/txlogger/pkg/model"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

// parseHex16 takes the 16-bit hex a user types for the T7 seed/key pair,
// with or without a 0x prefix and surrounding blanks, and rejects anything
// that is not exactly that.
func TestParseHex16(t *testing.T) {
	tests := []struct {
		in      string
		want    uint16
		wantErr bool
	}{
		{"1234", 0x1234, false},
		{"0xBEEF", 0xBEEF, false},
		{"  0x00ff\t", 0x00FF, false},
		{"ffff", 0xFFFF, false},
		{"0", 0, false},
		{"10000", 0, true}, // 17 bits
		{"", 0, true},
		{"0x", 0, true},
		{"xyz", 0, true},
		{"-1", 0, true},
	}
	for _, tt := range tests {
		got, err := parseHex16(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseHex16(%q) err = %v, want error %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseHex16(%q) = %#04x, want %#04x", tt.in, got, tt.want)
		}
	}
}

// addSuffix appends the extension only when the name lacks it.
func TestAddSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"dump":     "dump.bin",
		"dump.bin": "dump.bin",
		"a.b":      "a.b.bin",
		"":         ".bin",
	} {
		if got := addSuffix(in, ".bin"); got != want {
			t.Errorf("addSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

// The seed/key boxes give the clients a custom SecurityAccess pair only when
// both hold valid hex; anything else means "use the known-good pairs".
func TestSeedKey(t *testing.T) {
	r := newRig(t)
	tests := []struct {
		xor, sub string
		want     *ecu.SeedKey
	}{
		{"0x8142", "0x2356", &ecu.SeedKey{XOR: 0x8142, Sub: 0x2356}},
		{"4081", "1F6F", &ecu.SeedKey{XOR: 0x4081, Sub: 0x1F6F}},
		{"", "", nil},
		{"0x8142", "", nil},
		{"", "0x2356", nil},
		{"0x8142", "nope", nil},
	}
	for _, tt := range tests {
		var got *ecu.SeedKey
		r.onUI(func() {
			r.w.seedKeyXOR.SetText(tt.xor)
			r.w.seedKeySub.SetText(tt.sub)
			got = r.w.ecuConfig().SeedKey
		})
		switch {
		case tt.want == nil && got != nil:
			t.Errorf("xor %q sub %q: SeedKey = %+v, want nil", tt.xor, tt.sub, *got)
		case tt.want != nil && (got == nil || *got != *tt.want):
			t.Errorf("xor %q sub %q: SeedKey = %v, want %+v", tt.xor, tt.sub, got, *tt.want)
		}
	}
}

// The seed/key boxes accept blank or 16-bit hex, persist what is typed and
// come back filled in on the next flasher.
func TestSeedKeyEntries(t *testing.T) {
	r := newRig(t)
	prefs := fyne.CurrentApp().Preferences()
	r.onUI(func() {
		for in, valid := range map[string]bool{"": true, "  ": true, "0x1F2E": true, "abcd": true, "12345": false, "g": false} {
			if err := r.w.seedKeyXOR.Validator(in); (err == nil) != valid {
				t.Errorf("XOR validator(%q) = %v, want valid %v", in, err, valid)
			}
		}
		r.w.seedKeyXOR.SetText("0x1234")
		r.w.seedKeySub.SetText("0xABCD")
	})
	if got := prefs.String("canflasher_t7_xor"); got != "0x1234" {
		t.Errorf("stored XOR = %q", got)
	}
	if got := prefs.String("canflasher_t7_sub"); got != "0xABCD" {
		t.Errorf("stored SUB = %q", got)
	}

	var xor, sub string
	r.onUI(func() {
		w := New(&Config{CSW: settings.New(&settings.Config{})})
		test.WidgetRenderer(w)
		xor, sub = w.seedKeyXOR.Text, w.seedKeySub.Text
	})
	if xor != "0x1234" || sub != "0xABCD" {
		t.Errorf("new flasher seed/key = %q/%q, want the stored 0x1234/0xABCD", xor, sub)
	}
}

// The PIN box only accepts four characters.
func TestPinValidator(t *testing.T) {
	r := newRig(t)
	r.onUI(func() {
		for in, valid := range map[string]bool{"1234": true, "abcd": true, "123": false, "12345": false, "": false} {
			if err := r.w.pinEntry.Validator(in); (err == nil) != valid {
				t.Errorf("PIN validator(%q) = %v, want valid %v", in, err, valid)
			}
		}
	})
}

// Picking an ECU shows only the controls that apply to it — marriage,
// recovery and partition unlocks for T8, the seed/key pair and EOL for T7 —
// and remembers the choice for the next flasher, which starts on Trionic 5
// the first time.
func TestECUSelectControls(t *testing.T) {
	r := newRig(t)
	var first string
	r.onUI(func() { first = r.w.ecuSelect.Selected })
	if first != "Trionic 5" {
		t.Errorf("first flasher starts on %q, want Trionic 5", first)
	}

	t8 := func(w *CanFlasherWidget) []fyne.CanvasObject {
		return []fyne.CanvasObject{w.marryBTN, w.recoveryBTN, w.bootBOX, w.nvdmBOX, w.pinEntry, w.pinLabel, w.flashLabel}
	}
	t7 := func(w *CanFlasherWidget) []fyne.CanvasObject {
		return []fyne.CanvasObject{w.seedKeyLabel, w.seedKeyBox, w.eolBTN}
	}
	allVisible := func(objs []fyne.CanvasObject, want bool) bool {
		for _, o := range objs {
			if o.Visible() != want {
				return false
			}
		}
		return true
	}
	for _, tt := range []struct {
		ecu    string
		t8, t7 bool
	}{
		{"Trionic 8", true, false},
		{"Trionic 7", false, true},
		{"Trionic 5", false, false},
		{fakeECU, false, false},
	} {
		r.selectECU(tt.ecu)
		r.onUI(func() {
			if !allVisible(t8(r.w), tt.t8) {
				t.Errorf("%s: T8 controls visible = %v expected", tt.ecu, tt.t8)
			}
			if !allVisible(t7(r.w), tt.t7) {
				t.Errorf("%s: T7 controls visible = %v expected", tt.ecu, tt.t7)
			}
		})
		if got := fyne.CurrentApp().Preferences().String("canflasher_ecu"); got != tt.ecu {
			t.Errorf("stored ECU = %q, want %q", got, tt.ecu)
		}
	}

	r.selectECU("Trionic 7")
	r.onUI(func() {
		w := New(&Config{CSW: settings.New(&settings.Config{})})
		test.WidgetRenderer(w)
		if w.ecuSelect.Selected != "Trionic 7" || !allVisible(t7(w), true) || !allVisible(t8(w), false) {
			t.Errorf("new flasher starts on %q, want the stored Trionic 7 with its controls", w.ecuSelect.Selected)
		}
	})
}

func controls(w *CanFlasherWidget) map[string]fyne.Disableable {
	return map[string]fyne.Disableable{
		"info": w.infoBTN, "dump": w.dumpBTN, "flash": w.flashBTN, "eol": w.eolBTN,
		"marry": w.marryBTN, "recovery": w.recoveryBTN, "reset": w.resetBTN,
		"boot": w.bootBOX, "nvdm": w.nvdmBOX, "pin": w.pinEntry,
		"xor": w.seedKeyXOR, "sub": w.seedKeySub,
	}
}

// Disable locks every action and option while an operation runs; Enable
// gives them all back.
func TestDisableEnable(t *testing.T) {
	r := newRig(t)
	r.onUI(func() {
		r.w.Disable()
		for name, c := range controls(r.w) {
			if !c.Disabled() {
				t.Errorf("%s still enabled after Disable", name)
			}
		}
		r.w.Enable()
		for name, c := range controls(r.w) {
			if c.Disabled() {
				t.Errorf("%s still disabled after Enable", name)
			}
		}
	})
}

// A running operation keeps the controls locked until its bus is closed.
func TestControlsLockedWhileRunning(t *testing.T) {
	r := newRig(t)
	r.selectECU(fakeECU)
	entered, release := make(chan struct{}), make(chan struct{})
	r.mu.Lock()
	r.hook = func(m string) {
		if m == "Info" {
			close(entered)
			<-release
		}
	}
	r.mu.Unlock()
	r.tap(r.w.infoBTN)
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("Info never reached the client")
	}
	r.onUI(func() {
		for name, c := range controls(r.w) {
			if !c.Disabled() {
				t.Errorf("%s enabled mid-operation", name)
			}
		}
	})
	close(release)
	r.wait()
	r.onUI(func() {
		for name, c := range controls(r.w) {
			if c.Disabled() {
				t.Errorf("%s still disabled after the operation", name)
			}
		}
	})
}

// Each log line is stamped with the wall clock; an empty message is a blank
// line.
func TestLog(t *testing.T) {
	r := newRig(t)
	r.w.log("hello")
	r.w.log("")
	r.w.log("world")
	var text string
	r.onUI(func() { text = r.w.logText.Text })
	if !regexp.MustCompile(`^\d\d:\d\d:\d\d\.\d{3} - hello\n\n\d\d:\d\d:\d\d\.\d{3} - world\n$`).MatchString(text) {
		t.Errorf("log text = %q", text)
	}
}

// A negative progress value announces the total and resets the bar; positive
// values are the amount done.
func TestProgress(t *testing.T) {
	r := newRig(t)
	r.w.progress(-2048)
	var maxV, val float64
	r.onUI(func() { maxV, val = r.w.progressBar.Max, r.w.progressBar.Value })
	if maxV != 2048 || val != 0 {
		t.Errorf("after -2048: max %v value %v, want 2048 / 0", maxV, val)
	}
	r.w.progress(512)
	r.onUI(func() { maxV, val = r.w.progressBar.Max, r.w.progressBar.Value })
	if maxV != 2048 || val != 512 {
		t.Errorf("after 512: max %v value %v, want 2048 / 512", maxV, val)
	}
}

// The renderer lays the split out over the whole widget and asks for room
// for the log and the button column.
func TestRenderer(t *testing.T) {
	r := newRig(t)
	r.onUI(func() {
		rend := test.WidgetRenderer(r.w)
		if got := rend.MinSize(); got != fyne.NewSize(600, 450) {
			t.Errorf("MinSize = %v", got)
		}
		if objs := rend.Objects(); len(objs) != 1 || objs[0] != r.w.container {
			t.Errorf("Objects = %v, want the split container", objs)
		}
		rend.Layout(fyne.NewSize(700, 500))
		if got := r.w.container.Size(); got != fyne.NewSize(700, 500) {
			t.Errorf("container size = %v after Layout(700x500)", got)
		}
	})
}

// Info builds the adapter for the selected ECU's CAN rate and filter on the
// UI goroutine, logs every header field the ECU reports and resets it; a
// failed read still logs what came back and still resets.
func TestInfo(t *testing.T) {
	fields := []model.HeaderResult{
		{Header: model.Header{Desc: "VIN"}, Value: "YS3FD49Y941012345"},
		{Header: model.Header{Desc: "Software"}, Value: "5166225"},
	}
	tests := []struct {
		name      string
		errs      map[string]error
		wantLog   []string
		wantCalls []string
	}{
		{
			name:      "ok",
			wantLog:   []string{"VIN: YS3FD49Y941012345", "Software: 5166225"},
			wantCalls: []string{"open", "Info", "ResetECU", "close"},
		},
		{
			name:      "read fails",
			errs:      map[string]error{"Info": errors.New("no answer to 0x1A")},
			wantLog:   []string{"no answer to 0x1A", "VIN: YS3FD49Y941012345"},
			wantCalls: []string{"open", "Info", "ResetECU", "close"},
		},
		{
			name:      "reset fails",
			errs:      map[string]error{"ResetECU": errors.New("reset refused")},
			wantLog:   []string{"Software: 5166225", "reset refused"},
			wantCalls: []string{"open", "Info", "ResetECU", "close"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.info = fields
			for m, err := range tt.errs {
				r.setErr(m, err)
			}
			var uiGID string
			r.onUI(func() { uiGID = goid() })
			r.selectECU(fakeECU)
			r.tap(r.w.infoBTN)
			r.wait()

			r.wantLog(tt.wantLog...)
			r.wantCalls(tt.wantCalls...)
			r.mu.Lock()
			defer r.mu.Unlock()
			if c := r.adapterCfg; c == nil || c.CANRate != 500 || !slices.Equal(c.CANFilter, []uint32{0x7E8}) {
				t.Errorf("adapter config = %+v, want 500 kbit with filter [0x7E8]", c)
			}
			if r.adapterGID != uiGID {
				t.Error("adapter and config were not read on the UI goroutine")
			}
			if r.ecuCfg == nil || r.ecuCfg.Name != fakeECU {
				t.Errorf("client config = %+v, want Name %q", r.ecuCfg, fakeECU)
			}
		})
	}
}

// The clients' error callback lands in the log.
func TestConfigOnError(t *testing.T) {
	r := newRig(t)
	r.selectECU(fakeECU)
	var cfg *ecu.Config
	r.onUI(func() { cfg = r.w.ecuConfig() })
	if cfg.Name != fakeECU {
		t.Errorf("config Name = %q", cfg.Name)
	}
	cfg.OnError(errors.New("checksum mismatch"))
	cfg.OnMessage("erasing")
	r.wantLog("checksum mismatch", "erasing")
}

// Without an adapter chosen in the settings none of the seven bus operations
// starts a worker, and each tells the user to pick one.
func TestNoAdapterSelected(t *testing.T) {
	for name, run := range actions(t, t.TempDir()) {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			fyne.CurrentApp().Preferences().SetString("adapter", "")
			r.selectECU(fakeEOLECU)
			run(r)
			var disabled bool
			r.onUI(func() { disabled = r.w.infoBTN.Disabled() })
			if disabled {
				t.Error("buttons disabled although nothing started")
			}
			if got := r.logLines(); !slices.Equal(got, []string{"Select CANbus adapter in settings"}) {
				t.Errorf("log = %q, want only the adapter hint", got)
			}
			r.wantCalls()
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.adapterCfg != nil {
				t.Error("adapter built with none selected")
			}
		})
	}
}

// The Info and Reset buttons go straight to the bus operation, so with no
// adapter they also only show the hint.
func TestButtonsNoAdapter(t *testing.T) {
	r := newRig(t)
	fyne.CurrentApp().Preferences().SetString("adapter", "")
	r.selectECU(fakeECU)
	for _, b := range []*widget.Button{r.w.infoBTN, r.w.resetBTN} {
		r.tap(b)
	}
	if got := r.logLines(); !slices.Equal(got, []string{"Select CANbus adapter in settings", "Select CANbus adapter in settings"}) {
		t.Errorf("log = %q, want the adapter hint once per button", got)
	}
	r.wantCalls()
}

// actions runs every bus operation the flasher has, the way its buttons do,
// on files in dir.
func actions(t *testing.T, dir string) map[string]func(*rig) {
	bin := filepath.Join(dir, "in.bin")
	if err := os.WriteFile(bin, []byte{1, 2, 3, 4}, 0o600); err != nil {
		t.Fatal(err)
	}
	params := t7.EOLParams{VIN: testVIN, ProgDate: "261010", TesterSerial: t7.DefaultTesterSerial}
	return map[string]func(*rig){
		"info":    func(r *rig) { r.onUI(r.w.ecuInfo) },
		"dump":    func(r *rig) { r.onUI(func() { r.w.ecuDump(filepath.Join(dir, "out")) }) },
		"flash":   func(r *rig) { r.onUI(func() { r.w.ecuFlash(bin) }) },
		"recover": func(r *rig) { r.onUI(func() { r.w.ecuRecover(bin) }) },
		"marry":   func(r *rig) { r.onUI(func() { r.w.ecuMarry("1234") }) },
		"reset":   func(r *rig) { r.onUI(r.w.ecuReset) },
		"eol":     func(r *rig) { r.onUI(func() { r.w.runEOL(eolTestBin(), params) }) },
	}
}

// Every operation reports an adapter that will not open and builds no client.
func TestAdapterOpenFails(t *testing.T) {
	for name, run := range actions(t, t.TempDir()) {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.openErr = errors.New("adapter unplugged")
			r.selectECU(fakeEOLECU)
			run(r)
			r.wait()
			r.wantLog("adapter unplugged")
			r.wantCalls("open")
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.ecuCfg != nil {
				t.Error("client built on a bus that never opened")
			}
		})
	}
}

// With no ECU picked every operation reports an unknown ECU and closes the
// bus it opened.
func TestNoECUSelected(t *testing.T) {
	for name, run := range actions(t, t.TempDir()) {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.onUI(func() { r.w.ecuSelect.ClearSelected() })
			run(r)
			r.wait()
			r.wantLog("unknown ECU")
			r.wantCalls("open", "close")
		})
	}
}

// Dump saves the image under the chosen name with .bin added once, logs
// adapter events and client messages, and resets the ECU afterwards unless it
// is one the user must reset by hand.
func TestDump(t *testing.T) {
	tests := []struct {
		name      string
		ecu       string
		file      string // relative to the temp dir
		saved     string // "" = nothing written
		dumpErr   error
		wantLog   []string
		wantCalls []string
	}{
		{
			name: "resets over CAN", ecu: fakeECU, file: "dump", saved: "dump.bin",
			wantLog:   []string{"[INFO] fake adapter up", "dumping"},
			wantCalls: []string{"open", "DumpECU", "ResetECU", "close"},
		},
		{
			name: "keeps .bin", ecu: fakeECU, file: "dump.bin", saved: "dump.bin",
			wantCalls: []string{"open", "DumpECU", "ResetECU", "close"},
		},
		{
			name: "manual reset", ecu: fakeManualECU, file: "t7", saved: "t7.bin",
			wantCalls: []string{"open", "DumpECU", "close"},
		},
		{
			name: "dump fails", ecu: fakeECU, file: "dump", dumpErr: errors.New("read timeout at 0x40000"),
			wantLog:   []string{"read timeout at 0x40000"},
			wantCalls: []string{"open", "DumpECU", "close"},
		},
		{
			name: "cannot write", ecu: fakeECU, file: "missing/dir/dump",
			wantCalls: []string{"open", "DumpECU", "close"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.setErr("DumpECU", tt.dumpErr)
			dir := t.TempDir()
			r.selectECU(tt.ecu)
			r.onUI(func() { r.w.ecuDump(filepath.Join(dir, tt.file)) })
			r.wait()

			r.wantCalls(tt.wantCalls...)
			r.wantLog(tt.wantLog...)
			if tt.saved == "" {
				entries, _ := os.ReadDir(dir)
				if len(entries) != 0 {
					t.Errorf("files written: %v", entries)
				}
				if tt.file == "missing/dir/dump" && !strings.Contains(strings.Join(r.logLines(), "\n"), "no such file") {
					t.Errorf("log %q does not report the write error", r.logLines())
				}
				return
			}
			path := filepath.Join(dir, tt.saved)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, r.dump) {
				t.Errorf("saved % X, want % X", got, r.dump)
			}
			r.wantLog("Saved as " + path)
		})
	}
}

// Flash sends the file's bytes to the client, then resets the ECU or tells
// the user to; a failed flash is reported and nothing is reset.
func TestFlash(t *testing.T) {
	bin := bytes.Repeat([]byte{0x4E, 0x71}, 32)
	tests := []struct {
		name      string
		ecu       string
		errs      map[string]error
		missing   bool
		wantLog   []string
		wantCalls []string
	}{
		{name: "resets over CAN", ecu: fakeECU, wantLog: []string{"[INFO] fake adapter up"}, wantCalls: []string{"open", "FlashECU", "ResetECU", "close"}},
		{name: "manual reset", ecu: fakeManualECU, wantLog: []string{"Flash done, reset ECU or pull fuse to finnish"}, wantCalls: []string{"open", "FlashECU", "close"}},
		{name: "flash fails", ecu: fakeECU, errs: map[string]error{"FlashECU": errors.New("erase failed")}, wantLog: []string{"erase failed"}, wantCalls: []string{"open", "FlashECU", "close"}},
		{name: "reset fails", ecu: fakeECU, errs: map[string]error{"ResetECU": errors.New("no reset ack")}, wantLog: []string{"no reset ack"}, wantCalls: []string{"open", "FlashECU", "ResetECU", "close"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			for m, err := range tt.errs {
				r.setErr(m, err)
			}
			path := filepath.Join(t.TempDir(), "in.bin")
			if err := os.WriteFile(path, bin, 0o600); err != nil {
				t.Fatal(err)
			}
			r.selectECU(tt.ecu)
			r.onUI(func() { r.w.ecuFlash(path) })
			r.wait()
			r.wantCalls(tt.wantCalls...)
			r.wantLog(tt.wantLog...)
			r.mu.Lock()
			defer r.mu.Unlock()
			if tt.errs["FlashECU"] == nil && !bytes.Equal(r.written, bin) {
				t.Errorf("client got % X, want the file's bytes", r.written)
			}
		})
	}
}

// Flash, recovery and EOL refuse a file that cannot be read before anything
// touches the bus.
func TestUnreadableFile(t *testing.T) {
	r := newRig(t)
	r.selectECU(fakeECU)
	missing := filepath.Join(t.TempDir(), "gone.bin")
	r.onUI(func() { r.w.ecuFlash(missing) })
	r.onUI(func() { r.w.ecuRecover(missing) })
	got := r.logLines()
	if len(got) != 2 || !strings.Contains(got[0], "no such file") || !strings.Contains(got[1], "no such file") {
		t.Errorf("log = %q, want two read errors", got)
	}
	r.wantCalls()
}

// Recovery listens on the T8 boot ids instead of the ECU's normal filter,
// hands the file to the client, notifies when done and resets.
func TestRecover(t *testing.T) {
	bin := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	for _, fail := range []bool{false, true} {
		name := map[bool]string{false: "ok", true: "fails"}[fail]
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			if fail {
				r.setErr("RecoverECU", errors.New("no boot reply"))
			}
			path := filepath.Join(t.TempDir(), "in.bin")
			if err := os.WriteFile(path, bin, 0o600); err != nil {
				t.Fatal(err)
			}
			r.selectECU(fakeECU)
			r.onUI(func() { r.w.ecuRecover(path) })
			r.wait()

			r.mu.Lock()
			cfg, written := r.adapterCfg, r.written
			r.mu.Unlock()
			if cfg == nil || cfg.CANRate != 500 || !slices.Equal(cfg.CANFilter, []uint32{0x011, 0x311}) {
				t.Errorf("adapter config = %+v, want 500 kbit with filter [0x011 0x311]", cfg)
			}
			if fail {
				r.wantCalls("open", "RecoverECU", "close")
				r.wantLog("no boot reply")
				select {
				case n := <-r.notes:
					t.Errorf("notification %q after a failed recovery", n.Content)
				default:
				}
				return
			}
			r.wantCalls("open", "RecoverECU", "ResetECU", "close")
			if !bytes.Equal(written, bin) {
				t.Errorf("client got % X", written)
			}
			select {
			case n := <-r.notes:
				if n.Title != "txlogger" || n.Content != "ECU flash completed" {
					t.Errorf("notification = %+v", n)
				}
			default:
				t.Error("no notification")
			}
		})
	}
}

// Marrying needs a four character PIN, listens on 0x645 on top of the ECU's
// filter (without changing the registered one), passes the PIN on, notifies
// and resets.
func TestMarry(t *testing.T) {
	t.Run("short pin", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeECU)
		r.onUI(func() { r.w.ecuMarry("123") })
		r.wantLog("Pin code must be 4 char long")
		r.wantCalls()
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.adapterCfg != nil {
			t.Error("adapter built for a bad PIN")
		}
	})
	t.Run("ok", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeECU)
		r.onUI(func() { r.w.ecuMarry("1234") })
		r.wait()
		r.wantCalls("open", "MarryECU", "ResetECU", "close")
		r.mu.Lock()
		cfg, pin := r.adapterCfg, r.pin
		r.mu.Unlock()
		if cfg == nil || !slices.Equal(cfg.CANFilter, []uint32{0x7E8, 0x645}) {
			t.Errorf("adapter config = %+v, want filter [0x7E8 0x645]", cfg)
		}
		if got := ecu.Info(fakeECU).Filter; !slices.Equal(got, []uint32{0x7E8}) {
			t.Errorf("registered filter changed to %#x", got)
		}
		if pin != "1234" {
			t.Errorf("client got PIN %q", pin)
		}
		select {
		case n := <-r.notes:
			if n.Content != "ECU Marry" {
				t.Errorf("notification = %+v", n)
			}
		default:
			t.Error("no notification")
		}
	})
	t.Run("fails", func(t *testing.T) {
		r := newRig(t)
		r.setErr("MarryECU", errors.New("immo refused"))
		r.selectECU(fakeECU)
		r.onUI(func() { r.w.ecuMarry("1234") })
		r.wait()
		r.wantCalls("open", "MarryECU", "close")
		r.wantLog("immo refused")
		if len(r.notes) != 0 {
			t.Error("notification after a failed marriage")
		}
	})
}

// After a successful marriage or recovery, a reset the ECU refuses is logged
// after the completion notification has gone out.
func TestResetAfterWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.bin")
	if err := os.WriteFile(path, []byte{1, 2, 3, 4}, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, call, note string
		run              func(*CanFlasherWidget)
	}{
		{"marry", "MarryECU", "ECU Marry", func(w *CanFlasherWidget) { w.ecuMarry("1234") }},
		{"recover", "RecoverECU", "ECU flash completed", func(w *CanFlasherWidget) { w.ecuRecover(path) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.setErr("ResetECU", errors.New("reset not acknowledged"))
			r.selectECU(fakeECU)
			r.onUI(func() { tt.run(r.w) })
			r.wait()
			r.wantCalls("open", tt.call, "ResetECU", "close")
			r.wantLog("reset not acknowledged")
			select {
			case n := <-r.notes:
				if n.Content != tt.note {
					t.Errorf("notification = %q, want %q", n.Content, tt.note)
				}
			default:
				t.Error("no notification")
			}
		})
	}
}

// The MarryECM button asks first and marries with the PIN box's text only on
// Yes.
func TestMarryButton(t *testing.T) {
	r := newRig(t)
	r.selectECU(fakeECU)
	r.onUI(func() { r.w.pinEntry.SetText("4321") })

	r.tap(r.w.marryBTN)
	if got := r.dialogText(); !strings.Contains(got, "erase NVDM flash") {
		t.Fatalf("dialog text = %q", got)
	}
	r.tapDialog("No")
	if got := r.dialogText(); got != "" {
		t.Errorf("dialog still up after No: %q", got)
	}

	r.tap(r.w.marryBTN)
	r.tapDialog("Yes")
	r.wait()
	r.wantCalls("open", "MarryECU", "ResetECU", "close") // nothing from the No
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pin != "4321" {
		t.Errorf("client got PIN %q", r.pin)
	}
}

// Reset goes straight to the ECU when a CAN reset is harmless, asks first
// when it puts the throttle body in limp mode, and logs the outcome.
func TestResetButton(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeECU)
		r.tap(r.w.resetBTN)
		r.wait()
		if got := r.dialogText(); got != "" {
			t.Errorf("asked %q before a harmless reset", got)
		}
		r.wantCalls("open", "ResetECU", "close")
		r.wantLog("ECU reset")
	})
	t.Run("manual asks", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeManualECU)
		r.tap(r.w.resetBTN)
		if got := r.dialogText(); !strings.Contains(got, "TURN OFF THE IGNITION") {
			t.Fatalf("dialog text = %q", got)
		}
		r.tapDialog("No")
		r.wantCalls()
		r.tap(r.w.resetBTN)
		r.tapDialog("Yes")
		r.wait()
		r.wantCalls("open", "ResetECU", "close")
		r.wantLog("ECU reset")
	})
	t.Run("fails", func(t *testing.T) {
		r := newRig(t)
		r.setErr("ResetECU", errors.New("no ack"))
		r.selectECU(fakeECU)
		r.tap(r.w.resetBTN)
		r.wait()
		r.wantLog("no ack")
		if slices.Contains(r.logLines(), "ECU reset") {
			t.Error("reported a reset that failed")
		}
	})
}

// The partition unlock boxes start from the stored setting, only turn on
// after the warning is confirmed, and store whatever they end up as.
func TestPartitionUnlocks(t *testing.T) {
	for _, tt := range []struct {
		name, pref, warning string
		box                 func(*CanFlasherWidget) *widget.Check
	}{
		{"boot", settings.PrefsBoot, "Boot is for advanced users only", func(w *CanFlasherWidget) *widget.Check { return w.bootBOX }},
		{"nvdm", settings.PrefsNvdm, "overwrite any keys and marriage status", func(w *CanFlasherWidget) *widget.Check { return w.nvdmBOX }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			prefs := fyne.CurrentApp().Preferences()
			box := tt.box(r.w)
			checked := func() (c bool) { r.onUI(func() { c = box.Checked }); return }
			if checked() {
				t.Fatal("box starts checked with nothing stored")
			}

			r.tap(box)
			if got := r.dialogText(); !strings.Contains(got, tt.warning) {
				t.Fatalf("dialog text = %q", got)
			}
			r.tapDialog("No")
			if checked() || prefs.BoolWithFallback(tt.pref, true) {
				t.Errorf("after No: checked %v stored %v, want both false", checked(), prefs.Bool(tt.pref))
			}

			r.tap(box)
			r.tapDialog("Yes")
			if !checked() || !prefs.Bool(tt.pref) {
				t.Errorf("after Yes: checked %v stored %v, want both true", checked(), prefs.Bool(tt.pref))
			}

			var stored bool
			r.onUI(func() {
				w := New(&Config{CSW: settings.New(&settings.Config{})})
				test.WidgetRenderer(w)
				stored = tt.box(w).Checked
			})
			if !stored {
				t.Error("new flasher does not start from the stored unlock")
			}

			r.tap(box) // unchecking needs no confirmation
			if got := r.dialogText(); got != "" {
				t.Errorf("asked %q to lock again", got)
			}
			if checked() || prefs.Bool(tt.pref) {
				t.Errorf("after uncheck: checked %v stored %v", checked(), prefs.Bool(tt.pref))
			}
		})
	}
}

// t7Bin is a 512 KB Trionic 7 image in Intel byte order whose PI area holds
// fields, last pair first from the end of flash.
func t7Bin(fields ...[2]string) []byte {
	bin := bytes.Repeat([]byte{0xFF}, 0x80000)
	copy(bin, []byte{0xFF, 0xFF, 0xEF, 0xFC})
	for i := 4; i < 0x1000; i++ {
		bin[i] = byte(i)
	}
	addr := len(bin) - 1
	for _, f := range fields {
		data := f[1]
		bin[addr] = byte(len(data))
		bin[addr-1] = f[0][0]
		for i := range len(data) {
			bin[addr-2-i] = data[i]
		}
		addr -= 2 + len(data)
	}
	return bin
}

const testVIN = "YS3FD49Y941012345"

func eolTestBin() []byte {
	return t7Bin([2]string{"\x90", testVIN}, [2]string{"\x98", "PPCAN Nr 123"})
}

// eolEntries returns the VIN, date and tester serial boxes of the EOL form.
func (r *rig) eolEntries() []*widget.Entry {
	var out []*widget.Entry
	r.onUI(func() {
		for _, o := range dialogObjects(r.win) {
			if e, ok := o.(*widget.Entry); ok {
				out = append(out, e)
			}
		}
	})
	if len(out) != 3 {
		r.t.Fatalf("EOL form has %d entries, want 3", len(out))
	}
	return out
}

// The EOL form starts from the bin's VIN, today's date and txlogger's tester
// serial (showing the old one), and on Continue and Yes runs the whole EOL
// procedure with exactly those values and bin.
func TestEOLForm(t *testing.T) {
	r := newRig(t)
	r.selectECU(fakeEOLECU)
	bin := eolTestBin()
	before := time.Now().Format("060102")
	r.onUI(func() { r.w.eolForm(bin) })
	after := time.Now().Format("060102")

	e := r.eolEntries()
	var vin, date, serial, was string
	r.onUI(func() { vin, date, serial, was = e[0].Text, e[1].Text, e[2].Text, e[2].PlaceHolder })
	if vin != testVIN {
		t.Errorf("VIN = %q, want %q from the bin", vin, testVIN)
	}
	if date != before && date != after {
		t.Errorf("date = %q, want today %q", date, after)
	}
	if serial != t7.DefaultTesterSerial || was != "was: PPCAN Nr 123" {
		t.Errorf("serial = %q (%q), want %q (was: PPCAN Nr 123)", serial, was, t7.DefaultTesterSerial)
	}

	r.onUI(func() { e[1].SetText("261010") })
	r.tapDialog("Continue")
	if got := r.dialogText(); !strings.Contains(got, "erases the whole flash") {
		t.Fatalf("confirm text = %q", got)
	}
	r.tapDialog("Yes")
	r.wait()
	r.wantCalls("open", "EOLFlash", "close")
	r.wantLog("EOL done, turn off the ignition or pull the fuse to finish")
	r.mu.Lock()
	defer r.mu.Unlock()
	if want := (t7.EOLParams{VIN: testVIN, ProgDate: "261010", TesterSerial: t7.DefaultTesterSerial}); r.eolParams != want {
		t.Errorf("EOL params = %+v, want %+v", r.eolParams, want)
	}
	if !bytes.Equal(r.written, bin) {
		t.Error("EOL flashed a different bin")
	}
}

// Backing out at any step, or values the ECU would reject, start nothing.
func TestEOLFormAborts(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeEOLECU)
		r.onUI(func() { r.w.eolForm(eolTestBin()) })
		r.tapDialog("Cancel")
		if got := r.dialogText(); got != "" {
			t.Errorf("dialog after Cancel: %q", got)
		}
		r.wantCalls()
	})
	t.Run("no at the warning", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeEOLECU)
		r.onUI(func() { r.w.eolForm(eolTestBin()) })
		r.tapDialog("Continue")
		r.tapDialog("No")
		r.wantCalls()
	})
	t.Run("missing VIN", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeEOLECU)
		r.onUI(func() { r.w.eolForm(eolTestBin()) })
		e := r.eolEntries()
		r.onUI(func() { e[0].SetText("") })
		r.tapDialog("Continue")
		if got := r.dialogText(); !strings.Contains(got, "VIN (0x90) is required") {
			t.Errorf("dialog text = %q, want the VIN error", got)
		}
		r.tapDialog("OK")
		r.wantCalls()
	})
	t.Run("killed tester serial", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeEOLECU)
		r.onUI(func() { r.w.eolForm(eolTestBin()) })
		e := r.eolEntries()
		r.onUI(func() { e[2].SetText("PPCAN Nr 2") }) // packs to 0x4232
		r.tapDialog("Continue")
		if got := r.dialogText(); !strings.Contains(got, "kill-list") {
			t.Errorf("dialog text = %q, want the kill-list error", got)
		}
		r.wantCalls()
	})
}

// runEOL refuses clients that cannot do EOL programming and reports a failed
// run.
func TestRunEOL(t *testing.T) {
	params := t7.EOLParams{VIN: testVIN, ProgDate: "261010", TesterSerial: t7.DefaultTesterSerial}
	t.Run("not a T7", func(t *testing.T) {
		r := newRig(t)
		r.selectECU(fakeECU)
		r.onUI(func() { r.w.runEOL(eolTestBin(), params) })
		r.wait()
		r.wantLog("EOL programming is only supported on Trionic 7")
		r.wantCalls("open", "close")
	})
	t.Run("fails", func(t *testing.T) {
		r := newRig(t)
		r.setErr("EOLFlash", errors.New("end of procedure rejected"))
		r.selectECU(fakeEOLECU)
		r.onUI(func() { r.w.runEOL(eolTestBin(), params) })
		r.wait()
		r.wantLog("end of procedure rejected")
		if slices.Contains(r.logLines(), "EOL done, turn off the ignition or pull the fuse to finish") {
			t.Error("reported success after a failure")
		}
	})
}
