package j1979

import (
	"math"
	"reflect"
	"testing"
)

func TestDecodeDTC(t *testing.T) {
	cases := []struct {
		hi, lo byte
		want   string
	}{
		{0x01, 0x05, "P0105"},
		{0x03, 0x00, "P0300"},
		{0x1A, 0xBC, "P1ABC"},
		{0x43, 0x21, "C0321"},
		{0x81, 0x23, "B0123"},
		{0xC1, 0x55, "U0155"},
	}
	for _, c := range cases {
		if got := DecodeDTC(c.hi, c.lo); got != c.want {
			t.Errorf("DecodeDTC(%02X,%02X) = %s, want %s", c.hi, c.lo, got, c.want)
		}
	}
}

func TestDecodePID(t *testing.T) {
	// Firmware encodings from Scantool.cpp mode1
	cases := []struct {
		pid  byte
		data []byte
		want float64
	}{
		{0x05, []byte{130}, 90},          // 90 °C coolant
		{0x06, []byte{0x80 + 32}, 25},    // +2500 (25%) trim → 160
		{0x07, []byte{0x80 - 32}, -25},   // -2500 → 96
		{0x0C, []byte{0x1F, 0x40}, 2000}, // 2000 rpm * 4 = 8000
		{0x0E, []byte{148}, 10},          // 10.0° BTDC → 128+100/5/2... firmware: 128+fi/5, fi=100
		{0x10, []byte{0x07, 0xD0}, 20},   // 20 g/s
		{0x21, []byte{0x00, 0x64}, 100},  // 100 km MIL distance
	}
	for _, c := range cases {
		got := DecodePID(c.pid, c.data)
		if math.Abs(got.Value-c.want) > 0.01 {
			t.Errorf("DecodePID($%02X, % X) = %v, want %v", c.pid, c.data, got.Value, c.want)
		}
	}
	// Unknown PID keeps raw and NaN
	if v := DecodePID(0x42, []byte{1, 2}); !math.IsNaN(v.Value) {
		t.Errorf("unknown PID should decode to NaN, got %v", v.Value)
	}
}

func TestMaskToIDs(t *testing.T) {
	// T7 mode $01 PID $00 answer: BE 3F B8 11
	got := maskToIDs(0, []byte{0xBE, 0x3F, 0xB8, 0x11})
	want := []byte{0x01, 0x03, 0x04, 0x05, 0x06, 0x07, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x13, 0x14, 0x15, 0x1C, 0x20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("maskToIDs = %02X, want %02X", got, want)
	}
	// Page 2: only PID $21
	if got := maskToIDs(0x20, []byte{0x80, 0x00, 0x00, 0x00}); !reflect.DeepEqual(got, []byte{0x21}) {
		t.Errorf("maskToIDs page 2 = %02X, want [21]", got)
	}
}

func TestUnwrapMode9VIN(t *testing.T) {
	// Firmware layout (Scantool.cpp mode9 infotype 2): 29 bytes
	vin := "YS3DF78K537000001"
	data := []byte{0x01, 0x00, 0x00, 0x00, vin[0]}
	for g := 0; g < 4; g++ {
		data = append(data, 0x02, byte(g+2))
		data = append(data, vin[1+g*4:5+g*4]...)
	}
	if len(data) != 29 {
		t.Fatalf("test data length %d, want 29", len(data))
	}
	got := unwrapMode9(data)
	// 3 leading nulls then the 17 VIN chars
	if string(got[3:]) != vin {
		t.Errorf("unwrapMode9 VIN = %q, want %q", string(got[3:]), vin)
	}
}

func TestDecodeMonitorStatus(t *testing.T) {
	// MIL on, 2 DTCs, continuous supported+complete,
	// cat/evap/O2/O2heater supported (T7 OBD2: C=0x65), evap incomplete
	m := DecodeMonitorStatus([]byte{0x82, 0x07, 0x65, 0x04})
	if !m.MILOn || m.DTCCount != 2 {
		t.Errorf("MIL/count wrong: %+v", m)
	}
	if !m.Misfire.Supported || !m.Misfire.Complete {
		t.Errorf("misfire wrong: %+v", m.Misfire)
	}
	if !m.Catalyst.Supported || !m.Evap.Supported || !m.O2Sensor.Supported || !m.O2Heater.Supported {
		t.Errorf("supported flags wrong: %+v", m)
	}
	if m.Evap.Complete {
		t.Error("evap should be incomplete")
	}
	if m.EGR.Supported {
		t.Error("EGR should be unsupported on T7")
	}
}

func TestTestResultString(t *testing.T) {
	r := TestResult{TestID: 0x19, LimitType: 0, Value: 123, Limit: 200}
	if r.String() != "TID $19: value 123 (max 200)" {
		t.Errorf("got %q", r.String())
	}
}
