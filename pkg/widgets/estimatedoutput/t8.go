package estimatedoutput

import (
	"fmt"

	symbol "github.com/roffe/ecusymbol"
)

// T8 estimator, ported from T8Suite ctrlAirmassResult.cs. Unlike T7 the
// pedal request map holds torque in 0.1 Nm which is converted to airmass
// through the air/torque calibration first; all torque limits are 0.1 Nm.

type t8 struct{}

const (
	t8TorqueLimitAuto   = 3500 // 0.1 Nm
	t8TorqueLimitManual = 4000
	t8NoTorqueLimit     = 10000
)

var t8Gears = []string{"Undefined", "First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Reverse"}

func t8Biopower(fw symbol.FirmwareFile) bool {
	return has(fw, "FFTrqCal.FFTrq_MaxEngineTab1") || has(fw, "FFTrqCal.FFTrq_MaxEngineTab2")
}

func (t8) Options(fw symbol.FirmwareFile) []Option {
	return []Option{
		{Key: "automatic", Label: "Automatic gearbox", Kind: OptionBool},
		{Key: "e85", Label: "E85 fuel", Kind: OptionBool, Disabled: !t8Biopower(fw)},
		{Key: "highoutput", Label: "High output (175/210 hp)", Kind: OptionBool, Default: 1},
		{Key: "overboost", Label: "View in overboost", Kind: OptionBool, Disabled: !nonZero(fw, "TrqLimCal.EnableOverBoost")},
		{Key: "fwlimit", Label: "Firmware limited (400/350 Nm)", Kind: OptionBool, Default: 1},
		{Key: "nominal", Label: "Torque from nominal map", Kind: OptionBool},
		{Key: "gear", Label: "Gear", Kind: OptionChoice, Choices: t8Gears, Default: 5},
		{Key: "efficiency", Label: "Torque efficiency (%)", Kind: OptionNumber, Default: 100},
	}
}

type t8calc struct {
	fuelCalc
	automatic, overboost bool
	fwLimit, nominal     bool
	highOutput           bool
	gear                 int
	eff                  float64

	pedalMap, rpmAxis, pedalAxis             []int
	airTorqueMap, airTorqueX, airTorqueY     []int
	bstknkMax, bstknkMaxAu, bstknkX, bstknkY []int
	ffMaxAirmass                             []int
	nominalMap, nominalX                     []int
	trqLimEng, trqLimE85                     []int
	trqLimAuto, trqLimGear                   []int
	trqLimOverboost                          []int
	egtMap                                   []int
	fuelcutLimit                             int
}

