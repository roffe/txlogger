package settings

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/debug"
)

var builtInPresets = map[string]adScannerPreset{
	"[Builtin] AEM Uego X-Series T7": {
		ECU: "T7",
		Y:   []int{102, 153, 205, 256, 307, 358, 409, 460, 512, 563, 614, 665, 716, 767, 818, 870, 921},
		Z:   []float64{0.58, 0.62, 0.66, 0.7, 0.74, 0.78, 0.82, 0.86, 0.9, 0.94, 0.99, 1.03, 1.07, 1.11, 1.15, 1.19, 1.23},
	},

	"[Builtin] AEM Uego X-Series T5": {
		ECU: "T5",
		Y:   []int{26, 38, 51, 64, 77, 89, 102, 115, 128, 140, 153, 166, 179, 191, 204, 217, 230},
		Z:   []float64{0.58, 0.62, 0.66, 0.7, 0.74, 0.78, 0.82, 0.86, 0.9, 0.94, 0.99, 1.03, 1.07, 1.11, 1.15, 1.19, 1.23},
	},
}

type mapRow struct {
	y  int
	z  float64
	ye *widget.Entry
	ze *widget.Entry
	vo *widget.Entry
	rm *widget.Button
	hb *fyne.Container

	w *WBLEditor
}

type WBLEditor struct {
	widget.BaseWidget
	rows         []*mapRow
	rowsBox      *fyne.Container
	ecuSelect    *widget.Select
	presetSelect *widget.Select
	graph        *graphView
	adresolution int
	// lastEcu      string
}

func NewWBLEditor(yAxis []int, zValues []float64) *WBLEditor {
	m := &WBLEditor{}
	n := min(len(yAxis), len(zValues))
	for i := range n {
		m.rows = append(m.rows, &mapRow{y: yAxis[i], z: zValues[i]})
	}
	m.ExtendBaseWidget(m)

	switch fyne.CurrentApp().Preferences().String(prefsLastADScannerECU) {
	case "T5":
		m.adresolution = 255
	case "T7", "T8":
		m.adresolution = 1023
	default:
		m.adresolution = 1023
	}

	return m
}

func (m *WBLEditor) YAxis() []int {
	out := make([]int, len(m.rows))
	for i, r := range m.rows {
		out[i] = r.y
	}
	return out
}

func (m *WBLEditor) ZValues() []float64 {
	out := make([]float64, len(m.rows))
	for i, r := range m.rows {
		out[i] = r.z
	}
	return out
}

func (m *WBLEditor) buildRow(r *mapRow) {
	r.w = m

	r.vo = widget.NewEntry()
	r.vo.SetText(fmt.Sprintf("%.2f", m.voltFromY(r.y)))
	r.vo.OnChanged = func(s string) {
		s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return
		}
		r.y = clampInt(m.yFromVolt(v), yMin, yMax)
		r.ye.Text = strconv.Itoa(r.y)
		r.ye.Refresh()
		m.refreshGraph()
		m.save()
	}

	r.ye = widget.NewEntry()
	r.ye.SetText(strconv.Itoa(r.y))
	r.ye.OnChanged = func(s string) {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return
		}
		r.y = clampInt(v, yMin, yMax)
		r.vo.Text = fmt.Sprintf("%.2f", m.voltFromY(r.y))
		r.vo.Refresh()
		m.refreshGraph()
		m.save()
	}

	r.ze = widget.NewEntry()
	r.ze.SetText(strconv.FormatFloat(r.z, 'f', 2, 64))
	r.ze.OnChanged = func(s string) {
		s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return
		}
		r.z = clampFloat(v, zMin, zMax)
		m.refreshGraph()
		m.save()
	}

	r.rm = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		m.removeRow(r)
	})

	r.hb = container.NewBorder(nil, nil, nil, r.rm,
		container.NewGridWithColumns(3, r.ye, r.vo, r.ze),
	)
}

func (m *WBLEditor) addRow() {
	var y int
	var z float64
	if n := len(m.rows); n > 0 {
		y = m.rows[n-1].y
		z = m.rows[n-1].z
	}
	r := &mapRow{y: y, z: z}
	m.buildRow(r)
	m.rows = append(m.rows, r)
	if m.rowsBox != nil {
		m.rowsBox.Add(r.hb)
		m.rowsBox.Refresh()
	}
	m.refreshGraph()
}

