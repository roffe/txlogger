package interpolate

import (
	"fmt"
)

type InterPolFunc func([]uint16, []uint16, []int, uint16, uint16) (float64, float64, float64, error)

// Helper function to clamp offset values
func clamp(offset, len int) int {
	if offset < 0 {
		return 0
	}
	if offset >= len {
		return len - 1
	}
	return offset
}

func U16_u16_int(xAxis, yAxis []uint16, data []int, xValue uint16, yValue uint16) (float64, float64, float64, error) {
	if len(xAxis) == 0 || len(yAxis) == 0 || len(data) == 0 {
		return 0, 0, 0.0, fmt.Errorf("xAxis, yAxis or data is empty")
	}
	// Find the indices of the nearest x and y values in the arrays
	var xIdx, yIdx int
	for i, x := range xAxis {
		if x >= xValue {
			xIdx = i
			break
		}
	}
	for i, y := range yAxis {
		if y >= yValue {
			yIdx = i
			break
		}
	}

	var xFrac, yFrac float64
	if xIdx > 0 {
		xFrac = float64(xValue-xAxis[xIdx-1]) / float64(xAxis[xIdx]-xAxis[xIdx-1])
	}
	if yIdx > 0 {
		yFrac = float64(yValue-yAxis[yIdx-1]) / float64(yAxis[yIdx]-yAxis[yIdx-1])
	}

	dataLen := len(data)

	// Calculate the offsets in the data array for the four surrounding data points
	offset00 := clamp((yIdx-1)*len(xAxis)+xIdx-1, dataLen)
	offset01 := clamp((yIdx-1)*len(xAxis)+xIdx, dataLen)
	offset10 := clamp(yIdx*len(xAxis)+xIdx-1, dataLen)
	offset11 := clamp(yIdx*len(xAxis)+xIdx, dataLen)

	value00 := float64(data[offset00])
	value01 := float64(data[offset01])
	value10 := float64(data[offset10])
	value11 := float64(data[offset11])
	//log.Printf("%.02f %.02f", value10*.01, value11*.01)
	//log.Printf("%.02f %.02f", value00*.01, value01*.01)

	interpolatedX0 := (1.0-xFrac)*value00 + xFrac*value01
	interpolatedX1 := (1.0-xFrac)*value10 + xFrac*value11
	interpolatedValue := (1.0-yFrac)*interpolatedX0 + yFrac*interpolatedX1
	return float64(xIdx-1) + xFrac, float64(yIdx-1) + yFrac, interpolatedValue, nil
}
