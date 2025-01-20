package mainmenu

import (
	"strings"

	"fyne.io/fyne/v2"
	symbol "github.com/roffe/ecusymbol"
)

var T5SymbolsTuningOrder = []string{
	"Options",
	// "Injectors",
	"Fuel",
	// "Ignition",
	// "Boost",
	// "Idle",
	"Diagnostics",
}

var T5SymbolsTuning = map[string][]string{
	"Options": {
		"Pgm_mod!",
	},
	"Injectors": {
		"Inj_konst!",
		"Batt_korr_tab!",
	},
	"Fuel": {
		"Insp_mat!",
	},
	"Ignition": {
		"Ign_map_0!",
	},
	"Boost": {
		"Tryck_mat!",
		"Reg_kon_mat!",
		"P_fors!",
		"I_fors!",
		"D_fors!",
		"Regl_tryck_fgm!",
		"Regl_tryck_sgm!",
	},
	"Idle": {
		"Idle_rpm_tab!",
		"Ign_idle_angle!",
		"Idle_fuel_korr!",
	},
	"Diagnostics": {
		"Pgm_status",
	},
}

var T7SymbolsTuningOrder = []string{
	"Diagnostics",
	"Calibration",
	"Injectors",
	"Limiters",
	"Fuel",
	"Boost",
	"Ignition",
	"Adaption",
	"Myrtilos",
}

var T7SymbolsTuning = map[string][]string{
	"Diagnostics": {
		"KnkDetAdap.KnkCntMap",
		"F_KnkDetAdap.FKnkCntMap",
		"F_KnkDetAdap.RKnkCntMap",
		"MissfAdap.MissfCntMap",
	},
	"Calibration": {
		"AirCompCal.PressMap",
		"E85.X_EthAct_Tech2",
		"MAFCal.m_RedundantAirMap",
		"PedalMapCal.m_RequestMap",
		"TCompCal.EnrFacE85Tab",
		"TCompCal.EnrFacTab",
		"VIOSMAFCal.FreqSP",
		"VIOSMAFCal.Q_AirInletTab2",
	},
	"Injectors": {
		"InjCorrCal.BattCorrTab",
		"InjCorrCal.BattCorrSP",
		"InjCorrCal.InjectorConst",
	},
	"Limiters": {
		"BstKnkCal.MaxAirmass",
		"BstKnkCal.MaxAirmassAu",
		"TorqueCal.M_ManGearLim",
	},
	"Fuel": {
		"BFuelCal.Map",
		"BFuelCal.StartMap / E85 Map|BFuelCal.StartMap",
		"KnkFuelCal.EnrichmentMap",
		"StartCal.EnrFacE85Tab",
		"StartCal.EnrFacTab",
	},
	"Boost": {
		//"...|BoostCal.RegMap|BoostCal.PMap|BoostCal.IMap|BoostCal.DMap",
		"BoostCal.RegMap",
		"BoostCal.PMap",
		"BoostCal.IMap",
		"BoostCal.DMap",
	},
	"Ignition": {
		"IgnE85Cal.fi_AbsMap",
		"IgnIdleCal.fi_IdleMap",
		"IgnNormCal.Map",
		"IgnStartCal.fi_StartMap",
	},
	"Adaption": {
		"AdpFuelCal.T_AdaptLim",
		"FCutCal.ST_Enable",
		"LambdaCal.ST_Enable",
		"PurgeCal.ST_PurgeEnable",
		"E85Cal.ST_Enable",
	},
	"Myrtilos": {
		"Register EU0D",
		"MyrtilosCal.Launch_DisableSpeed",
		"MyrtilosCal.Launch_Ign_fi_Min",
		"MyrtilosCal.Launch_RPM",
		"MyrtilosCal.Launch_InjFac_at_rpm",
		"MyrtilosCal.Launch_PWM_max_at_stand",
		"MyrtilosAdap.WBLambda_FeedbackMap",
		"MyrtilosAdap.WBLambda_FFMap",
	},
}

var T8SymbolsTuningOrder = []string{
	"Injectors",
	"Limiters",
	"Fuel",
	"Boost",
	"Ignition",
	"Torque",
}

