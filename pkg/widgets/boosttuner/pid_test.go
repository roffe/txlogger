package boosttuner

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
)

// pidSubTabs returns the P/I/D editor tabs inside the PID maps tab.
func pidSubTabs(t *testing.T, bt *BoostTuner) *container.AppTabs {
	t.Helper()
	tabs := findAll[*container.AppTabs](tab(t, bt, "PID maps"), nil)
	if len(tabs) != 1 {
		t.Fatalf("found %d PID sub-tab sets, want 1", len(tabs))
	}
	return tabs[0]
}

// pidEditorRoot returns the content of the editor tab for map name ("P"...).
func pidEditorRoot(t *testing.T, bt *BoostTuner, name string) fyne.CanvasObject {
	t.Helper()
	for _, it := range pidSubTabs(t, bt).Items {
		if it.Text == name+" map" {
			return it.Content
		}
	}
	t.Fatalf("no %s map tab", name)
	return nil
}

// overshootLog is one boost onset at 4000 rpm that overshoots by 60 mg/c with
// two error sign changes before settling, then the loop disengages.
func overshootLog() string {
	var s []sample
	for _, e := range []float64{150, 90, 30, -40, -60, 10, 5, 4} {
		s = append(s, sample{rpm: 4000, setv: 800, loadDiff: e, pwm: 500})
	}
	s = append(s, sample{rpm: 4000, setv: 800, pwm: 500, twopct: 1})
	return boostLog(s...)
}

// fmtInts formats whole-number map values the way a precision-0 viewer does.
func fmtInts(vals []float64) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = formatFloat(v)
	}
	return out
}

// The PID tab builds one editor per gain map, loaded from the binary, on the
// shared PIDXSP/PIDYSP axes.
func TestPIDTab_LoadsEditors(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})

	want := map[string]struct {
		sym  string
		data []float64
	}{
		"P": {symPMap, fakePMap},
		"I": {symIMap, fakeIMap},
		"D": {symDMap, fakeDMap},
	}
	if len(bt.pidEditors) != len(want) {
		t.Fatalf("editors = %d, want %d", len(bt.pidEditors), len(want))
	}
	for name, w := range want {
		ed := bt.pidEditors[name]
		if ed == nil {
			t.Fatalf("no %s editor", name)
		}
		if ed.symbolName != w.sym || ed.cols != 3 || ed.rows != 2 {
			t.Errorf("%s: symbol %s %dx%d", name, ed.symbolName, ed.cols, ed.rows)
		}
		if !nearSlice(ed.axisX, fakePIDXSP) || !nearSlice(ed.axisY, fakePIDYSP) {
			t.Errorf("%s axes = %v / %v", name, ed.axisX, ed.axisY)
		}
		if !nearSlice(ed.data, w.data) {
			t.Errorf("%s data = %v, want %v", name, ed.data, w.data)
		}
		if ed.status.Text != "" {
			t.Errorf("%s status = %q", name, ed.status.Text)
		}
		if got := mapTexts(t, ed.mv, 3); !reflect.DeepEqual(got, fmtInts(w.data)) {
			t.Errorf("%s viewer = %v, want %v", name, got, fmtInts(w.data))
		}
	}
	var names []string
	for _, it := range pidSubTabs(t, bt).Items {
		names = append(names, it.Text)
	}
	if !reflect.DeepEqual(names, []string{"P map", "I map", "D map"}) {
		t.Fatalf("PID sub-tabs = %v", names)
	}
}

