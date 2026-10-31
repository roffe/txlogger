package t5

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// The package registers itself as "Trionic 5" with the CAN rate and the
// reply IDs the T5 uses, and ecu.New hands back this package's client.
func TestRegistered(t *testing.T) {
	info := ecu.Info("Trionic 5")
	if info.NewFunc == nil {
		t.Fatal("Trionic 5 not registered")
	}
	if info.CANRate != 615.384 {
		t.Errorf("CANRate = %v, want 615.384", info.CANRate)
	}
	for _, id := range []uint32{0x05, 0x0C} {
		found := false
		for _, f := range info.Filter {
			found = found || f == id
		}
		if !found {
			t.Errorf("filter %v lacks 0x%03X", info.Filter, id)
		}
	}
	f := newFakeT5(amd28F010)
	bus, err := gocan.OpenAdapter(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	cl, err := ecu.New(bus, &ecu.Config{Name: "Trionic 5"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cl.(*Client); !ok {
		t.Fatalf("ecu.New returned %T", cl)
	}
}

// command sends the payload on 0x05 and accepts only a full 8-byte reply on
// 0x0C whose first byte echoes the command; anything else is an error.
func TestCommandValidatesReply(t *testing.T) {
	tests := []struct {
		name    string
		reply   []byte // nil = no reply
		short   bool   // deliver a 4-byte frame instead
		wantErr string
	}{
		{name: "echo", reply: []byte{0xC9, 0x00, 1, 2, 3, 4, 5, 6}},
		{name: "short", reply: []byte{0xC9, 0x00, 1, 2}, short: true, wantErr: "short response"},
		{name: "wrong echo", reply: []byte{0xC8, 0x00, 1, 2, 3, 4, 5, 6}, wantErr: "echo mismatch"},
		{name: "silent", wantErr: context.DeadlineExceeded.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeT5(amd28F010)
			cl, _ := newClient(t, f)
			f.setHook(func(d [8]byte) ([]byte, bool) {
				if tt.reply == nil {
					return nil, true
				}
				if tt.short {
					f.bus.Deliver(gocan.NewFrame(0x0C, tt.reply))
					return nil, true
				}
				return tt.reply, true
			})
			got, err := cl.command(context.Background(), []byte{0xC9, 0, 0, 0, 0, 0, 0, 0}, 50*time.Millisecond)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.reply) {
				t.Fatalf("reply = % X, want % X", got, tt.reply)
			}
			sent := f.frames(0xC9)
			if len(sent) != 1 || sent[0].ID != 0x05 || sent[0].Length != 8 {
				t.Fatalf("sent %v, want one 8-byte frame on 0x05", sent)
			}
		})
	}
}

// GetChipTypes asks MyBooty once with C9 and caches the six id bytes; a
// non-zero status is an error and is not cached.
func TestGetChipTypes(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.setHook(func(d [8]byte) ([]byte, bool) {
		if d[0] == 0xC9 {
			f.hook = nil // only the first one fails
			return []byte{0xC9, 0x01}, true
		}
		return nil, false
	})
	if _, err := cl.GetChipTypes(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid GetChipTypes response") {
		t.Fatalf("err = %v, want invalid response", err)
	}
	for range 2 {
		got, err := cl.GetChipTypes(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if want := []byte{0x00, 0x04, 0x00, 0x00, 0x01, 0xA7}; !bytes.Equal(got, want) {
			t.Fatalf("chip types = % X, want % X", got, want)
		}
	}
	if n := len(f.frames(0xC9)); n != 2 {
		t.Fatalf("sent %d C9 requests, want 2 (failed one + one cached)", n)
	}
}

// ReadMemoryByAddress sends C7 with the 32-bit address big endian and
// returns the six bytes ending at it in ascending address order.
func TestReadMemoryByAddress(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.load([]byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15})
	got, err := cl.ReadMemoryByAddress(context.Background(), 0x7FFFF)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15}; !bytes.Equal(got, want) {
		t.Fatalf("read = % X, want % X", got, want)
	}
	if want := [8]byte{0xC7, 0x00, 0x07, 0xFF, 0xFF, 0, 0, 0}; f.frames(0xC7)[0].Data != want {
		t.Fatalf("request = % X, want % X", f.frames(0xC7)[0].Data, want)
	}

	f.setHook(func(d [8]byte) ([]byte, bool) { return []byte{0xC7, 0x01}, true })
	if _, err := cl.ReadMemoryByAddress(context.Background(), 0x7FFFF); err == nil {
		t.Fatal("non-zero status accepted")
	}
}

