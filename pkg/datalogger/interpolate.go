package datalogger

func NewWBLInterpolator(wb WidebandConfig) func(int) float64 {
	return func(value int) float64 {
		if len(wb.SupportPoints) != len(wb.LambdaValues) {
			return -1
		}
		if value <= wb.SupportPoints[0] {
			return wb.LambdaValues[0]
		}
		if value >= wb.SupportPoints[len(wb.SupportPoints)-1] {
			return wb.LambdaValues[len(wb.LambdaValues)-1]
		}

		for i := 0; i < len(wb.SupportPoints)-1; i++ {
			if value >= wb.SupportPoints[i] && value <= wb.SupportPoints[i+1] {
				x0, y0 := float64(wb.SupportPoints[i]), wb.LambdaValues[i]
				x1, y1 := float64(wb.SupportPoints[i+1]), wb.LambdaValues[i+1]
				return y0 + (y1-y0)*(float64(value)-x0)/(x1-x0)
			}
		}
		return -1
	}
}

type WidebandConfig2 struct {
	SupportPoints []int16 // e.g. AFR or voltage breakpoints
	LambdaValues  []int16 // lambda × 100, so 101 means 1.01
}

// Returns lambda × 100 as int16, matching the 68k routine.
func NewWBLInterpolatorFixed(wb WidebandConfig2) func(int16) int16 {
	return func(value int16) int16 {
		n := len(wb.SupportPoints)
		if n == 0 || n != len(wb.LambdaValues) {
			return -1
		}

		// Min clamp (asm: cmp.w (a0),d5 / move.w (a0),d5)
		if value < wb.SupportPoints[0] {
			value = wb.SupportPoints[0]
		}
		// Max clamp (asm: the d2==d3 special case)
		if value >= wb.SupportPoints[n-1] {
			return wb.LambdaValues[n-1]
		}

		// Find largest pos with SP[pos] <= value (asm searches backward with dbge)
		pos := n - 2
		for pos > 0 && wb.SupportPoints[pos] > value {
			pos--
		}

		spDiff := int32(wb.SupportPoints[pos+1]) - int32(wb.SupportPoints[pos])
		if spDiff == 0 {
			return wb.LambdaValues[pos] // asm's divide-by-zero guard
		}

		// 8-bit fraction, matching asl.l #8 + divs.w (Go truncates toward zero, same as 68k divs)
		fraction := (int32(value) - int32(wb.SupportPoints[pos])) * 256 / spDiff

		// TBLS.W: TAB[pos] + round((TAB[pos+1] - TAB[pos]) * fraction / 256)
		tabDiff := int32(wb.LambdaValues[pos+1]) - int32(wb.LambdaValues[pos])
		product := tabDiff * fraction
		var quotient int32
		if product >= 0 {
			quotient = (product + 128) / 256
		} else {
			quotient = (product - 128) / 256
		}

		return int16(int32(wb.LambdaValues[pos]) + quotient)
	}
}
