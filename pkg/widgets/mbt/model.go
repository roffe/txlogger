// Package mbt estimates MBT ignition timing and cylinder pressure for the
// Saab H-engine family (B204/B205/B234/B235) from the maps in a loaded T7
// binary.
//
// The pressure model is the closed-form one from Ingemar Andersson,
// "Cylinder Pressure and Ionization Current Modeling for Spark Ignited
// Engines", Linköping thesis 962 (2002), chapter 5: a polytropic
// compression asymptote, a polytropic expansion asymptote anchored by an
// ideal Otto constant-volume combustion, and a Vibe pressure ratio
// interpolating between the two. No differential equation to solve, so a
// full 16x18 map is a few milliseconds.
//
// The thesis leaves a set of parameters to be tuned per engine, and none of
// them are observable in a T7 log. They are all knobs on the widget. The
// burn-angle model in particular is a guess with the right shape, not a
// measurement — calibrate it with Fit against the part-load region of a map
// you trust, where the calibration is at MBT and nothing is knock limited.
package mbt

import "math"

// gasR is the specific gas constant for the trapped charge, J/(kg K). Air
// is close enough; fuel vapour and residual shift it by ~1%.
const gasR = 287.0

// Engine is the cylinder geometry.
type Engine struct {
	Name   string
	Bore   float64 // mm
	Stroke float64 // mm
	Rod    float64 // mm, con rod centre to centre
	CR     float64 // geometric compression ratio
}

// Engines are the Saab H-engine variants. Deck height is shared across the
// family, so the 2.0 rods are 6 mm longer than the 2.3 ones to make up the
// stroke difference. Compression ratio varies by variant (L/R/E) — the
// values here are the common turbo ones, edit for the engine at hand.
var Engines = []Engine{
	{Name: "B204", Bore: 90, Stroke: 78, Rod: 159, CR: 9.2},
	{Name: "B205", Bore: 90, Stroke: 78, Rod: 159, CR: 9.3},
	{Name: "B234", Bore: 90, Stroke: 90, Rod: 153, CR: 8.8},
	{Name: "B235", Bore: 90, Stroke: 90, Rod: 153, CR: 9.3},
}

// Displacement is the swept volume of one cylinder, m³.
func (e Engine) Displacement() float64 {
	b := e.Bore / 1000
	return math.Pi / 4 * b * b * (e.Stroke / 1000)
}

// Volume is the cylinder volume in m³ at crank angle theta, degrees ATDC.
func (e Engine) Volume(theta float64) float64 {
	a := e.Stroke / 2000 // crank radius, m
	l := e.Rod / 1000
	b := e.Bore / 1000
	rad := theta * math.Pi / 180
	sin := a * math.Sin(rad)
	s := a*math.Cos(rad) + math.Sqrt(l*l-sin*sin)
	clearance := e.Displacement() / (e.CR - 1)
	return clearance + math.Pi/4*b*b*(l+a-s)
}

// Params are the thesis' tuning parameters plus the charge conditions.
type Params struct {
	Kc, Ke   float64 // polytropic exponents, compression / expansion
	Cv       float64 // J/(kg K), specific heat at the combustion
	QHV      float64 // J/kg, fuel lower heating value
	AFs      float64 // stoichiometric air/fuel mass ratio
	Tim      float64 // K, charge temperature at the inlet valve
	Tr       float64 // K, residual gas temperature
	Xr       float64 // residual gas fraction, 0..1
	ThetaIVC float64 // deg ATDC, inlet valve closing (negative = BTDC)
	ThetaEVO float64 // deg ATDC, exhaust valve opening
}

// DefaultParams are the thesis' engine B values where it gives them
// (section 5.2.3 for the polytropic exponents), pump gasoline for the fuel,
// and Saab H-engine cam timing for the valve angles.
func DefaultParams() Params {
	return Params{
		Kc: 1.25, Ke: 1.30,
		Cv: 1000, QHV: 44e6, AFs: 14.67,
		Tim: 313, Tr: 1000, Xr: 0.07,
		ThetaIVC: -130, ThetaEVO: 140,
	}
}

