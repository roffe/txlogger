package j1979

import (
	"bytes"
	"context"
	"fmt"

	"github.com/roffe/txlogger/pkg/dtc"
)

// ---- Mode $01: current data ----

// ReadPID returns the raw data bytes for a mode $01 PID. The T7 echoes the
// PID and returns no data for unsupported PIDs.
func (c *Client) ReadPID(ctx context.Context, pid byte) ([]byte, error) {
	data, err := c.mode(ctx, MODE_CURRENT_DATA, pid)
	if err != nil {
		return nil, err
	}
	if len(data) < 1 || data[0] != pid {
		return nil, fmt.Errorf("mode $01: expected PID $%02X echo, got % X", pid, data)
	}
	if len(data) == 1 {
		return nil, fmt.Errorf("mode $01: PID $%02X not supported", pid)
	}
	return data[1:], nil
}

// ReadPIDValue reads and decodes a mode $01 PID.
func (c *Client) ReadPIDValue(ctx context.Context, pid byte) (PIDValue, error) {
	data, err := c.ReadPID(ctx, pid)
	if err != nil {
		return PIDValue{}, err
	}
	return DecodePID(pid, data), nil
}

// SupportedPIDs walks the mode $01 supported-PID bitmasks ($00, $20, ...)
// and returns every supported PID, including the range PIDs themselves.
// T7 (ENGINE=1) reports: 01 03 04 05 06 07 0B 0C 0D 0E 0F 10 11 13 14 15 1C 20 21.
func (c *Client) SupportedPIDs(ctx context.Context) ([]byte, error) {
	var pids []byte
	for page := byte(0x00); ; page += 0x20 {
		data, err := c.ReadPID(ctx, page)
		if err != nil {
			if page == 0 {
				return nil, err
			}
			break // ECU claimed the page but doesn't answer it; stop walking
		}
		if len(data) < 4 {
			break
		}
		pids = append(pids, maskToIDs(page, data[:4])...)
		if data[3]&0x01 == 0 || page >= 0xE0 {
			break
		}
	}
	return pids, nil
}

// MonitorStatus reads and decodes PID $01 (MIL state, DTC count, readiness).
func (c *Client) MonitorStatus(ctx context.Context) (MonitorStatus, error) {
	data, err := c.ReadPID(ctx, 0x01)
	if err != nil {
		return MonitorStatus{}, err
	}
	if len(data) < 4 {
		return MonitorStatus{}, fmt.Errorf("mode $01 PID $01: short response % X", data)
	}
	return DecodeMonitorStatus(data), nil
}

// ---- Mode $02: freeze frame ----

// ReadFreezeFramePID returns the raw data for a mode $02 PID of freeze frame
// `frame` (0 = the OBD-II mandated frame). PID $02 holds the DTC that stored
// the frame; the T7 supports PIDs 02-11 (same encodings as mode $01).
func (c *Client) ReadFreezeFramePID(ctx context.Context, pid, frame byte) ([]byte, error) {
	data, err := c.mode(ctx, MODE_FREEZE_FRAME, pid, frame)
	if err != nil {
		return nil, err
	}
	if len(data) < 1 || data[0] != pid {
		return nil, fmt.Errorf("mode $02: expected PID $%02X echo, got % X", pid, data)
	}
	if len(data) == 1 {
		return nil, fmt.Errorf("mode $02: PID $%02X not supported", pid)
	}
	return data[1:], nil
}

// FreezeFrame holds a decoded mode $02 snapshot.
type FreezeFrame struct {
	DTC    dtc.DTC
	Values []PIDValue
}

// FreezeFrame reads the complete freeze frame `frame`. Returns nil (no error)
// when the ECU has no freeze frame stored.
func (c *Client) FreezeFrame(ctx context.Context, frame byte) (*FreezeFrame, error) {
	code, err := c.ReadFreezeFramePID(ctx, 0x02, frame)
	if err != nil {
		return nil, err
	}
	if len(code) < 2 {
		return nil, fmt.Errorf("mode $02: short DTC response % X", code)
	}
	if code[0] == 0 && code[1] == 0 {
		return nil, nil // no freeze frame stored
	}
	ff := &FreezeFrame{DTC: dtc.DTC{ECU: dtc.ECU_T7, Code: DecodeDTC(code[0], code[1])}}

	sup, err := c.ReadFreezeFramePID(ctx, 0x00, frame)
	if err != nil {
		return nil, err
	}
	if len(sup) < 4 {
		return nil, fmt.Errorf("mode $02: short supported-PID response % X", sup)
	}
	for _, pid := range maskToIDs(0, sup[:4]) {
		if pid == 0x02 { // already read
			continue
		}
		data, err := c.ReadFreezeFramePID(ctx, pid, frame)
		if err != nil {
			return nil, err
		}
		ff.Values = append(ff.Values, DecodePID(pid, data))
	}
	return ff, nil
}

