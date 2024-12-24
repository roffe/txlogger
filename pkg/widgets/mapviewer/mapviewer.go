package mapviewer

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"strconv"
	"strings"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/grid"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

const (
	minTextSize = 10
	maxTextSize = 28
)

type MapViewerInfo struct {
	XName, YName, ZName string
	XLen, YLen, ZLen    int
	XFrom, YFrom        string
}

type MapViewer struct {
	widget.BaseWidget

	focused bool

	funcs *funcs
	opts  *opts

	symbol *symbol.Symbol

	xData, yData, zData []int

	xValue, yValue int

	xCorrFac, yCorrFac, zCorrFac          float64
	xCorrOffset, yCorrOffset, zCorrOffset float64
	xIndex, yIndex                        float64

	xFrom, yFrom                 string
	numColumns, numRows, numData int

	min, max int

	xAxisLabel, yAxisLabel, zAxisLabel       string
	xAxisLabelContainer, yAxisLabelContainer *fyne.Container
	xAxisTexts, yAxisTexts                   []*canvas.Text
	textValues                               []*canvas.Text

	content fyne.CanvasObject

	innerView  *fyne.Container
	valueRects *fyne.Container
	valueTexts *fyne.Container

	grid *grid.Grid

	selectionRect *canvas.Rectangle
	crosshair     *canvas.Rectangle
	zDataRects    []*canvas.Rectangle

	ipf interpolate.InterPolFunc

	selectedX, SelectedY int

	mesh *meshgrid.Meshgrid

	// Mouse
	mousePos      fyne.Position
	selecting     bool
	selectedCells []int

	// Keyboard
	inputBuffer   strings.Builder
	restoreValues bool

	popup *widget.PopUpMenu

	lamb       *cbar.CBar
	lambdaName string

	widthFactor  float32
	heightFactor float32

	OnMouseDown func()
}

type opts struct {
	editable              bool
	buttonsEnabled        bool
	showWBL               bool
	cursorFollowCrosshair bool
	meshView              bool
}

type funcs struct {
	saveFileFunc  SaveFunc
	updateECUFunc UpdateFunc
	loadECUFunc   LoadFunc
	saveECUFunc   SaveFunc
}

func New(options ...MapViewerOption) (*MapViewer, error) {
	mv := &MapViewer{
		opts: &opts{
			editable: true,
		},
		funcs: &funcs{

			loadECUFunc:   func() {},
			saveECUFunc:   func(data []int) {},
			updateECUFunc: func(idx int, value []int) {},
		},
	}
	mv.ExtendBaseWidget(mv)

	for _, option := range options {
		if err := option(mv); err != nil {
			return nil, err
		}
	}

	if len(mv.zData) == 0 {
		return nil, fmt.Errorf("MapViewer zData is empty")
	}
	mv.min, mv.max = findMinMax(mv.zData)

	if mv.numColumns*mv.numRows != mv.numData && mv.numColumns > 1 && mv.numRows > 1 {
		return nil, fmt.Errorf("MapViewer columns * rows != datalen")
	}

	mv.content = mv.render()

	log.Printf("MapViewer c: %d r: %d dlen: %d x: %s y: %s z: %s", mv.numColumns, mv.numRows, mv.numData, mv.xFrom, mv.yFrom, mv.symbol.Name)
	return mv, nil
}

func (mv *MapViewer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(mv.content)
	//return &mapViewerRenderer{mv: mv}
}

var _ fyne.WidgetRenderer = (*mapViewerRenderer)(nil)

type mapViewerRenderer struct {
	mv      *MapViewer
	oldSize fyne.Size
}

func (r *mapViewerRenderer) Layout(size fyne.Size) {
	if size == r.oldSize {
		return
	}
	r.oldSize = size
	r.mv.content.Resize(size)
}

func (r *mapViewerRenderer) MinSize() fyne.Size {
	return r.mv.content.MinSize()
}

func (r *mapViewerRenderer) Refresh() {
}

func (r *mapViewerRenderer) Destroy() {
}

func (r *mapViewerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.mv.content}
}

