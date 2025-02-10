package plx

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"go.bug.st/serial"
)

const (
	ProductString = "PLX iMFD Serial Protocol"
)

/*
+--------------------------+-------------------+
|          Sensor          | Address (Decimal) |
+--------------------------+-------------------+
| Wideband Air/Fuel        |         0         |
| Exhaust Gas Temperature  |         1         |
| Fluid Temperature        |         2         |
| Vacuum                   |         3         |
| Boost                    |         4         |
| Air Intake Temperature   |         5         |
| RPM                      |         6         |
| Vehicle Speed            |         7         |
| Throttle Position        |         8         |
| Engine Load              |         9         |
| Fuel Pressure            |        10         |
| Timing                   |        11         |
| MAP                      |        12         |
| MAF                      |        13         |
| Short Term Fuel Trim     |        14         |
| Long Term Fuel Trim      |        15         |
| Narrowband Oxygen Sensor |        16         |
| Fuel Level               |        17         |
| Volt Meter               |        18         |
| Knock                    |        19         |
| Duty Cycle               |        20         |
| Fuel Efficiency          |        21         |
| Analog Voltage		   |        22         |
| Speed (Herz)             |        23         |
| Wideband AFR Status	   |        24         |
| Wideband AFR Health	   |        25         |
| Wideband AFR Reaction	   |        26         |
+--------------------------+-------------------+
*/

type SensorType int

const (
	WidebandAirFuel SensorType = iota
	ExhaustGasTemperature
	FluidTemperature
	Vacuum
	Boost
	AirIntakeTemperature
	RPM
	VehicleSpeed
	ThrottlePosition
	EngineLoad
	FuelPressure
	Timing
	MAP
	MAF
	ShortTermFuelTrim
	LongTermFuelTrim
	NarrowbandOxygenSensor
	FuelLevel
	VoltMeter
	Knock
	DutyCycle
	FuelEfficiency
	AnalogVoltage
	Speed
	WidebandAFRStatus
	WidebandAFRHealth
	WidebandAFRReaction
)

func (s SensorType) String() string {
	switch s {
	case WidebandAirFuel:
		return "Wideband Air/Fuel (0)"
	case ExhaustGasTemperature:
		return "Exhaust Gas Temperature (1)"
	case FluidTemperature:
		return "Fluid Temperature (2)"
	case Vacuum:
		return "Vacuum (3)"
	case Boost:
		return "Boost (4)"
	case AirIntakeTemperature:
		return "Air Intake Temperature (5)"
	case RPM:
		return "RPM (6)"
	case VehicleSpeed:
		return "Vehicle Speed (7)"
	case ThrottlePosition:
		return "Throttle Position (8)"
	case EngineLoad:
		return "Engine Load (9)"
	case FuelPressure:
		return "Fuel Pressure (10)"
	case Timing:
		return "Timing (11)"
	case MAP:
		return "MAP (12)"
	case MAF:
		return "MAF (13)"
	case ShortTermFuelTrim:
		return "Short Term Fuel Trim (14)"
	case LongTermFuelTrim:
		return "Long Term Fuel Trim (15)"
	case NarrowbandOxygenSensor:
		return "Narrowband Oxygen Sensor (16)"
	case FuelLevel:
		return "Fuel Level (17)"
	case VoltMeter:
		return "Volt Meter (18)"
	case Knock:
		return "Knock (19)"
	case DutyCycle:
		return "Duty Cycle (20)"
	case FuelEfficiency:
		return "Fuel Efficiency (21)"
	case AnalogVoltage:
		return "Analog Voltage (22)"
	case Speed:
		return "Speed (23)"
	case WidebandAFRStatus:
		return "Wideband AFR Status (24)"
	case WidebandAFRHealth:
		return "Wideband AFR Health (25)"
	case WidebandAFRReaction:
		return "Wideband AFR Reaction (26)"
	}
	return "Unknown (" + strconv.Itoa(int(s)) + ")"
}

const (
	StartBit = 0x80 // 1000 0000
	StopBit  = 0x40 // 0100 0000
	DataMask = 0x3F // 0011 1111
)

type IMFDClient struct {
	cfg      *IMFDClientConfig
	portName string

	buffer  []byte
	parsing bool

	logFunc func(string)

	values map[string]float64

	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.RWMutex
}