// readMemoryRetry retries transport errors up to three attempts in total.
func TestReadMemoryRetry(t *testing.T) {
	tests := []struct {
		name     string
		failures int
		wantErr  bool
	}{
		{"recovers after two failures", 2, false},
		{"gives up after three", 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, amd28F010)
			f.load([]byte{1, 2, 3, 4, 5, 6})
			n := 0
			f.setHook(func(d [8]byte) ([]byte, bool) {
				if n < tt.failures {
					n++
					return echoMismatch, true
				}
				return nil, false
			})
			got, err := cl.readMemoryRetry(context.Background(), 0x7FFFF)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !bytes.Equal(got, []byte{1, 2, 3, 4, 5, 6}) {
				t.Fatalf("read = % X", got)
			}
			if sent, want := len(f.frames(0xC7)), min(tt.failures+1, 3); sent != want {
				t.Fatalf("sent %d reads, want %d", sent, want)
			}
		})
	}
}

// readFlash covers [address, address+length) with six-byte reads and aligns
// the final read to the end of the range, so it never reads past it.
func TestReadFlashAlignsFinalRead(t *testing.T) {
	tests := []struct {
		name      string
		address   uint32
		length    int
		wantReads []uint32
	}{
		{"exact multiple", 0x7FFF4, 12, []uint32{0x7FFF9, 0x7FFFF}},
		{"overlapping tail", 0x7FFF3, 13, []uint32{0x7FFF8, 0x7FFFE, 0x7FFFF}},
		{"single read", 0x40000, 6, []uint32{0x40005}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, amd28F010)
			img := make([]byte, 0x40000)
			for i := range img {
				img[i] = byte(i * 13)
			}
			f.load(img)
			got, err := cl.readFlash(context.Background(), tt.address, tt.length)
			if err != nil {
				t.Fatal(err)
			}
			if want := f.flash(tt.address, tt.length); !bytes.Equal(got, want) {
				t.Fatalf("read % X, want % X", got, want)
			}
			var reads []uint32
			for _, fr := range f.frames(0xC7) {
				reads = append(reads, uint32(fr.Data[1])<<24|uint32(fr.Data[2])<<16|uint32(fr.Data[3])<<8|uint32(fr.Data[4]))
			}
			if !equalU32(reads, tt.wantReads) {
				t.Fatalf("read addresses %X, want %X", reads, tt.wantReads)
			}
		})
	}
}

func equalU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ResetECU sends C2, expects status 00 and the 0x08 fill byte, then treats
// the bootloader as gone; a bad reply keeps the bootloaded state.
func TestResetECU(t *testing.T) {
	cl, f, ev := newBootyClient(t, amd28F010)
	f.setHook(func(d [8]byte) ([]byte, bool) { return []byte{0xC2, 0x00, 0x00}, true })
	if err := cl.ResetECU(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid response to reset ECU") {
		t.Fatalf("err = %v, want invalid response", err)
	}
	if !cl.bootloaded {
		t.Fatal("failed reset cleared bootloaded")
	}

	f.setHook(nil)
	if err := cl.ResetECU(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cl.bootloaded {
		t.Fatal("bootloaded still set after reset")
	}
	if !ev.hasMessage("ECU has been reset") {
		t.Fatalf("messages %q lack reset notice", ev.messages)
	}
	if want := [8]byte{0xC2}; f.frames(0xC2)[1].Data != want {
		t.Fatalf("reset frame % X, want % X", f.frames(0xC2)[1].Data, want)
	}

	f.setHook(func(d [8]byte) ([]byte, bool) { return nil, true })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := cl.ResetECU(ctx); err == nil || !strings.Contains(err.Error(), "failed to reset ECU") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

// GetECUFooter reads the last 0x80 bytes of flash once and caches them until
// the flash is erased.
func TestGetECUFooterCache(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	bin := buildBin(t, 0x40000, 0x40000)
	f.load(bin)

	for range 2 {
		got, err := cl.GetECUFooter(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, bin[len(bin)-0x80:]) {
			t.Fatal("footer differs from the last 0x80 bytes of flash")
		}
	}
	reads := len(f.frames(0xC7))
	if reads != 22 {
		t.Fatalf("footer took %d reads, want 22 (one pass)", reads)
	}

	if err := cl.EraseECU(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := cl.GetECUFooter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte{0xFF}, 0x80)) {
		t.Fatalf("footer after erase = % X, want erased", got)
	}

	f.setHook(func(d [8]byte) ([]byte, bool) { return echoMismatch, true })
	cl.invalidateFooter()
	if _, err := cl.GetECUFooter(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to get ECU footer") {
		t.Fatalf("err = %v, want footer read failure", err)
	}
}

// DetermineECU combines the chip size from C9 with ROM_Offset from the flash
// footer to tell a T5.2, a T5.5 and a T5.5 running a T5.2 bin apart.
func TestDetermineECU(t *testing.T) {
	tests := []struct {
		name      string
		chip      chip
		romOffset uint32
		binSize   int
		want      ECUType
		wantErr   string
	}{
		{"T5.2", intel28F512, 0x60000, 0x20000, T52ECU, ""},
		{"T5.2 box, T5.5 offset", intel28F512, 0x40000, 0x20000, UnknownECU, "Trionic 5.2 ECU running an unknown firmware"},
		{"T5.5", amd28F010, 0x40000, 0x40000, T55ECU, ""},
		{"T5.5 with T5.2 bin", amd28F010, 0x60000, 0x20000, T55AST52, ""},
		{"T5.5 box, odd offset", amd28F010, 0x70000, 0x10000, UnknownECU, "Trionic 5.5 ECU running an unknown firmware"},
		{"unknown chip", chip{0x01, 0x42}, 0x40000, 0x40000, UnknownECU, "unknown ECU"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, tt.chip)
			f.load(buildBin(t, tt.binSize, tt.romOffset))
			got, err := cl.DetermineECU(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("type = %v, want %v", got, tt.want)
			}
		})
	}
}

