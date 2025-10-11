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
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

const (
	minTextSize = 11
	maxTextSize = 28
)

type MapViewerInfo struct {
	XName, YName, ZName string
	XLen, YLen, ZLen    int
	XFrom, YFrom        string
}

var (
	//  _ fyne.Tappable = (*MapViewer)(nil)
	// _ fyne.Focusable    = (*MapViewer)(nil)
	_ desktop.Mouseable = (*MapViewer)(nil)
	_ desktop.Hoverable = (*MapViewer)(nil)
	_ fyne.Draggable    = (*MapViewer)(nil)
)

type MapViewer struct {
	widget.BaseWidget
	focused    bool
	cfg        *Config
	zMin, zMax float64

	xValue, yValue float64
	xIndex, yIndex float64

	numColumns, numRows, numData int

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

	colorMode widgets.ColorBlindMode
}

func New(config *Config) (*MapViewer, error) {
	mv := &MapViewer{
		cfg:           config,
		crosshair:     NewCrosshair(color.RGBA{165, 55, 253, 180}, 3),
		selectionRect: NewRectangle(color.RGBA{0x30, 0x70, 0xFF, 0xFF}, 3),
		numColumns:    len(config.XData),
		numRows:       len(config.YData),
		numData:       len(config.ZData),
	}
	mv.ExtendBaseWidget(mv)

	log.Printf("mapViewer c:%d r:%d len:%d x:%s y:%s z:%s", mv.numColumns, mv.numRows, mv.numData, mv.cfg.XFrom, mv.cfg.YFrom, mv.cfg.Symbol.Name)

	if len(mv.cfg.ZData) == 0 {
		return nil, fmt.Errorf("mapViewer zData is empty")
	}

	mv.zMin, mv.zMax = widgets.FindMinMax(mv.cfg.ZData)

	if mv.numColumns*mv.numRows != mv.numData && mv.numColumns > 1 && mv.numRows > 1 {
		return nil, fmt.Errorf("mapViewer columns * rows != data length")
	}

	mv.createYAxis()
	mv.createXAxis()
	mv.createZdata()
	mv.createTextValues()

	return mv, nil
}

func (mv *MapViewer) SetColorBlindMode(mode widgets.ColorBlindMode) {
	if mv.colorMode != mode {
		mv.colorMode = mode
		mv.Refresh()
		mv.mesh.SetColorBlindMode(mode)
	}
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
	mv.content = mv.render()
	return widget.NewSimpleRenderer(mv.content)
	//return &mapViewerRenderer{mv: mv}
}

type movingRectsLayout struct {
	mv      *MapViewer
	oldSize fyne.Size
}

func (mr *movingRectsLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.Size{Width: 0, Height: 0}
}

func (mr *movingRectsLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	if size == mr.oldSize {
		return
	}
	mr.oldSize = size
	// Calculate shared factors
	mr.mv.widthFactor = mr.mv.innerView.Size().Width / float32(mr.mv.numColumns)
	mr.mv.heightFactor = mr.mv.innerView.Size().Height / float32(mr.mv.numRows)

	mr.mv.crosshair.Resize(fyne.Size{Width: mr.mv.widthFactor, Height: mr.mv.heightFactor})

	// Calculate and update text sizes
	newTextSize := calculateTextSize(mr.mv.widthFactor, mr.mv.heightFactor)
	for _, text := range mr.mv.textValues {
		if text.TextSize != newTextSize {
			text.TextSize = newTextSize
			text.Refresh()
		}
	}

	// Position and resize crosshair
	mr.mv.crosshair.Move(
		fyne.NewPos(
			float32(mr.mv.xIndex)*mr.mv.widthFactor,
			float32(float64(mr.mv.numRows)-1-mr.mv.yIndex)*mr.mv.heightFactor,
		),
	)
	mr.mv.resizeSelectionRect()
	mr.mv.updateCursor(false)
}

