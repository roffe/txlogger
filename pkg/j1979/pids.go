package j1979

import (
	"fmt"
	"math"
	"strings"
)

// PIDValue is a decoded mode $01/$02 PID.
type PIDValue struct {
	PID   byte
	Name  string
	Raw   []byte
	Value float64 // NaN for non-numeric PIDs (bitmasks, enums); see String()
	Unit  string
}

func (v PIDValue) String() string {
	if !math.IsNaN(v.Value) {
		return strings.TrimSpace(fmt.Sprintf("%s: %.2f %s", v.Name, v.Value, v.Unit))
	}
	switch v.PID {
	case 0x01:
		if len(v.Raw) >= 4 {
			return v.Name + ": " + DecodeMonitorStatus(v.Raw).String()
		}
	case 0x02:
		if len(v.Raw) >= 2 {
			if v.Raw[0] == 0 && v.Raw[1] == 0 {
				return v.Name + ": none"
			}
			return v.Name + ": " + DecodeDTC(v.Raw[0], v.Raw[1])
		}
	case 0x03:
		if len(v.Raw) >= 1 {
			return v.Name + ": " + fuelSystemStatus(v.Raw[0])
		}
	case 0x1C:
		if len(v.Raw) >= 1 {
			return v.Name + ": " + obdStandard(v.Raw[0])
		}
	}
	return fmt.Sprintf("%s: % X", v.Name, v.Raw)
}

var nan = math.NaN()

type pidDef struct {
	name   string
	unit   string
	decode func([]byte) float64
}

func a(b []byte) float64 { return float64(b[0]) }
func ab(b []byte) float64 {
	if len(b) < 2 {
		return nan
	}
	return float64(uint16(b[0])<<8 | uint16(b[1]))
}
func fuelTrim(b []byte) float64 { return (float64(b[0]) - 128) * 100 / 128 }

// Standard SAE J1979 decoders for the PIDs the T7 implements (the firmware
// normalizes all values to CARB scaling — Scantool.cpp SaeJ1979Comm::mode1).
var pidDefs = map[byte]pidDef{
	0x00: {"Supported PIDs $01-$20", "", nil},
	0x01: {"Monitor status since DTCs cleared", "", nil},
	0x02: {"Freeze frame DTC", "", nil},
	0x03: {"Fuel system status", "", nil},
	0x04: {"Calculated engine load", "%", func(b []byte) float64 { return float64(b[0]) * 100 / 255 }},
	0x05: {"Engine coolant temperature", "°C", func(b []byte) float64 { return float64(b[0]) - 40 }},
	0x06: {"Short term fuel trim", "%", fuelTrim},
	0x07: {"Long term fuel trim", "%", fuelTrim},
	0x0B: {"Intake manifold absolute pressure", "kPa", a},
	0x0C: {"Engine speed", "rpm", func(b []byte) float64 { return ab(b) / 4 }},
	0x0D: {"Vehicle speed", "km/h", a},
	0x0E: {"Timing advance", "°BTDC", func(b []byte) float64 { return float64(b[0])/2 - 64 }},
	0x0F: {"Intake air temperature", "°C", func(b []byte) float64 { return float64(b[0]) - 40 }},
	0x10: {"MAF air flow rate", "g/s", func(b []byte) float64 { return ab(b) / 100 }},
	0x11: {"Throttle position", "%", func(b []byte) float64 { return float64(b[0]) * 100 / 255 }},
	0x13: {"O2 sensors present", "", nil},
	0x14: {"O2 sensor 1 voltage", "V", func(b []byte) float64 { return float64(b[0]) / 200 }},
	0x15: {"O2 sensor 2 voltage", "V", func(b []byte) float64 { return float64(b[0]) / 200 }},
	0x1C: {"OBD standard", "", nil},
	0x20: {"Supported PIDs $21-$40", "", nil},
	0x21: {"Distance traveled with MIL on", "km", ab},
}