// BurnModel gives the two burn angles the pressure model needs at an
// arbitrary operating point, by scaling a reference pair. The shapes are
// standard SI-engine behaviour — burn angle in CAD grows slowly with speed,
// shrinks with load as density and turbulence rise, and grows as the
// mixture leans out — but the exponents are not measured. Fit them.
type BurnModel struct {
	Dev  float64 // Δθd, flame development angle (0-10% MFB) at the reference point, CAD
	Fast float64 // Δθb, fast burn angle (10-85% MFB) at the reference point, CAD

	RefRPM    float64
	RefAir    float64 // mg/c
	RefLambda float64

	RPMExp  float64 // burn angle ∝ (rpm/RefRPM)^RPMExp
	LoadExp float64 // burn angle ∝ (air/RefAir)^-LoadExp
	LambdaK float64 // fractional change per unit λ above RefLambda

	Scale float64 // overall multiplier, what Fit adjusts
}

// DefaultBurn is a starting point for a 2.0-2.3 turbo four. The λ slope is
// read off the thesis' figure 6.4(b) (64 CAD at λ=0.88 rising to 72 CAD at
// λ=1.2 on a ~68 CAD base). The two exponents were swept against the
// part-load region (100-500 mg/c above 1200 rpm) of a stock MY2007 B235R
// calibration, where the OEM map should be at MBT: the optimum sits at
// LoadExp 0.15 with RPMExp anywhere in 0.2-0.3, and lands within 2.9° RMS
// of the actual map. One binary is not a validation, so they are knobs.
func DefaultBurn() BurnModel {
	return BurnModel{
		Dev: 15, Fast: 25,
		RefRPM: 2000, RefAir: 400, RefLambda: 1.0,
		RPMExp: 0.25, LoadExp: 0.15, LambdaK: 0.35,
		Scale: 1,
	}
}

// vibeShape returns the Vibe efficiency and shape factors, (5.10) and
// (5.11), plus the total burn angle, for a pair of burn angles.
func vibeShape(dev, fast float64) (a, m, burn float64) {
	burn = 2*dev + fast // (5.12), 0-99% MFB
	m = math.Log(math.Log(1-0.1)/math.Log(1-0.85))/(math.Log(dev)-math.Log(dev+fast)) - 1
	a = -math.Log(1-0.1) * math.Pow(burn/dev, m+1)
	return a, m, burn
}

// vibeAngle returns the crank angle after start of combustion at which
// fraction x of the mass has burned — the exact inverse of (5.9), rather
// than the Δθd + Δθb/2 approximation for the 50% point.
func vibeAngle(dev, fast, x float64) float64 {
	a, m, burn := vibeShape(dev, fast)
	return burn * math.Pow(-math.Log(1-x)/a, 1/(m+1))
}

// At returns the two burn angles at an operating point, in CAD.
func (b BurnModel) At(rpm, air, lambda float64) (dev, fast float64) {
	f := b.Scale
	if f == 0 {
		f = 1
	}
	if b.RefRPM > 0 && rpm > 0 {
		f *= math.Pow(rpm/b.RefRPM, b.RPMExp)
	}
	if b.RefAir > 0 && air > 0 {
		f *= math.Pow(air/b.RefAir, -b.LoadExp)
	}
	f *= 1 + b.LambdaK*(lambda-b.RefLambda)
	if f < 0.1 {
		f = 0.1 // keep the Vibe shape factors finite
	}
	return b.Dev * f, b.Fast * f
}

// Point is one operating point.
type Point struct {
	RPM    float64
	Air    float64 // mg per combustion, the T7 load axis
	Lambda float64
	Ign    float64 // ignition advance, deg BTDC
}

// Model is a complete parameter set.
type Model struct {
	Engine Engine
	Params Params
	Burn   BurnModel
}

