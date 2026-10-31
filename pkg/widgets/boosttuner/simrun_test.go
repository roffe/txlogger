package boosttuner

import (
	"testing"

	"fyne.io/fyne/v2/test"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
)

// simLog is three samples at 4000 rpm / 800 mg/c (RegMap cell 45.0% = 450 raw):
// open loop, then closed loop at LoadDiff 100 and 70.
func simLog() string {
	return boostLog(
		sample{rpm: 4000, setv: 800, regCon: 450, pwm: 450, twopct: 1},
		sample{rpm: 4000, setv: 800, regCon: 450, p: 70, adap: 10, loadDiff: 100, pwm: 540},
		sample{rpm: 4000, setv: 800, regCon: 450, adap: 10, loadDiff: 70, pwm: 460},
	)
}

// runSimulation replays the log through RegMap (scaled to raw units), the
// editors' P/I/D maps and the binary's FilterFactor, and returns the plotter
// series by name.
func TestRunSimulation(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	loadLog(t, bt, "run.csv", simLog())

	got, err := bt.runSimulation()
	if err != nil {
		t.Fatal(err)
	}
	// Sample 1: env = 540-(450+70+10) = 10; P = 100*60/100; I below the 1000
	// accumulator threshold; D starts from no history.
	// Sample 2: P const 50+0.7*10 = 57, D const 500+0.7*100 = 570,
	// D = (70-100)*570/(20+10) = -570.
	want := map[string][]float64{
		"PWMCalc logged":    {450, 540, 460},
		"PWMCalc predicted": {450, 450 + 60 + 10 + 10, 450 + 39.9 - 570 + 10},
		"P (sim)":           {0, 60, 39.9},
		"I (sim)":           {0, 0, 0},
		"D (sim)":           {0, 0, -570},
	}
	if len(got) != len(want) {
		t.Fatalf("series = %d, want %d", len(got), len(want))
	}
	for k, w := range want {
		if !nearSlice(got[k], w) {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}

	// An edit in the P editor (row 4000 rpm, column +100) drives the replay.
	bt.pidEditors["P"].data[5] = 120
	got, err = bt.runSimulation()
	if err != nil {
		t.Fatal(err)
	}
	if p := got["P (sim)"][1]; !near(p, 120) {
		t.Fatalf("P (sim) after edit = %v, want 120", p)
	}
}

// runSimulation needs logs, every channel and each map symbol.
func TestRunSimulation_Errors(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	if _, err := bt.runSimulation(); err == nil || err.Error() != "load logs first (Logs tab)" {
		t.Fatalf("no logs: err = %v", err)
	}
	loadLog(t, bt, "rpm.csv", csvLog([]string{"ActualIn.n_Engine"}, []float64{3000}))
	if _, err := bt.runSimulation(); err == nil || err.Error() != "missing channels for simulation" {
		t.Fatalf("missing channels: err = %v", err)
	}

	for _, name := range []string{symSetLoadXSP, symNEngSP, symRegMap, symPIDXSP, symPIDYSP} {
		bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) { delete(m, name) })})
		loadLog(t, bt, "run.csv", simLog())
		_, err := bt.runSimulation()
		if want := "symbol " + name + " not found in binary"; err == nil || err.Error() != want {
			t.Errorf("without %s: err = %v, want %q", name, err, want)
		}
	}
}

// loadSimMaps converts RegMap to raw 0.1% units and reads the loop constants,
// defaulting IFacMax to 350 and FilterFactor to 0 when the binary lacks them.
func TestLoadSimMaps(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	m, err := bt.loadSimMaps()
	if err != nil {
		t.Fatal(err)
	}
	if want := []float64{300, 400, 500, 350, 450, 550}; !nearSlice(m.regMapRaw, want) {
		t.Errorf("regMapRaw = %v, want %v", m.regMapRaw, want)
	}
	if !nearSlice(m.setLoadXSP, fakeLoadXSP) || !nearSlice(m.nEngSP, fakeEngSP) ||
		!nearSlice(m.pidXSP, fakePIDXSP) || !nearSlice(m.pidYSP, fakePIDYSP) {
		t.Errorf("axes = %v %v %v %v", m.setLoadXSP, m.nEngSP, m.pidXSP, m.pidYSP)
	}
	if !nearSlice(m.pMap, fakePMap) || !nearSlice(m.iMap, fakeIMap) || !nearSlice(m.dMap, fakeDMap) {
		t.Errorf("PID maps = %v %v %v", m.pMap, m.iMap, m.dMap)
	}
	if m.iFacMax != 200 || m.filterFactor != 10 {
		t.Errorf("iFacMax = %v, filterFactor = %v; want 200, 10", m.iFacMax, m.filterFactor)
	}

	bt = New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) {
		delete(m, "BoostCal.IFacMax")
		delete(m, "BoostCal.FilterFactor")
	})})
	m, err = bt.loadSimMaps()
	if err != nil {
		t.Fatal(err)
	}
	if m.iFacMax != 350 || m.filterFactor != 0 {
		t.Errorf("defaults: iFacMax = %v, filterFactor = %v; want 350, 0", m.iFacMax, m.filterFactor)
	}
}

// pidMapData prefers the editor's live data and falls back to the binary when
// there is no editor (nil when the symbol is missing too).
func TestPIDMapData(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, func(m map[string]*symbol.Symbol) { delete(m, symDMap) })})
	bt.pidEditors["P"].data[0] = 77
	if got := bt.pidMapData("P", symPMap); got[0] != 77 {
		t.Fatalf("editor data not used: %v", got)
	}
	bt.pidEditors = nil
	if got := bt.pidMapData("P", symPMap); !nearSlice(got, fakePMap) {
		t.Fatalf("binary fallback = %v, want %v", got, fakePMap)
	}
	if got := bt.pidMapData("D", symDMap); got != nil {
		t.Fatalf("missing symbol = %v, want nil", got)
	}
}

// The Simulate button shows the error, or plots the replay.
func TestSimTab_Button(t *testing.T) {
	test.NewTempApp(t)
	bt := New(Config{Symbols: fakeBinary(t, nil)})
	root := tab(t, bt, "Simulator")
	btn := findButton(t, root, "Simulate")
	status := siblingLabel(t, root, btn)

	test.Tap(btn)
	if status.Text != "load logs first (Logs tab)" {
		t.Fatalf("status = %q", status.Text)
	}
	if n := len(findAll[*plotter.Plotter](root, nil)); n != 0 {
		t.Fatalf("plot shown after failure")
	}

	loadLog(t, bt, "run.csv", simLog())
	test.Tap(btn)
	if status.Text != "Simulated. Toggle series in the legend." {
		t.Fatalf("status = %q", status.Text)
	}
	if n := len(findAll[*plotter.Plotter](root, nil)); n != 1 {
		t.Fatalf("plots = %d, want 1", n)
	}
}
