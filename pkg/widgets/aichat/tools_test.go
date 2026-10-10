package aichat

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/ollama"
)

// newSym builds an unsigned 16-bit symbol from raw values.
func newSym(t *testing.T, name string, vals ...uint16) *symbol.Symbol {
	t.Helper()
	s := &symbol.Symbol{
		Name:             name,
		Length:           uint16(len(vals) * 2),
		Correctionfactor: 1,
	}
	data := make([]byte, 0, len(vals)*2)
	for _, v := range vals {
		data = append(data, byte(v>>8), byte(v))
	}
	if err := s.SetData(data); err != nil {
		t.Fatalf("SetData(%s): %v", name, err)
	}
	return s
}

// testConfig serves an in-memory T7 binary (a 2x2 BFuelCal.Map with its axes
// plus one scalar) and the given log.
func testConfig(t *testing.T, lf logfile.Logfile) *Config {
	t.Helper()
	fw := symbol.NewCollection(
		newSym(t, "BFuelCal.AirXSP", 20, 100),
		newSym(t, "BFuelCal.RpmYSP", 1000, 4000),
		newSym(t, "BFuelCal.Map", 100, 110, 120, 130),
		newSym(t, "Bosch.Nope", 7),
	)
	return &Config{
		FW:      func() symbol.FirmwareFile { return fw },
		ECU:     func() string { return "T7" },
		Log:     func() logfile.Logfile { return lf },
		BinName: func() string { return "test.bin" },
		LogName: func() string { return "test.t7l" },
		Client:  func() *ollama.Client { return nil },
	}
}

// testWidget wires the tools against an in-memory binary and log without
// building any UI (the tool methods only touch cfg).
func testWidget(t *testing.T, lf logfile.Logfile) *Widget {
	t.Helper()
	return &Widget{cfg: testConfig(t, lf)}
}

