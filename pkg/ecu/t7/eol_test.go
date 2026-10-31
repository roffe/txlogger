package t7

import (
	"testing"

	"github.com/roffe/gocan/v2/t7kwp"
)

// writePI lays out PI-area entries the way the ECU does: growing downward from
// the top of flash, each entry [data reversed][id][length].
func writePI(bin []byte, fields []struct {
	id   byte
	data []byte
},
) {
	a := len(bin) - 1
	for _, f := range fields {
		bin[a] = byte(len(f.data))
		bin[a-1] = f.id
		for i, b := range f.data {
			bin[a-2-i] = b
		}
		a -= 2 + len(f.data)
	}
	bin[a] = 0xFF // terminator
}

func TestROMChecksum(t *testing.T) {
	bin := make([]byte, 0x80000)
	for i := range bin {
		bin[i] = 0xFF
	}
	// program area: 0x00000-0x00010 counts, everything above is outside 0xFE
	copy(bin, []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02})
	const want = 0x00000003

	writePI(bin, []struct {
		id   byte
		data []byte
	}{
		{0xFB, []byte{0x00, 0x00, 0x00, 0x03}}, // stored checksum
		{0xFE, []byte{0x00, 0x00, 0x00, 0x08}}, // top of program
		{0xFD, []byte{0x00, 0x00, 0x00, 0x00}}, // bottom of flash
	})

	calc, stored, err := ROMChecksum(bin)
	if err != nil {
		t.Fatal(err)
	}
	if calc != want || stored != want {
		t.Fatalf("calc=%08X stored=%08X, want both %08X", calc, stored, want)
	}
}

func TestDefaultTesterSerial(t *testing.T) {
	padded := t7kwp.PadTesterSerial(DefaultTesterSerial)
	if t7kwp.TesterSerialBlocked(padded) {
		t.Fatalf("%q is on the ECU's kill-list — writing it would brick a T7", DefaultTesterSerial)
	}
	// The safety margin is structural, not luck: offsets 9..11 must all be
	// non-space so the packing takes the three-character branch, and name[9]
	// must be >= 0x40 so the code lands above the whole kill-list (max 0x7388).
	for i := 9; i <= 11; i++ {
		if padded[i] == 0x20 {
			t.Fatalf("offset %d is a space: the packing falls back to a short-form code near the kill-list", i)
		}
	}
	if padded[9] < 0x40 {
		t.Fatalf("offset 9 is %#02x, want >= 0x40 to keep the packed code above the kill-list", padded[9])
	}
}

func TestEOLParamsValidate(t *testing.T) {
	ok := EOLParams{VIN: "YS3DD58N512345678", ProgDate: "260809", TesterSerial: "PPCAN Nr 592"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}

	bad := ok
	bad.TesterSerial = "PPCAN Nr 2" // on the ECU's kill-list
	if err := bad.Validate(); err == nil {
		t.Fatal("kill-listed tester serial accepted")
	}

	noVIN := ok
	noVIN.VIN = ""
	if err := noVIN.Validate(); err == nil {
		t.Fatal("empty VIN accepted")
	}
}