func calculateTextSize(widthFactor, heightFactor float32) float32 {
	cellSize := fyne.Min(widthFactor, heightFactor)

	// Scale text size relative to cell size, but with a more conservative ratio
	// Reduced from 0.6 to 0.4 to prevent overflow
	baseTextSize := float64(cellSize * 0.5)

	if baseTextSize < minTextSize {
		return minTextSize
	}
	if baseTextSize > maxTextSize {
		return maxTextSize
	}
	return float32(math.Ceil(baseTextSize))
}

func (mv *MapViewer) render() fyne.CanvasObject {
	minSize := fyne.NewSize(float32(mv.numColumns*30), float32(max(mv.numRows, 2)*14))
	// y must be created before x as it's width is used to calculate x's offset
	mv.createYAxis()
	mv.createXAxis()
	mv.createZdata(minSize)
	mv.createTextValues(minSize)

	mv.crosshair = NewCrosshair(color.RGBA{165, 55, 253, 180}, 3)
	mv.selectionRect = NewRectangle(color.RGBA{0x30, 0x70, 0xB3, 235}, 4)
	mv.selectionRect.Resize(fyne.NewSize(1, 1))

	mv.grid = grid.New(mv.numColumns, mv.numRows)

	mv.innerView = container.New(&mapLayout{mv: mv, minSize: minSize})
	mv.innerView.Add(mv.valueRects)
	mv.innerView.Add(mv.crosshair)
	mv.innerView.Add(mv.grid)
	mv.innerView.Add(mv.valueTexts)
	mv.innerView.Add(mv.selectionRect)

	if mv.opts.showWBL {
		mv.lamb = cbar.New(widgets.GaugeConfig{
			Title:           "",
			Min:             0.50,
			Center:          1,
			Max:             1.50,
			Steps:           20,
			MinSize:         fyne.NewSize(100, 25),
			TextPosition:    widgets.TextAtCenter,
			DisplayString:   "λ %.3f",
			DisplayTextSize: 25,
		})
	}

	buttons := mv.setupButtons()

	if mv.symbol == nil || mv.numColumns == 1 && mv.numRows == 1 {
		var btns fyne.CanvasObject
		if mv.opts.showWBL {
			btns = container.NewBorder(
				mv.lamb,
				nil,
				nil,
				nil,
				buttons,
			)
		} else {
			btns = buttons
		}

		return container.NewBorder(
			nil,
			btns,
			nil,
			nil,
			container.NewBorder(
				mv.xAxisLabelContainer,
				nil,
				mv.yAxisLabelContainer,
				nil,
				mv.innerView,
			),
		)
	}

	var mid fyne.CanvasObject
	if mv.opts.showWBL {
		mid = container.NewBorder(
			mv.lamb,
			nil,
			nil,
			nil,
			buttons,
		)
	} else {
		mid = buttons
	}

	mapview := container.NewBorder(
		mv.xAxisLabelContainer,
		mid,
		mv.yAxisLabelContainer,
		nil,
		mv.innerView,
	)

	if mv.opts.meshView {
		var err error
		mv.mesh, err = meshgrid.NewMeshgrid(
			mv.xAxisLabel,
			mv.yAxisLabel,
			mv.zAxisLabel,
			mv.symbol.Float64s(),
			mv.numColumns,
			mv.numRows,
		)
		if err == nil {
			split := container.NewVSplit(
				mapview,
				mv.mesh,
			)
			split.Offset = 0.9
			return split
		}
	}
	return mapview
}