func TestReadSymbolMapGrid(t *testing.T) {
	got := testWidget(t, nil).readSymbol("BFuelCal.Map")

	for _, want := range []string{
		"columns = BFuelCal.AirXSP [mg/c]",
		"rows = BFuelCal.RpmYSP [rpm]",
		"y\\x,[0] 20,[1] 100",
		"[0] 1000,100,110",
		"[1] 4000,120,130",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestReadSymbolReportsScale(t *testing.T) {
	fw := symbol.NewCollection(
		newSym(t, "Out.X_AccPedal", 1000),
	)
	fw.Symbols()[0].Correctionfactor = 0.1

	w := &Widget{cfg: &Config{
		FW:      func() symbol.FirmwareFile { return fw },
		ECU:     func() string { return "T7" },
		Log:     func() logfile.Logfile { return nil },
		BinName: func() string { return "test.bin" },
		LogName: func() string { return "test.t7l" },
		Client:  func() *ollama.Client { return nil },
	}}

	got := w.readSymbol("Out.X_AccPedal")
	if !strings.Contains(got, "scale = 0.1") {
		t.Fatalf("readSymbol output = %q, want scale info", got)
	}
}

func TestReadSymbolScalarAndUnknown(t *testing.T) {
	w := testWidget(t, nil)

	if got := w.readSymbol("Bosch.Nope"); !strings.Contains(got, "7") {
		t.Errorf("scalar read = %q, want the value 7", got)
	}

	// A bad name must come back as a recoverable hint, not a bare failure, or
	// the model has nothing to correct itself with.
	got := w.readSymbol("BFuelCal.Mpa")
	if !strings.Contains(got, "Did you mean") || !strings.Contains(got, "BFuelCal.Map") {
		t.Errorf("unknown symbol = %q, want a suggestion naming BFuelCal.Map", got)
	}
}

func TestListSymbolsFilter(t *testing.T) {
	got := testWidget(t, nil).listSymbols("bfuelcal")
	if strings.Contains(got, "Bosch.Nope") {
		t.Errorf("filter leaked a non-match:\n%s", got)
	}
	if n := strings.Count(got, "BFuelCal."); n != 3 {
		t.Errorf("got %d BFuelCal rows, want 3:\n%s", n, got)
	}
}

func TestNoBinaryLoaded(t *testing.T) {
	w := &Widget{cfg: &Config{
		FW:  func() symbol.FirmwareFile { return nil },
		ECU: func() string { return "T7" },
		Log: func() logfile.Logfile { return nil },
	}}
	if got := w.readSymbol("Whatever"); !strings.Contains(got, "no binary") {
		t.Errorf("read with no bin = %q, want a 'no binary' message", got)
	}
	if got := w.logInfo(); !strings.Contains(got, "no log file") {
		t.Errorf("logInfo with no log = %q, want a 'no log file' message", got)
	}
}

func newFakeLog(rpm ...float64) logfile.Logfile {
	t0 := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	times := make([]time.Time, len(rpm))
	rows := make([]map[string]float64, len(rpm))
	for i, v := range rpm {
		times[i] = t0.Add(time.Duration(i) * 100 * time.Millisecond)
		rows[i] = map[string]float64{"ActualIn.n_Engine": v}
	}
	return logfile.FromRows(times, rows)
}

func TestLogInfoStats(t *testing.T) {
	got := testWidget(t, newFakeLog(1000, 3000, 2000)).logInfo()

	if !strings.Contains(got, "samples: 3 (indices 0-2)") {
		t.Errorf("missing sample count:\n%s", got)
	}
	// min,max,average over 1000/3000/2000
	if !strings.Contains(got, "ActualIn.n_Engine,1000,3000,2000") {
		t.Errorf("wrong stats:\n%s", got)
	}
}

func TestLogSamplesStride(t *testing.T) {
	w := testWidget(t, newFakeLog(1000, 2000, 3000, 4000, 5000))

	got := w.logSamples("ActualIn.n_Engine", 0, 10, 2)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 4 { // header + indices 0,2,4
		t.Fatalf("stride 2 gave %d lines, want 4:\n%s", len(lines), got)
	}
	if lines[1] != "0,0.00,1000" {
		t.Errorf("first row = %q, want %q", lines[1], "0,0.00,1000")
	}
	if lines[3] != "4,0.40,5000" {
		t.Errorf("last row = %q, want %q", lines[3], "4,0.40,5000")
	}

	// Past the end must say so rather than returning an empty table the model
	// would read as "no data in the log".
	if got := w.logSamples("", 99, 10, 1); !strings.Contains(got, "past the end") {
		t.Errorf("out-of-range start = %q", got)
	}
}

func TestArgCoercion(t *testing.T) {
	// Small local models routinely send numbers as strings and lists as arrays.
	args := map[string]any{
		"count":    "25",
		"stride":   float64(3),
		"channels": []any{"a", "b"},
	}
	if got := argInt(args, "count", 50); got != 25 {
		t.Errorf("argInt string = %d, want 25", got)
	}
	if got := argInt(args, "stride", 1); got != 3 {
		t.Errorf("argInt float = %d, want 3", got)
	}
	if got := argInt(args, "missing", 7); got != 7 {
		t.Errorf("argInt default = %d, want 7", got)
	}
	if got := argStr(args, "channels"); got != "a,b" {
		t.Errorf("argStr slice = %q, want \"a,b\"", got)
	}
}

func TestShowMapDiff(t *testing.T) {
	w := testWidget(t, nil)
	var got []float64
	w.cfg.ShowMapDiff = func(_ string, proposed []float64) error {
		got = proposed
		return nil
	}

	// BFuelCal.Map is 2x2; junk, out-of-range cells and empty edits must come
	// back as tool errors the model can correct, without opening a window.
	for _, tc := range []struct{ changes, want string }{
		{"1,1", "not a cell edit"},
		{"1,1=x", "not a number"},
		{"1,2=5", "outside the map"},
		{"a,b=5", "not a cell index"},
		{"", "no cell edits"},
	} {
		if out := w.showMapDiff("BFuelCal.Map", tc.changes); !strings.Contains(out, tc.want) {
			t.Errorf("showMapDiff(%q) = %q, want %q", tc.changes, out, tc.want)
		}
	}
	if out := w.showMapDiff("Nope.Map", "0,0=1"); !strings.Contains(out, "no symbol named") {
		t.Errorf("unknown map = %q", out)
	}
	if got != nil {
		t.Fatalf("invalid proposals reached the UI: %v", got)
	}

	// Unlisted cells keep their current value; row,col is row-major.
	if out := w.showMapDiff("BFuelCal.Map", "1,0=125; 0,1=111"); !strings.Contains(out, "2 cells changed") {
		t.Errorf("valid proposal = %q", out)
	}
	if want := []float64{100, 111, 125, 130}; !slices.Equal(got, want) {
		t.Errorf("proposed = %v, want %v", got, want)
	}
}

// rowsLog builds a log with one record per row, 100ms apart.
func rowsLog(rows ...map[string]float64) logfile.Logfile {
	t0 := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	times := make([]time.Time, len(rows))
	for i := range rows {
		times[i] = t0.Add(time.Duration(i) * 100 * time.Millisecond)
	}
	return logfile.FromRows(times, rows)
}

// rpmLog builds an n-sample log of ActualIn.n_Engine = 0, 1, 2, ...
func rpmLog(n int) logfile.Logfile {
	rpm := make([]float64, n)
	for i := range rpm {
		rpm[i] = float64(i)
	}
	return newFakeLog(rpm...)
}

// The tool schemas offered to the model name the arguments runTool reads and
// mark the ones the tools cannot do without as required.
func TestToolSchemas(t *testing.T) {
	want := map[string]struct{ props, required []string }{
		"list_symbols":  {[]string{"filter"}, []string{}},
		"read_symbol":   {[]string{"name"}, []string{"name"}},
		"log_info":      {[]string{}, []string{}},
		"log_samples":   {[]string{"channels", "count", "start", "stride"}, []string{}},
		"show_map_diff": {[]string{"changes", "name"}, []string{"name", "changes"}},
	}
	tools := (&Widget{}).tools()
	if len(tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(tools), len(want))
	}
	for _, tl := range tools {
		w, ok := want[tl.Function.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tl.Function.Name)
			continue
		}
		params, _ := tl.Function.Parameters.(map[string]any)
		props, _ := params["properties"].(map[string]any)
		required, _ := params["required"].([]string)
		if tl.Type != "function" || params["type"] != "object" || tl.Function.Description == "" {
			t.Errorf("%s: type %q, params type %v, description %q", tl.Function.Name, tl.Type, params["type"], tl.Function.Description)
		}
		if got := sortedKeys(props); !slices.Equal(got, w.props) {
			t.Errorf("%s properties = %v, want %v", tl.Function.Name, got, w.props)
		}
		if !slices.Equal(required, w.required) {
			t.Errorf("%s required = %v, want %v", tl.Function.Name, required, w.required)
		}
	}
}

