package camtiming

import (
	"fmt"
	"math"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

// The measured-VE tab. What a camshaft actually does to an engine is fill
// the cylinder better or worse at a given rpm, and the ECU already logs
// everything needed to see that: airmass per combustion, manifold
// pressure and inlet air temperature. Everything here is measured, in
// contrast with the timing diagram next door which is all specification.

const (
	cylinders  = 4      // every engine in this list is a four
	rGasAir    = 287.05 // J/(kg·K)
	veBinWidth = 250.0  // rpm
	veMinBin   = 3      // samples needed before a bin is plotted
	veRPMMin   = 1000.0
	veRPMMax   = 7000.0
)

type veSignals struct{ rpm, air, map_, iat, pedal string }

// The airmass channel is MAF.m_AirInlet, the mass the ECU actually
// measured per combustion — deliberately not m_Request, which is the
// airmass the torque demand asked for and says nothing about what the
// engine swallowed. Worth knowing that on a T7 with the MAF disabled
// m_AirInlet falls back to the pressure model, and VE then measures that
// model rather than the engine.
//
// T5 is missing on purpose: it logs no airmass, so there is nothing to
// divide by the reference mass.
var veSignalsByECU = map[string]veSignals{
	"T7": {"ActualIn.n_Engine", "MAF.m_AirInlet", "ActualIn.p_AirInlet", "ActualIn.T_AirInlet", "Out.X_AccPedal"},
	"T8": {"ActualIn.n_Engine", "MAF.m_AirInlet", "In.p_AirInlet", "ActualIn.T_AirInlet", "Out.X_AccPos"},
}

type veSample struct{ rpm, air, bar, iat, pedal float64 }

type veTab struct {
	w              *Widget
	disp, minPedal *numericentry.Widget
	info, camInfo  *widget.Label
	chart          *chart
	samples        []veSample
	content        fyne.CanvasObject
}

func newVETab(w *Widget) *veTab {
	v := &veTab{
		w:        w,
		disp:     numericentry.New("2290"),
		minPedal: numericentry.New("80"),
		info:     widget.NewLabel("Load a log to measure volumetric efficiency."),
		camInfo:  widget.NewLabel(""),
		chart:    newChart("RPM", "volumetric efficiency %", veRPMMin, veRPMMax, 1000),
	}
	v.chart.yFloat = true
	v.chart.xUnit, v.chart.yUnit, v.chart.yDec, v.chart.xSnap = "rpm", "%", 1, 50
	v.info.Wrapping = fyne.TextWrapWord
	v.camInfo.Wrapping = fyne.TextWrapWord
	for _, e := range []*numericentry.Widget{v.disp, v.minPedal} {
		e.OnChanged = func(string) { v.recalc() }
	}

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("Displacement (cc)"), v.disp,
		widget.NewLabel("Min pedal (%)"), v.minPedal,
	)
	note := widget.NewLabel("VE is airmass per combustion over the mass that would fill one " +
		"cylinder at the logged manifold pressure and inlet temperature. Over 100% means the " +
		"cylinder fills better than that, which a turbo engine with overlap does. Heat soaked " +
		"inlet air temperature reads VE high.")
	note.Wrapping = fyne.TextWrapWord

	left := container.NewVScroll(container.NewVBox(
		widget.NewButtonWithIcon("Load log", theme.FolderOpenIcon(), v.load),
		widget.NewButton("Clear log", func() {
			v.samples = nil
			v.info.SetText("Load a log to measure volumetric efficiency.")
			v.recalc()
		}),
		form,
		v.info,
		widget.NewSeparator(),
		v.camInfo,
		note,
	))
	left.SetMinSize(fyne.NewSize(290, 100))
	v.content = container.NewBorder(nil, nil, left, nil, v.chart)
	return v
}

func (v *veTab) setCamInfo(s string) { v.camInfo.SetText(s) }

