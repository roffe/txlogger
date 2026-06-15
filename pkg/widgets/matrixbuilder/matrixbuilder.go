// Package matrixbuilder provides a widget that learns a 2D map (matrix) from
// one or more log files. The widget loads the logs itself and merges their
// series, then builds the matrix from three selected series: one drives the X
// axis, one the Y axis and one supplies the Z value written into the cell the
// X/Y pair lands on. Every sample that maps to a cell is accumulated and the
// cell's final value is the average of all its hits. The resulting matrix is
// shown with a mapviewer (colored grid + 3D meshgrid), and both the axis
// breakpoints and the series can be edited or typed by hand.
package matrixbuilder

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	xlayout "fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

const (
	minAxis     = 1
	maxAxis     = 40
	defaultCols = 8
	defaultRows = 8

	// Tolerance is expressed as a percentage of a cell's half-spacing (the
	// distance from a breakpoint to the midpoint between it and its neighbor).
	// At 100% the whole nearest-neighbor region counts as a hit (the original
	// behaviour); lower values reject samples that fall near the cell edges,
	// keeping only those close to the breakpoint.
	minTolerance     = 1
	defaultTolerance = 100
)

var _ fyne.Widget = (*MatrixBuilder)(nil)

type MatrixBuilder struct {
	widget.BaseWidget

	// values holds every series merged across all loaded log files. All series
	// share the same length (nrecords); gaps are padded with NaN so samples
	// from different files stay row-aligned.
	values      map[string][]float64
	order       []string
	loadedFiles []string
	nrecords    int

	xSeries, ySeries, zSeries string

	cols, rows int
	xAxis      []float64
	yAxis      []float64
	zData      []float64

	// xTolerance/yTolerance gate how close (as a percentage of the cell's
	// half-spacing) a sample must be to its nearest breakpoint to count as a
	// Z-hit on that axis. A sample is mapped only if it passes on both axes.
	xTolerance, yTolerance float64

	// built becomes true after the first successful analysis; until then the
	// display area shows a placeholder instead of an all-zero grid.
	built bool

	// widgets
	colsLabel, rowsLabel   *widget.Label
	status                 *widget.Label
	logsLabel              *widget.Label
	xBox, yBox             *fyne.Container
	xEntries, yEntries     []*widget.Entry
	xSel, ySel, zSel       *widget.SelectEntry
	xTolSlider, yTolSlider *widget.Slider
	xTolLabel, yTolLabel   *widget.Label
	presetSelect           *widget.Select
	nameEntry              *widget.Entry
	display                *fyne.Container

	content fyne.CanvasObject
}

// Preset is the on-disk representation of a matrix builder configuration. It
// holds only settings (series, dimensions and axis breakpoints); the learned
// matrix values are never stored.
type Preset struct {
	XSeries string    `json:"x_series"`
	YSeries string    `json:"y_series"`
	ZSeries string    `json:"z_series"`
	Cols    int       `json:"cols"`
	Rows    int       `json:"rows"`
	XAxis   []float64 `json:"x_axis"`
	YAxis   []float64 `json:"y_axis"`
	// XTolerance/YTolerance are percentages (1..100). Omitted in older presets,
	// which decode to 0 and are treated as the default (no filtering) on load.
	XTolerance float64 `json:"x_tolerance,omitempty"`
	YTolerance float64 `json:"y_tolerance,omitempty"`
}

// New creates an empty MatrixBuilder. Log files are loaded from within the
// widget via a native file dialog.
func New() *MatrixBuilder {
	mb := &MatrixBuilder{
		values:     make(map[string][]float64),
		cols:       defaultCols,
		rows:       defaultRows,
		xTolerance: defaultTolerance,
		yTolerance: defaultTolerance,
	}
	mb.ExtendBaseWidget(mb)
	mb.xAxis = make([]float64, mb.cols)
	mb.yAxis = make([]float64, mb.rows)
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.buildUI()
	return mb
}

