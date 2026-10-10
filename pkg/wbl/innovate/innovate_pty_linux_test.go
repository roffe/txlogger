package innovate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// waitSlaveClosed fails the test unless every handle on the pty's slave side
// gets closed: master reads then fail with EIO.
func waitSlaveClosed(t *testing.T, master *os.File) {
	t.Helper()
	master.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer master.SetReadDeadline(time.Time{})
	buf := make([]byte, 64)
	for {
		_, err := master.Read(buf)
		switch {
		case err == nil:
		case errors.Is(err, unix.EIO):
			return
		default:
			t.Fatalf("serial port not closed: %v", err)
		}
	}
}

func writeMaster(t *testing.T, master *os.File, b []byte) {
	t.Helper()
	if _, err := master.Write(b); err != nil {
		t.Fatalf("write to pty: %v", err)
	}
}

// Start opens the port at 19200 and decodes whatever the wideband streams,
// split however the reads fall; Stop closes the port.
func TestStartReadsSerialPort(t *testing.T) {
	master, slave := openPTY(t)
	var rec logRecorder
	c, _ := NewISP2Client(slave, rec.log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	p := isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(991))
	writeMaster(t, master, p[:7])
	writeMaster(t, master, p[7:])
	waitFor(t, "λ 0.991", func() bool { return c.GetLambda() == 0.991 })

	c.Stop()
	waitSlaveClosed(t, master)
	if rec.count("Starting ISP2 client") != 1 || rec.count("Stopping ISP2 client") != 1 {
		t.Errorf("log = %q, want one Starting and one Stopping line", rec.all())
	}
}

// When the port drops, run reopens it, discards the half-read packet from
// before the drop and keeps decoding.
func TestRunReconnectsAfterPortDrop(t *testing.T) {
	master, slave := openPTY(t)
	var rec logRecorder
	c, _ := NewISP2Client(slave, rec.log)
	if err := c.openPort(); err != nil {
		t.Fatalf("openPort: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exited := make(chan struct{})
	go func() { c.run(ctx); close(exited) }()
	defer func() {
		c.Stop()
		<-exited
	}()

	normal := statusWord(ISP2_NORMAL, 147)
	writeMaster(t, master, isp2Packet(7, normal, lambdaWord(991)))
	waitFor(t, "λ 0.991", func() bool { return c.GetLambda() == 0.991 })

	// Header and status word of a λ 1.2 packet, then the port drops.
	cut := isp2Packet(7, normal, lambdaWord(1200))
	writeMaster(t, master, cut[:4])
	waitFor(t, "the partial packet to be buffered", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.syncBuffer) == 4
	})
	c.closePort()
	waitFor(t, "the reconnect", func() bool { return rec.count("isp2: reconnected to "+slave) == 1 })

	// The rest of the cut packet, then a status-only packet (multiplier 6.4)
	// to mark the end of the stream.
	marker := statusWord(ISP2_NORMAL, 64)
	writeMaster(t, master, append(cut[4:], 0xB2, 0x81, marker[0], marker[1]))
	waitFor(t, "the marker packet", func() bool { return near(c.GetAFRMultiplier(), 6.4) })
	if got := c.GetLambda(); got != 0.991 {
		t.Errorf("lambda = %v, want 0.991: the packet cut by the drop was stitched to bytes read after it", got)
	}
}

// With no port open run keeps retrying, one attempt per reconnectDelay, and
// starts decoding once the device node appears.
func TestRunOpensPortWhenDeviceAppears(t *testing.T) {
	master, slave := openPTY(t)
	var rec logRecorder
	// The device node shows up later as a symlink to the pty slave.
	node := filepath.Join(t.TempDir(), "ttyUSB0")
	c, _ := NewISP2Client(node, rec.log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exited := make(chan struct{})
	go func() { c.run(ctx); close(exited) }()
	defer func() {
		c.Stop()
		<-exited
	}()

	waitFor(t, "a failed open", func() bool { return rec.count("isp2: reconnect failed") > 0 })
	if err := os.Symlink(slave, node); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	waitFor(t, "the retry to open the port", func() bool { return rec.count("isp2: reconnected to "+node) == 1 })

	writeMaster(t, master, isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(991)))
	waitFor(t, "λ 0.991", func() bool { return c.GetLambda() == 0.991 })
}

// When the device disappears run logs the read error, closes the port and
// keeps retrying until stopped.
func TestRunRetriesUntilStopped(t *testing.T) {
	master, slave := openPTY(t)
	var rec logRecorder
	c, _ := NewISP2Client(slave, rec.log)
	if err := c.openPort(); err != nil {
		t.Fatalf("openPort: %v", err)
	}
	// The device node goes away with the device. A path that never exists
	// stands in for it, as the freed pts number could be handed to another
	// test's pty.
	gone := filepath.Join(t.TempDir(), "ttyUSB9")
	c.port = gone
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exited := make(chan struct{})
	go func() { c.run(ctx); close(exited) }()

	master.Close() // unplug
	waitFor(t, "a failed reconnect", func() bool { return rec.count("isp2: reconnect failed") > 0 })
	if rec.count("isp2: reconnecting to "+gone) == 0 {
		t.Errorf("no reconnect attempt logged: %q", rec.all())
	}
	if rec.count("isp2: Port has been closed") != 1 {
		t.Errorf("read error not logged: %q", rec.all())
	}
	if c.getPort() != nil {
		t.Error("port still stored after the device went away")
	}

	c.Stop()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after Stop")
	}
}
