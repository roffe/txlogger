package symbol

var correctionFactors = map[string]float64{

	"KnkSoundRedCal.fi_OffsMa":     0.1,
	"IgnE85Cal.fi_AbsMap":          0.1,
	"MAFCal.cd_ThrottleMap":        0.0009765625,
	"TrqMastCal.Trq_NominalMap":    0.1,
	"TrqMastCal.Trq_MBTMAP":        0.1,
	"AfterStCal.StartMAP":          0.0009765625, // 1/1024
	"KnkFuelCal.EnrichmentMap":     0.0009765625, // 1/1024
	"AfterStCal.HotSoakMAP":        0.0009765625, // 1/1024
	"MAFCal.NormAdjustFacMap":      0.0078125,    // 1/128
	"BFuelCal.LambdaOneFacMap":     0.0078125,    // 1/128
	"BFuelCal.TempEnrichFacMap":    0.0078125,    // 1/128
	"BFuelCal.E85TempEnrichFacMap": 0.0078125,    // 1/128
	"AfterStCal.AmbientMAP":        0.0078125,    // 1/128
	"FFFuelCal.KnkEnrichmentMAP":   0.0078125,    // 1/128
	"FFFuelCal.TempEnrichFacMAP":   0.0078125,    // 1/128

	"ActualIn.p_AirBefThrottle": 0.001,
	"ActualIn.p_AirInlet":       0.001,
	"AirCompCal.PressMap":       0.1,
	"BFuelCal.Map":              0.01,
	"BFuelCal.StartMap":         0.01,
	"BoostCal.RegMap":           0.1,
	"DisplProt.LambdaScanner":   0.01,
	"ECMStat.p_Diff":            0.001,
	"ECMStat.p_DiffThrot":       0.001,
	"IgnAbsCal.fi_NormalMAP":    0.1,
	"IgnIdleCal.fi_IdleMap":     0.1,
	"IgnMastProt.fi_Offset":     0.1,
	"IgnNormCal.Map":            0.1,
	"IgnProt.fi_Offset":         0.1,
	"IgnStartCal.fi_StartMap":   0.1,
	"IgnStartCal.X_EthActSP":    0.1,
	"In.p_AirAmbient":           0.1,
	"In.p_AirBefThrottle":       0.001,
	"In.p_AirInlet":             0.001,
	"In.v_Vehicle":              0.1,
	"Lambda.LambdaInt":          0.01,
	"MyrtilosCal.Launch_RPM":    100,
	"Out.fi_Ignition":           0.1,
	"Out.PWM_BoostCntrl":        0.1,
	"Out.X_AccPedal":            0.1,
	"Out.X_AccPos":              0.1,
	"BstKnkCal.OffsetXSP":       0.1,
	"InjCorrCal.BattCorrSP":     0.1,
	"MAFCal.LoadYSP":            0.001,
}

func GetCorrectionfactor(name string) float64 {
	if val, exists := correctionFactors[name]; exists {
		return val
	}
	return 1
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
