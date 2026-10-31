package common

import (
	"math"
	"math/rand/v2"
	"testing"
)

// FindMinMaxFloat64 agrees with a plain scan for every slice length, covering
// both the scalar build and the 4-lane SIMD build with its remainder loop.
func TestFindMinMaxFloat64Values(t *testing.T) {
	tests := []struct {
		name             string
		data             []float64
		wantMin, wantMax float64
	}{
		{"single", []float64{7}, 7, 7},
		{"pair", []float64{2, -2}, -2, 2},
		{"three", []float64{1, 3, 2}, 1, 3},
		{"one vector", []float64{4, -1, 9, 0}, -1, 9},
		{"min in tail", []float64{4, 1, 9, 0, -8}, -8, 9},
		{"max in tail", []float64{4, 1, 9, 0, 3, 2, 1, 5, 30}, 0, 30},
		{"extremes in first vector", []float64{-50, 50, 0, 0, 1, 2, 3, 4}, -50, 50},
		{"extremes in second vector", []float64{0, 1, 2, 3, 99, -99, 4, 5}, -99, 99},
		{"all equal", []float64{3.5, 3.5, 3.5, 3.5, 3.5, 3.5}, 3.5, 3.5},
		{"all negative", []float64{-3, -1, -7, -2, -9, -4}, -9, -1},
		{"infinities", []float64{1, math.Inf(1), 2, math.Inf(-1), 3}, math.Inf(-1), math.Inf(1)},
	}
	for _, tt := range tests {
		mn, mx := FindMinMaxFloat64(tt.data)
		if mn != tt.wantMin || mx != tt.wantMax {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", tt.name, mn, mx, tt.wantMin, tt.wantMax)
		}
	}

	rng := rand.New(rand.NewPCG(1, 2))
	for n := 1; n <= 67; n++ {
		data := make([]float64, n)
		for i := range data {
			data[i] = rng.NormFloat64() * 1000
		}
		wmn, wmx := data[0], data[0]
		for _, v := range data {
			wmn, wmx = min(wmn, v), max(wmx, v)
		}
		if mn, mx := FindMinMaxFloat64(data); mn != wmn || mx != wmx {
			t.Fatalf("n=%d: got (%v, %v), want (%v, %v)", n, mn, mx, wmn, wmx)
		}
	}
}
