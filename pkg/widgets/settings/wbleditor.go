package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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

type adScannerPreset struct {
	ECU string    `json:"ecu"`
	Y   []int     `json:"y"`
	Z   []float64 `json:"z"`
}

// adResolutionForECU returns the AD converter resolution for an ECU type.
func adResolutionForECU(ecu string) int {
	if ecu == "T5" {
		return 255
	}
	return 1023
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
}

func NewWBLEditor(yAxis []int, zValues []float64) *WBLEditor {
	m := &WBLEditor{}
	n := min(len(yAxis), len(zValues))
	for i := range n {
		m.rows = append(m.rows, &mapRow{y: yAxis[i], z: zValues[i]})
	}
	m.ExtendBaseWidget(m)
	m.adresolution = adResolutionForECU(prefLastADScannerECU.get())
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

// setRows replaces all rows with the supplied y/z pairs, rebuilding their
// widgets and the rows container (used when loading presets at runtime).
func (m *WBLEditor) setRows(yAxis []int, zValues []float64) {
	m.rows = nil
	n := min(len(yAxis), len(zValues))
	for i := range n {
		r := &mapRow{y: yAxis[i], z: zValues[i]}
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
}

func (m *WBLEditor) refreshGraph() {
	if m.graph != nil {
		m.graph.Refresh()
	}
}

func (m *WBLEditor) save() {
	prefWBLSupportPoints.set(m.YAxis())
	prefWBLLambdaValues.set(m.ZValues())
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
	if lastPreset := prefLastADScannerPreset.get(); lastPreset != "" {
		m.presetSelect.Selected = lastPreset
	}

	m.ecuSelect = widget.NewSelect([]string{"T5", "T7", "T8"}, func(s string) {
		m.adresolution = adResolutionForECU(s)
		prefLastADScannerECU.set(s)
		for _, r := range m.rows {
			if r.vo != nil {
				r.vo.Text = fmt.Sprintf("%.2f", m.voltFromY(r.y))
				r.vo.Refresh()
			}
		}
	})
	m.ecuSelect.Selected = prefLastADScannerECU.get()

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
			widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), m.refreshPresets),
		),
		m.presetSelect,
	)
	bottom := widget.NewButton("Save", m.savePreset)

	view := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		container.NewHSplit(left, m.graph),
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
		preset := adScannerPreset{
			ECU: m.ecuSelect.Selected,
			Y:   m.YAxis(),
			Z:   m.ZValues(),
		}

		f, err := os.Create(filepath.Join(layoutPath, name.Text+".json"))
		if err != nil {
			debug.Log(err.Error())
			return
		}
		defer f.Close()

		if err := json.NewEncoder(f).Encode(preset); err != nil {
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
		m.setRows(preset.Y, preset.Z)
		m.ecuSelect.SetSelected(preset.ECU)
		prefLastADScannerPreset.set(name)
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

	m.setRows(preset.Y, preset.Z)
	prefLastADScannerPreset.set(name)
	m.save()
}

func (m *WBLEditor) deletePreset(name string) {
	layoutPath, err := common.GetADScannerPath()
	if err != nil {
		fyne.LogError("Could not get layout path", err)
		return
	}
	if err := os.Remove(filepath.Join(layoutPath, name+".json")); err != nil {
		fyne.LogError("Could not delete preset file", err)
		return
	}
}