func (m *WBLEditor) removeRow(r *mapRow) {
	for i, rr := range m.rows {
		if rr == r {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			break
		}
	}
	if m.rowsBox != nil {
		m.rowsBox.Remove(r.hb)
		m.rowsBox.Refresh()
	}
	m.refreshGraph()
}

func (m *WBLEditor) refreshGraph() {
	if m.graph != nil {
		m.graph.Refresh()
	}
}

func (m *WBLEditor) save() {
	fyne.CurrentApp().Preferences().SetIntList(prefsWBLSupportPoints, m.YAxis())
	fyne.CurrentApp().Preferences().SetFloatList(prefsWBLLambdaValues, m.ZValues())
}

// updateRowEntries writes a row's current y/z to its entries without
// triggering the OnChanged callbacks (avoids feedback loops during drag).
func (m *WBLEditor) updateRowEntries(r *mapRow) {
	if r.ye != nil {
		r.ye.Text = strconv.Itoa(r.y)
		r.ye.Refresh()
	}
	if r.ze != nil {
		r.ze.Text = strconv.FormatFloat(r.z, 'f', 2, 64)
		r.ze.Refresh()
	}
	if r.vo != nil {
		r.vo.Text = fmt.Sprintf("%.2f", m.voltFromY(r.y))
		r.vo.Refresh()
	}
}

func (m *WBLEditor) voltFromY(y int) float64 {
	if m.adresolution <= 0 {
		return 0
	}
	return 5.0 * float64(y) / float64(m.adresolution)
}

func (m *WBLEditor) yFromVolt(v float64) int {
	return int(v*float64(m.adresolution)/5.0 + 0.5)
}

func (m *WBLEditor) CreateRenderer() fyne.WidgetRenderer {
	m.rowsBox = container.NewVBox()
	for _, r := range m.rows {
		m.buildRow(r)
		m.rowsBox.Add(r.hb)
	}

	addBtn := widget.NewButtonWithIcon("Add Value", theme.ContentAddIcon(), m.addRow)

	header := container.NewGridWithColumns(3,
		widget.NewLabelWithStyle("Y (AD value)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("V (volt)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Z (lambda)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	)

	left := container.NewBorder(header, addBtn, nil, nil,
		container.NewVScroll(m.rowsBox),
	)

	m.graph = newGraphView(m)

	m.presetSelect = widget.NewSelect(m.listPresets(), m.loadPreset)

	lastPreset := fyne.CurrentApp().Preferences().String(prefsLastADScannerPreset)
	if lastPreset != "" {
		m.presetSelect.Selected = lastPreset
	}

	m.ecuSelect = widget.NewSelect([]string{"T5", "T7", "T8"}, func(s string) {
		switch s {
		case "T5":
			m.adresolution = 255
		case "T7", "T8":
			m.adresolution = 1023
		}
		fyne.CurrentApp().Preferences().SetString(prefsLastADScannerECU, s)
		for _, r := range m.rows {
			if r.vo != nil {
				r.vo.Text = fmt.Sprintf("%.2f", m.voltFromY(r.y))
				r.vo.Refresh()
			}
		}
	})
	m.ecuSelect.Selected = fyne.CurrentApp().Preferences().StringWithFallback(prefsLastADScannerECU, "T7")

	top := container.NewBorder(
		nil,
		nil,
		m.ecuSelect,
		container.NewHBox(
			widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				if m.presetSelect.Selected == "" {
					return
				}
				m.deletePreset(m.presetSelect.Selected)
				m.refreshPresets()
			}),
			widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
				presets := m.listPresets()
				m.presetSelect.Options = presets
				m.presetSelect.Refresh()
			}),
		),
		m.presetSelect,
	)
	bottom := widget.NewButton("Save", m.savePreset)

	view := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		container.NewHSplit(
			left,
			m.graph,
		),
	)
	return widget.NewSimpleRenderer(view)
}

func (m *WBLEditor) refreshPresets() {
	m.presetSelect.Options = m.listPresets()
	m.presetSelect.Refresh()
}

