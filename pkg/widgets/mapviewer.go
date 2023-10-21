package widgets

import (
	"bytes"
	"encoding/binary"
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
	"github.com/roffe/txlogger/pkg/symbol"
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

	xName, yName, zName string
	xValue, yValue      int
	corrFac             float64

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

	ipf interpolate.InterPolFunc

	valueContainer *fyne.Container
	valueMap       *canvas.Image

	setChan chan xyUpdate

	W fyne.Window

	curX, curY int

	moving bool

	SetValueFunc func(name string, value float64)
}

func NewMapViewer(w fyne.Window, axis symbol.Axis, symbols symbol.SymbolCollection, interPolfunc interpolate.InterPolFunc) (*MapViewer, error) {
	xData, yData, zData, corrFac, err := getXYZ(axis.X, axis.Y, axis.Z, symbols)
	if err != nil {
		return nil, err
	}

	xLen := len(xData)
	yLen := len(yData)
	zLen := len(zData)

	if xLen*yLen != zLen && xLen > 1 && yLen > 1 {
		return nil, fmt.Errorf("xLen * yLen != zLen")
	}

	xAxisButtons := container.New(&layout.Horizontal{})
	if xLen > 1 {
		for i := 0; i < xLen; i++ {
			text := &canvas.Text{Text: fmt.Sprintf("%d", xData[i]), TextSize: 13}
			xAxisButtons.Add(text)
		}
	}

	yAxisButtons := container.New(&layout.Vertical{})
	if yLen > 1 {
		for i := yLen - 1; i >= 0; i-- {
			text := &canvas.Text{Text: fmt.Sprintf("%d", yData[i]), TextSize: 13}
			yAxisButtons.Add(text)
		}
	}

	crosshair := NewRectangle(color.RGBA{0xfc, 0x4a, 0xFA, 255}, 4)
	cursor := NewRectangle(color.RGBA{0x00, 0x0a, 0xFF, 255}, 4)

	textValues, valueContainer := createTextValues(zData, corrFac)

	width := float32(xLen * cellWidth)
	height := float32(yLen * cellHeight)
	valueMap := canvas.NewImageFromImage(createImage(xData, yData, zData, corrFac))
	valueMap.ScaleMode = canvas.ImageScalePixels
	valueMap.SetMinSize(fyne.NewSize(width, height))
	valueMap.Resize(fyne.NewSize(width, height))

	inner := container.NewStack(
		valueMap,
		valueContainer,
		container.NewWithoutLayout(
			cursor,
			crosshair,
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
		W:   w,
		ipf: interPolfunc,

		content:   content,
		innerView: inner,
		cursor:    cursor,
		crosshair: crosshair,

		xAxisButtons: xAxisButtons,
		yAxisButtons: yAxisButtons,

		numColumns: xLen,
		numRows:    yLen,
		numData:    zLen,

		xName: axis.X,
		yName: axis.Y,
		zName: axis.Z,

		xFrom: axis.XFrom,
		yFrom: axis.YFrom,

		xData: xData,
		yData: yData,
		zData: zData,

		valueContainer: valueContainer,
		valueMap:       valueMap,
		corrFac:        corrFac,

		textValues: textValues,

		setChan: make(chan xyUpdate, 100),
	}
	mv.ExtendBaseWidget(mv)

	go func() {
		for xy := range mv.setChan {
			mv.setXY(xy.x, xy.y)
		}
	}()

	if w != nil {
		w.Canvas().SetOnTypedKey(mv.TypedKey)
		w.SetTitle(axis.Z)
	}

	return mv, nil
}

func (mv *MapViewer) Refresh() {
	mv.valueMap.Image = createImage(mv.xData, mv.yData, mv.zData, mv.corrFac)
	mv.valueMap.Refresh()
}

func (mv *MapViewer) Close() {
	close(mv.setChan)
	if mv.W != nil {
		mv.W.Close()
	}
}

type MapViewerInfo struct {
	XName, YName, ZName string
	XLen, YLen, ZLen    int
	XFrom, YFrom        string
}

func (mv *MapViewer) Info() MapViewerInfo {
	return MapViewerInfo{
		XName: mv.xName,
		YName: mv.yName,
		ZName: mv.zName,
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

func getXYZ(xAxis, yAxis, zAxis string, symbols symbol.SymbolCollection) ([]int, []int, []int, float64, error) {
	symx, symy, symz := symbols.GetByName(xAxis), symbols.GetByName(yAxis), symbols.GetByName(zAxis)

	var xOut, yOut []int
	if xAxis == "none" {
		xOut = []int{0}
	} else if xAxis != "" && symx != nil {
		xOut = symx.IntFromData()
	}
	if yAxis == "none" {
		yOut = []int{0}
	} else if yAxis != "" && symy != nil {
		yOut = symy.IntFromData()
	}
	if xAxis == "none" || yAxis == "none" {
		return xOut, yOut, symz.IntFromData(), symz.Correctionfactor, nil
	}
	asd := map[string]*symbol.Symbol{
		xAxis: symx,
		yAxis: symy,
		zAxis: symz,
	}
	for k, v := range asd {
		if v == nil {
			return nil, nil, nil, 0, fmt.Errorf("failed to find %s", k)
		}
	}

	var x, y, z []int
	if symx.Type&symbol.SIGNED == 1 {
		x = symx.DataToInt16()
	} else {
		x = symx.DataToUint16()
	}
	if symy.Type&symbol.SIGNED == 1 {
		y = symy.DataToInt16()
	} else {
		y = symy.DataToUint16()
	}

	if len(x)*len(y) == len(symz.Bytes()) {
		if symz.Type&symbol.SIGNED == 1 {
			for _, v := range symz.DataToInt8() {
				z = append(z, int(v))
			}
		} else {
			for _, v := range symz.DataToUint8() {
				z = append(z, int(v))
			}
		}
		return x, y, z, symz.Correctionfactor, nil
	}

	if len(x)*len(y) == int(symz.Length/2) {
		data := make([]int16, symz.Length/2)
		reader := bytes.NewReader(symz.Bytes())
		if err := binary.Read(reader, binary.BigEndian, &data); err != nil {
			log.Fatalf("Failed to convert zData to int16 slice: %v", err)
		}
		for _, v := range data {
			z = append(z, int(v))
		}
		return x, y, z, symz.Correctionfactor, nil
	}

	return nil, nil, nil, 0, fmt.Errorf("failed to convert %s %s %s", xAxis, yAxis, zAxis)
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

	for x := 1; x < lenX; x++ {
		cellX := x * cellWidth
		draw.Draw(img, image.Rect(cellX-1, 0, cellX+1, height), &image.Uniform{color.Black}, image.Point{}, draw.Src)
	}

	for y := 1; y < lenY; y++ {
		cellY := y * cellHeight
		draw.Draw(img, image.Rect(0, cellY-1, width, cellY+1), &image.Uniform{color.Black}, image.Point{}, draw.Src)
	}

	return img
}

func createTextValues(zData []int, corrFac float64) ([]*canvas.Text, *fyne.Container) {
	var values []*canvas.Text
	valueContainer := container.NewStack()
	prec := 0
	if corrFac != 1 {
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
