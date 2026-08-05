package boosttuner

import (
	"fmt"
	"math"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// PID heuristics. The PID gain maps cannot be cleanly auto-learned from arbitrary
// logs, so instead we measure the *quality* of logged boost transients and
// suggest bounded per-rpm-band scalings of the maps. These are starting points to
// review and validate in the Simulator, never an automatic flash.
//
// Error convention: loadDiff = SetValue - airmass, so a positive error means we
// are under the request (still spooling) and a negative error means airmass has
// exceeded the request (overshoot).

// transientCfg tunes the boost-onset detector.
type transientCfg struct {
	startErr   float64 // onset: error above this (mg/c) with the loop engaged
	settleBand float64 // |error| within this counts as "on target" (mg/c)
	minLen     int     // ignore events shorter than this many samples
}

func defaultTransientCfg() transientCfg {
	return transientCfg{startErr: 100, settleBand: 20, minLen: 5}
}

// transient summarises one boost-onset event.
type transient struct {
	rpm         float64 // mean rpm over the event
	overshoot   float64 // max airmass-over-request after onset (mg/c, >=0)
	crossings   int     // sign changes of the error (oscillation)
	riseSamples int     // samples from onset until first settle (or event length)
	settled     bool
	ssError     float64 // mean error in the settled tail (mg/c)
}

// pidSuggestInputs holds the row-aligned channels the detector consumes.
type pidSuggestInputs struct {
	n                      int
	rpm, loadDiff, pwm2pct []float64
}

// detectTransients walks the log and extracts boost-onset events: a stretch where
// the loop is engaged and the error starts large-positive, tracked until the loop
// disengages or the log ends.
func detectTransients(in pidSuggestInputs, cfg transientCfg) []transient {
	var out []transient
	active := false
	var onset, settleAt int
	var sumRPM, maxNeg, prevSign float64
	var crossings, length int

	finalize := func(end int) {
		if !active {
			return
		}
		active = false
		if length < cfg.minLen {
			return
		}
		t := transient{
			rpm:       sumRPM / float64(length),
			overshoot: maxNeg,
			crossings: crossings,
			settled:   settleAt >= 0,
		}
		if settleAt >= 0 {
			t.riseSamples = settleAt - onset
			// Steady-state error: mean over the settled tail.
			var s float64
			var c int
			for i := settleAt; i < end; i++ {
				if !math.IsNaN(in.loadDiff[i]) {
					s += in.loadDiff[i]
					c++
				}
			}
			if c > 0 {
				t.ssError = s / float64(c)
			}
		} else {
			t.riseSamples = length
		}
		out = append(out, t)
	}

	for i := 0; i < in.n; i++ {
		e, r, p := in.loadDiff[i], in.rpm[i], in.pwm2pct[i]
		if math.IsNaN(e) || math.IsNaN(r) || math.IsNaN(p) {
			finalize(i)
			continue
		}
		engaged := p == 0
		if !active {
			if engaged && e > cfg.startErr {
				active = true
				onset, settleAt = i, -1
				sumRPM, maxNeg, crossings, length = 0, 0, 0, 0
				prevSign = 1 // onset error is positive
			} else {
				continue
			}
		}
		if !engaged {
			finalize(i)
			continue
		}
		sumRPM += r
		length++
		if sign := signOf(e); sign != 0 && sign != prevSign {
			crossings++
			prevSign = sign
		}
		if e < 0 && -e > maxNeg {
			maxNeg = -e
		}
		if settleAt < 0 && math.Abs(e) <= cfg.settleBand {
			settleAt = i
		}
	}
	finalize(in.n)
	return out
}

// suggestCfg tunes how metrics map to scaling factors.
type suggestCfg struct {
	overshootHi float64 // mg/c above which we trim P & D
	riseHi      int     // rise samples above which we add P
	ssHi        float64 // |steady error| above which we add I
	trim        float64 // factor applied when trimming (e.g. 0.85)
	boost       float64 // factor applied when adding (e.g. 1.15)
}

func defaultSuggestCfg() suggestCfg {
	return suggestCfg{overshootHi: 50, riseHi: 25, ssHi: 20, trim: 0.85, boost: 1.15}
}

// bandSuggestion is a per-rpm-band scaling recommendation for the PID maps.
type bandSuggestion struct {
	row     int     // PIDYSP index
	rpm     float64 // band breakpoint
	factorP float64
	factorI float64
	factorD float64
	reason  string
	events  int
}

// factorFor returns the suggested factor for the named map ("P"/"I"/"D").
func (b bandSuggestion) factorFor(name string) float64 {
	switch name {
	case "P":
		return b.factorP
	case "I":
		return b.factorI
	case "D":
		return b.factorD
	}
	return 1
}

// suggestPID aggregates transients into per-rpm-band scaling factors.
func suggestPID(ts []transient, pidYSP []float64, cfg suggestCfg) []bandSuggestion {
	type acc struct {
		overshoot, rise, ss float64
		crossings, n        int
	}
	bands := make(map[int]*acc)
	for _, t := range ts {
		row := nearestIndex(pidYSP, t.rpm)
		a := bands[row]
		if a == nil {
			a = &acc{}
			bands[row] = a
		}
		a.overshoot += t.overshoot
		a.rise += float64(t.riseSamples)
		a.ss += t.ssError
		a.crossings += t.crossings
		a.n++
	}

	var out []bandSuggestion
	for row, a := range bands {
		n := float64(a.n)
		avgOver := a.overshoot / n
		avgRise := a.rise / n
		avgSS := a.ss / n
		avgCross := float64(a.crossings) / n

		b := bandSuggestion{row: row, rpm: pidYSP[row], factorP: 1, factorI: 1, factorD: 1, events: a.n}
		switch {
		case avgOver > cfg.overshootHi || avgCross >= 2:
			b.factorP = cfg.trim
			b.factorD = cfg.trim
			b.reason = fmt.Sprintf("overshoot %.0f mg/c, %.1f crossings", avgOver, avgCross)
		case avgRise > float64(cfg.riseHi):
			b.factorP = cfg.boost
			b.reason = fmt.Sprintf("slow rise (%.0f samples)", avgRise)
		}
		if math.Abs(avgSS) > cfg.ssHi {
			b.factorI = cfg.boost
			if b.reason != "" {
				b.reason += "; "
			}
			b.reason += fmt.Sprintf("steady error %.0f mg/c", avgSS)
		}
		if b.factorP != 1 || b.factorI != 1 || b.factorD != 1 {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rpm < out[j].rpm })
	return out
}

func signOf(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// --- UI wiring ---

// computePIDSuggestions runs the transient analysis over the loaded logs and
// fills each editor's suggestion panel. Returns a user-facing status string.
func (bt *BoostTuner) computePIDSuggestions() string {
	if len(bt.values) == 0 {
		return "Load logs first (Logs tab)."
	}
	rpm, ok1 := bt.series("rpm")
	loadDiff, ok2 := bt.series("loadDiff")
	pwm2pct, ok3 := bt.series("pwm2pct")
	if !ok1 || !ok2 || !ok3 {
		return "Need rpm, LoadDiff and the 2% flag channels."
	}
	ts := detectTransients(pidSuggestInputs{
		n: bt.nrecords, rpm: rpm, loadDiff: loadDiff, pwm2pct: pwm2pct,
	}, defaultTransientCfg())
	if len(ts) == 0 {
		bt.renderSuggestions(nil)
		return "No boost transients detected."
	}
	var ysp []float64
	if ed := bt.pidEditors["P"]; ed != nil {
		ysp = ed.axisY
	}
	if len(ysp) == 0 {
		return "PID maps/axes not found in this binary."
	}
	sugg := suggestPID(ts, ysp, defaultSuggestCfg())
	bt.renderSuggestions(sugg)
	return fmt.Sprintf("%d transients, %d band suggestion(s).", len(ts), len(sugg))
}

// renderSuggestions populates every editor's suggestion box with the rows
// relevant to its map.
func (bt *BoostTuner) renderSuggestions(sugg []bandSuggestion) {
	for name, ed := range bt.pidEditors {
		ed.suggestBox.Objects = ed.suggestBox.Objects[:0]
		var rows []bandSuggestion
		for _, b := range sugg {
			if b.factorFor(name) != 1 {
				rows = append(rows, b)
			}
		}
		if len(rows) == 0 {
			ed.suggestBox.Add(widget.NewLabel("No suggestions for this map."))
			ed.suggestBox.Refresh()
			continue
		}
		factors := map[int]float64{}
		for _, b := range rows {
			b := b
			factors[b.row] = b.factorFor(name)
			lbl := widget.NewLabel(fmt.Sprintf("%.0f rpm: ×%.2f (%s)", b.rpm, b.factorFor(name), b.reason))
			lbl.Wrapping = fyne.TextWrapWord
			apply := widget.NewButton("Apply", func() {
				ed.scaleRows(map[int]float64{b.row: b.factorFor(name)})
			})
			apply.Importance = widget.LowImportance
			ed.suggestBox.Add(container.NewBorder(nil, nil, nil, apply, lbl))
		}
		allBtn := widget.NewButton("Apply all", func() { ed.scaleRows(factors) })
		allBtn.Importance = widget.LowImportance
		ed.suggestBox.Add(allBtn)
		ed.suggestBox.Refresh()
	}
}
