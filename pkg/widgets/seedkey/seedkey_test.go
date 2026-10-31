package seedkey

import (
	"encoding/binary"
	"testing"
)

func TestFindApplyRoundtrip(t *testing.T) {
	data := make([]byte, 0x2000)
	off := 0x1000
	// planted method-0 routine: E548 323C 8142 B340 0640 DCAA  (0xDCAA = -0x2356)
	copy(data[off:], []byte{0xE5, 0x48, 0x32, 0x3C, 0x81, 0x42, 0xB3, 0x40, 0x06, 0x40, 0xDC, 0xAA})

	o, xor, sub, err := Find(data)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if o != off || xor != 0x8142 || sub != 0x2356 {
		t.Fatalf("got off=0x%X xor=0x%04X sub=0x%04X", o, xor, sub)
	}
	if got := MethodName(xor, sub); got != "Method 0" {
		t.Fatalf("MethodName = %q, want Method 0", got)
	}

	// switch to method 1 and verify the subtract is re-encoded as its complement
	Apply(data, o, 0x4081, 0x1F6F)
	if got := binary.BigEndian.Uint16(data[o+subOff:]); got != 0xE091 { // 0x10000 - 0x1F6F
		t.Fatalf("neg-sub encoding = 0x%04X, want 0xE091", got)
	}
	_, xor, sub, _ = Find(data)
	if xor != 0x4081 || sub != 0x1F6F {
		t.Fatalf("after Apply: xor=0x%04X sub=0x%04X", xor, sub)
	}

	// sub == 0 edge (0x10000-0 wraps to 0)
	Apply(data, o, 0x1234, 0)
	if _, _, sub, _ = Find(data); sub != 0 {
		t.Fatalf("sub=0 roundtrip got 0x%04X", sub)
	}
}

func TestFindMissing(t *testing.T) {
	if _, _, _, err := Find(make([]byte, 0x100)); err == nil {
		t.Fatal("expected error on empty data")
	}
}