func (mb *MatrixBuilder) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(mb.content)
}

func (mb *MatrixBuilder) buildUI() {
	// SelectEntry lets the user pick a loaded series from the dropdown or type a
	// name manually.
	mb.xSel = widget.NewSelectEntry(mb.order)
	mb.xSel.OnChanged = func(s string) { mb.xSeries = s }
	mb.ySel = widget.NewSelectEntry(mb.order)
	mb.ySel.OnChanged = func(s string) { mb.ySeries = s }
	mb.zSel = widget.NewSelectEntry(mb.order)
	mb.zSel.OnChanged = func(s string) { mb.zSeries = s }
	mb.xSel.PlaceHolder = "X series"
	mb.ySel.PlaceHolder = "Y series"
	mb.zSel.PlaceHolder = "Z series"

	mb.colsLabel = widget.NewLabel(strconv.Itoa(mb.cols))
	mb.rowsLabel = widget.NewLabel(strconv.Itoa(mb.rows))
	mb.status = widget.NewLabel("")
	mb.status.Wrapping = fyne.TextWrapWord

	colsRow := container.NewBorder(nil, nil,
		widget.NewLabel("Columns (X)"),
		container.NewHBox(
			widget.NewButton("-", func() { mb.setCols(mb.cols - 1) }),
			mb.colsLabel,
			widget.NewButton("+", func() { mb.setCols(mb.cols + 1) }),
			widget.NewButton("Auto", func() { mb.autoFill(true) }),
		),
	)
	rowsRow := container.NewBorder(nil, nil,
		widget.NewLabel("Rows (Y)"),
		container.NewHBox(
			widget.NewButton("-", func() { mb.setRows(mb.rows - 1) }),
			mb.rowsLabel,
			widget.NewButton("+", func() { mb.setRows(mb.rows + 1) }),
			widget.NewButton("Auto", func() { mb.autoFill(false) }),
		),
	)

	buildBtn := widget.NewButtonWithIcon("Build matrix", theme.GridIcon(), func() {
		if err := mb.analyze(); err != nil {
			mb.status.SetText(err.Error())
			return
		}
		mb.rebuildDisplay()
	})
	buildBtn.Importance = widget.HighImportance

	mb.xBox = container.NewVBox()
	mb.yBox = container.NewHBox()
	mb.rebuildAxisEntries()

	controls := container.NewVBox(
		// widget.NewLabelWithStyle("Series", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		colsRow,
		rowsRow,
		widget.NewSeparator(),
		labeled("X", mb.xSel),
		labeled("Y", mb.ySel),
		labeled("Z", mb.zSel),
		widget.NewSeparator(),
		mb.buildToleranceSection(),
		widget.NewSeparator(),
		buildBtn,
		widget.NewSeparator(),
		mb.status,
		widget.NewSeparator(),
		xlayout.NewSpacer(),
		mb.buildPresetSection(),
	)

	left := container.NewVScroll(controls)
	left.SetMinSize(fyne.NewSize(240, 0))

	right := container.NewVScroll(
		mb.buildLogSection(),
	)

	mb.display = container.NewStack(mb.placeholder())

	// The Y scale runs along the vertical axis of the map; its editor sits as a
	// horizontal strip beneath the display.
	yPanel := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Y axis values", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		container.NewHScroll(mb.yBox),
	)

	mb.content = container.NewBorder(
		nil,
		nil,
		right,
		left,
		container.NewBorder(nil, yPanel, container.NewVBox(widget.NewLabelWithStyle("X axis values", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			mb.xBox), nil, mb.display),
	)
}

func (mb *MatrixBuilder) placeholder() fyne.CanvasObject {
	return container.NewCenter(widget.NewLabel("Select X, Y and Z series, then click \"Build matrix\""))
}

