package symbol

var axisT8 = AxisInformation{
	"AirCtrlCal.RegMap": Axis{
		"AirCtrlCal.SetLoadXSP",
		"BstKnkCal.n_EngYSP",
		"AirCtrlCal.RegMap",
		"",
		"",
		"",
		"MAF.m_AirInlet",
		"ActualIn.n_Engine",
	},
	"AirCtrlCal.Ppart_BoostMap": Axis{
		"AirCtrlCal.PIDXSP",
		"AirCtrlCal.PIDYSP",
		"AirCtrlCal.Ppart_BoostMap",
		"mg/c error",
		"rpm",
		"",
		"AirDIFF",
		"ActualIn.n_Engine",
	},
	"AirCtrlCal.Ipart_BoostMap": Axis{
		"AirCtrlCal.PIDXSP",
		"AirCtrlCal.PIDYSP",
		"AirCtrlCal.Ipart_BoostMap",
		"mg/c error",
		"rpm",
		"",
		"AirDIFF",
		"ActualIn.n_Engine",
	},
	"AirCtrlCal.Dpart_BoostMap": Axis{
		"AirCtrlCal.PIDXSP",
		"AirCtrlCal.PIDYSP",
		"AirCtrlCal.Dpart_BoostMap",
		"mg/c error",
		"rpm",
		"",
		"AirDIFF",
		"ActualIn.n_Engine",
	},
	"BstKnkCal.MaxAirmass": Axis{
		"BstKnkCal.fi_offsetXSP", // BioPower "BstKnkCal.fi_offsetXSP"
		"BstKnkCal.n_EngYSP",
		"BstKnkCal.MaxAirmass",
		"° ignition retard (Ioff)",
		"rpm",
		"mg/c",
		"IgnMastProt.fi_Offset",
		"ActualIn.n_Engine",
	},
	"IgnAbsCal.fi_NormalMAP": Axis{
		"IgnAbsCal.m_AirNormXSP",
		"IgnAbsCal.n_EngNormYSP",
		"IgnAbsCal.fi_NormalMAP",
		"",
		"",
		"",
		"MAF.m_AirInlet",
		"ActualIn.n_Engine",
	},
	"IgnAbsCal.fi_highOctanMAP": Axis{
		"IgnAbsCal.m_AirNormXSP",
		"IgnAbsCal.n_EngNormYSP",
		"IgnAbsCal.fi_highOctanMAP",
		"",
		"",
		"",
		"MAF.m_AirInlet",
		"ActualIn.n_Engine",
	},
	"BFuelCal.LambdaOneFacMap": Axis{
		"BFuelCal.AirXSP",
		"BFuelCal.RpmYSP",
		"BFuelCal.LambdaOneFacMap",
		"mg/c",
		"rpm",
		"fac",
		"MAF.m_AirInlet",
		"ActualIn.n_Engine",
	},
	"BFuelCal.TempEnrichFacMap": Axis{
		"BFuelCal.AirXSP",
		"BFuelCal.RpmYSP",
		"BFuelCal.TempEnrichFacMap",
		"mg/c",
		"rpm",
		"fac",
		"MAF.m_AirInlet",
		"ActualIn.n_Engine",
	},
	"FFFuelCal.TempEnrichFacMAP": Axis{
		"BFuelCal.AirXSP",
		"BFuelCal.RpmYSP",
		"FFFuelCal.TempEnrichFacMAP",
		"mg/c",
		"rpm",
		"fac",
		"MAF.m_AirInlet",
		"ActualIn.n_Engine",
	},
	"AirCtrlCal.AirmassLimiter": Axis{
		"",
		"",
		"AirCtrlCal.AirmassLimiter",
		"",
		"",
		"mg/c",
		"",
		"",
	},
	"AirCtrlCal.ST_BoostEnable": Axis{
		"",
		"",
		"AirCtrlCal.ST_BoostEnable",
		"",
		"",
		"",
		"",
		"",
	},
	"BoostAdapCal.ST_enable": Axis{
		"",
		"",
		"BoostAdapCal.ST_enable",
		"",
		"",
		"",
		"",
		"",
	},
	"FrompAdapCal.ST_enable": Axis{
		"",
		"",
		"FrompAdapCal.ST_enable",
		"",
		"",
		"",
		"",
		"",
	},
	"AreaAdapCal.ST_enable": Axis{
		"",
		"",
		"AreaAdapCal.ST_enable",
		"",
		"",
		"",
		"",
		"",
	},
	"InjCorrCal.InjectorConst": Axis{
		"",
		"",
		"InjCorrCal.InjectorConst",
		"",
		"",
		"",
		"",
		"",
	},
	"InjCorrCal.BattCorrTab": Axis{
		"",
		"InjCorrCal.BattCorrSP",
		"InjCorrCal.BattCorrTab",
		"",
		"V",
		"",
		"",
		"",
	},
	"IgnAbsCal.ST_EnableOctanMaps": Axis{
		"",
		"",
		"IgnAbsCal.ST_EnableOctanMaps",
		"",
		"",
		"",
		"",
		"",
	},
}