type IMFDClientConfig struct {
	// 0 Lambda
	// 1 Gasoline 14.7
	// 2 Diesel 14.6
	// 3 Methanol 6.4
	// 4 Ethanol 9.0
	// 5 LPG 15.5
	// 6 CNG 17.2
	WidebandAirFuel int

	// 0 Celsius
	// 1 Fahrenheit
	ExhaustGasTemperature int

	// 0 Degrees Celsius Water
	// 1 Degrees Fahrenheit Water
	// 2 Degrees Celsius Oil
	// 3 Degrees Fahrenheit Oil
	FluidTemperature int

	// 0 in/Hg (inch Mercury)
	// 1 mm/Hg (millimeters Mercury)
	Vacuum int

	// 0 0-30 PSI
	// 1 0-2 kg/cm^2
	// 2 0-15 PSI
	// 3 0-1 kg/cm^2
	// 4 0-60 PSI
	// 5 0-4 kg/cm^2
	Boost int

	// 0 Celsius
	// 1 Fahrenheit
	AirIntakeTemperature int

	// 0 MPH
	// 1 km/h
	VehicleSpeed int

	// 0 PSI Fuel
	// 1 kg/cm^2 Fuel
	// 2 Bar Fuel
	// 3 PSI Oil
	// 4 kg/cm^2 Oil
	// 5 Bar Oil
	FuelPressure int

	// 0 kPa
	// 1 inHg
	MAP int

	// 0 g/s (grams per second)
	// 1 lb/min (pounds per minute)
	MAF int

	// 0 Percent
	// 1 Volts
	NarrowbandOxygenSensor int

	// 0 Positive Duty
	// 1 Negative Duty
	DutyCycle int

	// 0 MPG
	// 1 km/L
	// 2 L/100km
	FuelEfficiency int
}

var DefaultIMFDClientConfig = &IMFDClientConfig{
	VehicleSpeed: 1,
}

func NewIMFDClient(port string, cfg *IMFDClientConfig, logFunc func(string)) (*IMFDClient, error) {
	if cfg == nil {
		cfg = DefaultIMFDClientConfig
	}
	client := &IMFDClient{
		cfg:      cfg,
		buffer:   make([]byte, 0, 32),
		parsing:  false,
		portName: port,
		values:   make(map[string]float64),
		closed:   make(chan struct{}),
		logFunc:  logFunc,
	}

	return client, nil
}

func (s *IMFDClient) Parse(p []byte) error {
	for _, b := range p {
		if b == StartBit {
			s.mu.Lock()
			s.buffer = s.buffer[:0]
			s.parsing = true
			s.mu.Unlock()
			continue
		} else if b == StopBit {
			s.mu.Lock()
			s.parsing = false
			if err := s.parsePackets(s.buffer); err != nil {
				return err
			}
			s.mu.Unlock()
			continue
		}
		if s.parsing {
			s.mu.Lock()
			s.buffer = append(s.buffer, b)
			s.mu.Unlock()
		}
	}
	return nil
}

func (s *IMFDClient) Start(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: 19200,
	}

	p, err := serial.Open("COM3", mode)
	if err != nil {
		return err
	}
	p.SetReadTimeout(2)

	buf := make([]byte, 1)
	go func() {
		defer p.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := p.Read(buf)
				if err != nil {
					log.Println(err)
					return
				}
				if n == 0 {
					continue
				}
				if err := s.Parse(buf[:n]); err != nil {
					log.Println(err)
				}
			}
		}
	}()

	return nil
}

func (s *IMFDClient) Stop() {
	s.closeOnce.Do(func() {
		s.logFunc("Stopping IMFD client")
		close(s.closed)
	})
}

func (s *IMFDClient) GetSensor(sensorType SensorType, instance uint8) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.values[createMapkey(sensorType, instance)]; ok {
		return v
	}
	return 0.0
}

func (s *IMFDClient) GetLambda() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.values[createMapkey(WidebandAirFuel, 0)]; ok {
		return v
	}
	return 0.500
}

func (s *IMFDClient) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out strings.Builder
	out.WriteString("-- IMFDClient -----\n")
	for k, v := range s.values {
		out.WriteString(fmt.Sprintf("%s: %.3f\n", k, v))
	}
	out.WriteString(strings.Repeat("-", 20) + "\n")
	return out.String()
}

