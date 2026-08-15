package aem

import "testing"

func TestParse(t *testing.T) {
	// Corrupt lines are taken from a real log of an AEM on a Prolific cable.
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"clean", "10.3\r\n", 1.03},
		{"split reads", "10", 0},
		{"leading nul byte", "\xff10.3\r\n", 0},
		{"corrupt first digit", "\x100.3\r\n", 0},
		{"digit dropped to space", " 0.3\r\n", 0},
		{"corrupt second digit", "1\x10.3\r\n", 0},
		{"garbage then good line", "\x110.4\r\n14.7\r\n", 1.47},
		{"no line ending", "1234567890123\r\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewAEMuegoClient("test", func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			a.parse([]byte(tt.in))
			if got := a.GetLambda(); got != tt.want {
				t.Errorf("parse(%q) lambda = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseAcrossReads(t *testing.T) {
	a, _ := NewAEMuegoClient("test", func(string) {})
	for _, chunk := range []string{"1", "0.", "3\r", "\n"} {
		a.parse([]byte(chunk))
	}
	if got := a.GetLambda(); got != 1.03 {
		t.Errorf("lambda = %v, want 1.03", got)
	}
}
