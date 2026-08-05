package boosttuner

import (
	"fmt"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
)

// RegMap symbol and its axes in the binary. RegMap is row-major [rpm][load]:
// rows follow n_EngSP, columns follow SetLoadXSP.
const (
	symRegMap     = "BoostCal.RegMap"
	symSetLoadXSP = "BoostCal.SetLoadXSP" // X axis: load set value (mg/c)
	symNEngSP     = "BoostCal.n_EngSP"    // Y axis: engine speed (rpm)
)

// regMapView lists the selectable views for the learned map.
const (
	viewCurrent  = "Current"
	viewLearned  = "Learned (what gets written)"
	viewDelta    = "Delta"
	viewCoverage = "Coverage (samples)"
)

func (bt *BoostTuner) buildRegMapTab() fyne.CanvasObject {
	bt.rmStatus = widget.NewLabel("Load logs, then click Analyze.")
	bt.rmStatus.Wrapping = fyne.TextWrapWord

	analyzeBtn := widget.NewButtonWithIcon("Analyze", theme.SearchIcon(), func() {
		if err := bt.analyzeRegMap(); err != nil {
			bt.rmStatus.SetText(err.Error())
			return
		}
		bt.refreshRegMapDisplay()
	})
	analyzeBtn.Importance = widget.HighImportance

	bt.rmView = widget.NewSelect(
		[]string{viewCurrent, viewLearned, viewDelta, viewCoverage},
		func(string) { bt.refreshRegMapDisplay() },
	)
	bt.rmView.SetSelected(viewLearned)

	applyBtn := widget.NewButtonWithIcon("Apply to binary", theme.DocumentSaveIcon(), bt.applyRegMap)
	if bt.cfg.Save == nil {
		applyBtn.Disable()
	}

	controls := container.NewVBox(
		analyzeBtn,
		labeledRow("View", bt.rmView),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Sample filter", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		bt.slider("On-target |error| (mg/c)", 5, 200, 5, bt.onTarget, func(v float64) { bt.onTarget = v }),
		bt.slider("Max rpm step (rpm)", 20, 500, 10, bt.rpmStab, func(v float64) { bt.rpmStab = v }),
		bt.slider("Max load step (mg/c)", 10, 300, 10, bt.loadStab, func(v float64) { bt.loadStab = v }),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Apply", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		bt.slider("Min samples / cell", 1, 50, 1, float64(bt.minSamples), func(v float64) { bt.minSamples = int(v); bt.refreshRegMapDisplay() }),
		bt.slider("Blend %", 0, 100, 5, bt.blend*100, func(v float64) { bt.blend = v / 100; bt.refreshRegMapDisplay() }),
		applyBtn,
		widget.NewSeparator(),
		bt.rmStatus,
	)

	bt.rmDisplay = container.NewStack(container.NewCenter(
		widget.NewLabel("Learn BoostCal.RegMap from the loaded logs."),
	))

	controlScroll := container.NewVScroll(controls)
	controlScroll.SetMinSize(fyne.NewSize(250, 0))
	split := container.NewHSplit(controlScroll, bt.rmDisplay)
	split.Offset = 0.28
	return split
}

// slider builds a labelled slider with a live value readout that calls onChange.
func (bt *BoostTuner) slider(label string, min, max, step, init float64, onChange func(float64)) fyne.CanvasObject {
	val := widget.NewLabel(formatFloat(init))
	s := widget.NewSlider(min, max)
	s.Step = step
	s.SetValue(init)
	s.OnChanged = func(v float64) {
		val.SetText(formatFloat(v))
		onChange(v)
	}
	return container.NewVBox(
		widget.NewLabel(label),
		container.NewBorder(nil, nil, nil, layout.NewFixedWidth(48, val), s),
	)
}