// runTool routes each tool name to its implementation with the arguments
// coerced from whatever JSON types the model sent, and fills in the
// documented defaults (start 0, count 50, stride 1) for missing ones.
func TestRunToolDispatch(t *testing.T) {
	w := testWidget(t, newFakeLog(1000, 2000, 3000, 4000))
	var diffName string
	var diff []float64
	w.cfg.ShowMapDiff = func(name string, proposed []float64) error {
		diffName, diff = name, proposed
		return nil
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"list_symbols", map[string]any{"filter": "bosch"}, "name,bytes,unit,scale\nBosch.Nope,2,,1\n"},
		{"read_symbol", map[string]any{"name": "Bosch.Nope"}, "Bosch.Nope (1 values)\nscale = 1 (raw ECU counts, unscaled)\n7\n"},
		{"log_info", nil, "log: test.t7l\nsamples: 4 (indices 0-3)\nduration: 300ms\n\nchannel,min,max,average\nActualIn.n_Engine,1000,4000,2500\n"},
		{"log_samples", map[string]any{"channels": []any{"ActualIn.n_Engine"}, "start": "1", "count": float64(2), "stride": 2},
			"index,t_s,ActualIn.n_Engine\n1,0.10,2000\n3,0.30,4000\n"},
		{"log_samples", nil, "index,t_s,ActualIn.n_Engine\n0,0.00,1000\n1,0.10,2000\n2,0.20,3000\n3,0.30,4000\n"},
		{"show_map_diff", map[string]any{"name": "BFuelCal.Map", "changes": "0,0=1"},
			"Opened a Before/After/Diff view of BFuelCal.Map for the user, with 1 cells changed. Now summarise what you changed and why."},
		{"frobnicate", nil, "Error: unknown tool frobnicate"},
	} {
		if got := w.runTool(tc.name, tc.args); got != tc.want {
			t.Errorf("runTool(%s, %v) =\n%q\nwant\n%q", tc.name, tc.args, got, tc.want)
		}
	}
	if diffName != "BFuelCal.Map" || !slices.Equal(diff, []float64{1, 110, 120, 130}) {
		t.Errorf("show_map_diff opened %q with %v, want BFuelCal.Map with [1 110 120 130]", diffName, diff)
	}
}