// buildToleranceSection builds the per-axis Z-hit tolerance sliders. Each
// slider sets the maximum distance (as a percentage of the cell's half-spacing)
// a sample may sit from its nearest breakpoint and still count as a hit.
func (mb *MatrixBuilder) buildToleranceSection() fyne.CanvasObject {
	mb.xTolLabel = widget.NewLabel(tolText(mb.xTolerance))
	mb.yTolLabel = widget.NewLabel(tolText(mb.yTolerance))
	mb.xTolSlider = mb.newToleranceSlider(true)
	mb.yTolSlider = mb.newToleranceSlider(false)

	return container.NewVBox(
		widget.NewLabelWithStyle("Z-hit tolerance", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, widget.NewLabel("X"), layout.NewFixedWidth(44, mb.xTolLabel), mb.xTolSlider),
		container.NewBorder(nil, nil, widget.NewLabel("Y"), layout.NewFixedWidth(44, mb.yTolLabel), mb.yTolSlider),
	)
}

// newToleranceSlider builds a 1..100% slider bound to the X or Y tolerance.
// While dragging it only updates the value label; on release it re-runs the
// analysis (if a matrix is already built) so the user sees the effect live.
func (mb *MatrixBuilder) newToleranceSlider(isX bool) *widget.Slider {
	cur := mb.yTolerance
	if isX {
		cur = mb.xTolerance
	}
	s := widget.NewSlider(minTolerance, 100)
	s.Step = 1
	s.SetValue(cur)
	s.OnChanged = func(f float64) {
		if isX {
			mb.xTolerance = f
			mb.xTolLabel.SetText(tolText(f))
		} else {
			mb.yTolerance = f
			mb.yTolLabel.SetText(tolText(f))
		}
	}
	s.OnChangeEnded = func(float64) {
		if !mb.built {
			return
		}
		if err := mb.analyze(); err != nil {
			mb.status.SetText(err.Error())
			return
		}
		mb.rebuildDisplay()
	}
	return s
}

// rebuildAxisEntries regenerates the editable entry fields for the current
// column/row counts, seeding them from the current axis values.
func (mb *MatrixBuilder) rebuildAxisEntries() {
	mb.xEntries = make([]*widget.Entry, mb.cols)
	mb.xBox.Objects = mb.xBox.Objects[:0]
	for i := 0; i < mb.cols; i++ {
		mb.xBox.Add(mb.makeAxisEntry(true, i))
	}
	mb.xBox.Refresh()

	mb.yEntries = make([]*widget.Entry, mb.rows)
	mb.yBox.Objects = mb.yBox.Objects[:0]
	for i := 0; i < mb.rows; i++ {
		mb.yBox.Add(mb.makeAxisEntry(false, i))
	}
	mb.yBox.Refresh()
}

func (mb *MatrixBuilder) makeAxisEntry(isX bool, idx int) fyne.CanvasObject {
	axis := mb.xAxis
	prefix := "X"
	if !isX {
		axis = mb.yAxis
		prefix = "Y"
	}
	e := widget.NewEntry()
	e.SetText(formatFloat(axis[idx]))
	e.OnChanged = func(s string) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return
		}
		if isX {
			mb.xAxis[idx] = v
		} else {
			mb.yAxis[idx] = v
		}
	}
	// Apply (relabel the displayed map) when the user commits a value.
	e.OnSubmitted = func(string) {
		if mb.built {
			mb.rebuildDisplay()
		}
	}
	if isX {
		mb.xEntries[idx] = e
		// X breakpoints stack vertically in the side panel: label beside entry.
		return container.NewBorder(nil, nil, widget.NewLabel(prefix+strconv.Itoa(idx)), nil, e)
	}
	mb.yEntries[idx] = e
	// Y breakpoints run horizontally along the bottom: label above a
	// fixed-width entry so the strip stays compact.
	label := widget.NewLabelWithStyle(prefix+strconv.Itoa(idx), fyne.TextAlignCenter, fyne.TextStyle{})
	return container.NewVBox(label, layout.NewFixedWidth(64, e))
}

