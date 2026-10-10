package stag

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.bug.st/serial"
)

// errPortClosed is what fakePort.Read returns once the port is closed.
var errPortClosed = errors.New("port closed")

// waitTimeout bounds every wait on the client's goroutines.
const waitTimeout = 5 * time.Second

// readEvent is one result handed out by fakePort.Read: data or an error.
type readEvent struct {
	data []byte
	err  error
}

// fakePort is a serial.Port whose reads are scripted by the test, in order,
// and whose writes are recorded. Like the real port opened with a 5ms read
// timeout, Read returns (0, nil) when nothing arrives in time, and it fails
// once the port is closed.
type fakePort struct {
	events   chan readEvent
	writes   chan []byte
	closed   chan struct{}
	closeErr error

	mu      sync.Mutex
	pending []byte
	closes  int
}

func newFakePort() *fakePort {
	return &fakePort{
		events: make(chan readEvent, 64),
		writes: make(chan []byte, 64),
		closed: make(chan struct{}),
	}
}

// feed queues data for the next Read.
func (f *fakePort) feed(b []byte) { f.events <- readEvent{data: b} }

// fail makes the next Read (after queued data) return err.
func (f *fakePort) fail(err error) { f.events <- readEvent{err: err} }

func (f *fakePort) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

func (f *fakePort) Read(p []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, errPortClosed
	default:
	}
	f.mu.Lock()
	if len(f.pending) > 0 {
		n := copy(p, f.pending)
		f.pending = f.pending[n:]
		f.mu.Unlock()
		return n, nil
	}
	f.mu.Unlock()

	timeout := time.NewTimer(5 * time.Millisecond)
	defer timeout.Stop()
	select {
	case <-f.closed:
		return 0, errPortClosed
	case ev := <-f.events:
		if ev.err != nil {
			return 0, ev.err
		}
		n := copy(p, ev.data)
		f.mu.Lock()
		f.pending = ev.data[n:]
		f.mu.Unlock()
		return n, nil
	case <-timeout.C:
		return 0, nil
	}
}

func (f *fakePort) Write(p []byte) (int, error) {
	f.writes <- slices.Clone(p)
	return len(p), nil
}

func (f *fakePort) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	if f.closes == 1 {
		close(f.closed)
	}
	return f.closeErr
}

func (f *fakePort) SetMode(*serial.Mode) error                           { return nil }
func (f *fakePort) Drain() error                                         { return nil }
func (f *fakePort) ResetInputBuffer() error                              { return nil }
func (f *fakePort) ResetOutputBuffer() error                             { return nil }
func (f *fakePort) SetDTR(bool) error                                    { return nil }
func (f *fakePort) SetRTS(bool) error                                    { return nil }
func (f *fakePort) GetModemStatusBits() (*serial.ModemStatusBits, error) { return nil, nil }
func (f *fakePort) SetReadTimeout(time.Duration) error                   { return nil }
func (f *fakePort) Break(time.Duration) error                            { return nil }

// nextWrite returns the next request the client wrote to f.
func nextWrite(t *testing.T, f *fakePort) []byte {
	t.Helper()
	select {
	case w := <-f.writes:
		return w
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for a request")
		return nil
	}
}

// logRec records the client's log lines; it never touches t, so goroutines
// that outlive a test can still log into it.
type logRec struct {
	mu      sync.Mutex
	lines   []string
	changed chan struct{}
}

func newLogRec() *logRec { return &logRec{changed: make(chan struct{}, 1)} }

func (l *logRec) log(s string) {
	l.mu.Lock()
	l.lines = append(l.lines, s)
	l.mu.Unlock()
	select {
	case l.changed <- struct{}{}:
	default:
	}
}

func (l *logRec) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

// count returns how many lines equal s.
func (l *logRec) count(s string) int {
	n := 0
	for _, line := range l.all() {
		if line == s {
			n++
		}
	}
	return n
}

// hasPrefix reports whether any line starts with prefix.
func (l *logRec) hasPrefix(prefix string) bool {
	return slices.ContainsFunc(l.all(), func(s string) bool { return strings.HasPrefix(s, prefix) })
}

// waitFor blocks until a line starting with prefix has been logged.
func (l *logRec) waitFor(t *testing.T, prefix string) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for !l.hasPrefix(prefix) {
		select {
		case <-l.changed:
		case <-deadline:
			t.Fatalf("timed out waiting for log %q; got %q", prefix, l.all())
		}
	}
}

// waitClosed blocks until ch is closed.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// missingPort returns a serial port path that does not exist.
func missingPort(t *testing.T) string {
	return filepath.Join(t.TempDir(), "ttyMissing")
}
