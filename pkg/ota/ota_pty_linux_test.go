package ota

import (
	"bytes"
	"fmt"
	"os"
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
	// Non-blocking, so os.File uses the poller and Close unblocks a Read.
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	t.Cleanup(func() { master.Close() })
	return master, fmt.Sprintf("/dev/pts/%d", n)
}

// UpdateOTA on a serial port path (not tcp://) opens it with go.bug.st/serial
// and runs the same protocol as over TCP: version request, the whole embedded
// image in 250 byte frames, then "ok" to the finish.
func TestUpdateOTASerialPort(t *testing.T) {
	master, slave := openPTY(t)
	dev := &fakeBridge{version: "1.1.3", done: make(chan struct{})}
	go func() {
		defer close(dev.done)
		dev.serve(master)
	}()

	var rec recorder
	err := UpdateOTA(rec.cfg(slave))
	if err != nil {
		master.Close() // unblock the device goroutine
	}
	dev.wait(t)
	if err != nil {
		t.Fatalf("UpdateOTA: %v (device: %v)", err, dev.readErr)
	}
	if dev.readErr != nil {
		t.Fatalf("device: %v", dev.readErr)
	}
	if !bytes.Equal(dev.verReq, []byte{'v', 0x01, 0x10, 0x10}) {
		t.Errorf("version request = % X, want 76 01 10 10", dev.verReq)
	}
	if !dev.gotF || !bytes.Equal(dev.image.Bytes(), firmware) {
		t.Errorf("gotF %v after %d of %d bytes, want the whole image then F", dev.gotF, dev.image.Len(), len(firmware))
	}
}