func (mb *MatrixBuilder) setCols(n int) {
	n = clamp(n, minAxis, maxAxis)
	if n == mb.cols {
		return
	}
	mb.xAxis = resizeAxis(mb.xAxis, n)
	mb.cols = n
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.built = false
	mb.colsLabel.SetText(strconv.Itoa(n))
	mb.rebuildAxisEntries()
	mb.rebuildDisplay()
}

func (mb *MatrixBuilder) setRows(n int) {
	n = clamp(n, minAxis, maxAxis)
	if n == mb.rows {
		return
	}
	mb.yAxis = resizeAxis(mb.yAxis, n)
	mb.rows = n
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.built = false
	mb.rowsLabel.SetText(strconv.Itoa(n))
	mb.rebuildAxisEntries()
	mb.rebuildDisplay()
}

// autoFill spreads the selected series' min..max evenly across the axis
// breakpoints. isX selects the X axis, otherwise the Y axis.
func (mb *MatrixBuilder) autoFill(isX bool) {
	series := mb.ySeries
	axis := mb.yAxis
	if isX {
		series = mb.xSeries
		axis = mb.xAxis
	}
	data, ok := mb.values[series]
	if !ok || len(data) == 0 {
		return
	}
	lo, hi := minMax(data)
	n := len(axis)
	for i := 0; i < n; i++ {
		if n == 1 {
			axis[i] = lo
			continue
		}
		axis[i] = lo + (hi-lo)*float64(i)/float64(n-1)
	}
	mb.syncAxisEntries()
	if mb.built {
		mb.rebuildDisplay()
	}
}

// syncAxisEntries pushes the current axis values back into the entry widgets.
// The entry/axis lengths track each other via rebuildAxisEntries, but the
// guard keeps a transient desync from panicking the UI thread.
func (mb *MatrixBuilder) syncAxisEntries() {
	for i, e := range mb.xEntries {
		if i >= len(mb.xAxis) {
			break
		}
		e.SetText(formatFloat(mb.xAxis[i]))
	}
	for i, e := range mb.yEntries {
		if i >= len(mb.yAxis) {
			break
		}
		e.SetText(formatFloat(mb.yAxis[i]))
	}
}

// analyze walks the log and learns the matrix: each sample is assigned to the
// nearest cell and the cell's value becomes the average of its hits.
func (mb *MatrixBuilder) analyze() error {
	if len(mb.values) == 0 {
		return fmt.Errorf("load a log file first")
	}
	if mb.xSeries == "" || mb.ySeries == "" || mb.zSeries == "" {
		return fmt.Errorf("select X, Y and Z series first")
	}
	xv, okX := mb.values[mb.xSeries]
	yv, okY := mb.values[mb.ySeries]
	zv, okZ := mb.values[mb.zSeries]
	if !okX || !okY || !okZ {
		return fmt.Errorf("selected series not found in the loaded logs")
	}
	n := min(len(xv), min(len(yv), len(zv)))
	if n == 0 {
		return fmt.Errorf("selected series contain no samples")
	}

	// Sort the axes ascending so the learned map reads like a normal table.
	sort.Float64s(mb.xAxis)
	sort.Float64s(mb.yAxis)
	mb.syncAxisEntries()

	size := mb.cols * mb.rows
	sum := make([]float64, size)
	cnt := make([]int, size)
	used := 0
	skipped := 0
	for i := 0; i < n; i++ {
		// Skip rows where any of the three series is missing (NaN padding from
		// merging logs with differing channel sets).
		if math.IsNaN(xv[i]) || math.IsNaN(yv[i]) || math.IsNaN(zv[i]) {
			continue
		}
		c := nearestIndex(mb.xAxis, xv[i])
		r := nearestIndex(mb.yAxis, yv[i])
		// Reject samples sitting too far from their nearest breakpoint on
		// either axis, so only values close to a cell count as a Z-hit.
		if !withinTolerance(mb.xAxis, c, xv[i], mb.xTolerance) ||
			!withinTolerance(mb.yAxis, r, yv[i], mb.yTolerance) {
			skipped++
			continue
		}
		idx := r*mb.cols + c
		sum[idx] += zv[i]
		cnt[idx]++
		used++
	}

	mb.zData = make([]float64, size)
	filled := 0
	for i := range sum {
		if cnt[i] > 0 {
			mb.zData[i] = sum[i] / float64(cnt[i])
			filled++
		}
	}
	mb.built = true
	msg := fmt.Sprintf("Mapped %d samples, %d/%d cells filled", used, filled, size)
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d skipped by tolerance)", skipped)
	}
	mb.status.SetText(msg)
	return nil
}