func (mv *MapViewer) render() fyne.CanvasObject {
	//mv.crosshair.CornerRadius = 4
	mv.crosshair.Resize(fyne.NewSize(34, 14))
	mv.crosshair.Hide()

	//mv.selectionRect.CornerRadius = 4
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

	if mv.cfg.Symbol == nil || (mv.numColumns == 1 || mv.numRows == 1) {
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

	if mv.cfg.MeshView {
		var err error
		mv.mesh, err = meshgrid.NewMeshgrid(
			mv.cfg.XAxisLabel,
			mv.cfg.YAxisLabel,
			mv.cfg.ZAxisLabel,
			mv.cfg.ZData,
			mv.numColumns,
			mv.numRows,
			mv.colorMode,
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
			split.Offset = 0.2
			return split
		} else {
			log.Println("MapViewer meshview failed:", err)
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
		XFrom: mv.cfg.XFrom,
		YFrom: mv.cfg.YFrom,
	}
}

func (mv *MapViewer) SetX(xValue float64) {
	mv.xValue = xValue
}

func (mv *MapViewer) SetY(yValue float64) {
	mv.yValue = yValue
	if mv.crosshair.Hidden {
		size := fyne.Size{Width: mv.widthFactor, Height: mv.heightFactor}
		fyne.Do(func() {
			mv.crosshair.Show()
			if mv.crosshair.Size() != size {
				mv.crosshair.Resize(size)
			}
		})
	}
	mv.setXY()
}

func (mv *MapViewer) setCellText(idx int, value float64) {
	textValue := strconv.FormatFloat(value, 'f', mv.cfg.ZPrecision, 64)
	if mv.textValues[idx].Text != textValue {
		mv.textValues[idx].Text = textValue
		mv.textValues[idx].Refresh()
	}
}

func (mv *MapViewer) SetZData(zData []float64) error {
	if len(zData) != mv.numData {
		return fmt.Errorf("MapViewer SetZ len mismatch %d != %d", len(zData), mv.numData)
	}
	mv.cfg.ZData = zData
	mv.numData = len(zData)
	mv.Refresh()
	return nil
}

func (mv *MapViewer) Refresh() {
	mv.zMin, mv.zMax = widgets.FindMinMax(mv.cfg.ZData)
	for idx, value := range mv.cfg.ZData {
		mv.setCellText(idx, value)
		col := widgets.GetColorInterpolation(
			mv.zMin,
			mv.zMax,
			value,
			mv.colorMode,
		)
		r := mv.zDataRects[idx]
		if col != r.FillColor {
			r.StrokeColor = col
			r.FillColor = col
			r.Refresh()
		}
	}
	if mv.mesh != nil {
		mv.mesh.LoadFloat64s(mv.zMin, mv.zMax, mv.cfg.ZData)
	}
}

func (mv *MapViewer) createYAxis() {
	mv.yAxisLabelContainer = container.New(&layout.Vertical{})
	if mv.numRows >= 1 {
		for i := mv.numRows - 1; i >= 0; i-- {
			text := &canvas.Text{
				Alignment: fyne.TextAlignCenter,
				Text:      strconv.FormatFloat(mv.cfg.YData[i], 'f', mv.cfg.YPrecision, 64),
				TextSize:  minTextSize + 2,
			}
			mv.yAxisTexts = append(mv.yAxisTexts, text)
			mv.yAxisLabelContainer.Add(text)
		}
		return
	}
}

func (mv *MapViewer) createXAxis() {
	mv.xAxisLabelContainer = container.New(&layout.Horizontal{Offset: mv.yAxisLabelContainer})
	if mv.numColumns >= 1 {
		for i := 0; i < mv.numColumns; i++ {
			text := &canvas.Text{
				Alignment: fyne.TextAlignCenter,
				Text:      strconv.FormatFloat(mv.cfg.XData[i], 'f', mv.cfg.XPrecision, 64),
				TextSize:  minTextSize + 2,
			}
			mv.xAxisTexts = append(mv.xAxisTexts, text)
			mv.xAxisLabelContainer.Add(text)
		}
		return
	}
}

func (mv *MapViewer) createTextValues() {
	mv.valueTexts = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.32))
	for _, v := range mv.cfg.ZData {
		text := &canvas.Text{
			Text:      strconv.FormatFloat(v, 'f', mv.cfg.ZPrecision, 64),
			TextSize:  minTextSize,
			Color:     color.Black,
			Alignment: fyne.TextAlignCenter,
		}
		mv.textValues = append(mv.textValues, text)
		mv.valueTexts.Add(text)
	}
}

