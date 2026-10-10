package t5

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
)

const dirtyNotice = "FLASH is NOT intact"

// FlashECU from a cold ECU uploads MyBooty, picks the start address from
// the chip id, erases, programs every non-blank 0x80 block and verifies:
// T5.2 and T5.5 bins on their own boxes, and a T5.2 bin written twice back
// to back on a T5.5 box.
func TestFlashECU(t *testing.T) {
	tests := []struct {
		name      string
		chip      chip
		binSize   int
		romOffset uint32
		copies    int
	}{
		{"T5.5 bin on T5.5", amd28F010, 0x40000, 0x40000, 1},
		{"T5.2 bin on T5.2", intel28F512, 0x20000, 0x60000, 1},
		{"T5.2 bin on T5.5", amd28F010, 0x20000, 0x60000, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeT5(tt.chip)
			// Old content that must be gone after the flash.
			f.load(bytes.Repeat([]byte{0x5A}, 0x80000-int(f.flashStart)))
			cl, ev := newClient(t, f)
			bin := buildBin(t, tt.binSize, tt.romOffset)

			if err := cl.FlashECU(context.Background(), bin); err != nil {
				t.Fatal(err)
			}

			image := bytes.Repeat(bin, tt.copies)
			if got := f.flash(f.flashStart, len(image)); !bytes.Equal(got, image) {
				t.Fatal("flash content differs from the image")
			}
			// Only the code block(s) and footer block(s) are non-blank.
			blocks := 0
			for off := 0; off < len(image); off += blockSize {
				if !bytes.Equal(image[off:off+blockSize], bytes.Repeat([]byte{0xFF}, blockSize)) {
					blocks++
				}
			}
			cmds := f.commands()
			c0 := bytes.IndexByte(cmds, 0xC0)
			if c0 < 0 || bytes.Count(cmds, []byte{0xC0}) != 1 {
				t.Fatalf("want exactly one erase, commands % X", cmds)
			}
			if n := bytes.Count(cmds[c0:], []byte{0xA5}); n != blocks {
				t.Fatalf("programmed %d blocks after erase, want %d non-blank blocks", n, blocks)
			}
			if bytes.Contains(cmds[c0:], []byte{0xC2}) {
				t.Fatal("reset sent during the flash")
			}
			first, last := ev.lastProgress()
			if first != -float64(len(image)) || last != float64(len(image)) {
				t.Fatalf("progress %v..%v, want %d..%d", first, last, -len(image), len(image))
			}
			if got := ev.hasMessage("two copies"); got != (tt.copies == 2) {
				t.Fatalf("mirror notice = %v, want %v", got, tt.copies == 2)
			}
			if !ev.hasMessage("FLASH checksum verified") {
				t.Fatalf("messages %q lack the verify notice", ev.messages)
			}
		})
	}
}

// RecoverECU works on an ECU left with MyBooty running and an erased flash
// after a failed flash: the client finds the running loader, plans from the
// chip id alone and flashes.
func TestRecoverECU(t *testing.T) {
	f := newFakeT5(amd28F010)
	f.booty = true
	cl, ev := newClient(t, f)
	bin := buildBin(t, 0x40000, 0x40000)
	if err := cl.RecoverECU(context.Background(), bin); err != nil {
		t.Fatal(err)
	}
	if !ev.hasMessage("Bootloader already running") {
		t.Fatalf("messages %q", ev.messages)
	}
	if len(f.frames(0xC1)) != 0 {
		t.Fatal("jumped to 0x5000 while MyBooty was running")
	}
	if !bytes.Equal(f.flash(0x40000, len(bin)), bin) {
		t.Fatal("flash content differs from the bin")
	}
}

// Problems found before erasing restart the ECU so it keeps running its
// firmware, and the flash is left untouched.
func TestFlashECURefusesBeforeErase(t *testing.T) {
	badSum := buildBin(t, 0x40000, 0x40000)
	badSum[0x10] ^= 0xFF
	tests := []struct {
		name    string
		chip    chip
		bin     []byte
		wantErr string
	}{
		{"T5.5 bin on T5.2 box", intel28F512, buildBin(t, 0x40000, 0x40000), "a T5.2 box needs a 0x20000 byte bin"},
		{"odd size on T5.5 box", amd28F010, make([]byte, 0x30000), "a T5.5 box needs a 0x20000 or 0x40000 byte bin"},
		{"unknown chip", chip{0x01, 0x42}, buildBin(t, 0x40000, 0x40000), "unknown FLASH chip type"},
		{"bad checksum", amd28F010, badSum, "refusing to flash: bin checksum mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, tt.chip)
			old := bytes.Repeat([]byte{0x5A}, 0x80000-int(f.flashStart))
			f.load(old)
			err := cl.FlashECU(context.Background(), tt.bin)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), dirtyNotice) {
				t.Fatalf("intact ECU reported as dirty: %v", err)
			}
			cmds := f.commands()
			if bytes.IndexByte(cmds, 0xC0) >= 0 || bytes.IndexByte(cmds, 0xA5) >= 0 {
				t.Fatalf("erased or programmed anyway: % X", cmds)
			}
			if cmds[len(cmds)-1] != 0xC2 {
				t.Fatalf("ECU not reset after refusal: % X", cmds)
			}
			if !bytes.Equal(f.flash(f.flashStart, len(old)), old) {
				t.Fatal("flash changed")
			}
		})
	}
}