func (s *IMFDClient) parsePackets(data []byte) error {
	// check that data is a multiple of 5
	if len(data)%5 != 0 {
		return fmt.Errorf("invalid data length: %d", len(data))
	}

	var packets [][]byte
	// Find all packets in the buffer
	for len(data) >= 5 {
		packets = append(packets, data[:5])
		data = data[5:]
	}
	if len(data) > 0 {
		return fmt.Errorf("data left in buffer: %X", data)
	}
	for _, p := range packets {
		if err := s.parsePacket(p); err != nil {
			return err
		}
	}
	return nil
}

func createMapkey(typ SensorType, instance uint8) string {
	return fmt.Sprintf("%s #%d", typ.String(), instance)

}

func (s *IMFDClient) parsePacket(packet []byte) error {
	addrMSB := packet[0] & DataMask
	addrLSB := packet[1] & DataMask
	instance := packet[2] & DataMask
	dataMSB := packet[3] & DataMask
	dataLSB := packet[4] & DataMask
	address := uint16(addrMSB)<<6 | uint16(addrLSB)
	data := uint16(dataMSB)<<6 | uint16(dataLSB)
	st := SensorType(address)
	value, _ := ConvertData(st, 0, data)
	key := createMapkey(st, instance)
	s.values[key] = value
	return nil
}

