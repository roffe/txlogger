package wbl

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
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

// On a serial port New opens it and starts the wideband's reader, which
// decodes what the device sends.
func TestNewSerialReaders(t *testing.T) {
	for _, c := range []struct {
		typ  string
		data []byte
		want float64
	}{
		{aem.ProductString, aemLine, 1.03},
		{innovate.ProductString, isp2Packet, 0.991},
		{plx.ProductString, plxPacket, 1.0},
		{zeitronix.ProductString, ztPacket, 1.0},
		{stag.ProductString, stagFrame, 40.228}, // free air, raw 0x9D24 at 0.001/bit
	} {
		t.Run(c.typ, func(t *testing.T) {
			master, slave := openPTY(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p, err := New(ctx, nil, &WBLConfig{WBLType: c.typ, Port: slave, Log: func(string) {}})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Stop()
			if _, err := master.Write(c.data); err != nil {
				t.Fatal(err)
			}
			waitLambda(t, p, c.want)
		})
	}
}
