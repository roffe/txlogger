package estimatedoutput

import (
	"math"
	"slices"
	"strings"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

// TestFor returns an estimator for T5/T7/T8 only.
func TestFor(t *testing.T) {
	for ecu, want := range map[string]Calculator{"T5": t5{}, "T7": t7{}, "T8": t8{}} {
		if got := For(ecu); got != want {
			t.Errorf("For(%q) = %T, want %T", ecu, got, want)
		}
	}
	for _, ecu := range []string{"", "t7", "T6", "ME9"} {
		if got := For(ecu); got != nil {
			t.Errorf("For(%q) = %T, want nil", ecu, got)
		}
	}
}

// TestHasAndNonZero: has checks presence, nonZero also needs a set byte.
func TestHasAndNonZero(t *testing.T) {
	fw := newSyms(bsym(t, "Zero", 0, 0), bsym(t, "Set", 0, 1)).fw()
	if !has(fw, "Zero") || has(fw, "Missing") {
		t.Fatal("has: wrong presence")
	}
	if nonZero(fw, "Zero") || !nonZero(fw, "Set") || nonZero(fw, "Missing") {
		t.Fatal("nonZero: wrong result")
	}
}

// TestSignCorrections: signed16 folds raw values above 32000 negative,
// unsigned16 lifts negative values back to the raw 16-bit view; both work
// in place.
func TestSignCorrections(t *testing.T) {
	v := []int{0, 32000, 32001, 65436, 65535}
	if got := signed16(v); !slices.Equal(got, []int{0, 32000, -33535, -100, -1}) || &got[0] != &v[0] {
		t.Fatalf("signed16 = %v", got)
	}
	u := []int{-1, -200, 0, 300}
	if got := unsigned16(u); !slices.Equal(got, []int{65535, 65336, 0, 300}) {
		t.Fatalf("unsigned16 = %v", got)
	}
}

// TestCint rounds half to even like C# Convert.ToInt32(double).
func TestCint(t *testing.T) {
	for in, want := range map[float64]int{0.5: 0, 1.5: 2, 2.5: 2, -0.5: 0, -1.5: -2, 2.49: 2, 2.51: 3} {
		if got := cint(in); got != want {
			t.Errorf("cint(%v) = %d, want %d", in, got, want)
		}
	}
}

// TestSymLoaderCollectsMissing names every missing symbol in one error and
// returns zero values for them.
func TestSymLoaderCollectsMissing(t *testing.T) {
	l := &symLoader{fw: newSyms(sym(t, "A", 7, 8)).fw()}
	if got := l.first("A"); got != 7 {
		t.Fatalf("first(A) = %d", got)
	}
	if l.ints("B") != nil || l.first("C") != 0 {
		t.Fatal("missing symbols must yield nil/0")
	}
	if err := l.err(); err == nil || err.Error() != "missing symbols: B, C" {
		t.Fatalf("err = %v", err)
	}
	if err := (&symLoader{fw: symbol.NewCollection()}).err(); err != nil {
		t.Fatalf("no lookups -> err = %v", err)
	}
}

// TestEffFactor turns the efficiency percentage into a factor, ignoring
// unset or implausible values.
func TestEffFactor(t *testing.T) {
	for in, want := range map[float64]float64{0: 1, 9.9: 1, 10: 0.1, 85: 0.85, 300: 3, 300.1: 1, -50: 1} {
		if got := effFactor(map[string]float64{"efficiency": in}); math.Abs(got-want) > 1e-12 {
			t.Errorf("effFactor(%v) = %v, want %v", in, got, want)
		}
	}
	if got := effFactor(nil); got != 1 {
		t.Errorf("effFactor(nil) = %v", got)
	}
}

// TestRPMCorrection pins the suites' high-rpm roll-off breakpoints.
func TestRPMCorrection(t *testing.T) {
	for rpm, want := range map[int]float64{0: 1, 5059: 1, 5060: 0.99, 5439: 0.99, 5440: 0.95, 5819: 0.95, 5820: 0.94, 5999: 0.94, 6000: 0.85, 9000: 0.85} {
		if got := rpmCorrection(rpm); got != want {
			t.Errorf("rpmCorrection(%d) = %v, want %v", rpm, got, want)
		}
	}
}

func optionByKey(opts []Option, key string) Option {
	for _, o := range opts {
		if o.Key == key {
			return o
		}
	}
	return Option{Key: "<missing " + key + ">"}
}

// TestT5Options: the automatic option is only enabled with Tryck_mat_a!.
func TestT5Options(t *testing.T) {
	opts := t5{}.Options(t5Syms(t).fw())
	if keys := []string{opts[0].Key, opts[1].Key, opts[2].Key}; !slices.Equal(keys, []string{"turbo", "mapsensor", "automatic"}) {
		t.Fatalf("keys = %v", keys)
	}
	if !slices.Equal(opts[0].Choices, t5Turbos) || !slices.Equal(opts[1].Choices, t5MapSensors) || opts[0].Kind != OptionChoice {
		t.Fatal("unexpected choice options")
	}
	if optionByKey(opts, "automatic").Disabled {
		t.Fatal("automatic disabled although Tryck_mat_a! exists")
	}
	opts = t5{}.Options(t5Syms(t).del("Tryck_mat_a!").fw())
	if !optionByKey(opts, "automatic").Disabled {
		t.Fatal("automatic enabled without Tryck_mat_a!")
	}
}

// TestT7Options: E85, convertible and overboost follow the binary's tables;
// defaults are firmware limit on, 5th gear, 100 kPa and 100 % efficiency.
func TestT7Options(t *testing.T) {
	opts := t7{}.Options(t7Syms(t).fw())
	for _, k := range []string{"e85", "convertible", "overboost"} {
		if optionByKey(opts, k).Disabled {
			t.Errorf("%s disabled on a binary that has it", k)
		}
	}
	defaults := map[string]float64{"fwlimit": 1, "gear": 5, "ambient": 100, "efficiency": 100, "automatic": 0}
	for k, want := range defaults {
		if got := optionByKey(opts, k).Default; got != want {
			t.Errorf("%s default = %v, want %v", k, got, want)
		}
	}
	if g := optionByKey(opts, "gear"); !slices.Equal(g.Choices, t7Gears) || g.Kind != OptionChoice {
		t.Errorf("gear option = %+v", g)
	}
	if optionByKey(opts, "ambient").Kind != OptionNumber {
		t.Error("ambient is not a number option")
	}

	bare := t7Syms(t).del("TorqueCal.M_EngMaxE85Tab", "TorqueCal.M_CabGearLim").set(bsym(t, "TorqueCal.EnableOverBoost", 0))
	opts = t7{}.Options(bare.fw())
	for _, k := range []string{"e85", "convertible", "overboost"} {
		if !optionByKey(opts, k).Disabled {
			t.Errorf("%s enabled on a binary without it", k)
		}
	}
}

// TestT8Options: E85 needs a flexfuel torque table, overboost a non-zero
// TrqLimCal.EnableOverBoost; high output defaults on.
func TestT8Options(t *testing.T) {
	opts := t8{}.Options(t8Syms(t).fw())
	if optionByKey(opts, "e85").Disabled || optionByKey(opts, "overboost").Disabled {
		t.Fatal("e85/overboost disabled on a flexfuel binary with overboost")
	}
	if optionByKey(opts, "highoutput").Default != 1 || !slices.Equal(optionByKey(opts, "gear").Choices, t8Gears) {
		t.Fatal("unexpected highoutput/gear option")
	}
	// either flexfuel table marks the binary as biopower
	onlyTab2 := t8Syms(t).del("FFTrqCal.FFTrq_MaxEngineTab1")
	if optionByKey(t8{}.Options(onlyTab2.fw()), "e85").Disabled {
		t.Fatal("e85 disabled with FFTrq_MaxEngineTab2 present")
	}
	petrol := t8Syms(t).del("FFTrqCal.FFTrq_MaxEngineTab1", "FFTrqCal.FFTrq_MaxEngineTab2").set(bsym(t, "TrqLimCal.EnableOverBoost", 0))
	opts = t8{}.Options(petrol.fw())
	if !optionByKey(opts, "e85").Disabled || !optionByKey(opts, "overboost").Disabled {
		t.Fatal("e85/overboost enabled on a petrol binary without overboost")
	}
}

// TestCalculateMissingSymbols reports every required table the binary
// lacks in one error.
func TestCalculateMissingSymbols(t *testing.T) {
	tests := []struct {
		name string
		calc Calculator
		fw   symbol.FirmwareFile
		opts map[string]float64
		want []string
	}{
		{"T5", t5{}, t5Syms(t).del("Insp_mat!", "Min_tid!").fw(), nil, []string{"Min_tid!", "Insp_mat!"}},
		{"T5 automatic", t5{}, t5Syms(t).del("Tryck_mat_a!").fw(), map[string]float64{"automatic": 1}, []string{"Tryck_mat_a!"}},
		{"T7 E85", t7{}, t7Syms(t).del("BFuelCal.StartMap", "TorqueCal.M_EngMaxE85Tab").fw(), t7Opts(map[string]float64{"e85": 1}), []string{"BFuelCal.StartMap", "TorqueCal.M_EngMaxE85Tab"}},
		{"T7 overboost", t7{}, t7Syms(t).del("TorqueCal.M_OverBoostTab").fw(), t7Opts(map[string]float64{"overboost": 1}), []string{"TorqueCal.M_OverBoostTab"}},
		{"T7 convertible", t7{}, t7Syms(t).del("TorqueCal.M_CabGearLim").fw(), t7Opts(map[string]float64{"convertible": 1}), []string{"TorqueCal.M_CabGearLim"}},
		{"T7 5th gear", t7{}, t7Syms(t).del("TorqueCal.n_Eng5GearSP").fw(), t7Opts(nil), []string{"TorqueCal.n_Eng5GearSP"}},
		{"T8 E85", t8{}, t8Syms(t).del("FFAirCal.m_maxAirmass").fw(), t8Opts(map[string]float64{"e85": 1}), []string{"FFAirCal.m_maxAirmass"}},
		{"T8 engine limiter", t8{}, t8Syms(t).del("TrqLimCal.Trq_MaxEngineManTab1").fw(), t8Opts(nil), []string{"TrqLimCal.Trq_MaxEngineTab1"}},
		{"T8 overboost", t8{}, t8Syms(t).del("TrqLimCal.Trq_OverBoostTab").fw(), t8Opts(map[string]float64{"overboost": 1}), []string{"TrqLimCal.Trq_OverBoostTab"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.calc.Calculate(tt.fw, tt.opts)
			if err == nil {
				t.Fatal("expected an error")
			}
			if want := "missing symbols: " + strings.Join(tt.want, ", "); err.Error() != want {
				t.Fatalf("err = %q, want %q", err, want)
			}
		})
	}
	// an empty T8 collection names the whole required set
	_, err := t8{}.Calculate(symbol.NewCollection(), t8Opts(nil))
	if err == nil || !strings.Contains(err.Error(), "PedalMapCal.Trq_RequestMap") || !strings.Contains(err.Error(), "BFuelCal.TempEnrichFacMap") {
		t.Fatalf("empty T8 err = %v", err)
	}
}

// TestCalculateDimensionErrors rejects request maps smaller than their axes.
func TestCalculateDimensionErrors(t *testing.T) {
	short := []int{1, 2, 3}
	if _, err := (t7{}).Calculate(t7Syms(t).set(sym(t, "PedalMapCal.m_RequestMap", short...)).fw(), t7Opts(nil)); err == nil || !strings.Contains(err.Error(), "3 cells, expected 4x3") {
		t.Errorf("T7 err = %v", err)
	}
	if _, err := (t8{}).Calculate(t8Syms(t).set(sym(t, "PedalMapCal.Trq_RequestMap", short...)).fw(), t8Opts(nil)); err == nil || !strings.Contains(err.Error(), "3 cells, expected 4x3") {
		t.Errorf("T8 err = %v", err)
	}
	if _, err := (t5{}).Calculate(t5Syms(t).set(bsym(t, "Tryck_mat!", 1, 2)).fw(), nil); err == nil || !strings.Contains(err.Error(), "unexpected Tryck_mat!/Pwm_ind_rpm! dimensions") {
		t.Errorf("T5 err = %v", err)
	}
}

// TestT5ResultLayout: curve names/flags, power from torque, and the boost
// heat table with WOT on top, throttle headers, kPa -> gauge bar.
func TestT5ResultLayout(t *testing.T) {
	res, err := t5{}.Calculate(t5Syms(t).fw(), map[string]float64{"mapsensor": 1})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{res.Curves[0].Name, res.Curves[1].Name, res.Curves[2].Name}
	if !slices.Equal(names, []string{"Power (hp)", "Torque (Nm)", "Injector DC (%)"}) {
		t.Fatalf("curves = %v", names)
	}
	if !res.Curves[0].Peak || !res.Curves[1].Peak || res.Curves[2].Peak || res.Limiters != nil {
		t.Fatal("unexpected peak flags or limiters")
	}
	tbl := res.Table
	if tbl.Title != "Boost request (bar)" || tbl.Precision != 2 || !slices.Equal(tbl.Rows, []float64{155, 60, 10}) {
		t.Fatalf("table header = %q/%d/%v", tbl.Title, tbl.Precision, tbl.Rows)
	}
	// 3.0 bar sensor: raw * 1.2 kPa absolute -> bar gauge; the WOT column
	// raw 150/180/200/170 is 180/216/240/204 kPa
	wantTop := []float64{0.8, 1.16, 1.4, 1.04}
	wantBottom := []float64{0.2, 0.2, 0.2, 0.2}
	for i := range wantTop {
		if math.Abs(tbl.Cells[0][i]-wantTop[i]) > 1e-12 || math.Abs(tbl.Cells[2][i]-wantBottom[i]) > 1e-12 {
			t.Fatalf("cells = %v", tbl.Cells)
		}
	}
	// throttle axis shorter than the map: missing headers fall back to the column index
	res, err = t5{}.Calculate(t5Syms(t).set(bsym(t, "Pwm_ind_trot!", 10)).fw(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Table.Rows, []float64{2, 1, 10}) {
		t.Fatalf("fallback rows = %v", res.Table.Rows)
	}
}

// TestT5OutOfRangeChoices: unknown turbo/MAP sensor indices fall back to
// the stock turbo and 2.5 bar sensor.
func TestT5OutOfRangeChoices(t *testing.T) {
	fw := t5Syms(t).fw()
	base, err := t5{}.Calculate(fw, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []map[string]float64{{"turbo": 99, "mapsensor": -1}, {"turbo": -3, "mapsensor": 5}} {
		res, err := t5{}.Calculate(fw, opts)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(res.Curves[1].Values, base.Curves[1].Values) || !slices.Equal(res.Curves[2].Values, base.Curves[2].Values) {
			t.Fatalf("opts %v: %v, want %v", opts, res.Curves[1].Values, base.Curves[1].Values)
		}
	}
}

// TestCalculateIsRepeatable: recalculating from the same binary (the UI
// does this on every option change) gives identical results, so no scaling
// accumulates in the symbol data.
func TestCalculateIsRepeatable(t *testing.T) {
	for _, tc := range []struct {
		calc Calculator
		fw   symbol.FirmwareFile
		opts map[string]float64
	}{
		{t5{}, t5Syms(t).fw(), nil},
		{t7{}, t7Syms(t).fw(), t7Opts(map[string]float64{"nominal": 1})},
		{t8{}, t8Syms(t).fw(), t8Opts(map[string]float64{"nominal": 1})},
	} {
		a, err := tc.calc.Calculate(tc.fw, tc.opts)
		if err != nil {
			t.Fatal(err)
		}
		b, err := tc.calc.Calculate(tc.fw, tc.opts)
		if err != nil {
			t.Fatal(err)
		}
		for i := range a.Curves {
			if !slices.Equal(a.Curves[i].Values, b.Curves[i].Values) {
				t.Fatalf("%T curve %q changed: %v -> %v", tc.calc, a.Curves[i].Name, a.Curves[i].Values, b.Curves[i].Values)
			}
		}
	}
}

// TestEfficiencyScalesTorqueOnly: the efficiency option scales the torque
// estimate (and so power) but not the limited airmass or the fuel curves.
// At 90 % the fixtures' WOT torque (T7 290/373/350/274, T8 290/381/350/274
// Nm, see TestT7MatchesSuite/TestT8MatchesSuite) drops to the values below,
// rounded from the unrounded airmass/3.1 estimate.
func TestEfficiencyScalesTorqueOnly(t *testing.T) {
	for _, tc := range []struct {
		calc      Calculator
		fw        symbol.FirmwareFile
		opts      func(map[string]float64) map[string]float64
		tq, power []float64
	}{
		{t7{}, t7Syms(t).fw(), t7Opts, []float64{261, 336, 315, 247}, []float64{37, 143, 233, 210}},
		{t8{}, t8Syms(t).fw(), t8Opts, []float64{261, 343, 315, 247}, []float64{37, 146, 233, 210}},
	} {
		full, err := tc.calc.Calculate(tc.fw, tc.opts(nil))
		if err != nil {
			t.Fatal(err)
		}
		eff, err := tc.calc.Calculate(tc.fw, tc.opts(map[string]float64{"efficiency": 90}))
		if err != nil {
			t.Fatal(err)
		}
		if got := eff.Curves[1].Values; !slices.Equal(got, tc.tq) {
			t.Errorf("%T torque = %v, want %v", tc.calc, got, tc.tq)
		}
		if got := eff.Curves[0].Values; !slices.Equal(got, tc.power) {
			t.Errorf("%T power = %v, want %v", tc.calc, got, tc.power)
		}
		for _, k := range []int{2, 3, 4} {
			if !slices.Equal(eff.Curves[k].Values, full.Curves[k].Values) {
				t.Errorf("%T %s changed with efficiency", tc.calc, full.Curves[k].Name)
			}
		}
		if !slices.Equal(eff.Table.Cells[0], full.Table.Cells[0]) {
			t.Errorf("%T airmass changed with efficiency", tc.calc)
		}
	}
}

// TestT7AmbientFallback: an ambient pressure outside 50-150 kPa is treated
// as 100 kPa for the turbo speed limiter.
func TestT7AmbientFallback(t *testing.T) {
	fw := t7Syms(t).fw()
	base, err := t7{}.Calculate(fw, t7Opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, amb := range []float64{0, 49, 151} {
		res, err := t7{}.Calculate(fw, t7Opts(map[string]float64{"ambient": amb}))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(res.Table.Cells[0], base.Table.Cells[0]) || !slices.Equal(res.Limiters, base.Limiters) {
			t.Errorf("ambient %v: %v %q, want %v %q", amb, res.Table.Cells[0], res.Limiters, base.Table.Cells[0], base.Limiters)
		}
	}
}

// TestT7TurboSpeedNeedsAllTables: the turbo speed limiter is skipped unless
// all four LimEngCal tables are present.
func TestT7TurboSpeedNeedsAllTables(t *testing.T) {
	res, err := t7{}.Calculate(t7Syms(t).del("LimEngCal.TurboSpeedTab2").fw(), t7Opts(map[string]float64{"ambient": 70}))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(res.Limiters, limTurboSpeed) {
		t.Fatalf("turbo speed limiter active without TurboSpeedTab2: %q", res.Limiters)
	}
}

// TestT7AirmassTableLayout: table rows run from the highest pedal position
// down, labeled in percent, with RPM columns.
func TestT7AirmassTableLayout(t *testing.T) {
	res, err := t7{}.Calculate(t7Syms(t).fw(), t7Opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	tbl := res.Table
	if tbl.Title != "Limited airmass (mg/c)" || !slices.Equal(tbl.Rows, []float64{100, 50, 0}) || !slices.Equal(tbl.Cols, []float64{1000, 3000, 5200, 6000}) {
		t.Fatalf("table = %q rows %v cols %v", tbl.Title, tbl.Rows, tbl.Cols)
	}
	if len(tbl.Limiters) != 3 || len(tbl.Limiters[0]) != 4 {
		t.Fatalf("limiter grid shape %d x %d", len(tbl.Limiters), len(tbl.Limiters[0]))
	}
	want := []string{"Power (hp)", "Torque (Nm)", "Injector DC (%)", "Target lambda (*100)", "Fuel flow (l/h)"}
	for i, c := range res.Curves {
		if c.Name != want[i] {
			t.Fatalf("curve %d = %q, want %q", i, c.Name, want[i])
		}
		if c.Hidden != (c.Name == "Fuel flow (l/h)") || c.Peak != (i < 2) {
			t.Fatalf("curve %q hidden=%v peak=%v", c.Name, c.Hidden, c.Peak)
		}
	}
}

// TestT8EGTCurveOptional: the hidden EGT curve only appears when the binary
// has ExhaustCal.T_Lambda1Map.
func TestT8EGTCurveOptional(t *testing.T) {
	res, err := t8{}.Calculate(t8Syms(t).del("ExhaustCal.T_Lambda1Map").fw(), t8Opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Curves) != 5 {
		t.Fatalf("got %d curves without an EGT map, want 5", len(res.Curves))
	}
	res, err = t8{}.Calculate(t8Syms(t).fw(), t8Opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Curves[5]; c.Name != "EGT estimate (°C)" || !c.Hidden {
		t.Fatalf("EGT curve = %+v", c)
	}
}

// TestT8AutomaticWithoutOptionalTables: without BstKnkCal.MaxAirmassAu the
// automatic airmass limit is MaxAirmass, and a missing TMCCal table skips
// the TCM limiter instead of failing.
func TestT8AutomaticWithoutOptionalTables(t *testing.T) {
	m := t8Syms(t).del("BstKnkCal.MaxAirmassAu", "TMCCal.Trq_MaxEngineTab", "TMCCal.Trq_MaxEngineLowTab")
	res, err := t8{}.Calculate(m.fw(), t8Opts(map[string]float64{"automatic": 1, "fwlimit": 0}))
	if err != nil {
		t.Fatal(err)
	}
	// 1000/6000 rpm hit BstKnkCal.MaxAirmass (900/1000; the Au map would
	// give 850/950); in between Trq_MaxEngineAutTab1 binds and no TCM
	// limiter applies
	if want := []float64{900, 1095, 1072, 1000}; !slices.Equal(res.Table.Cells[0], want) {
		t.Fatalf("WOT airmass = %v, want %v", res.Table.Cells[0], want)
	}
	if want := []string{limAirmass, limTorqueEngine, limTorqueEngine, limAirmass}; !slices.Equal(res.Limiters, want) {
		t.Fatalf("limiters = %q, want %q", res.Limiters, want)
	}
}

// TestT8TorqueToAirmassQuirk: torque requests read as raw unsigned words
// are sign-folded with 65535 (not 65536), as in T8Suite.
func TestT8TorqueToAirmassQuirk(t *testing.T) {
	c := &t8calc{
		airTorqueMap: []int{0, 100, 0, 100},
		airTorqueX:   []int{-100, 0},
		airTorqueY:   []int{1000, 2000},
	}
	// 65486 -> -(65535-65486) = -49: 51 % of the way from -100 to 0
	if got := c.torqueToAirmass(65486, 1000); got != 51 {
		t.Fatalf("torqueToAirmass(65486) = %d, want 51", got)
	}
	if got := c.torqueToAirmass(-50, 1000); got != 50 {
		t.Fatalf("torqueToAirmass(-50) = %d, want 50", got)
	}
}

// TestT8EstimateEGT: no map gives 0, the VE map scales by 128/VE and E85
// subtracts 50 °C only above 50 °C.
func TestT8EstimateEGT(t *testing.T) {
	c := &t8calc{}
	if got := c.estimateEGT(500, 3000); got != 0 {
		t.Fatalf("no map: %d", got)
	}
	c.egtMap = []int{600}
	c.veX, c.veY = []int{0}, []int{0}
	if got := c.estimateEGT(500, 3000); got != 600 {
		t.Fatalf("no VE map: %d", got)
	}
	c.veMap = []int{160} // VE 1.25 -> egt / 1.25
	if got := c.estimateEGT(500, 3000); got != 480 {
		t.Fatalf("VE scaled: %d", got)
	}
	c.e85 = true
	if got := c.estimateEGT(500, 3000); got != 430 {
		t.Fatalf("E85: %d", got)
	}
	c.egtMap, c.veMap = []int{40}, nil
	if got := c.estimateEGT(500, 3000); got != 40 {
		t.Fatalf("E85 below 50 °C: %d", got)
	}
}

// TestFuelCalcEdges: missing injector data or VE map yields zeros instead
// of dividing by zero.
func TestFuelCalcEdges(t *testing.T) {
	f := &fuelCalc{veScale: 100, veMap: []int{100}, veX: []int{0}, veY: []int{0}}
	if got := f.injectorDC(500, 3000); got != 0 {
		t.Errorf("injConst 0: dc = %d", got)
	}
	f.injConst = 300
	f.veMap = nil
	if f.injectorDC(500, 3000) != 0 || f.targetLambda(500, 3000, 50) != 0 {
		t.Error("empty VE map must give 0 DC and lambda")
	}
	f.veMap = []int{0}
	if got := f.targetLambda(500, 3000, 50); got != 0 {
		t.Errorf("VE 0: lambda = %d", got)
	}
	if got := f.fuelFlow(500, 3000, 0); got != 0 {
		t.Errorf("lambda 0: flow = %d", got)
	}
	f.e85 = true
	if f.afr() != 9.84 {
		t.Errorf("E85 afr = %v", f.afr())
	}
	// 1000 mg/c at lambda 1.00, 3000 rpm on E85: 0.1016 g/c * 100 c/s =
	// 10.16 g/s = 47.2 l/h at 0.775 kg/l
	if got := f.fuelFlow(1000, 3000, 100); got != 47 {
		t.Errorf("E85 flow = %d, want 47", got)
	}
}

// TestSummarize lists curve peaks (skipping NaN) and merges consecutive
// rpm points sharing a limiter into spans.
func TestSummarize(t *testing.T) {
	nan := math.NaN()
	res := &Result{
		RPM: []float64{1000, 2000, 3000, 4000, 5000, 6000},
		Curves: []Curve{
			{Name: "Power (hp)", Values: []float64{nan, 50, 120, 180, 175, nan}, Peak: true},
			{Name: "Hidden", Values: []float64{999, 999, 999, 999, 999, 999}},
			{Name: "Empty", Values: nil, Peak: true},
			{Name: "AllNaN", Values: []float64{nan, nan, nan, nan, nan, nan}, Peak: true},
		},
		Limiters: []string{limAirmass, limAirmass, "", limFuelCut, limAirmass, limAirmass},
	}
	want := "peak Power (hp) 180 @ 4000 rpm · airmass (knock) limiter 1000-2000 rpm · fuel cut limiter @ 4000 rpm · airmass (knock) limiter 5000-6000 rpm"
	if got := summarize(res); got != want {
		t.Fatalf("summarize =\n%q\nwant\n%q", got, want)
	}
	// the same limiter on both sides of a gap gives two spans, not one
	gap := &Result{RPM: []float64{1000, 2000, 3000}, Limiters: []string{limAirmass, "", limAirmass}}
	if got, want := summarize(gap), "airmass (knock) limiter @ 1000 rpm · airmass (knock) limiter @ 3000 rpm"; got != want {
		t.Fatalf("gap: %q, want %q", got, want)
	}
	// limiters only count when there is one per rpm point
	res.Limiters = res.Limiters[:3]
	if got := summarize(res); got != "peak Power (hp) 180 @ 4000 rpm" {
		t.Fatalf("mismatched limiters: %q", got)
	}
	if got := summarize(&Result{}); got != "" {
		t.Fatalf("empty result: %q", got)
	}
}

// t5Calc is the fixture's injection data with byte-wide temperature tables.
func t5Calc(t *testing.T) *t5calc {
	t.Helper()
	m := t5Syms(t)
	fuelY := m["Fuel_map_yaxis!"].Ints()
	for i := range fuelY {
		fuelY[i] *= 10
	}
	return &t5calc{
		injKonst:     21,
		minTid:       250,
		battKorr:     m["Batt_korr_tab!"].Ints(),
		luftKompfak:  m["Luft_kompfak!"].Ints(),
		lufttempSteg: m["Lufttemp_steg!"].Ints(),
		insp:         m["Insp_mat!"].Ints(),
		fuelX:        m["Fuel_map_xaxis!"].Ints(),
		fuelY:        fuelY,
	}
}

// TestT5InjectorDCClamps: injection time is clamped to [Min_tid!, 32500]
// ticks of 4 µs before converting to duty cycle, and a battery correction
// table without the 13 V entry adds nothing.
func TestT5InjectorDCClamps(t *testing.T) {
	c := t5Calc(t)
	c.minTid = 5000
	// 5000 ticks = 20 ms per injection, at 3000 rpm (25 injections/s) 50 %
	if got := c.injectorDC(1, 3000); got != 50 {
		t.Errorf("Min_tid clamp: DC = %v, want 50", got)
	}
	c = t5Calc(t)
	c.injKonst = 1000
	// 32500 ticks = 130 ms per injection: 325 % at 3000 rpm
	if got := c.injectorDC(200, 3000); got != 325 {
		t.Errorf("32500 clamp: DC = %v, want 325", got)
	}
	zero, short := t5Calc(t), t5Calc(t)
	zero.battKorr = []int{500, 0}
	short.battKorr = []int{500}
	if a, b := zero.injectorDC(180, 4000), short.injectorDC(180, 4000); a != b {
		t.Errorf("short battery table: DC %v, want %v (no correction)", b, a)
	}
}

// TestT5TempTableEdges: without a 255 sentinel, or with a table shorter
// than its axis, the last available entry holds.
func TestT5TempTableEdges(t *testing.T) {
	if got := t5TempTable(250, []int{10, 20, 30}, []int{50, 100, 150}); got != 30 {
		t.Errorf("past unterminated axis = %d, want 30", got)
	}
	if got := t5TempTable(250, []int{10, 20}, []int{50, 100, 150}); got != 20 {
		t.Errorf("table shorter than axis = %d, want 20", got)
	}
}

// TestT5FuelCellShortMatrix: cells beyond a truncated matrix read as 0.
func TestT5FuelCellShortMatrix(t *testing.T) {
	// row 1 is missing: the rpm interpolation pulls towards 0
	if got := t5FuelCell(1500, 40, []int{100, 100}, []int{1000, 2000}, []int{40, 100}); got != 75 {
		t.Fatalf("t5FuelCell = %d, want 75", got)
	}
}