// DetermineECU passes on footer and chip id read failures as UnknownECU.
func TestDetermineECUReadErrors(t *testing.T) {
	for _, cmd := range []byte{0xC7, 0xC9} {
		cl, f, _ := newBootyClient(t, amd28F010)
		f.load(buildBin(t, 0x40000, 0x40000))
		f.setHook(func(d [8]byte) ([]byte, bool) {
			if d[0] == cmd {
				return echoMismatch, true
			}
			return nil, false
		})
		if typ, err := cl.DetermineECU(context.Background()); err == nil || typ != UnknownECU {
			t.Fatalf("failing %02X: got %v, %v", cmd, typ, err)
		}
	}
}

// flashSizeFromChip knows every device id MyBooty can program and chip
// descriptions name the maker, device and size, or say what is unknown.
func TestChipDescription(t *testing.T) {
	tests := []struct {
		maker, dev byte
		size       uint16
		want       string
	}{
		{0x89, 0xB8, 128, "Intel 28F512 (128 kB)"},
		{0x01, 0x25, 128, "AMD 28F512 (128 kB)"},
		{0x1F, 0x5D, 128, "Atmel 29C512 (128 kB)"},
		{0x31, 0xB4, 256, "CSI/Catalyst 28F010 (256 kB)"},
		{0x01, 0xA7, 256, "AMD 28F010 (256 kB)"},
		{0x20, 0x20, 256, "ST 29F010 (256 kB)"},
		{0x1F, 0xD5, 256, "Atmel 29C010 (256 kB)"},
		{0xBF, 0xB5, 256, "SST 39SF010A (256 kB)"},
		{0x37, 0xA4, 256, "AMIC A29010L (256 kB)"},
		{0x42, 0xA7, 256, "unknown make 0x42 28F010 (256 kB)"},
		{0x89, 0x99, 0, "Intel unknown device 0x99"},
	}
	for _, tt := range tests {
		if got := flashSizeFromChip(tt.dev); got != tt.size {
			t.Errorf("flashSizeFromChip(%02X) = %d, want %d", tt.dev, got, tt.size)
		}
		if got := chipDescription(tt.maker, tt.dev); got != tt.want {
			t.Errorf("chipDescription(%02X, %02X) = %q, want %q", tt.maker, tt.dev, got, tt.want)
		}
	}
}

// Info uploads the loader when needed and lists every T5 header from the
// flash footer plus the FLASH chip description.
func TestInfo(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.load(buildBin(t, 0x40000, 0x40000))
	res, err := cl.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Part Number": "4780656",
		"Software ID": "4782506",
		"SW Version":  "A55L41VQ.29H",
		"Engine Type": "9000 2.3T B234R",
		"IMMO Code":   "QQ12345",
		"Other Info":  "",
		"ROM Start":   "040000",
		"ROM End":     "07FFFF",
		"Code End":    "0401FF",
		"FLASH Chips": "AMD 28F010 (256 kB)",
	}
	if len(res) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(res), len(want), res)
	}
	for _, r := range res {
		if v, ok := want[r.Desc]; !ok || v != r.Value {
			t.Errorf("%s = %q, want %q", r.Desc, r.Value, v)
		}
	}
}

