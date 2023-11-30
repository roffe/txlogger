package symbol

func GetUnit(name string) string {
	switch name {
	case "Out.X_AccPedal", "Out.PWM_BoostCntrl":
		return "%"
	case "In.p_AirAmbient":
		return "kPa"
	case "In.v_Vehicle":
		return "Km/h"
	case "IgnProt.fi_Offset":
		return "Degrees"
	case "Out.fi_Ignition":
		return "° BTDC"
	case "ECMStat.p_Diff", "ECMStat.p_DiffThrot", "In.p_AirBefThrottle":
		return "Bar"
	case "m_Request", "MAF.m_AirInlet":
		return "Mg/c"
	default:
		return ""
	}
}
