package t8

import "testing"

// DiagnosticType and TankType names round trip through their FromString
// parsers; unknown names fall back to None and US.
func TestEnumStrings(t *testing.T) {
	for _, d := range []DiagnosticType{DiagnosticTypeNone, DiagnosticTypeOBD2, DiagnosticTypeEOBD, DiagnosticTypeLOBD} {
		if got := DiagnosticTypeFromString(d.String()); got != d {
			t.Errorf("DiagnosticTypeFromString(%q) = %v, want %v", d.String(), got, d)
		}
	}
	for _, tt := range []TankType{TankTypeUS, TankTypeEU, TankTypeAWD} {
		if got := TankTypeFromString(tt.String()); got != tt {
			t.Errorf("TankTypeFromString(%q) = %v, want %v", tt.String(), got, tt)
		}
	}
	if s := DiagnosticType(9).String(); s != "Unknown" {
		t.Errorf("DiagnosticType(9) = %q, want Unknown", s)
	}
	if s := TankType(9).String(); s != "Unknown" {
		t.Errorf("TankType(9) = %q, want Unknown", s)
	}
	if d := DiagnosticTypeFromString("bogus"); d != DiagnosticTypeNone {
		t.Errorf("DiagnosticTypeFromString(bogus) = %v, want None", d)
	}
	if tt := TankTypeFromString("bogus"); tt != TankTypeUS {
		t.Errorf("TankTypeFromString(bogus) = %v, want US", tt)
	}
}

// PI01Data.Bytes encodes each option into its PI 0x01 bits.
func TestPI01Bytes(t *testing.T) {
	for _, c := range []struct {
		name string
		in   PI01Data
		want [2]byte
	}{
		{"defaults", PI01Data{}, [2]byte{0x08, 0x00}},
		{"biopower", PI01Data{BioPower: true}, [2]byte{0x09, 0x00}},
		{"convertible EU EOBD", PI01Data{Convertible: true, TankType: TankTypeEU, DiagnosticType: DiagnosticTypeEOBD}, [2]byte{0x54, 0x00}},
		{"AWD LOBD", PI01Data{TankType: TankTypeAWD, DiagnosticType: DiagnosticTypeLOBD}, [2]byte{0x78, 0x00}},
		{"OBD2", PI01Data{DiagnosticType: DiagnosticTypeOBD2}, [2]byte{0x28, 0x00}},
		{"clutch SAI high output", PI01Data{ClutchStart: true, SAI: true, HighOutput: true}, [2]byte{0x08, 0x34}},
	} {
		got := c.in.Bytes()
		if len(got) != 2 || got[0] != c.want[0] || got[1] != c.want[1] {
			t.Errorf("%s: Bytes = % X, want % X", c.name, got, c.want[:])
		}
	}
}

// DecodePI01 reads the "off" encodings an ECU stores (the inverted bit set)
// as false, and rejects short and all-zero data.
func TestDecodePI01(t *testing.T) {
	// clutch start off (bit 1), SAI off (bit 3), low output (bit 6)
	got, err := DecodePI01([]byte{0x10, 0x4A})
	if err != nil {
		t.Fatal(err)
	}
	want := PI01Data{TankType: TankTypeEU}
	if got != want {
		t.Errorf("DecodePI01(10 4A) = %v, want %v", got, want)
	}

	if _, err := DecodePI01([]byte{0x08}); err == nil {
		t.Error("DecodePI01 accepted one byte")
	}
	if _, err := DecodePI01([]byte{0x00, 0x00}); err == nil {
		t.Error("DecodePI01 accepted 00 00")
	}
}

// Every combination of options survives Bytes followed by DecodePI01.
func TestPI01RoundTrip(t *testing.T) {
	for bits := range 32 {
		for _, d := range []DiagnosticType{DiagnosticTypeNone, DiagnosticTypeOBD2, DiagnosticTypeEOBD, DiagnosticTypeLOBD} {
			for _, tt := range []TankType{TankTypeUS, TankTypeEU, TankTypeAWD} {
				in := PI01Data{
					Convertible:    bits&1 != 0,
					SAI:            bits&2 != 0,
					HighOutput:     bits&4 != 0,
					BioPower:       bits&8 != 0,
					ClutchStart:    bits&16 != 0,
					DiagnosticType: d,
					TankType:       tt,
				}
				out, err := DecodePI01(in.Bytes())
				if err != nil {
					t.Fatalf("%v: %v", in, err)
				}
				if out != in {
					t.Fatalf("round trip of %v gave %v", in, out)
				}
			}
		}
	}
}

// PI01Data.String names every field.
func TestPI01String(t *testing.T) {
	got := PI01Data{SAI: true, DiagnosticType: DiagnosticTypeEOBD, TankType: TankTypeAWD}.String()
	want := "PI01Data{Convertible:false, SAI:true, HighOutput:false, BioPower:false, DiagnosticType:EOBD, ClutchStart:false, TankType:AWD}"
	if got != want {
		t.Errorf("String =\n%s\nwant\n%s", got, want)
	}
}
