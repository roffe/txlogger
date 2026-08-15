package mbt

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/widgets/graph2d"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

type Config struct {
	GetFW      func() symbol.SymbolCollection
	Colorblind colors.ColorBlindMode

	// OpenWindow, if set, shows the help document in the host's window
	// manager so it behaves like every other tool window. Without it the
	// help opens as a top-level window instead.
	OpenWindow func(title string, content fyne.CanvasObject)
}

// map views, in the order they appear in the selector
const (
	viewDelta  = "Δ MBT − map (°)"
	viewMBT    = "MBT timing (° BTDC)"
	viewMap    = "Map timing (° BTDC)"
	viewLoss   = "Torque lost to retard (%)"
	viewPPA    = "Peak pressure (bar)"
	viewPPL    = "Peak pressure location (° ATDC)"
	viewLambda = "Assumed λ"
)

var views = []string{viewDelta, viewMBT, viewMap, viewLoss, viewPPA, viewPPL, viewLambda}

type Widget struct {
	widget.BaseWidget
	cfg *Config

	model Model
	crit  Criterion // the combustion descriptor that defines MBT
	lamCL float64   // closed-loop lambda
	lamOL float64   // open-loop (high load) lambda
	fitTo float64   // fit against cells at or below this airmass, mg/c

	// read from the loaded binary
	rpmAxis, airAxis, ignMap, clLimit []float64

	// entries that a preset rewrites
	eBore, eStroke, eRod, eCR *numericentry.Widget
	eQHV, eAFs                *numericentry.Widget
	eScale                    *numericentry.Widget

	view      *widget.Select
	mv        *mapviewer.MapViewer
	status    *widget.Label
	graphBox  *fyne.Container
	pointRPM  *numericentry.Widget
	pointAir  *numericentry.Widget
	pointInfo *widget.Label

	content fyne.CanvasObject
}

