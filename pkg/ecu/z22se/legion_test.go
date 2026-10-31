package z22se

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/roffe/gocan/v2"
)

// legionECU returns a fake Z22SE already running the Legion loader with
// img in flash.
func legionECU(img []byte) *fakeZ22 {
	f := newFake(img)
	f.legion = true
	return f
}

// countReads counts the loader reads (0x21) the tester sent.
func (s *fakeZ22) countReads() int {
	n := 0
	s.with(func() {
		for _, f := range s.sent {
			if f.ID == 0x7E0 && f.Length == 8 && f.Data[1] == 0x21 {
				n++
			}
		}
	})
	return n
}

// DumpECU reads the whole 1 MiB flash through the loader, including the
// blocks it reports as erased, after setting the inter frame latency, and
// checks it against the loader's md5 of the full image.
func TestDumpECU(t *testing.T) {
	img := flashImage()
	f := legionECU(img)
	c, rec := newClient(t, f)
	bin, err := c.DumpECU(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, img) {
		t.Fatal("dump differs from the flash content")
	}
	f.with(func() {
		if !slices.Equal(f.demands, []string{"0:200", "2:0"}) {
			t.Errorf("demands = %v, want the latency, then the full image md5", f.demands)
		}
	})
	for _, m := range []string{"Dumping ECU", "Verifying md5..", "Remote MD5 : ", "Local MD5  : ", "Done, took: "} {
		if !rec.hasMessage(m) {
			t.Errorf("no %q message in %q", m, rec.msgs)
		}
	}
}

// DumpECU rejects a dump whose md5 does not match the loader's.
func TestDumpECUMD5Mismatch(t *testing.T) {
	f := legionECU(flashImage())
	f.md5Lie = true
	c, _ := newClient(t, f)
	bin, err := c.DumpECU(testCtx(t))
	if err == nil || err.Error() != "md5 Verification failed" {
		t.Fatalf("DumpECU error = %v, want an md5 verification failure", err)
	}
	if bin != nil {
		t.Fatalf("DumpECU returned %d bytes with the error", len(bin))
	}
}

// DumpECU returns the bootstrap error when the loader cannot be started,
// without going on to the dump.
func TestDumpECUBootstrapFails(t *testing.T) {
	f := newFake(flashImage()) // stock firmware that ignores the bootstrap
	c, rec := newClient(t, f)
	if _, err := c.DumpECU(testCtx(t)); err == nil {
		t.Fatal("DumpECU succeeded without a loader")
	}
	// The read loop itself refuses to run without a loader, so the
	// "Dumping ECU" message is what shows DumpECU stopped on its own.
	if rec.hasMessage("Dumping ECU") {
		t.Fatalf("went on to dump: %q", rec.msgs)
	}
	if n := f.countReads(); n != 0 {
		t.Fatalf("sent %d reads", n)
	}
}

// FlashECU refuses anything but a 1 MiB image before touching the bus.
func TestFlashECUWrongSize(t *testing.T) {
	f := legionECU(flashImage())
	c, _ := newClient(t, f)
	for _, n := range []int{0, 0x100000 - 1, 0x100001, 0x40100} {
		if err := c.FlashECU(testCtx(t), make([]byte, n)); err == nil || err.Error() != "err: Invalid Z22SE file size" {
			t.Errorf("FlashECU(%d bytes) = %v", n, err)
		}
	}
	f.with(func() {
		if len(f.sent) != 0 {
			t.Errorf("sent %d frames", len(f.sent))
		}
	})
}

// FlashECU erases and rewrites exactly the partitions whose md5 differs,
// boot included (the Z22SE always considers every partition), then
// verifies them.
func TestFlashECU(t *testing.T) {
	old := flashImage()
	img := flashImage()
	img[0x10] ^= 0xFF   // partition 1, boot
	img[0x30000] = 0x00 // partition 5
	img[0x30081] = 0x42 // partition 5, second block
	f := legionECU(old)
	c, rec := newClient(t, f)
	if err := c.FlashECU(testCtx(t), img); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if !bytes.Equal(f.flash, img) {
			t.Error("flash differs from the image")
		}
		if !slices.Equal(f.erases, []uint16{0x0011}) {
			t.Errorf("erase masks = %04X, want partitions 1 and 5 (0011)", f.erases)
		}
		for _, a := range f.writes {
			if (a >= 0x4000 && a < 0x20000) || a >= 0x40000 {
				t.Errorf("wrote block %X outside partitions 1 and 5", a)
			}
		}
		md5s := slices.DeleteFunc(slices.Clone(f.demands), func(d string) bool { return d == "0:200" })
		want := []string{"2:1", "2:2", "2:3", "2:4", "2:5", "2:6", "2:7", "2:8", "2:9", "2:1", "2:5"}
		if !slices.Equal(md5s, want) {
			t.Errorf("md5 demands = %v, want every partition then the two written", md5s)
		}
	})
	if !rec.hasMessage("Verifying md5: sucess") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// FlashECU does nothing when every partition already matches.
