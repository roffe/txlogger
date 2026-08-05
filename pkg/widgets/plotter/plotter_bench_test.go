package plotter

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

func benchValues(numSeries, numPoints int) map[string][]float64 {
	r := rand.New(rand.NewSource(1))
	values := make(map[string][]float64, numSeries)
	for i := 0; i < numSeries; i++ {
		name := string(rune('A'+i)) + "series"
		data := make([]float64, numPoints)
		v := 0.0
		for j := range data {
			v += r.Float64()*2 - 1
			data[j] = v
		}
		values[name] = data
	}
	return values
}

func benchPlot(b *testing.B, w, h, numSeries, pointsShown int) {
	values := benchValues(numSeries, pointsShown+10)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	series := make([]*TimeSeries, 0, numSeries)
	for name := range values {
		series = append(series, NewTimeSeries(name, values))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clear(img.Pix)
		for _, ts := range series {
			ts.PlotImage(img, values, 0, pointsShown, 1)
		}
	}
}

// Default zoom: 250 points visible
func BenchmarkPlot_1080p_10series_250pts(b *testing.B) { benchPlot(b, 1920, 1080, 10, 250) }
func BenchmarkPlot_1080p_30series_250pts(b *testing.B) { benchPlot(b, 1920, 1080, 30, 250) }

// Zoomed out: 10000 points visible (zoom slider at max = 25*400)
func BenchmarkPlot_1080p_10series_10kpts(b *testing.B) { benchPlot(b, 1920, 1080, 10, 10000) }
func BenchmarkPlot_1080p_30series_10kpts(b *testing.B) { benchPlot(b, 1920, 1080, 30, 10000) }

func BenchmarkClearOnly_1080p(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for i := 0; i < b.N; i++ {
		clear(img.Pix)
	}
}

func BenchmarkBresenhamSingleLine(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	col := color.RGBA{255, 0, 0, 255}
	for i := 0; i < b.N; i++ {
		BresenhamThick(img, 0, 0, 1919, 1079, 1, col)
	}
}
