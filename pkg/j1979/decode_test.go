package j1979

import (
	"math"
	"testing"
)

// DecodePID applies the SAE J1979 formula for each numeric PID the T7
// implements; known answers at the range ends and typical values.
func TestDecodePIDFormulas(t *testing.T) {
	cases := []struct {
		pid  byte
		data []byte
		want float64
		unit string
	}{
		{0x04, []byte{0xFF}, 100, "%"},
		{0x04, []byte{0x80}, 50.19607843137255, "%"},
		{0x05, []byte{0x00}, -40, "°C"},
		{0x05, []byte{0xFF}, 215, "°C"},
		{0x06, []byte{0x00}, -100, "%"},
		{0x06, []byte{0x80}, 0, "%"},
		{0x07, []byte{0xFF}, 99.21875, "%"},
		{0x0B, []byte{0x65}, 101, "kPa"},
		{0x0C, []byte{0xFF, 0xFF}, 16383.75, "rpm"},
		{0x0C, []byte{0x0B, 0xB8}, 750, "rpm"},
		{0x0D, []byte{0x78}, 120, "km/h"},
		{0x0E, []byte{0x00}, -64, "°BTDC"},
		{0x0E, []byte{0xFF}, 63.5, "°BTDC"},
		{0x0F, []byte{0x41}, 25, "°C"},
		{0x10, []byte{0xFF, 0xFF}, 655.35, "g/s"},
		{0x11, []byte{0x33}, 20, "%"},
		{0x14, []byte{0x5A, 0x80}, 0.45, "V"},
		{0x15, []byte{0xFF}, 1.275, "V"},
		{0x21, []byte{0x01, 0x00}, 256, "km"},
	}
	for _, c := range cases {
		v := DecodePID(c.pid, c.data)
		if math.Abs(v.Value-c.want) > 1e-9 || v.Unit != c.unit || v.PID != c.pid {
			t.Errorf("DecodePID($%02X, % X) = %v %s, want %v %s", c.pid, c.data, v.Value, v.Unit, c.want, c.unit)
		}
	}
}

// Two-byte PIDs given one byte, numeric PIDs given no data, and the
// bitmask/enum PIDs all decode to NaN with the raw bytes kept.
func TestDecodePIDNaN(t *testing.T) {
	cases := []struct {
		pid  byte
		data []byte
		name string
	}{
		{0x0C, []byte{0x10}, "Engine speed"},
		{0x10, []byte{0x10}, "MAF air flow rate"},
		{0x21, []byte{0x10}, "Distance traveled with MIL on"},
		{0x05, nil, "Engine coolant temperature"},
		{0x00, []byte{0xBE, 0x3F, 0xB8, 0x11}, "Supported PIDs $01-$20"},
		{0x03, []byte{0x02}, "Fuel system status"},
		{0x42, []byte{0x01, 0x02}, "PID $42"},
	}
	for _, c := range cases {
		v := DecodePID(c.pid, c.data)
		if !math.IsNaN(v.Value) || v.Name != c.name || string(v.Raw) != string(c.data) {
			t.Errorf("DecodePID($%02X, % X) = %+v, want NaN named %q", c.pid, c.data, v, c.name)
		}
	}
}

// PIDValue.String formats numeric values with two decimals and the unit,
// renders the bitmask/enum PIDs as text, and falls back to hex otherwise.
func TestPIDValueString(t *testing.T) {
	cases := []struct {
		pid  byte
		data []byte
		want string
	}{
		{0x05, []byte{130}, "Engine coolant temperature: 90.00 °C"},
		{0x0C, []byte{0x0B, 0xB8}, "Engine speed: 750.00 rpm"},
		{0x01, []byte{0x82, 0x07, 0x65, 0x04}, "Monitor status since DTCs cleared: MIL ON, 2 DTC(s), incomplete: evap"},
		{0x01, []byte{0x82, 0x07}, "Monitor status since DTCs cleared: 82 07"},
		{0x02, []byte{0x00, 0x00}, "Freeze frame DTC: none"},
		{0x02, []byte{0x03, 0x01}, "Freeze frame DTC: P0301"},
		{0x02, []byte{0x03}, "Freeze frame DTC: 03"},
		{0x03, []byte{0x02, 0x00}, "Fuel system status: closed loop"},
		{0x03, nil, "Fuel system status: "},
		{0x1C, []byte{0x06}, "OBD standard: EOBD"},
		{0x1C, nil, "OBD standard: "},
		{0x00, []byte{0xBE, 0x3F, 0xB8, 0x11}, "Supported PIDs $01-$20: BE 3F B8 11"},
		{0x42, []byte{0x01, 0x02}, "PID $42: 01 02"},
	}
	for _, c := range cases {
		if got := DecodePID(c.pid, c.data).String(); got != c.want {
			t.Errorf("DecodePID($%02X, % X).String() = %q, want %q", c.pid, c.data, got, c.want)
		}
	}
}

