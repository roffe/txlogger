package boosttuner

import (
	"math"
	"os"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

const testBinary = "/home/roffe/temp/bosse.bin"

// TestRegMapBilerp_RealBinary checks that, against a real T7 binary, the RegMap is
// read row-major [rpm][load], the %->raw conversion is right, and bilerp returns
// the stored cell values at the axis breakpoints. Skips when the binary is absent.
func TestRegMapBilerp_RealBinary(t *testing.T) {
	data, err := os.ReadFile(testBinary)
	if err != nil {
		t.Skipf("test binary not available: %v", err)
	}
	_, syms, err := symbol.Load(testBinary, data, func(string) {})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	x := syms.GetByName(symSetLoadXSP).Float64s()
	y := syms.GetByName(symNEngSP).Float64s()
	regPct := syms.GetByName(symRegMap).Float64s()
	if len(x)*len(y) != len(regPct) {
		t.Fatalf("RegMap %d != %d x %d", len(regPct), len(x), len(y))
	}
	raw := make([]float64, len(regPct))
	for i, v := range regPct {
		raw[i] = v * dutyRawPerPct
	}

	// First cell is [rpm[0]][load[0]]; last is [rpm[last]][load[last]].
	first := regPct[0] * dutyRawPerPct
	last := regPct[len(regPct)-1] * dutyRawPerPct
	if got := bilerp(x, y, raw, x[0], y[0]); math.Abs(got-first) > 1e-6 {
		t.Errorf("bilerp at first breakpoint = %v, want %v", got, first)
	}
	if got := bilerp(x, y, raw, x[len(x)-1], y[len(y)-1]); math.Abs(got-last) > 1e-6 {
		t.Errorf("bilerp at last breakpoint = %v, want %v", got, last)
	}

	// An interior breakpoint must equal its exact stored cell (row-major index).
	r, c := len(y)/2, len(x)/2
	want := regPct[r*len(x)+c] * dutyRawPerPct
	if got := bilerp(x, y, raw, x[c], y[r]); math.Abs(got-want) > 1e-6 {
		t.Errorf("bilerp at interior breakpoint = %v, want %v", got, want)
	}
}