// rebuildDisplay swaps a freshly built mapviewer (grid + 3D mesh) into the
// display area, reflecting the current axes and learned Z data. Before the
// first build it shows the placeholder instead.
func (mb *MatrixBuilder) rebuildDisplay() {
	if !mb.built {
		mb.display.Objects = []fyne.CanvasObject{mb.placeholder()}
		mb.display.Refresh()
		return
	}

	noop := func([]float64) {}
	mv, err := mapviewer.New(&mapviewer.Config{
		Name:           mb.zSeries,
		XData:          mb.xAxis,
		YData:          mb.yAxis,
		ZData:          mb.zData,
		XPrecision:     precisionFor(mb.xAxis),
		YPrecision:     precisionFor(mb.yAxis),
		ZPrecision:     precisionFor(mb.zData),
		XLabel:         mb.xSeries,
		YLabel:         mb.ySeries,
		ZLabel:         mb.zSeries,
		MeshView:       true,
		Editable:       true,
		ColorblindMode: colors.ModeNormal,
		// The matrix is in-memory only; editing cells just mutates zData.
		SaveECUFunc:  noop,
		OnUpdateCell: func(int, []float64) {},
	})
	if err != nil {
		mb.display.Objects = []fyne.CanvasObject{container.NewCenter(widget.NewLabel(err.Error()))}
		mb.display.Refresh()
		return
	}
	mb.display.Objects = []fyne.CanvasObject{mv}
	mb.display.Refresh()
}

// --- log files ---

func (mb *MatrixBuilder) buildLogSection() fyne.CanvasObject {
	mb.logsLabel = widget.NewLabel("No log files loaded")
	mb.logsLabel.Wrapping = fyne.TextWrapWord

	addBtn := widget.NewButtonWithIcon("Add log files", theme.FolderOpenIcon(), mb.openLogDialog)
	clearBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), mb.clearLogs)

	return container.NewVBox(
		// widget.NewLabelWithStyle("Log files", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, addBtn, clearBtn),
		mb.logsLabel,
	)
}

// openLogDialog shows the native multi-file picker and loads the chosen logs.
// Parsing runs off the UI goroutine behind a progress modal, then the parsed
// series are merged on the UI goroutine.
func (mb *MatrixBuilder) openLogDialog() {
	widgets.SelectFiles(func(readers []fyne.URIReadCloser) {
		c := fyne.CurrentApp().Driver().CanvasForObject(mb)
		if c == nil {
			if wins := fyne.CurrentApp().Driver().AllWindows(); len(wins) > 0 {
				c = wins[0].Canvas()
			}
		}
		var pm *progressmodal.ProgressModal
		if c != nil {
			pm = progressmodal.New(c, fmt.Sprintf("Parsing %d log file(s)...", len(readers)))
			pm.Show()
		}

		go func() {
			type parsed struct {
				name  string
				local map[string][]float64
				n     int
			}
			var ok []parsed
			var failed int
			for _, r := range readers {
				name := r.URI().Name()
				local, n, err := parseLog(name, r)
				r.Close()
				if err != nil {
					log.Println("matrixbuilder:", err)
					failed++
					continue
				}
				ok = append(ok, parsed{name, local, n})
			}

			fyne.Do(func() {
				if pm != nil {
					pm.Hide()
				}
				for _, p := range ok {
					mb.mergeLog(p.name, p.local, p.n)
				}
				mb.rebuildOrder()
				mb.refreshSeriesOptions()
				mb.refreshLogList()
				if failed > 0 {
					mb.status.SetText(fmt.Sprintf("Loaded %d file(s), %d failed", len(ok), failed))
				} else {
					mb.status.SetText(fmt.Sprintf("Loaded %d file(s), %d records total", len(ok), mb.nrecords))
				}
			})
		}()
	}, "logfile", "t5l", "t7l", "t8l", "csv", "bpl")
}

