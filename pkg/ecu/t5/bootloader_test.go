package t5

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/roffe/txlogger/pkg/ecusim"
	"github.com/roffe/txlogger/pkg/srec"
)

// myBootyRecords parses the embedded loader.
func myBootyRecords(t *testing.T) []*srec.Record {
	t.Helper()
	sr := srec.NewSrec()
	if err := sr.Parse(strings.NewReader(MyBooty)); err != nil {
		t.Fatal(err)
	}
	return sr.Records
}

// UploadBootLoader loads every S1 record of MyBooty into RAM at its address
// through the stock firmware's A5 + data frame upload, here against the
// pkg/ecusim T5, and reports progress from -1884 up to the 1884 data bytes.
func TestUploadBootLoaderLoadsRAM(t *testing.T) {
	sim := &ecusim.T5{}
	cl, ev := newClient(t, sim)
	if err := cl.UploadBootLoader(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !cl.bootloaded {
		t.Fatal("bootloaded not set")
	}
	total := 0
	for _, rec := range myBootyRecords(t) {
		if rec.Srectype != "S1" {
			continue
		}
		total += len(rec.Data)
		if got := sim.Peek(rec.Address, len(rec.Data)); !bytes.Equal(got, rec.Data) {
			t.Fatalf("RAM at 0x%04X = % X, want % X", rec.Address, got, rec.Data)
		}
	}
	first, last := ev.lastProgress()
	if first != -float64(total) || last != float64(total) {
		t.Fatalf("progress ran %v..%v, want %d..%d", first, last, -total, total)
	}
	if !ev.hasMessage("Uploading bootloader") || !ev.hasMessage("Done, took") {
		t.Fatalf("messages %q", ev.messages)
	}
}

// The upload is framed as: A5 for S0, then per S1 record an A5 with the
// record address and data length followed by data frames whose first byte
// is the offset (0, 7, 14...), and finally C1 to the S9 entry point 0x5000,
// which is never answered.
func TestUploadBootLoaderWire(t *testing.T) {
	f := newFakeT5(amd28F010)
	cl, _ := newClient(t, f)
	if err := cl.UploadBootLoader(context.Background()); err != nil {
		t.Fatal(err)
	}
	recs := myBootyRecords(t)
	a5 := f.frames(0xA5)
	if len(a5) != len(recs)-1 { // every record but S9
		t.Fatalf("sent %d A5 frames, want %d", len(a5), len(recs)-1)
	}
	if want := [8]byte{0xA5}; a5[0].Data != want {
		t.Fatalf("S0 address frame % X, want % X", a5[0].Data, want)
	}
	if want := [8]byte{0xA5, 0x00, 0x00, 0x50, 0x00, 0x20, 0x00, 0x00}; a5[1].Data != want {
		t.Fatalf("first S1 address frame % X, want % X", a5[1].Data, want)
	}
	cmds := f.commands()
	if want := []byte{0xA5, 0xA5, 0x00, 0x07, 0x0E, 0x15, 0x1C, 0xA5}; !bytes.Equal(cmds[:8], want) {
		t.Fatalf("first frames % X, want % X", cmds[:8], want)
	}
	c1 := f.frames(0xC1)
	if len(c1) != 1 || cmds[len(cmds)-1] != 0xC1 {
		t.Fatalf("C1 frames %v, want exactly one, sent last", c1)
	}
	if want := [8]byte{0xC1, 0x00, 0x00, 0x50, 0x00}; c1[0].Data != want {
		t.Fatalf("C1 frame % X, want % X", c1[0].Data, want)
	}
	if !bytes.Equal(f.flash(0x5000, 0x20), recs[1].Data) {
		t.Fatal("first S1 record not in RAM")
	}
}

// With MyBooty already running (ECU kept powered after a failed flash) the
// loader's flash range check rejects the 0x5000 upload on the last frame of
// the first S1 record; the client takes that as "already running" and stops
// without jumping anywhere.
func TestUploadBootLoaderAlreadyRunning(t *testing.T) {
	f := newFakeT5(amd28F010)
	f.booty = true
	cl, ev := newClient(t, f)
	if err := cl.UploadBootLoader(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !cl.bootloaded {
		t.Fatal("bootloaded not set")
	}
	if !ev.hasMessage("Bootloader already running") {
		t.Fatalf("messages %q", ev.messages)
	}
	if n := len(f.frames(0xC1)); n != 0 {
		t.Fatalf("sent %d C1 frames to a running loader", n)
	}
	if n := len(f.frames(0xA5)); n != 2 {
		t.Fatalf("sent %d A5 frames, want 2 (S0 and the first S1)", n)
	}
}

// Upload failures: an address command rejected three times, a data frame
// with an error status or a broken reply, a jump frame the adapter could not
// send, and a cancel while waiting for MyBooty to start.
func TestUploadBootLoaderErrors(t *testing.T) {
	t.Run("address rejected", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xA5 {
				return []byte{0xA5, 0x01}, true
			}
			return nil, false
		})
		err := cl.UploadBootLoader(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid response to sendBootloaderAddressCommand") {
			t.Fatalf("err = %v", err)
		}
		if n := len(f.frames(0xA5)); n != 3 {
			t.Fatalf("sent %d A5 frames, want 3 attempts", n)
		}
		if cl.bootloaded {
			t.Fatal("bootloaded set after failure")
		}
	})
	t.Run("S1 address rejected", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xA5 && d[3] != 0 { // 0x5000, not the S0 address 0
				return []byte{0xA5, 0x02}, true
			}
			return nil, false
		})
		if err := cl.UploadBootLoader(context.Background()); err == nil {
			t.Fatal("rejected S1 address accepted")
		}
		if n := len(f.frames(0xA5)); n != 4 {
			t.Fatalf("sent %d A5 frames, want S0 + 3 attempts", n)
		}
	})
	t.Run("data frame status", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0x07 {
				return []byte{0x07, 0x02}, true
			}
			return nil, false
		})
		err := cl.UploadBootLoader(context.Background())
		if err == nil || !strings.Contains(err.Error(), "failed to upload bootloader") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("data frame transport", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0x0E {
				return echoMismatch, true
			}
			return nil, false
		})
		err := cl.UploadBootLoader(context.Background())
		if err == nil || !strings.Contains(err.Error(), "echo mismatch") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("jump not sent", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		f.sendErr = func(d [8]byte) error {
			if d[0] == 0xC1 {
				return errors.New("adapter write failed")
			}
			return nil
		}
		err := cl.UploadBootLoader(context.Background())
		if err == nil || !strings.Contains(err.Error(), "failed to start bootloader: adapter write failed") {
			t.Fatalf("err = %v", err)
		}
		if cl.bootloaded {
			t.Fatal("bootloaded set although the jump never reached the ECU")
		}
	})
	t.Run("cancel during start", func(t *testing.T) {
		f := newFakeT5(amd28F010)
		cl, _ := newClient(t, f)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xC1 {
				cancel()
			}
			return nil, false
		})
		if err := cl.UploadBootLoader(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}