func (t8) Calculate(fw symbol.FirmwareFile, opts map[string]float64) (*Result, error) {
	c := &t8calc{
		automatic:  opts["automatic"] == 1,
		overboost:  opts["overboost"] == 1,
		fwLimit:    opts["fwlimit"] == 1,
		nominal:    opts["nominal"] == 1,
		highOutput: opts["highoutput"] == 1,
		gear:       int(opts["gear"]),
		eff:        effFactor(opts),
	}
	c.e85 = opts["e85"] == 1
	c.veScale = 128 // T8 VE table: 1.00 = 128

	l := &symLoader{fw: fw}
	c.pedalMap = l.ints("PedalMapCal.Trq_RequestMap")
	c.rpmAxis = l.ints("PedalMapCal.n_EngineMap")
	c.pedalAxis = l.ints("PedalMapCal.X_PedalMap")
	c.airTorqueMap = l.ints("TrqMastCal.m_AirTorqMap")
	c.airTorqueX = unsigned16(l.ints("TrqMastCal.Trq_EngXSP"))
	c.airTorqueY = l.ints("TrqMastCal.n_EngineYSP")
	c.bstknkMax = l.ints("BstKnkCal.MaxAirmass")
	bstknkXName := "BstKnkCal.OffsetXSP"
	if !has(fw, bstknkXName) {
		bstknkXName = "BstKnkCal.fi_offsetXSP" // flexifuel binary
	}
	c.bstknkX = signed16(l.ints(bstknkXName))
	c.bstknkY = l.ints("BstKnkCal.n_EngYSP")
	c.nominalMap = signed16(l.ints("TrqMastCal.Trq_NominalMap"))
	for i := range c.nominalMap {
		c.nominalMap[i] /= 10 // tenths of Nm -> whole Nm
	}
	c.nominalX = l.ints("TrqMastCal.m_AirXSP")
	c.fuelcutLimit = l.first("FCutCal.m_AirInletLimit")

	// old style TrqLimCal.Trq_MaxEngineMan/AutTab1/2, else Trq_MaxEngineTab1/2
	tabNo := "2"
	if c.highOutput {
		tabNo = "1"
	}
	engineTorqueLimiter := "TrqLimCal.Trq_MaxEngineManTab" + tabNo
	if c.automatic {
		engineTorqueLimiter = "TrqLimCal.Trq_MaxEngineAutTab" + tabNo
	}
	if !has(fw, engineTorqueLimiter) {
		engineTorqueLimiter = "TrqLimCal.Trq_MaxEngineTab" + tabNo
	}
	c.trqLimEng = l.ints(engineTorqueLimiter)
	c.trqLimGear = l.ints("TrqLimCal.Trq_ManGear")
	if c.e85 {
		c.trqLimE85 = l.ints("FFTrqCal.FFTrq_MaxEngineTab" + tabNo)
		c.ffMaxAirmass = l.ints("FFAirCal.m_maxAirmass")
		c.veMap = l.ints("FFFuelCal.TempEnrichFacMAP")
	} else {
		c.veMap = l.ints("BFuelCal.TempEnrichFacMap")
	}
	if c.overboost {
		c.trqLimOverboost = l.ints("TrqLimCal.Trq_OverBoostTab")
	}
	c.injConst = l.first("InjCorrCal.InjectorConst")
	c.battCorr = l.ints("InjCorrCal.BattCorrTab")
	c.battCorrY = l.ints("InjCorrCal.BattCorrSP")
	c.veX = l.ints("BFuelCal.AirXSP")
	c.veY = l.ints("BFuelCal.RpmYSP")
	if err := l.err(); err != nil {
		return nil, err
	}
	// optional: absent maps skip their limiter instead of clamping to zero
	if c.automatic {
		if has(fw, "BstKnkCal.MaxAirmassAu") {
			c.bstknkMaxAu = fw.GetByName("BstKnkCal.MaxAirmassAu").Ints()
		} else {
			c.bstknkMaxAu = c.bstknkMax
		}
		tmc := "TMCCal.Trq_MaxEngineLowTab"
		if c.highOutput {
			tmc = "TMCCal.Trq_MaxEngineTab"
		}
		if has(fw, tmc) {
			c.trqLimAuto = fw.GetByName(tmc).Ints()
		}
	}
	if s := fw.GetByName("ExhaustCal.T_Lambda1Map"); s != nil {
		c.egtMap = s.Ints()
	}

	rows, cols := len(c.rpmAxis), len(c.pedalAxis)
	if len(c.pedalMap) < rows*cols {
		return nil, fmt.Errorf("pedal request map is %d cells, expected %dx%d", len(c.pedalMap), rows, cols)
	}

	// full pedal x rpm grid: torque request -> airmass -> limited airmass
	airGrid := make([][]int, cols)
	limGrid := make([][]string, cols)
	for p := 0; p < cols; p++ {
		airGrid[p] = make([]int, rows)
		limGrid[p] = make([]string, rows)
		for i := 0; i < rows; i++ {
			rpm := c.rpmAxis[i]
			request := c.torqueToAirmass(c.pedalMap[p*rows+i], rpm)
			airGrid[p][i], limGrid[p][i] = c.maxAirmass(rpm, request)
		}
	}

	res := &Result{
		RPM:      make([]float64, rows),
		Limiters: make([]string, rows),
	}
	power := make([]float64, rows)
	torque := make([]float64, rows)
	injdc := make([]float64, rows)
	lambda := make([]float64, rows)
	flow := make([]float64, rows)
	egt := make([]float64, rows)
	for i := 0; i < rows; i++ {
		rpm := c.rpmAxis[i]
		air, lim := airGrid[cols-1][i], limGrid[cols-1][i] // WOT column
		tq := c.airmassToTorque(air, rpm)
		dc := c.injectorDC(air, rpm)
		lam := c.targetLambda(air, rpm, dc)
		if dc > 100 {
			dc = 100
		}
		res.RPM[i] = float64(rpm)
		res.Limiters[i] = lim
		power[i] = float64((tq * rpm) / 7024) // integer division, as in the original
		torque[i] = float64(tq)
		injdc[i] = float64(dc)
		lambda[i] = float64(lam)
		flow[i] = float64(c.fuelFlow(air, rpm, lam))
		egt[i] = float64(c.estimateEGT(air, rpm))
	}
	res.Curves = []Curve{
		{Name: "Power (hp)", Values: power, Peak: true},
		{Name: "Torque (Nm)", Values: torque, Peak: true},
		{Name: "Injector DC (%)", Values: injdc},
		{Name: "Target lambda (*100)", Values: lambda},
		{Name: "Fuel flow (l/h)", Values: flow, Hidden: true},
	}
	if len(c.egtMap) > 0 {
		res.Curves = append(res.Curves, Curve{Name: "EGT estimate (°C)", Values: egt, Hidden: true})
	}
	res.Table = airmassTable(res.RPM, c.pedalAxis, airGrid, limGrid)
	return res, nil
}