// ---- Modes $03 / $07: stored & pending DTCs ----

// StoredDTCs returns the confirmed emission-related DTCs (mode $03).
func (c *Client) StoredDTCs(ctx context.Context) ([]dtc.DTC, error) {
	return c.readDTCs(ctx, MODE_STORED_DTCS)
}

// PendingDTCs returns preliminary DTCs and DTCs reported this driving cycle
// (mode $07).
func (c *Client) PendingDTCs(ctx context.Context) ([]dtc.DTC, error) {
	return c.readDTCs(ctx, MODE_PENDING_DTCS)
}

func (c *Client) readDTCs(ctx context.Context, mode byte) ([]dtc.DTC, error) {
	data, err := c.mode(ctx, mode)
	if err != nil {
		return nil, err
	}
	dtcs := make([]dtc.DTC, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			continue // zero padding
		}
		dtcs = append(dtcs, dtc.DTC{ECU: dtc.ECU_T7, Code: DecodeDTC(data[i], data[i+1])})
	}
	return dtcs, nil
}

// ---- Mode $04: clear diagnostic information ----

// ClearDTCs clears emission-related diagnostic information (mode $04): DTCs,
// freeze frames, readiness results — and, on T7, the emission-related
// adaptation values including the fuel trims (AdpFuelAdap.MulFuelAdapt /
// AddFuelAdapt are reset to 0). Do not send this casually on a tuned car.
func (c *Client) ClearDTCs(ctx context.Context) error {
	_, err := c.mode(ctx, MODE_CLEAR_DTCS)
	return err
}

// ---- Mode $05: O2 sensor monitoring test results ----

// O2TestResult is one mode $05 test result: value with its min/max limits,
// scaled per test ID (voltage tests are 5 mV/bit → Volts helper below).
type O2TestResult struct {
	TestID byte
	Sensor byte // 1 = front (bank 1 sensor 1), 2 = rear (bank 1 sensor 2)
	Value  byte
	Min    byte
	Max    byte
}

// Volts converts a 5 mV/bit test byte to Volts (valid for the voltage tests).
func Volts(b byte) float64 { return float64(b) * 0.005 }

// O2SensorTest runs a mode $05 test on a sensor. T7 sensors: 1 = front,
// 2 = rear. Supported test IDs come from O2SupportedTests.
func (c *Client) O2SensorTest(ctx context.Context, testID, sensor byte) (O2TestResult, error) {
	data, err := c.mode(ctx, MODE_O2_TEST_RESULTS, testID, sensor)
	if err != nil {
		return O2TestResult{}, err
	}
	if len(data) < 3 {
		return O2TestResult{}, fmt.Errorf("mode $05: test $%02X sensor %d not supported", testID, sensor)
	}
	return O2TestResult{TestID: testID, Sensor: sensor, Value: data[0], Min: data[1], Max: data[2]}, nil
}

// O2SupportedTests walks the mode $05 supported-test bitmasks ($00, $20, ...)
// for a sensor and returns the supported test IDs.
func (c *Client) O2SupportedTests(ctx context.Context, sensor byte) ([]byte, error) {
	var tids []byte
	for page := byte(0x00); ; page += 0x20 {
		data, err := c.mode(ctx, MODE_O2_TEST_RESULTS, page, sensor)
		if err != nil {
			if page == 0 {
				return nil, err
			}
			break
		}
		if len(data) < 4 {
			break
		}
		tids = append(tids, maskToIDs(page, data[:4])...)
		if data[3]&0x01 == 0 || page >= 0xE0 {
			break
		}
	}
	return tids, nil
}

// ---- Mode $06: on-board monitoring test results ----

// TestResult is one mode $06 result block.
type TestResult struct {
	TestID    byte
	LimitType byte // 0 = value must be below Limit, 1 = above (SAE TestLimitType)
	Value     uint16
	Limit     uint16
}

func (t TestResult) String() string {
	cmp := "max"
	if t.LimitType&0x80 != 0 || t.LimitType&0x01 != 0 {
		cmp = "min"
	}
	return fmt.Sprintf("TID $%02X: value %d (%s %d)", t.TestID, t.Value, cmp, t.Limit)
}

// TestResults runs/reads a mode $06 test. One test ID can return several
// result blocks. T7 test IDs (OBD2 variant): 01-08 continuous, 13 evap,
// 14 tank pressure, 15 brake light, 19 catalyst, 1D cooling system,
// 1E tank adaptation.
func (c *Client) TestResults(ctx context.Context, testID byte) ([]TestResult, error) {
	data, err := c.mode(ctx, MODE_TEST_RESULTS, testID)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("mode $06: test $%02X not supported", testID)
	}
	var res []TestResult
	for i := 0; i+6 <= len(data); i += 6 {
		res = append(res, TestResult{
			TestID:    data[i],
			LimitType: data[i+1],
			Value:     uint16(data[i+2])<<8 | uint16(data[i+3]),
			Limit:     uint16(data[i+4])<<8 | uint16(data[i+5]),
		})
	}
	return res, nil
}

