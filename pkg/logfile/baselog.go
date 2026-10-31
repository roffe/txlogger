package logfile

import (
	"math"
	"time"
)

// table holds a log's values column-major: one slice per column, NaN where a
// row lacks the value. A map per record cost about four times the memory, and
// the columns feed the plotter without being copied.
type table struct {
	names []string
	index map[string]int
	cols  [][]float64
	times []time.Time
}

func newTable() *table {
	return &table{index: make(map[string]int)}
}

// column returns the index of the named column, adding it, missing for every
// row so far, the first time the name is seen.
func (t *table) column(name string) int {
	if c, ok := t.index[name]; ok {
		return c
	}
	col := make([]float64, len(t.times), cap(t.times))
	for i := range col {
		col[i] = math.NaN()
	}
	t.index[name] = len(t.names)
	t.names = append(t.names, name)
	t.cols = append(t.cols, col)
	return len(t.names) - 1
}

// addRow appends a row at ts with every value missing and returns its index.
func (t *table) addRow(ts time.Time) int {
	t.times = append(t.times, ts)
	for c := range t.cols {
		t.cols[c] = append(t.cols[c], math.NaN())
	}
	return len(t.times) - 1
}

// trim drops the spare capacity append growth leaves behind, up to half of
// each column, which an open log would otherwise hold on to.
func (t *table) trim() {
	for c, col := range t.cols {
		t.cols[c] = append([]float64(nil), col...)
	}
	t.times = append([]time.Time(nil), t.times...)
}

type BaseLogfile struct {
	t      *table
	length int
	pos    int
	end    int
}

func newBaseLogfile(t *table) *BaseLogfile {
	t.trim()
	return &BaseLogfile{t: t, length: len(t.times), pos: -1, end: len(t.times) - 1}
}

func (l *BaseLogfile) record(i int) Record {
	if i < 0 || i >= l.length {
		return Record{EOF: true}
	}
	r := Record{Time: l.t.times[i], t: l.t, row: i}
	if i < l.end {
		r.DelayTillNext = l.t.times[i+1].Sub(r.Time).Milliseconds()
	}
	return r
}

func (l *BaseLogfile) Get() Record {
	return l.record(max(l.pos, 0))
}

// Next returns the current record and advances the position to the next record.
func (l *BaseLogfile) Next() Record {
	l.pos++
	if l.pos > l.end {
		l.pos = l.end
		return Record{
			EOF: true,
		}
	}
	return l.record(l.pos)
}

// Prev moves the position to the previous record and returns the record.
func (l *BaseLogfile) Prev() Record {
	l.pos--
	if l.pos < 0 {
		l.pos = 0
	}
	if l.pos > l.end {
		l.pos = l.end
	}
	return l.record(l.pos)
}

func (l *BaseLogfile) Seek(pos int) {
	l.pos = pos
	if l.pos >= l.end {
		l.pos = l.end
	}
	if l.pos < 0 {
		l.pos = -1
	}
}

func (l *BaseLogfile) Pos() int {
	return max(l.pos, 0)
}

// RecordAt returns the record at the given index without changing the playback
// position. The index is clamped to the valid range. It is safe to call
// concurrently with playback as it only reads the immutable column store.
func (l *BaseLogfile) RecordAt(i int) Record {
	if l.length == 0 {
		return Record{EOF: true}
	}
	return l.record(min(max(i, 0), l.length-1))
}

func (l *BaseLogfile) Len() int {
	return l.length
}

func (l *BaseLogfile) Columns() []string {
	if l.t == nil {
		return nil
	}
	return l.t.names
}

func (l *BaseLogfile) Column(name string) []float64 {
	if l.t == nil {
		return nil
	}
	if c, ok := l.t.index[name]; ok {
		return l.t.cols[c]
	}
	return nil
}

func (l *BaseLogfile) Start() time.Time {
	if l.length > 0 {
		return l.t.times[0]
	}
	return time.Time{}
}

func (l *BaseLogfile) End() time.Time {
	if l.length > 0 {
		return l.t.times[l.end]
	}
	return time.Time{}
}

func (l *BaseLogfile) Length() time.Duration {
	if l.length > 0 {
		return l.End().Sub(l.Start())
	}
	return 0
}

// Close drops the log's reference to its values. Records handed out earlier
// keep what they point at alive and stay readable.
func (l *BaseLogfile) Close() {
	l.t = nil
	l.length = 0
	l.pos = -1
}
