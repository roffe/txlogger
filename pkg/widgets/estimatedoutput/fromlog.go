package estimatedoutput

import (
	"fmt"
	"math"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/widgets"
)

// Street-dyno mode: extract the WOT pull from a recorded log and compute
// measured power from dv/dt (plus, when the ECU logs airmass, a second
// torque curve through the same airmass model the estimator uses).

// per-ECU log signal names; values are as recorded (rpm, km/h, mg/c).
// speedAlt is preferred over speed when logged: on T7/T8 In.v_Vehicle is
// the left front (driven) wheel, so wheelspin corrupts dv/dt, while
// In.v_Vehicle2 is the undriven rear wheel. throttle gates pull detection
// on (near) full pedal so brisk part-throttle driving is not mistaken for
// a WOT pull; its scale differs per ECU, so gating is relative (wotMask).
type logSignals struct {
	rpm, speed, speedAlt, airmass, throttle string
}

var logSignalsByECU = map[string]logSignals{
	"T5": {rpm: "Rpm", speed: "Bil_hast", throttle: "Medeltrot"},
	"T7": {rpm: "ActualIn.n_Engine", speed: "In.v_Vehicle", speedAlt: "In.v_Vehicle2", airmass: "MAF.m_AirInlet", throttle: "Out.X_AccPedal"},
	"T8": {rpm: "ActualIn.n_Engine", speed: "In.v_Vehicle", speedAlt: "In.v_Vehicle2", airmass: "MAF.m_AirInlet", throttle: "Out.X_AccPos"},
}

// logParams are the street-dyno inputs for the dv/dt power calculation.
type logParams struct {
	MassKg  float64 // vehicle + driver
	LossPct float64 // drivetrain loss, wheel -> crank
	Cd      float64 // drag coefficient
	AreaM2  float64 // frontal area, m²
	Crr     float64 // rolling resistance coefficient
}

var defaultLogParams = logParams{MassKg: 1500, LossPct: 15, Cd: 0.31, AreaM2: 2.1, Crr: 0.012}

const (
	airDensity = 1.225 // kg/m³, sea level 15°C
	gravity    = 9.81

	pullMinSlope    = 250.0 // rpm/s: below this the engine is not pulling (no throttle info)
	pullMinSlopeWOT = 120.0 // rpm/s when throttle-gated: catches 4th/5th gear pulls too
	pullMinSpan     = 1000.0
	rpmWindow       = 250.0 // half-width of the per-point sample window
)

// logPull is the extracted acceleration segment of a log.
type logPull struct {
	t, v, rpm, air []float64 // seconds, m/s, rpm, mg/c (air may be NaN)
	hasAir         bool
}

func extractPulls(lf logfile.Logfile, sig logSignals) ([]*logPull, error) {
	var t, v, rpm, air, thr []float64
	n := lf.Len()
	speedKey := sig.speed
	if sig.speedAlt != "" {
		// prefer the undriven rear wheel only when the channel is alive:
		// a dead rear ABS ring (common) reads a constant 0 and would turn
		// the whole dyno curve into ~0 hp. Require it to vary and to
		// roughly agree with the primary speed when both are logged.
		var sumA, sumS float64
		minA, maxA := math.Inf(1), math.Inf(-1)
		var cntA, cntBoth int
		for i := 0; i < n; i++ {
			rec := lf.RecordAt(i)
			a, okA := rec.Values[sig.speedAlt]
			if !okA {
				continue
			}
			cntA++
			minA = math.Min(minA, a)
			maxA = math.Max(maxA, a)
			if s, okS := rec.Values[sig.speed]; okS {
				cntBoth++
				sumA += a
				sumS += s
			}
		}
		if cntA > 0 && maxA > minA && (cntBoth == 0 || (sumA > 0.8*sumS && sumA < 1.2*sumS)) {
			speedKey = sig.speedAlt
		}
	}
	var start bool
	var t0 float64
	for i := 0; i < n; i++ {
		rec := lf.RecordAt(i)
		r, ok := rec.Values[sig.rpm]
		s, ok2 := rec.Values[speedKey]
		if !ok || !ok2 {
			continue
		}
		ts := float64(rec.Time.UnixMilli()) / 1000
		if !start {
			t0 = ts
			start = true
		}
		t = append(t, ts-t0)
		rpm = append(rpm, r)
		v = append(v, s/3.6)
		a, okA := rec.Values[sig.airmass]
		if !okA {
			a = math.NaN()
		}
		air = append(air, a)
		x, okT := rec.Values[sig.throttle]
		if !okT {
			x = math.NaN()
		}
		thr = append(thr, x)
	}
	if len(t) < 10 {
		return nil, fmt.Errorf("log has no usable %s/%s samples", sig.rpm, sig.speed)
	}
	return findPulls(t, v, rpm, air, thr)
}