// analyzeRegMap reads the current RegMap and its axes from the binary, then learns
// a new map from the logs by folding the loop's corrections back into the
// feedforward. See the package doc for the control-law rationale.
func (bt *BoostTuner) analyzeRegMap() error {
	if len(bt.values) == 0 {
		return fmt.Errorf("load a log file first (Logs tab)")
	}
	if missing := bt.missingChannels(); len(missing) > 0 {
		return fmt.Errorf("missing channels: %v", missing)
	}
	if err := bt.loadRegMapFromBinary(); err != nil {
		return err
	}

	in := regMapInputs{
		n:        bt.nrecords,
		rpm:      mustSeries(bt.series("rpm")),
		setv:     mustSeries(bt.series("setValue")),
		regCon:   mustSeries(bt.series("regCon")),
		pFac:     mustSeries(bt.series("pFac")),
		iFac:     mustSeries(bt.series("iFac")),
		dFac:     mustSeries(bt.series("dFac")),
		adap:     mustSeries(bt.series("adaption")),
		loadDiff: mustSeries(bt.series("loadDiff")),
		pwmCalc:  mustSeries(bt.series("pwmCalc")),
		pwm2pct:  mustSeries(bt.series("pwm2pct")),
	}
	p := regMapParams{
		axisX: bt.rmAxisX, axisY: bt.rmAxisY,
		cols: bt.rmCols, rows: bt.rmRows,
		current:  bt.rmCurrent,
		onTarget: bt.onTarget, rpmStab: bt.rpmStab, loadStab: bt.loadStab,
	}

	learned, cnt, used, filtered := learnRegMap(in, p)
	bt.rmLearned, bt.rmCounts = learned, cnt
	filled := 0
	for _, c := range cnt {
		if c > 0 {
			filled++
		}
	}
	bt.rmBuilt = true
	bt.rmStatus.SetText(fmt.Sprintf("Used %d samples (%d filtered). %d/%d cells have data.",
		used, filtered, filled, bt.rmCols*bt.rmRows))
	return nil
}

// regMapInputs holds the row-aligned log channels the learner consumes. All
// slices have length n (NaN where a sample is missing).
type regMapInputs struct {
	n                                                                     int
	rpm, setv, regCon, pFac, iFac, dFac, adap, loadDiff, pwmCalc, pwm2pct []float64
}

// regMapParams holds the map geometry (from the binary) and the sample filter.
type regMapParams struct {
	axisX, axisY                []float64
	cols, rows                  int
	current                     []float64
	onTarget, rpmStab, loadStab float64
}

// learnRegMap folds the loop's corrections back into the feedforward map. For
// each accepted sample it adds RegConValue+PFac+IFac+DFac+Adaption (converted to
// %) to the cell its (load, rpm) lands on, then averages per cell. Empty cells
// return the current value so they contribute a zero delta. Pure: no UI/state.
func learnRegMap(in regMapInputs, p regMapParams) (learned []float64, counts []int, used, filtered int) {
	size := p.cols * p.rows
	sum := make([]float64, size)
	counts = make([]int, size)

	prevRPM, prevLoad := math.NaN(), math.NaN()
	for i := 0; i < in.n; i++ {
		r, s := in.rpm[i], in.setv[i]
		if anyNaN(r, s, in.regCon[i], in.pFac[i], in.iFac[i], in.dFac[i], in.adap[i], in.loadDiff[i], in.pwmCalc[i], in.pwm2pct[i]) {
			prevRPM, prevLoad = r, s
			continue
		}
		ok := acceptSample(p, r, s, in.loadDiff[i], in.pwmCalc[i], in.pwm2pct[i], prevRPM, prevLoad)
		prevRPM, prevLoad = r, s
		if !ok {
			filtered++
			continue
		}
		target := (in.regCon[i] + in.pFac[i] + in.iFac[i] + in.dFac[i] + in.adap[i]) / dutyRawPerPct
		c := nearestIndex(p.axisX, s)
		row := nearestIndex(p.axisY, r)
		idx := row*p.cols + c
		sum[idx] += target
		counts[idx]++
		used++
	}

	learned = make([]float64, size)
	for i := range sum {
		if counts[i] > 0 {
			learned[i] = sum[i] / float64(counts[i])
		} else {
			learned[i] = p.current[i]
		}
	}
	return learned, counts, used, filtered
}

// acceptSample reports whether a sample is trustworthy for learning the
// feedforward: on target, steady (not mid-transient), in closed loop, and not at
// the duty rails.
func acceptSample(p regMapParams, rpm, load, loadDiff, pwmCalc, pwm2pct, prevRPM, prevLoad float64) bool {
	if math.Abs(loadDiff) > p.onTarget {
		return false
	}
	if pwm2pct != 0 { // open-loop / forced 2%
		return false
	}
	if pwmCalc <= 20 || pwmCalc >= 980 { // clamped at a rail
		return false
	}
	if !math.IsNaN(prevRPM) && math.Abs(rpm-prevRPM) > p.rpmStab {
		return false
	}
	if !math.IsNaN(prevLoad) && math.Abs(load-prevLoad) > p.loadStab {
		return false
	}
	return true
}

// mustSeries unwraps a resolved channel; analyzeRegMap guarantees presence via
// missingChannels before calling, so the bool is discarded here.
func mustSeries(s []float64, _ bool) []float64 { return s }

