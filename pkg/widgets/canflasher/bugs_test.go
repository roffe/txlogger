package canflasher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/roffe/txlogger/pkg/ecu"
)

// Dump and flash decide between a CAN reset and "reset it yourself" from the
// ECU they ran on. The ECU select stays enabled while they run, and picking a
// different ECU mid-operation must not make them reset a ManualReset ECU over
// CAN, which on a T7 puts the throttle body in limp mode.
func TestResetFollowsOperatedECU(t *testing.T) {
	for _, op := range []string{"DumpECU", "FlashECU"} {
		t.Run(op, func(t *testing.T) {
			r := newRig(t)
			entered, release := make(chan struct{}), make(chan struct{})
			r.mu.Lock()
			r.hook = func(m string) {
				if m == op {
					close(entered)
					<-release
				}
			}
			r.mu.Unlock()
			dir := t.TempDir()
			bin := filepath.Join(dir, "in.bin")
			if err := os.WriteFile(bin, []byte{1, 2, 3, 4}, 0o600); err != nil {
				t.Fatal(err)
			}
			r.selectECU(fakeManualECU)
			r.onUI(func() {
				if op == "DumpECU" {
					r.w.ecuDump(filepath.Join(dir, "out"))
				} else {
					r.w.ecuFlash(bin)
				}
			})
			select {
			case <-entered:
			case <-time.After(30 * time.Second):
				t.Fatalf("%s never reached the client", op)
			}
			r.selectECU(fakeECU)
			close(release)
			r.wait()
			r.wantCalls("open", op, "close")
		})
	}
}

// ecuConfig and the adapter read widget state, so every action builds them on
// the UI goroutine before its worker starts — MarryECM included, whose
// confirmation callback already runs there.
func TestMarryReadsWidgetsOnUIGoroutine(t *testing.T) {
	r := newRig(t)
	r.selectECU(fakeECU)
	r.onUI(func() { r.w.pinEntry.SetText("1234") })
	var uiGID string
	r.onUI(func() { uiGID = goid() })
	r.tap(r.w.marryBTN)
	r.tapDialog("Yes")
	r.wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapterGID != uiGID {
		t.Errorf("adapter and config built on goroutine %s, not the UI goroutine %s", r.adapterGID, uiGID)
	}
}

// The clients call OnProgress from their own goroutine; announcing the total
// must reach the progress bar through fyne.Do like the value does, not by
// writing its Max from the client goroutine while the UI goroutine may be
// rendering it. Only -race can see the difference.
func TestProgressTotalSetOnUIGoroutine(t *testing.T) {
	if !raceEnabled {
		t.Skip("needs -race")
	}
	r := newRig(t)
	var cfg *ecu.Config
	r.onUI(func() { cfg = r.w.ecuConfig() })
	start, done := make(chan struct{}), make(chan struct{})
	go func() {
		<-start
		cfg.OnProgress(-4096)
		close(done)
	}()
	r.onUI(func() {
		close(start)
		r.w.progressBar.Refresh() // a render the client did not wait for
	})
	<-done
	var maxV float64
	r.onUI(func() { maxV = r.w.progressBar.Max })
	if maxV != 4096 {
		t.Errorf("progress bar max = %v, want 4096", maxV)
	}
}