// FlashECU passes on upload and chip id failures without erasing.
func TestFlashECUSetupErrors(t *testing.T) {
	t.Run("upload", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.setHook(func(d [8]byte) ([]byte, bool) { return echoMismatch, true })
		if err := cl.FlashECU(context.Background(), buildBin(t, 0x40000, 0x40000)); err == nil {
			t.Fatal("flashed without a bootloader")
		}
	})
	t.Run("chip id", func(t *testing.T) {
		cl, f, _ := newBootyClient(t, amd28F010)
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xC9 {
				return echoMismatch, true
			}
			return nil, false
		})
		err := cl.FlashECU(context.Background(), buildBin(t, 0x40000, 0x40000))
		if err == nil || !strings.Contains(err.Error(), "failed to get chip types") {
			t.Fatalf("err = %v", err)
		}
		if bytes.IndexByte(f.commands(), 0xC0) >= 0 {
			t.Fatal("erased without knowing the chip")
		}
	})
}

// Failures once erasing has started never reset the ECU (the SRAM loader is
// the only way back) and carry the "do not power off" notice.
func TestFlashECUFailuresAfterErase(t *testing.T) {
	tests := []struct {
		name    string
		chip    chip
		binSize int
		romOff  uint32
		hook    func(f *fakeT5) func(d [8]byte) ([]byte, bool)
		wantErr string
	}{
		{
			name: "erase fails", chip: amd28F010, binSize: 0x40000, romOff: 0x40000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				return func(d [8]byte) ([]byte, bool) {
					if d[0] == 0xC0 {
						return []byte{0xC0, 0x03}, true
					}
					return nil, false
				}
			},
			wantErr: "unrecognised FLASH chips, unknown make",
		},
		{
			name: "chip fails to program", chip: amd28F010, binSize: 0x40000, romOff: 0x40000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				return func(d [8]byte) ([]byte, bool) {
					if d[0] == 0x7E { // last frame of a block
						return []byte{0x7E, 0x01, 0x00, 0x04, 0x00, 0x00}, true
					}
					return nil, false
				}
			},
			wantErr: "flashing failed after 0x0 bytes: bootloader failed to program FLASH at 0x040000",
		},
		{
			name: "ECU checksum mismatch", chip: amd28F010, binSize: 0x40000, romOff: 0x40000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				return func(d [8]byte) ([]byte, bool) {
					if d[0] == 0xC8 {
						return []byte{0xC8, 0x01}, true
					}
					return nil, false
				}
			},
			wantErr: "post-flash verification failed: ECU reports FLASH checksum mismatch",
		},
		{
			name: "ECU checksum differs from bin", chip: amd28F010, binSize: 0x40000, romOff: 0x40000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				return func(d [8]byte) ([]byte, bool) {
					if d[0] == 0xC8 {
						return []byte{0xC8, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}, true
					}
					return nil, false
				}
			},
			wantErr: "ECU checksum DEADBEEF",
		},
		{
			// C8 only covers the upper copy of a mirrored T5.2 bin: a bad
			// lower copy (the reset vectors) must be caught by read-back.
			name: "lower mirror copy corrupt", chip: amd28F010, binSize: 0x20000, romOff: 0x60000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				return func(d [8]byte) ([]byte, bool) {
					if d[0] == 0xC8 {
						f.mem[0x40001] ^= 0x01
					}
					return nil, false
				}
			},
			wantErr: "post-flash read-back mismatch at 0x040000",
		},
		{
			name: "footer block corrupt", chip: amd28F010, binSize: 0x40000, romOff: 0x40000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				return func(d [8]byte) ([]byte, bool) {
					if d[0] == 0xC8 {
						f.mem[0x7FF80] ^= 0x01 // outside both the C8 range and the stored sum
					}
					return nil, false
				}
			},
			wantErr: "post-flash read-back mismatch at 0x07FF80",
		},
		{
			name: "read-back fails", chip: amd28F010, binSize: 0x40000, romOff: 0x40000,
			hook: func(f *fakeT5) func(d [8]byte) ([]byte, bool) {
				verified := false
				return func(d [8]byte) ([]byte, bool) {
					verified = verified || d[0] == 0xC8
					if verified && d[0] == 0xC7 {
						return echoMismatch, true
					}
					return nil, false
				}
			},
			wantErr: "post-flash read-back at 0x040000 failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, tt.chip)
			f.setHook(tt.hook(f))
			err := cl.FlashECU(context.Background(), buildBin(t, tt.binSize, tt.romOff))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), dirtyNotice) {
				t.Fatalf("error lacks the dirty flash notice: %v", err)
			}
			if bytes.IndexByte(f.commands(), 0xC2) >= 0 {
				t.Fatal("ECU reset after erase")
			}
		})
	}
}

