package estimatedoutput

import "math"

// interpTable is a faithful port of GetInterpolatedTableValue from
// T7Suite/T8Suite ctrlAirmassResult.cs, including its quirks: the axis
// scans keep the LAST index whose axis value is strictly below the lookup
// value (the original loop has no break), values beyond the last breakpoint
// clamp flat, and any out-of-bounds table access yields 0 (the C# catch).
// yaxis selects the row (usually RPM), xaxis the column; table is row-major
// with len(xaxis) columns. 1-D tables pass xaxis []int{0} and xvalue 0.
func interpTable(table, xaxis, yaxis []int, yvalue, xvalue int) float64 {
	if len(table) == 0 || len(xaxis) == 0 || len(yaxis) == 0 {
		return 0
	}
	var yi, xi int
	var ypct, xpct float64
	for i, av := range yaxis {
		if yvalue > av {
			yi = i
			if i < len(yaxis)-1 {
				ypct = math.Abs(float64(av-yvalue)) / float64(yaxis[i+1]-av)
			} else {
				ypct = 0
			}
		}
	}
	for i, av := range xaxis {
		if xvalue > av {
			xi = i
			if i < len(xaxis)-1 {
				xpct = math.Abs(float64(av-xvalue)) / float64(xaxis[i+1]-av)
			} else {
				xpct = 0
			}
		}
	}

	oob := false
	at := func(i int) int {
		if i < 0 || i >= len(table) {
			oob = true
			return 0
		}
		return table[i]
	}
	xl := len(xaxis)
	var a1, a2, b1, b2 int
	switch {
	case yi == len(yaxis)-1 && xi < xl-1:
		// last row, extend with the same values
		a1, a2 = at(yi*xl+xi), at(yi*xl+xi+1)
		b1, b2 = a1, a2
	case yi == len(yaxis)-1 && xi == xl-1:
		v := at(yi*xl + xi)
		if oob {
			return 0
		}
		return float64(v)
	case xi == xl-1:
		a1 = at(yi*xl + xi)
		a2 = a1
		b1 = at((yi+1)*xl + xi)
		b2 = b1
	default:
		a1, a2 = at(yi*xl+xi), at(yi*xl+xi+1)
		b1, b2 = at((yi+1)*xl+xi), at((yi+1)*xl+xi+1)
	}
	if oob {
		return 0
	}

	aval := float64(a1)
	if adiff := math.Abs(float64(a1 - a2)); a1 > a2 {
		aval = float64(a1) - xpct*adiff
	} else {
		aval = float64(a1) + xpct*adiff
	}
	bval := float64(b1)
	if bdiff := math.Abs(float64(b1 - b2)); b1 > b2 {
		bval = float64(b1) - xpct*bdiff
	} else {
		bval = float64(b1) + xpct*bdiff
	}
	if abdiff := math.Abs(aval - bval); aval > bval {
		return aval - ypct*abdiff
	} else {
		return aval + ypct*abdiff
	}
}