// fuelSystemStatus and obdStandard map every SAE-defined value and show
// anything else as unknown with its hex value.
func TestEnumStrings(t *testing.T) {
	fuel := map[byte]string{
		0x01: "open loop (conditions not yet met)",
		0x02: "closed loop",
		0x04: "open loop (driving conditions)",
		0x08: "open loop (system fault)",
		0x10: "closed loop with fault",
		0x03: "unknown ($03)",
		0x00: "unknown ($00)",
	}
	for b, want := range fuel {
		if got := fuelSystemStatus(b); got != want {
			t.Errorf("fuelSystemStatus(%02X) = %q, want %q", b, got, want)
		}
	}
	obd := map[byte]string{
		0x01: "OBD-II (CARB)",
		0x02: "OBD (EPA)",
		0x03: "OBD and OBD-II",
		0x04: "OBD-I",
		0x05: "Not OBD compliant",
		0x06: "EOBD",
		0x07: "unknown ($07)",
	}
	for b, want := range obd {
		if got := obdStandard(b); got != want {
			t.Errorf("obdStandard(%02X) = %q, want %q", b, got, want)
		}
	}
}

// Byte C bit i marks non-continuous monitor i supported and byte D bit i
// marks it incomplete, in SAE order catalyst..EGR.
func TestDecodeMonitorStatusBits(t *testing.T) {
	names := []string{"catalyst", "heated cat", "evap", "sec air", "A/C", "O2 sensor", "O2 heater", "EGR"}
	for i, name := range names {
		bit := byte(1 << i)
		m := DecodeMonitorStatus([]byte{0x00, 0x00, bit, bit})
		mons := []Monitor{m.Catalyst, m.HeatedCatalyst, m.Evap, m.SecondaryAir, m.ACRefrigerant, m.O2Sensor, m.O2Heater, m.EGR}
		for j, mon := range mons {
			want := Monitor{Supported: j == i, Complete: j != i}
			if mon != want {
				t.Errorf("bit %d (%s): monitor %d = %+v, want %+v", i, name, j, mon, want)
			}
		}
		if want := "0 DTC(s), incomplete: " + name; m.String() != want {
			t.Errorf("bit %d: String() = %q, want %q", i, m.String(), want)
		}
	}
	// Byte B: bits 0-2 supported, bits 4-6 incomplete for misfire, fuel, components.
	m := DecodeMonitorStatus([]byte{0x00, 0x52, 0x00, 0x00})
	if m.Misfire != (Monitor{false, false}) || m.FuelSystem != (Monitor{true, true}) || m.Components != (Monitor{false, false}) {
		t.Errorf("continuous monitors = %+v %+v %+v", m.Misfire, m.FuelSystem, m.Components)
	}
}

// MonitorStatus.String lists supported-but-incomplete monitors only, and
// says all complete when there are none.
func TestMonitorStatusString(t *testing.T) {
	cases := []struct {
		b    []byte
		want string
	}{
		{[]byte{0x00, 0x07, 0x65, 0x00}, "0 DTC(s), all monitors complete"},
		{[]byte{0x00, 0x70, 0x00, 0xFF}, "0 DTC(s), all monitors complete"}, // incomplete but unsupported
		{[]byte{0x81, 0x77, 0xFF, 0xFF}, "MIL ON, 1 DTC(s), incomplete: misfire, fuel, components, catalyst, heated cat, evap, sec air, A/C, O2 sensor, O2 heater, EGR"},
		{[]byte{0x7F, 0x00, 0x00, 0x00}, "127 DTC(s), all monitors complete"},
	}
	for _, c := range cases {
		if got := DecodeMonitorStatus(c.b).String(); got != c.want {
			t.Errorf("% X: %q, want %q", c.b, got, c.want)
		}
	}
}

// TestResult.String shows the limit as a minimum when bit 7 or bit 0 of the
// limit type is set, otherwise as a maximum.
func TestTestResultStringLimitType(t *testing.T) {
	cases := map[byte]string{
		0x00: "TID $01: value 5 (max 9)",
		0x01: "TID $01: value 5 (min 9)",
		0x80: "TID $01: value 5 (min 9)",
	}
	for lt, want := range cases {
		if got := (TestResult{TestID: 1, LimitType: lt, Value: 5, Limit: 9}).String(); got != want {
			t.Errorf("LimitType %02X: %q, want %q", lt, got, want)
		}
	}
}

// unwrapMode9 returns nil for replies shorter than the first message and
// ignores a trailing partial group.
func TestUnwrapMode9Short(t *testing.T) {
	if got := unwrapMode9([]byte{0x01, 0xDE, 0xAD}); got != nil {
		t.Errorf("short = % X, want nil", got)
	}
	got := unwrapMode9([]byte{0x01, 1, 2, 3, 4, 0x06, 0x02, 5, 6})
	if string(got) != "\x01\x02\x03\x04" {
		t.Errorf("partial group = % X, want 01 02 03 04", got)
	}
}
