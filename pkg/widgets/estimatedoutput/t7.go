package estimatedoutput

import (
	"fmt"

	symbol "github.com/roffe/ecusymbol"
)

// T7 estimator, ported from T7Suite ctrlAirmassResult.cs. Torque in whole
// Nm, airmass in mg/c; all tables are raw values as stored in the binary.

type t7 struct{}

const (
	t7TorqueLimitAuto   = 350 // TCM limiter
	t7TorqueLimitManual = 400 // ECU hardcoded limiter
	t7NoTorqueLimit     = 1000
)

const (
	limTorqueEngine  = "engine torque limiter"
	limTorqueE85     = "E85 torque limiter"
	limTorqueE85Auto = "E85 auto torque limiter"
	limTorqueGear    = "gear torque limiter"
	limOverboost     = "overboost limiter"
	limAirmass       = "airmass (knock) limiter"
	limTurboSpeed    = "turbo speed limiter"
	limFuelCut       = "fuel cut limiter"
)

var t7Gears = []string{"Reverse", "First", "Second", "Third", "Fourth", "Fifth"}

func (t7) Options(fw symbol.FirmwareFile) []Option {
	return []Option{
		{Key: "automatic", Label: "Automatic gearbox", Kind: OptionBool},
		{Key: "e85", Label: "E85 fuel", Kind: OptionBool, Disabled: !has(fw, "TorqueCal.M_EngMaxE85Tab")},
		{Key: "convertible", Label: "Convertible", Kind: OptionBool, Disabled: !has(fw, "TorqueCal.M_CabGearLim")},
		{Key: "overboost", Label: "View in overboost", Kind: OptionBool, Disabled: !nonZero(fw, "TorqueCal.EnableOverBoost")},
		{Key: "fwlimit", Label: "Firmware limited (400/350 Nm)", Kind: OptionBool, Default: 1},
		{Key: "nominal", Label: "Torque from nominal map", Kind: OptionBool},
		{Key: "gear", Label: "Gear", Kind: OptionChoice, Choices: t7Gears, Default: 5},
		{Key: "ambient", Label: "Ambient pressure (kPa)", Kind: OptionNumber, Default: 100},
		{Key: "efficiency", Label: "Torque efficiency (%)", Kind: OptionNumber, Default: 100},
	}
}

// effFactor returns the user's torque-efficiency option as a factor,
// defaulting to 1.0 when unset or out of a sane range.
func effFactor(opts map[string]float64) float64 {
	eff := opts["efficiency"]
	if eff < 10 || eff > 300 {
		return 1
	}
	return eff / 100
}

type t7calc struct {
	fuelCalc
	automatic, convertible, overboost bool
	fwLimit, nominal                  bool
	e85Auto, torqueLimitEnabled       bool
	gear1st                           bool
	gear                              int
	ambient                           int
	eff                               float64

	pedalMap, rpmAxis, pedalAxis             []int
	airTorqueMap, airTorqueX, airTorqueY     []int
	bstknkMax, bstknkMaxAu, bstknkX, bstknkY []int
	turboSpeed, turboSpeedY                  []int
	turboSpeed2, turboSpeed2Y                []int
	nominalMap, nominalX                     []int
	trqLimEng, trqLimE85, trqLimE85Auto      []int
	trqLimAuto, trqLimConv, trqLimGear       []int
	trqLim1st, trqLim1stY                    []int
	trqLim5th, trqLim5thY                    []int
	trqLimOverboost                          []int
	fuelcutLimit                             int
}