// list_symbols prints one name,bytes,unit,scale row per match, says when
// nothing matched, and caps a long listing with a note.
func TestListSymbolsRowsAndLimits(t *testing.T) {
	w := testWidget(t, nil)
	sym := w.cfg.FW().GetByName("BFuelCal.Map")
	sym.Unit = "mg/c"
	sym.Correctionfactor = 0.0009765625
	if got, want := w.listSymbols("BFuelCal.MAP"), "name,bytes,unit,scale\nBFuelCal.Map,8,mg/c,0.0009765625\n"; got != want {
		t.Errorf("listSymbols = %q, want %q", got, want)
	}
	if got, want := w.listSymbols("ZZZ"), `No symbol name contains "zzz" (binary has 4 symbols). Try a shorter filter.`; got != want {
		t.Errorf("no match = %q, want %q", got, want)
	}

	syms := make([]*symbol.Symbol, 0, maxSymbolRows+10)
	for i := range maxSymbolRows + 10 {
		syms = append(syms, newSym(t, fmt.Sprintf("Sym.%03d", i), 1))
	}
	fw := symbol.NewCollection(syms...)
	w.cfg.FW = func() symbol.FirmwareFile { return fw }
	got := w.listSymbols("")
	lines := strings.Split(strings.TrimSpace(got), "\n")
	// header + maxSymbolRows rows + blank + note
	if n := strings.Count(got, "Sym."); n != maxSymbolRows {
		t.Errorf("listed %d symbols, want %d", n, maxSymbolRows)
	}
	if note := lines[len(lines)-1]; note != fmt.Sprintf("(%d matches, showing first %d — narrow the filter)", maxSymbolRows+10, maxSymbolRows) {
		t.Errorf("truncation note = %q", note)
	}

	w.cfg.FW = func() symbol.FirmwareFile { return nil }
	if got := w.listSymbols(""); !strings.HasPrefix(got, "Error: no binary") {
		t.Errorf("no binary = %q", got)
	}
}

// A scaled map prints the scale step, uses the unit fallback from the axis
// table and formats cells with the precision the scale implies.
func TestReadSymbolScaledGrid(t *testing.T) {
	fw := symbol.NewCollection(
		newSym(t, "BFuelCal.AirXSP", 20, 100),
		newSym(t, "BFuelCal.RpmYSP", 1000, 4000),
		newSym(t, "BFuelCal.Map", 100, 110, 125, 133),
	)
	fw.GetByName("BFuelCal.Map").Correctionfactor = 0.01
	w := testWidget(t, nil)
	w.cfg.FW = func() symbol.FirmwareFile { return fw }

	want := "BFuelCal.Map [Fuel correction factor] (4 values)\n" +
		"scale = 0.01 (values below are already scaled, smallest step 0.01)\n" +
		"columns = BFuelCal.AirXSP [mg/c]\n" +
		"rows = BFuelCal.RpmYSP [rpm]\n" +
		"\ny\\x,[0] 20,[1] 100\n" +
		"[0] 1000,1,1.10\n" +
		"[1] 4000,1.25,1.33\n"
	if got := w.readSymbol(" BFuelCal.Map "); got != want {
		t.Errorf("readSymbol =\n%s\nwant\n%s", got, want)
	}
}

// Without axes that multiply out to the map size, a map prints as a flat list
// rather than a grid labelled with the wrong axes; the symbol's own unit wins
// over the axis table's description.
func TestReadSymbolFlatFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		syms []*symbol.Symbol
	}{
		{"size mismatch", []*symbol.Symbol{
			newSym(t, "BFuelCal.AirXSP", 20, 100),
			newSym(t, "BFuelCal.RpmYSP", 1000, 4000),
			newSym(t, "BFuelCal.Map", 100, 110, 120),
		}},
		{"no x axis", []*symbol.Symbol{
			newSym(t, "BFuelCal.RpmYSP", 1000, 4000),
			newSym(t, "BFuelCal.Map", 100, 110, 120),
		}},
		{"no y axis", []*symbol.Symbol{
			newSym(t, "BFuelCal.AirXSP", 20, 100),
			newSym(t, "BFuelCal.Map", 100, 110, 120),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fw := symbol.NewCollection(tc.syms...)
			fw.GetByName("BFuelCal.Map").Unit = "lambda"
			w := testWidget(t, nil)
			w.cfg.FW = func() symbol.FirmwareFile { return fw }
			want := "BFuelCal.Map [lambda] (3 values)\nscale = 1 (raw ECU counts, unscaled)\n100,110,120\n"
			if got := w.readSymbol("BFuelCal.Map"); got != want {
				t.Errorf("readSymbol = %q, want %q", got, want)
			}
		})
	}
}