func (m *WBLEditor) listPresets() []string {
	dir, err := common.GetADScannerPath() // ensure dir exists
	if err != nil {
		return []string{}
	}
	files, err := common.ListFilesInPathByExtension(dir, ".json")
	if err != nil {
		return []string{}
	}
	presets := make([]string, len(files))
	for i, f := range files {
		presets[i] = strings.TrimSuffix(f, ".json")
	}

	for name := range builtInPresets {
		presets = append(presets, name)
	}

	// sort presets case insensitively
	sort.SliceStable(presets, func(i, j int) bool {
		return strings.ToLower(presets[i]) < strings.ToLower(presets[j])
	})

	return presets
}

type adScannerPreset struct {
	ECU string    `json:"ecu"`
	Y   []int     `json:"y"`
	Z   []float64 `json:"z"`
}

func (m *WBLEditor) savePreset() {
	name := widget.NewEntry()

	items := []*widget.FormItem{
		widget.NewFormItem("", name),
	}

	saveFunc := func(b bool) {
		if !b {
			return
		}
		layoutPath, err := common.GetADScannerPath()
		if err != nil {
			debug.Log(err.Error())
			return
		}
		fname := filepath.Join(layoutPath, name.Text+".json")
		var preset adScannerPreset

		preset.ECU = m.ecuSelect.Selected
		preset.Y = m.YAxis()
		preset.Z = m.ZValues()

		f, err := os.Create(fname)
		if err != nil {
			debug.Log(err.Error())
			return
		}
		defer f.Close()

		encoder := json.NewEncoder(f)
		// encoder.SetIndent("", "  ")
		if err := encoder.Encode(preset); err != nil {
			debug.Log(err.Error())
			return
		}
		m.refreshPresets()
		m.presetSelect.Selected = name.Text
		m.presetSelect.Refresh()
	}

	dialog.NewForm("Create new preset", "Save", "Cancel", items, saveFunc, fyne.CurrentApp().Driver().AllWindows()[0]).Show()
}

func (m *WBLEditor) loadPreset(name string) {
	if name == "" {
		return
	}
	debug.Log("loading AD preset: " + name)

	if preset, ok := builtInPresets[name]; ok {
		m.rows = nil
		for i := range preset.Y {
			r := &mapRow{y: preset.Y[i], z: preset.Z[i]}
			m.buildRow(r)
			m.rows = append(m.rows, r)
		}
		if m.rowsBox != nil {
			m.rowsBox.Objects = nil
			for _, r := range m.rows {
				m.rowsBox.Add(r.hb)
			}
			m.rowsBox.Refresh()
		}
		m.refreshGraph()

		m.ecuSelect.SetSelected(preset.ECU)

		fyne.CurrentApp().Preferences().SetString(prefsLastADScannerPreset, name)
		m.save()
		return
	}

	layoutPath, err := common.GetADScannerPath()
	if err != nil {
		debug.Log(err.Error())
		return
	}
	data, err := os.ReadFile(filepath.Join(layoutPath, name+".json"))
	if err != nil {
		debug.Log(err.Error())
		return
	}
	var preset adScannerPreset
	if err := json.Unmarshal(data, &preset); err != nil {
		debug.Log(err.Error())
		return
	}
	if len(preset.Y) != len(preset.Z) {
		debug.Log("invalid preset: y and z length mismatch")
		return
	}

	m.rows = nil
	for i := range preset.Y {
		r := &mapRow{y: preset.Y[i], z: preset.Z[i]}
		m.buildRow(r)
		m.rows = append(m.rows, r)
	}
	if m.rowsBox != nil {
		m.rowsBox.Objects = nil
		for _, r := range m.rows {
			m.rowsBox.Add(r.hb)
		}
		m.rowsBox.Refresh()
	}
	m.refreshGraph()
	fyne.CurrentApp().Preferences().SetString(prefsLastADScannerPreset, name)
	m.save()
}

func (m *WBLEditor) deletePreset(name string) {
	layoutPath, err := common.GetADScannerPath()
	if err != nil {
		fyne.LogError("Could not get layout path", err)
		return
	}
	err = os.Remove(filepath.Join(layoutPath, name+".json"))
	if err != nil {
		fyne.LogError("Could not delete preset file", err)
		return
	}
}

// --- graph view (native fyne primitives) -----------------------------------

var (
	bgColor    = color.NRGBA{R: 24, G: 24, B: 28, A: 255}
	gridColor  = color.NRGBA{R: 60, G: 60, B: 68, A: 255}
	axisColor  = color.NRGBA{R: 140, G: 140, B: 150, A: 255}
	lineColor  = color.NRGBA{R: 80, G: 200, B: 120, A: 255}
	pointColor = color.NRGBA{R: 240, G: 200, B: 60, A: 255}
	pointEdge  = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
)