func New(cfg *Config) *Widget {
	w := &Widget{
		cfg:   cfg,
		model: Model{Engine: Engines[3], Params: DefaultParams(), Burn: DefaultBurn()},
		crit:  DefaultCriterion(),
		lamCL: 1.0,
		lamOL: 0.82,
		fitTo: 350,
	}
	w.ExtendBaseWidget(w)

	if err := w.load(); err != nil {
		w.content = container.NewCenter(widget.NewLabel(err.Error()))
		return w
	}

	w.status = widget.NewLabel("")
	w.status.Wrapping = fyne.TextWrapWord
	w.pointInfo = widget.NewLabel("")
	w.pointInfo.Wrapping = fyne.TextWrapWord
	w.graphBox = container.NewStack()

	w.view = widget.NewSelect(views, func(string) { w.recalc() })
	w.view.SetSelectedIndex(0)

	mv, err := mapviewer.New(&mapviewer.Config{
		Name:           "MBT",
		XData:          w.airAxis,
		YData:          w.rpmAxis,
		ZData:          make([]float64, len(w.ignMap)),
		XLabel:         "mg/c",
		YLabel:         "rpm",
		ZLabel:         "°",
		XPrecision:     0,
		YPrecision:     0,
		ZPrecision:     1,
		ColorblindMode: cfg.Colorblind,
	})
	if err != nil {
		w.content = container.NewCenter(widget.NewLabel(err.Error()))
		return w
	}
	w.mv = mv

	w.pointRPM = numericentry.New("3000")
	w.pointAir = numericentry.New("900")
	w.pointRPM.OnChanged = func(string) { w.recalcPoint() }
	w.pointAir.OnChanged = func(string) { w.recalcPoint() }

	point := container.NewBorder(
		container.NewGridWithColumns(4,
			widget.NewLabel("rpm"), w.pointRPM,
			widget.NewLabel("mg/c"), w.pointAir,
		),
		w.pointInfo, nil, nil, w.graphBox,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("Map", container.NewBorder(w.view, nil, nil, nil, w.mv)),
		container.NewTabItem("Cycle", point),
	)

	w.content = container.NewBorder(nil, w.status, w.controls(), nil, tabs)
	w.recalc()
	return w
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

// load pulls the ignition map, its axes and the closed-loop load limit out
// of the currently loaded binary.
func (w *Widget) load() error {
	fw := w.cfg.GetFW()
	if fw == nil {
		return fmt.Errorf("no binary loaded")
	}
	var missing []string
	get := func(name string) []float64 {
		s := fw.GetByName(name)
		if s == nil {
			missing = append(missing, name)
			return nil
		}
		return s.Float64s()
	}
	w.ignMap = get("IgnNormCal.Map")
	w.airAxis = get("IgnNormCal.m_AirXSP")
	w.rpmAxis = get("IgnNormCal.n_EngYSP")
	if len(missing) > 0 {
		return fmt.Errorf("missing symbols: %s", strings.Join(missing, ", "))
	}
	if len(w.ignMap) != len(w.rpmAxis)*len(w.airAxis) {
		return fmt.Errorf("ignition map is %d cells, axes say %dx%d",
			len(w.ignMap), len(w.rpmAxis), len(w.airAxis))
	}
	// optional: without it every cell is assumed closed loop
	if s := fw.GetByName("LambdaCal.MaxLoadNormTab"); s != nil {
		w.clLimit = s.Float64s()
	}
	return nil
}

// lambda returns the assumed air/fuel ratio at a cell. T7 runs closed loop
// up to LambdaCal.MaxLoadNormTab and enriches above it.
func (w *Widget) lambda(rpmIdx int, air float64) float64 {
	if rpmIdx < len(w.clLimit) && air > w.clLimit[rpmIdx] {
		return w.lamOL
	}
	return w.lamCL
}

// The corner of IgnNormCal below these is not the map the ECU runs on:
// idle comes from IgnIdleCal, and the bottom airmass columns are overrun.
// Fitting or summarising over them just adds noise.
const (
	minUsableRPM = 1200
	minUsableAir = 100
)

func (w *Widget) usable(pt Point) bool {
	return pt.RPM >= minUsableRPM && pt.Air >= minUsableAir
}

func (w *Widget) point(rpmIdx, airIdx int) Point {
	air := w.airAxis[airIdx]
	return Point{
		RPM:    w.rpmAxis[rpmIdx],
		Air:    air,
		Lambda: w.lambda(rpmIdx, air),
		Ign:    w.ignMap[rpmIdx*len(w.airAxis)+airIdx],
	}
}

func (w *Widget) recalc() {
	if w.mv == nil {
		return
	}
	z := make([]float64, len(w.ignMap))
	var sumDelta, worst float64
	var counted int
	var worstAt Point
	for y := range w.rpmAxis {
		for x := range w.airAxis {
			pt := w.point(y, x)
			i := y*len(w.airAxis) + x
			mbt := w.model.MBT(pt, w.crit)
			switch w.view.Selected {
			case viewDelta:
				z[i] = mbt - pt.Ign
			case viewMBT:
				z[i] = mbt
			case viewMap:
				z[i] = pt.Ign
			case viewLoss:
				z[i] = w.model.IMEPLoss(pt, w.crit) * 100
			case viewPPA:
				z[i] = w.model.Trace(pt).PPA / 1e5
			case viewPPL:
				z[i] = w.model.Trace(pt).PPL
			case viewLambda:
				z[i] = pt.Lambda
			}
			// Summarise the high-load region only: below the fit
			// threshold the model was fitted to this very map, so the
			// delta there is circular.
			if pt.Air <= w.fitTo || !w.usable(pt) {
				continue
			}
			d := mbt - pt.Ign
			sumDelta += d
			counted++
			if d > worst {
				worst, worstAt = d, pt
			}
		}
	}
	w.mv.SetZData(z)
	if counted == 0 {
		w.status.SetText("no cells above the fit threshold to summarise")
	} else {
		w.status.SetText(fmt.Sprintf(
			"above %.0f mg/c: mean %.1f° retarded from MBT, worst %.1f° at %.0f rpm / %.0f mg/c, costing %.1f%% torque. "+
				"Burn angles are modelled, not measured — Fit against the part-load region before reading anything into these cells.",
			w.fitTo, sumDelta/float64(counted), worst, worstAt.RPM, worstAt.Air,
			w.model.IMEPLoss(worstAt, w.crit)*100))
	}
	w.recalcPoint()
}

func (w *Widget) recalcPoint() {
	if w.graphBox == nil {
		return
	}
	rpm, _ := strconv.ParseFloat(w.pointRPM.Text, 64)
	air, _ := strconv.ParseFloat(w.pointAir.Text, 64)
	if rpm <= 0 || air <= 0 {
		return
	}
	// nearest rpm row, so the closed-loop limit lookup matches the map
	row := 0
	for i, v := range w.rpmAxis {
		if v <= rpm {
			row = i
		}
	}
	pt := Point{RPM: rpm, Air: air, Lambda: w.lambda(row, air)}
	pt.Ign = interp2(w.rpmAxis, w.airAxis, w.ignMap, rpm, air)
	r := w.model.Trace(pt)

	bar := make([]float64, len(r.P))
	for i, p := range r.P {
		bar[i] = p / 1e5
	}
	w.graphBox.Objects = []fyne.CanvasObject{graph2d.New(&graph2d.Config{
		AxisData:       r.Theta,
		Values:         bar,
		AxisPrecision:  0,
		ValuePrecision: 1,
		AxisLabel:      "crank angle (° ATDC)",
		ColorblindMode: w.cfg.Colorblind,
	})}
	w.graphBox.Refresh()

	mbt := w.model.MBT(pt, w.crit)
	// The three descriptors at MBT are the sanity check on the model: the
	// literature puts them at roughly 9° (45% burned), 10° (50%) and 14-16°
	// (peak pressure) ATDC. If this row is far off, the burn angles are.
	at := pt
	at.Ign = mbt
	rm := w.model.Trace(at)

	w.pointInfo.SetText(fmt.Sprintf(
		"map %.1f° BTDC, MBT %.1f° BTDC (%+.1f) · λ %.2f · burn %.0f/%.0f CAD · "+
			"peak %.0f bar @ %.0f° ATDC · IMEP %.1f bar, %.1f%% below MBT\n"+
			"at MBT: 45%% burned %.1f°, 50%% burned %.1f°, peak pressure %.0f° ATDC "+
			"(literature: 9°, 10°, 14-16°)",
		pt.Ign, mbt, mbt-pt.Ign, pt.Lambda, r.Dev, r.Fast,
		r.PPA/1e5, r.PPL, r.IMEP/1e5, w.model.IMEPLoss(pt, w.crit)*100,
		rm.MFB45, rm.MFB50, rm.PPL))
}

// fit scales the burn model so predicted MBT matches the map in the
// part-load region, where the calibration is at MBT rather than knock
// limited.
func (w *Widget) fit() {
	var pts []Point
	for y := range w.rpmAxis {
		for x := range w.airAxis {
			if pt := w.point(y, x); pt.Air <= w.fitTo && w.usable(pt) {
				pts = append(pts, pt)
			}
		}
	}
	if len(pts) == 0 {
		w.status.SetText(fmt.Sprintf("no cells between %d and %.0f mg/c above %d rpm to fit against",
			minUsableAir, w.fitTo, minUsableRPM))
		return
	}
	w.model.Burn.Scale = Fit(w.model, pts, w.crit)
	if w.eScale != nil {
		// fires OnChanged, which recalcs
		w.eScale.SetText(strconv.FormatFloat(w.model.Burn.Scale, 'f', 3, 64))
	}
	w.recalc()
	w.status.SetText(fmt.Sprintf("%s (burn scale fitted to %.3f over %d cells, %d-%.0f mg/c above %d rpm)",
		w.status.Text, w.model.Burn.Scale, len(pts), minUsableAir, w.fitTo, minUsableRPM))
}

// interp2 is a bilinear lookup into a row-major [len(ys)][len(xs)] map,
// clamped at the axis ends like the ECU's own interpolation.
func interp2(ys, xs, z []float64, y, x float64) float64 {
	iy, fy := bracket(ys, y)
	ix, fx := bracket(xs, x)
	at := func(r, c int) float64 { return z[r*len(xs)+c] }
	top := at(iy, ix)*(1-fx) + at(iy, ix+1)*fx
	bot := at(iy+1, ix)*(1-fx) + at(iy+1, ix+1)*fx
	return top*(1-fy) + bot*fy
}

// bracket returns the lower index and the 0..1 fraction into that interval.
func bracket(axis []float64, v float64) (int, float64) {
	if len(axis) < 2 {
		return 0, 0
	}
	if v <= axis[0] {
		return 0, 0
	}
	if v >= axis[len(axis)-1] {
		return len(axis) - 2, 1
	}
	for i := 0; i < len(axis)-1; i++ {
		if v < axis[i+1] {
			return i, (v - axis[i]) / (axis[i+1] - axis[i])
		}
	}
	return len(axis) - 2, 1
}
