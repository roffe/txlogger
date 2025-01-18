package mapviewer

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

const (
	minTextSize             = 11
	maxTextSize             = 28
	oneThousandTwentyfourth = 1.0 / 1024
	oneHundredTwentyeighth  = 1.0 / 128
)

type MapViewerInfo struct {
	XName, YName, ZName string
	XLen, YLen, ZLen    int
	XFrom, YFrom        string
}

var (
	//  _ fyne.Tappable = (*MapViewer)(nil)
	_ fyne.Focusable    = (*MapViewer)(nil)
	_ desktop.Mouseable = (*MapViewer)(nil)
	_ desktop.Hoverable = (*MapViewer)(nil)
	_ fyne.Draggable    = (*MapViewer)(nil)
)

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

	selectionRect *canvas.Rectangle
	crosshair     *canvas.Rectangle

	zDataRects []*canvas.Rectangle

	ipf interpolate.InterPolFunc

	selectedX, SelectedY int

	mesh *meshgrid.Meshgrid

	// Mouse
	mousePos      fyne.Position
	selecting     bool
	lastModifier  fyne.KeyModifier
	selectedCells []int

	// Keyboard
	inputBuffer   strings.Builder
	restoreValues bool

	popup *widget.PopUpMenu

	widthFactor  float32
	heightFactor float32

	OnMouseDown func()
}

type opts struct {
	editable              bool
	buttonsEnabled        bool
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

	mv.crosshair = NewCrosshair(color.RGBA{165, 55, 253, 180}, 3)
	mv.selectionRect = NewRectangle(color.RGBA{0x30, 0x70, 0xFF, 0xFF}, 3)
	// log.Printf("MapViewer c: %d r: %d dlen: %d x: %s y: %s z: %s", mv.numColumns, mv.numRows, mv.numData, mv.xFrom, mv.yFrom, mv.symbol.Name)

	mv.createYAxis()
	mv.createXAxis()
	mv.createZdata()
	mv.createTextValues()

	return mv, nil
}

// Dragged is called when the user drags the window.
func (mv *MapViewer) Dragged(ev *fyne.DragEvent) {
	moveEvent := &desktop.MouseEvent{}
	moveEvent.Position = ev.Position
	if mv.selecting {
		moveEvent.Button = desktop.MouseButtonPrimary
	}
	mv.MouseMoved(moveEvent)
}

// DragEnd is called when the user stops dragging the window.
func (mv *MapViewer) DragEnd() {}

func (mv *MapViewer) CreateRenderer() fyne.WidgetRenderer {
	//return widget.NewSimpleRenderer(mv.content)
	mv.content = mv.render()
	return &mapViewerRenderer{mv: mv}
}

type movingRectsLayout struct {
	mv *MapViewer
}

func (mr *movingRectsLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 0)
}

func (mr *movingRectsLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	// Calculate shared factors
	mr.mv.widthFactor = mr.mv.innerView.Size().Width / float32(mr.mv.numColumns)
	mr.mv.heightFactor = mr.mv.innerView.Size().Height / float32(mr.mv.numRows)

	// Calculate and update text sizes
	newTextSize := calculateTextSize(mr.mv.widthFactor, mr.mv.heightFactor)
	for _, text := range mr.mv.textValues {
		if text.TextSize != newTextSize {
			text.TextSize = newTextSize
			text.Refresh()
		}
	}

	// Position and resize crosshair
	mr.mv.crosshair.Resize(fyne.NewSize(mr.mv.widthFactor, mr.mv.heightFactor))
	mr.mv.crosshair.Move(
		fyne.NewPos(
			float32(mr.mv.xIndex)*mr.mv.widthFactor,
			float32(float64(mr.mv.numRows)-1-mr.mv.yIndex)*mr.mv.heightFactor,
		),
	)
	mr.mv.resizeCursor()
	mr.mv.updateCursor()
}

func (mv *MapViewer) render() fyne.CanvasObject {

	// y must be created before x as it's width is used to calculate x's offset

	mv.crosshair.CornerRadius = 4
	mv.crosshair.Resize(fyne.NewSize(34, 14))
	mv.crosshair.Hide()

	mv.selectionRect.CornerRadius = 4
	mv.selectionRect.Resize(fyne.NewSize(34, 14))
	//mv.selectionRect.Hide()

	mv.innerView = container.NewStack(
		mv.valueRects,
		container.New(&movingRectsLayout{mv: mv},
			mv.crosshair,
			mv.selectionRect,
		),
		mv.valueTexts,
	)

	buttons := mv.createButtons()

	if mv.symbol == nil || mv.numColumns == 1 && mv.numRows == 1 {
		return container.NewBorder(
			nil,
			buttons,
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

	mapview := container.NewBorder(
		mv.xAxisLabelContainer,
		nil,
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
				container.NewBorder(
					nil,
					buttons,
					nil,
					nil,
					mv.mesh,
				),
			)
			split.Offset = 0.9
			return split
		}
	}
	return container.NewBorder(
		nil,
		buttons,
		nil,
		nil,
		mapview,
	)
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
		//fyne.Do(func() {
		if mv.crosshair.Hidden {
			mv.crosshair.Resize(fyne.NewSize(mv.widthFactor, mv.heightFactor))
			mv.crosshair.Show()
		}
		mv.setXY(mv.xValue, mv.yValue)
		//})
	}
}

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
	//fyne.Do(func() {
	mv.Refresh()
	//})
}