func ConvertData(sensor SensorType, unit, raw uint16) (float64, string) {
	var retUnit string
	var value float64
	switch sensor {
	case WidebandAirFuel: // Wideband Air/Fuel
		switch unit {
		case 0: // Lambda
			value = (float64(raw)/3.75 + 68) / 100
			retUnit = "λ"
		case 1: // Gasoline 14.7
			value = (float64(raw)/2.55 + 100) / 10
			retUnit = "Gasoline"
		case 2: // Diesel 14.6
			value = (float64(raw)/2.58 + 100) / 10
			retUnit = "Diesel"
		case 3: // Methanol 6.4
			value = (float64(raw)/5.856 + 43.5) / 10
			retUnit = "Methanol"
		case 4: // Ethanol 9.0
			value = (float64(raw)/4.167 + 61.7) / 10
			retUnit = "Ethanol"
		case 5: // LPG 15.5
			value = (float64(raw)/2.417 + 105.6) / 10
			retUnit = "LPG"
		case 6: // CNG 17.2
			value = (float64(raw)/2.18 + 117) / 10
			retUnit = "CNG"
		}
	case ExhaustGasTemperature: // EGT
		switch unit {
		case 0: // Celsius
			value = float64(raw)
			retUnit = "°C"
		case 1: // Fahrenheit
			value = float64(raw)/0.555 + 32
			retUnit = "°F"
		}
	case FluidTemperature: // Fluid Temp
		switch unit {
		case 0: // Degrees Celsius Water
			value = float64(raw)
			retUnit = "°C"
		case 1: // Degrees Fahrenheit Water
			value = float64(raw)/0.555 + 32
			retUnit = "°F"
		case 2: // Degrees Celsius Oil
			value = float64(raw)
			retUnit = "°C"
		case 3: // Degrees Fahrenheit Oil
			value = float64(raw)/0.555 + 32
			retUnit = "°F"
		}
	case Vacuum: // Vac
		switch unit {
		case 0: // in/Hg (inch Mercury)
			value = -(float64(raw)/11.39 - 29.93)
			retUnit = "in/Hg"
		case 1: // mm/Hg (millimeters Mercury)
			value = -(float64(raw)*2.23 + 760.4)
			retUnit = "mm/Hg"
		}
	case Boost: // Boost
		switch unit {
		case 0: // 0-30 PSI
			value = float64(raw) / 22.73
			retUnit = "PSI"
		case 1: // 0-2 kg/cm^2
			value = float64(raw) / 329.47
			retUnit = "kg/cm^2"
		case 2: // 0-15 PSI
			value = float64(raw) / 22.73
			retUnit = "PSI"
		case 3: // 0-1 kg/cm^2
			value = float64(raw) / 329.47
			retUnit = "kg/cm^2"
		case 4: // 0-60 PSI
			value = float64(raw) / 22.73
			retUnit = "PSI"
		case 5: // 0-4 kg/cm^2
			value = float64(raw) / 329.47
			retUnit = "kg/cm^2"
		}
	case AirIntakeTemperature: // AIT
		switch unit {
		case 0: // Celsius
			value = float64(raw)
			retUnit = "°C"
		case 1: // Fahrenheit
			value = float64(raw)/0.555 + 32
			retUnit = "°F"
		}
	case RPM: // RPM
		value = float64(raw) * 19.55
		retUnit = "RPM"
	case VehicleSpeed: // Speed
		switch unit {
		case 0: // MPH
			value = float64(raw) / 6.39
			retUnit = "MPH"
		case 1: // km/h
			value = float64(raw) / 3.97
			retUnit = "km/h"
		}
	case ThrottlePosition: // TPS
		value = float64(raw) //Throttle Position % 0-100
		retUnit = "%"
	case EngineLoad: // Engine Load %
		value = float64(raw)
		retUnit = "%"
	case FuelPressure: // Fluid Pressure
		switch unit {
		case 0: // PSI Fuel
			value = float64(raw) / 5.115
			retUnit = "PSI"
		case 1: // kg/cm^2 Fuel
			value = float64(raw) / 72.73
			retUnit = "kg/cm^2"
		case 2: // Bar Fuel
			value = float64(raw) / 74.22
			retUnit = "Bar"
		case 3: // PSI Oil
			value = float64(raw) / 5.115
			retUnit = "PSI"
		case 4: // kg/cm^2 Oil
			value = float64(raw) / 72.73
			retUnit = "kg/cm^2"
		case 5: // Bar Oil
			value = float64(raw) / 74.22
			retUnit = "Bar"
		}
	case Timing: // Engine timing
		value = float64(raw) - 64
		retUnit = "°"
	case MAP: // MAP
		switch unit {
		case 0: // kPa
			value = float64(raw)
			retUnit = "kPa"
		case 1: // inHg
			value = float64(raw) / 3.386
			retUnit = "inHg"
		}
	case MAF: // MAF
		switch unit {
		case 0: // g/s (grams per second)
			value = float64(raw)
			retUnit = "g/s"
		case 1: // lb/min (pounds per minute)
			value = float64(raw) / 7.54
			retUnit = "lb/min"
		}

	case ShortTermFuelTrim: // Short term fuel trim
		value = float64(raw) - 100
		retUnit = "%"
	case LongTermFuelTrim: // Long term fuel trim
		value = float64(raw) - 100
		retUnit = "%"
	case NarrowbandOxygenSensor: // Narrowband O2 sensor
		switch unit {
		case 0: // Percent
			value = float64(raw)
			retUnit = "%"
		case 1: // Volts
			value = float64(raw) / 78.43
			retUnit = "v"
		}
	case FuelLevel: // Fuel level
		value = float64(raw) //Fuel Level %
		retUnit = "%"
	case VoltMeter: // Volts
		value = float64(raw) / 51.15 //Volt Meter Volts
		retUnit = "v"
	case Knock: // Knock
		value = float64(raw) / 204.6 //Knock volts 0-5
		retUnit = "v"
	case DutyCycle: // Duty cycle
		switch unit {
		case 0: // Positive Duty
			value = float64(raw) / 10.23
			retUnit = "+"
		case 1: // Negative Duty
			value = 100 - (float64(raw) / 10.23)
			retUnit = "-"
		}
	case FuelEfficiency: // Fuel Efficiency
		switch unit {
		case 0: // MPG 0-100
			value = float64(raw)
			retUnit = "MPG"
		case 1: // km/L 0.43 - 42.51
			value = float64(raw) / 1.0
			retUnit = "km/L"
		case 2: // L/100km
			value = 100 / float64(raw)
			retUnit = "L/100km"
		}

	case AnalogVoltage: // Analog Voltage 0-5v
		value = float64(raw) / 51.15
		retUnit = "v"
	case Speed: // Speed (HZ)
		value = float64(raw) * 0.1
		retUnit = "Hz"
	case WidebandAFRStatus: // Wideband AFR Status 0-1
		value = float64(raw)
		retUnit = ""
	case WidebandAFRHealth: // Wideband AFR Health 0-101
		value = float64(raw)
		retUnit = ""
	case WidebandAFRReaction: // Wideband AFR Reaction 0-999ms
		value = float64(raw)
		retUnit = "ms"
	default:
		value = float64(raw)
		retUnit = ""
	}
	return value, retUnit
}
