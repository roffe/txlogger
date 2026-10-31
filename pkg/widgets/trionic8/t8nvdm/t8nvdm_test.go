package t8nvdm

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildSlot lays out a plaintext slot the way a shipped firmware does: crypt[]
// at 80 and the identity block at +2 vs NVDM.h.
func buildSlot() []byte {
	d := make([]byte, slotSize)
	copy(d[cryptOff:], "Cryp")
	copy(d[109:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) // powerTrainSK
	copy(d[srcVIN+shippedDelta:], "YS3FH46YX31040360")
	copy(d[173+shippedDelta:], "Trionic 8 ")
	d[296+shippedDelta] = 1 // ST_ImmoEnabled
	return d
}

func TestRoundTrip(t *testing.T) {
	plain := buildSlot()
	buf := make([]byte, 0x1000)
	const at = 0x400
	Encrypt(buf[at:at+slotSize], plain)

	slots := Scan(buf)
	if len(slots) != 1 || slots[0] != at {
		t.Fatalf("Scan = %v, want [%d]", slots, at)
	}
	got := Decrypt(buf, slots[0])
	if !bytes.Equal(got, plain) {
		t.Fatal("decrypt(encrypt(x)) != x")
	}
	if d := Delta(got); d != shippedDelta {
		t.Fatalf("Delta = %d, want %d", d, shippedDelta)
	}
}

// The live bank is the one with the higher generation counter, not the one at
// the higher address: a real dump has bank 0x4000 newer than bank 0x6000.
func TestCurrentPicksNewestBank(t *testing.T) {
	buf := make([]byte, 0x8000)
	bank := func(base, gen int, vin string) int {
		copy(buf[base:], "MFS*")
		binary.BigEndian.PutUint32(buf[base+4:], uint32(gen))
		d := buildSlot()
		copy(d[srcVIN+shippedDelta:], vin)
		slot := base + 0x2D8
		Encrypt(buf[slot:slot+slotSize], d)
		return slot
	}
	older := bank(0x6000, 0x4BC, "YS3FF45S23104040X")
	newer := bank(0x4000, 0x4BD, "YS3FH46YX31040360")

	slots := Scan(buf)
	if len(slots) != 2 {
		t.Fatalf("Scan found %d slots, want 2", len(slots))
	}
	if got := Current(buf, slots); got != newer {
		t.Fatalf("Current = %#x, want %#x (older bank is %#x)", got, newer, older)
	}
}

func TestFieldReadWrite(t *testing.T) {
	plain := buildSlot()
	delta := Delta(plain)

	find := func(name string) *field {
		for _, set := range [][]field{idFields, immoFields} {
			for i := range set {
				if set[i].name == name {
					return &set[i]
				}
			}
		}
		t.Fatalf("no field %q", name)
		return nil
	}

	vin := find("VIN (DID 90)")
	b, err := vin.get(plain, delta)
	if err != nil {
		t.Fatal(err)
	}
	if got := vin.encode(b); got != "YS3FH46YX31040360" {
		t.Fatalf("VIN = %q", got)
	}

	immo := find("ST_ImmoEnabled")
	b, _ = immo.get(plain, delta)
	if got := immo.encode(b); got != "1" {
		t.Fatalf("ST_ImmoEnabled = %q, want 1", got)
	}

	sk := find("powerTrainSK")
	b, _ = sk.get(plain, delta)
	if got := sk.encode(b); got != "0102030405060708090A0B0C0D0E0F10" {
		t.Fatalf("powerTrainSK = %q", got)
	}

	// Edit VIN and immo flag, everything else must stay byte identical.
	want := append([]byte(nil), plain...)
	copy(want[srcVIN+delta:], "YS3FF45S23104040X")
	want[296+delta] = 0

	nv, err := vin.decode("YS3FF45S23104040X")
	if err != nil {
		t.Fatal(err)
	}
	if err := vin.set(plain, delta, nv); err != nil {
		t.Fatal(err)
	}
	ni, err := immo.decode("0")
	if err != nil {
		t.Fatal(err)
	}
	if err := immo.set(plain, delta, ni); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, want) {
		t.Fatal("edit changed bytes outside the edited fields")
	}
}

// A blank field padded with 0xFF (virgin ECU) renders as dots, so encode is not
// reversible — which is why the editor only writes fields you actually changed.
func TestASCIIEncodeIsLossy(t *testing.T) {
	raw := bytes.Repeat([]byte{0xFF}, 17)
	s := fVIN.encode(raw)
	b, err := fVIN.decode(s)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(b, raw) {
		t.Fatal("encode/decode round trip is lossless, the unchanged-field guard is no longer needed")
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	vin := field{"vin", srcVIN, 17, kASCII, true}
	if _, err := vin.decode("THIS VIN IS FAR TOO LONG"); err == nil {
		t.Fatal("oversized ascii accepted")
	}
	if b, err := vin.decode("ABC"); err != nil || len(b) != 17 || string(b) != "ABC              " {
		t.Fatalf("short ascii not space padded: %q %v", b, err)
	}
	sk := field{"sk", 109, 16, kHex, false}
	if _, err := sk.decode("0102"); err == nil {
		t.Fatal("short hex accepted")
	}
	if _, err := sk.decode("zz"); err == nil {
		t.Fatal("non-hex accepted")
	}
	bt := field{"b", 296, 1, kByte, true}
	if _, err := bt.decode("256"); err == nil {
		t.Fatal("out of range byte accepted")
	}
}