// A data frame status of 01 is MyBooty giving up on programming the FLASH;
// it is reported as a flashProgramError carrying the failing address.
func TestSendBootloaderDataCommand(t *testing.T) {
	tests := []struct {
		name    string
		reply   []byte
		wantErr string
		program bool
	}{
		{"ok", []byte{0x07, 0x00}, "", false},
		{"program error", []byte{0x07, 0x01, 0x00, 0x06, 0x00, 0x80}, "bootloader failed to program FLASH at 0x060080", true},
		{"other status", []byte{0x07, 0x05}, "unexpected response to data frame", false},
		{"transport", echoMismatch, "failed to send data frame 07", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, amd28F010)
			f.setHook(func(d [8]byte) ([]byte, bool) { return tt.reply, true })
			err := cl.sendBootloaderDataCommand(context.Background(), []byte{0x07, 1, 2, 3, 4, 5, 6, 7})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			var pe *flashProgramError
			if errors.As(err, &pe) != tt.program {
				t.Fatalf("flashProgramError = %v, want %v", errors.As(err, &pe), tt.program)
			}
		})
	}
}

// A loader image that fails to parse, or a record whose checksum does not
// match, aborts the upload before the jump to 0x5000.
func TestUploadBootLoaderCorruptImage(t *testing.T) {
	orig := MyBooty
	t.Cleanup(func() { MyBooty = orig })
	lines := strings.Split(orig, "\n")
	last := len(lines) - 3 // the S1 record at 0x5764
	flipped := []byte(lines[last])
	flipped[10] ^= 0x01 // one hex digit of the data
	lines[last] = string(flipped)

	tests := []struct {
		name    string
		image   string
		wantErr string
	}{
		{"not srec", "X0garbage", "invalid srectype"},
		{"bad record checksum", strings.Join(lines, "\n"), "does not match calculated CRC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MyBooty = tt.image
			f := newFakeT5(amd28F010)
			cl, _ := newClient(t, f)
			err := cl.UploadBootLoader(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if len(f.frames(0xC1)) != 0 || cl.bootloaded {
				t.Fatal("jumped to a corrupt loader")
			}
		})
	}
}
