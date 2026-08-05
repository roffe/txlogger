package rescaler

import "testing"

func eq(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRescale(t *testing.T) {
	// Surface z(x,y) = x + y on a 2x2 grid, row-major row=Y col=X.
	oldX := []float64{0, 10}
	oldY := []float64{0, 10}
	z := []float64{0, 10, 10, 20}

	// Identity: same axes -> same data.
	if got := Rescale(oldX, oldY, z, oldX, oldY); !eq(got, z) {
		t.Fatalf("identity rescale changed data: %v", got)
	}

	// Add a midpoint X breakpoint: linear surface => exact midpoints.
	newX := []float64{0, 5, 10}
	want := []float64{0, 5, 10, 10, 15, 20}
	if got := Rescale(oldX, oldY, z, newX, oldY); !eq(got, want) {
		t.Fatalf("midpoint rescale = %v, want %v", got, want)
	}

	// Outside the old range clamps to the edge (no extrapolation).
	if got := Rescale(oldX, oldY, z, []float64{-5, 0}, []float64{0, 0})[0]; got != 0 {
		t.Fatalf("clamp below min = %v, want 0", got)
	}
}
