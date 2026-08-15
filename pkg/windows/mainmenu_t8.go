package windows

import symbol "github.com/roffe/ecusymbol"

func (mw *MainWindow) t8Menu() []MenuItem {
	return []MenuItem{
		{Name: "Diagnostics", Children: []MenuItem{
			{Name: "DTC Reader", Func: mw.openDTCReader},
			{Name: "Edit Parameters", Func: mw.openEditParameters},
			{Name: "Firmware info edit", Func: mw.openFirmwareInfoEdit},
			{Name: "NVDM editor", Func: mw.openNVDMEditor},
		}},
		{Name: "Airmass", Children: []MenuItem{
			{Name: "Max airmass map (manual)", Data: "BstKnkCal.MaxAirmass"},
			{Name: "Max airmass map (auto)", Data: "BstKnkCal.MaxAirmassAu"},
			{Name: "Airmass Fuelcut", Data: "FCutCal.m_AirInletLimit"},
			{Name: "AirCtrlCal.AirmassLimiter"},
			{Name: "AirCtrlCal.PRatioMaxTab"},
		}},
		{Name: "Torque", Children: []MenuItem{
			{Name: "Nominal torque map", Data: "TrqMastCal.Trq_NominalMap"},
			{Name: "Airmass torque map", Data: "TrqMastCal.m_AirTorqMap"},
			{Name: "Ambient pressure trq limiter", Data: "TrqLimCal.Trq_CompressorNoiseRedLimMAP"},
			{Name: "Trq limit in overboost", Data: "TrqLimCal.Trq_OverBoostTab"},
			{Name: "Trq limit manual 150hp", Data: "TrqLimCal.Trq_MaxEngineManTab2"},
			{Name: "Trq limit manual 175+hp", Data: "TrqLimCal.Trq_MaxEngineManTab1"},
			{Name: "Trq limit auto 150hp", Data: "TrqLimCal.Trq_MaxEngineAutTab2"},
			{Name: "Trq limit auto 175+hp", Data: "TrqLimCal.Trq_MaxEngineAutTab1"},
			{Name: "Manual gear trq limit", Data: "TrqLimCal.Trq_ManGear"},
			{Name: "RPM limiter", Data: "MaxEngSpdCal.n_EngLimTab"},
			{Name: "TrqLimCal.Trq_MaxEngineTab1"},
			{Name: "TrqLimCal.Trq_MaxEngineTab2"},
			{Name: "FFTrqCal.FFTrq_MaxEngineTab1"},
			{Name: "FFTrqCal.FFTrq_MaxEngineTab2"},
		}},
		{Name: "Injectors", Children: []MenuItem{
			{Name: "Inj. Constant", Data: "InjCorrCal.InjectorConst"},
			{Name: "Inj. dead time", Data: "InjCorrCal.BattCorrTab"},
			{Name: "Inj. dead time (Y)", Data: "InjCorrCal.BattCorrSP"},
		}},
		{Name: "Fuel", Children: []MenuItem{
			{Name: "Fuel correction map", Data: "BFuelCal.LambdaOneFacMap"},
			{Name: "Enrichment Petrol", Data: "BFuelCal.TempEnrichFacMap"},
			{Name: "Enrichment E85", Data: "FFFuelCal.TempEnrichFacMAP"},
			{Name: "Knock fuel map", Data: "KnkFuelCal.EnrichmentMap"},
			{Name: "Injection end angle map", Data: "InjAnglCal.Map"},
			{Name: "Jerk enrichment petrol", Data: "BFuelCal.m_AirJerkTab"},
			{Name: "Jerk enrichment Fuelmaster", Data: "BFuelCal.JerkEnrichFacTab"},
			{Name: "PurgeCal.ST_PurgeEnable"},
			{Name: "LambdaCal.ST_Enable"},
			{Name: "FCutCal.ST_Enable"},
			{Name: "FFFuelCal.ST_enable"},
			{Name: "FuelDynCal.ST_Enable"},
			{Name: "TCompCal.ST_Enable"},
		}},
		{Name: "Boost", Children: []MenuItem{
			{Name: "Boost regulation map", Data: "AirCtrlCal.RegMap"},
			{Name: "P factor", Data: "AirCtrlCal.Ppart_BoostMap"},
			{Name: "I factor", Data: "AirCtrlCal.Ipart_BoostMap"},
			{Name: "D factor", Data: "AirCtrlCal.Dpart_BoostMap"},
			{Name: "AirCtrlCal.ST_BoostEnable"},
			{Name: "BoostAdapCal.ST_enable"},
			{Name: "FrompAdapCal.ST_enable"},
			{Name: "AreaAdapCal.ST_enable"},
		}},
		{Name: "Ignition", Children: []MenuItem{
			{Name: "Normal ignition map", Data: "IgnAbsCal.fi_NormalMAP"},
			{Name: "High octane map", Data: "IgnAbsCal.fi_highOctanMAP"},
			{Name: "Low octane map", Data: "IgnAbsCal.fi_lowOctanMAP"},
			{Name: "MBT ignition map", Data: "IgnAbsCal.fi_IgnMBTMAP"},
			{Name: "Fuel cut ignition map", Data: "IgnAbsCal.fi_FuelCutMAP"},
			{Name: "Startup map", Data: "IgnAbsCal.fi_StartMAP"},
			{Name: "IgnAbsCal.ST_EnableOctanMaps"},
		}},
		{Name: "Pedal", Children: []MenuItem{
			{Name: "Pedal position map", Data: "TrqMastCal.X_AccPedalMAP"},
			{Name: "Torque request map", Data: "PedalMapCal.Trq_RequestMap"},
			{Name: "Rescale AccPedalMap", Func: func() {
				mw.openRescaler(symbol.ECU_T8, "TrqMastCal.X_AccPedalMAP")
			}},
		}},
	}
}
