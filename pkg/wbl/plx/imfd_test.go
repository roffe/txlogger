package plx

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// plxPacket builds one 5-byte sensor packet: 12-bit address, instance and
// 12-bit data, six bits per byte.
func plxPacket(addr sensorType, instance uint8, data uint16) []byte {
	return []byte{byte(addr>>6) & DataMask, byte(addr) & DataMask, instance & DataMask, byte(data>>6) & DataMask, byte(data) & DataMask}
}

// plxFrame wraps sensor packets in the start and stop bytes.
func plxFrame(packets ...[]byte) []byte {
	f := []byte{StartBit}
	for _, p := range packets {
		f = append(f, p...)
	}
	return append(f, StopBit)
}

func newTestClient(t *testing.T) *IMFDClient {
	t.Helper()
	c, err := NewIMFDClient("test", nil, func(string) {})
	if err != nil {
		t.Fatalf("NewIMFDClient: %v", err)
	}
	return c
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

type reading struct {
	sensor   sensorType
	instance uint8
	want     float64
}

func checkReadings(t *testing.T, c *IMFDClient, readings []reading) {
	t.Helper()
	for _, r := range readings {
		if got := c.GetSensor(r.sensor, r.instance); !near(got, r.want) {
			t.Errorf("%s #%d = %v, want %v", r.sensor, r.instance, got, r.want)
		}
	}
}

// The byte streams from PLXApp018's examples decode to one reading per
// sensor and instance; wideband values are λ and EGT is °C. Expected λ is
// the spec's (raw/3.75+68)/100 worked out by hand: raw 10, 4 and 5 give
// 0.70667, 0.69067 and 0.69333.
func TestParseSpecExamples(t *testing.T) {
	tests := []struct {
		name     string
		in       []byte
		readings []reading
		lambda   float64
	}{
		{
			name:     "1 SM-AFR",
			in:       []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x0A, 0x40},
			readings: []reading{{WidebandAirFuel, 0, 0.706666666667}},
			lambda:   0.706666666667,
		},
		{
			name: "1 SM-AFR and 1 SM-EGT",
			in: []byte{0x80,
				0x00, 0x00, 0x00, 0x00, 0x04,
				0x00, 0x01, 0x00, 0x00, 0x0F,
				0x40},
			readings: []reading{{WidebandAirFuel, 0, 0.690666666667}, {ExhaustGasTemperature, 0, 15}},
			lambda:   0.690666666667,
		},
		{
			name: "2 SM-AFR and 1 SM-EGT",
			in: []byte{0x80,
				0x00, 0x00, 0x00, 0x00, 0x04,
				0x00, 0x00, 0x01, 0x00, 0x05,
				0x00, 0x01, 0x00, 0x00, 0x0F,
				0x40},
			readings: []reading{
				{WidebandAirFuel, 0, 0.690666666667},
				{WidebandAirFuel, 1, 0.693333333333},
				{ExhaustGasTemperature, 0, 15},
			},
			lambda: 0.690666666667,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			if err := c.Parse(tt.in); err != nil {
				t.Fatalf("Parse: %v", err)
			}
			checkReadings(t, c, tt.readings)
			if got := c.GetLambda(); !near(got, tt.lambda) {
				t.Errorf("GetLambda = %v, want %v", got, tt.lambda)
			}
		})
	}
}

// Before any wideband reading GetLambda reports 0.5 and unknown sensors 0.
func TestDefaults(t *testing.T) {
	c := newTestClient(t)
	if err := c.Parse(plxFrame(plxPacket(ExhaustGasTemperature, 0, 800))); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.GetLambda(); got != 0.5 {
		t.Errorf("GetLambda = %v, want 0.5 without a wideband reading", got)
	}
	if got := c.GetSensor(ExhaustGasTemperature, 1); got != 0 {
		t.Errorf("EGT #1 = %v, want 0 for an instance never seen", got)
	}
}

// A frame split across reads is applied only when its stop byte arrives.
func TestParseSplitInput(t *testing.T) {
	c := newTestClient(t)
	f := plxFrame(plxPacket(WidebandAirFuel, 0, 120), plxPacket(RPM, 0, 100))
	for i, b := range f {
		if err := c.Parse([]byte{b}); err != nil {
			t.Fatalf("Parse byte %d: %v", i, err)
		}
		if i < len(f)-1 && c.GetLambda() != 0.5 {
			t.Fatalf("reading applied after byte %d of %d", i+1, len(f))
		}
	}
	checkReadings(t, c, []reading{{WidebandAirFuel, 0, 1.0}, {RPM, 0, 1955}})
}