// Without the PID axes the tab only explains why, and no editors exist.
func TestPIDTab_NoAxes(t *testing.T) {
	test.NewTempApp(t)
	for name, syms := range map[string]symbol.FirmwareFile{
		"no binary": nil,
		"no PIDYSP": fakeBinary(t, func(m map[string]*symbol.Symbol) { delete(m, symPIDYSP) }),
		"no PIDXSP": fakeBinary(t, func(m map[string]*symbol.Symbol) { delete(m, symPIDXSP) }),
	} {
		bt := New(Config{Symbols: syms})
		if len(bt.pidEditors) != 0 {
			t.Errorf("%s: %d editors, want 0", name, len(bt.pidEditors))
		}
		if got := labelTexts(tab(t, bt, "PID maps")); !reflect.DeepEqual(got, []string{"BoostCal PID axes not found in this binary."}) {
			t.Errorf("%s: labels = %q", name, got)
		}
	}
}

// An editor whose map symbol is missing starts from zeros and shows the error.
func TestPIDEditor_MissingMap(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) { delete(m, symDMap) })})
	ed := bt.pidEditors["D"]
	if !nearSlice(ed.data, make([]float64, 6)) {
		t.Fatalf("D data = %v, want 6 zeros", ed.data)
	}
	if ed.status.Text != "symbol BoostCal.DMap not found in binary" {
		t.Fatalf("D status = %q", ed.status.Text)
	}
	if got := mapTexts(t, ed.mv, 3); !reflect.DeepEqual(got, []string{"0", "0", "0", "0", "0", "0"}) {
		t.Fatalf("D viewer = %v", got)
	}
}

// A value typed into the editor's viewer is what Apply writes into the
// symbol; Reload discards edits by re-reading the binary into a fresh viewer.
func TestPIDEditor_ApplyAndReload(t *testing.T) {
	test.NewTempApp(t)
	rec := &saveRecorder{}
	bt := New(Config{Symbols: fakeBinary(t, nil), Save: rec.save})
	ed := bt.pidEditors["P"]
	root := pidEditorRoot(t, bt, "P")
	apply := findButton(t, root, "Apply to binary")
	reload := findButton(t, root, "Reload from binary")

	// Select cell 0 (2000 rpm, -100 mg/c) with the keyboard and type 99.
	mapTexts(t, ed.mv, 3) // the viewer takes keys once rendered
	for _, k := range []fyne.KeyName{fyne.KeyLeft, fyne.Key9, fyne.Key9, fyne.KeyReturn} {
		ed.mv.TypedKey(&fyne.KeyEvent{Name: k})
	}
	test.Tap(apply)
	if len(rec.calls) != 1 {
		t.Fatalf("saves = %d, want 1", len(rec.calls))
	}
	if c := rec.calls[0]; c.name != symPMap || !nearSlice(c.data, []float64{99, 20, 30, 40, 50, 60}) {
		t.Fatalf("saved %s %v", c.name, c.data)
	}
	if ed.status.Text != "Wrote BoostCal.PMap" {
		t.Fatalf("status = %q", ed.status.Text)
	}

	rec.err = errors.New("read-only")
	test.Tap(apply)
	if ed.status.Text != "Save failed: read-only" {
		t.Fatalf("status = %q", ed.status.Text)
	}

	oldMV := ed.mv
	test.Tap(reload)
	if !nearSlice(ed.data, fakePMap) {
		t.Fatalf("data after reload = %v, want %v", ed.data, fakePMap)
	}
	if ed.status.Text != "Reloaded BoostCal.PMap" {
		t.Fatalf("status = %q", ed.status.Text)
	}
	if ed.mv == oldMV {
		t.Fatal("reload kept the old viewer")
	}
	host := root.(*container.Split).Leading.(*fyne.Container)
	if displayed(t, host) != ed.mv {
		t.Fatal("display does not show the reloaded viewer")
	}
	if got := mapTexts(t, ed.mv, 3); !reflect.DeepEqual(got, fmtInts(fakePMap)) {
		t.Fatalf("viewer after reload = %v", got)
	}
}