func (mv *MapViewer) createZdata() {
	mv.valueRects = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.32))
	for _, value := range mv.cfg.ZData {
		color := widgets.GetColorInterpolation(mv.zMin, mv.zMax, value, mv.colorMode)
		rect := &canvas.Rectangle{FillColor: color, StrokeColor: color, StrokeWidth: 0}
		rect.SetMinSize(fyne.NewSize(34, 14))
		mv.zDataRects = append(mv.zDataRects, rect)
		mv.valueRects.Add(rect)
	}
}

func (mv *MapViewer) setXY() error {
	xIdx, yIdx, err := interpolate.Interpolate64S(mv.cfg.XData, mv.cfg.YData, mv.cfg.ZData, mv.xValue, mv.yValue)
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

	crosshairPos := fyne.Position{
		X: float32(xIdx) * mv.widthFactor,
		Y: float32(float64(mv.numRows-1)-yIdx) * mv.heightFactor,
	}

	mv.crosshair.Move(crosshairPos)
	if mv.cfg.CursorFollowCrosshair {
		mv.selectedX = int(math.Round(xIdx))
		mv.SelectedY = int(math.Round(yIdx))
		mv.updateCursor(true)
	}
	return nil
}

func (mv *MapViewer) createButtons() *fyne.Container {
	if mv.cfg.Buttons {
		return container.NewGridWithColumns(4,
			widget.NewButtonWithIcon("Load File", theme.DocumentIcon(), func() {
				if mv.cfg.Symbol != nil {
					log.Println("load", mv.cfg.Symbol.Name)
					mv.cfg.ZData = mv.cfg.Symbol.Float64s()
					mv.Refresh()
				}
			}),
			widget.NewButtonWithIcon("Save File", theme.DocumentSaveIcon(), func() {
				mv.cfg.SaveFileFunc(mv.cfg.ZData)
			}),
			widget.NewButtonWithIcon("Load ECU", theme.DownloadIcon(), func() {
				p := progressmodal.New(fyne.CurrentApp().Driver().AllWindows()[0].Canvas(), "Loading map from ECU")
				p.Show()
				go func() {
					mv.cfg.LoadECUFunc()
					fyne.Do(func() {
						p.Hide()
					})
				}()
			}),
			widget.NewButtonWithIcon("Save ECU", theme.UploadIcon(), func() {
				p := progressmodal.New(fyne.CurrentApp().Driver().AllWindows()[0].Canvas(), "Saving map to ECU")
				p.Show()
				go func() {
					mv.cfg.SaveECUFunc(mv.cfg.ZData)
					fyne.Do(func() {
						p.Hide()
					})

				}()
			}),
		)
	} else {
		return container.NewWithoutLayout()
	}
}

func (mv *MapViewer) resizeSelectionRect() {
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
	mv.selectionRect.Move(pos)
	mv.selectionRect.Resize(size)
	//mv.selectionRect.MoveAndResize(pos, size)
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

func NewCrosshair(strokeColor color.RGBA, strokeWidth float32) *canvas.Rectangle {
	return &canvas.Rectangle{
		FillColor:    strokeColor,
		StrokeColor:  strokeColor,
		StrokeWidth:  strokeWidth,
		CornerRadius: 4,
	}
}

func NewRectangle(strokeColor color.RGBA, strokeWidth float32) *canvas.Rectangle {
	return &canvas.Rectangle{
		FillColor:    color.RGBA{0, 0, 0, 0},
		StrokeColor:  strokeColor,
		StrokeWidth:  strokeWidth,
		CornerRadius: 4,
	}
}
