package plx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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

// waitFor polls cond until it holds, failing the test after five seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !cond() {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for %s", what)
		case <-tick.C:
		}
	}
}

// Start reads frames off the port byte by byte, carries on past a malformed
// frame, and closes the port when the context ends.
func TestStartReadsSerialPort(t *testing.T) {
	master, slave := openPTY(t)
	c, _ := NewIMFDClient(slave, nil, func(string) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	in := append([]byte{StartBit, 0x00, 0x00, 0x00, StopBit}, plxFrame(plxPacket(WidebandAirFuel, 0, 120), plxPacket(RPM, 0, 100))...)
	if _, err := master.Write(in); err != nil {
		t.Fatalf("write to pty: %v", err)
	}
	waitFor(t, "the RPM reading", func() bool { return c.GetSensor(RPM, 0) == 1955 })
	if got := c.GetLambda(); !near(got, 1.0) {
		t.Errorf("GetLambda = %v, want 1.0", got)
	}

	cancel()
	waitSlaveClosed(t, master)
}

// Stop ends the reader and releases the port even while the context lives on.
func TestStopReleasesPort(t *testing.T) {
	master, slave := openPTY(t)
	c, _ := NewIMFDClient(slave, nil, func(string) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	c.Stop()
	waitSlaveClosed(t, master)
}

// Losing the device is reported through the client's log function, the one
// txlogger shows to the user.
func TestReadErrorLogged(t *testing.T) {
	master, slave := openPTY(t)
	var mu sync.Mutex
	var lines []string
	c, _ := NewIMFDClient(slave, nil, func(s string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, s)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	master.Close() // unplug
	waitFor(t, "the read error in the client log", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, l := range lines {
			if strings.Contains(l, "Port has been closed") {
				return true
			}
		}
		return false
	})
}
