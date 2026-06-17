package boosttuner

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
)

// PID maps and their shared axes. All three are row-major [rpm][loadDiff]:
// rows follow PIDYSP (rpm), columns follow PIDXSP (load error mg/c).
const (
	symPMap   = "BoostCal.PMap"
	symIMap   = "BoostCal.IMap"
	symDMap   = "BoostCal.DMap"
	symPIDXSP = "BoostCal.PIDXSP" // X axis: load error (mg/c)
	symPIDYSP = "BoostCal.PIDYSP" // Y axis: engine speed (rpm)
)

// pidEditor is one editable PID gain map. data is the same slice the mapviewer
// edits in place, so hand-edits flow back without a copy; Apply writes data into
// the binary.
type pidEditor struct {
	bt           *BoostTuner
	name         string // "P" / "I" / "D"
	symbolName   string
	axisX, axisY []float64
	cols, rows   int
	data         []float64
	mv           *mapviewer.MapViewer
	status       *widget.Label
	suggestBox   *fyne.Container // populated by the heuristics in pidtune.go
}

func (bt *BoostTuner) buildPIDTab() fyne.CanvasObject {
	x, errX := bt.readSymbol(symPIDXSP)
	y, errY := bt.readSymbol(symPIDYSP)
	if errX != nil || errY != nil {
		return container.NewCenter(widget.NewLabel("BoostCal PID axes not found in this binary."))
	}

	bt.pidEditors = map[string]*pidEditor{}
	tabs := container.NewAppTabs()
	for _, m := range []struct{ name, sym string }{
		{"P", symPMap}, {"I", symIMap}, {"D", symDMap},
	} {
		ed := bt.newPIDEditor(m.name, m.sym, x, y)
		bt.pidEditors[m.name] = ed
		tabs.Append(container.NewTabItem(m.name+" map", ed.object()))
	}

	intro := widget.NewLabel(
		"Edit the PID gain maps directly, or use the per-rpm-band suggestions " +
			"(from logged boost transients) to scale a map. Validate changes in the " +
			"Simulator tab before flashing.")
	intro.Wrapping = fyne.TextWrapWord

	suggestStatus := widget.NewLabel("")
	computeBtn := widget.NewButtonWithIcon("Compute suggestions from logs", theme.SearchIcon(), func() {
		suggestStatus.SetText(bt.computePIDSuggestions())
	})

	header := container.NewVBox(
		intro,
		container.NewBorder(nil, nil, nil, suggestStatus, computeBtn),
	)
	return container.NewBorder(header, nil, nil, nil, tabs)
}

func (bt *BoostTuner) newPIDEditor(name, symName string, x, y []float64) *pidEditor {
	ed := &pidEditor{
		bt: bt, name: name, symbolName: symName,
		axisX: x, axisY: y, cols: len(x), rows: len(y),
	}
	ed.status = widget.NewLabel("")
	ed.suggestBox = container.NewVBox()
	ed.reload()
	return ed
}

func (ed *pidEditor) object() fyne.CanvasObject {
	display := container.NewStack()
	ed.rebuildViewer(display)

	applyBtn := widget.NewButtonWithIcon("Apply to binary", theme.DocumentSaveIcon(), func() {
		if ed.bt.cfg.Save == nil {
			ed.status.SetText("No binary to write to.")
			return
		}
		if err := ed.bt.cfg.Save(ed.symbolName, ed.data); err != nil {
			ed.status.SetText("Save failed: " + err.Error())
			return
		}
		ed.status.SetText("Wrote " + ed.symbolName)
	})
	if ed.bt.cfg.Save == nil {
		applyBtn.Disable()
	}
	reloadBtn := widget.NewButtonWithIcon("Reload from binary", theme.ViewRefreshIcon(), func() {
		ed.reload()
		ed.rebuildViewer(display)
		ed.status.SetText("Reloaded " + ed.symbolName)
	})

	controls := container.NewVBox(
		container.NewGridWithColumns(2, applyBtn, reloadBtn),
		ed.status,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Suggestions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVScroll(ed.suggestBox),
	)
	side := container.NewBorder(nil, nil, nil, nil, controls)

	split := container.NewHSplit(display, side)
	split.Offset = 0.72
	return split
}

// reload reads the map from the binary into a fresh editable slice.
func (ed *pidEditor) reload() {
	z, err := ed.bt.readSymbol(ed.symbolName)
	if err != nil {
		ed.data = make([]float64, ed.cols*ed.rows)
		ed.status.SetText(err.Error())
		return
	}
	ed.data = z
}

// rebuildViewer swaps a fresh editable map viewer (bound to ed.data) into host.
func (ed *pidEditor) rebuildViewer(host *fyne.Container) {
	mv, err := mapviewer.New(&mapviewer.Config{
		Name:           ed.symbolName,
		XData:          ed.axisX,
		YData:          ed.axisY,
		ZData:          ed.data,
		XPrecision:     0,
		YPrecision:     0,
		ZPrecision:     0,
		XLabel:         "Load error (mg/c)",
		YLabel:         "Engine speed (rpm)",
		ZLabel:         ed.name + " constant",
		MeshView:       true,
		MeshRenderer:   ed.bt.cfg.MeshRenderer,
		Editable:       true,
		ColorblindMode: ed.bt.cfg.Colorblind,
		SaveECUFunc:    func([]float64) {},
		OnUpdateCell:   func(int, []float64) {},
	})
	if err != nil {
		host.Objects = []fyne.CanvasObject{container.NewCenter(widget.NewLabel(err.Error()))}
		host.Refresh()
		return
	}
	ed.mv = mv
	host.Objects = []fyne.CanvasObject{mv}
	host.Refresh()
}

// scaleRows multiplies whole rpm rows (indexed by PIDYSP) by per-row factors and
// refreshes the viewer. Used by the heuristic suggestions in pidtune.go.
func (ed *pidEditor) scaleRows(factors map[int]float64) {
	for row, f := range factors {
		if row < 0 || row >= ed.rows {
			continue
		}
		for c := 0; c < ed.cols; c++ {
			ed.data[row*ed.cols+c] *= f
		}
	}
	if ed.mv != nil {
		_ = ed.mv.SetZData(ed.data)
		ed.mv.Refresh()
	}
	ed.status.SetText(fmt.Sprintf("Scaled %d row(s); review then Apply.", len(factors)))
}
