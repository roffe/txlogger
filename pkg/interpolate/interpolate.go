package interpolate

import (
	"fmt"
)

type InterPolFunc func([]int, []int, []int, int, int) (float64, float64, float64, error)

// Helper function to clamp offset values
func clamp(offset, max int) int {
	if offset < 0 {
		return 0
	}
	if offset >= max {
		return max - 1
	}
	return offset
}

// Finds the index and fraction of the nearest value in the given array
func findIndexAndFrac(arr []int, value int) (int, float64) {
	idx := len(arr) - 1
	frac := 0.0

	for i, v := range arr {
		if v >= value {
			idx = i
			break
		}
	}

	if idx > 0 {
		delta := arr[idx] - arr[idx-1]
		frac = float64(value-arr[idx-1]) / float64(delta)
	}

	return idx, frac
}

// returns x, y, value, err
func Interpolate(xAxis, yAxis, data []int, xValue, yValue int) (float64, float64, float64, error) {
	if len(xAxis) == 0 || len(yAxis) == 0 || len(data) == 0 {
		return 0, 0, 0, fmt.Errorf("xAxis, yAxis or data is empty")
	}

	xIdx, xFrac := findIndexAndFrac(xAxis, xValue)
	yIdx, yFrac := findIndexAndFrac(yAxis, yValue)

	dataLen := len(data)
	// Calculate the offsets in the data array for the four surrounding data points
	getOffset := func(i, j int) int {
		return clamp(i*len(xAxis)+j, dataLen)
	}

	offsets := [4]int{
		getOffset(yIdx-1, xIdx-1),
		getOffset(yIdx-1, xIdx),
		getOffset(yIdx, xIdx-1),
		getOffset(yIdx, xIdx),
	}

	values := [4]float64{
		float64(data[offsets[0]]),
		float64(data[offsets[1]]),
		float64(data[offsets[2]]),
		float64(data[offsets[3]]),
	}

	//	log.Printf("%f %.f", values[0], values[1])
	//	log.Printf("%f %.f", values[2], values[3])

	// Perform bilinear interpolation
	interpolatedX0 := (1.0-xFrac)*values[0] + xFrac*values[1]
	interpolatedX1 := (1.0-xFrac)*values[2] + xFrac*values[3]
	interpolatedValue := (1.0-yFrac)*interpolatedX0 + yFrac*interpolatedX1
	return float64(xIdx-1) + xFrac, float64(yIdx-1) + yFrac, interpolatedValue, nil
}

func SetInterpolated(xAxis, yAxis []int, data []int, xValue, yValue, zValue int) error {
	if len(xAxis) == 0 || len(yAxis) == 0 || len(data) == 0 {
		return fmt.Errorf("xAxis, yAxis or data is empty")
	}

	xIdx, xFrac := findIndexAndFrac(xAxis, xValue)
	yIdx, yFrac := findIndexAndFrac(yAxis, yValue)

	dataLen := len(data)

	getOffset := func(i, j int) int {
		return clamp(i*len(xAxis)+j, dataLen)
	}

	offsets := [4]int{
		getOffset(yIdx-1, xIdx-1),
		getOffset(yIdx-1, xIdx),
		getOffset(yIdx, xIdx-1),
		getOffset(yIdx, xIdx),
	}

	// Calculate the adjustments to apply to the surrounding data points based on the interpolation fractions
	adjValues := [4]int{
		int(float64(zValue) * (1.0 - xFrac) * (1.0 - yFrac)),
		int(float64(zValue) * xFrac * (1.0 - yFrac)),
		int(float64(zValue) * (1.0 - xFrac) * yFrac),
		int(float64(zValue) * xFrac * yFrac),
	}

	// Update the data array based on the calculated adjustments
	for i := 0; i < 4; i++ {
		data[offsets[i]] += adjValues[i]
	}

	return nil
}

// Adjusts the surrounding data points to make the interpolated value match zValue
func SetInterpolated2(xAxis, yAxis []int, data []int, xValue, yValue, zValue int) error {
	if len(xAxis) == 0 || len(yAxis) == 0 || len(data) == 0 {
		return fmt.Errorf("xAxis, yAxis or data is empty")
	}

	xIdx, xFrac := findIndexAndFrac(xAxis, xValue)
	yIdx, yFrac := findIndexAndFrac(yAxis, yValue)

	dataLen := len(data)

	getOffset := func(i, j int) int {
		return clamp(i*len(xAxis)+j, dataLen)
	}

	offsets := [4]int{
		getOffset(yIdx-1, xIdx-1),
		getOffset(yIdx-1, xIdx),
		getOffset(yIdx, xIdx-1),
		getOffset(yIdx, xIdx),
	}

	// Calculate the differences to apply to the surrounding data points to make the interpolated value match zValue
	diff := float64(zValue) - ((1.0-yFrac)*((1.0-xFrac)*float64(data[offsets[0]])+xFrac*float64(data[offsets[1]])) + yFrac*((1.0-xFrac)*float64(data[offsets[2]])+xFrac*float64(data[offsets[3]])))

	// Distribute the difference based on the interpolation weights
	adjValues := [4]int{
		int(diff * (1.0 - xFrac) * (1.0 - yFrac)),
		int(diff * xFrac * (1.0 - yFrac)),
		int(diff * (1.0 - xFrac) * yFrac),
		int(diff * xFrac * yFrac),
	}

	// Update the data array based on the calculated adjustments
	for i := 0; i < 4; i++ {
		data[offsets[i]] += adjValues[i]
	}

	return nil
}

func U16_u16_int2(xAxis, yAxis []uint16, data []int, xValue uint16, yValue uint16) (float64, float64, float64, error) {
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
	if xIdx == 0 && xValue > xAxis[len(xAxis)-1] {
		xIdx = len(xAxis) - 1
	}

	for i, y := range yAxis {
		if y >= yValue {
			yIdx = i
			break
		}
	}
	if yIdx == 0 && yValue > yAxis[len(yAxis)-1] {
		yIdx = len(yAxis) - 1
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
