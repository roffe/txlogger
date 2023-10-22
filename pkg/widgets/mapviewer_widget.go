package widgets

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/interpolate"
)

const (
	cellWidth  = 46
	cellHeight = 26
)

type xyUpdate struct {
	x, y int
}

type MapViewer struct {
	tik uint8

	widget.BaseWidget

	//xName, yName, zName string
	xValue, yValue               int
	xCorrFac, yCorrFac, zCorrFac float64

	xFrom, yFrom string

	numColumns, numRows, numData int
	xData, yData, zData          []int

	content   *fyne.Container
	innerView *fyne.Container
	grid      *Grid
	cursor    fyne.CanvasObject
	crosshair fyne.CanvasObject

	textValues []*canvas.Text

	xAxisButtons, yAxisButtons *fyne.Container
	xAxis, yAxis               []*canvas.Text

	ipf interpolate.InterPolFunc

	valueTexts *fyne.Container
	valueMap   *canvas.Image

	setChan chan xyUpdate

	xIdx, yIdx           float64
	selectedX, SelectedY int

	moving        bool
	selecting     bool
	selectedCells []int

	SetValueFunc func(name string, value float64)
}

func (mv *MapViewer) Refresh() {
	mv.valueMap.Image = createImage(mv.xData, mv.yData, mv.zData, mv.zCorrFac)
	mv.valueMap.Refresh()
}

func (mv *MapViewer) Close() {
	close(mv.setChan)
}

type MapViewerInfo struct {
	XName, YName, ZName string
	XLen, YLen, ZLen    int
	XFrom, YFrom        string
}

func (mv *MapViewer) Info() MapViewerInfo {
	return MapViewerInfo{
		XLen:  mv.numColumns,
		YLen:  mv.numRows,
		ZLen:  mv.numData,
		XFrom: mv.xFrom,
		YFrom: mv.yFrom,
	}
}

func (mv *MapViewer) SetValue(name string, value float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
		}
	}()
	//log.Printf("MapViewer: SetValue: %s: %f", name, value)
	if mv.SetValueFunc != nil {
		mv.SetValueFunc(name, value)
		return
	}
	if name == mv.xFrom || (mv.xFrom == "" && name == "MAF.m_AirInlet") {
		mv.xValue = int(value)
		mv.tik++
	}
	if name == mv.yFrom || (mv.yFrom == "" && name == "ActualIn.n_Engine") {
		mv.yValue = int(value)
		mv.tik++
	}
	if mv.tik >= 2 {
		update := xyUpdate{mv.xValue, mv.yValue}
		//log.Printf("MapViewer: SetValue: x: %d, y: %d", mv.xValue, mv.yValue)
		mv.setChan <- update
		mv.tik = 0
	}
}

func (mv *MapViewer) setXY(xValue, yValue int) error {
	//log.Println("Set", xValue, yValue)
	mv.xValue = xValue
	mv.yValue = yValue

	xIdx, yIdx, _, err := mv.ipf(mv.xData, mv.yData, mv.zData, xValue, yValue)
	if err != nil {
		return err
	}
	//_, xfrac := math.Modf(xIdx)
	//_, yfrac := math.Modf(yIdx)
	//if xfrac < 0.09 && yfrac < 0.09 && yValue > 1000 && xValue > 650 {
	//	log.Printf("Hit at Cell %f %f %f %f diff: %f", xIdx, yIdx, value, mv.mAir, value-mv.mAir)
	//}

	if yIdx < 0 {
		yIdx = 0
	} else if yIdx > float64(mv.numRows-1) {
		yIdx = float64(mv.numRows - 1)
	}
	if xIdx < 0 {
		xIdx = 0
	} else if xIdx > float64(mv.numColumns-1) {
		xIdx = float64(mv.numColumns - 1)
	}
	mv.xIdx = xIdx
	mv.yIdx = yIdx
	sz := mv.innerView.Size()

	mv.crosshair.Move(
		fyne.NewPos(
			float32(min(xIdx, float64(mv.numColumns)))*sz.Width/float32(mv.numColumns),
			float32(float64(mv.numRows-1)-yIdx)*sz.Height/float32(mv.numRows),
		),
	)
	return nil
}

func createImage(xData, yData []int, zData []int, correctionFactor float64) *image.RGBA {
	lenX := len(xData)
	lenY := len(yData)
	width := lenX * cellWidth
	height := lenY * cellHeight

	// Create a new RGBA image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	min, max := findMinMax(zData)
	// Calculate the colors for each cell based on data
	for y := 0; y < lenY; y++ {
		for x := 0; x < lenX; x++ {
			cellX := x * cellWidth
			cellY := (lenY - 1 - y) * cellHeight
			if cellY >= 0 && cellY+cellHeight <= height && cellX >= 0 && cellX+cellWidth <= width {
				index := y*lenX + x
				value := float64(zData[index]) * correctionFactor
				color := getColorInterpolation(float64(min)*correctionFactor, float64(max)*correctionFactor, value)
				draw.Draw(img, image.Rect(cellX, cellY, cellX+cellWidth, cellY+cellHeight), &image.Uniform{color}, image.Point{0, 0}, draw.Src)
			}
		}
	}
	return img
}

func createTextValues(zData []int, corrFac float64) ([]*canvas.Text, *fyne.Container) {
	var values []*canvas.Text
	valueContainer := container.NewWithoutLayout()
	prec := 0
	if corrFac < 1 {
		prec = 2
	}
	for _, v := range zData {
		text := &canvas.Text{
			Text:      strconv.FormatFloat(float64(v)*corrFac, 'f', prec, 64),
			TextSize:  13,
			Color:     color.Black,
			TextStyle: fyne.TextStyle{Monospace: false},
		}
		text.SetMinSize(fyne.NewSize(cellWidth, cellHeight))
		values = append(values, text)
		valueContainer.Add(text)
	}
	return values, valueContainer
}

func findMinMax(data []int) (int, int) {
	var min, max int = data[0], data[0]
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

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// getColorInterpolation returns a color interpolated on the color spectrum green to yellow to red.
// value should be between min and max.
func getColorInterpolation(min, max, value float64) color.RGBA {
	// Normalize the value to a 0-1 range
	t := (value - min) / (max - min)

	divider := .5

	var r, g, b float64
	if t < divider { // Green to Yellow interpolation
		r = lerp(0, 1, t/divider)
		g = 1
	} else { // Yellow to Red interpolation
		r = 1
		g = lerp(1, 0, (t-divider)/(1-divider))
	}
	b = 0

	// Convert from 0-1 range to 0-255 for color.RGBA
	return color.RGBA{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
		A: 255,
	}
}