// Noise outside frames is ignored, a start byte restarts a frame, a frame
// that isn't whole packets is rejected without touching the readings, and
// the two reserved top bits of the payload bytes are ignored.
func TestParseGarbledInput(t *testing.T) {
	good := plxFrame(plxPacket(WidebandAirFuel, 0, 120))
	tests := []struct {
		name    string
		chunks  [][]byte
		wantErr string // for the last chunk
		lambda  float64
	}{
		{"noise before the first start byte", [][]byte{{0x01, 0x02, 0x03, 0x3F}, good}, "", 1.0},
		{"stop byte with no frame", [][]byte{{StopBit}}, "", 0.5},
		{"empty frame", [][]byte{{StartBit, StopBit}}, "", 0.5},
		{"start byte mid-frame restarts it", [][]byte{{StartBit, 0x00, 0x00}, good}, "", 1.0},
		{"frame missing a byte", [][]byte{good, append(plxFrame(plxPacket(WidebandAirFuel, 0, 30))[:5], StopBit)}, "invalid data length: 4", 1.0},
		{"frame with an extra byte", [][]byte{good, append(plxFrame(plxPacket(WidebandAirFuel, 0, 30))[:6], 0x01, StopBit)}, "invalid data length: 6", 1.0},
		{"reserved bits set", [][]byte{{StartBit, 0xC0, 0xC0, 0xC0, 0xC1, 0xF8, StopBit}}, "", 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			var err error
			for _, ch := range tt.chunks {
				err = c.Parse(ch)
			}
			if tt.wantErr == "" && err != nil {
				t.Errorf("Parse: %v", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Errorf("Parse error = %v, want %q", err, tt.wantErr)
			}
			if got := c.GetLambda(); !near(got, tt.lambda) {
				t.Errorf("GetLambda = %v, want %v", got, tt.lambda)
			}
		})
	}
}

// Address and data are 12 bits each (MSB byte << 6 | LSB byte); addresses
// beyond the known sensors are stored raw.
func TestParseTwelveBitFields(t *testing.T) {
	c := newTestClient(t)
	if err := c.Parse(plxFrame(plxPacket(RPM, 0, 0xFFF), plxPacket(sensorType(69), 2, 1234))); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkReadings(t, c, []reading{{RPM, 0, 80057.25}, {sensorType(69), 2, 1234}})
}

// The Parse call that ends a malformed frame returns its error, and the next
// call decodes normally. Both callers feed Parse that way: the serial reader
// one byte at a time, and txbridge one start-to-stop frame per message.
func TestParseRecoversAfterMalformedFrame(t *testing.T) {
	c := newTestClient(t)
	if err := c.Parse([]byte{StartBit, 0x00, 0x00, 0x00, StopBit}); err == nil || err.Error() != "invalid data length: 3" {
		t.Fatalf("Parse error = %v, want invalid data length: 3", err)
	}
	if err := c.Parse(plxFrame(plxPacket(WidebandAirFuel, 0, 120))); err != nil {
		t.Fatalf("Parse of the next frame: %v", err)
	}
	if got := c.GetLambda(); !near(got, 1.0) {
		t.Errorf("GetLambda = %v, want 1.0 from the frame after the malformed one", got)
	}
}

// Readings are converted with the units chosen in the client config; the
// default config shows vehicle speed in km/h.
func TestConfigUnits(t *testing.T) {
	c := newTestClient(t)
	if err := c.Parse(plxFrame(plxPacket(VehicleSpeed, 0, 397))); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkReadings(t, c, []reading{{VehicleSpeed, 0, 100}})
}

// sensorType names follow the PLXApp018 address table (plus the newer
// sensors 21-26) and fall back to "Unknown (n)".
func TestSensorTypeString(t *testing.T) {
	want := []string{
		"Wideband Air/Fuel", "Exhaust Gas Temperature", "Fluid Temperature", "Vacuum", "Boost",
		"Air Intake Temperature", "RPM", "Vehicle Speed", "Throttle Position", "Engine Load",
		"Fuel Pressure", "Timing", "MAP", "MAF", "Short Term Fuel Trim", "Long Term Fuel Trim",
		"Narrowband Oxygen Sensor", "Fuel Level", "Volt Meter", "Knock", "Duty Cycle",
		"Fuel Efficiency", "Analog Voltage", "Speed", "Wideband AFR Status", "Wideband AFR Health",
		"Wideband AFR Reaction", "Unknown",
	}
	for i, name := range want {
		if got, w := sensorType(i).String(), name+" ("+strconv.Itoa(i)+")"; got != w {
			t.Errorf("sensorType(%d) = %q, want %q", i, got, w)
		}
	}
}

