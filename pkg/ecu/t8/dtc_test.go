package t8

import (
	"bytes"
	"testing"
)

// ReadDTC opens a diagnostic session, asks for the DTCs matching status mask
// 0x12, collects the UUDT stream until its end marker and returns to normal
// mode.
func TestReadDTC(t *testing.T) {
	ecu := newFakeT8()
	ecu.dtcs = [][]byte{
		{0x81, 0x01, 0x01, 0x00, 0x0A},
		{0x81, 0xE1, 0x03, 0x02, 0x48},
		{0x81, 0x42, 0xAB, 0x01, 0x02},
	}
	c, _ := newClient(t, ecu)

	got, err := c.ReadDTC(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		code   string
		status byte
	}{{"P0101", 0x0A}, {"U2103", 0x48}, {"C02AB", 0x02}}
	if len(got) != len(want) {
		t.Fatalf("got %d DTCs, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Code != w.code || got[i].Status != w.status {
			t.Errorf("DTC %d = %s/%02X, want %s/%02X", i, got[i].Code, got[i].Status, w.code, w.status)
		}
	}

	var seq [][]byte
	for _, d := range ecu.sentTo(0x7E0) {
		seq = append(seq, d[:d[0]+1])
	}
	wantSeq := [][]byte{{0x02, 0x10, 0x02}, {0x03, 0xA9, 0x81, 0x12}, {0x01, 0x20}}
	if len(seq) != len(wantSeq) {
		t.Fatalf("requests = % X, want % X", seq, wantSeq)
	}
	for i := range wantSeq {
		if !bytes.Equal(seq[i], wantSeq[i]) {
			t.Errorf("request %d = % X, want % X", i, seq[i], wantSeq[i])
		}
	}
}

// ReadDTC returns no DTCs from an ECU that has none stored.
func TestReadDTCNone(t *testing.T) {
	c, _ := newClient(t, newFakeT8())
	got, err := c.ReadDTC(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no DTCs", got)
	}
}

// ReadDTC stops when the ECU refuses the diagnostic session and does not
// ask for DTCs.
func TestReadDTCSessionRefused(t *testing.T) {
	ecu := newFakeT8()
	ecu.nrc[0x10] = 0x22 // conditionsNotCorrect
	c, _ := newClient(t, ecu)
	if _, err := c.ReadDTC(testCtx(t)); err == nil {
		t.Fatal("ReadDTC succeeded with the session refused")
	}
	if ecu.sentWith(0x03, 0xA9) {
		t.Error("DTCs were requested without a diagnostic session")
	}
}

// getDTCDescription decodes the letter and digits of a 4-byte DTC record and
// returns its status byte.
func TestGetDTCDescription(t *testing.T) {
	for _, c := range []struct {
		in     []byte
		code   string
		status byte
	}{
		{[]byte{0xE1, 0x03, 0x00, 0x48}, "U2103", 0x48},
		{[]byte{0x01, 0x01, 0x00, 0x0A}, "P0101", 0x0A},
		{[]byte{0x92, 0x34, 0x00, 0x01}, "B1234", 0x01},
		{[]byte{0x40, 0x00, 0x00, 0x00}, "C0000", 0x00},
	} {
		code, status, err := getDTCDescription(c.in)
		if err != nil || code != c.code || status != c.status {
			t.Errorf("getDTCDescription(% X) = %q %02X %v, want %q %02X", c.in, code, status, err, c.code, c.status)
		}
	}
	if _, _, err := getDTCDescription([]byte{1, 2, 3}); err == nil {
		t.Error("getDTCDescription accepted 3 bytes")
	}
}

// getDTCDescription writes the third to fifth characters as hex digits, as
// the table above it documents (1010 = A).
func TestGetDTCDescriptionHexDigits(t *testing.T) {
	code, _, err := getDTCDescription([]byte{0x0F, 0xAB, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if code != "P0FAB" {
		t.Errorf("code = %q, want P0FAB", code)
	}
}
