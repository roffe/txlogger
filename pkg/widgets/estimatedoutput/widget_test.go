package estimatedoutput

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets/numericentry"
)

// walk visits o and everything inside its containers and scrollers.
func walk(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(o)
	switch c := o.(type) {
	case *fyne.Container:
		for _, ch := range c.Objects {
			walk(ch, fn)
		}
	case *container.Scroll:
		walk(c.Content, fn)
	}
}

func findCheck(t *testing.T, root fyne.CanvasObject, label string) *widget.Check {
	t.Helper()
	var found *widget.Check
	walk(root, func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok && c.Text == label {
			found = c
		}
	})
	if found == nil {
		t.Fatalf("no check %q", label)
	}
	return found
}

func findButton(t *testing.T, root fyne.CanvasObject, label string) *widget.Button {
	t.Helper()
	var found *widget.Button
	walk(root, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && b.Text == label {
			found = b
		}
	})
	if found == nil {
		t.Fatalf("no button %q", label)
	}
	return found
}

// entryAfter returns the numeric entry that follows the label in its box.
func entryAfter(t *testing.T, root fyne.CanvasObject, label string) *numericentry.Widget {
	t.Helper()
	var found *numericentry.Widget
	walk(root, func(o fyne.CanvasObject) {
		c, ok := o.(*fyne.Container)
		if !ok {
			return
		}
		for i, ch := range c.Objects {
			if l, ok := ch.(*widget.Label); ok && l.Text == label && i+1 < len(c.Objects) {
				found, _ = c.Objects[i+1].(*numericentry.Widget)
			}
		}
	})
	if found == nil {
		t.Fatalf("no entry after %q", label)
	}
	return found
}

func legendNames(w *Widget) []string {
	var names []string
	for _, o := range w.legend.Objects {
		names = append(names, o.(*fyne.Container).Objects[0].(*widget.Check).Text)
	}
	return names
}

func newT7Widget(t *testing.T, fw symbol.FirmwareFile) *Widget {
	t.Helper()
	test.NewTempApp(t)
	return New(&Config{ECU: "T7", GetFW: func() symbol.FirmwareFile { return fw }})
}

// the T7 fixture's WOT summary with default options (see TestT7MatchesSuite)
const t7DefaultSummary = "peak Power (hp) 259 @ 5200 rpm · peak Torque (Nm) 373 @ 3000 rpm · airmass (knock) limiter @ 1000 rpm · gear torque limiter @ 3000 rpm · airmass (knock) limiter 5200-6000 rpm"

// TestWidgetUnknownECU shows a notice instead of the estimator.
func TestWidgetUnknownECU(t *testing.T) {
	test.NewTempApp(t)
	w := New(&Config{ECU: "ME9"})
	var texts []string
	walk(test.WidgetRenderer(w).Objects()[0], func(o fyne.CanvasObject) {
		if l, ok := o.(*widget.Label); ok {
			texts = append(texts, l.Text)
		}
	})
	if !slices.Equal(texts, []string{"No output estimator for ECU ME9"}) {
		t.Fatalf("labels = %q", texts)
	}
	if w.graph != nil || w.calc != nil {
		t.Fatal("estimator built for an unknown ECU")
	}
}

// TestWidgetInitialState: the options panel starts from each option's
// default, the summary reports the WOT result and the legend lists every
// curve with hidden ones unchecked.
func TestWidgetInitialState(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).fw())
	for k, want := range map[string]float64{"fwlimit": 1, "gear": 5, "ambient": 100, "efficiency": 100, "automatic": 0} {
		if w.opts[k] != want {
			t.Errorf("opts[%s] = %v, want %v", k, w.opts[k], want)
		}
	}
	if w.summary.Text != t7DefaultSummary {
		t.Fatalf("summary =\n%q\nwant\n%q", w.summary.Text, t7DefaultSummary)
	}
	if got := legendNames(w); !slices.Equal(got, []string{"Power (hp)", "Torque (Nm)", "Injector DC (%)", "Target lambda (*100)", "Fuel flow (l/h)"}) {
		t.Fatalf("legend = %q", got)
	}
	for _, o := range w.legend.Objects {
		chk := o.(*fyne.Container).Objects[0].(*widget.Check)
		if chk.Checked == (chk.Text == "Fuel flow (l/h)") {
			t.Errorf("legend %q checked = %v", chk.Text, chk.Checked)
		}
	}
	if !findCheck(t, w.content, "Firmware limited (400/350 Nm)").Checked {
		t.Error("firmware limit check not preset")
	}
	if len(w.graph.series) != 5 || len(w.graph.rpm) != 4 || w.table.data == nil {
		t.Fatal("graph/table not fed")
	}
	if got := test.WidgetRenderer(w).Objects(); len(got) != 1 || got[0] != w.content {
		t.Fatal("renderer does not show the content")
	}
}

