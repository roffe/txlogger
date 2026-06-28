package txbridge

import "testing"

func TestResolveAddress(t *testing.T) {
	cases := map[string]string{
		"":                       DefaultAddress,
		"COM3":                   DefaultAddress, // serial port, not tcp
		"tcp://":                 DefaultAddress,
		"tcp://10.0.0.5":         "10.0.0.5:1337", // port appended
		"tcp://10.0.0.5:8080":    "10.0.0.5:1337", // wrong port overwritten
		"tcp://192.168.4.1:1337": "192.168.4.1:1337",
	}
	for in, want := range cases {
		if got := ResolveAddress(in); got != want {
			t.Errorf("ResolveAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
