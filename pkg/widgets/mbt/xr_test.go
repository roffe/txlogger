package mbt

import (
	"math"
	"testing"
)

// Residual fraction only enters Tivc and the combustion temperature rise,
// never the burn angles, so it moves the pressure trace but cannot move
// MBT. Worth pinning down: it is why a better residual model (e.g. the
// pressure-ratio one in Eriksson et al., IFAC 2002, eq. 3) would not
// improve the ignition answer, only the peak pressure figure. If residuals
// are ever coupled into BurnModel this test fires, which is the point.
func TestXrDoesNotMoveMBT(t *testing.T) {
	pt := Point{RPM: 3000, Air: 900, Lambda: 0.82}
	base := Model{Engine: Engines[3], Params: DefaultParams(), Burn: DefaultBurn()}

	ref := base.MBT(pt, c8)
	var loPPA, hiPPA float64
	for _, xr := range []float64{0.03, 0.20} {
		m := base
		m.Params.Xr = xr
		if got := m.MBT(pt, c8); got != ref {
			t.Errorf("xr %.2f moved MBT to %.3f, want %.3f", xr, got, ref)
		}
		p := pt
		p.Ign = ref
		if xr < 0.1 {
			loPPA = m.Trace(p).PPA
		} else {
			hiPPA = m.Trace(p).PPA
		}
	}
	// it must still do something, or the knob is dead
	if math.Abs(hiPPA/loPPA-1) < 0.05 {
		t.Errorf("xr barely moved peak pressure: %.1f vs %.1f bar", loPPA/1e5, hiPPA/1e5)
	}
}