// Without a Save func the Apply button is disabled, and its handler refuses too.
func TestPIDEditor_ApplyWithoutSave(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	apply := findButton(t, pidEditorRoot(t, bt, "I"), "Apply to binary")
	if !apply.Disabled() {
		t.Fatal("Apply enabled without a Save func")
	}
	apply.OnTapped()
	if got := bt.pidEditors["I"].status.Text; got != "No binary to write to." {
		t.Fatalf("status = %q", got)
	}
}

// scaleRows multiplies whole rpm rows, ignores rows outside the map and
// refreshes a rendered viewer.
func TestPIDEditor_ScaleRows(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	ed := bt.pidEditors["P"]
	mapTexts(t, ed.mv, 3) // render the viewer first

	ed.scaleRows(map[int]float64{1: 0.5})
	if want := []float64{10, 20, 30, 20, 25, 30}; !nearSlice(ed.data, want) {
		t.Fatalf("data = %v, want %v", ed.data, want)
	}
	if ed.status.Text != "Scaled 1 row(s); review then Apply." {
		t.Fatalf("status = %q", ed.status.Text)
	}
	if got := mapTexts(t, ed.mv, 3); !reflect.DeepEqual(got, []string{"10", "20", "30", "20", "25", "30"}) {
		t.Fatalf("viewer = %v", got)
	}

	ed.scaleRows(map[int]float64{-1: 9, 2: 9})
	if want := []float64{10, 20, 30, 20, 25, 30}; !nearSlice(ed.data, want) {
		t.Fatalf("out-of-range rows changed data: %v", ed.data)
	}
}

// computePIDSuggestions explains what it needs, and with a usable log reports
// the transients and band suggestions it found.
func TestPIDSuggestions_Status(t *testing.T) {
	test.NewTempApp(t)
	cases := []struct {
		name, log, want string
	}{
		{"no logs", "", "Load logs first (Logs tab)."},
		{"no LoadDiff", csvLog([]string{"ActualIn.n_Engine", "BoostProt.ST_PWM2Perc"}, []float64{3000, 0}),
			"Need rpm, LoadDiff and the 2% flag channels."},
		{"steady", boostLog(repeat(sample{rpm: 3000, loadDiff: 5}, 10)...), "No boost transients detected."},
		{"overshoot", overshootLog(), "1 transients, 1 band suggestion(s)."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bt := New(Config{Symbols: fakeBinary(t, nil)})
			if c.log != "" {
				loadLog(t, bt, "run.csv", c.log)
			}
			root := tab(t, bt, "PID maps")
			btn := findButton(t, root, "Compute suggestions from logs")
			test.Tap(btn)
			if got := siblingLabel(t, root, btn).Text; got != c.want {
				t.Fatalf("status = %q, want %q", got, c.want)
			}
		})
	}

	// Transients found but no PID axes to place them on.
	bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) { delete(m, symPIDXSP) })})
	loadLog(t, bt, "run.csv", overshootLog())
	if got := bt.computePIDSuggestions(); got != "PID maps/axes not found in this binary." {
		t.Fatalf("status = %q", got)
	}
}