func (v *veTab) load() {
	sig, ok := veSignalsByECU[v.w.cfg.ECU]
	if !ok {
		v.info.SetText(fmt.Sprintf("Measured VE needs airmass, manifold pressure and inlet air "+
			"temperature in the log; %q does not log all three.", v.w.cfg.ECU))
		return
	}
	widgets.SelectFile(func(r fyne.URIReadCloser) {
		defer r.Close()
		lf, err := logfile.Open(r.URI().Name(), r)
		var samples []veSample
		if err == nil {
			samples = readSamples(lf, sig)
			lf.Close()
		}
		fyne.Do(func() {
			if err != nil {
				v.info.SetText(err.Error())
				return
			}
			v.samples = samples
			if len(samples) == 0 {
				v.info.SetText("no usable records: the log needs " + sig.rpm + ", " +
					sig.air + ", " + sig.map_ + " and " + sig.iat)
			}
			v.recalc()
		})
	}, "Log file", "t5l", "t7l", "t8l", "csv", "bpl")
}

// readSamples pulls the four channels VE needs out of every record that
// carries them all.
func readSamples(lf logfile.Logfile, sig veSignals) []veSample {
	var out []veSample
	for i := 0; i < lf.Len(); i++ {
		vals := lf.RecordAt(i).Values
		rpm, ok1 := vals[sig.rpm]
		air, ok2 := vals[sig.air]
		p, ok3 := vals[sig.map_]
		t, ok4 := vals[sig.iat]
		if !ok1 || !ok2 || !ok3 || !ok4 || rpm <= 0 || air <= 0 {
			continue
		}
		out = append(out, veSample{rpm: rpm, air: air, bar: p, iat: t, pedal: vals[sig.pedal]})
	}
	// txlogger records manifold pressure in bar, but a csv exported from
	// another tool may hold kPa. Scale by what the numbers themselves say:
	// nothing runs a 10 bar manifold.
	if med := median(pressures(out)); med > 10 {
		for i := range out {
			out[i].bar /= 100
		}
	}
	return out
}

func (v *veTab) recalc() {
	disp := parse(v.disp)
	minPedal := parse(v.minPedal)
	if len(v.samples) == 0 || disp <= 0 {
		v.chart.set(nil, nil, nil)
		return
	}

	bins := map[int][]float64{}
	var used int
	for _, s := range v.samples {
		if s.pedal < minPedal {
			continue
		}
		ve := veFrom(s.air, s.bar, s.iat, disp)
		if ve <= 0 {
			continue
		}
		used++
		bin := int(math.Round(s.rpm / veBinWidth))
		bins[bin] = append(bins[bin], ve)
	}

	var pts []xy
	for bin, vals := range bins {
		rpm := float64(bin) * veBinWidth
		if len(vals) < veMinBin || rpm < veRPMMin || rpm > veRPMMax {
			continue
		}
		pts = append(pts, xy{rpm, median(vals)})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].x < pts[j].x })

	if len(pts) == 0 {
		v.info.SetText(fmt.Sprintf("%d samples, none above %.0f%% pedal with %d+ per 250 rpm bin",
			len(v.samples), minPedal, veMinBin))
		v.chart.set(nil, nil, nil)
		return
	}
	peak := pts[0]
	for _, p := range pts {
		if p.y > peak.y {
			peak = p
		}
	}
	v.info.SetText(fmt.Sprintf("%d of %d samples above %.0f%% pedal — peak VE %.0f%% @ %.0f rpm",
		used, len(v.samples), minPedal, peak.y, peak.x))
	v.chart.set([]series{{name: "VE", col: veColor, pts: pts}}, nil, nil)
}

// veFrom is volumetric efficiency in percent: the logged airmass per
// combustion over the mass of air one cylinder would hold at the logged
// manifold pressure (bar absolute) and inlet temperature (°C).
func veFrom(airMG, bar, iatC, dispCC float64) float64 {
	if bar <= 0 || iatC <= -273 || dispCC <= 0 {
		return 0
	}
	rho := bar * 1e5 / (rGasAir * (iatC + 273.15)) // kg/m³
	refMG := rho * (dispCC / cylinders * 1e-6) * 1e6
	if refMG <= 0 {
		return 0
	}
	return 100 * airMG / refMG
}

func pressures(s []veSample) []float64 {
	out := make([]float64, len(s))
	for i := range s {
		out[i] = s[i].bar
	}
	return out
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}