var T8SymbolsTuning = map[string][]string{
	"Injectors": {
		"InjCorrCal.InjectorConst",
		"InjCorrCal.BattCorrTab",
		"InjCorrCal.BattCorrSP",
	},
	"Limiters": {
		"BstKnkCal.MaxAirmass",
		"AirCtrlCal.AirmassLimiter",
	},
	"Fuel": {
		"BFuelCal.LambdaOneFacMap",
		"BFuelCal.TempEnrichFacMap",
		"FFFuelCal.TempEnrichFacMAP",
		"PurgeCal.ST_PurgeEnable",
		"LambdaCal.ST_Enable",
		"FCutCal.ST_Enable",
		"FFFuelCal.ST_enable",
		"FuelDynCal.ST_Enable",
		"TCompCal.ST_Enable",
	},
	"Boost": {
		//"...|AirCtrlCal.RegMap|AirCtrlCal.Ppart_BoostMap|AirCtrlCal.Ipart_BoostMap|AirCtrlCal.Dpart_BoostMap",
		"AirCtrlCal.RegMap",
		"AirCtrlCal.Ppart_BoostMap",
		"AirCtrlCal.Ipart_BoostMap",
		"AirCtrlCal.Dpart_BoostMap",
		"AirCtrlCal.ST_BoostEnable",
		"BoostAdapCal.ST_enable",
		"FrompAdapCal.ST_enable",
		"AreaAdapCal.ST_enable",
	},
	"Ignition": {
		"IgnAbsCal.fi_NormalMAP",
		"IgnAbsCal.fi_lowOctanMAP",
		"IgnAbsCal.fi_highOctanMAP",
		"IgnAbsCal.ST_EnableOctanMaps",
	},
	"Torque": {
		"TrqMastCal.X_AccPedalMAP",
		"TrqLimCal.Trq_ManGear",
		"TrqLimCal.Trq_MaxEngineTab1",
		"TrqLimCal.Trq_MaxEngineTab2",
		"FFTrqCal.FFTrq_MaxEngineTab1",
		"FFTrqCal.FFTrq_MaxEngineTab2",
	},
}

type MainMenu struct {
	w                 fyne.Window
	leading, trailing []*fyne.Menu
	oneFunc           func(symbol.ECUType, string)
	multiFunc         func(symbol.ECUType, ...string)
	otherFunc         func(string)
}

func New(w fyne.Window, leading, trailing []*fyne.Menu, oneFunc func(symbol.ECUType, string), otherFunc func(string)) *MainMenu {
	return &MainMenu{
		w:         w,
		oneFunc:   oneFunc,
		leading:   leading,
		trailing:  trailing,
		otherFunc: otherFunc,
	}
}

func (mw *MainMenu) GetMenu(name string) *fyne.MainMenu {
	var order []string
	var ecuM map[string][]string
	var typ symbol.ECUType

	switch name {
	case "T5":
		order = T5SymbolsTuningOrder
		ecuM = T5SymbolsTuning
		typ = symbol.ECU_T5
	case "T7":
		order = T7SymbolsTuningOrder
		ecuM = T7SymbolsTuning
		typ = symbol.ECU_T7
	case "T8":
		order = T8SymbolsTuningOrder
		ecuM = T8SymbolsTuning
		typ = symbol.ECU_T8
	}

	menus := append([]*fyne.Menu{}, mw.leading...)

	for _, category := range order {
		var items []*fyne.MenuItem
		for _, mapName := range ecuM[category] {
			if mapName == "Register EU0D" {
				itm := fyne.NewMenuItem(mapName, func() {
					mw.otherFunc(mapName)
				})
				items = append(items, itm)
				continue
			}

			if strings.Contains(mapName, "|") {
				parts := strings.Split(mapName, "|")
				names := parts[1:]
				if len(parts) == 2 {
					itm := fyne.NewMenuItem(parts[0], func() {
						mw.oneFunc(typ, names[0])
					})
					items = append(items, itm)
					continue
				}
				itm := fyne.NewMenuItem(parts[0], func() {
					mw.multiFunc(typ, names...)
				})
				items = append(items, itm)
				continue
			}

			itm := fyne.NewMenuItem(mapName, func() {
				mw.oneFunc(typ, mapName)
			})
			items = append(items, itm)
		}
		menus = append(menus, fyne.NewMenu(category, items...))
	}

	menus = append(menus, mw.trailing...)

	return fyne.NewMainMenu(menus...)
}
