package logfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// Open picks the parser from the extension, case-insensitively, and rejects
// extensions it does not know.
func TestOpenDispatch(t *testing.T) {
	txl := "18-07-2025 14:21:20.196|A=1|\n"
	csv := "Time,A\n\"2025-07-18 14:21:20,196\",1\n"
	for _, tc := range []struct {
		name, data string
		ok         bool
	}{
		{"log.T7L", txl, true},
		{"dir.v2/log.t5l", txl, true},
		{"log.t8l", txl, true},
		{"LOG.CSV", csv, true},
		{"log.bpl", string(bplFile([]string{"A"}, bplRow{t0, []float32{1}})), true},
		{"log.txt", txl, false},
		{"log", txl, false},
	} {
		lf, err := Open(tc.name, strings.NewReader(tc.data))
		if (err == nil) != tc.ok {
			t.Errorf("Open(%q) err = %v, want ok=%v", tc.name, err, tc.ok)
			continue
		}
		if tc.ok && lf.Len() != 1 {
			t.Errorf("Open(%q) Len = %d, want 1", tc.name, lf.Len())
		}
	}
	if _, err := Open("x.log", strings.NewReader("")); err == nil || err.Error() != "Unsupported filetype" {
		t.Errorf("unsupported extension err = %v", err)
	}
}

// Each supported t5l/t7l/t8l date layout is recognised from the first line and
// parsed to the same instant.
func TestTXLTimeFormats(t *testing.T) {
	want := time.Date(2025, 7, 18, 14, 21, 20, 196e6, time.UTC)
	for _, stamp := range []string{
		"18/07/2025 14:21:20.196",
		"2025/07/18 14:21:20.196",
		"18-07-2025 14:21:20.196",
		"2025-07-18 14:21:20.196",
		"18.07.2025 14:21:20.196",
	} {
		lf, err := NewFromTxLogfile(strings.NewReader(stamp + "|A=1|\n"))
		if err != nil {
			t.Errorf("%s: %v", stamp, err)
			continue
		}
		if got := lf.Start(); !got.Equal(want) {
			t.Errorf("%s: parsed as %v, want %v", stamp, got, want)
		}
	}
}