func (t7) Calculate(fw symbol.FirmwareFile, opts map[string]float64) (*Result, error) {
	c := &t7calc{
		automatic:   opts["automatic"] == 1,
		convertible: opts["convertible"] == 1,
		overboost:   opts["overboost"] == 1,
		fwLimit:     opts["fwlimit"] == 1,
		nominal:     opts["nominal"] == 1,
		gear:        int(opts["gear"]),
		ambient:     cint(opts["ambient"]),
		eff:         effFactor(opts),
	}
	c.e85 = opts["e85"] == 1
	c.veScale = 100
	if c.ambient < 50 || c.ambient > 150 {
		c.ambient = 100
	}
	c.e85Auto = c.e85 && c.automatic && has(fw, "TorqueCal.M_EngMaxE85TabAut")
	c.torqueLimitEnabled = !has(fw, "TorqueCal.ST_Loop") || nonZero(fw, "TorqueCal.ST_Loop")
	c.gear1st = has(fw, "TorqueCal.M_1GearTab")

	l := &symLoader{fw: fw}
	c.pedalMap = l.ints("PedalMapCal.m_RequestMap")
	c.rpmAxis = l.ints("PedalMapCal.n_EngineMap")
	c.pedalAxis = l.ints("PedalMapCal.X_PedalMap")
	c.airTorqueMap = l.ints("TorqueCal.m_AirTorqMap")
	c.airTorqueX = unsigned16(l.ints("TorqueCal.M_EngXSP"))
	c.airTorqueY = l.ints("TorqueCal.n_EngYSP")
	c.bstknkMax = l.ints("BstKnkCal.MaxAirmass")
	c.bstknkX = signed16(l.ints("BstKnkCal.OffsetXSP"))
	c.bstknkY = l.ints("BstKnkCal.n_EngYSP")
	c.nominalMap = signed16(l.ints("TorqueCal.M_NominalMap"))
	c.nominalX = l.ints("TorqueCal.m_AirXSP")
	c.fuelcutLimit = l.first("FCutCal.m_AirInletLimit")
	c.trqLimEng = l.ints("TorqueCal.M_EngMaxTab")
	c.trqLimAuto = l.ints("TorqueCal.M_EngMaxAutTab")
	c.trqLimGear = l.ints("TorqueCal.M_ManGearLim")
	c.injConst = l.first("InjCorrCal.InjectorConst")
	c.battCorr = l.ints("InjCorrCal.BattCorrTab")
	c.battCorrY = l.ints("InjCorrCal.BattCorrSP")
	c.veX = l.ints("BFuelCal.AirXSP")
	c.veY = l.ints("BFuelCal.RpmYSP")
	if c.e85 {
		c.veMap = l.ints("BFuelCal.StartMap")
		c.trqLimE85 = l.ints("TorqueCal.M_EngMaxE85Tab")
	} else {
		c.veMap = l.ints("BFuelCal.Map")
	}
	if c.e85Auto {
		c.trqLimE85Auto = l.ints("TorqueCal.M_EngMaxE85TabAut")
	}
	if c.convertible {
		c.trqLimConv = l.ints("TorqueCal.M_CabGearLim")
	}
	if c.overboost {
		c.trqLimOverboost = l.ints("TorqueCal.M_OverBoostTab")
	}
	if !c.automatic && c.gear == 1 && c.gear1st {
		c.trqLim1st = l.ints("TorqueCal.M_1GearTab")
		c.trqLim1stY = l.ints("TorqueCal.n_Eng1GearSP")
	}
	if !c.automatic && c.gear == 5 {
		c.trqLim5th = l.ints("TorqueCal.M_5GearLimTab")
		c.trqLim5thY = l.ints("TorqueCal.n_Eng5GearSP")
	}
	if err := l.err(); err != nil {
		return nil, err
	}
	// optional: absent maps skip their limiter instead of clamping to zero
	// like the original does on a missing symbol
	if c.automatic && has(fw, "BstKnkCal.MaxAirmassAu") {
		c.bstknkMaxAu = fw.GetByName("BstKnkCal.MaxAirmassAu").Ints()
	}
	if has(fw, "LimEngCal.TurboSpeedTab") && has(fw, "LimEngCal.p_AirSP") &&
		has(fw, "LimEngCal.TurboSpeedTab2") && has(fw, "LimEngCal.n_EngSP") {
		c.turboSpeed = fw.GetByName("LimEngCal.TurboSpeedTab").Ints()
		c.turboSpeedY = fw.GetByName("LimEngCal.p_AirSP").Ints()
		c.turboSpeed2 = fw.GetByName("LimEngCal.TurboSpeedTab2").Ints()
		c.turboSpeed2Y = fw.GetByName("LimEngCal.n_EngSP").Ints()
	}

	rows, cols := len(c.rpmAxis), len(c.pedalAxis)
	if len(c.pedalMap) < rows*cols {
		return nil, fmt.Errorf("pedal request map is %d cells, expected %dx%d", len(c.pedalMap), rows, cols)
	}

	// full pedal x rpm grid of limited airmass
	airGrid := make([][]int, cols)
	limGrid := make([][]string, cols)
	for p := 0; p < cols; p++ {
		airGrid[p] = make([]int, rows)
		limGrid[p] = make([]string, rows)
		for i := 0; i < rows; i++ {
			airGrid[p][i], limGrid[p][i] = c.maxAirmass(c.rpmAxis[i], c.pedalMap[p*rows+i])
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
	}
	res.Curves = []Curve{
		{Name: "Power (hp)", Values: power, Peak: true},
		{Name: "Torque (Nm)", Values: torque, Peak: true},
		{Name: "Injector DC (%)", Values: injdc},
		{Name: "Target lambda (*100)", Values: lambda},
		{Name: "Fuel flow (l/h)", Values: flow, Hidden: true},
	}
	res.Table = airmassTable(res.RPM, c.pedalAxis, airGrid, limGrid)
	return res, nil
}

// airmassTable arranges a pedal x rpm airmass grid as a display table with
// the highest pedal position on top; row headers are pedal position in %.
func airmassTable(rpm []float64, pedalAxis []int, airGrid [][]int, limGrid [][]string) *Table {
	tbl := &Table{Title: "Limited airmass (mg/c)", Cols: rpm}
	for p := len(airGrid) - 1; p >= 0; p-- {
		row := make([]float64, len(airGrid[p]))
		for i, v := range airGrid[p] {
			row[i] = float64(v)
		}
		tbl.Cells = append(tbl.Cells, row)
		tbl.Limiters = append(tbl.Limiters, limGrid[p])
		tbl.Rows = append(tbl.Rows, float64(pedalAxis[p])/10)
	}
	return tbl
}

func (c *t7calc) maxAirmass(rpm, request int) (int, string) {
	restricted := request
	limiter := ""
	if c.torqueLimitEnabled {
		r, trqLim := c.checkTorqueLimiters(rpm, request)
		restricted = r
		if restricted < request {
			limiter = trqLim
		}
	}
	airLim := ""
	torqueLimited := restricted
	restricted = c.checkAirmassLimiters(rpm, restricted, &airLim)
	restricted = c.checkTurboSpeed(rpm, restricted, &airLim)
	restricted = c.checkFuelcut(restricted, &airLim)
	if restricted < torqueLimited {
		limiter = airLim
	}
	return restricted, limiter
}

var xdummy = []int{0}

func (c *t7calc) checkTorqueLimiters(rpm, request int) (int, string) {
	// pre-seeded like the original, whose "no limiter" branches are dead code
	trqLimiter := limTorqueGear
	limited := request
	torque := t7NoTorqueLimit
	if c.fwLimit {
		if c.automatic {
			torque = t7TorqueLimitAuto
		} else {
			torque = t7TorqueLimitManual
		}
	}
	switch {
	case c.e85Auto:
		if lim := cint(interpTable(c.trqLimE85Auto, xdummy, c.airTorqueY, rpm, 0)); torque > lim {
			torque = lim
			trqLimiter = limTorqueE85Auto
		}
	case c.e85:
		if lim := cint(interpTable(c.trqLimE85, xdummy, c.airTorqueY, rpm, 0)); torque > lim {
			torque = lim
			trqLimiter = limTorqueE85
		}
	case c.automatic:
		if lim := cint(interpTable(c.trqLimAuto, xdummy, c.airTorqueY, rpm, 0)); torque > lim {
			torque = lim
			trqLimiter = limTorqueEngine
		}
	default:
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
	if !c.automatic {
		gears := []int{0, 1, 2, 3, 4, 5}
		if c.convertible {
			if lim := cint(interpTable(c.trqLimConv, xdummy, gears, c.gear, 0)); torque > lim {
				torque = lim
				trqLimiter = limTorqueGear
			}
		}
		if lim := cint(interpTable(c.trqLimGear, xdummy, gears, c.gear, 0)); torque > lim {
			torque = lim
			trqLimiter = limTorqueGear
		}
		if c.gear == 1 && c.gear1st {
			if lim := cint(interpTable(c.trqLim1st, xdummy, c.trqLim1stY, rpm, 0)); torque > lim {
				torque = lim
				trqLimiter = limTorqueGear
			}
		} else if c.gear == 5 {
			if lim := cint(interpTable(c.trqLim5th, xdummy, c.trqLim5thY, rpm, 0)); torque > lim {
				torque = lim
				trqLimiter = limTorqueGear
			}
		}
	}
	if test := cint(interpTable(c.airTorqueMap, c.airTorqueX, c.airTorqueY, rpm, torque)); test < limited {
		limited = test
	}
	return limited, trqLimiter
}

func (c *t7calc) checkAirmassLimiters(rpm, request int, limiter *string) int {
	tbl := c.bstknkMax
	if c.automatic && len(c.bstknkMaxAu) > 0 {
		tbl = c.bstknkMaxAu
	}
	if lim := cint(interpTable(tbl, c.bstknkX, c.bstknkY, rpm, 0)); lim < request {
		request = lim
		*limiter = limAirmass
	}
	return request
}

func (c *t7calc) checkTurboSpeed(rpm, request int, limiter *string) int {
	if len(c.turboSpeed) == 0 {
		return request
	}
	ambient := c.ambient * 10 // table unit is 0.1 kPa
	lim := cint(interpTable(c.turboSpeed, xdummy, c.turboSpeedY, ambient, 0))
	cf := cint(interpTable(c.turboSpeed2, xdummy, c.turboSpeed2Y, rpm, 0))
	lim = lim * cf / 1000 // correction factor unit is 0.001
	if lim < request {
		request = lim
		*limiter = limTurboSpeed
	}
	return request
}

func (c *t7calc) checkFuelcut(request int, limiter *string) int {
	if c.fuelcutLimit < request {
		request = c.fuelcutLimit
		*limiter = limFuelCut
	}
	return request
}

// airmassToTorque estimates torque from airmass; the user's efficiency
// factor scales the estimate only — the limiter chain and fuel math use
// the binary's own maps and stay untouched.
func (c *t7calc) airmassToTorque(airmass, rpm int) int {
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

// rpmCorrection is the suites' fixed high-rpm torque roll-off for the
// simple airmass/3.1 torque model.
func rpmCorrection(rpm int) float64 {
	switch {
	case rpm >= 6000:
		return 0.85
	case rpm >= 5820:
		return 0.94
	case rpm >= 5440:
		return 0.95
	case rpm >= 5060:
		return 0.99
	default:
		return 1.00
	}
}