// wotMask flags samples at (near) full throttle. Scales differ per ECU
// (T7/T8 log percent, T5 Medeltrot is raw-34), so the WOT level is taken
// from the log itself: the 98th percentile of the channel, spike-proof.
// Returns nil (detection falls back to rpm slope alone) when the channel
// is absent or patchy.
// ponytail: a log with no true WOT pull gates on its own best effort;
// absolute per-ECU thresholds if that ever misleads.
func wotMask(thr []float64, n int) []bool {
	vals := make([]float64, 0, len(thr))
	for _, x := range thr {
		if !math.IsNaN(x) {
			vals = append(vals, x)
		}
	}
	if len(vals) < n/2 {
		return nil
	}
	sort.Float64s(vals)
	ref := vals[len(vals)*98/100]
	if ref <= 0 {
		return nil
	}
	wot := make([]bool, n)
	for i, x := range thr {
		wot[i] = math.IsNaN(x) || x >= 0.85*ref
	}
	return wot
}

// findPulls returns every segment (in log order) where the engine keeps
// pulling (smoothed rpm slope above the threshold, dropouts up to half a
// second allowed) over at least pullMinSpan rpm. With a usable throttle
// channel, samples must also be at WOT and the slope threshold drops so
// tall-gear pulls (~170 rpm/s in 5th) are kept while part-throttle
// driving is rejected.
func findPulls(t, v, rpm, air, thr []float64) ([]*logPull, error) {
	n := len(t)
	wot := wotMask(thr, n)
	minSlope := pullMinSlope
	if wot != nil {
		minSlope = pullMinSlopeWOT
	}
	pulling := make([]bool, n)
	for i := 0; i < n; i++ {
		if wot != nil && !wot[i] {
			continue
		}
		j := i
		for j < n-1 && t[j]-t[i] < 0.3 {
			j++
		}
		if j == i || t[j] == t[i] {
			continue
		}
		pulling[i] = (rpm[j]-rpm[i])/(t[j]-t[i]) > minSlope
	}
	var pulls []*logPull
	for i := 0; i < n; {
		if !pulling[i] {
			i++
			continue
		}
		last := i
		j := i
		for j < n-1 {
			j++
			if rpm[j] < rpm[last]-300 {
				break // gear change or lift-off: never bridge an rpm drop
			}
			if pulling[j] {
				last = j
				continue
			}
			if t[j]-t[last] > 0.5 {
				break
			}
		}
		if rpm[last]-rpm[i] >= pullMinSpan {
			p := &logPull{
				t:   t[i : last+1],
				v:   v[i : last+1],
				rpm: rpm[i : last+1],
				air: air[i : last+1],
			}
			for _, a := range p.air {
				if !math.IsNaN(a) {
					p.hasAir = true
					break
				}
			}
			pulls = append(pulls, p)
		}
		i = j + 1
	}
	if len(pulls) == 0 {
		return nil, fmt.Errorf("no WOT pull found — record several seconds of continuously rising rpm in one gear")
	}
	return pulls, nil
}

func (p *logPull) info() string {
	return fmt.Sprintf("pull: %.0f-%.0f rpm, %.1f s", p.rpm[0], p.rpm[len(p.rpm)-1], p.t[len(p.t)-1]-p.t[0])
}

// curves resamples the pull onto the estimator's rpm axis. Points the pull
// does not cover stay NaN and are skipped by the graph.
func (p *logPull) curves(prm logParams, rpmAxis []float64, e85 bool, eff float64) []Curve {
	if prm.MassKg <= 100 {
		prm.MassKg = defaultLogParams.MassKg
	}
	if prm.LossPct < 0 || prm.LossPct >= 90 {
		prm.LossPct = defaultLogParams.LossPct
	}
	if prm.Cd < 0 {
		prm.Cd = 0
	}
	if prm.AreaM2 < 0 {
		prm.AreaM2 = 0
	}
	if prm.Crr < 0 {
		prm.Crr = 0
	}
	power := nanSlice(len(rpmAxis))
	torque := nanSlice(len(rpmAxis))
	airPower := nanSlice(len(rpmAxis))
	airTorque := nanSlice(len(rpmAxis))
	for k, r := range rpmAxis {
		var ts, vs []float64
		var airSum float64
		var airN int
		for i := range p.rpm {
			if math.Abs(p.rpm[i]-r) > rpmWindow {
				continue
			}
			ts = append(ts, p.t[i])
			vs = append(vs, p.v[i])
			if !math.IsNaN(p.air[i]) {
				airSum += p.air[i]
				airN++
			}
		}
		if len(ts) >= 3 {
			accel, vMean := slopeAndMean(ts, vs)
			if !math.IsNaN(accel) {
				pWheel := prm.MassKg*accel*vMean +
					0.5*airDensity*prm.Cd*prm.AreaM2*vMean*vMean*vMean +
					prm.Crr*prm.MassKg*gravity*vMean
				pCrank := pWheel / (1 - prm.LossPct/100)
				nm := pCrank * 60 / (2 * math.Pi * r)
				torque[k] = nm
				power[k] = nm * r / 7024
			}
		}
		if airN > 0 {
			tq := airSum / float64(airN) / 3.1
			if e85 {
				tq *= 1.07
			}
			tq *= rpmCorrection(int(r)) * eff
			airTorque[k] = tq
			airPower[k] = tq * r / 7024
		}
	}
	curves := []Curve{
		{Name: "Log power (hp)", Values: power, Peak: true},
		{Name: "Log torque (Nm)", Values: torque, Peak: true},
	}
	if p.hasAir {
		curves = append(curves,
			Curve{Name: "Log air power (hp)", Values: airPower, Hidden: true},
			Curve{Name: "Log air torque (Nm)", Values: airTorque, Hidden: true},
		)
	}
	return curves
}