// Criterion is the combustion descriptor that defines MBT: the crank angle
// at which a given mass fraction should have burned.
//
// Eriksson, "Spark Advance Modeling and Control", Linköping dissertation
// 580 (1999), publication 7, table 4, measures how far each descriptor's
// optimum moves when the model parameters vary. With the flame development
// and rapid burn angles correlated — which they are here, both scale with
// BurnModel.Scale — the 45% position moves 1.0°, the 50% position 3.0° and
// the peak pressure position 15°, while optimal ignition itself moves 50°.
// So 45% is the criterion to use and peak pressure position is the one to
// avoid. The paper's own recommendation is 45% at 9° ATDC, for which
// "the ignition timing will at most be 1.2 degrees off optimum".
//
// Heat transfer is what makes the target engine-specific (it moves the 45%
// optimum by 20° over the plausible range of the Woschni coefficient) and
// this model has no heat transfer term, so Target stays a knob.
type Criterion struct {
	Fraction float64 // mass fraction burned, 0..1
	Target   float64 // its crank angle at MBT, deg ATDC
}

// DefaultCriterion is publication 7's recommendation.
func DefaultCriterion() Criterion { return Criterion{Fraction: 0.45, Target: 9} }

// Result is the modelled closed part of the cycle at one point.
type Result struct {
	Theta []float64 // deg ATDC
	P     []float64 // Pa
	PPA   float64   // peak pressure, Pa
	PPL   float64   // peak pressure location, deg ATDC
	IMEP  float64   // gross IMEP over IVC..EVO, Pa
	MFB45 float64   // 45% mass fraction burned, deg ATDC
	MFB50 float64   // 50% mass fraction burned, deg ATDC
	Dev   float64   // burn angles actually used, CAD
	Fast  float64
}

// traceStep is the crank angle resolution of a trace, degrees.
const traceStep = 1.0

// Trace runs the pressure model at one operating point.
func (m Model) Trace(pt Point) Result {
	p := m.Params
	e := m.Engine
	lambda := pt.Lambda
	if lambda <= 0 {
		lambda = 1
	}

	dev, fast := m.Burn.At(pt.RPM, pt.Air, lambda)
	soc := -pt.Ign // start of combustion, deg ATDC
	shapeA, shapeM, burn := vibeShape(dev, fast)
	mfb45 := soc + vibeAngle(dev, fast, 0.45)
	mfb50 := soc + vibeAngle(dev, fast, 0.50)

	// Charge trapped at IVC: the map's airmass per combustion plus its
	// fuel, diluted by the residual fraction. Using the load axis directly
	// avoids having to guess a manifold pressure and a volumetric
	// efficiency, which is what the thesis does with pim.
	mTot := pt.Air * 1e-6 * (1 + 1/(lambda*p.AFs)) / (1 - p.Xr)
	tivc := p.Tim*(1-p.Xr) + p.Xr*p.Tr
	vivc := e.Volume(p.ThetaIVC)
	pivc := mTot * gasR * tivc / vivc

	// Compression asymptote, (5.1) and (5.2).
	pc := func(th float64) float64 { return pivc * math.Pow(vivc/e.Volume(th), p.Kc) }
	tc := func(th float64) float64 { return tivc * math.Pow(vivc/e.Volume(th), p.Kc-1) }

	// Ideal Otto combustion, placed at this point's own 50% burn angle
	// rather than at the thesis' fixed MFB50_OPT. The fixed angle works
	// when every cycle is well phased, but this model is asked to trace
	// arbitrary map timing: anchoring the expansion asymptote at a constant
	// angle would leave it unmoved by spark, so retard would only reshape
	// the Vibe blend and IMEPLoss below would read near zero. Tying it to
	// mfb50 makes late combustion expand from a later, larger volume, which
	// is where the lost work actually comes from.
	thetaC := mfb50
	p2, t2 := pc(thetaC), tc(thetaC)
	etaF := 0.95 * math.Min(1, 1.2*lambda-0.2)
	dTcomb := (1 - p.Xr) * p.QHV * etaF / ((lambda*p.AFs + 1) * p.Cv)
	p3 := p2 * (t2 + dTcomb) / t2
	vc3 := e.Volume(thetaC)

	// Expansion asymptote, (5.4).
	pe := func(th float64) float64 { return p3 * math.Pow(vc3/e.Volume(th), p.Ke) }

	// Vibe pressure ratio, (5.9).
	press := func(th float64) float64 {
		if th <= soc {
			return pc(th)
		}
		x := 1 - math.Exp(-shapeA*math.Pow((th-soc)/burn, shapeM+1))
		if x > 1 {
			x = 1
		}
		return (1-x)*pc(th) + x*pe(th)
	}

	res := Result{PPL: p.ThetaIVC, MFB45: mfb45, MFB50: mfb50, Dev: dev, Fast: fast}
	var work, prevP, prevV float64
	for th := p.ThetaIVC; th <= p.ThetaEVO; th += traceStep {
		v := e.Volume(th)
		pr := press(th)
		res.Theta = append(res.Theta, th)
		res.P = append(res.P, pr)
		if pr > res.PPA {
			res.PPA, res.PPL = pr, th
		}
		if len(res.Theta) > 1 {
			work += (pr + prevP) / 2 * (v - prevV) // trapezoid ∮p dV
		}
		prevP, prevV = pr, v
	}
	res.IMEP = work / e.Displacement()
	return res
}