// convertData matches ConveriMFDRaw2Data from PLXApp018 for every sensor and
// unit. Expected values are the spec's formulas worked out by hand: raw 120
// on the wideband is λ 1, i.e. each fuel's stoich AFR, and 100 °C on the EGT
// is 212.18 °F (the spec divides by .555 instead of multiplying by 9/5).
func TestConvertData(t *testing.T) {
	tests := []struct {
		name   string
		sensor sensorType
		unit   uint16
		raw    uint16
		want   float64
		unitS  string
	}{
		{"wideband lambda", WidebandAirFuel, 0, 120, 1.0, "λ"},
		{"wideband gasoline", WidebandAirFuel, 1, 120, 14.7058823529, "Gasoline"},
		{"wideband diesel", WidebandAirFuel, 2, 120, 14.6511627907, "Diesel"},
		{"wideband methanol", WidebandAirFuel, 3, 120, 6.39918032787, "Methanol"},
		{"wideband ethanol", WidebandAirFuel, 4, 120, 9.04976961843, "Ethanol"},
		{"wideband LPG", WidebandAirFuel, 5, 120, 15.5248324369, "LPG"},
		{"wideband CNG", WidebandAirFuel, 6, 120, 17.204587156, "CNG"},
		{"wideband unsupported unit", WidebandAirFuel, 7, 120, 0, ""},
		{"EGT celsius", ExhaustGasTemperature, 0, 850, 850, "°C"},
		{"EGT fahrenheit", ExhaustGasTemperature, 1, 100, 212.18018018, "°F"},
		{"water celsius", FluidTemperature, 0, 90, 90, "°C"},
		{"water fahrenheit", FluidTemperature, 1, 90, 194.162162162, "°F"},
		{"oil celsius", FluidTemperature, 2, 110, 110, "°C"},
		{"oil fahrenheit", FluidTemperature, 3, 110, 230.198198198, "°F"},
		{"vacuum inHg", Vacuum, 0, 100, 21.1503687445, "in/Hg"},
		{"vacuum mmHg", Vacuum, 1, 100, -983.4, "mm/Hg"},
		{"boost 0-30 PSI", Boost, 0, 500, 21.9973603168, "PSI"},
		{"boost 0-2 kg/cm2", Boost, 1, 500, 1.51758885483, "kg/cm^2"},
		{"boost 0-15 PSI", Boost, 2, 500, 21.9973603168, "PSI"},
		{"boost 0-1 kg/cm2", Boost, 3, 500, 1.51758885483, "kg/cm^2"},
		{"boost 0-60 PSI", Boost, 4, 500, 21.9973603168, "PSI"},
		{"boost 0-4 kg/cm2", Boost, 5, 500, 1.51758885483, "kg/cm^2"},
		{"AIT celsius", AirIntakeTemperature, 0, 30, 30, "°C"},
		{"AIT fahrenheit", AirIntakeTemperature, 1, 30, 86.0540540541, "°F"},
		{"RPM", RPM, 0, 300, 5865, "RPM"},
		{"speed MPH", VehicleSpeed, 0, 639, 100, "MPH"},
		{"speed km/h", VehicleSpeed, 1, 397, 100, "km/h"},
		{"TPS", ThrottlePosition, 0, 42, 42, "%"},
		{"engine load", EngineLoad, 0, 77, 77, "%"},
		{"fuel PSI", FuelPressure, 0, 300, 58.651026393, "PSI"},
		{"fuel kg/cm2", FuelPressure, 1, 300, 4.1248453183, "kg/cm^2"},
		{"fuel bar", FuelPressure, 2, 300, 4.04203718674, "Bar"},
		{"oil PSI", FuelPressure, 3, 300, 58.651026393, "PSI"},
		{"oil kg/cm2", FuelPressure, 4, 300, 4.1248453183, "kg/cm^2"},
		{"oil bar", FuelPressure, 5, 300, 4.04203718674, "Bar"},
		{"timing", Timing, 0, 84, 20, "°"},
		{"MAP kPa", MAP, 0, 180, 180, "kPa"},
		{"MAP inHg", MAP, 1, 180, 53.1600708801, "inHg"},
		{"MAF g/s", MAF, 0, 55, 55, "g/s"},
		{"MAF lb/min", MAF, 1, 55, 7.29442970822, "lb/min"},
		{"STFT", ShortTermFuelTrim, 0, 95, -5, "%"},
		{"LTFT", LongTermFuelTrim, 0, 103, 3, "%"},
		{"narrowband percent", NarrowbandOxygenSensor, 0, 50, 50, "%"},
		{"narrowband volts", NarrowbandOxygenSensor, 1, 50, 0.637511156445, "v"},
		{"fuel level", FuelLevel, 0, 60, 60, "%"},
		{"volt meter", VoltMeter, 0, 720, 14.0762463343, "v"},
		{"knock", Knock, 0, 300, 1.46627565982, "v"},
		{"duty positive", DutyCycle, 0, 512, 50.0488758553, "+"},
		{"duty negative", DutyCycle, 1, 512, 49.9511241447, "-"},
		{"fuel efficiency MPG", FuelEfficiency, 0, 30, 30, "MPG"},
		{"fuel efficiency km/L", FuelEfficiency, 1, 30, 30, "km/L"},
		{"fuel efficiency L/100km", FuelEfficiency, 2, 8, 12.5, "L/100km"},
		{"analog voltage", AnalogVoltage, 0, 255, 4.9853372434, "v"},
		{"speed Hz", Speed, 0, 1234, 123.4, "Hz"},
		{"wideband status", WidebandAFRStatus, 0, 1, 1, ""},
		{"wideband health", WidebandAFRHealth, 0, 101, 101, ""},
		{"wideband reaction", WidebandAFRReaction, 0, 250, 250, "ms"},
		{"unknown sensor", sensorType(40), 0, 77, 77, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, unit := convertData(tt.sensor, tt.unit, tt.raw)
			if !near(got, tt.want) || unit != tt.unitS {
				t.Errorf("convertData(%v, %d, %d) = %v %q, want %v %q", tt.sensor, tt.unit, tt.raw, got, unit, tt.want, tt.unitS)
			}
		})
	}
}

