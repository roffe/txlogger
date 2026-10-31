package estimatedoutput

import (
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

// Synthetic T7/T8 binaries for the estimator tests. Every table the suites
// read is present, so the original C# (which clamps to zero on a missing
// symbol) and the port see the same inputs; the expected values in the
// oracle tests were produced by running the T7Suite/T8Suite
// ctrlAirmassResult.cs methods verbatim over these exact bytes.
//
// Shapes: 4 rpm rows {1000, 3000, 5200, 6000} x 3 pedal columns
// {0, 50, 100 %}; 5200 rpm falls between breakpoints and 5200/6000 rpm hit
// the high-rpm torque roll-off.

// ssym is a signed 16-bit symbol, decoded like ecusymbol's SIGNED tables.
func ssym(t *testing.T, name string, vals ...int) *symbol.Symbol {
	t.Helper()
	s := sym(t, name, vals...)
	s.Type = symbol.SIGNED
	return s
}

// bsym is an 8-bit symbol, decoded like ecusymbol's CHAR tables.
func bsym(t *testing.T, name string, vals ...int) *symbol.Symbol {
	t.Helper()
	data := make([]byte, len(vals))
	for i, v := range vals {
		data[i] = byte(v)
	}
	s := &symbol.Symbol{Name: name, Length: uint16(len(data)), Type: symbol.CHAR}
	if err := s.SetData(data); err != nil {
		t.Fatal(err)
	}
	return s
}

// syms is a mutable symbol set so a test can replace or drop single tables
// before building the firmware.
type syms map[string]*symbol.Symbol

func newSyms(list ...*symbol.Symbol) syms {
	m := syms{}
	for _, s := range list {
		m[s.Name] = s
	}
	return m
}

func (m syms) set(list ...*symbol.Symbol) syms {
	for _, s := range list {
		m[s.Name] = s
	}
	return m
}

func (m syms) del(names ...string) syms {
	for _, n := range names {
		delete(m, n)
	}
	return m
}

func (m syms) fw() symbol.FirmwareFile {
	c := symbol.NewCollection()
	for _, s := range m {
		c.Add(s)
	}
	return c
}

func t7Syms(t *testing.T) syms {
	t.Helper()
	return newSyms(
		sym(t, "PedalMapCal.n_EngineMap", 1000, 3000, 5200, 6000),
		sym(t, "PedalMapCal.X_PedalMap", 0, 500, 1000),
		// pedal-major: 0 %, 50 %, 100 % (WOT requests 1400 mg/c)
		sym(t, "PedalMapCal.m_RequestMap",
			80, 80, 80, 80,
			450, 600, 650, 600,
			1400, 1400, 1400, 1400),
		sym(t, "TorqueCal.n_EngYSP", 1000, 3000, 5000, 6000),
		ssym(t, "TorqueCal.M_EngXSP", -20, 100, 300, 500),
		sym(t, "TorqueCal.m_AirTorqMap",
			40, 340, 960, 1600,
			45, 330, 940, 1560,
			50, 335, 950, 1580,
			55, 350, 990, 1650),
		// raw unsigned with -100 stored as 65436, sign-fixed by signed16
		sym(t, "BstKnkCal.OffsetXSP", 65436, 0),
		sym(t, "BstKnkCal.n_EngYSP", 1000, 3000, 5000, 6000),
		sym(t, "BstKnkCal.MaxAirmass", 800, 900, 1000, 1180, 1050, 1120, 900, 1000),
		sym(t, "BstKnkCal.MaxAirmassAu", 700, 850, 900, 1000, 950, 1050, 850, 950),
		sym(t, "TorqueCal.m_AirXSP", 100, 600, 1200),
		sym(t, "TorqueCal.M_NominalMap",
			65526, 180, 390,
			5, 190, 395,
			0, 185, 380,
			65531, 170, 350),
		sym(t, "FCutCal.m_AirInletLimit", 1250),
		sym(t, "TorqueCal.M_EngMaxTab", 300, 380, 360, 330),
		sym(t, "TorqueCal.M_EngMaxAutTab", 280, 330, 330, 300),
		sym(t, "TorqueCal.M_EngMaxE85Tab", 320, 400, 390, 350),
		sym(t, "TorqueCal.M_EngMaxE85TabAut", 290, 340, 345, 310),
		sym(t, "TorqueCal.M_ManGearLim", 500, 330, 400, 450, 500, 500),
		sym(t, "TorqueCal.M_CabGearLim", 500, 300, 310, 400, 420, 420),
		sym(t, "TorqueCal.M_5GearLimTab", 350, 370, 390, 390),
		sym(t, "TorqueCal.n_Eng5GearSP", 1000, 3000, 5000, 6000),
		sym(t, "TorqueCal.M_1GearTab", 200, 260, 280, 280),
		sym(t, "TorqueCal.n_Eng1GearSP", 1000, 3000, 5000, 6000),
		sym(t, "TorqueCal.M_OverBoostTab", 420, 350, 450, 300),
		bsym(t, "TorqueCal.EnableOverBoost", 1),
		bsym(t, "TorqueCal.ST_Loop", 2),
		sym(t, "LimEngCal.TurboSpeedTab", 900, 1400),
		sym(t, "LimEngCal.p_AirSP", 650, 1000),
		sym(t, "LimEngCal.TurboSpeedTab2", 1000, 800),
		sym(t, "LimEngCal.n_EngSP", 1000, 6000),
		sym(t, "InjCorrCal.InjectorConst", 220),
		sym(t, "InjCorrCal.BattCorrTab", 900, 700),
		sym(t, "InjCorrCal.BattCorrSP", 100, 140),
		sym(t, "BFuelCal.AirXSP", 100, 600, 1200),
		sym(t, "BFuelCal.RpmYSP", 1000, 3000, 5000, 6000),
		bsym(t, "BFuelCal.Map", 100, 102, 110, 100, 105, 115, 100, 108, 120, 100, 110, 125),
		bsym(t, "BFuelCal.StartMap", 100, 130, 140, 100, 135, 145, 100, 138, 150, 100, 140, 155),
	)
}

func t8Syms(t *testing.T) syms {
	t.Helper()
	return newSyms(
		sym(t, "PedalMapCal.n_EngineMap", 1000, 3000, 5200, 6000),
		sym(t, "PedalMapCal.X_PedalMap", 0, 500, 1000),
		// pedal-major torque request in 0.1 Nm; WOT asks for 450 Nm
		ssym(t, "PedalMapCal.Trq_RequestMap",
			-300, -200, -150, -100,
			1500, 2000, 2200, 2000,
			4500, 4500, 4500, 4500),
		sym(t, "TrqMastCal.n_EngineYSP", 1000, 3000, 5000, 6000),
		ssym(t, "TrqMastCal.Trq_EngXSP", -200, 1000, 3000, 5000),
		sym(t, "TrqMastCal.m_AirTorqMap",
			60, 340, 960, 1600,
			65, 330, 940, 1560,
			70, 335, 950, 1580,
			75, 350, 990, 1650),
		ssym(t, "BstKnkCal.OffsetXSP", -100, 0),
		sym(t, "BstKnkCal.n_EngYSP", 1000, 3000, 5000, 6000),
		ssym(t, "BstKnkCal.MaxAirmass", 800, 900, 1000, 1180, 1050, 1120, 900, 1000),
		ssym(t, "BstKnkCal.MaxAirmassAu", 700, 850, 900, 1000, 950, 1050, 850, 950),
		sym(t, "TrqMastCal.m_AirXSP", 100, 600, 1200),
		ssym(t, "TrqMastCal.Trq_NominalMap",
			-105, 1805, 3907,
			55, 1900, 3950,
			0, 1850, 3800,
			-55, 1700, 3500),
		sym(t, "FCutCal.m_AirInletLimit", 1250),
		sym(t, "TrqLimCal.Trq_MaxEngineManTab1", 3000, 3800, 3600, 3300),
		sym(t, "TrqLimCal.Trq_MaxEngineManTab2", 2800, 3400, 3300, 3000),
		sym(t, "TrqLimCal.Trq_MaxEngineAutTab1", 2900, 3500, 3400, 3200),
		sym(t, "TrqLimCal.Trq_MaxEngineAutTab2", 2700, 3200, 3100, 2900),
		sym(t, "TrqLimCal.Trq_ManGear", 4000, 3300, 4000, 4500, 5000, 5000, 5000, 2500),
		sym(t, "FFTrqCal.FFTrq_MaxEngineTab1", 3200, 4000, 3900, 3500),
		sym(t, "FFTrqCal.FFTrq_MaxEngineTab2", 3000, 3700, 3600, 3200),
		ssym(t, "FFAirCal.fi_offsetXSP", -100, 0),
		ssym(t, "FFAirCal.m_maxAirmass", 850, 950, 1050, 1200, 1100, 1150, 950, 1050),
		bsym(t, "FFFuelCal.TempEnrichFacMAP", 128, 150, 160, 128, 155, 165, 128, 158, 170, 128, 160, 175),
		bsym(t, "BFuelCal.TempEnrichFacMap", 128, 130, 140, 128, 135, 145, 128, 138, 150, 128, 140, 155),
		sym(t, "TrqLimCal.Trq_OverBoostTab", 4200, 3500, 4500, 3000),
		bsym(t, "TrqLimCal.EnableOverBoost", 1),
		sym(t, "InjCorrCal.InjectorConst", 220),
		sym(t, "InjCorrCal.BattCorrTab", 900, 700),
		sym(t, "InjCorrCal.BattCorrSP", 100, 140),
		sym(t, "BFuelCal.AirXSP", 100, 600, 1200),
		sym(t, "BFuelCal.RpmYSP", 1000, 3000, 5000, 6000),
		sym(t, "TMCCal.Trq_MaxEngineTab", 2800, 3300, 3300, 3000),
		sym(t, "TMCCal.Trq_MaxEngineLowTab", 2600, 3000, 3000, 2800),
		ssym(t, "ExhaustCal.T_Lambda1Map", 500, 650, 800, 520, 700, 850, 540, 750, 900, 560, 780, 950),
	)
}

// t5Syms is a 4 rpm x 3 throttle T5 binary with byte-wide temperature
// tables, as T5Suite reads Luft_kompfak!/Lufttemp_steg!.
func t5Syms(t *testing.T) syms {
	t.Helper()
	return newSyms(
		// boost request, absolute kPa; last column is WOT
		bsym(t, "Tryck_mat!", 100, 120, 150, 100, 140, 180, 100, 150, 200, 100, 140, 170),
		bsym(t, "Tryck_mat_a!", 100, 110, 130, 100, 120, 150, 100, 130, 160, 100, 120, 140),
		sym(t, "Pwm_ind_rpm!", 100, 250, 400, 550), // rpm / 10
		bsym(t, "Pwm_ind_trot!", 10, 60, 155),
		bsym(t, "Inj_konst!", 21),
		sym(t, "Min_tid!", 250),
		sym(t, "Batt_korr_tab!", 148, 193, 196, 234, 321, 376, 463, 580, 932, 932, 932),
		bsym(t, "Luft_kompfak!", 86, 93, 99, 106, 113, 120, 128, 136, 144, 154, 163, 173, 183, 195, 206, 219),
		bsym(t, "Lufttemp_steg!", 11, 15, 19, 26, 34, 45, 58, 74, 96, 124, 150, 174, 201, 220, 234, 255),
		bsym(t, "Insp_mat!",
			40, 60, 80, 100,
			45, 65, 90, 110,
			50, 70, 95, 120,
			55, 75, 100, 127),
		bsym(t, "Fuel_map_xaxis!", 40, 100, 160, 230),
		sym(t, "Fuel_map_yaxis!", 100, 250, 400, 550), // rpm / 10
	)
}
