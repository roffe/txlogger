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

	content    fyne.CanvasObject
	innerView  *fyne.Container
	grid       *Grid
	cursor     *canvas.Rectangle
	crosshair  *canvas.Rectangle
	textValues []*canvas.Text

	ipf interpolate.InterPolFunc

	valueRects *fyne.Container
	valueTexts *fyne.Container

	setChan chan xyUpdate

	xIdx, yIdx           float64
	selectedX, SelectedY int

	meshView bool
	mesh     *Meshgrid

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
		setChan: make(chan xyUpdate, 10),
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

func (mv *MapViewer) render() fyne.CanvasObject {

	// y must be created before x as it's width is used to calculate x's offset
	mv.yAxisLabels = mv.createYAxis()
	mv.xAxisLabels = mv.createXAxis()
	mv.zDataRects = mv.createZdata()

	mv.crosshair = NewRectangle(color.RGBA{0xfc, 0x4a, 0xFA, 235}, 4)

	mv.cursor = NewRectangle(color.RGBA{0x00, 0x0a, 0xFF, 235}, 4)
	mv.selectedX = -1
	mv.cursor.Resize(fyne.NewSize(1, 1))

	mv.textValues = createTextValues(mv.zData, mv.zCorrFac)

	mv.valueTexts = container.New(&layout.Grid{Cols: mv.numColumns, Rows: mv.numRows, Text: true})
	for _, t := range mv.textValues {
		mv.valueTexts.Add(t)
	}

	mv.valueRects = container.New(&layout.Grid{Cols: mv.numColumns, Rows: mv.numRows})
	for _, r := range mv.zDataRects {
		mv.valueRects.Add(r)
	}

	mv.grid = NewGrid(mv.numColumns, mv.numRows)

	mv.innerView = container.NewStack(
		mv.valueRects,
		container.NewWithoutLayout(
			mv.crosshair,
			mv.cursor,
		),
		mv.grid,
		mv.valueTexts,
	)

	if mv.symbol == nil || mv.numColumns == 1 && mv.numRows == 1 {
		return container.NewBorder(
			mv.xAxisLabels,
			nil,
			mv.yAxisLabels,
			nil,
			mv.innerView,
		)
	}

	fixed := container.NewBorder(
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

	if mv.meshView {
		mv.mesh = NewMeshgrid(mv.symbol.Float64s(), mv.numColumns, mv.numRows)
		return container.NewBorder(
			fixed,
			nil,
			nil,
			nil,
			mv.mesh,
		)
	}

	return fixed

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
	precission := 0
	switch mv.zCorrFac {
	case 0.1:
		precission = 1
	case 0.01:
		precission = 2
	case 0.001:
		precission = 3
	}
	textValue := strconv.FormatFloat(float64(value)*mv.zCorrFac, 'f', precission, 64)
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

func (mv *MapViewer) resize(size fyne.Size) {
	mv.content.Resize(size)
	sz := mv.innerView.Size()

	// Calculate shared factors
	numColumnsFloat := float32(mv.numColumns)
	numRowsFloat := float32(mv.numRows)
	widthFactor := sz.Width / numColumnsFloat
	heightFactor := sz.Height / numRowsFloat

	// Position and resize crosshair
	mv.crosshair.Resize(fyne.NewSize(widthFactor, heightFactor))
	mv.crosshair.Move(
		fyne.NewPos(
			float32(mv.xIdx)*widthFactor,
			float32(float64(mv.numRows)-1-mv.yIdx)*heightFactor,
		),
	)

	// Calculate text size
	textSize := calculateOptimalTextSize(sz.Width, sz.Height, mv.numColumns, mv.numRows)

	// Position and resize text values
	for i := mv.numRows; i > 0; i-- {
		for j := 0; j < mv.numColumns; j++ {
			iDx := (i * mv.numColumns) - (mv.numColumns - j)
			t := mv.textValues[iDx]
			t.TextSize = textSize
			// t.Resize(t.MinSize())
			// t.Move(fyne.NewPos(
			// 	(float32(j)*widthFactor)+(widthFactor/2)-(t.MinSize().Width/2),
			// 	(sz.Height-float32(i)*heightFactor)+(heightFactor/2)-(t.MinSize().Height/2),
			// ))
			// mv.zDataRects[iDx].Resize(fyne.NewSize(widthFactor, heightFactor))
			// mv.zDataRects[iDx].Move(fyne.NewPos(
			// 	float32(j)*widthFactor,
			// 	(sz.Height - float32(i)*heightFactor),
			// ))
		}
	}

	// Update x and y axes
	for _, xb := range mv.xAxisTexts {
		xb.TextSize = textSize
		//xb.Resize(xb.MinSize())
		xb.Refresh()
	}
	for _, yb := range mv.yAxisTexts {
		yb.TextSize = textSize
		//yb.Resize(yb.MinSize())
		yb.Refresh()
	}

	mv.resizeCursor(sz, numColumnsFloat, numRowsFloat, widthFactor, heightFactor)
}

func calculateOptimalTextSize(width, height float32, columns, rows int) float32 {
	cellWidth := float64(width / float32(columns))
	return max(min(float32(cellWidth/5), 21), 12)
}

func (mv *MapViewer) resizeCursor(viewportSize fyne.Size, numColumnsFloat, numRowsFloat, widthFactor, heightFactor float32) {
	// Position and resize cursor
	if mv.selectedX >= 0 {
		if len(mv.selectedCells) > 1 {
			mv := mv
			// if multiple cells are selected, we need to calculate the bounding box
			// of the selection area
			minX := mv.numColumns
			maxX := 0
			minY := mv.numRows
			maxY := 0
			for _, cell := range mv.selectedCells {
				x := cell % mv.numColumns
				y := cell / mv.numColumns
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
			widthFactor := viewportSize.Width / numColumnsFloat
			heightFactor := viewportSize.Height / numRowsFloat
			topLeftX := float32(minX) * widthFactor
			topLeftY := float32(mv.numRows-1-maxY) * heightFactor
			width := float32(maxX-minX+1) * widthFactor
			height := float32(maxY-minY+1) * heightFactor
			mv.cursor.Resize(fyne.NewSize(width, height))
			mv.cursor.Move(fyne.NewPos(topLeftX, topLeftY))

		} else {
			mv.cursor.Resize(fyne.NewSize(widthFactor, heightFactor))
			mv.cursor.Move(
				fyne.NewPos(
					float32(mv.selectedX)*widthFactor,
					float32(mv.numRows-1-mv.SelectedY)*heightFactor,
				),
			)
		}
	}
}

func (mv *MapViewer) Refresh() {
	mv.min, mv.max = findMinMax(mv.zData)
	for i, tv := range mv.zData {
		mv.SetCellText(i, tv)
	}
	for idx, r := range mv.zDataRects {
		col := GetColorInterpolation(float64(mv.min)*mv.zCorrFac, float64(mv.max)*mv.zCorrFac, float64(mv.zData[idx])*mv.zCorrFac)
		if col != r.FillColor {
			r.FillColor = col
			r.Refresh()
		}
	}

	if mv.mesh != nil {
		mv.mesh.SetMin(float64(mv.min) * mv.zCorrFac)
		mv.mesh.SetMax(float64(mv.max) * mv.zCorrFac)
		var values []float64
		for _, v := range mv.zData {
			values = append(values, float64(v)*mv.zCorrFac)
		}
		mv.mesh.LoadFloat64s(values)
	}
}

func (mv *MapViewer) Close() {
	close(mv.setChan)
}

func getPrecission(corrFac float64) int {
	precission := 0
	switch corrFac {
	case 0.1:
		precission = 1
	case 0.01:
		precission = 2
	case 0.001:
		precission = 3
	case 1.0 / 128:
		precission = 3
	}
	return precission
}

func createTextValues(zData []int, corrFac float64) []*canvas.Text {
	var values []*canvas.Text
	for _, v := range zData {
		text := &canvas.Text{
			Text:      strconv.FormatFloat(float64(v)*corrFac, 'f', getPrecission(corrFac), 64),
			TextSize:  13,
			Color:     color.Black,
			TextStyle: fyne.TextStyle{Monospace: false},
		}
		values = append(values, text)
	}
	return values
}

func (mv *MapViewer) createXAxis() *fyne.Container {
	labels := container.New(&layout.Horizontal{Offset: mv.yAxisLabels})
	if mv.numColumns >= 1 {

		for i := 0; i < mv.numColumns; i++ {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.xData[i])*mv.xCorrFac, 'f', getPrecission(mv.xCorrFac), 64), TextSize: 13}
			mv.xAxisTexts = append(mv.xAxisTexts, text)
			labels.Add(text)
		}
	}
	return labels
}

func (mv *MapViewer) createYAxis() *fyne.Container {
	labels := container.New(&layout.Vertical{})
	if mv.numRows >= 1 {
		for i := mv.numRows - 1; i >= 0; i-- {
			text := &canvas.Text{Alignment: fyne.TextAlignCenter, Text: strconv.FormatFloat(float64(mv.yData[i])*mv.yCorrFac, 'f', getPrecission(mv.yCorrFac), 64), TextSize: 13}
			mv.yAxisTexts = append(mv.yAxisTexts, text)
			labels.Add(text)
		}
	}
	return labels
}

func (mv *MapViewer) createZdata() []*canvas.Rectangle {
	var rects []*canvas.Rectangle
	minCorrected := float64(mv.min) * mv.zCorrFac
	maxCorrected := float64(mv.max) * mv.zCorrFac
	// Calculate the colors for each cell based on data
	for y := 0; y < mv.numRows; y++ {
		for x := 0; x < mv.numColumns; x++ {
			index := y*mv.numColumns + x
			value := float64(mv.zData[index]) * mv.zCorrFac
			color := GetColorInterpolation(minCorrected, maxCorrected, value)
			rect := &canvas.Rectangle{FillColor: color}
			rects = append(rects, rect)
		}
	}
	return rects
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