// parseLog reads a single log file into a row-aligned series map, padding gaps
// with NaN. It touches no shared state, so it is safe to call off the UI
// goroutine.
func parseLog(name string, r io.Reader) (map[string][]float64, int, error) {
	lf, err := logfile.Open(name, r)
	if err != nil {
		return nil, 0, err
	}
	defer lf.Close()

	local := make(map[string][]float64)
	n := 0
	for {
		rec := lf.Next()
		if rec.EOF {
			break
		}
		for k, v := range rec.Values {
			if k == "Pgm_status" {
				continue
			}
			arr := local[k]
			for len(arr) < n { // back-fill records before this key first appeared
				arr = append(arr, math.NaN())
			}
			local[k] = append(arr, v)
		}
		n++
		for k, arr := range local { // forward-fill keys missing from this record
			for len(arr) < n {
				arr = append(arr, math.NaN())
			}
			local[k] = arr
		}
	}
	if n == 0 {
		return nil, 0, fmt.Errorf("%s contains no records", name)
	}
	return local, n, nil
}

// mergeLog merges a parsed log into the global series set, padding so all
// series stay row-aligned. Must run on the UI goroutine.
func (mb *MatrixBuilder) mergeLog(name string, local map[string][]float64, n int) {
	base := mb.nrecords
	for k, arr := range local {
		cur, ok := mb.values[k]
		if !ok {
			cur = nanSlice(base)
		}
		mb.values[k] = append(cur, arr...)
	}
	for k, cur := range mb.values {
		if _, ok := local[k]; !ok {
			mb.values[k] = append(cur, nanSlice(n)...)
		}
	}
	mb.nrecords = base + n
	mb.loadedFiles = append(mb.loadedFiles, name)
}

func (mb *MatrixBuilder) clearLogs() {
	mb.values = make(map[string][]float64)
	mb.order = nil
	mb.loadedFiles = nil
	mb.nrecords = 0
	mb.built = false
	mb.refreshSeriesOptions()
	mb.refreshLogList()
	mb.rebuildDisplay()
	mb.status.SetText("Cleared loaded logs")
}

// rebuildOrder refreshes the sorted list of available series names.
func (mb *MatrixBuilder) rebuildOrder() {
	mb.order = make([]string, 0, len(mb.values))
	for k := range mb.values {
		mb.order = append(mb.order, k)
	}
	sort.Slice(mb.order, func(i, j int) bool {
		return strings.ToLower(mb.order[i]) < strings.ToLower(mb.order[j])
	})
}

func (mb *MatrixBuilder) refreshSeriesOptions() {
	mb.xSel.SetOptions(mb.order)
	mb.ySel.SetOptions(mb.order)
	mb.zSel.SetOptions(mb.order)
}

func (mb *MatrixBuilder) refreshLogList() {
	if len(mb.loadedFiles) == 0 {
		mb.logsLabel.SetText("No log files loaded")
		return
	}
	mb.logsLabel.SetText(fmt.Sprintf("%d file(s), %d records:\n%s",
		len(mb.loadedFiles), mb.nrecords, strings.Join(mb.loadedFiles, "\n")))
}

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// --- presets ---