func TestFlashECUUpToDate(t *testing.T) {
	img := flashImage()
	f := legionECU(img)
	c, rec := newClient(t, f)
	if err := c.FlashECU(testCtx(t), img); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if len(f.erases) != 0 || len(f.writes) != 0 {
			t.Errorf("erases %v, writes %d", f.erases, len(f.writes))
		}
	})
	if !rec.hasMessage("Noting to flash") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// FlashECU fails when the written partitions do not verify.
func TestFlashECUVerifyFails(t *testing.T) {
	img := flashImage()
	img[0x30000] = 0
	f := legionECU(flashImage())
	f.dropWrites = true
	c, _ := newClient(t, f)
	if err := c.FlashECU(testCtx(t), img); err == nil || err.Error() != "failed md5 verification" {
		t.Fatalf("FlashECU = %v, want an md5 verification failure", err)
	}
}

// FlashECU returns the bootstrap error when the loader cannot be started,
// before comparing partitions or erasing.
func TestFlashECUBootstrapFails(t *testing.T) {
	f := newFake(flashImage())
	c, rec := newClient(t, f)
	if err := c.FlashECU(testCtx(t), flashImage()); err == nil {
		t.Fatal("FlashECU succeeded without a loader")
	}
	if rec.hasMessage("Determine Partition Mask") {
		t.Fatalf("went on to compare partitions: %q", rec.msgs)
	}
	f.with(func() {
		for _, fr := range f.sent {
			if fr.Data[1] == 0x34 && fr.Data[2] == 0x01 {
				t.Fatalf("sent an erase: %s", fr)
			}
		}
	})
}

// EraseECU needs the loader already running; then it erases every
// partition of the main flash.
func TestEraseECU(t *testing.T) {
	f := legionECU(flashImage())
	c, _ := newClient(t, f)
	ctx := testCtx(t)
	if err := c.EraseECU(ctx); err == nil || !strings.Contains(err.Error(), "legion not running") {
		t.Fatalf("EraseECU before bootstrap = %v", err)
	}
	if err := c.legion.Bootstrap(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := c.EraseECU(ctx); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if !slices.Equal(f.erases, []uint16{0xFFFF}) {
			t.Errorf("erase masks = %04X, want FFFF", f.erases)
		}
		if !allFF(f.flash) {
			t.Error("flash not erased")
		}
	})
}

// ResetECU is a no-op until the loader runs; then it exits the loader.
func TestResetECU(t *testing.T) {
	f := legionECU(flashImage())
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
	if err := c.ResetECU(ctx); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if f.exits != 1 || f.legion {
			t.Fatalf("exits = %d, loader running %v", f.exits, f.legion)
		}
		if last := f.sent[len(f.sent)-1]; last != gocan.NewFrame(0x7E0, []byte{0x01, 0x20}) {
			t.Fatalf("last frame %s, want the exit command", last)
		}
	})
}

// ResetECU tries the exit three times before giving up.
func TestResetECUExitRefused(t *testing.T) {
	f := legionECU(flashImage())
	f.exitNRC = 0x22
	c, _ := newClient(t, f)
	ctx := testCtx(t)
	if err := c.legion.Bootstrap(ctx, true); err != nil {
		t.Fatal(err)
	}
	err := c.ResetECU(ctx)
	if err == nil || !strings.HasPrefix(err.Error(), "failed to exit legion: ") {
		t.Fatalf("ResetECU = %v", err)
	}
	f.with(func() {
		if f.exits != 3 {
			t.Fatalf("exit sent %d times, want 3", f.exits)
		}
	})
}
