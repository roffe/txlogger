package t7

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GetHeaderField walks the PI area down from the top of the bin and returns
// a field's data in ECU read order.
func TestGetHeaderField(t *testing.T) {
	bin := bytes.Repeat([]byte{0xFF}, 0x80000)
	writePI(bin, []piField{
		{0x95, []byte("EU0AF01C.55P")},
		{0x90, []byte("YS3AAAAAAAAAAAAA1")},
		{0xFB, be32(0x11223344)},
	})

	for _, tt := range []struct {
		id   byte
		want string
	}{
		{0x95, "EU0AF01C.55P"},
		{0x90, "YS3AAAAAAAAAAAAA1"},
		{0xFB, "\x11\x22\x33\x44"},
	} {
		got, err := GetHeaderField(bin, tt.id)
		if err != nil || got != tt.want {
			t.Errorf("GetHeaderField(0x%02X) = %q, %v; want %q", tt.id, got, err, tt.want)
		}
	}
	if _, err := GetHeaderField(bin, 0x97); err == nil {
		t.Error("missing field 0x97 found")
	}
}

// After a match GetHeaderField keeps walking the PI area field by field: it
// returns the lowest of duplicate entries (the one appended last, e.g. a
// re-written VIN, as its comment says), and never reads data bytes below a
// matched field as a field header.
func TestGetHeaderFieldAfterMatch(t *testing.T) {
	t.Run("duplicate VIN", func(t *testing.T) {
		bin := bytes.Repeat([]byte{0xFF}, 0x80000)
		writePI(bin, []piField{
			{0x90, []byte("YS3AAAAAAAAAAAAA1")},
			{0x95, []byte("EU0AF01C.55P")},
			{0x90, []byte("YS3BBBBBBBBBBBBB2")},
		})
		if got, err := GetHeaderField(bin, 0x90); err != nil || got != "YS3BBBBBBBBBBBBB2" {
			t.Fatalf("GetHeaderField(0x90) = %q, %v; want the VIN appended last", got, err)
		}
	})
	t.Run("data below a match", func(t *testing.T) {
		// The 0x9A data bytes 02 FB EE EE sit exactly where a walk that skips
		// 0xFB's four data bytes twice lands, and read as a 2-byte 0xFB field.
		bin := bytes.Repeat([]byte{0xFF}, 0x80000)
		writePI(bin, []piField{
			{0xFB, be32(0x11223344)},
			{0x9A, []byte{0x30, 0x37, 0x02, 0xFB, 0xEE, 0xEE, 0x34, 0x38}},
		})
		if got, err := GetHeaderField(bin, 0xFB); err != nil || got != "\x11\x22\x33\x44" {
			t.Fatalf("GetHeaderField(0xFB) = %q, %v; want 11 22 33 44", got, err)
		}
	})
}

// A zero length byte ends the PI area just like the 0xFF terminator.
func TestGetHeaderFieldZeroTerminator(t *testing.T) {
	bin := make([]byte, 0x80000)
	writePI(bin, []piField{{0x95, []byte("ABC")}})
	bin[len(bin)-1-2-3] = 0x00 // replace the 0xFF terminator
	bin[len(bin)-1-2-3-1] = 0x90
	if got, err := GetHeaderField(bin, 0x95); err != nil || got != "ABC" {
		t.Fatalf("GetHeaderField(0x95) = %q, %v", got, err)
	}
	if _, err := GetHeaderField(bin, 0x90); err == nil {
		t.Fatal("read past a zero length byte")
	}
}

// GetBinInfo maps the PI fields onto BinInfo and leaves missing ones empty.
func TestGetBinInfo(t *testing.T) {
	bin := bytes.Repeat([]byte{0xFF}, 0x80000)
	writePI(bin, []piField{
		{0x90, []byte("YS3DD58N512345678")},
		{0x91, []byte("5382019")},
		{0x92, []byte("123456789012345")},
		{0x94, []byte("5387998")},
		{0x95, []byte("EU0AF01C.55P")},
		{0x97, []byte("B235R")},
		{0x98, []byte("txlogger EOL")},
	})
	want := BinInfo{
		Vin:             "YS3DD58N512345678",
		HwPartNo:        "5382019",
		ImmoCode:        "123456789012345",
		SoftwarePartNo:  "5387998",
		SoftwareVersion: "EU0AF01C.55P",
		EngineType:      "B235R",
		Tester:          "txlogger EOL",
	}
	if got := GetBinInfo(bin); got != want {
		t.Fatalf("GetBinInfo = %+v, want %+v", got, want)
	}
}

