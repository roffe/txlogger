package boosttuner

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
)

// learnLog is a session with a well-covered cell and a sparse one:
//   - 6 steady on-target samples at 4000 rpm / 800 mg/c whose loop output sums
//     to 500 raw (50.0%) against a stored 45.0%,
//   - 3 samples at 2000 rpm / 400 mg/c summing to 310 raw (31.0%) against 30.0%;
//     the first is rejected for the 2000 rpm step, leaving 2,
//   - 1 open-loop sample, rejected.
func learnLog() string {
	s := repeat(sample{rpm: 4000, setv: 800, regCon: 450, p: 20, i: 30, loadDiff: 5, pwm: 500}, 6)
	s = append(s, repeat(sample{rpm: 2000, setv: 400, regCon: 300, p: 10, loadDiff: -5, pwm: 310}, 3)...)
	s = append(s, sample{rpm: 2000, setv: 400, regCon: 300, p: 10, loadDiff: -5, pwm: 310, twopct: 1})
	return boostLog(s...)
}

// Analyze reports why it cannot learn and leaves the display untouched.
func TestRegMapTab_AnalyzeErrors(t *testing.T) {
	test.NewTempApp(t)
	without := func(names ...string) func(map[string]*symbol.Symbol) {
		return func(m map[string]*symbol.Symbol) {
			for _, n := range names {
				delete(m, n)
			}
		}
	}
	cases := []struct {
		name  string
		syms  symbol.FirmwareFile
		log   string
		wantP string // status prefix
	}{
		{"no logs", fakeBinary(t, nil), "", "load a log file first (Logs tab)"},
		{"missing channels", fakeBinary(t, nil), csvLog([]string{"ActualIn.n_Engine"}, []float64{3000}),
			"missing channels: [Load set value (RegMap X axis) Feedforward duty from RegMap"},
		{"no binary", nil, learnLog(), "no binary loaded"},
		{"no X axis", fakeBinary(t, without(symSetLoadXSP)), learnLog(), "symbol BoostCal.SetLoadXSP not found in binary"},
		{"no Y axis", fakeBinary(t, without(symNEngSP)), learnLog(), "symbol BoostCal.n_EngSP not found in binary"},
		{"no RegMap", fakeBinary(t, without(symRegMap)), learnLog(), "symbol BoostCal.RegMap not found in binary"},
		{"size mismatch", fakeBinary(t, func(m map[string]*symbol.Symbol) {
			m[symRegMap] = mkSym(t, symRegMap, false, 0.1, 1, 2, 3, 4, 5)
		}), learnLog(), "RegMap size 5 != 3 x 2 axes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bt := New(Config{Symbols: c.syms})
			if c.log != "" {
				loadLog(t, bt, "run.csv", c.log)
			}
			test.Tap(findButton(t, tab(t, bt, "RegMap"), "Analyze"))
			if !strings.HasPrefix(bt.rmStatus.Text, c.wantP) {
				t.Fatalf("status = %q, want prefix %q", bt.rmStatus.Text, c.wantP)
			}
			if bt.rmBuilt {
				t.Fatal("rmBuilt set after a failed analyze")
			}
			if _, ok := bt.rmDisplay.Objects[0].(*mapviewer.MapViewer); ok {
				t.Fatal("display shows a map after a failed analyze")
			}
		})
	}
}

