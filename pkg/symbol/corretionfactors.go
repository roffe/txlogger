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
		"IgnNormCal.Map",
		"IgnAbsCal.fi_NormalMAP",
		"IgnE85Cal.fi_AbsMap",
		"IgnMastProt.fi_Offset":
		return 0.1
	case "DisplProt.LambdaScanner",
		"BFuelCal.Map",
		"BFuelCal.StartMap",
		"Lambda.LambdaInt":
		return 0.01
	case "BFuelCal.LambdaOneFacMap", "BFuelCal.E85TempEnrichFacMap", "BFuelCal.TempEnrichFacMap":
		return 1.0 / 128
	case "ECMStat.p_Diff",
		"ECMStat.p_DiffThrot",
		"In.p_AirBefThrottle", "ActualIn.p_AirBefThrottle",
		"In.p_AirInlet", "ActualIn.p_AirInlet":
		return 0.001
	default:
		return 1
	}
}

/*
   if (symbolname == "KnkSoundRedCal.fi_OffsMa") returnvalue = 0.1;
   else if (symbolname == "IgnE85Cal.fi_AbsMap") returnvalue = 0.1;
   else if (symbolname == "MAFCal.cd_ThrottleMap") returnvalue = 0.0009765625;
   else if (symbolname == "TrqMastCal.Trq_NominalMap") returnvalue = 0.1;
   else if (symbolname == "TrqMastCal.Trq_MBTMAP") returnvalue = 0.1;
   else if (symbolname == "AfterStCal.StartMAP") returnvalue = 0.0009765625; // 1/1024
   else if (symbolname == "KnkFuelCal.EnrichmentMap") returnvalue = 0.0009765625; // 1/1024
   else if (symbolname == "AfterStCal.HotSoakMAP") returnvalue = 0.0009765625; // 1/1024
   else if (symbolname == "MAFCal.NormAdjustFacMap") returnvalue = 0.0078125; // 1/128
   else if (symbolname == "BFuelCal.LambdaOneFacMap") returnvalue = 0.0078125; // 1/128
   else if (symbolname == "BFuelCal.TempEnrichFacMap") returnvalue = 0.0078125; // 1/128
   else if (symbolname == "BFuelCal.E85TempEnrichFacMap") returnvalue = 0.0078125; // 1/128
   else if (symbolname == "AfterStCal.AmbientMAP") returnvalue = 0.0078125; // 1/128
   else if (symbolname == "FFFuelCal.KnkEnrichmentMAP") returnvalue = 0.0078125; // 1/128
   else if (symbolname == "FFFuelCal.TempEnrichFacMAP") returnvalue = 0.0078125; // 1/128
*/
