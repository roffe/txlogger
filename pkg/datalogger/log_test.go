package datalogger

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roffe/txlogger/pkg/logfile"
)

// NewWriter picks the writer and file extension from the log format: CSV and
// BPL keep their names, TXL uses the ECU type (t5l/t7l/t8l). The file lands in
// LogPath named after the prefix with dots replaced.
func TestNewWriterFormats(t *testing.T) {
	tests := []struct {
		format, ecu, ext, writer string
	}{
		{"CSV", "T7", ".csv", "*datalogger.CSVWriter"},
		{"TXL", "T7", ".t7l", "*datalogger.TXWriter"},
		{"TXL", "T8", ".t8l", "*datalogger.TXWriter"},
		{"BPL", "T5", ".bpl", "*datalogger.BPLWriter"},
	}
	for _, tt := range tests {
		t.Run(tt.format+"_"+tt.ecu, func(t *testing.T) {
			dir := t.TempDir()
			name, lw, err := NewWriter(Config{LogFormat: tt.format, ECU: tt.ecu, LogPath: dir, FilenamePrefix: "my.bin"})
			if err != nil {
				t.Fatal(err)
			}
			defer lw.Close()
			if filepath.Dir(name) != dir || filepath.Ext(name) != tt.ext || !strings.HasPrefix(filepath.Base(name), "my_bin-") {
				t.Fatalf("filename %q", name)
			}
			if _, err := os.Stat(name); err != nil {
				t.Fatalf("log file not created: %v", err)
			}
			if got := fmt.Sprintf("%T", lw); got != tt.writer {
				t.Fatalf("writer %s, want %s", got, tt.writer)
			}
		})
	}
}

// A log file that cannot be created fails NewWriter for every format.
func TestNewWriterCreateError(t *testing.T) {
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"CSV", "TXL", "BPL"} {
		if _, lw, err := NewWriter(Config{LogFormat: format, ECU: "T7", LogPath: notDir, FilenamePrefix: "x"}); err == nil || lw != nil {
			t.Errorf("%s: writer %v err %v", format, lw, err)
		}
	}
}

// An unknown log format is an error and no file is created.
func TestNewWriterUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	name, lw, err := NewWriter(Config{LogFormat: "XML", LogPath: dir, FilenamePrefix: "x"})
	if err == nil || lw != nil || name != "unknown" {
		t.Fatalf("NewWriter = %q %v %v", name, lw, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files created: %v", entries)
	}
}

// createLog creates a missing log directory, but not its missing parents.
func TestCreateLogDirectory(t *testing.T) {
	base := t.TempDir()
	f, name, err := createLog(filepath.Join(base, "logs"), "p", "csv")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if filepath.Dir(name) != filepath.Join(base, "logs") {
		t.Fatalf("file %q not in the new dir", name)
	}

	if _, _, err := createLog(filepath.Join(base, "a", "b"), "p", "csv"); err == nil || !strings.Contains(err.Error(), "failed to create logs dir") {
		t.Fatalf("nested missing dir: %v", err)
	}

	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := createLog(file, "p", "csv"); err == nil || !strings.Contains(err.Error(), "failed to open file") {
		t.Fatalf("path is a file: %v", err)
	}
}

// A prefix carrying path separators or odd characters cannot escape the log
// directory: dots become underscores and the name is sanitized to its base.
func TestCreateLogSanitizesPrefix(t *testing.T) {
	dir := t.TempDir()
	f, name, err := createLog(dir, "../../evil name", "csv")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if filepath.Dir(name) != dir {
		t.Fatalf("log escaped the directory: %q", name)
	}
	if base := filepath.Base(name); !strings.HasPrefix(base, "evil_name-") || strings.ContainsAny(base, " /") {
		t.Fatalf("unsanitized name %q", base)
	}
}

func testChannels(vals map[string]*float64, names ...string) []Channel {
	out := make([]Channel, 0, len(names))
	for _, n := range names {
		p := vals[n]
		out = append(out, Channel{Name: n, read: func() float64 { return *p }, appendFmt: sysvarFormat(n)})
	}
	return out
}