// A data frame with a programming error is not retried (MyBooty already
// retried internally) and aborts the whole flash.
func TestWriteBlockNoRetryOnProgramError(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.setHook(func(d [8]byte) ([]byte, bool) {
		if d[0] == 0x7E {
			return []byte{0x7E, 0x01, 0x00, 0x04, 0x00, 0x00}, true
		}
		return nil, false
	})
	block := make([]byte, blockSize)
	if err := cl.writeBlock(context.Background(), 0x40000, block); err == nil {
		t.Fatal("program error ignored")
	}
	if n := len(f.frames(0x7E)); n != 1 {
		t.Fatalf("sent the failing frame %d times, want once", n)
	}
}

// writeBlock frames a 0x80 byte block as A5 <address> 80 followed by
// nineteen data frames carrying offset + seven bytes (two in the last one);
// an all-0xFF block sends nothing.
func TestWriteBlockWire(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	if err := cl.writeBlock(context.Background(), 0x40000, bytes.Repeat([]byte{0xFF}, blockSize)); err != nil {
		t.Fatal(err)
	}
	if n := len(f.commands()); n != 0 {
		t.Fatalf("blank block sent %d frames", n)
	}

	block := make([]byte, blockSize)
	for i := range block {
		block[i] = byte(0x80 + i)
	}
	if err := cl.writeBlock(context.Background(), 0x7FF80, block); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	sent := append([]byte(nil), f.sent[0].Data[:]...)
	frames := f.sent[1:]
	f.mu.Unlock()
	if want := []byte{0xA5, 0x00, 0x07, 0xFF, 0x80, 0x80, 0x00, 0x00}; !bytes.Equal(sent, want) {
		t.Fatalf("address frame % X, want % X", sent, want)
	}
	if len(frames) != 19 {
		t.Fatalf("sent %d data frames, want 19", len(frames))
	}
	for i, fr := range frames {
		off := i * 7
		if fr.Data[0] != byte(off) {
			t.Fatalf("frame %d offset %02X, want %02X", i, fr.Data[0], off)
		}
		n := min(7, blockSize-off)
		if !bytes.Equal(fr.Data[1:1+n], block[off:off+n]) {
			t.Fatalf("frame %d data % X, want % X", i, fr.Data[1:1+n], block[off:off+n])
		}
	}
	if !bytes.Equal(f.flash(0x7FF80, blockSize), block) {
		t.Fatal("block not programmed")
	}
}

// writeBlock retries a data frame after a transport error, reporting the
// retry, and still programs the block.
func TestWriteBlockRetriesTransportErrors(t *testing.T) {
	cl, f, ev := newBootyClient(t, amd28F010)
	failed := false
	f.setHook(func(d [8]byte) ([]byte, bool) {
		if d[0] == 0x15 && !failed {
			failed = true
			return echoMismatch, true
		}
		return nil, false
	})
	block := bytes.Repeat([]byte{0x42}, blockSize)
	if err := cl.writeBlock(context.Background(), 0x40000, block); err != nil {
		t.Fatal(err)
	}
	if n := len(f.frames(0x15)); n != 2 {
		t.Fatalf("frame 0x15 sent %d times, want 2", n)
	}
	errs := ev.errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "retrying data frame at 0x040000") {
		t.Fatalf("reported errors %v", errs)
	}
	if !bytes.Equal(f.flash(0x40000, blockSize), block) {
		t.Fatal("block not programmed")
	}
}