func (mb *MatrixBuilder) buildPresetSection() fyne.CanvasObject {
	mb.presetSelect = widget.NewSelect(mb.listPresets(), func(name string) {
		if name == "" {
			return
		}
		if err := mb.loadPreset(name); err != nil {
			mb.status.SetText(err.Error())
		}
	})
	mb.presetSelect.PlaceHolder = "Load preset"

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), mb.refreshPresets)

	mb.nameEntry = widget.NewEntry()
	mb.nameEntry.SetPlaceHolder("preset name")
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		name := strings.TrimSpace(mb.nameEntry.Text)
		if name == "" {
			mb.status.SetText("enter a preset name to save")
			return
		}
		saved, err := mb.savePreset(name)
		if err != nil {
			mb.status.SetText(err.Error())
			return
		}
		mb.refreshPresets()
		// Reflect the saved name in the picker without re-triggering a load.
		mb.presetSelect.Selected = saved
		mb.presetSelect.Refresh()
		mb.status.SetText("Saved preset " + saved)
	})

	return container.NewVBox(
		// widget.NewLabelWithStyle("Presets", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, refreshBtn, mb.presetSelect),
		container.NewBorder(nil, nil, nil, saveBtn, mb.nameEntry),
	)
}

// listPresets returns the names (without extension) of the saved presets.
func (mb *MatrixBuilder) listPresets() []string {
	path, err := common.GetMatrixBuilderPath()
	if err != nil {
		return nil
	}
	files, err := common.ListFilesInPathByExtension(path, ".json")
	if err != nil {
		return nil
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = strings.TrimSuffix(f, ".json")
	}
	return names
}

func (mb *MatrixBuilder) refreshPresets() {
	mb.presetSelect.Options = mb.listPresets()
	mb.presetSelect.Refresh()
}

// savePreset writes the current configuration (and learned matrix, if any) to
// name.json in the matrix builder directory. It returns the stored preset name
// (without extension), which may differ from name after sanitization.
func (mb *MatrixBuilder) savePreset(name string) (string, error) {
	path, err := common.GetMatrixBuilderPath()
	if err != nil {
		return "", err
	}
	p := Preset{
		XSeries:    mb.xSeries,
		YSeries:    mb.ySeries,
		ZSeries:    mb.zSeries,
		Cols:       mb.cols,
		Rows:       mb.rows,
		XAxis:      mb.xAxis,
		YAxis:      mb.yAxis,
		XTolerance: mb.xTolerance,
		YTolerance: mb.yTolerance,
	}
	b, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return "", err
	}
	stored := common.SanitizeFilename(name + ".json")
	if err := os.WriteFile(filepath.Join(path, stored), b, 0o644); err != nil {
		return "", err
	}
	return strings.TrimSuffix(stored, ".json"), nil
}

// loadPreset reads name.json and applies it to the builder.
func (mb *MatrixBuilder) loadPreset(name string) error {
	path, err := common.GetMatrixBuilderPath()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(path, common.SanitizeFilename(name+".json")))
	if err != nil {
		return err
	}
	var p Preset
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("failed to decode preset: %w", err)
	}
	mb.applyPreset(&p)
	mb.status.SetText("Loaded preset " + name)
	return nil
}