// T7TestIDs is the mode $06 test-ID set the T7 firmware means to advertise
// (Scantool.cpp mode6 case 0 comments): $01-$08 continuous monitors, $13
// evap, $14 tank pressure, $15 brake light, $19 catalyst, $1D cooling
// system, $1E tank adaptation. The bitmask SupportedTests decodes is shifted
// one byte by a firmware off-by-one (an extra 0x00 written before the
// 0x38/0x8C bytes), so probe these IDs in addition to the advertised ones.
var T7TestIDs = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x13, 0x14, 0x15, 0x19, 0x1D, 0x1E}

// SupportedTests returns the mode $06 test IDs the ECU advertises. The T7
// echoes the requested TID $00 and follows with a 5-byte bitmask (not the
// standard 4); the firmware writes that mask misaligned — see T7TestIDs.
func (c *Client) SupportedTests(ctx context.Context) ([]byte, error) {
	data, err := c.mode(ctx, MODE_TEST_RESULTS, 0x00)
	if err != nil {
		return nil, err
	}
	if len(data) < 2 || data[0] != 0x00 {
		return nil, fmt.Errorf("mode $06: unexpected supported-TID response % X", data)
	}
	return maskToIDs(0, data[1:]), nil
}

// ---- Mode $08: control of on-board system ----

// ControlTest requests a mode $08 control operation. The only one the T7
// implements is TID $01: close the EVAP purge valve for leak testing
// (requires engine off, key on, vehicle standing still). The 5 reply bytes
// report acceptance; byte 0 = 0 with no error bits means the test started.
func (c *Client) ControlTest(ctx context.Context, testID byte) ([]byte, error) {
	data, err := c.mode(ctx, MODE_CONTROL, testID)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("mode $08: test $%02X not supported or conditions not met", testID)
	}
	return data, nil
}

// ---- Mode $09: vehicle information ----

// VehicleInfo returns the raw mode $09 payload for an infotype, with the
// pre-CAN multi-message structure intact. Use VIN/CalibrationID/CVN for
// decoded values. T7 infotypes: 1-6 (+7-8 when in-use performance tracking
// is calibrated on).
func (c *Client) VehicleInfo(ctx context.Context, infotype byte) ([]byte, error) {
	data, err := c.mode(ctx, MODE_VEHICLE_INFO, infotype)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("mode $09: infotype $%02X not supported", infotype)
	}
	return data, nil
}

// VIN returns the vehicle identification number (infotype $02).
func (c *Client) VIN(ctx context.Context) (string, error) {
	data, err := c.VehicleInfo(ctx, 0x02)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimLeft(unwrapMode9(data), "\x00")), nil
}

// CalibrationID returns the calibration identification (infotype $04).
func (c *Client) CalibrationID(ctx context.Context) (string, error) {
	data, err := c.VehicleInfo(ctx, 0x04)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimRight(unwrapMode9(data), "\x00 ")), nil
}

// CVN returns the calibration verification number (infotype $06).
func (c *Client) CVN(ctx context.Context) ([]byte, error) {
	data, err := c.VehicleInfo(ctx, 0x06)
	if err != nil {
		return nil, err
	}
	cvn := unwrapMode9(data)
	if len(cvn) < 4 {
		return nil, fmt.Errorf("mode $09: short CVN % X", data)
	}
	return cvn[:4], nil
}

// unwrapMode9 strips the flattened pre-CAN J1979 message structure the T7
// returns: [msgCount, d0 d1 d2 d3] then groups of [infotype, msgNo, d0..d3].
func unwrapMode9(data []byte) []byte {
	if len(data) < 5 {
		return nil
	}
	out := append([]byte{}, data[1:5]...)
	for i := 5; i+6 <= len(data); i += 6 {
		out = append(out, data[i+2:i+6]...)
	}
	return out
}

// DecodeDTC converts a raw 2-byte SAE J2012 trouble code to its string form,
// e.g. 0x01,0x05 → "P0105", 0x43,0x21 → "C0321".
func DecodeDTC(hi, lo byte) string {
	return fmt.Sprintf("%c%d%X%02X", "PCBU"[hi>>6], (hi>>4)&0x03, hi&0x0F, lo)
}

// maskToIDs expands a supported-ID bitmask (MSB of byte 0 = base+1) into the
// list of IDs it marks supported. Works for PID, TID and infotype masks of
// any byte length.
func maskToIDs(base byte, mask []byte) []byte {
	var ids []byte
	for i, b := range mask {
		for bit := 0; bit < 8; bit++ {
			if b&(0x80>>bit) != 0 {
				ids = append(ids, base+byte(i*8+bit)+1)
			}
		}
	}
	return ids
}
