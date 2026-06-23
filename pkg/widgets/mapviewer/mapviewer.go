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
	fynelayout "fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets/graph2d"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
)

const (
	minTextSize = 11
	maxTextSize = 28
)

// singleCellColor is used for 1x1 maps where value-based color interpolation
// is meaningless (min == max yields a flat gray). ponytail: fixed accent.
var singleCellColor = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF} // white

var (
	//  _ fyne.Tappable = (*MapViewer)(nil)
	_ fyne.Focusable    = (*MapViewer)(nil)
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

	innerView        *fyne.Container
	valueRects       *fyne.Container
	valueTexts       *fyne.Container
	selectionOverlay *fyne.Container
	regionOverlay    *fyne.Container

	crosshair *canvas.Rectangle

	zDataRects     []*canvas.Rectangle
	selectionRects []*canvas.Rectangle

	selectedX, SelectedY int

	mesh        *meshgrid.Meshgrid
	meshSplit   *container.Split
	meshModeBtn *widget.Button
	graph       *graph2d.Graph

	// Mouse
	mousePos                 fyne.Position
	selecting                bool
	dragCornerX, dragCornerY int
	lastModifier             fyne.KeyModifier
	selectedCells            []int

	// Keyboard
	inputBuffer   strings.Builder
	restoreValues bool

	// scratch buffer for formatting cell values without allocating
	scratch []byte

	popup *widget.PopUpMenu

	widthFactor  float32
	heightFactor float32

	colorMode colors.ColorBlindMode
}

func New(config *Config) (*MapViewer, error) {
	mv := &MapViewer{
		cfg:        config,
		crosshair:  NewCrosshair(color.RGBA{165, 55, 253, 180}, 3),
		numColumns: len(config.XData),
		numRows:    len(config.YData),
		numData:    len(config.ZData),
		colorMode:  config.ColorblindMode,
	}
	mv.ExtendBaseWidget(mv)

	log.Printf("%s c:%d r:%d len:%d", config.Name, mv.numColumns, mv.numRows, mv.numData)

	if len(mv.cfg.ZData) == 0 {
		return nil, fmt.Errorf("mapViewer zData is empty")
	}
	mv.zMin, mv.zMax = common.FindMinMaxFloat64(mv.cfg.ZData)
	if mv.numColumns*mv.numRows != mv.numData && mv.numColumns > 1 && mv.numRows > 1 {
		return nil, fmt.Errorf("mapViewer columns * rows != data length")
	}
	return mv, nil
}

func (mv *MapViewer) toggleMesh() {
	if mv.mesh == nil || mv.meshSplit == nil {
		return
	}
	if mv.mesh.Visible() {
		mv.mesh.Hide()
		mv.meshModeBtn.Hide()
		mv.meshSplit.SetOffset(1)
	} else {
		mv.mesh.Show()
		mv.meshModeBtn.Show()
		mv.meshSplit.SetOffset(0.2)
	}
}

func (mv *MapViewer) SetColorBlindMode(mode colors.ColorBlindMode) {
	if mv.colorMode != mode {
		mv.colorMode = mode
		mv.Refresh()
		if mv.mesh != nil {
			mv.mesh.SetColorBlindMode(mode)
		}
		if mv.graph != nil {
			mv.graph.SetColorBlindMode(mode)
		}
	}
}

func (mv *MapViewer) CreateRenderer() fyne.WidgetRenderer {
	mv.createYAxis()
	mv.createXAxis()
	mv.createZdata()
	mv.createSelectionOverlay()
	mv.createRegionOverlay()
	mv.createTextValues()
	// Start with nothing selected; a cell is selected on first click/keypress.
	mv.selectedCells = nil
	mv.content = mv.render()
	return widget.NewSimpleRenderer(mv.content)
	// return &mapViewerRenderer{mv: mv}
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
	// The selection lives in selectionOverlay, which shares the cell grid and is
	// repositioned automatically by the layout, so there is nothing to recompute
	// for it here.
}