// applyPreset replaces the current state with the preset's, rebuilding the
// editor and display.
func (mb *MatrixBuilder) applyPreset(p *Preset) {
	mb.cols = clamp(p.Cols, minAxis, maxAxis)
	mb.rows = clamp(p.Rows, minAxis, maxAxis)
	mb.xAxis = resizeAxis(p.XAxis, mb.cols)
	mb.yAxis = resizeAxis(p.YAxis, mb.rows)
	// A preset carries only settings, so the matrix must be rebuilt after load.
	// Clear built first: Slider.SetValue below fires OnChangeEnded, and we must
	// not let it re-run analyze() against the half-applied state.
	mb.zData = make([]float64, mb.cols*mb.rows)
	mb.built = false

	// Older presets predate the tolerance fields and decode to 0; fall back to
	// the default (no filtering) rather than rejecting every sample.
	mb.xTolerance = toleranceOrDefault(p.XTolerance)
	mb.yTolerance = toleranceOrDefault(p.YTolerance)
	mb.xTolSlider.SetValue(mb.xTolerance)
	mb.yTolSlider.SetValue(mb.yTolerance)
	mb.xTolLabel.SetText(tolText(mb.xTolerance))
	mb.yTolLabel.SetText(tolText(mb.yTolerance))

	// SetText fires OnChanged, which just records the series name (no auto-fill),
	// so this is safe and keeps mb.xSeries/etc. in sync.
	mb.xSel.SetText(p.XSeries)
	mb.ySel.SetText(p.YSeries)
	mb.zSel.SetText(p.ZSeries)

	mb.colsLabel.SetText(strconv.Itoa(mb.cols))
	mb.rowsLabel.SetText(strconv.Itoa(mb.rows))
	mb.rebuildAxisEntries()
	mb.rebuildDisplay()
}

// --- helpers ---

func labeled(label string, obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, obj)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// resizeAxis grows or shrinks an axis to length n, preserving existing values
// and padding new slots with the previous value (or zero for an empty axis).
func resizeAxis(old []float64, n int) []float64 {
	out := make([]float64, n)
	copy(out, old)
	for i := len(old); i < n; i++ {
		if i > 0 {
			out[i] = out[i-1]
		}
	}
	return out
}

// withinTolerance reports whether v is close enough to breakpoint c on a sorted
// axis to count as a hit. tolPct is the allowed distance expressed as a
// percentage of the half-spacing to the neighbouring breakpoint on v's side, so
// 100% accepts the entire nearest-neighbor region and lower values reject
// samples sitting near the cell boundary. Edge cells use their inner spacing as
// the reference, so a sample far beyond the axis range is rejected too.
func withinTolerance(axis []float64, c int, v, tolPct float64) bool {
	if tolPct >= 100 || len(axis) < 2 {
		return true
	}
	bp := axis[c]
	var spacing float64
	if v >= bp { // neighbour on the high side, falling back to the low side
		switch {
		case c+1 < len(axis):
			spacing = axis[c+1] - bp
		case c-1 >= 0:
			spacing = bp - axis[c-1]
		}
	} else { // neighbour on the low side, falling back to the high side
		switch {
		case c-1 >= 0:
			spacing = bp - axis[c-1]
		case c+1 < len(axis):
			spacing = axis[c+1] - bp
		}
	}
	if spacing <= 0 { // duplicate/degenerate breakpoints: nothing to gate on
		return true
	}
	maxDist := tolPct / 100.0 * (spacing / 2.0)
	return math.Abs(v-bp) <= maxDist
}

// toleranceOrDefault clamps a stored tolerance into the valid range, mapping the
// zero value (older presets without the field) to the default.
func toleranceOrDefault(v float64) float64 {
	if v < minTolerance {
		return defaultTolerance
	}
	if v > 100 {
		return 100
	}
	return v
}

func tolText(v float64) string {
	return strconv.Itoa(int(v)) + "%"
}

// nearestIndex returns the index of the axis breakpoint closest to v.
func nearestIndex(axis []float64, v float64) int {
	best := 0
	bestDist := math.Abs(axis[0] - v)
	for i := 1; i < len(axis); i++ {
		d := math.Abs(axis[i] - v)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func minMax(data []float64) (float64, float64) {
	lo, hi := data[0], data[0]
	for _, v := range data[1:] {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// precisionFor picks a sensible decimal precision: 0 for all-integer data,
// otherwise more decimals for small-magnitude values.
func precisionFor(data []float64) int {
	allInt := true
	maxAbs := 0.0
	for _, v := range data {
		if v != math.Trunc(v) {
			allInt = false
		}
		if a := math.Abs(v); a > maxAbs {
			maxAbs = a
		}
	}
	if allInt {
		return 0
	}
	if maxAbs < 10 {
		return 3
	}
	return 2
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
