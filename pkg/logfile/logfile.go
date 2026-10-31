package logfile

import (
	"fmt"
	"io"
	"iter"
	"math"
	"path"
	"strings"
	"time"
)

const (
	ISO8601 = "2006-01-02T15:04:05.999-0700"
	ISONICO = "2006-01-02 15:04:05,999"
)

type Logfile interface {
	Get() Record
	Next() Record
	Prev() Record
	Seek(int)
	Pos() int
	Len() int
	RecordAt(int) Record
	Start() time.Time
	End() time.Time
	// Columns returns the value column names in file order.
	Columns() []string
	// Column returns every value of the named column, one per record, with NaN
	// where a record lacks the value, or nil when the log has no such column.
	// The slice is shared with the log and must not be modified.
	Column(name string) []float64
	Close()
}

// Record is one row of a log. The values stay in the log's column store, so a
// Record is only a timestamp and a row index and costs nothing to pass around.
type Record struct {
	Time          time.Time
	DelayTillNext int64
	EOF           bool

	t   *table
	row int
}

// Value returns the named value and whether this record has it.
func (r Record) Value(name string) (float64, bool) {
	if r.t == nil {
		return 0, false
	}
	c, ok := r.t.index[name]
	if !ok {
		return 0, false
	}
	v := r.t.cols[c][r.row]
	if math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// All yields every value the record has, in column order.
func (r Record) All() iter.Seq2[string, float64] {
	return func(yield func(string, float64) bool) {
		if r.t == nil {
			return
		}
		for c, name := range r.t.names {
			if v := r.t.cols[c][r.row]; !math.IsNaN(v) && !yield(name, v) {
				return
			}
		}
	}
}

func Open(filename string, reader io.Reader) (Logfile, error) {
	switch strings.ToLower(path.Ext(filename)) {
	case ".csv":
		return NewFromCSVLogfile(reader)
	case ".t5l", ".t7l", ".t8l":
		return NewFromTxLogfile(reader)
	case ".bpl":
		return NewFromBPLLogfile(reader)
	default:
		return nil, fmt.Errorf("Unsupported filetype")
	}
}

// FromRows builds an in-memory log, one record per time and value map.
func FromRows(times []time.Time, rows []map[string]float64) Logfile {
	t := newTable()
	for i, ts := range times {
		row := t.addRow(ts)
		for k, v := range rows[i] {
			t.cols[t.column(k)][row] = v
		}
	}
	return newBaseLogfile(t)
}