// ROMChecksum needs all three PI fields it is built from, each exactly four
// bytes wide.
func TestROMChecksumFieldErrors(t *testing.T) {
	full := []piField{
		{0xFB, be32(0)},
		{0xFE, be32(8)},
		{0xFD, be32(0)},
	}
	for _, tt := range []struct {
		name   string
		fields []piField
		want   string
	}{
		{"no bottom of flash", full[:2], "0xfd"},
		{"no top of program", []piField{full[0], full[2]}, "0xfe"},
		{"no stored checksum", full[1:], "0xfb"},
		{"short field", []piField{full[0], full[1], {0xFD, []byte{0, 0, 0}}}, "is 3 bytes, want 4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bin := bytes.Repeat([]byte{0xFF}, 0x80000)
			writePI(bin, tt.fields)
			_, _, err := ROMChecksum(bin)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

// The checksum sum stops at the top of flash even when 0xFE points past it,
// still counting the last word 0x7FFFC-0x7FFFF.
func TestROMChecksumClampsToFlash(t *testing.T) {
	bin := make([]byte, 0x80000)
	writePI(bin, []piField{
		{0xFB, be32(0)},
		{0xFE, be32(0xFFFFFFF0)},
		{0xFD, be32(0x7FFE0)},
	})
	// [0x7FFE0, 0x80000) is zero apart from the PI area and its 0xFF
	// terminator, which make up the last five words:
	//   7FFEC 00FFE0FF  7FFF0 0700FD04  7FFF4 F0FFFFFF  7FFF8 FE040000  7FFFC 0000FB04
	// Their 32-bit sum is F705D906; without the last word it would be F704DE02.
	calc, stored, err := ROMChecksum(bin)
	if err != nil {
		t.Fatal(err)
	}
	if calc != 0xF705D906 || stored != 0 {
		t.Fatalf("calc = %08X, stored = %08X; want F705D906, 0", calc, stored)
	}
}

// NormalizeBin accepts only 512 KB images with the T7 magic and converts a
// Motorola byte-order dump to Intel order without touching the caller's slice.
func TestNormalizeBin(t *testing.T) {
	intel := testBin()

	got, err := NormalizeBin(intel)
	if err != nil || !bytes.Equal(got, intel) {
		t.Fatalf("Intel bin: err %v, changed %v", err, !bytes.Equal(got, intel))
	}

	moto := make([]byte, len(intel))
	for i := 0; i < len(intel); i += 2 {
		moto[i], moto[i+1] = intel[i+1], intel[i]
	}
	orig := bytes.Clone(moto)
	got, err = NormalizeBin(moto)
	if err != nil || !bytes.Equal(got, intel) {
		t.Fatalf("Motorola bin: err %v, swapped back %v", err, bytes.Equal(got, intel))
	}
	if !bytes.Equal(moto, orig) {
		t.Fatal("NormalizeBin modified the caller's slice")
	}

	if _, err := NormalizeBin(intel[:0x40000]); err == nil {
		t.Fatal("256 KB bin accepted")
	}
	bad := bytes.Clone(intel)
	bad[2] = 0x00
	if _, err := NormalizeBin(bad); err == nil || !strings.Contains(err.Error(), "FFFF00FC") {
		t.Fatalf("err = %v, want the bad magic quoted", err)
	}
}

// LoadBinFile reads a 512 KB or 0x70100-byte image (swapping a Motorola
// byte-order one in place), refuses a 256 KB T5 image, and rejects anything
// else.
func TestLoadBinFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var c Client

	intel := testBin()
	moto := make([]byte, len(intel))
	for i := 0; i < len(intel); i += 2 {
		moto[i], moto[i+1] = intel[i+1], intel[i]
	}
	n, data, err := c.LoadBinFile(write("moto.bin", moto))
	if err != nil || n != 0x80000 || !bytes.Equal(data, intel) {
		t.Fatalf("Motorola bin: n=%#x err=%v intel=%v", n, err, bytes.Equal(data, intel))
	}

	short := bytes.Repeat([]byte{0x11}, 0x70100)
	if n, data, err := c.LoadBinFile(write("short.bin", short)); err != nil || n != 0x70100 || !bytes.Equal(data, short) {
		t.Fatalf("0x70100 bin: n=%#x err=%v", n, err)
	}

	if _, _, err := c.LoadBinFile(write("t5.bin", make([]byte, 256*1024))); err == nil || !strings.Contains(err.Error(), "Trionic 5") {
		t.Fatalf("T5 bin: err = %v", err)
	}
	if n, data, err := c.LoadBinFile(write("odd.bin", make([]byte, 1000))); err == nil || n != 1000 || data != nil {
		t.Fatalf("odd size: n=%d data=%v err=%v", n, data != nil, err)
	}
	if _, _, err := c.LoadBinFile(filepath.Join(dir, "missing.bin")); err == nil {
		t.Fatal("missing file loaded")
	}
}

// LookupID names a frame by the bus that uses it, preferring the P-bus name
// for ids both buses carry.
func TestLookupID(t *testing.T) {
	for _, tt := range []struct {
		id   uint32
		want string
	}{
		{0x1A0, "(P) Engine information"},
		{0x280, "(P) Pedals, reverse gear"},
		{0x220, "(I) Trionic data initialization"},
		{0x7A0, "(I) Outside temperature"},
		{0x123, "??"},
	} {
		if got := LookupID(tt.id); got != tt.want {
			t.Errorf("LookupID(0x%03X) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
