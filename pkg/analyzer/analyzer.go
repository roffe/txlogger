package analyzer

import (
	"math"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
)

// AnalyzeLambda analyzes lambda values based on stable pedal conditions
func AnalyzeLambda(fw symbol.SymbolCollection, xFrom, yFrom string, logfile logfile.Logfile) ([]int, []int, [][]float64) {
	x := fw.GetByName("IgnNormCal.m_AirXSP")
	y := fw.GetByName("IgnNormCal.n_EngYSP")

	xsp := x.Ints()
	ysp := y.Ints()

	zData := make([][]float64, len(xsp)*len(ysp))
	for i := range zData {
		zData[i] = make([]float64, 0, len(xsp)) // Preallocate with a reasonable capacity
	}

	// Ring buffer to store the last 5 pedal positions

	for rec := logfile.Next(); !rec.EOF; rec = logfile.Next() {
		rpm := rec.Values["ActualIn.n_Engine"]
		air := rec.Values["MAF.m_AirInlet"]
		lambda := rec.Values["Lambda.External"]
		//
		//// Lambda range check
		//if lambda < 0.6 || lambda > 1.2 {
		//	continue
		//}

		xIdx, xFrac := findIndexAndFrac(xsp, air)
		yIdx, yFrac := findIndexAndFrac(ysp, rpm)

		const fracThreshold = 0.50

		if xFrac > fracThreshold {
			continue
		}
		if yFrac > fracThreshold {
			continue
		}

		// Apply any index corrections if needed for boundary cases
		if xIdx >= len(xsp) {
			xIdx = len(xsp) - 1
		}
		if yIdx >= len(ysp) {
			yIdx = len(ysp) - 1
		}

		zPos := clamp(yIdx-1, len(ysp))*len(xsp) + clamp(xIdx-1, len(xsp))
		zData[zPos] = append(zData[zPos], lambda)
	}

	return xsp, ysp, zData
}

// isPedalStable checks if the pedal values in the buffer are within the threshold
func isPedalStable(buffer []float64, threshold float64) bool {
	if len(buffer) == 0 {
		return false
	}

	min, max := buffer[0], buffer[0]
	for _, val := range buffer {
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}

	// Alternative: use standard deviation
	// return stdDev(buffer) < threshold

	// Check if range is within threshold
	return (max - min) <= threshold
}

// stdDev calculates standard deviation of values
// Can be used as alternative stability check
func stdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Calculate mean
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate variance
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))

	// Return standard deviation
	return math.Sqrt(variance)
}

func clamp(i, max int) int {
	if i < 0 {
		return 0
	}
	if i >= max {
		return max - 1
	}
	return i
}

func findIndexAndFrac(axis []int, value float64) (int, float64) {
	n := len(axis)
	if value <= float64(axis[0]) {
		return 1, 0.0
	}
	if value >= float64(axis[n-1]) {
		return n - 1, 1.0
	}
	// Binary search to find the index
	left, right := 0, n-1
	for right-left > 1 {
		mid := (left + right) / 2
		if float64(axis[mid]) > value {
			right = mid
		} else {
			left = mid
		}
	}
	// Calculate fractional part
	frac := (value - float64(axis[left])) / (float64(axis[right]) - float64(axis[left]))
	return right, frac
}
