package widgets

import (
	"fmt"
	"image/color"
	"log"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/symbol"
)

type xyUpdate struct {
	x, y int
}

type MapViewerInfo struct {
	XName, YName, ZName string
	XLen, YLen, ZLen    int
	XFrom, YFrom        string
}

type MapViewer struct {
	widget.BaseWidget

	focused    bool
	updateFunc UpdateFunc
	loadFunc   LoadFunc
	saveFunc   SaveFunc

	tik uint8

	symbol *symbol.Symbol

	xData, yData, zData          []int
	xValue, yValue               int
	xCorrFac, yCorrFac, zCorrFac float64
	xFrom, yFrom                 string
	numColumns, numRows, numData int

	min, max int

	xAxisLabels, yAxisLabels *fyne.Container
	xAxisTexts, yAxisTexts   []*canvas.Text
	zDataRects               []*canvas.Rectangle

	content    *fyne.Container
	innerView  *fyne.Container
	grid       *Grid
	cursor     *canvas.Rectangle
	crosshair  *canvas.Rectangle
	textValues []*canvas.Text

	ipf interpolate.InterPolFunc

	valueTexts *fyne.Container

	setChan chan xyUpdate

	xIdx, yIdx           float64
	selectedX, SelectedY int

	// Mouse
	selecting     bool
	selectedCells []int

	// Keyboard
	inputBuffer   strings.Builder
	restoreValues bool

	mousePos fyne.Position
	popup    *widget.PopUpMenu
}

func NewMapViewer(options ...MapViewerOption) (*MapViewer, error) {
	mv := &MapViewer{
		yAxisTexts: make([]*canvas.Text, 0),
		xAxisTexts: make([]*canvas.Text, 0),
		zDataRects: make([]*canvas.Rectangle, 0),
		setChan:    make(chan xyUpdate, 10),
	}
	mv.ExtendBaseWidget(mv)
	for _, option := range options {
		if err := option(mv); err != nil {
			return nil, err
		}
	}

	mv.min, mv.max = findMinMax(mv.zData)

	log.Printf("NewMapViewer: mv.numColumns: %d mv.numRows: %d mv.numData: %d xf: %s  yf: %s", mv.numColumns, mv.numRows, mv.numData, mv.xFrom, mv.yFrom)
	mv.content = mv.render()

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

func (mv *MapViewer) SetCellText(idx int, value int) {
	prec := 0
	if mv.zCorrFac < 1 {
		prec = 2
	}
	textValue := strconv.FormatFloat(float64(value)*mv.zCorrFac, 'f', prec, 64)
	if mv.textValues[idx].Text != textValue {
		mv.textValues[idx].Text = textValue
		mv.textValues[idx].Refresh()
	}
}

func (mv *MapViewer) SetZ(zData []int) {
	mv.zData = zData
	mv.numData = len(zData)
	mv.Refresh()
}

func (mv *MapViewer) Refresh() {
	mv.min, mv.max = findMinMax(mv.zData)
	for i, tv := range mv.zData {
		mv.SetCellText(i, tv)
	}

	for idx, r := range mv.zDataRects {
		r.FillColor = GetColorInterpolation(float64(mv.min)*mv.zCorrFac, float64(mv.max)*mv.zCorrFac, float64(mv.zData[idx])*mv.zCorrFac)
		r.Refresh()
	}

	//mv.valueMap.Image = createImage(mv.xData, mv.yData, mv.zData, mv.zCorrFac)
	//mv.valueMap.Refresh()
}

func (mv *MapViewer) Close() {
	close(mv.setChan)
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
		values = append(values, text)
		valueContainer.Add(text)
	}
	return values, valueContainer
}