func (c *t8calc) torqueToAirmass(torque, rpm int) int {
	if torque > 32000 {
		torque = -(65535 - torque) // original quirk: 65535, not 65536
	}
	return cint(interpTable(c.airTorqueMap, c.airTorqueX, c.airTorqueY, rpm, torque))
}

// airmassToTorque estimates torque from airmass; the user's efficiency
// factor scales the estimate only — the limiter chain and fuel math use
// the binary's own maps and stay untouched.
func (c *t8calc) airmassToTorque(airmass, rpm int) int {
	var tq float64
	if c.nominal {
		tq = interpTable(c.nominalMap, c.nominalX, c.airTorqueY, rpm, airmass)
	} else {
		tq = float64(airmass) / 3.1
		if c.e85 {
			tq *= 1.07
		}
		tq *= rpmCorrection(rpm)
	}
	return cint(tq * c.eff)
}

func (c *t8calc) maxAirmass(rpm, request int) (int, string) {
	restricted, trqLim := c.checkTorqueLimiters(rpm, request)
	limiter := ""
	if restricted < request {
		limiter = trqLim
	}
	airLim := ""
	torqueLimited := restricted
	restricted = c.checkAirmassLimiters(rpm, restricted, &airLim)
	restricted = c.checkFuelcut(restricted, &airLim)
	if restricted < torqueLimited {
		limiter = airLim
	}
	return restricted, limiter
}

func (c *t8calc) checkTorqueLimiters(rpm, request int) (int, string) {
	// pre-seeded like the original, whose "no limiter" branches are dead code
	trqLimiter := limTorqueGear
	limited := request
	torque := t8NoTorqueLimit
	if c.fwLimit {
		if c.automatic {
			torque = t8TorqueLimitAuto
		} else {
			torque = t8TorqueLimitManual
		}
	}
	if c.e85 {
		if lim := cint(interpTable(c.trqLimE85, xdummy, c.airTorqueY, rpm, 0)); torque > lim {
			torque = lim
			trqLimiter = limTorqueE85
		}
	} else {
		obLim := 0
		if c.overboost {
			obLim = cint(interpTable(c.trqLimOverboost, xdummy, c.airTorqueY, rpm, 0))
		}
		petrol := cint(interpTable(c.trqLimEng, xdummy, c.airTorqueY, rpm, 0))
		switch {
		case c.overboost && torque > obLim:
			torque = obLim
			trqLimiter = limOverboost
		case c.overboost && torque < obLim && torque > petrol:
			torque = obLim
			trqLimiter = limOverboost
		case torque > petrol:
			torque = petrol
			trqLimiter = limTorqueEngine
		}
	}
	if c.automatic {
		if len(c.trqLimAuto) > 0 {
			if lim := cint(interpTable(c.trqLimAuto, xdummy, c.airTorqueY, rpm, 0)); torque > lim {
				torque = lim
				trqLimiter = limTorqueGear
			}
		}
	} else {
		gears := []int{0, 1, 2, 3, 4, 5, 6, 7}
		if lim := cint(interpTable(c.trqLimGear, xdummy, gears, c.gear, 0)); torque > lim {
			torque = lim
			trqLimiter = limTorqueGear
		}
	}
	if test := cint(interpTable(c.airTorqueMap, c.airTorqueX, c.airTorqueY, rpm, torque)); test < limited {
		limited = test
	}
	return limited, trqLimiter
}

func (c *t8calc) checkAirmassLimiters(rpm, request int, limiter *string) int {
	var tbl []int
	switch {
	case c.automatic:
		tbl = c.bstknkMaxAu
	case c.e85:
		// the original passes the BstKnk x-axis for the flexifuel map too
		tbl = c.ffMaxAirmass
	default:
		tbl = c.bstknkMax
	}
	if lim := cint(interpTable(tbl, c.bstknkX, c.bstknkY, rpm, 0)); lim < request {
		request = lim
		*limiter = limAirmass
	}
	return request
}

func (c *t8calc) checkFuelcut(request int, limiter *string) int {
	if c.fuelcutLimit < request {
		request = c.fuelcutLimit
		*limiter = limFuelCut
	}
	return request
}

func (c *t8calc) estimateEGT(airmass, rpm int) int {
	if len(c.egtMap) == 0 {
		return 0
	}
	egt := interpTable(c.egtMap, c.veX, c.veY, rpm, airmass)
	if len(c.veMap) > 0 {
		if ve := interpTable(c.veMap, c.veX, c.veY, rpm, airmass) / 128; ve != 0 {
			egt *= 1 / ve
		}
	}
	v := cint(egt)
	if v > 50 && c.e85 {
		v -= 50 // correction for E85 fuel, 50 degrees off
	}
	return v
}
