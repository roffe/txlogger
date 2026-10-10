package stag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// expectRead reads len(want) bytes from the device side and compares them.
func expectRead(t *testing.T, master *os.File, want []byte) {
	t.Helper()
	if err := master.SetReadDeadline(time.Now().Add(waitTimeout)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(master, got); err != nil {
		t.Fatalf("reading request: %v (got % X)", err, got)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("request = % X, want % X", got, want)
	}
}

// Start opens the serial port and runs the handshake over it: the init
// request goes out on the wire, each reply is answered with the next
// request, and a data frame becomes the lambda reading.
func TestStartHandshakeOverSerial(t *testing.T) {
	t.Parallel()
	master, slave := openPTY(t)
	rec := newLogRec()
	a, _ := NewSTAGClient(slave, rec.log)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start(%s): %v", slave, err)
	}
	t.Cleanup(a.Stop) // runs before the master is closed

	expectRead(t, master, initRequest)
	master.Write(frame(0x80))
	expectRead(t, master, reqAfter80)
	master.Write(dataFrame(0x02, 1000, 209))
	expectRead(t, master, reqPoll)
	if got := a.GetLambda(); got != 1 {
		t.Errorf("lambda = %v, want 1", got)
	}
}

// After the port drops, run reopens it and starts a fresh handshake there.
func TestRunReopensPortAfterDrop(t *testing.T) {
	t.Parallel()
	master, slave := openPTY(t)
	rec := newLogRec()
	a, _ := NewSTAGClient(slave, rec.log)
	f := newFakePort()
	a.sp = f
	exited := startRun(context.Background(), a)
	defer func() {
		a.Stop()
		waitClosed(t, exited, "run to return after Stop")
	}()

	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("first request = % X, want init", got)
	}
	f.feed(dataSample[:10])
	f.fail(errors.New("device gone"))

	expectRead(t, master, initRequest)
	if rec.count("Stag: reconnected to "+slave) != 1 {
		t.Errorf("logs %q, want one reconnect to %s", rec.all(), slave)
	}
	if n := f.closeCount(); n != 1 {
		t.Errorf("dropped port closed %d times, want 1", n)
	}
	master.Write(dataSample)
	expectRead(t, master, reqPoll)
	if got := a.GetLambda(); !near(got, 40.228) {
		t.Errorf("lambda = %v, want 40.228", got)
	}
}

// While the device is missing run retries every reconnectDelay, and once it
// shows up again (here a symlink to the pty appearing, like a USB adapter
// being plugged back in) it opens it and starts the handshake.
func TestRunRetriesUntilPortAppears(t *testing.T) {
	t.Parallel()
	master, slave := openPTY(t)
	port := missingPort(t)
	rec := newLogRec()
	a, _ := NewSTAGClient(port, rec.log)
	exited := startRun(context.Background(), a)
	defer func() {
		a.Stop()
		waitClosed(t, exited, "run to return after Stop")
	}()

	rec.waitFor(t, "Stag: reconnect failed: ")
	if err := os.Symlink(slave, port); err != nil {
		t.Fatal(err)
	}
	expectRead(t, master, initRequest)
	if rec.count("Stag: reconnecting to "+port) < 2 || rec.count("Stag: reconnected to "+port) != 1 {
		t.Errorf("logs %q, want a failed attempt, a retry and one reconnect", rec.all())
	}
}
