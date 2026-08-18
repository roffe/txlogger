package windows

import "testing"

func TestLookup1D(t *testing.T) {
	// MaxLoadNormTab rpm axis + values from the reference T7 binary.
	rpm := []float64{700, 880, 1260, 1640, 2020, 2400, 2780, 3160, 3540, 3920, 4300, 4680, 5060, 5440, 5820, 6200}
	max := []float64{650, 650, 650, 650, 650, 650, 660, 660, 660, 650, 570, 510, 450, 330, 330, 330}

	cases := []struct {
		x, want float64
	}{
		{700, 650},  // first breakpoint
		{6200, 330}, // last breakpoint
		{300, 650},  // below range -> clamp low
		{9000, 330}, // above range -> clamp high
		{5060, 450}, // exact breakpoint mid-table
		{4490, 540}, // halfway between 4300(570) and 4680(510)
	}

	for _, c := range cases {
		if got := lookup1D(rpm, max, c.x); got != c.want {
			t.Errorf("lookup1D(%g) = %g, want %g", c.x, got, c.want)
		}
	}

	// Boundary mapping: at rpm 5060 the closed-loop limit is 450 mg/c, so airmass
	// 420 is closed loop and 480 is open loop.
	limit := lookup1D(rpm, max, 5060)
	if !(420 <= limit) || 480 <= limit {
		t.Errorf("closed-loop boundary wrong: limit=%g, want 420<=limit<480", limit)
	}
}
