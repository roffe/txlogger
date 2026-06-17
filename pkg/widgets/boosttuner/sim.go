package boosttuner

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
)

// The replay simulator re-runs the ECU boost control law (from Boost.c) over a
// logged session using the *current* maps, so the effect of map edits on the
// controller's duty output can be inspected before flashing.
//
// IMPORTANT limitation: it drives the law with the *logged* error (LoadDiff),
// because predicting the resulting airmass would need a turbo/engine plant model
// we do not have. So it shows how the control effort (PWM and the P/I/D split)
// would differ for the same measured error — useful for spotting instability or
// I-windup — but it does NOT predict the resulting boost.

// bracket returns the two indices of an ascending table that surround v and the
// interpolation fraction between them, clamping to the ends (so values outside
// the table hold the edge value, matching the ECU's TAB/MAT routines).
func bracket(tab []float64, v float64) (i0, i1 int, frac float64) {
	n := len(tab)
	if n <= 1 {
		return 0, 0, 0
	}
	if v <= tab[0] {
		return 0, 0, 0
	}
	if v >= tab[n-1] {
		return n - 1, n - 1, 0
	}
	for i := 0; i < n-1; i++ {
		if v >= tab[i] && v <= tab[i+1] {
			span := tab[i+1] - tab[i]
			if span == 0 {
				return i, i, 0
			}
			return i, i + 1, (v - tab[i]) / span
		}
	}
	return n - 1, n - 1, 0
}

// bilerp bilinearly interpolates a row-major map z (rows follow ytab, cols follow
// xtab) at (x, y), clamping outside the axis ranges. This is the Go stand-in for
// the ECU's MATs16 routines (faithful, not bit-exact).
func bilerp(xtab, ytab, z []float64, x, y float64) float64 {
	cols := len(xtab)
	x0, x1, xf := bracket(xtab, x)
	y0, y1, yf := bracket(ytab, y)
	z00 := z[y0*cols+x0]
	z10 := z[y0*cols+x1]
	z01 := z[y1*cols+x0]
	z11 := z[y1*cols+x1]
	top := z00 + (z10-z00)*xf
	bot := z01 + (z11-z01)*xf
	return top + (bot-top)*yf
}

// simInputs holds the row-aligned log channels the replay consumes (all length n,
// raw units: duty terms in 0.1%, errors in mg/c).
type simInputs struct {
	n                                                                     int
	rpm, setv, loadDiff, regCon, pFac, iFac, dFac, adap, pwmCalc, pwm2pct []float64
}

// simMaps holds the maps and constants the law uses. regMapRaw is in raw 0.1%
// units (the % map symbol scaled by dutyRawPerPct).
type simMaps struct {
	setLoadXSP, nEngSP, regMapRaw []float64
	pidXSP, pidYSP                []float64
	pMap, iMap, dMap              []float64
	iFacMax, filterFactor         float64
}

// simOutput is the replay result, all in raw 0.1% duty units.
type simOutput struct {
	predicted, regCon, pFac, iFac, dFac []float64
}

// simulate replays the control law. It recomputes RegConValue and the P/I/D terms
// from the maps while carrying the logged environment residual (E85/altitude/temp/
// noise + any rounding) unchanged, so only map-driven differences show.
func simulate(in simInputs, m simMaps) simOutput {
	out := simOutput{
		predicted: make([]float64, in.n),
		regCon:    make([]float64, in.n),
		pFac:      make([]float64, in.n),
		iFac:      make([]float64, in.n),
		dFac:      make([]float64, in.n),
	}
	var iBuff, iFacAcc, dFacState, loadDiffOld float64
	haveOld := false

	for i := 0; i < in.n; i++ {
		if anyNaN(in.rpm[i], in.setv[i], in.loadDiff[i], in.regCon[i], in.pFac[i], in.iFac[i], in.dFac[i], in.adap[i], in.pwmCalc[i], in.pwm2pct[i]) {
			out.predicted[i], out.regCon[i] = math.NaN(), math.NaN()
			out.pFac[i], out.iFac[i], out.dFac[i] = math.NaN(), math.NaN(), math.NaN()
			haveOld = false
			continue
		}
		// Residual = everything in the logged PWMCalc the maps don't drive.
		env := in.pwmCalc[i] - (in.regCon[i] + in.pFac[i] + in.iFac[i] + in.dFac[i] + in.adap[i])
		regConSim := bilerp(m.setLoadXSP, m.nEngSP, m.regMapRaw, in.setv[i], in.rpm[i])
		out.regCon[i] = regConSim

		if in.pwm2pct[i] != 0 { // open loop: PID ramped off
			iBuff, iFacAcc, dFacState = 0, 0, 0
			haveOld = false
			out.predicted[i] = regConSim + in.adap[i] + env
			continue
		}

		ld := in.loadDiff[i]
		pConst := bilerp(m.pidXSP, m.pidYSP, m.pMap, ld, in.rpm[i])
		iConst := bilerp(m.pidXSP, m.pidYSP, m.iMap, ld, in.rpm[i])
		dConst := bilerp(m.pidXSP, m.pidYSP, m.dMap, ld, in.rpm[i])

		pFac := ld * pConst / 100
		iBuff += iConst * ld
		if iBuff > 1000 {
			iFacAcc += iBuff / 1000
			iBuff = 0
			if iFacAcc > m.iFacMax {
				iFacAcc = m.iFacMax
			}
		} else if iBuff < -1000 {
			iFacAcc += iBuff / 1000
			iBuff = 0
			if iFacAcc < -m.iFacMax {
				iFacAcc = -m.iFacMax
			}
		}
		if !haveOld {
			loadDiffOld = ld
			haveOld = true
		}
		dFacState = ((ld-loadDiffOld)*dConst + dFacState*m.filterFactor) / (20 + m.filterFactor)
		loadDiffOld = ld

		out.pFac[i], out.iFac[i], out.dFac[i] = pFac, iFacAcc, dFacState
		out.predicted[i] = regConSim + pFac + iFacAcc + dFacState + in.adap[i] + env
	}
	return out
}

