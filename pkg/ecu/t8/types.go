package t8

import (
	"errors"
	"strconv"
)

type DiagnosticType int

func (d DiagnosticType) String() string {
	switch d {
	case DiagnosticTypeNone:
		return "None"
	case DiagnosticTypeOBD2:
		return "OBD2"
	case DiagnosticTypeEOBD:
		return "EOBD"
	case DiagnosticTypeLOBD:
		return "LOBD"
	default:
		return "Unknown"
	}
}

func DiagnosticTypeFromString(s string) DiagnosticType {
	switch s {
	case "None":
		return DiagnosticTypeNone
	case "OBD2":
		return DiagnosticTypeOBD2
	case "EOBD":
		return DiagnosticTypeEOBD
	case "LOBD":
		return DiagnosticTypeLOBD
	default:
		return DiagnosticTypeNone
	}
}

const (
	DiagnosticTypeNone DiagnosticType = iota
	DiagnosticTypeOBD2
	DiagnosticTypeEOBD
	DiagnosticTypeLOBD
)

type TankType int

func (t TankType) String() string {
	switch t {
	case TankTypeUS:
		return "US"
	case TankTypeEU:
		return "EU"
	case TankTypeAWD:
		return "AWD"
	default:
		return "Unknown"
	}
}

func TankTypeFromString(s string) TankType {
	switch s {
	case "US":
		return TankTypeUS
	case "EU":
		return TankTypeEU
	case "AWD":
		return TankTypeAWD
	default:
		return TankTypeUS
	}
}

const (
	TankTypeUS TankType = iota
	TankTypeEU
	TankTypeAWD
)

type PI01Data struct {
	Convertible    bool
	SAI            bool
	HighOutput     bool
	BioPower       bool
	DiagnosticType DiagnosticType
	ClutchStart    bool
	TankType       TankType
}

func (p PI01Data) Bytes() []byte {
	var data [2]byte
	if p.BioPower {
		data[0] |= 0x01
	}
	if p.Convertible {
		data[0] |= 0x04
	}
	switch p.TankType {
	case TankTypeUS:
		data[0] |= 0x08
	case TankTypeEU:
		data[0] |= 0x10
	case TankTypeAWD:
		data[0] |= 0x18
	}
	switch p.DiagnosticType {
	case DiagnosticTypeOBD2:
		data[0] |= 0x20
	case DiagnosticTypeEOBD:
		data[0] |= 0x40
	case DiagnosticTypeLOBD:
		data[0] |= 0x60
	}
	if p.ClutchStart {
		data[1] |= 0x04
	}
	if p.SAI {
		data[1] |= 0x10
	}
	if p.HighOutput {
		data[1] |= 0x20
	}
	return data[:]
}

func (p PI01Data) String() string {
	return "PI01Data{" +
		"Convertible:" + strconv.FormatBool(p.Convertible) +
		", SAI:" + strconv.FormatBool(p.SAI) +
		", HighOutput:" + strconv.FormatBool(p.HighOutput) +
		", BioPower:" + strconv.FormatBool(p.BioPower) +
		", DiagnosticType:" + p.DiagnosticType.String() +
		", ClutchStart:" + strconv.FormatBool(p.ClutchStart) +
		", TankType:" + p.TankType.String() +
		"}"
}

func DecodePI01(data []byte) (PI01Data, error) {
	var out PI01Data
	if len(data) < 2 {
		return out, errors.New("data too short")
	}

	if data[0] == 0x00 && data[1] == 0x00 {
		return out, errors.New("data invalid")
	}

	// -------C
	out.BioPower = getBit(data[0], 0)

	// -----C--
	out.Convertible = getBit(data[0], 2)

	// ---01--- US
	// ---10--- EU
	// ---11--- AWD
	switch data[0] & 0x18 {
	case 0x08:
		out.TankType = TankTypeUS
	case 0x10:
		out.TankType = TankTypeEU
	case 0x18:
		out.TankType = TankTypeAWD
	}

	// -01----- OBD2
	// -10----- EOBD
	// -11----- LOBD
	switch data[0] & 0x60 {
	case 0x00:
		out.DiagnosticType = DiagnosticTypeNone
	case 0x20:
		out.DiagnosticType = DiagnosticTypeOBD2
	case 0x40:
		out.DiagnosticType = DiagnosticTypeEOBD
	case 0x60:
		out.DiagnosticType = DiagnosticTypeLOBD
	}

	// on = -----10-
	// off= -----01-
	out.ClutchStart = !getBit(data[1], 1) && getBit(data[1], 2)

	// on = ---10---
	// off= ---01---
	out.SAI = !getBit(data[1], 3) && getBit(data[1], 4)

	// high= -01-----
	// low = -10-----
	out.HighOutput = getBit(data[1], 5) && !getBit(data[1], 6)
	return out, nil
}

func getBit(data byte, pos uint) bool {
	return (data&(1<<pos) != 0)
}
