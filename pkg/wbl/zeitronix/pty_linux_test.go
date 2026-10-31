package zeitronix

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

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
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	t.Cleanup(func() { master.Close() })
	return master, fmt.Sprintf("/dev/pts/%d", n)
}

// Start opens the serial port and decodes the ZT-2 stream arriving on it.
func TestStartReadsSerialStream(t *testing.T) {
	t.Parallel()
	master, slave := openPTY(t)
	z, _ := NewZeitronixClient(slave, func(string) {})
	if err := z.Start(context.Background()); err != nil {
		t.Fatalf("Start(%s): %v", slave, err)
	}
	t.Cleanup(z.Stop) // runs before the master is closed

	// Written after Start, so the port is already in raw mode.
	master.Write(slices.Concat([]byte{0x55, 0x00}, packet(147, 850, 3000, 120), packet(130, 800, 4000, 180)))
	const want = "Lambda: 0.884, EGT: 800, RPM: 4000, MAP: 180"
	eventually(t, want, func() bool { return z.String() == want })
}

// After a read error the handler reopens the port and starts a fresh packet
// there, so the half packet read before the drop cannot shift the next one.
func TestSerialHandlerReopensPortAfterDrop(t *testing.T) {
	t.Parallel()
	master, slave := openPTY(t)
	rec := newLogRec()
	z, _ := NewZeitronixClient(slave, rec.log)
	f := newFakePort()
	z.p = f
	exited := startHandler(context.Background(), z)
	defer func() {
		z.Stop()
		waitClosed(t, exited, "serialHandler to return after Stop")
	}()

	f.feed(packet(100, 1, 2, 3)[:6])
	f.fail(errors.New("device gone"))
	rec.waitFor(t, "Zeitronix: reconnected to "+slave)
	if n := f.closeCount(); n != 1 {
		t.Errorf("dropped port closed %d times, want 1", n)
	}

	master.Write(packet(147, 850, 3000, 120))
	const want = "Lambda: 1.000, EGT: 850, RPM: 3000, MAP: 120"
	eventually(t, want, func() bool { return z.String() == want })
}

// While the device is missing the handler retries every reconnectDelay, and
// once it shows up again (a symlink to the pty appearing, like a USB adapter
// being plugged back in) it opens it and reads from it.
func TestSerialHandlerRetriesUntilPortAppears(t *testing.T) {
	t.Parallel()
	master, slave := openPTY(t)
	port := missingPort(t)
	rec := newLogRec()
	z, _ := NewZeitronixClient(port, rec.log)
	exited := startHandler(context.Background(), z)
	defer func() {
		z.Stop()
		waitClosed(t, exited, "serialHandler to return after Stop")
	}()

	rec.waitFor(t, "Zeitronix: reconnect failed: ")
	if err := os.Symlink(slave, port); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "Zeitronix: reconnected to "+port)
	if rec.count("Zeitronix: reconnecting to "+port) < 2 {
		t.Errorf("logs %q, want a failed attempt and a retry", rec.all())
	}
	master.Write(packet(147, 0, 0, 0))
	eventually(t, "lambda 1", func() bool { return z.GetLambda() == 1 })
}
