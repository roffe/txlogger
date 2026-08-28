//go:build goexperiment.simd && amd64

package common

import (
	"math/rand"
	"testing"
)

func TestFindMinMaxFloat64(t *testing.T) {
	for n := 0; n < 40; n++ {
		data := make([]float64, n)
		for i := range data {
			data[i] = rand.NormFloat64()
		}
		mn, mx := FindMinMaxFloat64(data)
		wmn, wmx := 0.0, 0.0
		if n > 0 {
			wmn, wmx = data[0], data[0]
			for _, v := range data {
				wmn, wmx = min(wmn, v), max(wmx, v)
			}
		}
		if mn != wmn || mx != wmx {
			t.Fatalf("n=%d got (%v,%v) want (%v,%v)", n, mn, mx, wmn, wmx)
		}
	}
}