// notFound suggests at most eight names sharing the part before the dot, and
// matches on the whole name when that part is too short to be useful.
func TestNotFoundSuggestions(t *testing.T) {
	syms := []*symbol.Symbol{newSym(t, "ab.cd", 1), newSym(t, "ab.xy", 1)}
	for i := range 10 {
		syms = append(syms, newSym(t, fmt.Sprintf("IgnNormCal.M%d", i), 1))
	}
	fw := symbol.NewCollection(syms...)

	got := notFound(fw, "IgnNormCal.Mapp")
	if n := strings.Count(got, "IgnNormCal.M"); n != 9 { // the name itself + 8 suggestions
		t.Errorf("notFound = %q, want eight suggestions", got)
	}
	for name, want := range map[string]string{
		"ab.c":  `Error: no symbol named "ab.c". Did you mean: ab.cd`,
		"Qq.Zz": `Error: no symbol named "Qq.Zz". Use list_symbols to find the right name.`,
	} {
		if got := notFound(fw, name); got != want {
			t.Errorf("notFound(%s) = %q, want %q", name, got, want)
		}
	}
}

// precOf uses the named symbol's scale and falls back to two decimals.
func TestPrecOf(t *testing.T) {
	fw := symbol.NewCollection(newSym(t, "X", 1))
	fw.GetByName("X").Correctionfactor = 0.1
	if got := precOf(fw, "X"); got != 1 {
		t.Errorf("precOf(X) = %d, want 1", got)
	}
	if got := precOf(fw, "missing"); got != 2 {
		t.Errorf("precOf(missing) = %d, want 2", got)
	}
}

// cellIndex turns "row,col" or a flat index into a row-major offset and
// rejects anything outside the map.
func TestCellIndex(t *testing.T) {
	for _, tc := range []struct {
		cell       string
		rows, cols int
		want       int
		err        string
	}{
		{"0,0", 2, 3, 0, ""},
		{" 1 , 2 ", 2, 3, 5, ""},
		{"4", 2, 3, 4, ""},
		{"6", 2, 3, 0, "outside the map (6 values)"},
		{"-1", 2, 3, 0, "outside the map (6 values)"},
		{"2,0", 2, 3, 0, "outside the map (rows 0-1, columns 0-2)"},
		{"0,-1", 2, 3, 0, "outside the map (rows 0-1, columns 0-2)"},
		{"1,2,3", 2, 3, 0, `not a cell index. Use "row,col".`},
		{"", 2, 3, 0, `not a cell index. Use "row,col".`},
		{"x", 2, 3, 0, "not a cell index. Use \"row,col\" with the numbers"},
	} {
		got, err := cellIndex(tc.cell, tc.rows, tc.cols)
		if tc.err == "" {
			if err != "" || got != tc.want {
				t.Errorf("cellIndex(%q) = %d, %q; want %d", tc.cell, got, err, tc.want)
			}
			continue
		}
		if !strings.HasPrefix(err, "Error: ") || !strings.Contains(err, tc.err) {
			t.Errorf("cellIndex(%q) error = %q, want %q", tc.cell, err, tc.err)
		}
	}
}

