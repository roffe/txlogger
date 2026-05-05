//go:build goexperiment.simd && amd64

package plotter

import (
	"simd/archsimd"
)

func findMinMaxFloat64(data []float64) (float64, float64) {
	n := len(data)
	if n == 0 {
		return 0, 0
	}
	if n < 4 {
		// scalar fallback for tiny slices
		mn, mx := data[0], data[0]
		for _, v := range data[1:] {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		return mn, mx
	}

	// Process 4 float64s at a time (256-bit AVX)
	vmin := archsimd.LoadFloat64x4Slice(data[:4])
	vmax := vmin

	i := 4
	for i+4 <= n {
		v := archsimd.LoadFloat64x4Slice(data[i : i+4])
		vmin = vmin.Min(v)
		vmax = vmax.Max(v)
		i += 4
	}

	// Reduce 4 lanes → 2 lanes → scalar
	lo := vmin.GetLo()
	hi := vmin.GetHi()
	rmin := lo.Min(hi)
	var minArr [2]float64
	rmin.Store(&minArr)

	lo = vmax.GetLo()
	hi = vmax.GetHi()
	rmax := lo.Max(hi)
	var maxArr [2]float64
	rmax.Store(&maxArr)

	mn := min(minArr[0], minArr[1])
	mx := max(maxArr[0], maxArr[1])

	// Handle remaining elements
	for ; i < n; i++ {
		if data[i] < mn {
			mn = data[i]
		}
		if data[i] > mx {
			mx = data[i]
		}
	}

	return mn, mx
}