// loadRegMapFromBinary reads BoostCal.RegMap and its axis breakpoints from the
// loaded binary into the tuner's state.
func (bt *BoostTuner) loadRegMapFromBinary() error {
	if bt.cfg.Symbols == nil {
		return fmt.Errorf("no binary loaded")
	}
	x, err := bt.readSymbol(symSetLoadXSP)
	if err != nil {
		return err
	}
	y, err := bt.readSymbol(symNEngSP)
	if err != nil {
		return err
	}
	z, err := bt.readSymbol(symRegMap)
	if err != nil {
		return err
	}
	if len(x)*len(y) != len(z) {
		return fmt.Errorf("RegMap size %d != %d x %d axes", len(z), len(x), len(y))
	}
	bt.rmAxisX, bt.rmAxisY = x, y
	bt.rmCols, bt.rmRows = len(x), len(y)
	bt.rmCurrent = z
	return nil
}

// readSymbol returns a symbol's values in engineering units.
func (bt *BoostTuner) readSymbol(name string) ([]float64, error) {
	if bt.cfg.Symbols == nil {
		return nil, fmt.Errorf("no binary loaded")
	}
	s := bt.cfg.Symbols.GetByName(name)
	if s == nil {
		return nil, fmt.Errorf("symbol %s not found in binary", name)
	}
	return s.Float64s(), nil
}

// writeTarget returns the map that "Apply" would write: filled cells blended
// toward the learned value, everything else left at its current value.
func (bt *BoostTuner) writeTarget() []float64 {
	out := make([]float64, len(bt.rmCurrent))
	copy(out, bt.rmCurrent)
	for i := range out {
		if bt.rmCounts[i] >= bt.minSamples {
			out[i] = bt.rmCurrent[i] + bt.blend*(bt.rmLearned[i]-bt.rmCurrent[i])
		}
	}
	return out
}

func (bt *BoostTuner) applyRegMap() {
	if !bt.rmBuilt {
		bt.rmStatus.SetText("Analyze first.")
		return
	}
	if bt.cfg.Save == nil {
		bt.rmStatus.SetText("No binary to write to.")
		return
	}
	data := bt.writeTarget()
	changed := 0
	for i := range data {
		if data[i] != bt.rmCurrent[i] {
			changed++
		}
	}
	if err := bt.cfg.Save(symRegMap, data); err != nil {
		bt.rmStatus.SetText("Save failed: " + err.Error())
		return
	}
	bt.rmCurrent = data // the binary now holds these values
	bt.rmStatus.SetText(fmt.Sprintf("Wrote %s: %d cells changed.", symRegMap, changed))
	bt.refreshRegMapDisplay()
}

// refreshRegMapDisplay rebuilds the map viewer for the selected view.
func (bt *BoostTuner) refreshRegMapDisplay() {
	if !bt.rmBuilt {
		return
	}
	var z []float64
	var label string
	zPrec := 1
	switch bt.rmView.Selected {
	case viewCurrent:
		z, label = bt.rmCurrent, "Current duty %"
	case viewDelta:
		target := bt.writeTarget()
		z = make([]float64, len(target))
		for i := range z {
			z[i] = target[i] - bt.rmCurrent[i]
		}
		label = "Delta duty %"
	case viewCoverage:
		z = make([]float64, len(bt.rmCounts))
		for i := range z {
			z[i] = float64(bt.rmCounts[i])
		}
		label, zPrec = "Samples", 0
	default: // viewLearned
		z, label = bt.writeTarget(), "Learned duty %"
	}

	mv, err := mapviewer.New(&mapviewer.Config{
		Name:           symRegMap,
		XData:          bt.rmAxisX,
		YData:          bt.rmAxisY,
		ZData:          z,
		XPrecision:     0,
		YPrecision:     0,
		ZPrecision:     zPrec,
		XLabel:         "Load set value (mg/c)",
		YLabel:         "Engine speed (rpm)",
		ZLabel:         label,
		MeshView:       true,
		MeshRenderer:   bt.cfg.MeshRenderer,
		Editable:       false,
		ColorblindMode: bt.cfg.Colorblind,
		SaveECUFunc:    func([]float64) {},
		OnUpdateCell:   func(int, []float64) {},
	})
	if err != nil {
		bt.rmDisplay.Objects = []fyne.CanvasObject{container.NewCenter(widget.NewLabel(err.Error()))}
		bt.rmDisplay.Refresh()
		return
	}
	bt.rmDisplay.Objects = []fyne.CanvasObject{mv}
	bt.rmDisplay.Refresh()
}

// --- small helpers ---

func labeledRow(label string, obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, obj)
}

func anyNaN(vals ...float64) bool {
	for _, v := range vals {
		if math.IsNaN(v) {
			return true
		}
	}
	return false
}
