//go:build !goexperiment.simd || !amd64

package plotter

func findMinMaxFloat64(data []float64) (float64, float64) {
	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}
