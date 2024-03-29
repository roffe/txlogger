package plotter

import (
	"hash/crc32"
	"image"
	"image/color"
	"log"
)

type TimeSeries struct {
	Name       string
	Min        float64
	Max        float64
	valueRange float64
	Color      color.RGBA
}

func NewTimeSeries(name string, values map[string][]float64) TimeSeries {
	ts := TimeSeries{
		Name:  name,
		Color: hashToRGB(name),
	}

	data, ok := values[name]
	if !ok {
		log.Println("Time series", name, "not found")
		return ts
	}

	ts.Min, ts.Max = findMinMaxFloat64(data)
	ts.valueRange = ts.Max - ts.Min
	return ts
}

func (ts *TimeSeries) Plot(values map[string][]float64, start, numPoints int, w, h int) image.Image {
	//log.Println("Plotting", ts.Name, "from", start, "to", numPoints, "width", w)
	dl := len(values[ts.Name]) - 1
	startN, endN := min(max(start, 0), dl), min(start+numPoints, dl)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	hh := h - 1
	dataLen := endN - startN
	heightFactor := float64(hh) / ts.valueRange
	widthFactor := float64(w) / float64(dataLen)

	// start at 1 since we need to draw a line from the previous point
	data := values[ts.Name][startN:endN]
	dle := dataLen - 1

	for x := 1; x < dataLen; x++ {
		fx := float64(x)
		x0 := int(((fx - 1) * widthFactor))
		y0 := int(float64(hh) - (data[x-1]-ts.Min)*heightFactor)

		x1 := (int(fx * widthFactor))
		if x == dle {
			x1 = w
		}
		y1 := int(float64(hh) - (data[x]-ts.Min)*heightFactor)

		Bresenham(img, x0, y0, x1, y1, ts.Color)

	}
	return img
}

func (ts *TimeSeries) PlotImage(img *image.RGBA, values map[string][]float64, start, numPoints int) {
	dl := len(values[ts.Name]) - 1
	startN, endN := min(max(start, 0), dl), min(start+numPoints, dl)

	s := img.Bounds().Size()
	w := s.X
	h := s.Y

	//log.Println("Plotting", ts.Name, "from", start, "to", numPoints, "width", w, "height", h)
	hh := h - 1
	dataLen := endN - startN
	heightFactor := float64(hh) / ts.valueRange
	widthFactor := float64(w) / float64(dataLen)

	// start at 1 since we need to draw a line from the previous point
	data := values[ts.Name][startN:endN]
	dle := dataLen - 1

	for x := 1; x < dataLen; x++ {
		fx := float64(x)
		x0 := int(((fx - 1) * widthFactor))
		y0 := int(float64(hh) - (data[x-1]-ts.Min)*heightFactor)
		x1 := (int(fx * widthFactor))
		if x == dle {
			x1 = w
		}
		y1 := int(float64(hh) - (data[x]-ts.Min)*heightFactor)
		Bresenham(img, x0, y0, x1, y1, ts.Color)
	}
}

/* func (ts *TimeSeries) newImage(data []float64, w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	hh := h - 1
	heightFactor := float64(hh) / ts.valueRange
	widthFactor := float64(w) / float64(len(data))
	// start at 1 since we need to draw a line from the previous point

	dataLen := len(data)
	for x := 1; x < dataLen; x++ {
		x0 := int((float64(x-1) * widthFactor))
		x1 := int((float64(x) * widthFactor))
		y0 := int(float64(hh) - (data[x-1]-ts.Min)*heightFactor)
		y1 := int(float64(hh) - (data[x]-ts.Min)*heightFactor)
		Bresenham(img, x0, y0, x1, y1, ts.Color)
	}

	return img
} */

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

func hashToRGB(input string) color.RGBA {
	// Calculate CRC32 hash
	hash := crc32.ChecksumIEEE([]byte(input))
	// Map the hash value to RGB color space
	return color.RGBA{byte(hash >> 8), byte(hash >> 16), byte(hash), 255}
}