// Analyze learns the map from the accepted samples, reports the sample counts,
// and each view shows the matching per-cell values (row 0 = 2000 rpm).
func TestRegMapTab_AnalyzeAndViews(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})

	// Switching views before analyzing does nothing.
	bt.rmView.SetSelected(viewDelta)
	if _, ok := bt.rmDisplay.Objects[0].(*mapviewer.MapViewer); ok {
		t.Fatal("display shows a map before analyze")
	}
	bt.rmView.SetSelected(viewLearned)

	loadLog(t, bt, "run.csv", learnLog())
	test.Tap(findButton(t, tab(t, bt, "RegMap"), "Analyze"))

	if want := "Used 8 samples (2 filtered). 2/6 cells have data."; bt.rmStatus.Text != want {
		t.Fatalf("status = %q, want %q", bt.rmStatus.Text, want)
	}
	if !bt.rmBuilt || bt.rmCols != 3 || bt.rmRows != 2 {
		t.Fatalf("built=%v cols=%d rows=%d", bt.rmBuilt, bt.rmCols, bt.rmRows)
	}
	if want := []int{2, 0, 0, 0, 6, 0}; !reflect.DeepEqual(bt.rmCounts, want) {
		t.Fatalf("counts = %v, want %v", bt.rmCounts, want)
	}
	if want := []float64{31, 40, 50, 35, 50, 55}; !nearSlice(bt.rmLearned, want) {
		t.Fatalf("learned = %v, want %v", bt.rmLearned, want)
	}

	// The sparse cell (2 < 5 samples) keeps its current value.
	views := []struct {
		view string
		want []string
	}{
		{viewLearned, []string{"30.0", "40.0", "50.0", "35.0", "50.0", "55.0"}},
		{viewCurrent, []string{"30.0", "40.0", "50.0", "35.0", "45.0", "55.0"}},
		{viewDelta, []string{"0.0", "0.0", "0.0", "0.0", "5.0", "0.0"}},
		{viewCoverage, []string{"2", "0", "0", "0", "6", "0"}},
	}
	for _, v := range views {
		bt.rmView.SetSelected(v.view)
		if got := mapTexts(t, displayed(t, bt.rmDisplay), 3); !reflect.DeepEqual(got, v.want) {
			t.Errorf("%s view = %v, want %v", v.view, got, v.want)
		}
	}
}

// The apply sliders retarget what gets written and refresh the view at once:
// min samples admits the sparse cell, blend moves cells part way.
func TestRegMapTab_ApplySliders(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	loadLog(t, bt, "run.csv", learnLog())
	root := tab(t, bt, "RegMap")
	test.Tap(findButton(t, root, "Analyze"))

	minS, minVal := findSlider(t, root, "Min samples / cell")
	minS.SetValue(1)
	if bt.minSamples != 1 || minVal.Text != "1" {
		t.Fatalf("minSamples = %d, readout %q", bt.minSamples, minVal.Text)
	}
	if got, want := mapTexts(t, displayed(t, bt.rmDisplay), 3), []string{"31.0", "40.0", "50.0", "35.0", "50.0", "55.0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("learned view = %v, want %v", got, want)
	}

	blend, blendVal := findSlider(t, root, "Blend %")
	blend.SetValue(50)
	if !near(bt.blend, 0.5) || blendVal.Text != "50" {
		t.Fatalf("blend = %v, readout %q", bt.blend, blendVal.Text)
	}
	if got, want := mapTexts(t, displayed(t, bt.rmDisplay), 3), []string{"30.5", "40.0", "50.0", "35.0", "47.5", "55.0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("blended view = %v, want %v", got, want)
	}
}

// Each sample filter slider updates its readout and changes which samples the
// next analysis accepts.
func TestRegMapTab_FilterSliders(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	// All samples are 20 mg/c off target and land in the 4000 rpm / 800 mg/c
	// cell. Sample 1 is a 180 rpm step and sample 2 an 80 mg/c load step, so
	// the default 150 rpm / 50 mg/c limits reject one each.
	base := sample{regCon: 450, loadDiff: 20, pwm: 450}
	at := func(rpm, setv float64) sample { s := base; s.rpm, s.setv = rpm, setv; return s }
	loadLog(t, bt, "run.csv", boostLog(at(3100, 800), at(3280, 800), at(3280, 880), at(3280, 880)))
	root := tab(t, bt, "RegMap")
	analyze := findButton(t, root, "Analyze")

	steps := []struct {
		slider string
		value  float64
		field  *float64
		want   string
	}{
		{"", 0, nil, "Used 2 samples (2 filtered). 1/6 cells have data."},
		{"Max rpm step (rpm)", 200, &bt.rpmStab, "Used 3 samples (1 filtered). 1/6 cells have data."},
		{"Max load step (mg/c)", 100, &bt.loadStab, "Used 4 samples (0 filtered). 1/6 cells have data."},
		{"On-target |error| (mg/c)", 15, &bt.onTarget, "Used 0 samples (4 filtered). 0/6 cells have data."},
	}
	for _, s := range steps {
		if s.slider != "" {
			sl, readout := findSlider(t, root, s.slider)
			sl.SetValue(s.value)
			if *s.field != s.value || readout.Text != formatFloat(s.value) {
				t.Fatalf("%s: field = %v, readout %q; want %v", s.slider, *s.field, readout.Text, s.value)
			}
		}
		test.Tap(analyze)
		if bt.rmStatus.Text != s.want {
			t.Fatalf("after %q: status = %q, want %q", s.slider, bt.rmStatus.Text, s.want)
		}
	}
}