// String lists every reading as "<sensor> #<instance>: <value>" between rules.
func TestString(t *testing.T) {
	c := newTestClient(t)
	if err := c.Parse(plxFrame(plxPacket(WidebandAirFuel, 0, 120))); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "-- IMFDClient -----\nWideband Air/Fuel (0) #0: 1.000\n" + strings.Repeat("-", 20) + "\n"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	if err := c.Parse(plxFrame(plxPacket(WidebandAirFuel, 0, 120), plxPacket(RPM, 1, 100))); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.String(); !strings.Contains(got, "\nRPM (6) #1: 1955.000\n") || strings.Count(got, "\n") != 4 {
		t.Errorf("String() = %q, want two readings", got)
	}
}

// Stop logs once however often it's called.
func TestStopIdempotent(t *testing.T) {
	var n int
	c, _ := NewIMFDClient("test", nil, func(s string) {
		if s == "Stopping IMFD client" {
			n++
		}
	})
	c.Stop()
	c.Stop()
	if n != 1 {
		t.Errorf("logged Stopping %d times, want 1", n)
	}
}

// Start returns the open error when the port doesn't exist.
func TestStartMissingPort(t *testing.T) {
	c, _ := NewIMFDClient(filepath.Join(t.TempDir(), "ttyUSB9"), nil, func(string) {})
	if err := c.Start(context.Background()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Start error = %v, want a does-not-exist error", err)
	}
}

// Parsing in one goroutine while another reads the values is safe.
func TestConcurrentParseAndGetters(t *testing.T) {
	c := newTestClient(t)
	f := plxFrame(plxPacket(WidebandAirFuel, 0, 120), plxPacket(RPM, 0, 100))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			c.Parse(f[:4])
			c.Parse(f[4:])
		}
	}()
	for {
		select {
		case <-done:
			checkReadings(t, c, []reading{{WidebandAirFuel, 0, 1.0}, {RPM, 0, 1955}})
			return
		default:
			_ = c.GetLambda()
			_ = c.GetSensor(RPM, 0)
			_ = c.String()
		}
	}
}