const (
	graphMargin = 16
	pointSize   = 12
	minGraph    = 280

	yMin = 0
	yMax = 1023
	zMin = 0.0
	zMax = 1.5
)

type graphView struct {
	widget.BaseWidget
	editor *WBLEditor
	r      *graphRenderer
}

func newGraphView(editor *WBLEditor) *graphView {
	g := &graphView{editor: editor}
	g.ExtendBaseWidget(g)
	return g
}

func (g *graphView) CreateRenderer() fyne.WidgetRenderer {
	r := &graphRenderer{g: g}
	r.bg = canvas.NewRectangle(bgColor)
	g.r = r
	r.rebuild()
	return r
}

type graphRenderer struct {
	g    *graphView
	size fyne.Size

	bg        *canvas.Rectangle
	gridLines []*canvas.Line
	axes      []*canvas.Line
	dataLines []*canvas.Line
	points    []*draggablePoint

	// cached mapping from last layout (used by drag math)
	x0, y0, x1, y1 float32
	minYv, maxYv   int
	minZv, maxZv   float64

	objects []fyne.CanvasObject
}

func (r *graphRenderer) rebuild() {
	if len(r.gridLines) == 0 {
		for range 6 {
			l := canvas.NewLine(gridColor)
			l.StrokeWidth = 1
			r.gridLines = append(r.gridLines, l)
		}
	}
	if len(r.axes) == 0 {
		for range 2 {
			l := canvas.NewLine(axisColor)
			l.StrokeWidth = 1
			r.axes = append(r.axes, l)
		}
	}

	rows := r.g.editor.rows
	wantLines := 0
	if n := len(rows); n > 1 {
		wantLines = n - 1
	}
	for len(r.dataLines) < wantLines {
		l := canvas.NewLine(lineColor)
		l.StrokeWidth = 2
		r.dataLines = append(r.dataLines, l)
	}
	r.dataLines = r.dataLines[:wantLines]

	for len(r.points) < len(rows) {
		p := newDraggablePoint(r.g)
		r.points = append(r.points, p)
	}
	r.points = r.points[:len(rows)]
	for i, row := range rows {
		r.points[i].row = row
	}

	// dataLines/points may have changed length; force Objects() to rebuild.
	r.objects = nil
}

func (r *graphRenderer) Layout(size fyne.Size) {
	r.size = size
	r.bg.Resize(size)
	r.layoutGraph()
}

func (r *graphRenderer) layoutGraph() {
	w := r.size.Width
	h := r.size.Height
	if w <= 0 || h <= 0 {
		return
	}

	r.x0 = float32(graphMargin)
	r.y0 = float32(graphMargin)
	r.x1 = w - graphMargin
	r.y1 = h - graphMargin

	for i := range 3 {
		gy := r.y0 + (r.y1-r.y0)*float32(i+1)/4
		gh := r.gridLines[i]
		gh.Position1 = fyne.NewPos(r.x0, gy)
		gh.Position2 = fyne.NewPos(r.x1, gy)
		gh.Refresh()

		gx := r.x0 + (r.x1-r.x0)*float32(i+1)/4
		gv := r.gridLines[3+i]
		gv.Position1 = fyne.NewPos(gx, r.y0)
		gv.Position2 = fyne.NewPos(gx, r.y1)
		gv.Refresh()
	}

	r.axes[0].Position1 = fyne.NewPos(r.x0, r.y1)
	r.axes[0].Position2 = fyne.NewPos(r.x1, r.y1)
	r.axes[0].Refresh()
	r.axes[1].Position1 = fyne.NewPos(r.x0, r.y0)
	r.axes[1].Position2 = fyne.NewPos(r.x0, r.y1)
	r.axes[1].Refresh()

	rows := r.g.editor.rows
	if len(rows) == 0 {
		r.minYv, r.maxYv = yMin, yMax
		r.minZv, r.maxZv = zMin, zMax
		return
	}

	minY, maxY := rows[0].y, rows[0].y
	minZ, maxZ := rows[0].z, rows[0].z
	for _, row := range rows {
		if row.y < minY {
			minY = row.y
		}
		if row.y > maxY {
			maxY = row.y
		}
		if row.z < minZ {
			minZ = row.z
		}
		if row.z > maxZ {
			maxZ = row.z
		}
	}
	// Enforce a minimum visible span so dragging stays responsive when
	// points are clustered and so the graph doesn't degenerate to a point.
	const minYSpan, minZSpan = 50, 0.2
	if maxY-minY < minYSpan {
		mid := (maxY + minY) / 2
		minY = clampInt(mid-minYSpan/2, yMin, yMax-minYSpan)
		maxY = minY + minYSpan
	}
	if maxZ-minZ < minZSpan {
		mid := (maxZ + minZ) / 2
		minZ = clampFloat(mid-minZSpan/2, zMin, zMax-minZSpan)
		maxZ = minZ + minZSpan
	}
	r.minYv, r.maxYv = minY, maxY
	r.minZv, r.maxZv = minZ, maxZ

	type pt struct{ x, y float32 }
	pts := make([]pt, len(rows))
	for i, row := range rows {
		fx := float32(row.y-minY) / float32(maxY-minY)
		fy := float32((row.z - minZ) / (maxZ - minZ))
		pts[i].x = r.x0 + fx*(r.x1-r.x0)
		pts[i].y = r.y1 - fy*(r.y1-r.y0)
	}

	for i := 1; i < len(pts); i++ {
		l := r.dataLines[i-1]
		l.Position1 = fyne.NewPos(pts[i-1].x, pts[i-1].y)
		l.Position2 = fyne.NewPos(pts[i].x, pts[i].y)
		l.Refresh()
	}

	half := float32(pointSize) / 2
	for i, p := range pts {
		dp := r.points[i]
		dp.Resize(fyne.NewSize(pointSize, pointSize))
		dp.Move(fyne.NewPos(p.x-half, p.y-half))
		dp.Refresh()
	}
}

