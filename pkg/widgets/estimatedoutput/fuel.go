package estimatedoutput

// fuelCalc holds the injector/VE data shared by the T7 and T8 estimators
// and implements the injector duty, target lambda and fuel flow math from
// ctrlAirmassResult.cs (identical in both suites apart from the VE table
// scale: 1.00 = 100 on T7, 1.00 = 128 on T8).
type fuelCalc struct {
	e85       bool
	injConst  int
	battCorr  []int
	battCorrY []int
	veMap     []int
	veX, veY  []int
	veScale   float64
}

func (f *fuelCalc) afr() float64 {
	if f.e85 {
		return 9.84
	}
	return 14.65
}

// injectorDC returns the injector duty cycle in percent at 13.0 V battery.
func (f *fuelCalc) injectorDC(airmass, rpm int) int {
	if f.injConst <= 0 || len(f.veMap) == 0 {
		return 0
	}
	fuelPerCycle := float64(airmass) / (f.afr() * 1000) // mg/c -> g/c
	injPerMs := float64(f.injConst) / (60 * 1000)
	pw := fuelPerCycle / injPerMs
	pw *= interpTable(f.veMap, f.veX, f.veY, rpm, airmass) / f.veScale
	dc := cint(pw * float64(rpm) / 1200)
	if dc < 100 {
		// battery correction only contributes below 100% DC
		batt := interpTable(f.battCorr, []int{0}, f.battCorrY, 130, 0) / 1000 // ms at 13.0V
		dc = cint((pw + batt) * float64(rpm) / 1200)
	}
	return dc
}

// targetLambda returns lambda * 100.
func (f *fuelCalc) targetLambda(airmass, rpm, injDC int) int {
	if len(f.veMap) == 0 {
		return 0
	}
	ve := interpTable(f.veMap, f.veX, f.veY, rpm, airmass) / f.veScale
	if ve == 0 {
		return 0
	}
	l := 1 / ve * 100
	if injDC > 100 {
		l = l * float64(injDC) / 100
	}
	return cint(l)
}

// fuelFlow returns fuel flow in l/h.
func (f *fuelCalc) fuelFlow(airmass, rpm, lambda int) int {
	if lambda == 0 {
		return 0
	}
	density := 0.742
	if f.e85 {
		density = 0.775
	}
	fuelPerCycle := float64(airmass) / (f.afr() * 1000)
	fuelPerCycle /= float64(lambda) / 100
	combPerSec := float64(rpm * 2 / 60) // integer division, as in the original
	flow := fuelPerCycle * combPerSec   // g/s
	return cint(flow * 3600 / (1000 * density))
}