// show_map_diff hands the host the trimmed name and the full proposal, takes
// newline-separated edits and flat indices into list-shaped symbols, and turns
// a missing binary, a missing host hook or a host failure into tool errors.
func TestShowMapDiffHost(t *testing.T) {
	w := testWidget(t, nil)
	var gotName string
	var got []float64
	w.cfg.ShowMapDiff = func(name string, proposed []float64) error {
		gotName, got = name, proposed
		return nil
	}

	if out := w.showMapDiff(" BFuelCal.Map ", "0,0=101\n1,1=131\n"); !strings.Contains(out, "with 2 cells changed") {
		t.Errorf("newline edits = %q", out)
	}
	if gotName != "BFuelCal.Map" || !slices.Equal(got, []float64{101, 110, 120, 131}) {
		t.Errorf("host got %q %v", gotName, got)
	}
	if out := w.showMapDiff("Bosch.Nope", "0=9"); !strings.Contains(out, "1 cells changed") || !slices.Equal(got, []float64{9}) {
		t.Errorf("flat index edit = %q, proposed %v", out, got)
	}
	if out := w.showMapDiff("BFuelCal.Map", " ; ;"); out != `Error: no cell edits given for BFuelCal.Map (it is 2 rows x 2 columns). Send at least one "row,col=value".` {
		t.Errorf("blank edits = %q", out)
	}

	w.cfg.ShowMapDiff = func(string, []float64) error { return fmt.Errorf("window closed") }
	if out := w.showMapDiff("BFuelCal.Map", "0,0=1"); out != "Error: window closed" {
		t.Errorf("host failure = %q", out)
	}
	w.cfg.ShowMapDiff = nil
	if out := w.showMapDiff("BFuelCal.Map", "0,0=1"); out != "Error: this build cannot display map diffs." {
		t.Errorf("no host hook = %q", out)
	}
	w.cfg.FW = func() symbol.FirmwareFile { return nil }
	if out := w.showMapDiff("BFuelCal.Map", "0,0=1"); !strings.HasPrefix(out, "Error: no binary") {
		t.Errorf("no binary = %q", out)
	}
}

// log_samples with no channels lists every channel in name order, leaves
// missing values empty and clamps a negative start and a zero stride.
func TestLogSamplesAllChannels(t *testing.T) {
	w := testWidget(t, rowsLog(
		map[string]float64{"b": 1, "a": 1.234},
		map[string]float64{"a": 2},
		map[string]float64{"a": 3, "b": 3},
	))
	want := "index,t_s,a,b\n0,0.00,1.23,1\n1,0.10,2,\n2,0.20,3,3\n"
	if got := w.logSamples("", -5, 10, 0); got != want {
		t.Errorf("logSamples =\n%q\nwant\n%q", got, want)
	}
	if got, want := w.logSamples(" b , nope ", 1, 1, 1), "index,t_s,b,nope\n1,0.10,,\n"; got != want {
		t.Errorf("named channels = %q, want %q", got, want)
	}
}

// log_samples defaults a non-positive count to 50 rows and caps a large one at
// maxLogRows with a note.
func TestLogSamplesCount(t *testing.T) {
	w := testWidget(t, rpmLog(maxLogRows+50))

	if got := w.logSamples("ActualIn.n_Engine", 0, 0, 1); strings.Count(got, "\n") != 51 {
		t.Errorf("count 0 gave %d lines, want header + 50 rows", strings.Count(got, "\n"))
	}
	got := w.logSamples("ActualIn.n_Engine", 0, 10000, 1)
	if !strings.HasSuffix(got, fmt.Sprintf("\n(capped at %d rows — use stride to cover more ground)\n", maxLogRows)) {
		t.Errorf("large count tail = %q", got[max(0, len(got)-80):])
	}
	if !strings.Contains(got, fmt.Sprintf("\n%d,", maxLogRows-1)) || strings.Contains(got, fmt.Sprintf("\n%d,", maxLogRows)) {
		t.Errorf("large count did not stop at row %d", maxLogRows-1)
	}

	w.cfg.Log = func() logfile.Logfile { return nil }
	if got := w.logSamples("", 0, 1, 1); !strings.HasPrefix(got, "Error: no log file") {
		t.Errorf("no log = %q", got)
	}
}

// log_samples with no channels returns every channel in the log, including
// ones the first requested record happens to lack.
func TestLogSamplesAllChannelsFromLaterRecords(t *testing.T) {
	w := testWidget(t, rowsLog(
		map[string]float64{"a": 1},
		map[string]float64{"a": 2, "lambda": 0.98},
	))
	if got, want := w.logSamples("", 0, 10, 1), "index,t_s,a,lambda\n0,0.00,1,\n1,0.10,2,0.98\n"; got != want {
		t.Errorf("logSamples = %q, want %q", got, want)
	}
}

