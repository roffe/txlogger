package analyzer

import (
	"fmt"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
)

func AnalyzeLambda(fw symbol.SymbolCollection, xFrom, yFrom string, logfile logfile.Logfile) {
	x := fw.GetByName("IgnNormCal.m_AirXSP")
	y := fw.GetByName("IgnNormCal.n_EngYSP")

	xsp := x.Ints()
	ysp := y.Ints()

	zData := make([][]float64, len(xsp)*len(ysp))
	for i := range zData {
		zData[i] = make([]float64, 0, len(xsp)) // Preallocate with a reasonable capacity
	}

	for rec := logfile.Next(); !rec.EOF; rec = logfile.Next() {
		rpm := rec.Values["ActualIn.n_Engine"]

		air := rec.Values["MAF.m_AirInlet"]

		//pedal := rec.Values["Out.X_AccPedal"]

		lambda := rec.Values["Lambda.External"]

		//oldPos := lf.Pos()
		//lf.Seek(oldPos + 3)
		//rec2 := lf.Get()
		//lambda := rec2.Values["Lambda.External"]
		//lf.Seek(oldPos)

		xIdx, xFrac := findIndexAndFrac(xsp, air)
		yIdx, yFrac := findIndexAndFrac(ysp, rpm)

		if xFrac > 0.2 {
			if yFrac > 0.85 {
				xIdx++
			} else {
				continue
			}
		}

		if yFrac > 0.2 {
			if xFrac > 0.85 {
				yIdx++
			} else {
				continue
			}
		}

		if lambda < 0.6 || lambda > 1.2 {
			continue
		}

		zPos := clamp(yIdx-1, len(ysp))*len(xsp) + clamp(xIdx-1, len(xsp))

		zData[zPos] = append(zData[zPos], lambda)
	}

	fmt.Printf("\t\033[1;34m")
	for i := 0; i < len(xsp); i++ {
		fmt.Printf("%d\t", xsp[i])
	}
	fmt.Println("\033[0m")
	for i := len(ysp) - 1; i >= 0; i-- {
		fmt.Printf("\033[1;34m%d\033[0m\t", ysp[i])
		for j := 0; j < len(xsp); j++ {

			zzs := zData[i*len(xsp)+j]
			// calculate average of zzs
			var sum float64
			for _, value := range zzs {
				sum += value
			}
			var value float64
			if len(zzs) > 0 {
				value = sum / float64(len(zzs))
			} else {
				value = 0.0
			}

			color := "\033[0;36m"
			if value < 0.8 {
				color = "\033[0;33m"
			} else if value < 1.0 {
				color = "\033[0;32m"
			} else if value > 1.0 {
				color = "\033[0;31m"
			}

			fmt.Printf("%s%4.2f\033[0m\t", color, value)
		}
		fmt.Println()
	}
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