func (r *graphRenderer) Refresh() {
	r.bg.FillColor = bgColor
	r.bg.Refresh()
	r.rebuild()
	r.layoutGraph()
	canvas.Refresh(r.g)
}

func (r *graphRenderer) Objects() []fyne.CanvasObject {
	if r.objects == nil {

		r.objects = make([]fyne.CanvasObject, 0, 1+len(r.gridLines)+len(r.axes)+len(r.dataLines)+len(r.points))
		r.objects = append(r.objects, r.bg)
		for _, l := range r.gridLines {
			r.objects = append(r.objects, l)
		}
		for _, l := range r.axes {
			r.objects = append(r.objects, l)
		}
		for _, l := range r.dataLines {
			r.objects = append(r.objects, l)
		}
		for _, p := range r.points {
			r.objects = append(r.objects, p)
		}
	}
	return r.objects
}

func (r *graphRenderer) MinSize() fyne.Size {
	return fyne.NewSize(minGraph, minGraph)
}

func (r *graphRenderer) Destroy() {}

// --- draggable point -------------------------------------------------------

type draggablePoint struct {
	widget.BaseWidget
	g    *graphView
	row  *mapRow
	accY float64 // fractional accumulator for integer y axis
}

func newDraggablePoint(g *graphView) *draggablePoint {
	p := &draggablePoint{g: g}
	p.ExtendBaseWidget(p)
	return p
}

func (p *draggablePoint) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(pointColor)
	rect.StrokeColor = pointEdge
	rect.StrokeWidth = 1
	return widget.NewSimpleRenderer(rect)
}

func (p *draggablePoint) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (p *draggablePoint) Dragged(e *fyne.DragEvent) {
	r := p.g.r
	if r == nil || p.row == nil {
		return
	}
	w := r.x1 - r.x0
	h := r.y1 - r.y0
	if w <= 0 || h <= 0 {
		return
	}

	// pixel delta → value delta using last layout's data range
	dY := float64(e.Dragged.DX) / float64(w) * float64(r.maxYv-r.minYv)
	dZ := -float64(e.Dragged.DY) / float64(h) * (r.maxZv - r.minZv)

	p.accY += dY
	step := int(p.accY)
	p.accY -= float64(step)

	p.row.y = clampInt(p.row.y+step, yMin, yMax)
	p.row.z = clampFloat(p.row.z+dZ, zMin, zMax)

	p.g.editor.updateRowEntries(p.row)
	p.g.Refresh()
}

func (p *draggablePoint) DragEnd() {
	p.accY = 0
	p.g.editor.save()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
