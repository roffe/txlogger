package innovate

import "testing"

func TestProcessBytes(t *testing.T) {
	// packet builds an ISP2 packet of n words: header, one lambda sub-packet
	// (normal, stoich 14.7, λ 0.991, from LM2.cs' sample) and zeroed aux words.
	packet := func(n int) []byte {
		p := []byte{0xB2, 0x80 | byte(n), 0x43, 0x13, 0x03, 0x6B}
		return append(p, make([]byte, (n-2)*2)...)
	}
	tests := []struct {
		name string
		in   []byte
	}{
		{"LM-2", packet(7)},
		{"chained MTS, 11 words", packet(11)},
		{"lambda word lost, then a good packet", append(packet(7)[:4], packet(7)...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := NewISP2Client("test", func(string) {})
			c.SetData(tt.in)
			if got := c.GetLambda(); got != 0.991 {
				t.Errorf("lambda = %v, want 0.991", got)
			}
		})
	}
}
