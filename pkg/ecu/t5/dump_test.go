package t5

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
)

// DumpECU reads the image the footer says is in flash: 128 kB from 0x60000
// on a T5.2, 256 kB from 0x40000 on a T5.5 (leftover of 4 bytes after the
// six-byte reads), and only the upper copy of a T5.2 bin on a T5.5. Reads
// never leave flash and a matching checksum raises no warning.
func TestDumpECU(t *testing.T) {
	tests := []struct {
		name      string
		chip      chip
		binSize   int
		romOffset uint32
		copies    int
	}{
		{"T5.2", intel28F512, 0x20000, 0x60000, 1},
		{"T5.5", amd28F010, 0x40000, 0x40000, 1},
		{"T5.2 bin on T5.5", amd28F010, 0x20000, 0x60000, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, ev := newBootyClient(t, tt.chip)
			bin := buildBin(t, tt.binSize, tt.romOffset)
			// Fill the blank area so every byte position is checked.
			for i := 0x200; i < len(bin)-0x80; i++ {
				bin[i] = byte(i>>8) ^ byte(i)
			}
			var sum uint32
			for _, b := range bin[:0x200] {
				sum += uint32(b)
			}
			binary.BigEndian.PutUint32(bin[len(bin)-4:], sum)
			f.load(bytes.Repeat(bin, tt.copies))

			got, err := cl.DumpECU(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, bin) {
				t.Fatalf("dump of %d bytes differs from the %d byte bin", len(got), len(bin))
			}
			if errs := ev.errors(); len(errs) != 0 {
				t.Fatalf("unexpected warnings %v", errs)
			}
			first, last := ev.lastProgress()
			if first != -float64(len(bin)) || last != float64(len(bin)) {
				t.Fatalf("progress %v..%v, want %d..%d", first, last, -len(bin), len(bin))
			}
		})
	}
}

// A dump whose checksum does not hold is still returned, with a warning:
// the ECU may really hold a broken bin.
func TestDumpECUChecksumWarnings(t *testing.T) {
	tests := []struct {
		name string
		hook func(d [8]byte) ([]byte, bool)
		want string
	}{
		{"ECU reports mismatch", func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xC8 {
				return []byte{0xC8, 0x01}, true
			}
			return nil, false
		}, "dump warning: ECU reports FLASH checksum mismatch"},
		{"ECU sum differs from dump", func(d [8]byte) ([]byte, bool) {
			if d[0] == 0xC8 {
				return []byte{0xC8, 0x00, 0x00, 0x00, 0x00, 0x01}, true
			}
			return nil, false
		}, "does not match dumped data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, ev := newBootyClient(t, intel28F512)
			bin := buildBin(t, 0x20000, 0x60000)
			f.load(bin)
			f.setHook(tt.hook)
			got, err := cl.DumpECU(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, bin) {
				t.Fatal("dump differs from flash")
			}
			errs := ev.errors()
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), tt.want) {
				t.Fatalf("warnings %v, want one containing %q", errs, tt.want)
			}
		})
	}
}

// A dump whose footer gives an unusable checksum range (odd byte count)
// warns about it instead of comparing checksums.
func TestDumpECUUnusableFooter(t *testing.T) {
	cl, f, ev := newBootyClient(t, intel28F512)
	bin := buildBin(t, 0x20000, 0x60000)
	e := footerEntry(CodeEnd, "060200") // replaces the last footer entry
	copy(bin[len(bin)-4-len(e):], e)
	f.load(bin)
	// The fake would hang like MyBooty on the odd range; answer C8 instead.
	f.setHook(func(d [8]byte) ([]byte, bool) {
		if d[0] == 0xC8 {
			return []byte{0xC8, 0x00, 1, 2, 3, 4}, true
		}
		return nil, false
	})
	got, err := cl.DumpECU(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bin) {
		t.Fatal("dump differs from flash")
	}
	errs := ev.errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "dump warning: unaligned code end") {
		t.Fatalf("warnings %v", errs)
	}
}

// DumpECU retries a failed read, reports it, and still returns the right
// data; reads that keep failing abort the dump, including the final
// leftover read.
func TestDumpECUReadRetries(t *testing.T) {
	cl, f, ev := newBootyClient(t, intel28F512)
	bin := buildBin(t, 0x20000, 0x60000)
	f.load(bin)
	reads := 0
	f.setHook(func(d [8]byte) ([]byte, bool) {
		if d[0] == 0xC7 {
			reads++
			if reads == 1000 {
				return echoMismatch, true
			}
		}
		return nil, false
	})
	got, err := cl.DumpECU(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bin) {
		t.Fatal("dump differs from flash after a retry")
	}
	errs := ev.errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "retrying to read memory by address") {
		t.Fatalf("errors %v", errs)
	}

	for _, tt := range []struct {
		name string
		addr uint32
	}{
		{"first read", 0x60005},
		{"leftover read", 0x7FFFF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cl, f, _ := newBootyClient(t, intel28F512)
			f.load(bin)
			cl.footer = bin[len(bin)-0x80:]
			f.setHook(func(d [8]byte) ([]byte, bool) {
				if d[0] == 0xC7 && binary.BigEndian.Uint32(d[1:5]) == tt.addr {
					return echoMismatch, true
				}
				return nil, false
			})
			if _, err := cl.DumpECU(context.Background()); err == nil {
				t.Fatal("dump succeeded with a failing read")
			}
			n := 0
			for _, fr := range f.frames(0xC7) {
				if binary.BigEndian.Uint32(fr.Data[1:5]) == tt.addr {
					n++
				}
			}
			if n != 3 {
				t.Fatalf("read 0x%X tried %d times, want 3", tt.addr, n)
			}
		})
	}
}

// DumpECU fails when the box cannot be identified.
func TestDumpECUUnknownBox(t *testing.T) {
	cl, f, _ := newBootyClient(t, amd28F010)
	f.load(buildBin(t, 0x10000, 0x70000))
	if _, err := cl.DumpECU(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown firmware") {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.frames(0xC7)); n != 22 {
		t.Fatalf("read %d times, want only the 22 footer reads", n)
	}
}
