//go:build !goexperiment.simd || !amd64

package common

import "testing"

// An empty slice yields (0, 0), matching the SIMD build (see
// math_simd_test.go, which checks n=0); callers such as the plotter can pass
// an empty series.
func TestFindMinMaxFloat64Empty(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FindMinMaxFloat64(nil) panicked: %v", r)
		}
	}()
	if mn, mx := FindMinMaxFloat64(nil); mn != 0 || mx != 0 {
		t.Fatalf("got (%v, %v), want (0, 0)", mn, mx)
	}
}
