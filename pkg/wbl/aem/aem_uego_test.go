package aem

import (
	"context"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// parse turns each clean line (AFR x10 in ASCII) into lambda and discards,
// and counts, every line holding a byte that is not part of a number, that
// does not parse as one or that overruns the 8 byte buffer; empty lines and
// a line still waiting for its ending are neither.
func TestParse(t *testing.T) {
	// Corrupt lines are taken from a real log of an AEM on a Prolific cable.
	tests := []struct {
		name string
		in   string
		want float64
		bad  int // lines discarded as corrupt
	}{
		{"clean", "10.3\r\n", 1.03, 0},
		{"split reads", "10", 0, 0},
		{"leading nul byte", "\xff10.3\r\n", 0, 1},
		{"corrupt first digit", "\x100.3\r\n", 0, 1},
		{"digit dropped to space", " 0.3\r\n", 0, 1},
		{"corrupt second digit", "1\x10.3\r\n", 0, 1},
		{"garbage then good line", "\x110.4\r\n14.7\r\n", 1.47, 1},
		{"no line ending", "1234567890123\r\n", 0, 1},
		{"LF only line ending", "14.7\n", 1.47, 0},
		{"eight characters fill the buffer", "14.70000\r\n", 1.47, 0},
		{"sign inside the number", "1+4.7\r\n", 0, 1},
		{"lone decimal point", ".\r\n", 0, 1},
		{"empty lines", "\r\n\n", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewAEMuegoClient("test", func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			a.parse([]byte(tt.in))
			if got := a.GetLambda(); got != tt.want {
				t.Errorf("parse(%q) lambda = %v, want %v", tt.in, got, tt.want)
			}
			if a.badFrames != tt.bad {
				t.Errorf("parse(%q) discarded %d corrupt lines, want %d", tt.in, a.badFrames, tt.bad)
			}
		})
	}
}

func TestParseAcrossReads(t *testing.T) {
	a, _ := NewAEMuegoClient("test", func(string) {})
	for _, chunk := range []string{"1", "0.", "3\r", "\n"} {
		a.parse([]byte(chunk))
	}
	if got := a.GetLambda(); got != 1.03 {
		t.Errorf("lambda = %v, want 1.03", got)
	}
}

// logRec records log lines and lets a test wait for one.
type logRec struct {
	mu    sync.Mutex
	lines []string
	added chan struct{}
}

func newLogRec() *logRec { return &logRec{added: make(chan struct{}, 1)} }

func (l *logRec) add(s string) {
	l.mu.Lock()
	l.lines = append(l.lines, s)
	l.mu.Unlock()
	select {
	case l.added <- struct{}{}:
	default:
	}
}

func (l *logRec) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

func (l *logRec) count(sub string) int {
	n := 0
	for _, s := range l.all() {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

// wait blocks until a line containing sub is logged, failing after 5 seconds.
func (l *logRec) wait(t *testing.T, sub string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for l.count(sub) == 0 {
		select {
		case <-l.added:
		case <-timeout:
			t.Fatalf("no log line containing %q in %q", sub, l.all())
		}
	}
}

// Corrupt lines are counted and reported on the 1st, 26th, 51st... one;
// empty lines are not corrupt.
func TestBadFramesLoggedPeriodically(t *testing.T) {
	logs := newLogRec()
	a, _ := NewAEMuegoClient("test", logs.add)
	a.parse([]byte("\r\n\n"))
	for range 51 {
		a.parse([]byte("1x\r\n"))
	}
	want := []string{
		"AEM: 1 corrupt readings discarded, check serial cable/adapter",
		"AEM: 26 corrupt readings discarded, check serial cable/adapter",
		"AEM: 51 corrupt readings discarded, check serial cable/adapter",
	}
	if got := logs.all(); !slices.Equal(got, want) {
		t.Errorf("logs = %q, want %q", got, want)
	}
}

// A good line after a corrupt one still updates lambda: the bad-line state
// is cleared at each line ending.
func TestParseRecoversAfterBadLine(t *testing.T) {
	a, _ := NewAEMuegoClient("test", func(string) {})
	a.parse([]byte("14.7\r\n"))
	a.parse([]byte("1\x002.3\r\n"))
	if got := a.GetLambda(); got != 1.47 {
		t.Fatalf("lambda = %v after a corrupt line, want 1.47 kept", got)
	}
	a.parse([]byte("12.3\r\n"))
	if got := a.GetLambda(); math.Abs(got-1.23) > 1e-9 {
		t.Errorf("lambda = %v, want 1.23", got)
	}
}

// SetData decodes the AEM CAN frame: λ in bytes 0-1 at 0.0001/bit, O2 in
// bytes 2-3 at 0.001 %/bit and system volts in byte 4 at 0.1 V/bit.
func TestSetData(t *testing.T) {
	a, _ := NewAEMuegoClient("test", func(string) {})
	if err := a.SetData([]byte{0x26, 0x8E, 0x04, 0xD2, 0x8A, 0x00, 0x80, 0x00}); err != nil {
		t.Fatal(err)
	}
	if got := a.GetLambda(); math.Abs(got-0.987) > 1e-9 {
		t.Errorf("lambda = %v, want 0.987", got)
	}
	if got, want := a.String(), "Lambda: 0.9870, Oxygen: 1.234, Voltage: 13.8"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// The O2 field of the CAN frame is a signed 16 bit value (-32.768 % to
// 32.767 % per the frame table), negative when running rich.
func TestSetDataNegativeOxygen(t *testing.T) {
	a, _ := NewAEMuegoClient("test", func(string) {})
	a.SetData([]byte{0x26, 0x8E, 0xFC, 0x18, 0x8A, 0x00, 0x80, 0x00})
	if got, want := a.String(), "Lambda: 0.9870, Oxygen: -1.000, Voltage: 13.8"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// Start reports a port that cannot be opened instead of starting the reader.
func TestStartPortError(t *testing.T) {
	a, _ := NewAEMuegoClient(filepath.Join(t.TempDir(), "missing"), func(string) {})
	if err := a.Start(context.Background()); err == nil {
		a.Stop()
		t.Fatal("Start on a missing port succeeded")
	}
	if a.getPort() != nil {
		t.Error("port stored after a failed open")
	}
}

// Stop logs once however often it is called, and a stopped client never
// tries to reconnect.
func TestStopOnce(t *testing.T) {
	logs := newLogRec()
	a, _ := NewAEMuegoClient(filepath.Join(t.TempDir(), "missing"), logs.add)
	a.Stop()
	a.Stop()
	if a.reconnect(context.Background()) {
		t.Error("reconnect after Stop reported success")
	}
	if got := logs.all(); !slices.Equal(got, []string{"Stopping AEM serial client"}) {
		t.Errorf("logs = %q, want one stop message", got)
	}
}

// reconnect retries a port that won't open until the client is stopped or
// its context ends, and then reports failure.
func TestReconnectGivesUp(t *testing.T) {
	for _, c := range []struct {
		name string
		end  func(*AEMuego, context.CancelFunc)
	}{
		{"Stop", func(a *AEMuego, _ context.CancelFunc) { a.Stop() }},
		{"context cancelled", func(_ *AEMuego, cancel context.CancelFunc) { cancel() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			logs := newLogRec()
			port := filepath.Join(t.TempDir(), "missing")
			a, _ := NewAEMuegoClient(port, logs.add)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan bool)
			go func() { done <- a.reconnect(ctx) }()
			logs.wait(t, "AEM: reconnect failed: ")
			if logs.count("AEM: reconnecting to "+port) == 0 {
				t.Errorf("logs = %q, want a reconnect attempt on %s", logs.all(), port)
			}
			c.end(a, cancel)
			select {
			case ok := <-done:
				if ok {
					t.Error("reconnect reported success")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("reconnect still retrying")
			}
		})
	}
}

// A cancelled context stops reconnect before any attempt.
func TestReconnectCancelledContext(t *testing.T) {
	logs := newLogRec()
	a, _ := NewAEMuegoClient("test", logs.add)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.reconnect(ctx) {
		t.Error("reconnect with a cancelled context reported success")
	}
	if got := logs.all(); len(got) != 0 {
		t.Errorf("logs = %q, want no attempt", got)
	}
}