// writeBlock gives up when the address command keeps failing.
func TestWriteBlockAddressFails(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.setHook(func(d [8]byte) ([]byte, bool) { return []byte{0xA5, 0x01}, true })
	if err := cl.writeBlock(context.Background(), 0x40000, make([]byte, blockSize)); err == nil {
		t.Fatal("rejected address accepted")
	}
	if n := len(f.frames(0xA5)); n != 3 {
		t.Fatalf("sent %d address frames, want 3", n)
	}
	if n := len(f.commands()); n != 3 {
		t.Fatalf("sent data frames without an address: % X", f.commands())
	}
}

// EraseECU sends C0, invalidates the cached footer either way and maps
// MyBooty's failure codes to messages, with the failing address for 01/02.
func TestEraseECU(t *testing.T) {
	tests := []struct {
		name    string
		reply   []byte
		wantErr string
	}{
		{"ok", nil, ""},
		{"cannot erase", []byte{0xC0, 0x01, 0x00, 0x06, 0x12, 0x34}, "unable to erase FLASH chips (address 0x061234)"},
		{"cannot zero", []byte{0xC0, 0x02, 0x00, 0x04, 0x00, 0x00}, "cannot write zeroes to 28F512/010 chips (address 0x040000)"},
		{"unknown AMD", []byte{0xC0, 0x05, 0x00, 0x04, 0x00, 0x00}, "erase failed: unrecognised AMD FLASH chips"},
		{"unknown AMIC", []byte{0xC0, 0x0A}, "erase failed: unrecognised AMIC FLASH chips"},
		{"undocumented", []byte{0xC0, 0x42, 0x01}, "erase failed: C042"},
		{"transport", echoMismatch, "erase failed: response echo mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, ev := newBootyClient(t, amd28F010)
			f.load([]byte{0x12, 0x34})
			cl.footer = []byte{1, 2, 3}
			if tt.reply != nil {
				f.setHook(func(d [8]byte) ([]byte, bool) { return tt.reply, true })
			}
			err := cl.EraseECU(context.Background())
			if cl.footer != nil {
				t.Fatal("footer cache survived the erase")
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "(address") != strings.Contains(tt.wantErr, "(address") {
					t.Fatalf("address reporting wrong: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(f.flash(0x7FFFE, 2), []byte{0xFF, 0xFF}) {
				t.Fatal("flash not erased")
			}
			if !ev.hasMessage("FLASH erased") {
				t.Fatalf("messages %q", ev.messages)
			}
			if want := [8]byte{0xC0}; f.frames(0xC0)[0].Data != want {
				t.Fatalf("erase frame % X", f.frames(0xC0)[0].Data)
			}
		})
	}
}

// GetECUChecksum returns the four checksum bytes MyBooty reports with C8
// when flash matches its stored checksum, and an error when it does not.
func TestGetECUChecksum(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	bin := buildBin(t, 0x40000, 0x40000)
	f.load(bin)
	got, err := cl.GetECUChecksum(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bin[len(bin)-4:]) {
		t.Fatalf("checksum % X, want % X", got, bin[len(bin)-4:])
	}
	if want := [8]byte{0xC8}; f.frames(0xC8)[0].Data != want {
		t.Fatalf("C8 frame % X", f.frames(0xC8)[0].Data)
	}

	bad := append([]byte(nil), bin...)
	binary.BigEndian.PutUint32(bad[len(bad)-4:], 1)
	f.load(bad)
	if _, err := cl.GetECUChecksum(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want mismatch", err)
	}

	f.setHook(func(d [8]byte) ([]byte, bool) { return echoMismatch, true })
	if _, err := cl.GetECUChecksum(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to get ECU checksum") {
		t.Fatalf("err = %v", err)
	}
}

// GetECUChecksum and EraseECU upload the loader first on a cold ECU and
// fail without sending their command when that fails.
func TestBootloaderCommandsNeedUpload(t *testing.T) {
	for _, tt := range []struct {
		cmd byte
		run func(*Client) error
	}{
		{0xC8, func(c *Client) error { _, err := c.GetECUChecksum(context.Background()); return err }},
		{0xC0, func(c *Client) error { return c.EraseECU(context.Background()) }},
		{0xC7, func(c *Client) error { _, err := c.Info(context.Background()); return err }},
		{0xC7, func(c *Client) error { _, err := c.DumpECU(context.Background()); return err }},
	} {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xA5 {
				return []byte{0xA5, 0x01}, true
			}
			return nil, false
		})
		if err := tt.run(cl); err == nil || !strings.Contains(err.Error(), "sendBootloaderAddressCommand") {
			t.Fatalf("%02X: err = %v, want upload failure", tt.cmd, err)
		}
		if bytes.IndexByte(f.commands(), tt.cmd) >= 0 {
			t.Fatalf("sent %02X without a bootloader", tt.cmd)
		}
	}
}
