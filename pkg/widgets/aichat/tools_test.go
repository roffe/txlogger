package aichat

import (
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

// testWidget wires the tools against an in-memory binary and log without
// building any UI (the tool methods only touch cfg).
func testWidget(t *testing.T, lf logfile.Logfile) *Widget {
	t.Helper()
	fw := symbol.NewCollection(
		newSym(t, "BFuelCal.AirXSP", 20, 100),
		newSym(t, "BFuelCal.RpmYSP", 1000, 4000),
		newSym(t, "BFuelCal.Map", 100, 110, 120, 130),
		newSym(t, "Bosch.Nope", 7),
	)
	return &Widget{cfg: &Config{
		FW:      func() symbol.SymbolCollection { return fw },
		ECU:     func() string { return "T7" },
		Log:     func() logfile.Logfile { return lf },
		BinName: func() string { return "test.bin" },
		LogName: func() string { return "test.t7l" },
		Client:  func() *ollama.Client { return nil },
	}}
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
		FW:      func() symbol.SymbolCollection { return fw },
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
		FW:  func() symbol.SymbolCollection { return nil },
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

// fakeLog is a minimal logfile.Logfile; only the read-only accessors the tools
// use are meaningful.
type fakeLog struct{ recs []logfile.Record }

func (f *fakeLog) Get() logfile.Record           { return f.recs[0] }
func (f *fakeLog) Next() logfile.Record          { return f.recs[0] }
func (f *fakeLog) Prev() logfile.Record          { return f.recs[0] }
func (f *fakeLog) Seek(int)                      {}
func (f *fakeLog) Pos() int                      { return 0 }
func (f *fakeLog) Len() int                      { return len(f.recs) }
func (f *fakeLog) RecordAt(i int) logfile.Record { return f.recs[i] }
func (f *fakeLog) Start() time.Time              { return f.recs[0].Time }
func (f *fakeLog) End() time.Time                { return f.recs[len(f.recs)-1].Time }
func (f *fakeLog) Close()                        {}

func newFakeLog(rpm ...float64) *fakeLog {
	t0 := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	f := &fakeLog{}
	for i, v := range rpm {
		rec := logfile.NewRecord(t0.Add(time.Duration(i) * 100 * time.Millisecond))
		rec.SetValue("ActualIn.n_Engine", v)
		f.recs = append(f.recs, rec)
	}
	return f
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