// TestWidgetDisabledOptions: features the binary lacks are disabled.
func TestWidgetDisabledOptions(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).set(bsym(t, "TorqueCal.EnableOverBoost", 0)).del("TorqueCal.M_CabGearLim").fw())
	if !findCheck(t, w.content, "View in overboost").Disabled() || !findCheck(t, w.content, "Convertible").Disabled() {
		t.Fatal("overboost/convertible enabled without their tables")
	}
	if findCheck(t, w.content, "E85 fuel").Disabled() {
		t.Fatal("E85 disabled on a biopower binary")
	}
}

// TestWidgetDisabledChoice: a disabled choice option renders a disabled
// select.
func TestWidgetDisabledChoice(t *testing.T) {
	test.NewTempApp(t)
	w := New(&Config{ECU: "T7", GetFW: func() symbol.FirmwareFile { return t7Syms(t).fw() }})
	w.calc = choiceCalc{}
	box := w.buildOptions()
	var sel *widget.Select
	walk(box, func(o fyne.CanvasObject) {
		if s, ok := o.(*widget.Select); ok {
			sel = s
		}
	})
	if sel == nil || !sel.Disabled() || sel.SelectedIndex() != 1 {
		t.Fatalf("select = %+v", sel)
	}
}

type choiceCalc struct{ t7 }

func (choiceCalc) Options(symbol.FirmwareFile) []Option {
	return []Option{{Key: "c", Label: "Choice", Kind: OptionChoice, Choices: []string{"a", "b"}, Default: 1, Disabled: true}}
}

// TestWidgetBoolOptionRecalculates: toggling a check updates the option
// and the result.
func TestWidgetBoolOptionRecalculates(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).fw())
	test.Tap(findCheck(t, w.content, "Automatic gearbox"))
	if w.opts["automatic"] != 1 {
		t.Fatalf("automatic = %v", w.opts["automatic"])
	}
	// TestT7MatchesSuite "automatic": torque 274/323/329/260, all airmass limited
	want := "peak Power (hp) 243 @ 5200 rpm · peak Torque (Nm) 329 @ 5200 rpm · airmass (knock) limiter 1000-6000 rpm"
	if w.summary.Text != want {
		t.Fatalf("summary = %q, want %q", w.summary.Text, want)
	}
	test.Tap(findCheck(t, w.content, "Automatic gearbox"))
	if w.opts["automatic"] != 0 || w.summary.Text != t7DefaultSummary {
		t.Fatalf("toggling back: opts %v summary %q", w.opts["automatic"], w.summary.Text)
	}
}

// TestWidgetChoiceAndNumberOptions: the gear select stores its index and
// numeric entries accept only parseable numbers.
func TestWidgetChoiceAndNumberOptions(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).fw())
	var gear *widget.Select
	walk(w.content, func(o fyne.CanvasObject) {
		if s, ok := o.(*widget.Select); ok && slices.Equal(s.Options, t7Gears) {
			gear = s
		}
	})
	if gear == nil || gear.Selected != "Fifth" {
		t.Fatalf("gear select = %+v", gear)
	}
	gear.SetSelected("First")
	if w.opts["gear"] != 1 || !strings.Contains(w.summary.Text, "gear torque limiter 1000-6000 rpm") {
		t.Fatalf("1st gear: opts %v summary %q", w.opts["gear"], w.summary.Text)
	}
	gear.SetSelected("Fifth")

	amb := entryAfter(t, w.content, "Ambient pressure (kPa)")
	if amb.Text != "100" {
		t.Fatalf("ambient entry = %q", amb.Text)
	}
	amb.SetText("70")
	if w.opts["ambient"] != 70 || !strings.Contains(w.summary.Text, "turbo speed limiter 3000-6000 rpm") {
		t.Fatalf("ambient 70: opts %v summary %q", w.opts["ambient"], w.summary.Text)
	}
	amb.SetText("-")
	if w.opts["ambient"] != 70 {
		t.Fatalf("unparseable text changed the option to %v", w.opts["ambient"])
	}
}

// TestWidgetCalculationError shows the estimator error in the summary.
func TestWidgetCalculationError(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).del("FCutCal.m_AirInletLimit").fw())
	if w.summary.Text != "Error: missing symbols: FCutCal.m_AirInletLimit" {
		t.Fatalf("summary = %q", w.summary.Text)
	}
	test.Tap(findButton(t, w.content, "Refresh"))
	if !strings.HasPrefix(w.summary.Text, "Error: ") {
		t.Fatalf("summary after refresh = %q", w.summary.Text)
	}
}