// The time layout comes from the first line: a file whose first line has no
// recognisable time is refused, and an empty file is an error.
func TestTXLFirstLineDecides(t *testing.T) {
	if _, err := NewFromTxLogfile(strings.NewReader("hello|A=1|\n18-07-2025 14:21:20.196|A=1|\n")); err == nil {
		t.Error("unrecognised first line accepted")
	}
	if _, err := NewFromTxLogfile(strings.NewReader("")); err == nil || err.Error() != "no lines in file" {
		t.Errorf("empty file err = %v", err)
	}

	// A later line in a different layout is skipped, not misread.
	lf, err := NewFromTxLogfile(strings.NewReader(strings.Join([]string{
		"18-07-2025 14:21:20.100|A=1|",
		"2025/07/18 14:21:20.200|A=2|",
		"18-07-2025 14:21:20.300|A=3|",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 2 {
		t.Fatalf("Len = %d, want 2", lf.Len())
	}
	if v, _ := lf.RecordAt(1).Value("A"); v != 3 {
		t.Errorf("A[1] = %v, want 3", v)
	}
}

// Values use a comma decimal separator, may be negative, and a field without
// "=" or with an empty value is ignored; IMPORTANTLINE is never a column.
func TestTXLValues(t *testing.T) {
	lf, err := NewFromTxLogfile(strings.NewReader(
		"18-07-2025 14:21:20.196|Out.fi_Ignition=-7,5|In.v_Vehicle=88|junk|Empty=|IMPORTANTLINE=1|Lambda=1,02\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := lf.RecordAt(0)
	for name, want := range map[string]float64{"Out.fi_Ignition": -7.5, "In.v_Vehicle": 88, "Lambda": 1.02} {
		if v, ok := r.Value(name); !ok || v != want {
			t.Errorf("%s = %v %v, want %v", name, v, ok, want)
		}
	}
	for _, c := range lf.Columns() {
		if c == "IMPORTANTLINE" || c == "junk" || c == "Empty" {
			t.Errorf("unexpected column %q in %v", c, lf.Columns())
		}
	}
}

// The CSV reader takes column names from the header and the time from the
// first field, keeps a row whose values partly fail to parse, and ignores
// fields beyond the header.
func TestCSVParse(t *testing.T) {
	// The ISONICO time contains a comma, so encoding/csv quotes it when
	// txlogger writes the log.
	data := strings.Join([]string{
		"Time,rpm,boost",
		`"2025-07-18 14:21:20,100",850,0.25`,
		`"2025-07-18 14:21:20,200",x,0.50,99`,
		`"2025-07-18 14:21:21,900"`,
	}, "\n")

	lf, err := NewFromCSVLogfile(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 3 || strings.Join(lf.Columns(), ",") != "rpm,boost" {
		t.Fatalf("Len %d Columns %v", lf.Len(), lf.Columns())
	}
	if v, ok := lf.RecordAt(0).Value("rpm"); !ok || v != 850 {
		t.Errorf("rpm[0] = %v %v", v, ok)
	}
	if _, ok := lf.RecordAt(1).Value("rpm"); ok {
		t.Error("rpm[1] should be missing, its value does not parse")
	}
	if v, ok := lf.RecordAt(1).Value("boost"); !ok || v != 0.5 {
		t.Errorf("boost[1] = %v %v", v, ok)
	}
	if _, ok := lf.RecordAt(2).Value("boost"); ok {
		t.Error("boost[2] should be missing, the row is short")
	}
	if got := lf.RecordAt(0).DelayTillNext; got != 100 {
		t.Errorf("DelayTillNext[0] = %d, want 100", got)
	}
	if got := lf.End().Sub(lf.Start()); got != 1800*time.Millisecond {
		t.Errorf("End-Start = %v, want 1.8s", got)
	}
}

// Rows whose time does not parse are skipped; a log with no valid rows, or
// with no header at all, is an error.
func TestCSVBadRows(t *testing.T) {
	lf, err := NewFromCSVLogfile(strings.NewReader("Time,A\nnot a time,1\n\n\"2025-07-18 14:21:20,100\",2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 1 {
		t.Errorf("Len = %d, want 1", lf.Len())
	}

	if _, err := NewFromCSVLogfile(strings.NewReader("Time,A\nnot a time,1\n")); err == nil {
		t.Error("log with no valid rows accepted")
	}
	if _, err := NewFromCSVLogfile(strings.NewReader("")); err == nil {
		t.Error("empty CSV accepted")
	}
}

type bplRow struct {
	ts   time.Time
	vals []float32
}

// bplFile encodes a BPL log the way datalogger.BPLWriter does.
func bplFile(names []string, rows ...bplRow) []byte {
	var b bytes.Buffer
	b.WriteString(bplMagic)
	b.WriteByte(bplVersion)
	binary.Write(&b, binary.LittleEndian, uint16(len(names)))
	for _, n := range names {
		binary.Write(&b, binary.LittleEndian, uint16(len(n)))
		b.WriteString(n)
	}
	for _, r := range rows {
		binary.Write(&b, binary.LittleEndian, r.ts.UnixNano())
		for _, v := range r.vals {
			binary.Write(&b, binary.LittleEndian, math.Float32bits(v))
		}
	}
	return b.Bytes()
}

// The BPL reader restores column names, nanosecond timestamps and float32
// values, and drops a partially written trailing record.
func TestBPLParse(t *testing.T) {
	data := bplFile([]string{"rpm", "λ"},
		bplRow{t0.Add(123456789), []float32{850, 0.98}},
		bplRow{t0.Add(time.Second), []float32{900, 1.01}},
	)
	data = append(data, 1, 2, 3, 4, 5) // torn write

	lf, err := NewFromBPLLogfile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 2 || strings.Join(lf.Columns(), ",") != "rpm,λ" {
		t.Fatalf("Len %d Columns %v", lf.Len(), lf.Columns())
	}
	if !lf.Start().Equal(t0.Add(123456789)) {
		t.Errorf("Start = %v", lf.Start())
	}
	if v, _ := lf.RecordAt(1).Value("λ"); float32(v) != float32(1.01) {
		t.Errorf("λ[1] = %v", v)
	}
	if v, _ := lf.RecordAt(0).Value("rpm"); v != 850 {
		t.Errorf("rpm[0] = %v", v)
	}
}

// A BPL header with no records opens as an empty log.
func TestBPLHeaderOnly(t *testing.T) {
	lf, err := NewFromBPLLogfile(bytes.NewReader(bplFile([]string{"a", "b"})))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 0 || len(lf.Columns()) != 2 {
		t.Errorf("Len %d Columns %v", lf.Len(), lf.Columns())
	}
}

type failingReader struct{ data []byte }

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, errors.New("disk on fire")
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// A damaged BPL header is an error naming the part that could not be read,
// and a read failure inside the records is reported rather than treated as
// the end of the file.
func TestBPLErrors(t *testing.T) {
	full := bplFile([]string{"rpm"})
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "magic"},
		{"short magic", []byte("BP"), "magic"},
		{"bad magic", []byte("CSV\x01"), "not a BPL"},
		{"no version", []byte("BPL"), "version"},
		{"bad version", []byte("BPL\x02\x00\x00"), "unsupported BPL version 2"},
		{"no column count", full[:4], "column count"},
		{"no name length", full[:6], "name length"},
		{"short name", full[:9], "column name"},
	} {
		_, err := NewFromBPLLogfile(bytes.NewReader(tc.data))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", tc.name, err, tc.want)
		}
	}

	_, err := NewFromBPLLogfile(&failingReader{data: full})
	if err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("record read failure err = %v", err)
	}
}
