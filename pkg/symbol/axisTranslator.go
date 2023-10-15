package symbol

type ECUType int

const (
	ECU_T7 ECUType = iota
	ECU_T8
)

type AxisInformation map[string]Axis

type Axis struct {
	X            string
	Y            string
	Z            string
	XDescription string
	YDescription string
	ZDescription string
}

func (a *Axis) String() string {
	return a.X + ", " + a.Y + ", " + a.Z
}

/*
var t7airRpm = Axis{
	"BFuelCal.AirXSP",
	"BFuelCal.RpmYSP",
	"",
	"",
	"",
	"",
}

var axisT7 = AxisInformation{
	"BFuelCal.Map":      t7airRpm,
	"BFuelCal.StartMap": t7airRpm,
	"IgnNormCal.Map":    t7airRpm,
}
*/

type AxisCollection map[ECUType]Axis

var axisTranslator = map[ECUType]AxisInformation{
	ECU_T7: axisT7,
	ECU_T8: {
		"BFuelCal.TempEnrichFacMap": Axis{
			"IgnAbsCal.m_AirNormXSP",
			"IgnAbsCal.n_EngNormYSP",
			"BFuelCal.TempEnrichFacMap",
			"",
			"",
			"",
		},
		"IgnAbsCal.fi_NormalMAP": Axis{
			"IgnAbsCal.m_AirNormXSP",
			"IgnAbsCal.n_EngNormYSP",
			"BFuelCal.TempEnrichFacMap",
			"",
			"",
			"",
		},
	},
}

func GetAxisCollection(ecu ECUType) AxisInformation {
	return axisTranslator[ecu]
}

func GetAxis(ecu ECUType, name string) Axis {
	return axisTranslator[ecu][name]
}

// returns x, y, z axis map name
func GetInfo(ecu ECUType, name string) (string, string, string) {
	axis := GetAxis(ecu, name)
	return axis.X, axis.Y, axis.Z
}
