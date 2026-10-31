package mbt

import (
	"math"
	"testing"
)

func b235() Model {
	return Model{Engine: Engines[3], Params: DefaultParams(), Burn: DefaultBurn()}
}

// c8 is a fixed criterion for the tests that only need one.
var c8 = Criterion{Fraction: 0.45, Target: 9}

func TestVolumeAtTDCAndBDC(t *testing.T) {
	e := Engines[3] // B235, 90x90
	vTDC, vBDC := e.Volume(0), e.Volume(180)
	if got := vBDC / vTDC; math.Abs(got-e.CR) > 0.01 {
		t.Errorf("volume ratio BDC/TDC = %.3f, want CR %.3f", got, e.CR)
	}
	if got := (vBDC - vTDC) / e.Displacement(); math.Abs(got-1) > 1e-9 {
		t.Errorf("swept volume = %.6f displacements, want 1", got)
	}
}

// The Vibe shape factors must reproduce the burn angle definitions they
// were derived from: 10% burned at Δθd, 85% at Δθd+Δθb.
func TestVibeShapeFactors(t *testing.T) {
	dev, fast := 15.0, 25.0
	burn := 2*dev + fast
	m := math.Log(math.Log(1-0.1)/math.Log(1-0.85))/(math.Log(dev)-math.Log(dev+fast)) - 1
	a := -math.Log(1-0.1) * math.Pow(burn/dev, m+1)
	mfb := func(x float64) float64 { return 1 - math.Exp(-a*math.Pow(x/burn, m+1)) }
	if got := mfb(dev); math.Abs(got-0.10) > 0.001 {
		t.Errorf("MFB at Δθd = %.4f, want 0.10", got)
	}
	if got := mfb(dev + fast); math.Abs(got-0.85) > 0.001 {
		t.Errorf("MFB at Δθd+Δθb = %.4f, want 0.85", got)
	}
}

func TestTraceIsPhysical(t *testing.T) {
	m := b235()
	// ~1.0 bar boost, mid rpm, open loop
	r := m.Trace(Point{RPM: 3000, Air: 900, Lambda: 0.82, Ign: 14})
	if r.PPA < 40e5 || r.PPA > 160e5 {
		t.Errorf("peak pressure %.1f bar, expected 40-160 bar at 900 mg/c", r.PPA/1e5)
	}
	if r.PPL < 5 || r.PPL > 40 {
		t.Errorf("peak pressure location %.1f°ATDC, expected 5-40", r.PPL)
	}
	if r.IMEP < 5e5 {
		t.Errorf("gross IMEP %.1f bar, expected well above 5 at full load", r.IMEP/1e5)
	}
	// motored: no combustion in range, peak must sit at TDC
	mot := m.Trace(Point{RPM: 3000, Air: 900, Lambda: 0.82, Ign: -400})
	if math.Abs(mot.PPL) > traceStep {
		t.Errorf("motored peak at %.1f°ATDC, want TDC", mot.PPL)
	}
}

// MBT must put the chosen burn fraction exactly on its target angle, and
// produce sane advance.
func TestMBTHitsTarget(t *testing.T) {
	m := b235()
	for _, c := range []Criterion{{0.45, 9}, {0.50, 10}, {0.45, 0}, {0.30, 5}} {
		pt := Point{RPM: 3000, Air: 500, Lambda: 0.9}
		pt.Ign = m.MBT(pt, c)
		r := m.Trace(pt)
		got := r.MFB45
		if c.Fraction == 0.50 {
			got = r.MFB50
		} else if c.Fraction != 0.45 {
			continue // Result only carries the two standard descriptors
		}
		if math.Abs(got-c.Target) > 1e-9 {
			t.Errorf("%.0f%% at target %.0f°ATDC: landed at %.3f", c.Fraction*100, c.Target, got)
		}
	}
	// a B235 at full load wants roughly 12-20°, at light load roughly 20-32°
	if a := m.MBT(Point{RPM: 3000, Air: 1100, Lambda: 0.8}, c8); a < 10 || a > 22 {
		t.Errorf("MBT at 1100 mg/c = %.1f°BTDC, expected roughly 12-20", a)
	}
	if a := m.MBT(Point{RPM: 3000, Air: 150, Lambda: 1.0}, c8); a < 18 || a > 34 {
		t.Errorf("MBT at 150 mg/c = %.1f°BTDC, expected roughly 20-32", a)
	}
}

