package ecumaster

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/roffe/gocan"
)

const (
	ProductString = "LAMBDA to CAN"

	CalibrationStart        = "Start"
	CalibrationWaitSPIReset = "Wait for SPI reset"
	CalibrationFinished     = "Finished"
	CalibrationError        = "Error"

	HeaterLowPower   = "Low power"
	HeaterRampUp     = "Ramp up"
	HeaterRegulation = "Regulation"

	LSU_42  = "LSU 4.2"
	LSU_49  = "LSU 4.9"
	LSU_ADV = "LSU ADV"

	Unknown = "Unknown"
)

var (
	ErrDataTooShort = errors.New("data is too short to decode, expected at least 8 bytes")
)

type LambdaToCAN struct {
	c       *gocan.Client
	st      LambdaToCANStatus
	running bool
	mu      sync.Mutex
}

func NewLambdaToCAN(c *gocan.Client) *LambdaToCAN {
	return &LambdaToCAN{c: c}
}

func (l *LambdaToCAN) Start(ctx context.Context) {
	l.running = true
	sub := l.c.Subscribe(ctx, 0x664, 0x665)
	go func() {
		defer sub.Close()
		for l.running {
			select {
			case msg := <-sub.C():
				if msg == nil {
					return //channel closed
				}
				if err := l.decodeCAN(msg.Identifier(), msg.Data()); err != nil {
					log.Println(err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (l *LambdaToCAN) Stop() {
	l.running = false
}

func (l *LambdaToCAN) Status() LambdaToCANStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.st
}

func (l *LambdaToCAN) GetSupplyVoltage() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.st.SupplyVoltage
}

func (l *LambdaToCAN) GetHeaterPower() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.st.HeaterPower
}

func (l *LambdaToCAN) GetSensorTemp() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.st.SensorTemp
}

func (l *LambdaToCAN) GetLambda() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.st.Lambda
}

type LambdaToCANStatus struct {
	SupplyVoltage    float64 // V
	HeaterPower      float64 // %DC
	SensorTemp       float64 // °C
	Lambda           float64 // λ
	VmShortVcc       bool
	VmShortGnd       bool
	UnShortVcc       bool
	UnShortGnd       bool
	IaipShortVcc     bool
	IaipShortGnd     bool
	VubLowVoltage    bool
	HeaterShortVcc   bool
	HeaterShortGnd   bool
	HeaterOpenLoad   bool
	CalibrationState string
	DeviceVersion    string
	IpCurrent        float64 // mA
	OxygenConc       float64 // %
	Resistance       float64 // Ohm
	HeaterState      string
	LambdaValid      bool
}

// PrettyPrint outputs the data in LambdaToCAN664 in a human-readable format.
func (l *LambdaToCAN) PrettyPrint() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out strings.Builder
	out.WriteString(fmt.Sprintf("Supply Voltage:   %.02f V\n", l.st.SupplyVoltage))
	out.WriteString(fmt.Sprintf("Heater Power:     %.02f %%DC\n", l.st.HeaterPower))
	out.WriteString(fmt.Sprintf("Sensor Temperature: %v °C\n", l.st.SensorTemp))
	out.WriteString(fmt.Sprintf("Lambda:           %.02f\n", l.st.Lambda))
	out.WriteString(fmt.Sprintf("VM Short VCC:     %v\n", l.st.VmShortVcc))
	out.WriteString(fmt.Sprintf("VM Short GND:     %v\n", l.st.VmShortGnd))
	out.WriteString(fmt.Sprintf("UN Short VCC:     %v\n", l.st.UnShortVcc))
	out.WriteString(fmt.Sprintf("UN Short GND:     %v\n", l.st.UnShortGnd))
	out.WriteString(fmt.Sprintf("IAIP Short VCC:   %v\n", l.st.IaipShortVcc))
	out.WriteString(fmt.Sprintf("IAIP Short GND:   %v\n", l.st.IaipShortGnd))
	out.WriteString(fmt.Sprintf("VUB Low Voltage:  %v\n", l.st.VubLowVoltage))
	out.WriteString(fmt.Sprintf("Heater Short VCC: %v\n", l.st.HeaterShortVcc))
	out.WriteString(fmt.Sprintf("Heater Short GND: %v\n", l.st.HeaterShortGnd))
	out.WriteString(fmt.Sprintf("Heater Open Load: %v\n", l.st.HeaterOpenLoad))
	out.WriteString(fmt.Sprintf("Calibration State: %s\n", l.st.CalibrationState))
	out.WriteString(fmt.Sprintf("Device Version:    %s\n", l.st.DeviceVersion))
	out.WriteString(fmt.Sprintf("Ip Current:   %.02f mA\n", l.st.IpCurrent))
	out.WriteString(fmt.Sprintf("Oxygen Conc:  %.02f %%\n", l.st.OxygenConc))
	out.WriteString(fmt.Sprintf("Resistance:   %.02f Ohm\n", l.st.Resistance))
	out.WriteString(fmt.Sprintf("Heater State: %s\n", l.st.HeaterState))
	out.WriteString(fmt.Sprintf("Lambda Valid: %v\n", l.st.LambdaValid))
	return out.String()
}

