package datalogger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// failAdapter is a device that cannot be opened.
type failAdapter struct{ err error }

func (f failAdapter) Open(context.Context, *gocan.Bus) error { return f.err }
func (failAdapter) Send(context.Context, gocan.Frame) error  { return nil }
func (failAdapter) Close() error                             { return nil }

// New builds the logger for the selected ECU, or the txbridge logger when the
// device is a txbridge, or the remote client in remote mode 2, and reports the
// log file it opened.
func TestNewPicksLogger(t *testing.T) {
	tests := []struct {
		name   string
		ecu    string
		dev    gocan.Adapter
		fast   bool
		remote int
		want   string
	}{
		{"T5", "T5", &gocan.Loopback{}, false, 0, "*datalogger.T5Client"},
		{"T5 fast", "T5", &gocan.Loopback{}, true, 0, "*datalogger.T5FastClient"},
		{"T7", "T7", &gocan.Loopback{}, false, 0, "*datalogger.T7Client"},
		{"T8", "T8", &gocan.Loopback{}, false, 0, "*datalogger.T8Client"},
		{"txbridge T7", "T7", newFakeTxbridge(nil), false, 0, "*datalogger.TxBridge"},
		{"remote", "T7", &gocan.Loopback{}, false, 2, "*datalogger.RemoteClient"},
		{"remote ignores ECU support", "AW55", &gocan.Loopback{}, false, 2, "*datalogger.RemoteClient"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := newMsgLog(t)
			cfg := testCfg(t, tt.ecu, tt.dev, nil, msgs)
			cfg.ExperimentalT5FastLogging = tt.fast
			cfg.RemoteMode = tt.remote
			cl, filename, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			c, ok := cl.(*Client)
			if !ok {
				t.Fatalf("New returned %T", cl)
			}
			if got := fmt.Sprintf("%T", c.IClient); got != tt.want {
				t.Fatalf("logger %s, want %s", got, tt.want)
			}
			if filepath.Dir(filename) != cfg.LogPath || filepath.Ext(filename) != ".csv" {
				t.Fatalf("filename %q", filename)
			}
			if !msgs.has("Logging to " + filename) {
				t.Fatalf("messages %v", msgs.all())
			}
			if _, err := os.Stat(filename); err != nil {
				t.Fatal(err)
			}
			cl.Close()
		})
	}
}

// An ECU without a logger (for the device in use) is refused before a log
// file is created.
func TestNewUnsupportedECU(t *testing.T) {
	for _, dev := range []gocan.Adapter{&gocan.Loopback{}, newFakeTxbridge(nil)} {
		msgs := newMsgLog(t)
		cfg := testCfg(t, "AW55", dev, nil, msgs)
		_, _, err := New(cfg)
		if err == nil || err.Error() != "AW55 not supported yet" {
			t.Fatalf("%s: err = %v", gocan.AdapterName(dev), err)
		}
		if entries, _ := os.ReadDir(cfg.LogPath); len(entries) != 0 {
			t.Fatalf("log files created: %v", entries)
		}
	}
}

// An unknown log format fails New.
func TestNewUnknownLogFormat(t *testing.T) {
	cfg := testCfg(t, "T7", &gocan.Loopback{}, nil, newMsgLog(t))
	cfg.LogFormat = "XLS"
	if _, _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("err = %v", err)
	}
}

// Client.Start zeroes the UI counters before handing over to the logger, and
// a device that cannot be opened ends the session with its error.
func TestClientStartResetsCountersAndReportsOpenError(t *testing.T) {
	var calls []string
	boom := errors.New("no such device")
	for _, ecu := range []string{"T5", "T7", "T8"} {
		calls = calls[:0]
		cfg := testCfg(t, ecu, failAdapter{boom}, nil, newMsgLog(t))
		cfg.ErrorCounter = func(n int) { calls = append(calls, fmt.Sprint("err", n)) }
		cfg.CaptureCounter = func(n int) { calls = append(calls, fmt.Sprint("cap", n)) }
		cfg.FpsCounter = func(n int) { calls = append(calls, fmt.Sprint("fps", n)) }
		cl, _, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		err = cl.Start(context.Background())
		cl.Close()
		if !errors.Is(err, boom) {
			t.Fatalf("%s: Start = %v", ecu, err)
		}
		if !slices.Equal(calls, []string{"err0", "cap0", "fps0"}) {
			t.Fatalf("%s: counter calls %v", ecu, calls)
		}
	}
}

// Adapter events reach the log window; error events also count as logging
// errors, lesser ones do not. The events are raised from inside the loop's
// first log write so the count cannot race the loop's own per-second reset
// (that race is TestAdapterErrorCountingIsRaceFree).
func TestAdapterEventsReportedAndCounted(t *testing.T) {
	t7 := &ecusim.T7{}
	t7.SetSymbol(100, []byte{0, 1})
	t8 := &ecusim.T8{}
	t8.SetSymbol(100, []byte{0, 1})
	tests := []struct {
		ecu  string
		dev  *hookAdapter
		syms []*symbol.Symbol
		new  func(Config, LogWriter) (IClient, error)
	}{
		{"T5", newHook(&ecusim.T5{}, "", nil), t5Symbols(), NewT5},
		{"T7", newHook(t7, "OBDLink SX", nil), []*symbol.Symbol{{Name: "MAF.m_AirInlet", Number: 100, Length: 2}}, NewT7},
		{"T8", newHook(t8, "", nil), []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 100, Length: 2}}, NewT8},
	}
	for _, tt := range tests {
		t.Run(tt.ecu, func(t *testing.T) {
			msgs := newMsgLog(t)
			warn := gocan.Event{Type: gocan.EventTypeWarning, Details: "bus load high"}
			bad := gocan.Event{Type: gocan.EventTypeError, Details: "frame dropped", Err: errors.New("overrun")}
			rec := newRecWriter()
			rec.onWrite = func(n int) {
				if n == 1 {
					tt.dev.emit(warn)
					tt.dev.emit(bad)
				}
			}
			cl, _ := tt.new(testCfg(t, tt.ecu, tt.dev, tt.syms, msgs), rec)
			runLogger(t, cl)
			rec.waitRows(t, 2, 10*time.Second) // the second write starts after both events were handled
			if !msgs.has(warn.String()) || !msgs.has(bad.String()) || msgs.errs.Load() != 1 {
				t.Fatalf("errors %d, messages %v", msgs.errs.Load(), msgs.all())
			}
		})
	}
}

// An adapter error event is counted on the goroutine that raised it, while
// the logging loop reads and resets the same per-second error count; the two
// must not race. The failure only shows under -race.
func TestAdapterErrorCountingIsRaceFree(t *testing.T) {
	hook := newHook(&ecusim.T5{}, "", nil)
	cfg := testCfg(t, "T5", hook, t5Symbols(), newMsgLog(t))
	ticked := make(chan struct{})
	var once sync.Once
	// FpsCounter runs in the loop's per-second step, right before it checks
	// and resets errPerSecond.
	cfg.FpsCounter = func(int) { once.Do(func() { close(ticked) }) }
	cl, _ := NewT5(cfg, newRecWriter())
	runLogger(t, cl)
	waitClosed(t, ticked, 5*time.Second, "the loop's per-second step")
	hook.emit(gocan.Event{Type: gocan.EventTypeError, Details: "frame dropped", Err: errors.New("overrun")})
}
