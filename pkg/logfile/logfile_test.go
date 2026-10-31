package logfile

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestOpenFormats(t *testing.T) {
	for name, data := range map[string][]byte{"x.t7l": synthTXL(), "x.csv": synthCSV(), "x.bpl": synthBPL()} {
		lf, err := Open(name, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if lf.Len() != benchRows || len(lf.Columns()) != benchCols {
			t.Fatalf("%s: %d rows %d cols, want %d %d", name, lf.Len(), len(lf.Columns()), benchRows, benchCols)
		}
		rec := lf.RecordAt(123)
		if v, ok := rec.Value(benchNames()[7]); !ok || float32(v) != float32(benchValue(123, 7)) {
			t.Errorf("%s: row 123 col 7 = %v %v, want %v", name, v, ok, benchValue(123, 7))
		}
		if rec.DelayTillNext != 50 {
			t.Errorf("%s: DelayTillNext = %d", name, rec.DelayTillNext)
		}
		if got := lf.Column(benchNames()[3])[9]; float32(got) != float32(benchValue(9, 3)) {
			t.Errorf("%s: Column = %v", name, got)
		}
	}
}

// A corrupt line is skipped and a column that comes and goes reads as missing
// where it is absent.
func TestTXLGaps(t *testing.T) {
	log := strings.Join([]string{
		"18-07-2025 14:21:20.100|A=1,5|IMPORTANTLINE=0|",
		"garbage",
		"18-07-2025 14:21:20.200|A=2|B=7|",
		"18-07-2025 14:21:20.300|B=x|A=3|",
	}, "\n")
	lf, err := Open("x.t7l", strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 3 {
		t.Fatalf("Len = %d, want 3", lf.Len())
	}
	if v, ok := lf.RecordAt(0).Value("A"); v != 1.5 || !ok {
		t.Errorf("A[0] = %v %v", v, ok)
	}
	if _, ok := lf.RecordAt(0).Value("B"); ok {
		t.Error("B[0] should be missing")
	}
	if _, ok := lf.RecordAt(2).Value("B"); ok {
		t.Error("B[2] should be missing, its value does not parse")
	}
	var keys []string
	for k := range lf.RecordAt(1).All() {
		keys = append(keys, k)
	}
	if strings.Join(keys, ",") != "A,B" {
		t.Errorf("All = %v", keys)
	}
	if got := lf.RecordAt(2).Time.Sub(lf.Start()); got != 200*time.Millisecond {
		t.Errorf("row 2 at %v", got)
	}
}