// The CSV writer emits a "Time" header plus one column per channel once, then
// one row per Write with the ISONICO timestamp and each channel's formatting.
func TestCSVWriter(t *testing.T) {
	rpm, lambda := 900.0, 0.9876
	chans := testChannels(map[string]*float64{"Rpm": &rpm, EXTERNALWBLSYM: &lambda}, "Rpm", EXTERNALWBLSYM)
	path := filepath.Join(t.TempDir(), "x.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := NewCSVWriter(f)
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 123_000_000, time.UTC) // the CSV reader parses zone-less times as UTC
	if err := w.Write(t0, chans); err != nil {
		t.Fatal(err)
	}
	rpm, lambda = 950.5, 1
	if err := w.Write(t0.Add(40*time.Millisecond), chans); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	rows, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"Time", "Rpm", EXTERNALWBLSYM},
		{"2026-10-10 12:00:00,123", "900", "0.988"},
		{"2026-10-10 12:00:00,163", "950.50", "1"},
	}
	if !slices.EqualFunc(rows, want, slices.Equal) {
		t.Fatalf("rows\n%v\nwant\n%v", rows, want)
	}

	lf, err := logfile.NewFromCSVLogfile(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if lf.Len() != 2 || !lf.RecordAt(1).Time.Equal(t0.Add(40*time.Millisecond)) {
		t.Fatalf("read back %d records, second at %v", lf.Len(), lf.RecordAt(1).Time)
	}
}