func (mv *MapViewer) render() *fyne.Container {

	// y must be created before x as it's width is used to calculate x's offset
	mv.createYAxis()
	mv.createXAxis()
	mv.createZdata()

	mv.crosshair = NewRectangle(color.RGBA{0xfc, 0x4a, 0xFA, 235}, 4)

	mv.cursor = NewRectangle(color.RGBA{0x00, 0x0a, 0xFF, 235}, 4)
	mv.selectedX = -1
	mv.cursor.Resize(fyne.NewSize(1, 1))
	mv.textValues, mv.valueTexts = createTextValues(mv.zData, mv.zCorrFac)

	mv.grid = NewGrid(mv.numColumns, mv.numRows)

	valueRects := container.NewWithoutLayout()
	for _, r := range mv.zDataRects {
		valueRects.Add(r)
	}

	mv.innerView = container.NewStack(
		valueRects,
		container.NewWithoutLayout(
			mv.crosshair,
			mv.cursor,
		),
		mv.grid,
		mv.valueTexts,
	)

	if mv.symbol == nil {
		return container.NewBorder(
			mv.xAxisLabels,
			nil,
			mv.yAxisLabels,
			nil,
			mv.innerView,
		)
	}

	return container.NewBorder(
		mv.xAxisLabels,
		container.NewGridWithColumns(3,
			widget.NewButtonWithIcon("Load from File", theme.DocumentIcon(), func() {
				if mv.symbol != nil {
					mv.zData = mv.symbol.Ints()
					mv.Refresh()
				}
			}),
			widget.NewButtonWithIcon("Load from ECU", theme.DocumentIcon(), func() {
				if mv.loadFunc != nil {
					mv.loadFunc()
				}
			}),
			//widget.NewButtonWithIcon("Save to File", theme.DocumentSaveIcon(), func() {
			//}),
			widget.NewButtonWithIcon("Save to ECU", theme.DocumentSaveIcon(), func() {
				if mv.saveFunc != nil {
					mv.saveFunc(mv.zData)
				}
			}),
		),
		mv.yAxisLabels,
		nil,
		mv.innerView,
	)
}

func (mv *MapViewer) createXAxis() {
	mv.xAxisLabels = container.New(&layout.Horizontal{Offset: mv.yAxisLabels})
	if mv.numColumns >= 1 {
		prec := 0
		if mv.xCorrFac < 1 {
			prec = 2
		}
		for i := 0; i < mv.numColumns; i++ {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.xData[i])*mv.xCorrFac, 'f', prec, 64), TextSize: 13}
			mv.xAxisTexts = append(mv.xAxisTexts, text)
			mv.xAxisLabels.Add(text)
		}
	}
}

func (mv *MapViewer) createYAxis() {
	mv.yAxisLabels = container.New(&layout.Vertical{})
	if mv.numRows >= 1 {
		prec := 0
		if mv.yCorrFac < 1 {
			prec = 2
		}
		for i := mv.numRows - 1; i >= 0; i-- {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.yData[i])*mv.yCorrFac, 'f', prec, 64), TextSize: 13}
			mv.yAxisTexts = append(mv.yAxisTexts, text)
			mv.yAxisLabels.Add(text)
		}
	}
}

func (mv *MapViewer) createZdata() {
	minCorrected := float64(mv.min) * mv.zCorrFac
	maxCorrected := float64(mv.max) * mv.zCorrFac
	// Calculate the colors for each cell based on data
	for y := 0; y < mv.numRows; y++ {
		for x := 0; x < mv.numColumns; x++ {
			index := y*mv.numColumns + x
			value := float64(mv.zData[index]) * mv.zCorrFac
			color := GetColorInterpolation(minCorrected, maxCorrected, value)
			rect := &canvas.Rectangle{FillColor: color}
			mv.zDataRects = append(mv.zDataRects, rect)
		}
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

	fyne.NewAnimation(10*time.Millisecond, func(p float32) {
		//mv.cursor.Resize(fyne.NewSize(1, 1))
		mv.crosshair.Move(
			fyne.NewPos(
				p*float32(min(xIdx, float64(mv.numColumns)))*sz.Width/float32(mv.numColumns),
				p*float32(float64(mv.numRows-1)-yIdx)*sz.Height/float32(mv.numRows),
			),
		)
	}).Start()
	return nil
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
func GetColorInterpolation(min, max, value float64) color.RGBA {
	//log.Println("getColorInterpolation", min, max, value)
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