func (mv *MapViewer) Refresh() {
	values := make([]float64, len(mv.zData))
	mv.min, mv.max = findMinMax(mv.zData)
	min := (float64(mv.min) * mv.zCorrFac) + mv.zCorrOffset
	max := (float64(mv.max) * mv.zCorrFac) + mv.zCorrOffset
	for i, v := range mv.zData {
		mv.SetCellText(i, v)
		values[i] = (float64(v) * mv.zCorrFac) + mv.zCorrOffset
		col := widgets.GetColorInterpolation(
			min,
			max,
			values[i],
		)
		r := mv.zDataRects[i]
		if col != r.FillColor {
			r.StrokeColor = col
			r.FillColor = col
			r.Refresh()
		}
	}
	if mv.mesh != nil {
		mv.mesh.SetMin(min)
		mv.mesh.SetMax(max)
		mv.mesh.LoadFloat64s(values)
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

func (mv *MapViewer) createTextValues() {
	mv.valueTexts = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.4))
	for _, v := range mv.zData {
		text := &canvas.Text{
			Text:      strconv.FormatFloat((float64(v)*mv.zCorrFac)+mv.zCorrOffset, 'f', getPrecission(mv.zCorrFac), 64),
			TextSize:  minTextSize,
			Color:     color.Black,
			Alignment: fyne.TextAlignCenter,
		}
		mv.textValues = append(mv.textValues, text)
		mv.valueTexts.Add(text)
	}
}

func (mv *MapViewer) createZdata() {
	mv.valueRects = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.33))
	minCorrected := (float64(mv.min) * mv.zCorrFac) + mv.zCorrOffset
	maxCorrected := (float64(mv.max) * mv.zCorrFac) + mv.zCorrOffset
	// Calculate the colors for each cell based on data
	for _, v := range mv.zData {
		value := (float64(v) * mv.zCorrFac) + mv.zCorrOffset
		color := widgets.GetColorInterpolation(minCorrected, maxCorrected, value)
		rect := &canvas.Rectangle{FillColor: color, StrokeColor: color, StrokeWidth: 0}
		rect.SetMinSize(fyne.NewSize(34, 14))
		mv.zDataRects = append(mv.zDataRects, rect)
		mv.valueRects.Add(rect)
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

func (mv *MapViewer) createButtons() *fyne.Container {
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
				p := progressmodal.New(fyne.CurrentApp().Driver().AllWindows()[0].Canvas(), "Loading map from ECU")
				p.Show()
				go func() {
					mv.funcs.loadECUFunc()
					//fyne.Do(func() {
					p.Hide()
					//})
				}()
			}),
			widget.NewButtonWithIcon("Save ECU", theme.UploadIcon(), func() {
				p := progressmodal.New(fyne.CurrentApp().Driver().AllWindows()[0].Canvas(), "Saving map to ECU")
				p.Show()
				go func() {
					mv.funcs.saveECUFunc(mv.zData)
					//fyne.Do(func() {
					p.Hide()
					//})

				}()
			}),
		)
	} else {
		return container.NewWithoutLayout()
	}
}

func (mv *MapViewer) resizeCursor() {
	// Early return if no selection
	if mv.selectedX < 0 {
		return
	}

	var pos fyne.Position
	var size fyne.Size

	// Handle multiple cell selection
	if len(mv.selectedCells) > 1 {
		// Pre-calculate divisor to avoid repeated division operations
		colDivisor := float32(mv.numColumns)

		// Initialize bounds using first cell to avoid unnecessary comparisons
		firstCell := mv.selectedCells[0]
		minX := firstCell % mv.numColumns
		maxX := minX
		minY := int(float32(firstCell) / colDivisor)
		maxY := minY

		// Find bounds in a single pass
		for i := 1; i < len(mv.selectedCells); i++ {
			cell := mv.selectedCells[i]
			x := cell % mv.numColumns
			y := int(float32(cell) / colDivisor)

			if x < minX {
				minX = x
			} else if x > maxX {
				maxX = x
			}

			if y < minY {
				minY = y
			} else if y > maxY {
				maxY = y
			}
		}

		// Calculate position and size once
		topLeftX := float32(minX) * mv.widthFactor
		topLeftY := float32(mv.numRows-1-maxY) * mv.heightFactor
		width := float32(maxX-minX+1) * mv.widthFactor
		height := float32(maxY-minY+1) * mv.heightFactor

		pos = fyne.NewPos(topLeftX-1, topLeftY)
		size = fyne.NewSize(width+1, height+1)
	} else {
		// Single cell selection
		pos = fyne.NewPos(
			(float32(mv.selectedX)*mv.widthFactor)-1,
			(float32(mv.numRows-1-mv.SelectedY) * mv.heightFactor),
		)
		size = fyne.NewSize(mv.widthFactor+1, mv.heightFactor+1)
	}

	// Batch UI updates
	mv.selectionRect.Resize(size)
	mv.selectionRect.Move(pos)
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