// The BPL writer emits "BPL", version 1, a little-endian column count and
// length-prefixed names once, then fixed-size records of an int64 unix-nano
// timestamp and one float32 per column. logfile reads it back unchanged.
func TestBPLWriter(t *testing.T) {
	a, b := 1.5, -2.25
	chans := testChannels(map[string]*float64{"A": &a, "Bee": &b}, "A", "Bee")
	path := filepath.Join(t.TempDir(), "x.bpl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := NewBPLWriter(f)
	t0 := time.Unix(1_700_000_000, 42)
	if err := w.Write(t0, chans); err != nil {
		t.Fatal(err)
	}
	a, b = 3.1, 1e6
	if err := w.Write(t0.Add(time.Second), chans); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)

	var want []byte
	want = append(want, "BPL"...)
	want = append(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = append(want, "A"...)
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = append(want, "Bee"...)
	header := len(want)
	want = binary.LittleEndian.AppendUint64(want, uint64(t0.UnixNano()))
	want = binary.LittleEndian.AppendUint32(want, math.Float32bits(1.5))
	want = binary.LittleEndian.AppendUint32(want, math.Float32bits(-2.25))
	if len(raw) != header+2*(8+2*4) || !bytes.Equal(raw[:len(want)], want) {
		t.Fatalf("file % X\nwant prefix % X", raw, want)
	}

	lf, err := logfile.NewFromBPLLogfile(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(lf.Columns(), []string{"A", "Bee"}) || lf.Len() != 2 {
		t.Fatalf("columns %v len %d", lf.Columns(), lf.Len())
	}
	if r := lf.RecordAt(1); !r.Time.Equal(t0.Add(time.Second)) {
		t.Fatalf("second record at %v", r.Time)
	}
	if got := lf.Column("A"); got[1] != float64(float32(3.1)) {
		t.Fatalf("A = %v", got)
	}
	if got := lf.Column("Bee"); got[1] != 1e6 {
		t.Fatalf("Bee = %v", got)
	}
}

// Close reports a failure to flush or close the underlying file.
func TestWritersCloseReportsFileErrors(t *testing.T) {
	for name, mk := range map[string]func(*os.File) LogWriter{
		"csv": func(f *os.File) LogWriter { return NewCSVWriter(f) },
		"bpl": func(f *os.File) LogWriter { return NewBPLWriter(f) },
		"txl": func(f *os.File) LogWriter { return NewTXLWriter(f) },
	} {
		f, err := os.Create(filepath.Join(t.TempDir(), "x"))
		if err != nil {
			t.Fatal(err)
		}
		w := mk(f)
		f.Close()
		if err := w.Close(); err == nil {
			t.Errorf("%s: Close on a closed file succeeded", name)
		}
	}
}

func exportFixture() []logfile.Record {
	t0 := time.Date(2026, 10, 10, 8, 30, 0, 0, time.UTC) // text formats carry no zone and read back as UTC
	times := []time.Time{t0, t0.Add(25 * time.Millisecond), t0.Add(50 * time.Millisecond)}
	rows := []map[string]float64{
		{"Rpm": 900, "Boost": 0.25, EXTERNALWBLSYM: 1.002},
		{"Rpm": 1200, "Boost": 0.5, EXTERNALWBLSYM: 0.987},
		{"Rpm": 1500, "Boost": -0.75, EXTERNALWBLSYM: 0.9},
	}
	lf := logfile.FromRows(times, rows)
	out := make([]logfile.Record, lf.Len())
	for i := range out {
		out[i] = lf.RecordAt(i)
	}
	return out
}

// ExportRecords writes a clip in the format named by ext with the columns in
// alphabetical order; reading the clip back gives the same timestamps and
// values (to the format's precision).
func TestExportRecordsRoundTrip(t *testing.T) {
	recs := exportFixture()
	for _, ext := range []string{"csv", ".BPL", "t7l", "t5l", "t8l"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			path, err := ExportRecords(dir, "clip.of.log", ext, recs)
			if err != nil {
				t.Fatal(err)
			}
			wantExt := "." + strings.ToLower(strings.TrimPrefix(ext, "."))
			if filepath.Dir(path) != dir || filepath.Ext(path) != wantExt || !strings.HasPrefix(filepath.Base(path), "clip_of_log-") {
				t.Fatalf("path %q", path)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			lf, err := logfile.Open(path, f)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(lf.Columns(), []string{"Boost", EXTERNALWBLSYM, "Rpm"}) {
				t.Fatalf("columns %v", lf.Columns())
			}
			if lf.Len() != len(recs) {
				t.Fatalf("%d records, want %d", lf.Len(), len(recs))
			}
			for i, want := range recs {
				got := lf.RecordAt(i)
				if !got.Time.Equal(want.Time) {
					t.Errorf("record %d at %v, want %v", i, got.Time, want.Time)
				}
				for _, col := range lf.Columns() {
					w, _ := want.Value(col)
					g, ok := got.Value(col)
					if !ok || math.Abs(g-w) > 1e-6 {
						t.Errorf("record %d %s = %v, want %v", i, col, g, w)
					}
				}
			}
		})
	}
}

// An empty clip is refused before any file is created.
func TestExportRecordsEmpty(t *testing.T) {
	dir := t.TempDir()
	if _, err := ExportRecords(dir, "x", "csv", nil); err == nil {
		t.Fatal("exporting nothing succeeded")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files created: %v", entries)
	}
}

// An unsupported format is an error.
func TestExportRecordsUnsupportedFormat(t *testing.T) {
	_, err := ExportRecords(t.TempDir(), "x", "xlsx", exportFixture())
	if err == nil || !strings.Contains(err.Error(), "unsupported export format") {
		t.Fatalf("err = %v", err)
	}
}

// A refused export leaves nothing behind in the log directory.
func TestExportRecordsUnsupportedFormatLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	_, _ = ExportRecords(dir, "x", "xlsx", exportFixture())
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files left behind: %v", entries)
	}
}

// Every column of the clip is exported even when the clip's first record has
// no value for it (TXL lines may carry different symbol sets).
func TestExportRecordsKeepsColumnsMissingFromFirstRecord(t *testing.T) {
	t0 := time.Date(2026, 10, 10, 8, 30, 0, 0, time.UTC)
	lf := logfile.FromRows(
		[]time.Time{t0, t0.Add(time.Second)},
		[]map[string]float64{{"Rpm": 900}, {"Rpm": 1000, "Boost": 0.5}},
	)
	path, err := ExportRecords(t.TempDir(), "x", "csv", []logfile.Record{lf.RecordAt(0), lf.RecordAt(1)})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := logfile.Open(path, f)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := got.RecordAt(1).Value("Boost"); !ok || v != 0.5 {
		t.Fatalf("Boost in second record = %v %v, columns %v", v, ok, got.Columns())
	}
}