// MBT returns the ignition advance in deg BTDC that satisfies the
// combustion descriptor c. Closed form: burn angles do not depend on spark
// timing in this model, so it is the burn delay to c.Fraction minus the
// angle that fraction should land on.
//
// ponytail: not a search for the model's own IMEP maximum. That optimum
// exists and is cheap to find, but it lands 10-15° advanced of reality
// because the model has no heat transfer term, so it would be a knob that
// only misleads. Relative IMEP between two nearby timings is still sound,
// which is what the loss figure below uses.
func (m Model) MBT(pt Point, c Criterion) float64 {
	dev, fast := m.Burn.At(pt.RPM, pt.Air, pt.Lambda)
	return vibeAngle(dev, fast, c.Fraction) - c.Target
}

// IMEPLoss returns the fraction of gross IMEP given up by running pt.Ign
// instead of MBT, 0..1.
//
// Sanity check against dissertation 580, publication 7, section 6.5, which
// gives 0.03% loss per degree off optimum and 3% at ten degrees — i.e. very
// nearly 0.03·Δ². At 18.7° that rule predicts 10.5% and this model says
// 12.1%, so the two agree to within ~15% despite the missing heat transfer.
func (m Model) IMEPLoss(pt Point, c Criterion) float64 {
	best := pt
	best.Ign = m.MBT(pt, c)
	ref := m.Trace(best).IMEP
	if ref <= 0 {
		return 0
	}
	loss := 1 - m.Trace(pt).IMEP/ref
	if loss < 0 {
		loss = 0
	}
	return loss
}

// Fit returns the BurnModel.Scale that makes predicted MBT best match the
// actual advance at the given points, least squares. Feed it the part-load
// region of a map you trust: that is where the calibration is at MBT and
// the deviation at high load then means something.
//
// ponytail: one scale factor, not a fit of every exponent. Golden section
// over a bounded range beats pulling in an optimiser for three parameters
// nobody can validate anyway.
func Fit(m Model, pts []Point, crit Criterion) float64 {
	if len(pts) == 0 {
		return m.Burn.Scale
	}
	sse := func(scale float64) float64 {
		mm := m
		mm.Burn.Scale = scale
		var s float64
		for _, pt := range pts {
			e := mm.MBT(pt, crit) - pt.Ign
			s += e * e
		}
		return s
	}
	a, b := 0.2, 4.0
	const invPhi, tol = 0.6180339887, 0.001
	c, d := b-invPhi*(b-a), a+invPhi*(b-a)
	fc, fd := sse(c), sse(d)
	for b-a > tol {
		if fc < fd {
			b, d, fd = d, c, fc
			c = b - invPhi*(b-a)
			fc = sse(c)
		} else {
			a, c, fc = c, d, fd
			d = a + invPhi*(b-a)
			fd = sse(d)
		}
	}
	return (a + b) / 2
}