// DecodePID decodes raw mode $01/$02 PID data into a PIDValue. Unknown PIDs
// come back named "PID $XX" with Value NaN and the raw bytes intact.
func DecodePID(pid byte, data []byte) PIDValue {
	v := PIDValue{PID: pid, Name: fmt.Sprintf("PID $%02X", pid), Raw: data, Value: nan}
	def, ok := pidDefs[pid]
	if !ok {
		return v
	}
	v.Name = def.name
	v.Unit = def.unit
	if def.decode != nil && len(data) >= 1 {
		v.Value = def.decode(data)
	}
	return v
}

// Monitor is one readiness monitor's state.
type Monitor struct {
	Supported bool
	Complete  bool
}

// MonitorStatus is decoded PID $01.
type MonitorStatus struct {
	MILOn    bool
	DTCCount int
	// Continuous monitors
	Misfire    Monitor
	FuelSystem Monitor
	Components Monitor
	// Non-continuous monitors (spark ignition)
	Catalyst       Monitor
	HeatedCatalyst Monitor
	Evap           Monitor
	SecondaryAir   Monitor
	ACRefrigerant  Monitor
	O2Sensor       Monitor
	O2Heater       Monitor
	EGR            Monitor
}

// DecodeMonitorStatus decodes the 4 data bytes of PID $01.
func DecodeMonitorStatus(b []byte) MonitorStatus {
	m := MonitorStatus{
		MILOn:    b[0]&0x80 != 0,
		DTCCount: int(b[0] & 0x7F),
		// Byte B: bits 0-2 = supported, bits 4-6 = incomplete
		Misfire:    Monitor{b[1]&0x01 != 0, b[1]&0x10 == 0},
		FuelSystem: Monitor{b[1]&0x02 != 0, b[1]&0x20 == 0},
		Components: Monitor{b[1]&0x04 != 0, b[1]&0x40 == 0},
	}
	// Byte C = supported, byte D = incomplete
	mon := []*Monitor{&m.Catalyst, &m.HeatedCatalyst, &m.Evap, &m.SecondaryAir, &m.ACRefrigerant, &m.O2Sensor, &m.O2Heater, &m.EGR}
	for i, p := range mon {
		bit := byte(1 << i)
		*p = Monitor{b[2]&bit != 0, b[3]&bit == 0}
	}
	return m
}

func (m MonitorStatus) String() string {
	var sb strings.Builder
	if m.MILOn {
		sb.WriteString("MIL ON, ")
	}
	fmt.Fprintf(&sb, "%d DTC(s)", m.DTCCount)
	named := []struct {
		n string
		m Monitor
	}{
		{"misfire", m.Misfire},
		{"fuel", m.FuelSystem},
		{"components", m.Components},
		{"catalyst", m.Catalyst},
		{"heated cat", m.HeatedCatalyst},
		{"evap", m.Evap},
		{"sec air", m.SecondaryAir},
		{"A/C", m.ACRefrigerant},
		{"O2 sensor", m.O2Sensor},
		{"O2 heater", m.O2Heater},
		{"EGR", m.EGR},
	}
	var incomplete []string
	for _, x := range named {
		if x.m.Supported && !x.m.Complete {
			incomplete = append(incomplete, x.n)
		}
	}
	if len(incomplete) > 0 {
		sb.WriteString(", incomplete: " + strings.Join(incomplete, ", "))
	} else {
		sb.WriteString(", all monitors complete")
	}
	return sb.String()
}

func fuelSystemStatus(b byte) string {
	switch b {
	case 0x01:
		return "open loop (conditions not yet met)"
	case 0x02:
		return "closed loop"
	case 0x04:
		return "open loop (driving conditions)"
	case 0x08:
		return "open loop (system fault)"
	case 0x10:
		return "closed loop with fault"
	}
	return fmt.Sprintf("unknown ($%02X)", b)
}

func obdStandard(b byte) string {
	switch b {
	case 0x01:
		return "OBD-II (CARB)"
	case 0x02:
		return "OBD (EPA)"
	case 0x03:
		return "OBD and OBD-II"
	case 0x04:
		return "OBD-I"
	case 0x05:
		return "Not OBD compliant"
	case 0x06:
		return "EOBD"
	}
	return fmt.Sprintf("unknown ($%02X)", b)
}
