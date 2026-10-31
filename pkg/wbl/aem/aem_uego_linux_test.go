package aem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// openPTY returns the master side of a new pseudo-terminal and the path of
// its slave, which go.bug.st/serial opens like a USB serial adapter.
func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Skipf("no pseudo-terminals: %v", err)
	}
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		unix.Close(fd)
		t.Fatalf("unlockpt: %v", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		unix.Close(fd)
		t.Fatalf("ptsname: %v", err)
	}
	// Non-blocking, so os.File uses the poller and read deadlines work.
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	t.Cleanup(func() { master.Close() })
	return master, fmt.Sprintf("/dev/pts/%d", n)
}

// write sends s to the device side of the pty.
func write(t *testing.T, master *os.File, s string) {
	t.Helper()
	if _, err := master.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// waitSlaveClosed fails the test unless every handle on the pty's slave
// side gets closed: the master then reads EIO.
func waitSlaveClosed(t *testing.T, master *os.File) {
	t.Helper()
	master.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64)
	for {
		_, err := master.Read(buf)
		if errors.Is(err, syscall.EIO) {
			return
		}
		if err != nil {
			t.Fatalf("serial port still open: %v", err)
		}
	}
}

// runLoops counts running run() goroutines.
func runLoops() int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	return strings.Count(string(buf), "aem.(*AEMuego).run(")
}

// waitRunExited fails the test unless every run() goroutine ends.
func waitRunExited(t *testing.T) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for runLoops() > 0 {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatal("run() still running")
		}
	}
}

// waitLambda polls until the client reads want.
func waitLambda(t *testing.T, a *AEMuego, want float64) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for a.GetLambda() != want {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("lambda = %v, want %v", a.GetLambda(), want)
		}
	}
}

// Start opens the serial port and reads the AEM's ASCII lines into lambda
// until Stop, which ends the reader and closes the port.
func TestStartReadsSerialPort(t *testing.T) {
	master, slave := openPTY(t)
	logs := newLogRec()
	a, _ := NewAEMuegoClient(slave, logs.add)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	write(t, master, "10.3\r\n")
	waitLambda(t, a, 1.03)
	write(t, master, "1")
	write(t, master, "4.7\r\n")
	waitLambda(t, a, 1.47)

	a.Stop()
	waitSlaveClosed(t, master)
	waitRunExited(t)
	if n := logs.count("reconnect"); n != 0 {
		t.Errorf("logs = %q, want no reconnect after Stop", logs.all())
	}
}

// Cancelling the Start context ends the reader and closes the port.
func TestRunEndsOnContextCancel(t *testing.T) {
	master, slave := openPTY(t)
	a, _ := NewAEMuegoClient(slave, func(string) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	write(t, master, "10.3\r\n")
	waitLambda(t, a, 1.03)
	cancel()
	waitSlaveClosed(t, master)
	waitRunExited(t)
}

// When the port drops, run() closes it, reopens the same path and starts a
// fresh line: a half-read line from before the drop is discarded.
func TestRunReconnectsAfterPortDrop(t *testing.T) {
	master1, slave1 := openPTY(t)
	master2, slave2 := openPTY(t)
	// The client opens a symlink, so the "same port" can come back as
	// another pty after the first one goes away.
	port := filepath.Join(t.TempDir(), "ttyAEM")
	if err := os.Symlink(slave1, port); err != nil {
		t.Fatal(err)
	}
	logs := newLogRec()
	a, _ := NewAEMuegoClient(port, logs.add)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	// "9" starts a line the drop cuts off.
	write(t, master1, "10.3\r\n9")
	waitLambda(t, a, 1.03)
	if err := os.Remove(port); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(slave2, port); err != nil {
		t.Fatal(err)
	}
	master1.Close()
	logs.wait(t, "AEM: reconnected to "+port)

	write(t, master2, "14.7\r\n")
	waitLambda(t, a, 1.47)
	if logs.count("AEM: reconnecting to "+port) == 0 {
		t.Errorf("logs = %q, want the reconnect attempt", logs.all())
	}
}

// When the dropped port does not come back, run() keeps retrying until
// Stop ends it.
func TestRunStopsWhileReconnecting(t *testing.T) {
	master, slave := openPTY(t)
	port := filepath.Join(t.TempDir(), "ttyAEM")
	if err := os.Symlink(slave, port); err != nil {
		t.Fatal(err)
	}
	logs := newLogRec()
	a, _ := NewAEMuegoClient(port, logs.add)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(port); err != nil {
		t.Fatal(err)
	}
	master.Close()
	logs.wait(t, "AEM: reconnect failed: ")
	a.Stop()
	waitRunExited(t)
}

// reconnect retries a port that will not open, once per reconnectDelay,
// until it opens; the reopened port is stored and a line cut off by the
// drop is forgotten.
func TestReconnectRetriesUntilPortOpens(t *testing.T) {
	_, slave := openPTY(t)
	port := filepath.Join(t.TempDir(), "ttyAEM")
	logs := newLogRec()
	a, _ := NewAEMuegoClient(port, logs.add)
	defer a.Stop()
	// Half a line, corrupt, was buffered when the port dropped.
	a.dataPos, a.frameBad = 3, true
	done := make(chan bool, 1)
	go func() { done <- a.reconnect(context.Background()) }()
	logs.wait(t, "AEM: reconnect failed: ")
	// The port comes back after the first attempt failed.
	if err := os.Symlink(slave, port); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("reconnect gave up")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not retry the port")
	}
	if n := logs.count("AEM: reconnecting to " + port); n < 2 {
		t.Errorf("logs = %q, want a second attempt after the failed one", logs.all())
	}
	if logs.count("AEM: reconnected to "+port) != 1 {
		t.Errorf("logs = %q, want one reconnected message", logs.all())
	}
	if a.getPort() == nil {
		t.Error("reopened port not stored")
	}
	if a.dataPos != 0 || a.frameBad {
		t.Errorf("line state dataPos=%d frameBad=%v after reconnect, want 0 false", a.dataPos, a.frameBad)
	}
}
