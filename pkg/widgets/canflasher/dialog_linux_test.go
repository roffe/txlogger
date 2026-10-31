package canflasher

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// On Linux the file dialogs run in a re-executed copy of the program, marked
// by FP=1, which answers with JSON on stdout. Here that copy is the test
// binary, and TestMain answers with the path the test put in envPick.
const envPick = "CANFLASHER_TEST_PICK"

func TestMain(m *testing.M) {
	if os.Getenv("FP") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"Paths": []string{os.Getenv(envPick)}})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// pick makes the next file dialogs answer path.
func pick(t *testing.T, path string) {
	t.Setenv(envPick, path)
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0")) // no 1s idle in -race children
}

// eventually polls cond on the UI goroutine until it holds.
func (r *rig) eventually(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var ok bool
		r.onUI(func() { ok = cond() })
		if ok {
			return
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// Dump saves to the name picked in the save dialog, with .bin added.
func TestDumpButton(t *testing.T) {
	r := newRig(t)
	dir := t.TempDir()
	pick(t, filepath.Join(dir, "picked"))
	r.selectECU(fakeECU)
	r.tap(r.w.dumpBTN)
	r.wait()
	got, err := os.ReadFile(filepath.Join(dir, "picked.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, r.dump) {
		t.Errorf("saved % X", got)
	}
}

// Flash and Recovery send the picked file; with the systems partition
// unlocked Flash first asks before overwriting keys and marriage.
func TestFlashAndRecoveryButtons(t *testing.T) {
	bin := []byte{0x11, 0x22, 0x33, 0x44}
	path := filepath.Join(t.TempDir(), "in.bin")
	if err := os.WriteFile(path, bin, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, call string
		nvdm       bool
	}{
		{"flash", "FlashECU", false},
		{"flash nvdm", "FlashECU", true},
		{"recovery", "RecoverECU", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			pick(t, path)
			r.selectECU(fakeManualECU) // no reset, so the call list ends at the write
			btn := r.w.flashBTN
			if tt.call == "RecoverECU" {
				btn = r.w.recoveryBTN
			}
			r.onUI(func() { r.w.nvdmBOX.Checked = tt.nvdm })
			r.tap(btn)
			if tt.nvdm {
				if got := r.dialogText(); !strings.Contains(got, "overwrite keys and marriage status") {
					t.Fatalf("dialog text = %q", got)
				}
				r.tapDialog("Yes")
			}
			r.wait()
			wantCalls := []string{"open", tt.call, "close"}
			if tt.call == "RecoverECU" {
				wantCalls = []string{"open", tt.call, "ResetECU", "close"} // recovery always resets
			}
			r.wantCalls(wantCalls...)
			r.mu.Lock()
			defer r.mu.Unlock()
			if !bytes.Equal(r.written, bin) {
				t.Errorf("client got % X", r.written)
			}
		})
	}
}

// Saying No to the systems partition warning opens no file dialog.
func TestFlashButtonNvdmNo(t *testing.T) {
	r := newRig(t)
	pick(t, filepath.Join(t.TempDir(), "never-read.bin"))
	r.selectECU(fakeECU)
	r.onUI(func() { r.w.nvdmBOX.Checked = true })
	r.tap(r.w.flashBTN)
	r.tapDialog("No")
	r.wantCalls()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapterCfg != nil {
		t.Error("an adapter was built after No")
	}
}

// EOL Flash takes a Motorola byte-order dump, swaps it to Intel order and
// programs that; a file that cannot be read or is not a T7 image is refused
// before the form.
func TestEOLButton(t *testing.T) {
	t.Run("motorola dump", func(t *testing.T) {
		r := newRig(t)
		intel := eolTestBin()
		motorola := make([]byte, len(intel))
		for i := 0; i < len(intel); i += 2 {
			motorola[i], motorola[i+1] = intel[i+1], intel[i]
		}
		path := filepath.Join(t.TempDir(), "motorola.bin")
		if err := os.WriteFile(path, motorola, 0o600); err != nil {
			t.Fatal(err)
		}
		pick(t, path)
		r.selectECU(fakeEOLECU)
		r.tap(r.w.eolBTN)
		r.eventually("the EOL form", func() bool { return r.win.Canvas().Overlays().Top() != nil })
		if e := r.eolEntries(); e[0].Text != testVIN {
			t.Errorf("form VIN = %q, want it read from the swapped bin", e[0].Text)
		}
		r.tapDialog("Continue")
		r.tapDialog("Yes")
		r.wait()
		r.mu.Lock()
		defer r.mu.Unlock()
		if !bytes.Equal(r.written, intel) {
			t.Error("EOL did not program the Intel-order image")
		}
	})
	t.Run("not a T7 image", func(t *testing.T) {
		r := newRig(t)
		path := filepath.Join(t.TempDir(), "small.bin")
		if err := os.WriteFile(path, make([]byte, 1024), 0o600); err != nil {
			t.Fatal(err)
		}
		pick(t, path)
		r.selectECU(fakeEOLECU)
		r.tap(r.w.eolBTN)
		r.eventually("the size error", func() bool { return strings.Contains(r.w.logText.Text, "expected a 512KB") })
		if got := r.dialogText(); got != "" {
			t.Errorf("form shown for a bad image: %q", got)
		}
		r.wantCalls()
	})
	t.Run("unreadable pick", func(t *testing.T) {
		r := newRig(t)
		dir := t.TempDir() // opens as a reader, but reading it fails
		pick(t, dir)
		r.selectECU(fakeEOLECU)
		r.tap(r.w.eolBTN)
		r.eventually("the read error", func() bool { return strings.Contains(r.w.logText.Text, "is a directory") })
		if got := r.dialogText(); got != "" {
			t.Errorf("form shown for an unreadable file: %q", got)
		}
		r.wantCalls()
	})
	t.Run("wrong cpu", func(t *testing.T) {
		r := newRig(t)
		path := filepath.Join(t.TempDir(), "t8.bin")
		if err := os.WriteFile(path, make([]byte, 0x80000), 0o600); err != nil {
			t.Fatal(err)
		}
		pick(t, path)
		r.selectECU(fakeEOLECU)
		r.tap(r.w.eolBTN)
		r.eventually("the header error", func() bool { return strings.Contains(r.w.logText.Text, "doesn't appear to be for a Trionic 7") })
		r.wantCalls()
	})
}
