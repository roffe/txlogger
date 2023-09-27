package symbol

func GetCorrectionfactor(name string) float64 {
	//log.Println(name)
	switch name {
	case "IgnProt.fi_Offset",
		"Out.X_AccPedal", "Out.X_AccPos",
		"Out.fi_Ignition",
		"Out.PWM_BoostCntrl",
		"In.v_Vehicle",
		"In.p_AirAmbient",
		"IgnMastProt.fi_Offset":
		return 0.1
	case "DisplProt.LambdaScanner",
		"Lambda.LambdaInt":
		return 0.01
	case "ECMStat.p_Diff",
		"ECMStat.p_DiffThrot",
		"In.p_AirBefThrottle", "ActualIn.p_AirBefThrottle",
		"In.p_AirInlet", "ActualIn.p_AirInlet":
		return 0.001
	default:
		return 1
	}
}