func (l *LambdaToCAN) decodeCAN(id uint32, data []byte) error {
	switch id {
	case 0x664:
		return l.decodeCAN664Data(data)
	case 0x665:
		return l.decodeCAN665Data(data)
	default:
		return fmt.Errorf("unknown identifier %x", id)
	}
}

func (l *LambdaToCAN) decodeCAN664Data(data []byte) error {
	if len(data) < 8 {
		return ErrDataTooShort
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st.SupplyVoltage = float64(binary.BigEndian.Uint16(data[0:2])) / 100
	l.st.HeaterPower = (float64(data[2]) / 255) * 100
	l.st.SensorTemp = float64(data[3]) * 4
	l.st.Lambda = float64(binary.BigEndian.Uint16(data[4:6])) / 1000
	l.st.VmShortVcc = (data[7] & 0x01) != 0
	l.st.VmShortGnd = (data[7] & 0x02) != 0
	l.st.UnShortVcc = (data[7] & 0x04) != 0
	l.st.UnShortGnd = (data[7] & 0x08) != 0
	l.st.IaipShortVcc = (data[7] & 0x10) != 0
	l.st.IaipShortGnd = (data[7] & 0x20) != 0
	l.st.VubLowVoltage = (data[7] & 0x40) != 0
	l.st.HeaterShortVcc = (data[7] & 0x80) != 0
	l.st.HeaterShortGnd = (data[6] & 0x01) != 0
	l.st.HeaterOpenLoad = (data[6] & 0x02) != 0
	l.st.CalibrationState = calibrationStateDescription((data[6] << 3) >> 5 & 0x07)
	l.st.DeviceVersion = deviceVersionDescription((data[6] >> 5) & 0x07)
	return nil
}

func (l *LambdaToCAN) decodeCAN665Data(data []byte) error {
	if len(data) < 8 {
		return ErrDataTooShort
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	rawIpCurrent := int16(binary.BigEndian.Uint16(data[0:2]))
	rawOxygenConc := int16(binary.BigEndian.Uint16(data[2:4]))
	rawResistance := binary.BigEndian.Uint16(data[4:6])

	l.st.IpCurrent = float64(rawIpCurrent) / 1000
	l.st.OxygenConc = float64(rawOxygenConc) / 100
	l.st.Resistance = float64(rawResistance) / 10
	l.st.HeaterState = heaterStateDescription(data[6])
	l.st.LambdaValid = (data[7] & 0x01) == 0x01
	return nil
}

func heaterStateDescription(value uint8) string {
	switch value {
	case 2:
		return HeaterLowPower
	case 4:
		return HeaterRampUp
	case 7:
		return HeaterRegulation
	default:
		return Unknown
	}
}

// calibrationStateDescription returns the description for a calibration state value.
func calibrationStateDescription(value uint8) string {
	switch value {
	case 0:
		return CalibrationStart
	case 1:
		return CalibrationWaitSPIReset
	case 2:
		return CalibrationFinished
	case 3:
		return CalibrationError
	default:
		return Unknown
	}
}

// deviceVersionDescription returns the description for a device version value.
func deviceVersionDescription(value uint8) string {
	switch value {
	case 0:
		return LSU_42
	case 1:
		return LSU_49
	case 2:
		return LSU_ADV
	default:
		return Unknown
	}
}