// Info still reports the footer when the chip id cannot be read, and fails
// when the footer cannot.
func TestInfoReadErrors(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.load(buildBin(t, 0x40000, 0x40000))
	f.setHook(func(d [8]byte) ([]byte, bool) {
		if d[0] == 0xC9 {
			return echoMismatch, true
		}
		return nil, false
	})
	res, err := cl.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != len(T5Headers) {
		t.Fatalf("got %d results, want only the %d footer headers", len(res), len(T5Headers))
	}

	cl.invalidateFooter()
	f.setHook(func(d [8]byte) ([]byte, bool) { return echoMismatch, true })
	if _, err := cl.Info(context.Background()); err == nil {
		t.Fatal("Info succeeded without a readable footer")
	}
}

// PrintECUInfo reports which kind of T5 this is.
func TestPrintECUInfo(t *testing.T) {
	tests := []struct {
		chip      chip
		romOffset uint32
		size      int
		want      string
	}{
		{intel28F512, 0x60000, 0x20000, "Trionic 5.2 ECU with 128 kB"},
		{amd28F010, 0x40000, 0x40000, "Trionic 5.5 ECU with 256 kB"},
		{amd28F010, 0x60000, 0x20000, "Trionic 5.5 ECU with a T5.2 BIN"},
	}
	for _, tt := range tests {
		cl, f, ev := newBootyClient(t, tt.chip)
		f.load(buildBin(t, tt.size, tt.romOffset))
		if err := cl.PrintECUInfo(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !ev.hasMessage(tt.want) {
			t.Errorf("messages %q lack %q", ev.messages, tt.want)
		}
	}

	cl, f, _ := newBootyClient(t, chip{0x01, 0x42})
	f.load(buildBin(t, 0x40000, 0x40000))
	if err := cl.PrintECUInfo(context.Background()); err == nil {
		t.Fatal("unknown chip accepted")
	}
}

// GetSRAMSnapshot reads the 32 kB SRAM from the stock firmware with 16-bit
// C7 reads, here against the pkg/ecusim T5.
func TestGetSRAMSnapshot(t *testing.T) {
	sim := &ecusim.T5{}
	ram := make([]byte, 0x8000)
	for i := range ram {
		ram[i] = byte(i ^ i>>8)
	}
	sim.Poke(0, ram)
	cl, _ := newClient(t, sim)
	got, err := cl.GetSRAMSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, ram) {
		t.Fatal("snapshot differs from SRAM")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cl.GetSRAMSnapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// MarryECU and ReadDTC are not available on the T5 and fail without
// touching the bus.
func TestUnsupported(t *testing.T) {
	f := newFakeT5(amd28F010)
	cl, _ := newClient(t, f)
	if err := cl.MarryECU(context.Background(), "VIN"); err == nil {
		t.Error("MarryECU succeeded")
	}
	if _, err := cl.ReadDTC(context.Background()); err == nil {
		t.Error("ReadDTC succeeded")
	}
	if n := len(f.commands()); n != 0 {
		t.Errorf("sent %d frames", n)
	}
}

// PrintECUInfo fails when the ECU info cannot be read.
func TestPrintECUInfoReadError(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.setHook(func(d [8]byte) ([]byte, bool) { return echoMismatch, true })
	if err := cl.PrintECUInfo(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to get ECU footer") {
		t.Fatalf("err = %v", err)
	}
}

// One client through a whole session: Info on the old T5.5 bin, flash a
// T5.2 bin (mirrored), reset, then dump. The footer read by Info must not
// survive the flash, and the reset must make the dump upload MyBooty again.
func TestSession(t *testing.T) {
	f := newFakeT5(amd28F010)
	f.load(buildBin(t, 0x40000, 0x40000))
	cl, _ := newClient(t, f)
	ctx := context.Background()

	res, err := cl.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Desc == "ROM Start" && r.Value != "040000" {
			t.Fatalf("ROM Start before flash = %q", r.Value)
		}
	}

	// Make the new code block differ from the old bin's, keeping the stored
	// checksum valid.
	bin := buildBin(t, 0x20000, 0x60000)
	bin[0x10] = 0x00
	var sum uint32
	for _, b := range bin[:0x200] {
		sum += uint32(b)
	}
	bin[len(bin)-4], bin[len(bin)-3], bin[len(bin)-2], bin[len(bin)-1] = byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum)
	if err := cl.FlashECU(ctx, bin); err != nil {
		t.Fatal(err)
	}
	if err := cl.ResetECU(ctx); err != nil {
		t.Fatal(err)
	}
	dump, err := cl.DumpECU(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dump, bin) {
		t.Fatalf("dump of %d bytes is not the flashed T5.2 bin", len(dump))
	}
	if n := len(f.frames(0xC1)); n != 2 {
		t.Fatalf("jumped to MyBooty %d times, want 2 (first use and after reset)", n)
	}
}