// Retarding from MBT must cost IMEP, and the cost must grow with retard.
func TestIMEPLossGrowsWithRetard(t *testing.T) {
	m := b235()
	pt := Point{RPM: 2500, Air: 600, Lambda: 0.85}
	mbt := m.MBT(pt, c8)
	var prev float64
	for _, retard := range []float64{0, 4, 8, 12} {
		p := pt
		p.Ign = mbt - retard
		loss := m.IMEPLoss(p, c8)
		if retard > 0 && loss <= prev {
			t.Errorf("loss at -%.0f° (%.3f) not above loss at the previous step (%.3f)", retard, loss, prev)
		}
		prev = loss
	}
	if prev > 0.5 {
		t.Errorf("12° retard costs %.0f%% IMEP, implausible", prev*100)
	}
}

// Burn angle must shorten with load and lengthen when lean, or the whole
// map comes out flat.
func TestBurnModelTrends(t *testing.T) {
	b := DefaultBurn()
	lowDev, _ := b.At(2000, 200, 1.0)
	highDev, _ := b.At(2000, 900, 1.0)
	if highDev >= lowDev {
		t.Errorf("burn angle %.1f at 900 mg/c not shorter than %.1f at 200 mg/c", highDev, lowDev)
	}
	richDev, _ := b.At(2000, 400, 0.85)
	leanDev, _ := b.At(2000, 400, 1.15)
	if leanDev <= richDev {
		t.Errorf("burn angle %.1f at λ1.15 not longer than %.1f at λ0.85", leanDev, richDev)
	}
}

// The Vibe inversion must land the standard fractions where the Vibe
// definitions say, and 45% must come before 50%.
func TestVibeAngleInversion(t *testing.T) {
	dev, fast := 15.0, 25.0
	a, m, burn := vibeShape(dev, fast)
	for _, x := range []float64{0.10, 0.45, 0.50, 0.85} {
		th := vibeAngle(dev, fast, x)
		back := 1 - math.Exp(-a*math.Pow(th/burn, m+1))
		if math.Abs(back-x) > 1e-9 {
			t.Errorf("vibeAngle(%.2f) = %.4f CAD, which burns %.4f", x, th, back)
		}
	}
	if vibeAngle(dev, fast, 0.45) >= vibeAngle(dev, fast, 0.50) {
		t.Error("45% burn point is not before the 50% point")
	}
}

// Table 4 of dissertation 580 publication 7 is adopted here, not
// reproduced: measuring how far each descriptor's *optimum* wanders needs a
// true MBT, and this model's IMEP optimum is not trustworthy enough to
// serve as one (see MBT). What is checked is the weaker, non-circular half:
// with the 45% descriptor held on target across a wide burn-angle sweep,
// the 50% position still wanders and the peak pressure position wanders
// much further — the ordering table 4 reports. The 45% spread is zero by
// construction, since it is what MBT is solved for.
func TestDescriptorRobustness(t *testing.T) {
	base := b235()
	pt := Point{RPM: 3000, Air: 600, Lambda: 0.85}

	// hold ignition at the unscaled model's MBT, then vary the burn scale
	// over a wide range and see how far each descriptor drifts
	pt.Ign = base.MBT(pt, c8)
	spread := func(pick func(Result) float64) float64 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, s := range []float64{0.6, 0.8, 1.0, 1.3, 1.6} {
			m := base
			m.Burn.Scale = s
			p := pt
			p.Ign = m.MBT(p, c8) // each keeps its own MBT
			v := pick(m.Trace(p))
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		return hi - lo
	}
	s45 := spread(func(r Result) float64 { return r.MFB45 })
	s50 := spread(func(r Result) float64 { return r.MFB50 })
	sPP := spread(func(r Result) float64 { return r.PPL })
	t.Logf("spread over a 0.6-1.6 burn scale: 45%% %.2f°, 50%% %.2f°, PPP %.2f°", s45, s50, sPP)

	if s45 > s50 {
		t.Errorf("45%% descriptor drifts %.2f°, more than 50%%'s %.2f°", s45, s50)
	}
	if sPP <= s50 {
		t.Errorf("peak pressure position drifts %.2f°, not more than 50%%'s %.2f° — "+
			"table 4 says it should be much worse", sPP, s50)
	}
}

// Fit must recover a scale it was given.
func TestFitRecoversScale(t *testing.T) {
	m := b235()
	truth := m
	truth.Burn.Scale = 1.4

	pts := []Point{
		{RPM: 1500, Air: 200, Lambda: 1},
		{RPM: 2500, Air: 300, Lambda: 1},
		{RPM: 3500, Air: 250, Lambda: 1},
		{RPM: 4500, Air: 350, Lambda: 1},
	}
	for i := range pts {
		pts[i].Ign = truth.MBT(pts[i], c8)
	}
	got := Fit(m, pts, c8)
	if math.Abs(got-1.4) > 0.05 {
		t.Errorf("Fit = %.3f, want 1.400", got)
	}
}