// log_info reports the duration and per-channel min/max/average in name order.
func TestLogInfoChannels(t *testing.T) {
	w := testWidget(t, rowsLog(
		map[string]float64{"rpm": 1000, "boost": 0.5},
		map[string]float64{"rpm": 2000},
		map[string]float64{"rpm": 4000, "boost": 1.25},
	))
	want := "log: test.t7l\nsamples: 3 (indices 0-2)\nduration: 200ms\n\n" +
		"channel,min,max,average\nboost,0.50,1.25,0.88\nrpm,1000,4000,2333.33\n"
	if got := w.logInfo(); got != want {
		t.Errorf("logInfo =\n%q\nwant\n%q", got, want)
	}
}

// fmtVal prints whole numbers without decimals and everything else with the
// requested precision.
func TestFmtVal(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		prec int
		want string
	}{
		{3, 2, "3"},
		{-2, 4, "-2"},
		{1.23456, 2, "1.23"},
		{1.5, 0, "2"},
		{1e16, 1, "10000000000000000.0"},
		{math.Inf(1), 2, "+Inf"},
		{math.NaN(), 2, "NaN"},
	} {
		if got := fmtVal(tc.v, tc.prec); got != tc.want {
			t.Errorf("fmtVal(%v, %d) = %q, want %q", tc.v, tc.prec, got, tc.want)
		}
	}
}

// argStr and argInt accept every JSON shape a model sends for a value and fall
// back to the default when it cannot be read as a number.
func TestArgCoercionShapes(t *testing.T) {
	args := map[string]any{
		"s":     "text",
		"f":     2.5,
		"whole": float64(3),
		"list":  []any{"a", 2.0},
		"bool":  true,
		"int":   7,
		"pad":   " 9 ",
		"word":  "nine",
	}
	for key, want := range map[string]string{
		"s": "text", "f": "2.5", "whole": "3", "list": "a,2", "bool": "true", "missing": "",
	} {
		if got := argStr(args, key); got != want {
			t.Errorf("argStr(%s) = %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]int{
		"f": 2, "int": 7, "pad": 9, "word": -1, "bool": -1, "missing": -1,
	} {
		if got := argInt(args, key, -1); got != want {
			t.Errorf("argInt(%s) = %d, want %d", key, got, want)
		}
	}
}

// A map whose axis table entry names only an X axis prints as a one-row grid
// with no rows line, and show_map_diff addresses it as row 0.
func TestReadSymbolSingleAxisGrid(t *testing.T) {
	// No shipped axis table has an X-only entry, so add one for the test.
	axes := symbol.GetAxisCollection(symbol.ECU_T7)
	axes["Test.Map"] = symbol.Axis{X: "Test.XSP", Z: "Test.Map", XDescription: "rpm", ZDescription: "ms"}
	t.Cleanup(func() { delete(axes, "Test.Map") })

	fw := symbol.NewCollection(
		newSym(t, "Test.XSP", 800, 1600, 3200),
		newSym(t, "Test.Map", 5, 6, 7),
	)
	w := testWidget(t, nil)
	w.cfg.FW = func() symbol.FirmwareFile { return fw }

	want := "Test.Map [ms] (3 values)\n" +
		"scale = 1 (raw ECU counts, unscaled)\n" +
		"columns = Test.XSP [rpm]\n" +
		"\ny\\x,[0] 800,[1] 1600,[2] 3200\n" +
		"[0] 0,5,6,7\n"
	if got := w.readSymbol("Test.Map"); got != want {
		t.Errorf("readSymbol =\n%q\nwant\n%q", got, want)
	}

	var got []float64
	w.cfg.ShowMapDiff = func(_ string, proposed []float64) error { got = proposed; return nil }
	if out := w.showMapDiff("Test.Map", "0,2=9"); !strings.Contains(out, "1 cells changed") || !slices.Equal(got, []float64{5, 6, 9}) {
		t.Errorf("showMapDiff(0,2) = %q, proposed %v; want [5 6 9]", out, got)
	}
	if out, want := w.showMapDiff("Test.Map", "1,0=9"), `Error: cell "1,0" is outside the map (rows 0-0, columns 0-2).`; out != want {
		t.Errorf("showMapDiff(1,0) = %q, want %q", out, want)
	}
}
