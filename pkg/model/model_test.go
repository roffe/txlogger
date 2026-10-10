package model

import "testing"

// HeaderResult.String renders "<Desc>: <Value>", which is how ECU info rows
// are shown in the info dialog.
func TestHeaderResultString(t *testing.T) {
	tests := []struct {
		name string
		in   HeaderResult
		want string
	}{
		{"typical", HeaderResult{Header: Header{Desc: "VIN", ID: 0x90}, Value: "YS3EF48E9X3041142"}, "VIN: YS3EF48E9X3041142"},
		{"empty value", HeaderResult{Header: Header{Desc: "Software"}}, "Software: "},
		{"empty desc", HeaderResult{Value: "T7"}, ": T7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// DTC.String renders the code followed by the status byte in unpadded
// uppercase hex.
func TestDTCString(t *testing.T) {
	tests := []struct {
		in   DTC
		want string
	}{
		{DTC{Code: "P0300", Status: 0x00}, "P0300, Status: 0"},
		{DTC{Code: "P0171", Status: 0x0a}, "P0171, Status: A"},
		{DTC{Code: "P1651", Status: 0xff}, "P1651, Status: FF"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}