// Apply refuses before analyzing, reports save errors without adopting the
// data, and on success writes the blended map and makes it the current map.
func TestRegMapTab_Apply(t *testing.T) {
	test.NewTempApp(t)
	rec := &saveRecorder{}
	bt := New(Config{Symbols: fakeBinary(t, nil), Save: rec.save})
	root := tab(t, bt, "RegMap")
	apply := findButton(t, root, "Apply to binary")
	if apply.Disabled() {
		t.Fatal("Apply disabled with a Save func")
	}

	test.Tap(apply)
	if bt.rmStatus.Text != "Analyze first." || len(rec.calls) != 0 {
		t.Fatalf("status = %q, %d saves", bt.rmStatus.Text, len(rec.calls))
	}

	loadLog(t, bt, "run.csv", learnLog())
	test.Tap(findButton(t, root, "Analyze"))

	rec.err = errors.New("disk full")
	test.Tap(apply)
	if bt.rmStatus.Text != "Save failed: disk full" {
		t.Fatalf("status = %q", bt.rmStatus.Text)
	}
	if !nearSlice(bt.rmCurrent, fakeRegMap) {
		t.Fatalf("current changed after failed save: %v", bt.rmCurrent)
	}

	rec.err = nil
	test.Tap(apply)
	if len(rec.calls) != 2 {
		t.Fatalf("saves = %d, want 2", len(rec.calls))
	}
	want := []float64{30, 40, 50, 35, 50, 55}
	if c := rec.calls[1]; c.name != symRegMap || !nearSlice(c.data, want) {
		t.Fatalf("saved %s %v, want %s %v", c.name, c.data, symRegMap, want)
	}
	if bt.rmStatus.Text != "Wrote BoostCal.RegMap: 1 cells changed." {
		t.Fatalf("status = %q", bt.rmStatus.Text)
	}
	if !nearSlice(bt.rmCurrent, want) {
		t.Fatalf("current = %v, want %v", bt.rmCurrent, want)
	}
	bt.rmView.SetSelected(viewDelta)
	if got := mapTexts(t, displayed(t, bt.rmDisplay), 3); !reflect.DeepEqual(got, []string{"0.0", "0.0", "0.0", "0.0", "0.0", "0.0"}) {
		t.Fatalf("delta after apply = %v", got)
	}
}

// Without a Save func the Apply button is disabled and applyRegMap says so.
func TestRegMapTab_ApplyWithoutSave(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	root := tab(t, bt, "RegMap")
	if !findButton(t, root, "Apply to binary").Disabled() {
		t.Fatal("Apply enabled without a Save func")
	}
	loadLog(t, bt, "run.csv", learnLog())
	test.Tap(findButton(t, root, "Analyze"))
	bt.applyRegMap()
	if bt.rmStatus.Text != "No binary to write to." {
		t.Fatalf("status = %q", bt.rmStatus.Text)
	}
}

// writeTarget moves only cells with at least minSamples hits, by blend of the
// way from current to learned.
func TestWriteTarget(t *testing.T) {
	bt := &BoostTuner{
		rmCurrent:  []float64{10, 20, 30, 40},
		rmLearned:  []float64{50, 60, 70, 80},
		rmCounts:   []int{0, 2, 3, 9},
		minSamples: 3,
		blend:      0.25,
	}
	if got, want := bt.writeTarget(), []float64{10, 20, 40, 50}; !nearSlice(got, want) {
		t.Fatalf("writeTarget = %v, want %v", got, want)
	}
	if !nearSlice(bt.rmCurrent, []float64{10, 20, 30, 40}) {
		t.Fatalf("writeTarget modified rmCurrent: %v", bt.rmCurrent)
	}
}
