package dtc

import (
	"strings"
)

type ECU int

const (
	ECU_T5 ECU = iota
	ECU_T7
	ECU_T8
)

type DTC struct {
	ECU    ECU
	Code   string
	Status byte
}

func (d DTC) String() string {
	return d.Code
}

func (d DTC) StatusString() string {
	return StatusBytetoString(d.Status)
}

func (d DTC) Info() DTCInfo {
	switch d.ECU {
	case ECU_T5:
		if info, ok := T5DTCS[d.Code]; ok {
			return info
		}
	case ECU_T7:
		if info, ok := T7DTCS[d.Code]; ok {
			return info
		}
	case ECU_T8:
		if info, ok := T8DTCS[d.Code]; ok {
			return info
		}
	}

	return DTCInfo{
		Name: "",
	}
}

/*
DTC Status Byte
bit #	hex		state								description
0		0x01	testFailed							DTC failed at the time of the request
1		0x02	testFailedThisOperationCycle		DTC failed on the current operation cycle
2		0x04	pendingDTC							DTC failed on the current or previous operation cycle
3		0x08	confirmedDTC						DTC is confirmed at the time of the request
4		0x10	testNotCompletedSinceLastClear		DTC test not completed since the last code clear
5		0x20	testFailedSinceLastClear			DTC test failed at least once since last code clear
6		0x40	testNotCompletedThisOperationCycle	DTC test not completed this operation cycle
7		0x80	warningIndicatorRequested			Server is requesting warningIndicator to be active
*/
func StatusBytetoString(status byte) string {
	var statusStrings []string
	if status&0x80 != 0 {
		statusStrings = append(statusStrings, "CEL illuminated")
	}
	if status&0x40 != 0 {
		statusStrings = append(statusStrings, "test not completed this operation cycle")
	}
	if status&0x20 != 0 {
		statusStrings = append(statusStrings, "test failed at least once since last code clear")
	}
	if status&0x10 != 0 {
		statusStrings = append(statusStrings, "test not completed since the last code clear")
	}
	if status&0x08 != 0 {
		statusStrings = append(statusStrings, "confirmed at the time of the request")
	}
	if status&0x04 != 0 {
		statusStrings = append(statusStrings, "failed on the current or previous operation cycle")
	}
	if status&0x02 != 0 {
		statusStrings = append(statusStrings, "failed on the current operation cycle")
	}
	if status&0x01 != 0 {
		statusStrings = append(statusStrings, "failed at the time of the request")
	}
	return strings.Join(statusStrings, ", ")
}

type DTCInfo struct {
	Name        string
	Description string
}