func (mv *MapViewer) render() fyne.CanvasObject {
	// mv.crosshair.CornerRadius = 4
	mv.crosshair.Resize(fyne.NewSize(34, 14))
	mv.crosshair.Hide()

	layers := []fyne.CanvasObject{mv.valueRects, mv.selectionOverlay}
	if mv.regionOverlay != nil {
		layers = append(layers, mv.regionOverlay)
	}
	layers = append(layers,
		container.New(&movingRectsLayout{mv: mv},
			mv.crosshair,
		),
		mv.valueTexts,
	)
	mv.innerView = container.NewStack(layers...)

	buttons := mv.createButtons()

	var stepButtons fyne.CanvasObject
	if mv.cfg.Editable {
		stepButtons = mv.createStepButtons()
	}

	mapview := container.NewBorder(
		mv.xAxisLabelContainer,
		stepButtons,
		mv.yAxisLabelContainer,
		nil,
		mv.innerView,
	)

	if mv.numColumns == 1 || mv.numRows == 1 {
		if mv.cfg.MeshView && mv.numData > 1 {
			axisData := mv.cfg.XData
			axisPrecision := mv.cfg.XPrecision
			axisLabel := mv.cfg.XLabel
			if mv.numColumns == 1 {
				axisData = mv.cfg.YData
				axisPrecision = mv.cfg.YPrecision
				axisLabel = mv.cfg.YLabel
			}
			mv.graph = graph2d.New(&graph2d.Config{
				AxisData:       axisData,
				Values:         mv.cfg.ZData,
				AxisPrecision:  axisPrecision,
				ValuePrecision: mv.cfg.ZPrecision,
				AxisLabel:      axisLabel,
				ColorblindMode: mv.colorMode,
			})
			if mv.cfg.OnMouseDown != nil {
				mv.graph.OnMouseDown = mv.cfg.OnMouseDown
			}
			split := container.NewVSplit(
				mapview,
				container.NewBorder(
					nil,
					buttons,
					nil,
					nil,
					mv.graph,
				),
			)
			split.Offset = 0.2
			return split
		}
		return container.NewBorder(
			nil,
			buttons,
			nil,
			nil,
			mapview,
		)
	}

	if mv.cfg.MeshView {
		var err error
		mv.mesh, err = meshgrid.NewMeshgrid(
			mv.cfg.XLabel,
			mv.cfg.YLabel,
			mv.cfg.ZLabel,
			mv.cfg.ZData,
			mv.numColumns,
			mv.numRows,
			mv.cfg.XData,
			mv.cfg.YData,
			mv.cfg.XPrecision,
			mv.cfg.YPrecision,
			mv.cfg.ZPrecision,
			mv.colorMode,
			mv.cfg.MeshRenderer,
		)

		if mv.cfg.OnMouseDown != nil {
			mv.mesh.OnMouseDown = mv.cfg.OnMouseDown
		}

		if err == nil {
			meshModeBtn := widget.NewButtonWithIcon("", theme.GridIcon(), mv.mesh.CycleRenderMode)
			meshModeBtn.Importance = widget.LowImportance
			mv.meshModeBtn = meshModeBtn
			split := container.NewVSplit(
				mapview,
				container.NewStack(
					mv.mesh,
					container.NewVBox(
						container.NewHBox(fynelayout.NewSpacer(), meshModeBtn),
					),
				),
			)
			split.Offset = 0.2
			mv.meshSplit = split
			// buttons live outside the split so they stay visible when the mesh is toggled off.
			return container.NewBorder(nil, buttons, nil, nil, split)
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

func (mv *MapViewer) SetX(xValue float64) {
	mv.xValue = xValue
}

func (mv *MapViewer) SetY(yValue float64) {
	mv.yValue = yValue
	if mv.crosshair.Hidden {
		size := fyne.Size{Width: mv.widthFactor, Height: mv.heightFactor}

		mv.crosshair.Show()
		if mv.crosshair.Size() != size {
			mv.crosshair.Resize(size)
		}

	}
	if err := mv.setXY(); err != nil {
		log.Println("MapViewer SetXY error:", err)
	}
}

func (mv *MapViewer) setCellText(idx int, value float64) {
	mv.scratch = strconv.AppendFloat(mv.scratch[:0], value, 'f', mv.cfg.ZPrecision, 64)
	text := mv.textValues[idx]
	if string(mv.scratch) != text.Text {
		text.Text = string(mv.scratch)
		text.Refresh()
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
	mv.zMin, mv.zMax = common.FindMinMaxFloat64(mv.cfg.ZData)
	if len(mv.textValues) == 0 {
		// renderer not created yet; createTextValues/createZdata pick up
		// the current ZData and color mode when it is
		return
	}
	for idx, value := range mv.cfg.ZData {
		mv.setCellText(idx, value)
		if mv.numData == 1 {
			continue // single cell keeps singleCellColor
		}
		col := colors.GetColorInterpolation(
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
	if mv.graph != nil {
		mv.graph.SetValues(mv.zMin, mv.zMax, mv.cfg.ZData)
	}
}

func (mv *MapViewer) createYAxis() {
	mv.yAxisTexts = make([]*canvas.Text, 0, mv.numRows)
	objs := make([]fyne.CanvasObject, 0, mv.numRows)
	// ponytail: single value view has only a "0" axis label, skip it
	for i := mv.numRows - 1; i >= 0 && mv.numData > 1; i-- {
		text := &canvas.Text{
			Alignment: fyne.TextAlignCenter,
			Text:      strconv.FormatFloat(mv.cfg.YData[i], 'f', mv.cfg.YPrecision, 64),
			TextSize:  minTextSize + 2,
		}
		mv.yAxisTexts = append(mv.yAxisTexts, text)
		objs = append(objs, text)
	}
	mv.yAxisLabelContainer = container.New(&layout.Vertical{}, objs...)
}

func (mv *MapViewer) createXAxis() {
	mv.xAxisTexts = make([]*canvas.Text, 0, mv.numColumns)
	objs := make([]fyne.CanvasObject, 0, mv.numColumns)
	// ponytail: single value view has only a "0" axis label, skip it
	for i := 0; i < mv.numColumns && mv.numData > 1; i++ {
		text := &canvas.Text{
			Alignment: fyne.TextAlignCenter,
			Text:      strconv.FormatFloat(mv.cfg.XData[i], 'f', mv.cfg.XPrecision, 64),
			TextSize:  minTextSize + 2,
		}
		mv.xAxisTexts = append(mv.xAxisTexts, text)
		objs = append(objs, text)
	}
	mv.xAxisLabelContainer = container.New(&layout.Horizontal{Offset: mv.yAxisLabelContainer}, objs...)
}

func (mv *MapViewer) createTextValues() {
	mv.textValues = make([]*canvas.Text, 0, mv.numData)
	objs := make([]fyne.CanvasObject, 0, mv.numData)
	for _, v := range mv.cfg.ZData {
		text := &canvas.Text{
			Text:      strconv.FormatFloat(v, 'f', mv.cfg.ZPrecision, 64),
			TextSize:  minTextSize,
			Color:     color.Black,
			Alignment: fyne.TextAlignCenter,
		}
		mv.textValues = append(mv.textValues, text)
		objs = append(objs, text)
	}
	mv.valueTexts = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.32), objs...)
}

func (mv *MapViewer) createZdata() {
	mv.zDataRects = make([]*canvas.Rectangle, 0, mv.numData)
	objs := make([]fyne.CanvasObject, 0, mv.numData)
	singleCell := mv.numData == 1
	for _, value := range mv.cfg.ZData {
		col := colors.GetColorInterpolation(mv.zMin, mv.zMax, value, mv.colorMode)
		minHeight := float32(14)
		if singleCell {
			col = singleCellColor
			minHeight = 28
		}
		rect := &canvas.Rectangle{FillColor: col, StrokeColor: col, StrokeWidth: 0}
		rect.SetMinSize(fyne.NewSize(34, minHeight))
		mv.zDataRects = append(mv.zDataRects, rect)
		objs = append(objs, rect)
	}
	mv.valueRects = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.32), objs...)
}

// createSelectionOverlay builds a dedicated highlight layer with one
// translucent rectangle per cell. The rectangles are hidden by default and
// only shown for cells present in mv.selectedCells. Keeping selection in its
// own layer decouples it from the value rects, so editing a cell's value never
// clears the selection visual and vice versa.
func (mv *MapViewer) createSelectionOverlay() {
	mv.selectionRects = make([]*canvas.Rectangle, mv.numData)
	objs := make([]fyne.CanvasObject, mv.numData)
	for i := range mv.selectionRects {
		rect := canvas.NewRectangle(color.RGBA{0xDE, 0xDF, 0xE4, 0xFF})
		rect.Hide()
		mv.selectionRects[i] = rect
		objs[i] = rect
	}
	mv.selectionOverlay = container.New(layout.NewGrid(mv.numColumns, mv.numRows, 1.32), objs...)
}

// createRegionOverlay builds a thin line layer that traces the boundary between
// the cells flagged in cfg.RegionBorder and the rest — e.g. the closed-loop /
// open-loop fuel transition — as a staircase that cuts through the map instead
// of boxing each cell. Leaves regionOverlay nil when there is no region or the
// region has no internal boundary (all cells in or all out).
func (mv *MapViewer) createRegionOverlay() {
	if len(mv.cfg.RegionBorder) != mv.numData || mv.numColumns <= 1 || mv.numRows <= 1 {
		return
	}
	edges := mv.regionEdges()
	if len(edges) == 0 {
		return
	}
	borderCol := mv.cfg.RegionBorderColor
	if borderCol.A == 0 {
		borderCol = color.RGBA{0x70, 0x80, 0x90, 0xFF} // ponytail: default slate boundary
	}
	lines := make([]*canvas.Line, len(edges))
	objs := make([]fyne.CanvasObject, len(edges))
	for i := range edges {
		ln := canvas.NewLine(borderCol)
		ln.StrokeWidth = 4
		lines[i] = ln
		objs[i] = ln
	}
	mv.regionOverlay = container.New(&regionBorderLayout{mv: mv, edges: edges, lines: lines}, objs...)
}

// regionEdge marks one shared cell edge on the region boundary. vertical = the
// edge to the right of cell (r,c); otherwise the edge on top of cell (r,c),
// shared with row r+1. Indices use the same row-major order as ZData.
type regionEdge struct {
	r, c     int
	vertical bool
}

// regionEdges collects every edge where a flagged cell touches an unflagged one.
// Only internal transitions are returned, so the map's outer border is never
// drawn — just the closed/open interface.
func (mv *MapViewer) regionEdges() []regionEdge {
	rb := mv.cfg.RegionBorder
	var edges []regionEdge
	for r := 0; r < mv.numRows; r++ {
		for c := 0; c < mv.numColumns; c++ {
			in := rb[r*mv.numColumns+c]
			if c+1 < mv.numColumns && in != rb[r*mv.numColumns+c+1] {
				edges = append(edges, regionEdge{r: r, c: c, vertical: true})
			}
			if r+1 < mv.numRows && in != rb[(r+1)*mv.numColumns+c] {
				edges = append(edges, regionEdge{r: r, c: c, vertical: false})
			}
		}
	}
	return edges
}

// regionBorderLayout positions the boundary lines onto cell edges, recomputing
// on resize. Cell slots are size/count (matching the value grid's slot pitch and
// the crosshair layout), and row 0 sits at the bottom, so Y is flipped.
type regionBorderLayout struct {
	mv      *MapViewer
	edges   []regionEdge
	lines   []*canvas.Line
	oldSize fyne.Size
}

func (l *regionBorderLayout) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (l *regionBorderLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	if size == l.oldSize {
		return
	}
	l.oldSize = size
	cw := size.Width / float32(l.mv.numColumns)
	ch := size.Height / float32(l.mv.numRows)
	for i, e := range l.edges {
		ln := l.lines[i]
		if e.vertical {
			x := float32(e.c+1) * cw
			ln.Position1 = fyne.NewPos(x, size.Height-float32(e.r+1)*ch)
			ln.Position2 = fyne.NewPos(x, size.Height-float32(e.r)*ch)
		} else {
			y := size.Height - float32(e.r+1)*ch
			ln.Position1 = fyne.NewPos(float32(e.c)*cw, y)
			ln.Position2 = fyne.NewPos(float32(e.c+1)*cw, y)
		}
		ln.Refresh()
	}
}

// clearSelectionVisual hides the highlight for every currently selected cell.
// Call it before mutating mv.selectedCells, then call drawSelectionVisual after.
func (mv *MapViewer) clearSelectionVisual() {
	for _, cell := range mv.selectedCells {
		if cell >= 0 && cell < len(mv.selectionRects) {
			mv.selectionRects[cell].Hide()
		}
	}
}

// drawSelectionVisual shows the highlight for every currently selected cell.
func (mv *MapViewer) drawSelectionVisual() {
	for _, cell := range mv.selectedCells {
		if cell >= 0 && cell < len(mv.selectionRects) {
			mv.selectionRects[cell].Show()
		}
	}
	// Show() only flips the Hidden flag. On a freshly opened window nothing has
	// dirtied the canvas yet, so the newly shown rects aren't painted until some
	// unrelated event (resize, button hover) forces a repaint. Refresh the
	// overlay container to repaint immediately. See handlePrimaryCtrlClick.
	canvas.Refresh(mv.selectionOverlay)
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
	if mv.graph != nil {
		if mv.numRows == 1 {
			mv.graph.SetCursor(xIdx)
		} else {
			mv.graph.SetCursor(yIdx)
		}
	}
	if mv.mesh != nil {
		mv.mesh.SetCursor(xIdx, yIdx)
	}
	if mv.cfg.CursorFollowCrosshair {
		mv.selectedX = int(math.Round(xIdx))
		mv.SelectedY = int(math.Round(yIdx))
		mv.updateCursor(true)
	}
	return nil
}

// stepSelected adjusts every selected cell by one ZPrecision step. sign is +1
// (incr) or -1 (decr). Same behaviour as the +/- keyboard shortcut.
func (mv *MapViewer) stepSelected(sign float64) {
	increment := sign * math.Pow(10, -float64(mv.cfg.ZPrecision))
	for _, cell := range mv.selectedCells {
		mv.cfg.ZData[cell] += increment
	}
	mv.updateCells()
	mv.Refresh()
}

func (mv *MapViewer) createStepButtons() *fyne.Container {
	decr := widget.NewButtonWithIcon("Decr", theme.ContentRemoveIcon(), func() { mv.stepSelected(-1) })
	incr := widget.NewButtonWithIcon("Incr", theme.ContentAddIcon(), func() { mv.stepSelected(1) })
	decr.Importance = widget.LowImportance
	incr.Importance = widget.LowImportance
	return container.NewGridWithColumns(2, decr, incr)
}

func (mv *MapViewer) createButtons() *fyne.Container {
	noButtons := len(mv.cfg.Buttons)
	if noButtons > 0 {
		buttonContainer := container.NewGridWithColumns(noButtons)
		for _, btn := range mv.cfg.Buttons {
			buttonContainer.Add(
				widget.NewButtonWithIcon(btn.Label, btn.Icon, btn.OnTapped),
			)
		}
		return buttonContainer
	} else {
		return container.NewWithoutLayout()
	}
}

/*
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
*/

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