func (mv *MapViewer) setupButtons() fyne.CanvasObject {
	if mv.opts.buttonsEnabled {
		return container.NewGridWithColumns(4,
			widget.NewButtonWithIcon("Load File", theme.DocumentIcon(), func() {
				if mv.symbol != nil {
					mv.zData = mv.symbol.Ints()
					mv.Refresh()
				}
			}),
			widget.NewButtonWithIcon("Save File", theme.DocumentSaveIcon(), func() {
				mv.funcs.saveFileFunc(mv.zData)
			}),
			widget.NewButtonWithIcon("Load ECU", theme.DownloadIcon(), func() {
				p := progressmodal.New(mv, "Loading map from ECU")
				p.Show()
				mv.funcs.loadECUFunc()
				p.Stop()
				p.Hide()
			}),
			widget.NewButtonWithIcon("Save ECU", theme.UploadIcon(), func() {
				p := progressmodal.New(mv, "Saving map to ECU")
				p.Show()
				mv.funcs.saveECUFunc(mv.zData)
				p.Stop()
				p.Hide()
			}),
		)
	} else {
		return container.NewWithoutLayout()
	}
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
	var hit bool
	if name == mv.xFrom {
		if name == "IgnProt.fi_Offset" || name == "IgnMastProt.fi_Offset" {
			mv.xValue = int(value * 10)
		} else {
			mv.xValue = int(value)
		}
		hit = true
	}
	if name == mv.yFrom {
		if name == "ActualIn.p_AirInlet" {
			mv.yValue = int(value * 1000)
		} else if name == "Out.X_AccPedal" {
			mv.yValue = int(value * 10)
		} else {
			mv.yValue = int(value)
		}
		hit = true
	}
	if hit {
		mv.setXY(mv.xValue, mv.yValue)
	}

	if name == mv.lambdaName {
		mv.lamb.SetValue(value)
	}
}

const (
	oneThousandTwentyfourth = 1.0 / 1024
	oneHundredTwentyeighth  = 1.0 / 128
)

func (mv *MapViewer) SetCellText(idx int, value int) {
	textValue := strconv.FormatFloat((float64(value)*mv.zCorrFac)+mv.zCorrOffset, 'f', getPrecission(mv.zCorrFac), 64)
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

func (mv *MapViewer) resizeCursor() {
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
			topLeftX := float32(minX) * mv.widthFactor
			topLeftY := float32(mv.numRows-1-maxY) * mv.heightFactor
			width := float32(maxX-minX+1) * mv.widthFactor
			height := float32(maxY-minY+1) * mv.heightFactor
			mv.selectionRect.Resize(fyne.NewSize(width+1, height+1))
			mv.selectionRect.Move(fyne.NewPos(topLeftX-1, topLeftY))

		} else {
			mv.selectionRect.Resize(fyne.NewSize(mv.widthFactor+1, mv.heightFactor+1))
			mv.selectionRect.Move(
				fyne.NewPos(
					(float32(mv.selectedX)*mv.widthFactor)-1,
					(float32(mv.numRows-1-mv.SelectedY) * mv.heightFactor),
				),
			)
		}
	}
}

func (mv *MapViewer) Size() fyne.Size {
	return mv.content.Size()
}

func (mv *MapViewer) Refresh() {
	mv.min, mv.max = findMinMax(mv.zData)
	for i, tv := range mv.zData {
		mv.SetCellText(i, tv)
	}
	for idx, r := range mv.zDataRects {
		col := widgets.GetColorInterpolation((float64(mv.min)*mv.zCorrFac)+mv.zCorrOffset,
			(float64(mv.max)*mv.zCorrFac)+mv.zCorrOffset,
			(float64(mv.zData[idx])*mv.zCorrFac)+mv.zCorrOffset,
		)
		if col != r.FillColor {
			r.FillColor = col
			r.Refresh()
		}
	}

	if mv.mesh != nil {
		mv.mesh.SetMin((float64(mv.min) * mv.zCorrFac) + mv.zCorrOffset)
		mv.mesh.SetMax((float64(mv.max) * mv.zCorrFac) + mv.zCorrOffset)
		var values []float64
		for _, v := range mv.zData {
			values = append(values, (float64(v)*mv.zCorrFac)+mv.zCorrOffset)
		}
		mv.mesh.LoadFloat64s(values)
	}
}

func getPrecission(corrFac float64) int {
	precission := 0
	switch corrFac {
	case 0.1:
		precission = 1
	case 0.01, 0.00390625, 0.004:
		precission = 2
	case 0.001:
		precission = 3
	case oneThousandTwentyfourth:
		precission = 4
	case oneHundredTwentyeighth:
		precission = 3
	}
	return precission
}

