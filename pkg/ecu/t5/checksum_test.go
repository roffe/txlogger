package t5

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// GetIdentifierFromFooter walks the footer entries backwards from just below
// the stored checksum and returns the value of the requested one, empty
// when it is missing.
func TestGetIdentifierFromFooter(t *testing.T) {
	var footer []byte
	footer = append(footer, footerEntry(Partnumber, "4780656")...)
	footer = append(footer, footerEntry(Unknown, "")...)
	footer = append(footer, footerEntry(Dataname, "A55L41VQ.29H")...)
	footer = append(footer, footerEntry(ROMoffset, "040000")...)
	footer = append(footer, 0xDE, 0xAD, 0xBE, 0xEF) // stored checksum
	tests := []struct {
		id   byte
		want string
	}{
		{ROMoffset, "040000"},
		{Dataname, "A55L41VQ.29H"},
		{Partnumber, "4780656"},
		{Unknown, ""},
		{EngineType, ""},
	}
	for _, tt := range tests {
		if got := GetIdentifierFromFooter(footer, tt.id); got != tt.want {
			t.Errorf("identifier %02X = %q, want %q", tt.id, got, tt.want)
		}
	}
	for _, short := range [][]byte{nil, {1, 2, 3, 4}, {0x01, 0x01, 0, 0, 0, 0}} {
		if got := GetIdentifierFromFooter(short, Partnumber); got != "" {
			t.Errorf("footer % X gave %q", short, got)
		}
	}
}

// binCodeEnd rejects bins whose footer cannot describe a checksum range
// MyBooty could sum.
func TestBinCodeEndErrors(t *testing.T) {
	withFooter := func(romOffset, codeEnd string) []byte {
		bin := bytes.Repeat([]byte{0xFF}, 0x20000)
		var e []byte
		if romOffset != "" {
			e = append(e, footerEntry(ROMoffset, romOffset)...)
		}
		if codeEnd != "" {
			e = append(e, footerEntry(CodeEnd, codeEnd)...)
		}
		copy(bin[len(bin)-4-len(e):], e)
		return bin
	}
	tests := []struct {
		name    string
		bin     []byte
		want    int64
		wantErr string
	}{
		{"valid", withFooter("060000", "0601FF"), 0x1FF, ""},
		{"too small", make([]byte, 0xFF), -1, "bin too small"},
		{"no code end", withFooter("060000", ""), -1, "could not read ROM offset / code end"},
		{"no ROM offset", withFooter("", "0601FF"), -1, "could not read ROM offset / code end"},
		{"bad ROM offset", withFooter("06G000", "0601FF"), -1, "invalid ROM offset"},
		{"bad code end", withFooter("060000", "06ZZFF"), -1, "invalid code end"},
		{"code end before ROM", withFooter("060000", "05FFFF"), -1, "code end outside of bin"},
		{"code end equals ROM", withFooter("060000", "060000"), -1, "code end outside of bin"},
		{"code end in stored checksum", withFooter("060000", "07FFFD"), -1, "code end outside of bin"},
		{"last byte before checksum", withFooter("060000", "07FFFB"), 0x1FFFB, ""},
	}
	for _, tt := range tests {
		got, err := binCodeEnd(tt.bin)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v, want %q", tt.name, err, tt.wantErr)
			}
		} else if err != nil {
			t.Errorf("%s: %v", tt.name, err)
		}
		if got != tt.want {
			t.Errorf("%s: end = 0x%X, want 0x%X", tt.name, got, tt.want)
		}
	}
}

// CalculateBinChecksum is the big endian byte sum from file start through
// Code_End; bytes past Code_End do not count.
func TestCalculateBinChecksum(t *testing.T) {
	c := &Client{}
	bin := buildBin(t, 0x40000, 0x40000)
	got, err := c.CalculateBinChecksum(bin)
	if err != nil {
		t.Fatal(err)
	}
	// buildBin's code bytes i*7+3 run through every byte value twice in
	// 0x200 bytes: 2 * (0+1+...+255) = 0xFF00.
	if want := []byte{0x00, 0x00, 0xFF, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("checksum % X, want % X", got, want)
	}
	bin[0x1FF]++ // Code_End itself counts
	if again, _ := c.CalculateBinChecksum(bin); binary.BigEndian.Uint32(again) != 0xFF01 {
		t.Fatalf("checksum % X after bumping the Code_End byte, want 0000FF01", again)
	}
	bin[0x1FF]--
	bin[0x200] = 0x00 // just past Code_End
	if again, _ := c.CalculateBinChecksum(bin); !bytes.Equal(again, got) {
		t.Fatal("byte past Code_End changed the checksum")
	}
	if _, err := c.CalculateBinChecksum(make([]byte, 0x10)); err == nil {
		t.Fatal("tiny bin accepted")
	}
}

// Real T5 bins carry footers the parser reads and checksums the validator
// accepts, the same check MyBooty makes after a flash. The bins live in the
// ecusymbol checkout next to this repository (the module cache copy has no
// testdata); the test skips when it is not there.
func TestRealBins(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "ecusymbol", "testdata", "T5Binaries")
	names, err := filepath.Glob(filepath.Join(dir, "*.BIN"))
	if err != nil || len(names) == 0 {
		t.Skip("ecusymbol T5 test binaries not available")
	}
	c := &Client{}
	checked := 0
	for _, name := range names {
		bin, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(bin) != 0x20000 && len(bin) != 0x40000 {
			continue
		}
		checked++
		wantOffset := "040000"
		if len(bin) == 0x20000 {
			wantOffset = "060000"
		}
		if got := GetIdentifierFromFooter(bin, ROMoffset); got != wantOffset {
			t.Errorf("%s: ROM offset %q, want %q", filepath.Base(name), got, wantOffset)
			continue
		}
		if end, err := strconv.ParseUint(GetIdentifierFromFooter(bin, ROMend), 16, 32); err != nil || end != 0x7FFFF {
			t.Errorf("%s: ROM end %X (%v), want 7FFFF", filepath.Base(name), end, err)
		}
		if err := c.ValidateBinChecksum(bin); err != nil {
			t.Errorf("%s: %v", filepath.Base(name), err)
		}
	}
	if checked == 0 {
		t.Skip("no 128/256 kB T5 bins found")
	}
	t.Logf("checked %d bins", checked)
}
