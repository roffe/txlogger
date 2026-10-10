package t8sec

import "testing"

// Known-answer vectors produced by running TrionicCANLib's SeedToKey.cs
// (calculateKey for AccessLevel01/FB/FD and calculateKeyForCIM) under mono,
// so they are independent of this package's arithmetic.
var keyVectors = []struct {
	seed                uint16
	key01, keyFB, keyFD uint16
	keyCIM              uint16
}{
	{0x0000, 0xB988, 0x8A4B, 0x44DF, 0x0F36},
	{0x0001, 0xC188, 0x824B, 0x5325, 0x0E36},
	{0x1234, 0x5A19, 0x2BFC, 0x25AF, 0xDB24},
	{0xABCD, 0x26E6, 0x675D, 0x1732, 0x428B},
	{0xFFFF, 0xB987, 0x8A7E, 0x44D2, 0x1036},
	{0x8000, 0xBD88, 0x8E4B, 0x428C, 0x0FB6},
	{0x5A5A, 0x8C5A, 0xDE39, 0x35EF, 0xB4DC},
	{0x00FF, 0xB18F, 0xF246, 0x468A, 0x1035},
}

func seedBytes(s uint16) []byte { return []byte{byte(s >> 8), byte(s)} }

func key16(hi, lo byte) uint16 { return uint16(hi)<<8 | uint16(lo) }

// CalculateAccessKey returns the T8 security key, high byte first, for the
// requested seed level: 0xFB and 0xFD get their extra transforms and every
// other level (0x01 included) gets the base rotate-and-add key.
func TestCalculateAccessKey(t *testing.T) {
	for _, v := range keyVectors {
		for _, lv := range []struct {
			level byte
			want  uint16
		}{
			{0x01, v.key01},
			{0xFB, v.keyFB},
			{0xFD, v.keyFD},
			{0x03, v.key01}, // unknown levels fall back to the base key
		} {
			hi, lo := CalculateAccessKey(seedBytes(v.seed), lv.level)
			if got := key16(hi, lo); got != lv.want {
				t.Errorf("CalculateAccessKey(%04X, %02X) = %04X, want %04X", v.seed, lv.level, got, lv.want)
			}
		}
	}
}

// CalculateAccessKey only reads the first two seed bytes, so a seed slice
// carrying trailing bytes from the response frame gives the same key.
func TestCalculateAccessKeyIgnoresTrailingBytes(t *testing.T) {
	hi, lo := CalculateAccessKey([]byte{0x12, 0x34, 0xAA, 0x55}, 0xFD)
	if got := key16(hi, lo); got != 0x25AF {
		t.Fatalf("key = %04X, want 25AF", got)
	}
}

// CalculateKeyForCIM returns the CIM key, high byte first, and ignores the
// access level.
func TestCalculateKeyForCIM(t *testing.T) {
	for _, v := range keyVectors {
		for _, level := range []byte{0x01, 0xFB, 0xFD} {
			hi, lo := CalculateKeyForCIM(seedBytes(v.seed), level)
			if got := key16(hi, lo); got != v.keyCIM {
				t.Errorf("CalculateKeyForCIM(%04X, %02X) = %04X, want %04X", v.seed, level, got, v.keyCIM)
			}
		}
	}
}