// Each editor lists only the suggestions that change its map, with per-band
// Apply and Apply all buttons that scale the matching rows.
func TestPIDSuggestions_ApplyButtons(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	loadLog(t, bt, "run.csv", overshootLog())
	if got := bt.computePIDSuggestions(); got != "1 transients, 1 band suggestion(s)." {
		t.Fatalf("status = %q", got)
	}

	wantLabel := "4000 rpm: ×0.85 (overshoot 60 mg/c, 2.0 crossings)"
	for _, name := range []string{"P", "D"} {
		box := bt.pidEditors[name].suggestBox
		if got := labelTexts(box); !reflect.DeepEqual(got, []string{wantLabel}) {
			t.Errorf("%s suggestions = %q, want [%q]", name, got, wantLabel)
		}
		if n := len(findAll[*widget.Button](box, nil)); n != 2 {
			t.Errorf("%s has %d buttons, want Apply + Apply all", name, n)
		}
	}
	if got := labelTexts(bt.pidEditors["I"].suggestBox); !reflect.DeepEqual(got, []string{"No suggestions for this map."}) {
		t.Fatalf("I suggestions = %q", got)
	}

	p := bt.pidEditors["P"]
	test.Tap(findButton(t, p.suggestBox, "Apply"))
	if want := []float64{10, 20, 30, 40 * 0.85, 50 * 0.85, 60 * 0.85}; !nearSlice(p.data, want) {
		t.Fatalf("P after Apply = %v, want %v", p.data, want)
	}
	if p.status.Text != "Scaled 1 row(s); review then Apply." {
		t.Fatalf("P status = %q", p.status.Text)
	}

	d := bt.pidEditors["D"]
	test.Tap(findButton(t, d.suggestBox, "Apply all"))
	if want := []float64{100, 200, 300, 400 * 0.85, 500 * 0.85, 600 * 0.85}; !nearSlice(d.data, want) {
		t.Fatalf("D after Apply all = %v, want %v", d.data, want)
	}
	if !nearSlice(bt.pidEditors["I"].data, fakeIMap) {
		t.Fatalf("I changed: %v", bt.pidEditors["I"].data)
	}

	// A log without transients replaces the old suggestions.
	bt.clearLogs()
	loadLog(t, bt, "steady.csv", boostLog(repeat(sample{rpm: 3000, loadDiff: 5}, 10)...))
	bt.computePIDSuggestions()
	for name, ed := range bt.pidEditors {
		if got := labelTexts(ed.suggestBox); len(got) != 1 || !strings.HasPrefix(got[0], "No suggestions") {
			t.Errorf("%s suggestions after reset = %q", name, got)
		}
	}
}

// A PID map whose size does not match its axes shows the viewer's error in
// place of the map.
func TestPIDEditor_SizeMismatchShowsError(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) {
		m[symPMap] = mkSym(t, symPMap, false, 1, 1, 2, 3, 4, 5)
	})})
	if bt.pidEditors["P"].mv != nil {
		t.Fatal("viewer built for a mismatched map")
	}
	host := pidEditorRoot(t, bt, "P").(*container.Split).Leading
	if got := labelTexts(host); !reflect.DeepEqual(got, []string{"mapViewer columns * rows != data length"}) {
		t.Fatalf("display labels = %q", got)
	}
}

// Apply and Apply all on a PID map whose size does not match its axes report
// the mismatch and leave the map alone.
func TestPIDSuggestions_ApplySizeMismatch(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) {
		m[symPMap] = mkSym(t, symPMap, false, 1, 1, 2, 3, 4, 5)
	})})
	loadLog(t, bt, "run.csv", overshootLog())
	bt.computePIDSuggestions()
	p := bt.pidEditors["P"]
	for _, btn := range []string{"Apply", "Apply all"} {
		test.Tap(findButton(t, p.suggestBox, btn))
		if want := []float64{1, 2, 3, 4, 5}; !nearSlice(p.data, want) {
			t.Fatalf("P after %s = %v, want %v", btn, p.data, want)
		}
		if want := "BoostCal.PMap size 5 != 3 x 2 axes"; p.status.Text != want {
			t.Fatalf("P status after %s = %q, want %q", btn, p.status.Text, want)
		}
	}
}

// A PID map shorter than PIDXSP x PIDYSP should be reported like a mismatched
// RegMap is, not index past its end during the replay.
func TestRunSimulation_PIDMapSizeMismatch(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) {
		m[symPMap] = mkSym(t, symPMap, false, 1, 1, 2, 3, 4, 5)
	})})
	loadLog(t, bt, "run.csv", simLog())
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("runSimulation panicked: %v", r)
			}
		}()
		_, err = bt.runSimulation()
	}()
	if err == nil {
		t.Fatal("runSimulation accepted a 5-cell PMap on 3x2 axes")
	}
}