// TestWidgetLegendVisibility: unchecking a legend entry hides the series
// and the choice survives recalculation.
func TestWidgetLegendVisibility(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).fw())
	chk := w.legend.Objects[1].(*fyne.Container).Objects[0].(*widget.Check)
	if chk.Text != "Torque (Nm)" {
		t.Fatalf("legend[1] = %q", chk.Text)
	}
	before := w.legend.Objects
	test.Tap(chk)
	if w.visible["Torque (Nm)"] || w.graph.series[1].visible {
		t.Fatal("torque still visible")
	}
	test.Tap(findButton(t, w.content, "Refresh"))
	if w.graph.series[1].visible {
		t.Fatal("recalculation re-showed a hidden series")
	}
	if &w.legend.Objects[0] != &before[0] {
		t.Fatal("legend rebuilt although the curve set did not change")
	}
}

// TestWidgetLogPulls: loaded pulls add the log curves; the selector picks
// one pull or the average, and Clear log removes them again.
func TestWidgetLogPulls(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).fw())
	w.pulls = []*logPull{rampPull(t, 2, 1000), rampPull(t, 4, 1000)}
	w.pullIdx = 0
	w.recalc()
	want := []string{"Power (hp)", "Torque (Nm)", "Injector DC (%)", "Target lambda (*100)", "Fuel flow (l/h)", "Log power (hp)", "Log torque (Nm)", "Log air power (hp)", "Log air torque (Nm)"}
	if got := legendNames(w); !slices.Equal(got, want) {
		t.Fatalf("legend = %q", got)
	}
	single := w.graph.series[6].values[1] // 3000 rpm, first pull

	w.pullSel.Options = []string{"Pull 1", "Pull 2", "Average of 2 pulls"}
	w.pullSel.SetSelectedIndex(2)
	if w.pullIdx != -1 {
		t.Fatalf("average entry -> pullIdx %d", w.pullIdx)
	}
	avg := w.graph.series[6].values[1]
	w.pullSel.SetSelectedIndex(1)
	second := w.graph.series[6].values[1]
	if w.pullIdx != 1 || !(single < avg && avg < second) {
		t.Fatalf("pull torque@3000: first %v avg %v second %v (idx %d)", single, avg, second, w.pullIdx)
	}

	w.logInfo.SetText("2 pull(s) found")
	test.Tap(findButton(t, w.content, "Clear log"))
	if w.pulls != nil || w.pullSel.Options != nil || w.logInfo.Text != "" || len(w.legend.Objects) != 5 {
		t.Fatalf("after clear: pulls %d options %v info %q legend %d", len(w.pulls), w.pullSel.Options, w.logInfo.Text, len(w.legend.Objects))
	}
}

// TestWidgetDynoParamsPersist: street-dyno inputs start from saved
// preferences and save valid edits.
func TestWidgetDynoParamsPersist(t *testing.T) {
	a := test.NewTempApp(t)
	a.Preferences().SetFloat("dyno_mass", 1650)
	w := New(&Config{ECU: "T7", GetFW: func() symbol.FirmwareFile { return t7Syms(t).fw() }})
	if w.logPrm.MassKg != 1650 || w.logPrm.LossPct != defaultLogParams.LossPct {
		t.Fatalf("logPrm = %+v", w.logPrm)
	}
	mass := entryAfter(t, w.content, "Vehicle mass incl. driver (kg)")
	if mass.Text != "1650" {
		t.Fatalf("mass entry = %q", mass.Text)
	}
	entryAfter(t, w.content, "Drag coefficient (Cd)").SetText("0.29")
	if w.logPrm.Cd != 0.29 || a.Preferences().Float("dyno_cd") != 0.29 {
		t.Fatalf("Cd = %v, saved %v", w.logPrm.Cd, a.Preferences().Float("dyno_cd"))
	}
	entryAfter(t, w.content, "Rolling resistance").SetText("x")
	if w.logPrm.Crr != defaultLogParams.Crr {
		t.Fatalf("unparseable Crr changed the value to %v", w.logPrm.Crr)
	}
}

// TestWidgetLoadLogWithoutSignals: an ECU without rpm/speed log signals
// reports that instead of opening a file picker.
func TestWidgetLoadLogWithoutSignals(t *testing.T) {
	w := newT7Widget(t, t7Syms(t).fw())
	w.cfg.ECU = "XX"
	w.loadLog()
	if w.logInfo.Text != "no log signals defined for XX" {
		t.Fatalf("logInfo = %q", w.logInfo.Text)
	}
}
