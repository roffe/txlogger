package estimatedoutput

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

// Known-answer vectors in this file were produced by compiling the
// TuningSuites C# methods verbatim (T7Suite/T8Suite ctrlAirmassResult.cs,
// T5Suite2.0 frmDynoChart.cs, CommonSuite PressureToTorque.cs) with mono
// and running them over the same inputs. Power is not compared: the suites
// divide by 7121 (bhp), the port by 7024 (metric hp).

// TestInterpTableMatchesSuite pins interpTable to GetInterpolatedTableValue,
// including its quirks: the axis scans keep the last index strictly below
// the value, out-of-range table reads yield 0 and values past the last
// breakpoint clamp.
func TestInterpTableMatchesSuite(t *testing.T) {
	tests := []struct {
		name        string
		table, x, y []int
		yv, xv      int
		want        float64
	}{
		{"center", []int{10, 20, 30, 40}, []int{0, 100}, []int{1000, 2000}, 1500, 50, 25},
		{"clamp above both axes", []int{10, 20, 30, 40}, []int{0, 100}, []int{1000, 2000}, 3000, 200, 40},
		{"below both axes", []int{10, 20, 30, 40}, []int{0, 100}, []int{1000, 2000}, 500, -50, 10},
		{"1-D", []int{100, 200}, []int{0}, []int{1000, 2000}, 1500, 0, 150},
		{"1-D clamp", []int{100, 200}, []int{0}, []int{1000, 2000}, 2500, 0, 200},
		{"non-monotonic axis keeps last match", []int{10, 20, 30, 40, 50, 60}, []int{0, 100}, []int{1000, 3000, 2000}, 2500, 0, 50},
		{"descending cells", []int{40, 30, 20, 10}, []int{0, 100}, []int{1000, 2000}, 1500, 50, 25},
		{"short table reads out of range", []int{10, 20, 30}, []int{0, 100}, []int{1000, 2000}, 1500, 50, 0},
		{"short table last cell", []int{10, 20, 30}, []int{0, 100}, []int{1000, 2000}, 2500, 150, 0},
		{"x past last column", []int{10, 20, 30, 40}, []int{0, 100}, []int{1000, 2000}, 1500, 500, 30},
		{"3x3 fractional", []int{0, 10, 20, 5, 15, 25, 10, 20, 30}, []int{0, 50, 100}, []int{0, 1000, 2000}, 1250, 30, 12.25},
		{"on interior breakpoint", []int{10, 20, 30, 40, 50, 60, 70, 80, 90}, []int{0, 100, 200}, []int{1000, 2000, 3000}, 2000, 100, 50},
		{"negative axis", []int{100, 200, 300, 400}, []int{-100, 0}, []int{1000, 2000}, 1500, 0, 300},
		{"unsigned axis, negative value", []int{1, 2, 3, 4, 5, 6, 7, 8}, []int{65516, 100, 300, 500}, []int{1000, 2000}, 1000, -50, 1},
		{"unsigned axis, overshoot pct", []int{1, 2, 3, 4, 5, 6, 7, 8}, []int{65516, 100, 300, 500}, []int{1000, 2000}, 1700, 400, 6.3},
		{"3x4 airtorque", []int{37, 69, 114, 155, 42, 78, 114, 154, 48, 81, 113, 155}, []int{0, 20, 40, 60}, []int{880, 1260, 1640}, 1450, 33, 101.6},
		{"turbo speed by pressure", []int{900, 1400}, []int{0}, []int{650, 1000}, 700, 0, 971.42857142857144},
		{"empty table", nil, []int{0}, []int{1000}, 1000, 0, 0},
		{"empty axis", []int{1}, nil, []int{1000}, 1000, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interpTable(tt.table, tt.x, tt.y, tt.yv, tt.xv); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("interpTable = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestT5PressureToTorqueAllTurbos pins every turbo family's boost->torque
// lookup to CommonSuite PressureToTorque, including extrapolation below
// 0.2 bar and the flat clamp above 2.0 bar.
func TestT5PressureToTorqueAllTurbos(t *testing.T) {
	bars := []float64{-0.5, 0, 0.2, 0.5, 1.0, 1.37, 2.0, 2.5}
	want := [][]float64{
		{-15, 110, 160, 225, 300, 402.5, 460, 460},        // Stock
		{22.5, 147.5, 197.5, 267.5, 365, 452.5, 640, 640}, // GT17
		{-5, 120, 170, 240, 340, 442.5, 560, 560},         // TD04 15T
		{35, 160, 210, 280, 400, 484, 620, 620},           // TD04 19T
		{35, 160, 210, 280, 400, 484, 620, 620},           // GT28BB
		{35, 160, 210, 280, 400, 484, 620, 620},           // GT28RS
		{75, 200, 250, 320, 440, 514, 650, 650},           // GT3071R
		{75, 200, 250, 320, 440, 514, 650, 650},           // HX35w
		{95, 220, 270, 340, 460, 534, 670, 670},           // HX40w
		{95, 220, 270, 340, 460, 534, 670, 670},           // S400SX3-71
	}
	if len(t5TorqueTables) != len(t5Turbos) || len(want) != len(t5Turbos) {
		t.Fatalf("%d torque tables for %d turbos", len(t5TorqueTables), len(t5Turbos))
	}
	for ti, row := range want {
		for bi, bar := range bars {
			if got := t5PressureToTorque(bar, t5TorqueTables[ti]); math.Abs(got-row[bi]) > 1e-9 {
				t.Errorf("%s @ %v bar = %v, want %v", t5Turbos[ti], bar, got, row[bi])
			}
		}
	}
}

// TestT5TempTableMatchesSuite pins Handle_temp_tables: integer breakpoint
// interpolation up to the 255 sentinel, for rising and falling tables.
func TestT5TempTableMatchesSuite(t *testing.T) {
	kompfak := []int{86, 93, 99, 106, 113, 120, 128, 136, 144, 154, 163, 173, 183, 195, 206, 219}
	steg := []int{11, 15, 19, 26, 34, 45, 58, 74, 96, 124, 150, 174, 201, 220, 234, 255}
	falling := []int{200, 180, 150, 120, 100, 90}
	fallingSteg := []int{20, 60, 100, 140, 180, 255}
	tests := []struct {
		name      string
		ad        int
		tab, steg []int
		want      int
	}{
		{"20 C", 163, kompfak, steg, 168},
		{"below first breakpoint", 5, kompfak, steg, 86},
		{"on a breakpoint", 34, kompfak, steg, 113},
		{"past last breakpoint, before sentinel", 240, kompfak, steg, 209},
		{"falling table on breakpoint", 100, falling, fallingSteg, 150},
		{"falling table between", 75, falling, fallingSteg, 169},
		{"empty table", 100, nil, steg, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := t5TempTable(tt.ad, tt.tab, tt.steg); got != tt.want {
				t.Fatalf("t5TempTable(%d) = %d, want %d", tt.ad, got, tt.want)
			}
		})
	}
}

// TestT5FuelCellMatchesSuite pins the byte flavor of Handle_tables,
// including its uint8 wrap-around when a negative step overflows.
func TestT5FuelCellMatchesSuite(t *testing.T) {
	insp := []int{40, 60, 80, 100, 45, 65, 90, 110, 50, 70, 95, 120, 55, 75, 100, 127}
	rpmAxis := []int{1000, 2500, 4000, 5500}
	loadAxis := []int{40, 100, 160, 230}
	tests := []struct {
		name         string
		rpm          int
		load         uint8
		matrix, y, x []int
		want         uint8
	}{
		{"on rpm breakpoint", 2500, 120, insp, rpmAxis, loadAxis, 69},
		{"between breakpoints", 3000, 130, insp, rpmAxis, loadAxis, 71},
		{"past both axes", 6000, 250, insp, rpmAxis, loadAxis, 127},
		{"below both axes", 500, 20, insp, rpmAxis, loadAxis, 36},
		{"negative step wraps", 1500, 70, []int{250, 10, 250, 10}, []int{1000, 2000}, []int{40, 100}, 190},
		{"mixed steps wrap", 1500, 70, []int{10, 250, 250, 10}, []int{1000, 2000}, []int{40, 100}, 130},
		{"empty matrix", 1500, 70, nil, []int{1000}, []int{40}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := t5FuelCell(tt.rpm, tt.load, tt.matrix, tt.y, tt.x); got != tt.want {
				t.Fatalf("t5FuelCell = %d, want %d", got, tt.want)
			}
		})
	}
}

// oracleWant is the suite's WOT row: limited airmass, active limiter,
// torque, injector DC (clamped to 100), target lambda*100, fuel flow and
// EGT (T8 only).
type oracleWant struct {
	air  []int
	lim  []string
	tq   []int
	dc   []int
	lam  []int
	flow []int
	egt  []int
}

func checkOracle(t *testing.T, res *Result, want oracleWant, withEGT bool) {
	t.Helper()
	curve := func(name string) []float64 {
		for _, c := range res.Curves {
			if c.Name == name {
				return c.Values
			}
		}
		t.Fatalf("no curve %q", name)
		return nil
	}
	ints := func(v []float64) []int {
		out := make([]int, len(v))
		for i, f := range v {
			out[i] = int(f)
		}
		return out
	}
	// the WOT row is the top row of the airmass table
	air := ints(res.Table.Cells[0])
	checks := []struct {
		name      string
		got, want []int
	}{
		{"airmass", air, want.air},
		{"torque", ints(curve("Torque (Nm)")), want.tq},
		{"injector DC", ints(curve("Injector DC (%)")), want.dc},
		{"lambda", ints(curve("Target lambda (*100)")), want.lam},
		{"fuel flow", ints(curve("Fuel flow (l/h)")), want.flow},
	}
	if withEGT {
		checks = append(checks, struct {
			name      string
			got, want []int
		}{"EGT", ints(curve("EGT estimate (°C)")), want.egt})
	}
	for _, c := range checks {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if !slices.Equal(res.Limiters, want.lim) {
		t.Errorf("limiters = %q, want %q", res.Limiters, want.lim)
	}
	if !slices.Equal(res.Table.Limiters[0], want.lim) {
		t.Errorf("WOT table limiters = %q, want %q", res.Table.Limiters[0], want.lim)
	}
	// power is the torque curve through the metric hp constant, integer
	// division like the suites
	tq, pwr := curve("Torque (Nm)"), curve("Power (hp)")
	for i, r := range res.RPM {
		if want := float64(int(tq[i]) * int(r) / 7024); pwr[i] != want {
			t.Errorf("power[%d] = %v, want %v", i, pwr[i], want)
		}
	}
}

func t7Opts(kv map[string]float64) map[string]float64 {
	m := map[string]float64{"fwlimit": 1, "gear": 5, "ambient": 100, "efficiency": 100}
	for k, v := range kv {
		m[k] = v
	}
	return m
}

func t8Opts(kv map[string]float64) map[string]float64 {
	m := map[string]float64{"fwlimit": 1, "gear": 5, "highoutput": 1, "efficiency": 100}
	for k, v := range kv {
		m[k] = v
	}
	return m
}

// TestT7MatchesSuite runs the whole T7 chain over the synthetic binary for
// every limiter path and compares the WOT row with T7Suite.
func TestT7MatchesSuite(t *testing.T) {
	type f = map[string]float64
	tests := []struct {
		name string
		mod  func(syms)
		opts map[string]float64
		want oracleWant
	}{
		{"default", nil, t7Opts(nil),
			oracleWant{air: []int{900, 1157, 1096, 1000}, lim: []string{limAirmass, limTorqueGear, limAirmass, limAirmass}, tq: []int{290, 373, 350, 274}, dc: []int{15, 63, 100, 100}, lam: []int{94, 88, 88, 93}, flow: []int{10, 44, 71, 71}}},
		{"no firmware limit, 3rd gear", nil, t7Opts(f{"fwlimit": 0, "gear": 3}),
			oracleWant{air: []int{900, 1180, 1096, 1000}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{290, 381, 350, 274}, dc: []int{15, 65, 100, 100}, lam: []int{94, 87, 88, 93}, flow: []int{10, 45, 71, 71}}},
		{"automatic", nil, t7Opts(f{"automatic": 1}),
			oracleWant{air: []int{850, 1000, 1030, 950}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{274, 323, 329, 260}, dc: []int{15, 54, 100, 100}, lam: []int{95, 90, 86, 88}, flow: []int{10, 37, 69, 72}}},
		{"automatic engine limiter", func(m syms) { m.set(sym(t, "TorqueCal.M_EngMaxAutTab", 240, 280, 280, 260)) }, t7Opts(f{"fwlimit": 0, "automatic": 1}),
			oracleWant{air: []int{774, 879, 884, 862}, lim: []string{limTorqueEngine, limTorqueEngine, limTorqueEngine, limTorqueEngine}, tq: []int{250, 284, 282, 236}, dc: []int{13, 47, 85, 97}, lam: []int{96, 91, 87, 86}, flow: []int{9, 32, 58, 66}}},
		{"engine limiter", func(m syms) { m.set(sym(t, "TorqueCal.M_EngMaxTab", 250, 300, 300, 280)) }, t7Opts(f{"fwlimit": 0, "gear": 4}),
			oracleWant{air: []int{805, 940, 946, 926}, lim: []string{limTorqueEngine, limTorqueEngine, limTorqueEngine, limTorqueEngine}, tq: []int{260, 303, 302, 254}, dc: []int{14, 50, 92, 100}, lam: []int{95, 90, 86, 86}, flow: []int{9, 35, 63, 71}}},
		{"E85", nil, t7Opts(f{"e85": 1}),
			oracleWant{air: []int{900, 1157, 1096, 1000}, lim: []string{limAirmass, limTorqueGear, limAirmass, limAirmass}, tq: []int{311, 399, 375, 293}, dc: []int{29, 100, 100, 100}, lam: []int{74, 80, 132, 139}, flow: []int{19, 68, 68, 68}}},
		{"E85 limiter", func(m syms) { m.set(sym(t, "TorqueCal.M_EngMaxE85Tab", 250, 300, 300, 280)) }, t7Opts(f{"fwlimit": 0, "gear": 4, "e85": 1}),
			oracleWant{air: []int{805, 940, 946, 926}, lim: []string{limTorqueE85, limTorqueE85, limTorqueE85, limTorqueE85}, tq: []int{278, 324, 323, 272}, dc: []int{25, 93, 100, 100}, lam: []int{75, 71, 114, 128}, flow: []int{17, 62, 68, 68}}},
		{"E85 automatic", nil, t7Opts(f{"e85": 1, "automatic": 1}),
			oracleWant{air: []int{850, 1000, 1030, 950}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{293, 345, 352, 279}, dc: []int{27, 100, 100, 100}, lam: []int{75, 71, 123, 132}, flow: []int{18, 66, 68, 68}}},
		{"E85 automatic limiter", func(m syms) { m.set(sym(t, "TorqueCal.M_EngMaxE85TabAut", 240, 280, 280, 260)) }, t7Opts(f{"fwlimit": 0, "e85": 1, "automatic": 1}),
			oracleWant{air: []int{774, 879, 884, 862}, lim: []string{limTorqueE85Auto, limTorqueE85Auto, limTorqueE85Auto, limTorqueE85Auto}, tq: []int{267, 303, 302, 253}, dc: []int{24, 87, 100, 100}, lam: []int{75, 72, 106, 119}, flow: []int{16, 58, 68, 68}}},
		{"overboost", nil, t7Opts(f{"overboost": 1}),
			oracleWant{air: []int{900, 1095, 1096, 990}, lim: []string{limAirmass, limOverboost, limAirmass, limOverboost}, tq: []int{290, 353, 350, 271}, dc: []int{15, 60, 100, 100}, lam: []int{94, 88, 88, 92}, flow: []int{10, 41, 71, 71}}},
		{"convertible 2nd gear", nil, t7Opts(f{"convertible": 1, "gear": 2}),
			oracleWant{air: []int{900, 971, 990, 1000}, lim: []string{limAirmass, limTorqueGear, limTorqueGear, limAirmass}, tq: []int{290, 313, 316, 274}, dc: []int{15, 52, 96, 100}, lam: []int{94, 90, 86, 93}, flow: []int{10, 36, 66, 71}}},
		{"1st gear table", nil, t7Opts(f{"gear": 1}),
			oracleWant{air: []int{650, 818, 896, 926}, lim: []string{limTorqueGear, limTorqueGear, limTorqueGear, limTorqueGear}, tq: []int{210, 264, 286, 254}, dc: []int{11, 43, 86, 100}, lam: []int{97, 92, 87, 86}, flow: []int{7, 29, 59, 71}}},
		{"1st gear without table", func(m syms) { m.del("TorqueCal.M_1GearTab", "TorqueCal.n_Eng1GearSP") }, t7Opts(f{"gear": 1}),
			oracleWant{air: []int{900, 1033, 1053, 1000}, lim: []string{limAirmass, limTorqueGear, limTorqueGear, limAirmass}, tq: []int{290, 333, 336, 274}, dc: []int{15, 56, 100, 100}, lam: []int{94, 89, 85, 93}, flow: []int{10, 38, 71, 71}}},
		{"nominal torque map", nil, t7Opts(f{"nominal": 1}),
			oracleWant{air: []int{900, 1157, 1096, 1000}, lim: []string{limAirmass, limTorqueGear, limAirmass, limAirmass}, tq: []int{285, 380, 341, 290}, dc: []int{15, 63, 100, 100}, lam: []int{94, 88, 88, 93}, flow: []int{10, 44, 71, 71}}},
		{"turbo speed at altitude", nil, t7Opts(f{"ambient": 70}),
			oracleWant{air: []int{900, 893, 807, 776}, lim: []string{limAirmass, limTurboSpeed, limTurboSpeed, limTurboSpeed}, tq: []int{290, 288, 258, 213}, dc: []int{15, 48, 77, 86}, lam: []int{94, 91, 89, 87}, flow: []int{10, 32, 52, 59}}},
		{"fuel cut", func(m syms) { m.set(sym(t, "FCutCal.m_AirInletLimit", 950)) }, t7Opts(nil),
			oracleWant{air: []int{900, 950, 950, 950}, lim: []string{limAirmass, limFuelCut, limFuelCut, limFuelCut}, tq: []int{290, 306, 303, 260}, dc: []int{15, 51, 92, 100}, lam: []int{94, 90, 86, 88}, flow: []int{10, 35, 63, 72}}},
		{"torque limiting disabled (ST_Loop 0)", func(m syms) { m.set(bsym(t, "TorqueCal.ST_Loop", 0)) }, t7Opts(nil),
			oracleWant{air: []int{900, 1180, 1096, 1000}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{290, 381, 350, 274}, dc: []int{15, 65, 100, 100}, lam: []int{94, 87, 88, 93}, flow: []int{10, 45, 71, 71}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := t7Syms(t)
			if tt.mod != nil {
				tt.mod(m)
			}
			res, err := t7{}.Calculate(m.fw(), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			checkOracle(t, res, tt.want, false)
		})
	}
}

// TestT7FullGridMatchesSuite compares every pedal x rpm cell, not just WOT.
func TestT7FullGridMatchesSuite(t *testing.T) {
	res, err := t7{}.Calculate(t7Syms(t).fw(), t7Opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	// suite grid by pedal column (0, 50, 100 %); the table puts WOT on top
	want := [][]float64{{80, 80, 80, 80}, {450, 600, 650, 600}, {900, 1157, 1096, 1000}}
	wantLim := [][]string{{"", "", "", ""}, {"", "", "", ""}, {limAirmass, limTorqueGear, limAirmass, limAirmass}}
	for p := range want {
		row := len(want) - 1 - p
		if !slices.Equal(res.Table.Cells[row], want[p]) {
			t.Errorf("pedal col %d = %v, want %v", p, res.Table.Cells[row], want[p])
		}
		if !slices.Equal(res.Table.Limiters[row], wantLim[p]) {
			t.Errorf("pedal col %d limiters = %q, want %q", p, res.Table.Limiters[row], wantLim[p])
		}
	}
}

// TestT7AutomaticWithoutAuMap: a binary lacking BstKnkCal.MaxAirmassAu
// falls back to the manual MaxAirmass map (the suite would clamp every cell
// to 0); the result equals the suite run with MaxAirmassAu = MaxAirmass.
func TestT7AutomaticWithoutAuMap(t *testing.T) {
	m := t7Syms(t).del("BstKnkCal.MaxAirmassAu")
	res, err := t7{}.Calculate(m.fw(), t7Opts(map[string]float64{"automatic": 1}))
	if err != nil {
		t.Fatal(err)
	}
	checkOracle(t, res, oracleWant{
		air: []int{898, 1033, 1034, 990}, lim: []string{limTorqueEngine, limTorqueEngine, limTorqueEngine, limTorqueEngine},
		tq: []int{290, 333, 330, 271}, dc: []int{15, 56, 100, 100}, lam: []int{94, 89, 86, 92}, flow: []int{10, 38, 69, 71},
	}, false)
}

// TestT8MatchesSuite runs the whole T8 chain over the synthetic binary for
// every limiter path and compares the WOT row with T8Suite.
func TestT8MatchesSuite(t *testing.T) {
	type f = map[string]float64
	tests := []struct {
		name string
		mod  func(syms)
		opts map[string]float64
		want oracleWant
	}{
		{"default", nil, t8Opts(nil),
			oracleWant{air: []int{900, 1180, 1096, 1000}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{290, 381, 350, 274}, dc: []int{15, 64, 100, 100}, lam: []int{95, 88, 89, 93}, flow: []int{10, 44, 71, 71}, egt: []int{687, 748, 760, 762}}},
		{"low output", nil, t8Opts(f{"highoutput": 0}),
			oracleWant{air: []int{898, 1064, 1034, 990}, lim: []string{limTorqueEngine, limTorqueEngine, limTorqueEngine, limTorqueEngine}, tq: []int{290, 343, 330, 271}, dc: []int{15, 57, 99, 100}, lam: []int{95, 90, 87, 92}, flow: []int{10, 39, 68, 71}, egt: []int{687, 732, 753, 761}}},
		{"new style engine limiter", func(m syms) {
			m.del("TrqLimCal.Trq_MaxEngineManTab1", "TrqLimCal.Trq_MaxEngineManTab2", "TrqLimCal.Trq_MaxEngineAutTab1", "TrqLimCal.Trq_MaxEngineAutTab2").
				set(sym(t, "TrqLimCal.Trq_MaxEngineTab1", 3100, 3700, 3500, 3200), sym(t, "TrqLimCal.Trq_MaxEngineTab2", 2900, 3300, 3200, 3000))
		}, t8Opts(f{"highoutput": 0}),
			oracleWant{air: []int{900, 1033, 1009, 990}, lim: []string{limAirmass, limTorqueEngine, limTorqueEngine, limTorqueEngine}, tq: []int{290, 333, 322, 271}, dc: []int{15, 55, 97, 100}, lam: []int{95, 90, 87, 92}, flow: []int{10, 38, 66, 71}, egt: []int{687, 727, 750, 761}}},
		{"no firmware limit, 4th gear", nil, t8Opts(f{"fwlimit": 0, "gear": 4}),
			oracleWant{air: []int{900, 1180, 1096, 1000}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{290, 381, 350, 274}, dc: []int{15, 64, 100, 100}, lam: []int{95, 88, 89, 93}, flow: []int{10, 44, 71, 71}, egt: []int{687, 748, 760, 762}}},
		{"reverse gear", nil, t8Opts(f{"gear": 7}),
			oracleWant{air: []int{805, 788, 803, 830}, lim: []string{limTorqueGear, limTorqueGear, limTorqueGear, limTorqueGear}, tq: []int{260, 254, 256, 228}, dc: []int{14, 41, 75, 92}, lam: []int{96, 93, 90, 88}, flow: []int{9, 28, 51, 62}, egt: []int{673, 692, 725, 742}}},
		{"automatic", nil, t8Opts(f{"automatic": 1}),
			oracleWant{air: []int{850, 1000, 1030, 950}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{274, 323, 329, 260}, dc: []int{14, 53, 99, 100}, lam: []int{95, 90, 87, 89}, flow: []int{10, 37, 68, 71}, egt: []int{680, 723, 752, 757}}},
		{"automatic low output", nil, t8Opts(f{"automatic": 1, "highoutput": 0}),
			oracleWant{air: []int{836, 940, 946, 926}, lim: []string{limTorqueGear, limTorqueGear, limTorqueGear, limTorqueGear}, tq: []int{270, 303, 302, 254}, dc: []int{14, 50, 90, 100}, lam: []int{96, 91, 88, 86}, flow: []int{10, 34, 62, 71}, egt: []int{678, 714, 742, 754}}},
		{"E85", nil, t8Opts(f{"e85": 1}),
			oracleWant{air: []int{950, 1200, 1130, 1050}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{328, 414, 386, 308}, dc: []int{27, 100, 100, 100}, lam: []int{82, 83, 136, 146}, flow: []int{18, 68, 68, 68}, egt: []int{556, 609, 624, 628}}},
		{"E85 limiter", func(m syms) { m.set(sym(t, "FFTrqCal.FFTrq_MaxEngineTab1", 2500, 3000, 3000, 2800)) }, t8Opts(f{"e85": 1, "fwlimit": 0, "gear": 4}),
			oracleWant{air: []int{805, 940, 946, 926}, lim: []string{limTorqueE85, limTorqueE85, limTorqueE85, limTorqueE85}, tq: []int{278, 324, 323, 272}, dc: []int{23, 84, 100, 100}, lam: []int{83, 80, 114, 129}, flow: []int{15, 55, 68, 68}, egt: []int{535, 575, 603, 614}}},
		{"E85 automatic", nil, t8Opts(f{"e85": 1, "automatic": 1}),
			oracleWant{air: []int{850, 1000, 1030, 950}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{293, 345, 352, 279}, dc: []int{24, 89, 100, 100}, lam: []int{83, 79, 124, 132}, flow: []int{16, 60, 68, 68}, egt: []int{542, 583, 612, 617}}},
		{"overboost", nil, t8Opts(f{"overboost": 1}),
			oracleWant{air: []int{900, 1095, 1096, 990}, lim: []string{limAirmass, limOverboost, limAirmass, limOverboost}, tq: []int{290, 353, 350, 271}, dc: []int{15, 59, 100, 100}, lam: []int{95, 89, 89, 92}, flow: []int{10, 41, 71, 71}, egt: []int{687, 736, 760, 761}}},
		{"nominal torque map", nil, t8Opts(f{"nominal": 1}),
			oracleWant{air: []int{900, 1180, 1096, 1000}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{285, 388, 341, 290}, dc: []int{15, 64, 100, 100}, lam: []int{95, 88, 89, 93}, flow: []int{10, 44, 71, 71}, egt: []int{687, 748, 760, 762}}},
		{"fuel cut", func(m syms) { m.set(sym(t, "FCutCal.m_AirInletLimit", 950)) }, t8Opts(nil),
			oracleWant{air: []int{900, 950, 950, 950}, lim: []string{limAirmass, limFuelCut, limFuelCut, limFuelCut}, tq: []int{290, 306, 303, 260}, dc: []int{15, 51, 91, 100}, lam: []int{95, 91, 88, 89}, flow: []int{10, 35, 62, 71}, egt: []int{687, 716, 743, 757}}},
		{"flexifuel knock axis name", func(m syms) {
			m.del("BstKnkCal.OffsetXSP").set(ssym(t, "BstKnkCal.fi_offsetXSP", -100, 0))
		}, t8Opts(nil),
			oracleWant{air: []int{900, 1180, 1096, 1000}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass}, tq: []int{290, 381, 350, 274}, dc: []int{15, 64, 100, 100}, lam: []int{95, 88, 89, 93}, flow: []int{10, 44, 71, 71}, egt: []int{687, 748, 760, 762}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := t8Syms(t)
			if tt.mod != nil {
				tt.mod(m)
			}
			res, err := t8{}.Calculate(m.fw(), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			checkOracle(t, res, tt.want, true)
		})
	}
}

// TestT8FullGridMatchesSuite compares every pedal x rpm cell, including the
// negative low-pedal torque requests.
func TestT8FullGridMatchesSuite(t *testing.T) {
	res, err := t8{}.Calculate(t8Syms(t).fw(), t8Opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]float64{{60, 65, 71, 75}, {495, 635, 710, 670}, {900, 1180, 1096, 1000}}
	for p := range want {
		if row := res.Table.Cells[len(want)-1-p]; !slices.Equal(row, want[p]) {
			t.Errorf("pedal col %d = %v, want %v", p, row, want[p])
		}
	}
}

// TestT8E85AirmassLimitAxis: T8Suite looks the flexfuel airmass limit
// FFAirCal.m_maxAirmass up at 0° ignition offset on its own x-axis
// FFAirCal.fi_offsetXSP. With an axis that differs from BstKnkCal's, the
// port must still read the 0° column.
func TestT8E85AirmassLimitAxis(t *testing.T) {
	m := t8Syms(t).set(
		ssym(t, "FFAirCal.fi_offsetXSP", -100, 0, 100),
		ssym(t, "FFAirCal.m_maxAirmass", 850, 900, 1300, 1000, 1150, 1300, 1050, 1100, 1300, 950, 980, 1300),
	)
	res, err := t8{}.Calculate(m.fw(), t8Opts(map[string]float64{"e85": 1}))
	if err != nil {
		t.Fatal(err)
	}
	checkOracle(t, res, oracleWant{
		air: []int{900, 1150, 1076, 980}, lim: []string{limAirmass, limAirmass, limAirmass, limAirmass},
		tq: []int{311, 397, 368, 288}, dc: []int{26, 100, 100, 100}, lam: []int{83, 80, 129, 136},
		flow: []int{17, 68, 68, 68}, egt: []int{549, 603, 618, 620},
	}, true)
}

// t5Check compares a T5 result with the suite's torque (exact) and
// injector DC (the suite computes it in float32).
func t5Check(t *testing.T, res *Result, rpm, tq, dc []float64) {
	t.Helper()
	if !slices.Equal(res.RPM, rpm) {
		t.Fatalf("rpm = %v, want %v", res.RPM, rpm)
	}
	gotTq, gotDC, gotP := res.Curves[1].Values, res.Curves[2].Values, res.Curves[0].Values
	for i := range rpm {
		if math.Abs(gotTq[i]-tq[i]) > 1e-9 {
			t.Errorf("torque[%d] = %v, want %v", i, gotTq[i], tq[i])
		}
		if math.Abs(gotDC[i]-dc[i]) > 1e-5*dc[i] {
			t.Errorf("injector DC[%d] = %v, want %v", i, gotDC[i], dc[i])
		}
		if want := tq[i] * rpm[i] / 7024; math.Abs(gotP[i]-want) > 1e-9 {
			t.Errorf("power[%d] = %v, want %v", i, gotP[i], want)
		}
	}
}

// TestT5MatchesSuite compares the T5 dyno curves (torque from the WOT boost
// request per turbo/MAP sensor, injector DC from the firmware injection
// math) with T5Suite's frmDynoChart for byte-wide temperature tables.
func TestT5MatchesSuite(t *testing.T) {
	rpm := []float64{1000, 2500, 4000, 5500}
	tests := []struct {
		name   string
		opts   map[string]float64
		tq, dc []float64
	}{
		{"stock, 2.5 bar", map[string]float64{},
			[]float64{225, 270, 300, 255}, []float64{9.84083366394043, 31.056575775146484, 57.286666870117188, 66.36895751953125}},
		{"GT17, 3.0 bar", map[string]float64{"turbo": 1, "mapsensor": 1},
			[]float64{340, 401, 460, 374}, []float64{11.8920316696167, 39.493099212646484, 72.749580383300781, 78.941612243652344}},
		{"HX40w, 5.0 bar, automatic", map[string]float64{"turbo": 8, "mapsensor": 4, "automatic": 1},
			[]float64{580, 670, 670, 620}, []float64{18.117395401000977, 54.2286491394043, 96.154586791992188, 119.29937744140625}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := t5{}.Calculate(t5Syms(t).fw(), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			t5Check(t, res, rpm, tt.tq, tt.dc)
		})
	}
}

// t5TempTablesAsLoaded retypes Luft_kompfak!/Lufttemp_steg! the way
// ecusymbol's T5 loader types them: no CHAR flag, so Ints() pairs the bytes
// into 16-bit words.
func t5TempTablesAsLoaded(t *testing.T, m syms) syms {
	t.Helper()
	for _, n := range []string{"Luft_kompfak!", "Lufttemp_steg!"} {
		old := m[n]
		s := &symbol.Symbol{Name: n, Length: old.Length}
		if err := s.SetData(old.Bytes()); err != nil {
			t.Fatal(err)
		}
		m[n] = s
	}
	return m
}

// TestT5InjectorDCWithLoaderTypes: T5Suite reads Luft_kompfak! and
// Lufttemp_steg! as byte tables. Loaded as ecusymbol types them, the
// injector DC must still follow the suite instead of pinning to the
// 32500-tick clamp (DC = rpm * 130 / 1200).
func TestT5InjectorDCWithLoaderTypes(t *testing.T) {
	res, err := t5{}.Calculate(t5TempTablesAsLoaded(t, t5Syms(t)).fw(), map[string]float64{})
	if err != nil {
		t.Fatal(err)
	}
	t5Check(t, res, []float64{1000, 2500, 4000, 5500}, []float64{225, 270, 300, 255},
		[]float64{9.84083366394043, 31.056575775146484, 57.286666870117188, 66.36895751953125})
}

// loadTestBin loads one ecusymbol test binary from the sibling ecusymbol
// checkout (the go.work layout), skipping when it is not present.
func loadTestBin(t *testing.T, rel string) symbol.FirmwareFile {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "ecusymbol", "testdata", filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("test binary not available: %v", err)
	}
	_, fw, err := symbol.Load(path, data, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	return fw
}

// TestT7RealBinaryMatchesSuite runs a stock T7 binary as ecusymbol decodes
// it and compares the WOT row with T7Suite over the same bytes.
func TestT7RealBinaryMatchesSuite(t *testing.T) {
	fw := loadTestBin(t, "T7Binaries/5168646.bin")
	opts := map[string]float64{}
	for _, o := range (t7{}).Options(fw) {
		opts[o.Key] = o.Default
	}
	res, err := t7{}.Calculate(fw, opts)
	if err != nil {
		t.Fatal(err)
	}
	lim := make([]string, 16)
	for i := range lim {
		lim[i] = limTorqueEngine
	}
	checkOracle(t, res, oracleWant{
		air:  []int{480, 480, 557, 704, 752, 759, 749, 743, 742, 720, 692, 680, 666, 643, 624, 611},
		lim:  lim,
		tq:   []int{155, 155, 180, 227, 243, 245, 242, 240, 239, 232, 223, 219, 213, 197, 189, 168},
		dc:   []int{7, 8, 14, 23, 32, 38, 43, 50, 58, 62, 65, 71, 78, 83, 85, 86},
		lam:  []int{97, 98, 98, 94, 90, 92, 92, 89, 87, 87, 88, 86, 82, 80, 82, 82},
		flow: []int{4, 5, 8, 13, 19, 22, 25, 29, 33, 36, 37, 41, 45, 48, 49, 49},
	}, false)
}

// TestT5RealBinaryMatchesSuite runs a stock T5.5 binary as ecusymbol
// decodes it; torque and injector DC must match T5Suite.
func TestT5RealBinaryMatchesSuite(t *testing.T) {
	fw := loadTestBin(t, "T5Binaries/4300331.BIN")
	res, err := t5{}.Calculate(fw, map[string]float64{})
	if err != nil {
		t.Fatal(err)
	}
	rpm := []float64{1000, 1500, 1750, 2000, 2250, 2500, 2750, 3000, 3250, 3500, 4000, 4500, 5000, 5500, 6000, 6500}
	tq := []float64{135, 240, 240, 246, 252, 246, 243, 238.5, 232.5, 223.5, 228, 228, 231, 213, 197.5, 197.5}
	if !slices.Equal(res.RPM, rpm) || !slices.Equal(res.Curves[1].Values, tq) {
		t.Fatalf("rpm/torque = %v / %v, want %v / %v", res.RPM, res.Curves[1].Values, rpm, tq)
	}
	dc := []float64{7.0956640243530273, 16.395664215087891, 19.285501480102539, 23.294584274291992,
		27.126914978027344, 30.0390625, 32.037284851074219, 34.469219207763672, 37.023002624511719,
		38.993099212646484, 48.826824188232422, 56.082244873046875, 67.628120422363281, 69.5212173461914,
		77.519378662109375, 86.442214965820313}
	t5Check(t, res, rpm, tq, dc)
}