func (mv *MapViewer) createTextValues(minsize fyne.Size) {
	mv.valueTexts = container.New(&layout.Grid{Cols: mv.numColumns, Rows: mv.numRows, Text: true, MinimumSize: minsize})
	for _, v := range mv.zData {
		text := &canvas.Text{
			Text:     strconv.FormatFloat((float64(v)*mv.zCorrFac)+mv.zCorrOffset, 'f', getPrecission(mv.zCorrFac), 64),
			TextSize: minTextSize,
			Color:    color.Black,
		}
		mv.textValues = append(mv.textValues, text)
		mv.valueTexts.Add(text)
	}
}

func (mv *MapViewer) createYAxis() {
	mv.yAxisLabelContainer = container.New(&layout.Vertical{})
	if mv.numRows >= 1 {
		for i := mv.numRows - 1; i >= 0; i-- {
			text := &canvas.Text{
				Alignment: fyne.TextAlignCenter,
				Text:      strconv.FormatFloat((float64(mv.yData[i])*mv.yCorrFac)+mv.xCorrOffset, 'f', getPrecission(mv.yCorrFac), 64),
				TextSize:  minTextSize + 2,
			}

			mv.yAxisTexts = append(mv.yAxisTexts, text)
			mv.yAxisLabelContainer.Add(text)
		}
	}
}

func (mv *MapViewer) createXAxis() {
	mv.xAxisLabelContainer = container.New(&layout.Horizontal{Offset: mv.yAxisLabelContainer})
	if mv.numColumns >= 1 {
		for i := 0; i < mv.numColumns; i++ {
			text := &canvas.Text{
				Alignment: fyne.TextAlignCenter,
				Text:      strconv.FormatFloat((float64(mv.xData[i])*mv.xCorrFac)+mv.xCorrOffset, 'f', getPrecission(mv.xCorrFac), 64),
				TextSize:  minTextSize + 2,
			}
			mv.xAxisTexts = append(mv.xAxisTexts, text)
			mv.xAxisLabelContainer.Add(text)
		}
	}
}

func (mv *MapViewer) createZdata(minsize fyne.Size) {
	mv.valueRects = container.New(&layout.Grid{Cols: mv.numColumns, Rows: mv.numRows, MinimumSize: minsize})
	minCorrected := (float64(mv.min) * mv.zCorrFac) + mv.zCorrOffset
	maxCorrected := (float64(mv.max) * mv.zCorrFac) + mv.zCorrOffset
	// Calculate the colors for each cell based on data
	for y := 0; y < mv.numRows; y++ {
		for x := 0; x < mv.numColumns; x++ {
			index := y*mv.numColumns + x
			value := (float64(mv.zData[index]) * mv.zCorrFac) + mv.zCorrOffset
			color := widgets.GetColorInterpolation(minCorrected, maxCorrected, value)
			rect := &canvas.Rectangle{FillColor: color}
			mv.zDataRects = append(mv.zDataRects, rect)
			mv.valueRects.Add(rect)
		}
	}
}

func (mv *MapViewer) setXY(xValue, yValue int) error {
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
	mv.xIndex = xIdx
	mv.yIndex = yIdx

	mv.crosshair.Move(
		fyne.NewPos(
			float32(xIdx)*mv.widthFactor,
			float32(float64(mv.numRows-1)-yIdx)*mv.heightFactor,
		),
	)
	if mv.opts.cursorFollowCrosshair {
		mv.selectedX = int(math.Round(xIdx))
		mv.SelectedY = int(math.Round(yIdx))
		mv.updateCursor()
	}
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

func NewCrosshair(strokeColor color.RGBA, strokeWidth float32) *canvas.Rectangle {
	return &canvas.Rectangle{
		FillColor:   strokeColor,
		StrokeColor: strokeColor,
		StrokeWidth: strokeWidth,
	}
}

func NewRectangle(strokeColor color.RGBA, strokeWidth float32) *canvas.Rectangle {
	return &canvas.Rectangle{
		FillColor:   color.RGBA{0, 0, 0, 0},
		StrokeColor: strokeColor,
		StrokeWidth: strokeWidth,
	}
}
