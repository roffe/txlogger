package t5

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFlashPlan(t *testing.T) {
	tests := []struct {
		name      string
		flashSize uint16
		binLen    int
		start     uint32
		copies    int
		wantErr   bool
	}{
		{"T5.2 bin on T5.2 box", 128, 0x20000, 0x60000, 1, false},
		{"T5.5 bin on T5.2 box", 128, 0x40000, 0, 0, true},
		{"T5.5 bin on T5.5 box", 256, 0x40000, 0x40000, 1, false},
		{"T5.2 bin on T5.5 box mirrored", 256, 0x20000, 0x40000, 2, false},
		{"garbage size on T5.5 box", 256, 0x12345, 0, 0, true},
		{"unknown chip", 0, 0x20000, 0, 0, true},
	}
	for _, tt := range tests {
		start, copies, err := flashPlan(tt.flashSize, tt.binLen)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
			continue
		}
		if start != tt.start || copies != tt.copies {
			t.Errorf("%s: got start 0x%X copies %d, want 0x%X %d", tt.name, start, copies, tt.start, tt.copies)
		}
	}
}

// footerEntry builds one footer entry as read by GetIdentifierFromFooter:
// the string stored reversed, followed by the identifier and length bytes.
func footerEntry(id byte, val string) []byte {
	out := make([]byte, 0, len(val)+2)
	for i := len(val) - 1; i >= 0; i-- {
		out = append(out, val[i])
	}
	return append(out, id, byte(len(val)))
}

// testBin builds a minimal valid T5.2 bin: code in [0..0x1FF], a footer with
// ROM_Offset/Code_End and the stored checksum in the last 4 bytes.
func testBin(t *testing.T) []byte {
	t.Helper()
	bin := bytes.Repeat([]byte{0xFF}, 0x20000)
	for i := 0; i < 0x200; i++ {
		bin[i] = byte(i)
	}
	entries := append(footerEntry(ROMoffset, "060000"), footerEntry(CodeEnd, "0601FF")...)
	copy(bin[len(bin)-4-len(entries):], entries)

	var sum uint32
	for _, b := range bin[:0x200] {
		sum += uint32(b)
	}
	binary.BigEndian.PutUint32(bin[len(bin)-4:], sum)
	return bin
}

func TestValidateBinChecksum(t *testing.T) {
	c := &Client{}
	bin := testBin(t)

	if err := c.ValidateBinChecksum(bin); err != nil {
		t.Fatalf("valid bin rejected: %v", err)
	}

	corrupt := append([]byte(nil), bin...)
	corrupt[0x100] ^= 0xFF
	if err := c.ValidateBinChecksum(corrupt); err == nil {
		t.Fatal("corrupt bin accepted")
	}

	erased := bytes.Repeat([]byte{0xFF}, 0x20000)
	if err := c.ValidateBinChecksum(erased); err == nil {
		t.Fatal("bin without footer accepted")
	}
}

func TestBinCodeEndOddCount(t *testing.T) {
	bin := bytes.Repeat([]byte{0xFF}, 0x20000)
	// Code_End 0x60200 makes the byte count 0x201 (odd): MyBooty would hang.
	entries := append(footerEntry(ROMoffset, "060000"), footerEntry(CodeEnd, "060200")...)
	copy(bin[len(bin)-4-len(entries):], entries)
	if _, err := binCodeEnd(bin); err == nil {
		t.Fatal("odd checksum range accepted, would hang the ECU in C8")
	}
}

func TestGetIdentifierFromFooterCorrupt(t *testing.T) {
	// A length byte larger than the remaining footer must not panic.
	footer := bytes.Repeat([]byte{0x00}, 0x80)
	footer[0x80-5] = 0x02
	footer[0x80-6] = ROMoffset
	footer[0x80-9] = 0xF0 // next entry claims a 240 byte string
	footer[0x80-10] = CodeEnd
	if got := GetIdentifierFromFooter(footer, CodeEnd); got != "" {
		t.Fatalf("expected empty result from corrupt footer, got %q", got)
	}
}