// --- UI ---

func (bt *BoostTuner) buildSimTab() fyne.CanvasObject {
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	intro := widget.NewLabel(
		"Replays the ECU boost control law over the loaded logs using the current " +
			"maps (RegMap from the binary; P/I/D from the editors). It predicts the " +
			"controller's duty output for the logged error — it does NOT predict the " +
			"resulting boost (no engine/turbo model). Use it to check edits don't make " +
			"the duty oscillate or wind up.")
	intro.Wrapping = fyne.TextWrapWord

	display := container.NewStack(container.NewCenter(
		widget.NewLabel("Load logs, then click Simulate."),
	))

	simBtn := widget.NewButtonWithIcon("Simulate", theme.MediaPlayIcon(), func() {
		values, err := bt.runSimulation()
		if err != nil {
			status.SetText(err.Error())
			return
		}
		order := []string{"PWMCalc logged", "PWMCalc predicted", "P (sim)", "I (sim)", "D (sim)"}
		p := plotter.NewPlotter(values, plotter.WithOrder(order))
		display.Objects = []fyne.CanvasObject{p}
		display.Refresh()
		status.SetText("Simulated. Toggle series in the legend.")
	})

	header := container.NewVBox(intro, container.NewBorder(nil, nil, nil, status, simBtn))
	return container.NewBorder(header, nil, nil, nil, display)
}

// runSimulation gathers inputs and current maps, runs the replay and returns the
// named series for the plotter.
func (bt *BoostTuner) runSimulation() (map[string][]float64, error) {
	if len(bt.values) == 0 {
		return nil, errString("load logs first (Logs tab)")
	}
	if missing := bt.missingChannels(); len(missing) > 0 {
		return nil, errString("missing channels for simulation")
	}
	in := simInputs{
		n:        bt.nrecords,
		rpm:      mustSeries(bt.series("rpm")),
		setv:     mustSeries(bt.series("setValue")),
		loadDiff: mustSeries(bt.series("loadDiff")),
		regCon:   mustSeries(bt.series("regCon")),
		pFac:     mustSeries(bt.series("pFac")),
		iFac:     mustSeries(bt.series("iFac")),
		dFac:     mustSeries(bt.series("dFac")),
		adap:     mustSeries(bt.series("adaption")),
		pwmCalc:  mustSeries(bt.series("pwmCalc")),
		pwm2pct:  mustSeries(bt.series("pwm2pct")),
	}

	m, err := bt.loadSimMaps()
	if err != nil {
		return nil, err
	}
	out := simulate(in, m)

	return map[string][]float64{
		"PWMCalc logged":    in.pwmCalc,
		"PWMCalc predicted": out.predicted,
		"P (sim)":           out.pFac,
		"I (sim)":           out.iFac,
		"D (sim)":           out.dFac,
	}, nil
}

// loadSimMaps reads the maps and constants for the replay: RegMap and its axes
// from the binary, P/I/D from the editors (so edits/suggestions are reflected),
// IFacMax and FilterFactor from the binary.
func (bt *BoostTuner) loadSimMaps() (simMaps, error) {
	var m simMaps
	var err error
	if m.setLoadXSP, err = bt.readSymbol(symSetLoadXSP); err != nil {
		return m, err
	}
	if m.nEngSP, err = bt.readSymbol(symNEngSP); err != nil {
		return m, err
	}
	regPct, err := bt.readSymbol(symRegMap)
	if err != nil {
		return m, err
	}
	m.regMapRaw = make([]float64, len(regPct))
	for i, v := range regPct {
		m.regMapRaw[i] = v * dutyRawPerPct // % -> raw 0.1%
	}
	if m.pidXSP, err = bt.readSymbol(symPIDXSP); err != nil {
		return m, err
	}
	if m.pidYSP, err = bt.readSymbol(symPIDYSP); err != nil {
		return m, err
	}
	m.pMap = bt.pidMapData("P", symPMap)
	m.iMap = bt.pidMapData("I", symIMap)
	m.dMap = bt.pidMapData("D", symDMap)

	if v, err := bt.readSymbol("BoostCal.IFacMax"); err == nil && len(v) > 0 {
		m.iFacMax = v[0]
	} else {
		m.iFacMax = 350
	}
	if v, err := bt.readSymbol("BoostCal.FilterFactor"); err == nil && len(v) > 0 {
		m.filterFactor = v[0]
	}
	return m, nil
}

// pidMapData returns the editor's live (possibly edited) map data, falling back
// to the binary if the editor is absent.
func (bt *BoostTuner) pidMapData(name, symName string) []float64 {
	if ed := bt.pidEditors[name]; ed != nil && len(ed.data) > 0 {
		return ed.data
	}
	v, _ := bt.readSymbol(symName)
	return v
}

type errString string

func (e errString) Error() string { return string(e) }
