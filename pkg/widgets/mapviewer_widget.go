package widgets

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
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

	xIdx, yIdx float64

	numColumns, numRows, numData int
	xData, yData, zData          []int

	content   *fyne.Container
	innerView *fyne.Container
	cursor    fyne.CanvasObject
	crosshair fyne.CanvasObject

	textValues []*canvas.Text

	xAxisButtons, yAxisButtons *fyne.Container
	xAxis, yAxis               []*canvas.Text

	ipf interpolate.InterPolFunc

	valueTexts *fyne.Container
	valueMap   *canvas.Image

	setChan chan xyUpdate

	curX, curY int

	moving        bool
	selecting     bool
	selectedCells []int

	SetValueFunc func(name string, value float64)
}

func NewMapViewer(xData, yData, zData []int, xCorrFac, yCorrFac, zCorrFac float64, interPolfunc interpolate.InterPolFunc) (*MapViewer, error) {
	xLen := len(xData)
	yLen := len(yData)
	zLen := len(zData)

	if xLen*yLen != zLen && xLen > 1 && yLen > 1 {
		return nil, fmt.Errorf("NewMapViewer xLen * yLen != zLen")
	}

	var yAxis []*canvas.Text
	yAxisButtons := container.New(&layout.Vertical{})
	if yLen > 1 {
		prec := 0
		if xCorrFac < 1 {
			prec = 2

		}
		for i := yLen - 1; i >= 0; i-- {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(yData[i])*yCorrFac, 'f', prec, 64), TextSize: 13}
			yAxis = append(yAxis, text)
			yAxisButtons.Add(text)
		}
	}

	var xAxis []*canvas.Text
	xAxisButtons := container.New(&layout.Horizontal{
		Offset: yAxisButtons,
	})
	if xLen > 1 {
		prec := 0
		if xCorrFac < 1 {
			prec = 2
		}
		for i := 0; i < xLen; i++ {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(xData[i])*xCorrFac, 'f', prec, 64), TextSize: 13}
			xAxis = append(xAxis, text)
			xAxisButtons.Add(text)
		}
	}

	crosshair := NewRectangle(color.RGBA{0xfc, 0x4a, 0xFA, 255}, 4)
	cursor := NewRectangle(color.RGBA{0x00, 0x0a, 0xFF, 255}, 4)

	textValues, valueTexts := createTextValues(zData, zCorrFac)

	width := float32(xLen * cellWidth)
	height := float32(yLen * cellHeight)
	valueMap := canvas.NewImageFromImage(createImage(xData, yData, zData, zCorrFac))
	valueMap.ScaleMode = canvas.ImageScalePixels
	valueMap.SetMinSize(fyne.NewSize(width, height))
	valueMap.Resize(fyne.NewSize(width, height))

	grid := NewGrid(xLen, yLen)

	inner := container.NewStack(
		valueMap,
		grid,
		valueTexts,
		container.NewWithoutLayout(
			crosshair,
			cursor,
		),
	)

	content := container.NewBorder(
		xAxisButtons,
		container.NewGridWithColumns(2,
			widget.NewButtonWithIcon("Load", theme.DocumentIcon(), func() {
				log.Println("Load")
			}),
			widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
				log.Println("Save")
			}),
		),
		yAxisButtons,
		nil,
		inner,
	)

	mv := &MapViewer{
		ipf: interPolfunc,

		content:   content,
		innerView: inner,
		cursor:    cursor,
		crosshair: crosshair,

		xAxisButtons: xAxisButtons,
		yAxisButtons: yAxisButtons,
		xAxis:        xAxis,
		yAxis:        yAxis,

		numColumns: xLen,
		numRows:    yLen,
		numData:    zLen,

		xData: xData,
		yData: yData,
		zData: zData,

		valueTexts: valueTexts,
		valueMap:   valueMap,

		xCorrFac: xCorrFac,
		yCorrFac: yCorrFac,
		zCorrFac: zCorrFac,

		textValues: textValues,

		setChan: make(chan xyUpdate, 100),
	}
	mv.ExtendBaseWidget(mv)

	go func() {
		for xy := range mv.setChan {
			mv.setXY(xy.x, xy.y)
		}
	}()

	return mv, nil
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
	if mv.tik == 2 {
		mv.setChan <- xyUpdate{mv.xValue, mv.yValue}
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
