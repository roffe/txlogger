package zeitronix

import "testing"

func TestFeed(t *testing.T) {
	z, _ := NewZeitronixClient("test", func(string) {})
	cmd := make([]byte, 14)
	step := 0
	// a stray 00 before the header must not cost the packet
	stream := []byte{0x00, 0x00, 0x01, 0x02, 147, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	for _, b := range stream {
		step = z.feed(cmd, step, b)
	}
	if got := z.GetLambda(); got != 1 {
		t.Errorf("lambda = %v, want 1 for AFR byte 147", got)
	}
}
