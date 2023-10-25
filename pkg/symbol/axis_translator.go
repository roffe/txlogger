package symbol

import (
	"fmt"
	"log"
)

type AxisInformation map[string]Axis

type Axis struct {
	X            string
	Y            string
	Z            string
	XDescription string
	YDescription string
	ZDescription string
	XFrom        string
	YFrom        string
}

func (a *Axis) String() string {
	return fmt.Sprintf("X: %s, Y: %s, Z: %s, XFrom: %s, YFrom: %s", a.X, a.Y, a.Z, a.XFrom, a.YFrom)
}

type AxisCollection map[ECUType]Axis

var axisTranslator = map[ECUType]AxisInformation{
	ECU_T7: axisT7,
	ECU_T8: axisT8,
}

func GetAxisCollection(ecu ECUType) AxisInformation {
	return axisTranslator[ecu]
}

func getAxis(ecu ECUType, name string) Axis {
	return axisTranslator[ecu][name]
}

// returns x, y, z axis map name
func GetInfo(ecu ECUType, name string) Axis {
	axis := getAxis(ecu, name)

	if axis.XFrom == "" {
		axis.XFrom = "MAF.m_AirInlet"
	}
	if axis.YFrom == "" {
		axis.YFrom = "ActualIn.n_Engine"
	}

	if axis.X == "" && axis.Y == "" && axis.Z == "" {
		return Axis{
			"",
			"",
			name,
			"",
			"",
			"",
			"",
			"",
		}
	}
	log.Println(axis)
	return axis
}
