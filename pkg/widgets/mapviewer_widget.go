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
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
)

const (
	cellWidth  = 40
	cellHeight = 23
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
	cursor    *canvas.Rectangle
	crosshair *canvas.Rectangle

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

	focused bool
}

func NewMapViewer(options ...MapViewerOption) (*MapViewer, error) {
	mv := &MapViewer{
		yAxis:   make([]*canvas.Text, 0),
		xAxis:   make([]*canvas.Text, 0),
		setChan: make(chan xyUpdate, 10),
	}
	mv.ExtendBaseWidget(mv)
	for _, option := range options {
		if err := option(mv); err != nil {
			return nil, err
		}
	}

	log.Printf("NewMapViewer: mv.numColumns: %d mv.numRows: %d mv.numData: %d xf: %s  yf: %s", mv.numColumns, mv.numRows, mv.numData, mv.xFrom, mv.yFrom)
	mv.render()

	if mv.numColumns*mv.numRows != mv.numData && mv.numColumns > 1 && mv.numRows > 1 {
		return nil, fmt.Errorf("NewMapViewer mv.numColumns * mv.numRows != mv.numData")
	}

	go func() {
		for xy := range mv.setChan {
			mv.setXY(xy.x, xy.y)
		}
	}()

	return mv, nil
}

func (mv *MapViewer) render() {
	// y must be created before x as it's width is used to calculate x's offset
	mv.createYAxis()
	mv.createXAxis()

	mv.crosshair = NewRectangle(color.RGBA{0xfc, 0x4a, 0xFA, 235}, 4)

	mv.cursor = NewRectangle(color.RGBA{0x00, 0x0a, 0xFF, 235}, 4)
	mv.selectedX = -1
	mv.cursor.Resize(fyne.NewSize(1, 1))
	mv.textValues, mv.valueTexts = createTextValues(mv.zData, mv.zCorrFac)

	width := float32(mv.numColumns * cellWidth)
	height := float32(mv.numRows * cellHeight)

	mv.valueMap = canvas.NewImageFromImage(createImage(mv.xData, mv.yData, mv.zData, mv.zCorrFac))
	mv.valueMap.ScaleMode = canvas.ImageScalePixels
	mv.valueMap.SetMinSize(fyne.NewSize(width, height))
	//mv.valueMap.Resize(fyne.NewSize(width, height))

	mv.grid = NewGrid(mv.numColumns, mv.numRows)

	mv.innerView = container.NewStack(
		mv.valueMap,
		container.NewWithoutLayout(
			mv.crosshair,
			mv.cursor,
		),
		mv.grid,
		mv.valueTexts,
	)

	mv.content = container.NewBorder(
		mv.xAxisButtons,
		nil,
		//container.NewGridWithColumns(2,
		//	widget.NewButtonWithIcon("Load", theme.DocumentIcon(), func() {
		//		log.Println("Load")
		//	}),
		//	widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		//		log.Println("Save")
		//	}),
		//),
		mv.yAxisButtons,
		nil,
		mv.innerView,
	)

}

func (mv *MapViewer) createXAxis() {
	mv.xAxisButtons = container.New(&layout.Horizontal{Offset: mv.yAxisButtons})
	if mv.numColumns > 1 {
		prec := 0
		if mv.xCorrFac < 1 {
			prec = 2
		}
		for i := 0; i < mv.numColumns; i++ {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.xData[i])*mv.xCorrFac, 'f', prec, 64), TextSize: 13}
			mv.xAxis = append(mv.xAxis, text)
			mv.xAxisButtons.Add(text)
		}
	}
}

func (mv *MapViewer) createYAxis() {
	mv.yAxisButtons = container.New(&layout.Vertical{})
	if mv.numRows > 1 {
		prec := 0
		if mv.yCorrFac < 1 {
			prec = 2
		}
		for i := mv.numRows - 1; i >= 0; i-- {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.yData[i])*mv.yCorrFac, 'f', prec, 64), TextSize: 13}
			mv.yAxis = append(mv.yAxis, text)
			mv.yAxisButtons.Add(text)
		}
	}
}

func (mv *MapViewer) Refresh() {
	log.Println("MapViewer: Refresh")
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
		select {
		case mv.setChan <- update:
		default:
			log.Println("MapViewer: setChan full")

		}
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
		//text.SetMinSize(fyne.NewSize(cellWidth, cellHeight))
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