// averagePullCurves averages same-named curves across pulls, per rpm point,
// skipping points a pull does not cover.
func averagePullCurves(pulls []*logPull, prm logParams, rpmAxis []float64, e85 bool, eff float64) []Curve {
	type acc struct {
		curve Curve
		sum   []float64
		n     []int
	}
	var order []*acc
	accs := map[string]*acc{}
	for _, p := range pulls {
		for _, c := range p.curves(prm, rpmAxis, e85, eff) {
			a := accs[c.Name]
			if a == nil {
				a = &acc{curve: c, sum: make([]float64, len(rpmAxis)), n: make([]int, len(rpmAxis))}
				accs[c.Name] = a
				order = append(order, a)
			}
			for i, v := range c.Values {
				if !math.IsNaN(v) {
					a.sum[i] += v
					a.n[i]++
				}
			}
		}
	}
	out := make([]Curve, len(order))
	for k, a := range order {
		vals := nanSlice(len(rpmAxis))
		for i := range vals {
			if a.n[i] > 0 {
				vals[i] = a.sum[i] / float64(a.n[i])
			}
		}
		a.curve.Values = vals
		out[k] = a.curve
	}
	return out
}

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// slopeAndMean is a least-squares fit of v over t: acceleration and mean speed.
func slopeAndMean(t, v []float64) (float64, float64) {
	var tm, vm float64
	for i := range t {
		tm += t[i]
		vm += v[i]
	}
	tm /= float64(len(t))
	vm /= float64(len(v))
	var num, den float64
	for i := range t {
		num += (t[i] - tm) * (v[i] - vm)
		den += (t[i] - tm) * (t[i] - tm)
	}
	if den == 0 {
		return math.NaN(), vm
	}
	return num / den, vm
}

// loadLog lets the user pick a log file and extracts the pulls from it.
func (w *Widget) loadLog() {
	sig, ok := logSignalsByECU[w.cfg.ECU]
	if !ok {
		w.logInfo.SetText("no log signals defined for " + w.cfg.ECU)
		return
	}
	widgets.SelectFile(func(r fyne.URIReadCloser) {
		defer r.Close()
		lf, err := logfile.Open(r.URI().Name(), r)
		var pulls []*logPull
		if err == nil {
			pulls, err = extractPulls(lf, sig)
			lf.Close()
		}
		fyne.Do(func() {
			if err != nil {
				w.logInfo.SetText(err.Error())
				return
			}
			w.pulls = pulls
			w.pullIdx = 0
			opts := make([]string, len(pulls))
			for i, p := range pulls {
				opts[i] = fmt.Sprintf("Pull %d: %s", i+1, p.info())
			}
			if len(pulls) > 1 {
				opts = append(opts, fmt.Sprintf("Average of %d pulls", len(pulls)))
			}
			w.pullSel.Options = opts
			w.pullSel.Selected = opts[0]
			w.pullSel.Refresh()
			w.logInfo.SetText(fmt.Sprintf("%d pull(s) found", len(pulls)))
			w.recalc()
		})
	}, "Log file", "t5l", "t7l", "t8l", "csv", "bpl")
}

// buildLogOptions is the street-dyno section of the options panel.
func (w *Widget) buildLogOptions() []fyne.CanvasObject {
	w.logInfo = widget.NewLabel("")
	w.logInfo.Wrapping = fyne.TextWrapWord
	w.pullSel = widget.NewSelect(nil, func(string) {
		i := w.pullSel.SelectedIndex()
		if i >= len(w.pulls) {
			i = -1 // the "Average of N pulls" entry
		}
		w.pullIdx = i
		w.recalc()
	})
	items := []fyne.CanvasObject{
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Street dyno from log", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	}
	items = append(items, w.numField("Vehicle mass incl. driver (kg)", &w.logPrm.MassKg)...)
	items = append(items, w.numField("Drivetrain loss (%)", &w.logPrm.LossPct)...)
	items = append(items, w.numField("Drag coefficient (Cd)", &w.logPrm.Cd)...)
	items = append(items, w.numField("Frontal area (m²)", &w.logPrm.AreaM2)...)
	items = append(items, w.numField("Rolling resistance", &w.logPrm.Crr)...)
	items = append(items,
		widget.NewButtonWithIcon("Load log", theme.FolderOpenIcon(), w.loadLog),
		widget.NewButton("Clear log", func() {
			w.pulls = nil
			w.pullSel.Options = nil
			w.pullSel.ClearSelected()
			w.logInfo.SetText("")
			w.recalc()
		}),
		w.pullSel,
		w.logInfo,
	)
	return items
}
