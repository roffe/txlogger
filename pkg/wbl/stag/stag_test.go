package stag

import (
	"slices"
	"testing"
)

func TestFrames(t *testing.T) {
	// data frame from STAG.cs: status work, sensor in free air
	sample := []byte{
		0x32, 0x00, 0x00, 0x1F, 0xE4, 0x00, 0x02, 0x00,
		0x00, 0x09, 0x83, 0xAE, 0x00, 0x00, 0x9D, 0x24,
		0x00, 0xC2, 0x00, 0x0A, 0x6A, 0x00, 0x41, 0x11,
		0x00, 0x04, 0x00, 0x00, 0x00, 0x78, 0x01, 0x2C,
		0x01, 0xF4, 0x58,
	}
	lambda := float64(0x9D24) * 0.001
	badSum := slices.Clone(sample)
	badSum[len(badSum)-1]++
	// valid checksum, but too short to hold the lambda bytes
	short := []byte{0x32, 0x00, 0x00, 0x05, 0xE4, 0x00, 0x02, 0x00, 0x1D}

	tests := []struct {
		name string
		in   []byte
		want float64
	}{
		{"frame", sample, lambda},
		{"junk before start byte", append([]byte{0x01, 0x07, 0xFF, 0x00}, sample...), lambda},
		{"bad checksum", badSum, 0},
		{"short data frame", short, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := NewSTAGClient("test", func(string) {})
			var p parser
			for _, b := range tt.in {
				if frame := p.feed(b); frame != nil {
					a.SetData(frame)
				}
			}
			if got := a.GetLambda(); got != tt.want {
				t.Errorf("lambda = %v, want %v", got, tt.want)
			}
		})
	}
}
