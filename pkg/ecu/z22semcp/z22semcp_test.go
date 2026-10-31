package z22semcp

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

// legionMCP returns a fake already running the Legion loader with img in
// the MCP flash.
func legionMCP(img []byte) *fakeMCP {
	f := newFake(img)
	f.legion = true
	return f
}

// The client registers as "Z22SE MCP" at 500 kbit/s listening on 0x7E8,
// and ecu.New builds it from that name.
func TestRegistration(t *testing.T) {
	info := ecu.Info("Z22SE MCP")
	if info.CANRate != 500 || !slices.Equal(info.Filter, []uint32{0x7E8}) || info.ManualReset {
		t.Fatalf("registration = %+v", info)
	}
	bus, err := gocan.OpenAdapter(context.Background(), newFake(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c, err := ecu.New(bus, &ecu.Config{Name: "Z22SE MCP"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*Client); !ok {
		t.Fatalf("ecu.New built a %T", c)
	}
}

// The MCP has no DTCs, info, marrying, recovery or separate erase: those
// calls refuse or do nothing, without touching the bus.
func TestUnsupportedOperations(t *testing.T) {
	f := legionMCP(mcpImage())
	c, _ := newClient(t, f)
	ctx := testCtx(t)
	if err := c.MarryECU(ctx, "x"); err == nil || err.Error() != "not supported" {
		t.Errorf("MarryECU = %v", err)
	}
	if err := c.RecoverECU(ctx, nil); err == nil || err.Error() != "not supported" {
		t.Errorf("RecoverECU = %v", err)
	}
	if dtcs, err := c.ReadDTC(ctx); dtcs != nil || err == nil || err.Error() != "MCP cannot do this" {
		t.Errorf("ReadDTC = %v, %v", dtcs, err)
	}
	if res, err := c.Info(ctx); res != nil || err != nil {
		t.Errorf("Info = %v, %v", res, err)
	}
	if err := c.PrintECUInfo(ctx); err != nil {
		t.Errorf("PrintECUInfo = %v", err)
	}
	if err := c.EraseECU(ctx); err != nil {
		t.Errorf("EraseECU = %v", err)
	}
	f.with(func() {
		if len(f.sent) != 0 {
			t.Errorf("sent %v", f.sent)
		}
	})
}

// DumpECU starts the secondary loader, reads the whole MCP flash including
// the shadow and checks it against the loader's MCP md5.
func TestDumpECU(t *testing.T) {
	img := mcpImage()
	f := legionMCP(img)
	c, rec := newClient(t, f)
	bin, err := c.DumpECU(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, img) {
		t.Fatal("dump differs from the MCP flash")
	}
	f.with(func() {
		reads := slices.Index(f.log, "read 5")
		if reads < 0 || !slices.Equal(f.log[:reads], []string{"demand 0:200", "demand 4:0"}) {
			t.Errorf("log = %v, want latency and secondary loader before the reads", f.log)
		}
		if last := f.log[len(f.log)-1]; last != "demand 3:0" {
			t.Errorf("last request %q, want the full MCP md5", last)
		}
	})
	for _, m := range []string{"Start MCP bootloader...", "Dumping MCP", "Remote md5 : ", "Local md5  : ", "Done, took: "} {
		if !rec.hasMessage(m) {
			t.Errorf("no %q message in %q", m, rec.msgs)
		}
	}
}

// DumpECU rejects a dump whose md5 does not match the loader's.
func TestDumpECUMD5Mismatch(t *testing.T) {
	f := legionMCP(mcpImage())
	f.md5Lie = true
	c, _ := newClient(t, f)
	bin, err := c.DumpECU(testCtx(t))
	if err == nil || err.Error() != "md5 Verification failed" || bin != nil {
		t.Fatalf("DumpECU = %d bytes, %v; want an md5 verification failure", len(bin), err)
	}
}

// sentIDemands counts the loader IDemand commands (02 A5) the tester sent.
func (s *fakeMCP) sentIDemands() int {
	n := 0
	s.with(func() {
		for _, f := range s.sent {
			if f.ID == 0x7E0 && f.Length == 8 && f.Data[0] == 0x02 && f.Data[1] == 0xA5 {
				n++
			}
		}
	})
	return n
}

// DumpECU returns the bootstrap error when the loader cannot be started,
// without asking for the secondary loader.
func TestDumpECUBootstrapFails(t *testing.T) {
	f := newFake(mcpImage()) // stock firmware: answers nothing
	c, rec := newClient(t, f)
	if _, err := c.DumpECU(testCtx(t)); err == nil {
		t.Fatal("DumpECU succeeded without a loader")
	}
	if rec.hasMessage("Start MCP bootloader") {
		t.Fatalf("went on to the secondary loader: %q", rec.msgs)
	}
	if n := f.sentIDemands(); n != 0 {
		t.Fatalf("sent %d loader commands", n)
	}
}

// DumpECU returns without dumping when the secondary loader does not
// start. The loader client keeps retrying with a growing backoff, so the
// context bounds the attempt; since every later loader call then gives up
// on the expired context without sending, the missing "Dumping MCP"
// message is what shows DumpECU stopped on its own.
func TestDumpECUSecondaryLoaderFails(t *testing.T) {
	f := legionMCP(mcpImage())
	f.secondaryNAK = true
	c, rec := newClient(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := c.DumpECU(ctx); err == nil {
		t.Fatal("DumpECU succeeded without the secondary loader")
	}
	if got := f.entries("demand 4:"); len(got) == 0 {
		t.Fatal("never asked for the secondary loader")
	}
	if rec.hasMessage("Dumping MCP") {
		t.Fatalf("went on to dump: %q", rec.msgs)
	}
	if got := f.entries("read"); len(got) != 0 {
		t.Fatalf("read the MCP: %v", got)
	}
}

// FlashECU refuses anything but a 0x40100 byte image before touching the
// bus.
func TestFlashECUWrongSize(t *testing.T) {
	f := legionMCP(mcpImage())
	c, _ := newClient(t, f)
	for _, n := range []int{0, 0x40000, mcpSize + 1, 0x100000} {
		if err := c.FlashECU(testCtx(t), make([]byte, n)); err == nil || err.Error() != "err: Invalid Z22SE MCP file size" {
			t.Errorf("FlashECU(%d bytes) = %v", n, err)
		}
	}
	f.with(func() {
		if len(f.sent) != 0 {
			t.Errorf("sent %d frames", len(f.sent))
		}
	})
}

// FlashECU starts the secondary loader, erases and writes the partitions
// whose md5 differs and verifies them. The shadow goes with partition 1:
// here it already matches the image, yet it is erased, rewritten and
// verified because partition 1 differs.
func TestFlashECU(t *testing.T) {
	img := mcpImage()
	old := bytes.Repeat([]byte{0xFF}, mcpSize)
	copy(old[0x40000:], img[0x40000:])
	f := legionMCP(old)
	c, rec := newClient(t, f)
	if err := c.FlashECU(testCtx(t), img); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if !bytes.Equal(f.flash, img) {
			t.Error("MCP flash differs from the image")
		}
		if !slices.Equal(f.erases, []uint16{0x0105}) {
			t.Errorf("erase masks = %04X, want partitions 1, 3 and the shadow (0105)", f.erases)
		}
		start := slices.Index(f.log, "demand 4:0")
		erase := slices.IndexFunc(f.log, func(l string) bool { return strings.HasPrefix(l, "erase") })
		if start < 0 || erase < start {
			t.Errorf("log = %v, want the secondary loader started before the erase", f.log)
		}
	})
	for _, w := range f.entries("write") {
		var a int
		if _, err := fmt.Sscanf(w, "write %X", &a); err != nil {
			t.Fatal(err)
		}
		if !(a < 0x8000 || (a >= 0x10000 && a < 0x18000) || a >= 0x40000) {
			t.Errorf("wrote block %X outside partitions 1, 3 and the shadow", a)
		}
	}
	want := []string{"demand 3:1", "demand 3:2", "demand 3:3", "demand 3:4", "demand 3:5", "demand 3:6", "demand 3:7", "demand 3:8", "demand 3:9", "demand 3:1", "demand 3:3", "demand 3:9"}
	if got := f.entries("demand 3:"); !slices.Equal(got, want) {
		t.Errorf("md5 demands = %v, want every partition then the three written", got)
	}
	if !rec.hasMessage("Verifying md5: sucess") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// FlashECU neither compares nor erases partitions when the secondary
// loader does not start, so device 5 commands never reach the main ECU's
// loader. As in the dump case, the missing "Determine Partition Mask"
// message is what shows FlashECU stopped on its own.
func TestFlashECUSecondaryLoaderFails(t *testing.T) {
	f := legionMCP(bytes.Repeat([]byte{0xFF}, mcpSize))
	f.secondaryNAK = true
	c, rec := newClient(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.FlashECU(ctx, mcpImage()); err == nil {
		t.Fatal("FlashECU succeeded without the secondary loader")
	}
	if got := f.entries("demand 4:"); len(got) == 0 {
		t.Fatal("never asked for the secondary loader")
	}
	if rec.hasMessage("Determine Partition Mask") {
		t.Fatalf("went on to compare partitions: %q", rec.msgs)
	}
	if got := append(f.entries("demand 3:"), f.entries("erase")...); len(got) != 0 {
		t.Fatalf("went on to %v", got)
	}
}

// FlashECU does nothing when every partition already matches.
func TestFlashECUUpToDate(t *testing.T) {
	img := mcpImage()
	f := legionMCP(img)
	c, rec := newClient(t, f)
	if err := c.FlashECU(testCtx(t), img); err != nil {
		t.Fatal(err)
	}
	if got := append(f.entries("erase"), f.entries("write")...); len(got) != 0 {
		t.Errorf("flashed: %v", got)
	}
	if !rec.hasMessage("Noting to flash") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// FlashECU fails when the written partitions do not verify.
func TestFlashECUVerifyFails(t *testing.T) {
	f := legionMCP(bytes.Repeat([]byte{0xFF}, mcpSize))
	f.dropWrites = true
	c, _ := newClient(t, f)
	if err := c.FlashECU(testCtx(t), mcpImage()); err == nil || err.Error() != "failed md5 verification" {
		t.Fatalf("FlashECU = %v, want an md5 verification failure", err)
	}
}

// ResetECU is a no-op until the loader runs; then it sends one exit and
// returns its error.
func TestResetECU(t *testing.T) {
	f := legionMCP(mcpImage())
	c, _ := newClient(t, f)
	ctx := testCtx(t)
	if err := c.ResetECU(ctx); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if len(f.sent) != 0 {
			t.Fatalf("ResetECU without a loader sent %v", f.sent)
		}
	})
	if err := c.legion.Bootstrap(ctx, true); err != nil {
		t.Fatal(err)
	}
	f.with(func() { f.exitNRC = 0x22 })
	if err := c.ResetECU(ctx); err == nil || !strings.HasPrefix(err.Error(), "LegionExit: ") {
		t.Fatalf("ResetECU with the exit refused = %v", err)
	}
	f.with(func() { f.exitNRC = 0 })
	if err := c.ResetECU(ctx); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if f.exits != 2 || f.legion {
			t.Fatalf("exits = %d, loader running %v", f.exits, f.legion)
		}
	})
}
