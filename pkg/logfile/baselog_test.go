package logfile

import (
	"math"
	"slices"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// fiveRows is a log of five records 100 ms apart where "rpm" is present in
// every row and "boost" only in rows 1 and 3.
func fiveRows() Logfile {
	times := make([]time.Time, 5)
	rows := make([]map[string]float64, 5)
	for i := range times {
		times[i] = t0.Add(time.Duration(i) * 100 * time.Millisecond)
		rows[i] = map[string]float64{"rpm": float64(800 + i*100)}
	}
	rows[1]["boost"] = 0.5
	rows[3]["boost"] = 1.25
	return FromRows(times, rows)
}

func rowOf(t *testing.T, r Record) int {
	t.Helper()
	if r.EOF {
		return -1
	}
	return int(r.Time.Sub(t0) / (100 * time.Millisecond))
}

// FromRows keeps one record per time, with values missing where a row lacks a
// column, and columns in the order they were first seen.
func TestFromRows(t *testing.T) {
	lf := fiveRows()
	if lf.Len() != 5 {
		t.Fatalf("Len = %d, want 5", lf.Len())
	}
	if !lf.Start().Equal(t0) || !lf.End().Equal(t0.Add(400*time.Millisecond)) {
		t.Errorf("Start/End = %v/%v", lf.Start(), lf.End())
	}
	if cols := lf.Columns(); len(cols) != 2 || !slices.Contains(cols, "rpm") || !slices.Contains(cols, "boost") {
		t.Errorf("Columns = %v", cols)
	}
	boost := lf.Column("boost")
	if len(boost) != 5 || !math.IsNaN(boost[0]) || boost[1] != 0.5 || !math.IsNaN(boost[2]) || boost[3] != 1.25 || !math.IsNaN(boost[4]) {
		t.Errorf("Column(boost) = %v", boost)
	}
	if lf.Column("nope") != nil {
		t.Error("Column of an unknown name is not nil")
	}
	if v, ok := lf.RecordAt(2).Value("rpm"); !ok || v != 1000 {
		t.Errorf("rpm[2] = %v %v", v, ok)
	}
	if _, ok := lf.RecordAt(2).Value("boost"); ok {
		t.Error("boost[2] should be missing")
	}
	if _, ok := lf.RecordAt(2).Value("nope"); ok {
		t.Error("unknown column reported as present")
	}
}

// All yields only the values a record has, in column order, and stops when
// the consumer breaks out of the loop.
func TestRecordAll(t *testing.T) {
	lf := FromRows([]time.Time{t0, t0.Add(time.Second)}, []map[string]float64{
		{"a": 1},
		{"a": 2, "b": 3},
	})
	var names []string
	var sum float64
	for k, v := range lf.RecordAt(1).All() {
		names = append(names, k)
		sum += v
	}
	if !slices.Equal(names, []string{"a", "b"}) || sum != 5 {
		t.Errorf("All = %v (sum %v), want [a b] (sum 5)", names, sum)
	}

	var first []string
	for k := range lf.RecordAt(1).All() {
		first = append(first, k)
		break
	}
	if len(first) != 1 {
		t.Errorf("All yielded %v after break", first)
	}

	names = names[:0]
	for k := range lf.RecordAt(0).All() {
		names = append(names, k)
	}
	if !slices.Equal(names, []string{"a"}) {
		t.Errorf("All on row 0 = %v, want [a]", names)
	}
}

// A zero Record has no values and yields nothing.
func TestZeroRecord(t *testing.T) {
	var r Record
	if _, ok := r.Value("rpm"); ok {
		t.Error("zero Record has a value")
	}
	for k := range r.All() {
		t.Errorf("zero Record yielded %q", k)
	}
}

// DelayTillNext is the gap to the following record in milliseconds, and zero
// for the last record.
func TestDelayTillNext(t *testing.T) {
	lf := FromRows(
		[]time.Time{t0, t0.Add(40 * time.Millisecond), t0.Add(140 * time.Millisecond)},
		[]map[string]float64{{}, {}, {}},
	)
	for i, want := range []int64{40, 100, 0} {
		if got := lf.RecordAt(i).DelayTillNext; got != want {
			t.Errorf("DelayTillNext[%d] = %d, want %d", i, got, want)
		}
	}
}

// Playback starts before the first record: Next steps forward and returns the
// record it lands on, stops with EOF past the end without moving off the last
// record, and Prev steps back without going before the first record.
func TestNavigation(t *testing.T) {
	lf := fiveRows()
	if lf.Pos() != 0 || rowOf(t, lf.Get()) != 0 {
		t.Errorf("fresh log Pos/Get = %d/%d, want 0/0", lf.Pos(), rowOf(t, lf.Get()))
	}
	for want := 0; want < 5; want++ {
		r := lf.Next()
		if got := rowOf(t, r); got != want || lf.Pos() != want {
			t.Fatalf("Next #%d = row %d at Pos %d", want, got, lf.Pos())
		}
	}
	if r := lf.Next(); !r.EOF {
		t.Errorf("Next past the end = row %d, want EOF", rowOf(t, r))
	}
	if lf.Pos() != 4 || rowOf(t, lf.Get()) != 4 {
		t.Errorf("after EOF Pos/Get = %d/%d, want 4/4", lf.Pos(), rowOf(t, lf.Get()))
	}
	if got := rowOf(t, lf.Prev()); got != 3 {
		t.Errorf("Prev = row %d, want 3", got)
	}
	lf.Seek(0)
	if got := rowOf(t, lf.Prev()); got != 0 || lf.Pos() != 0 {
		t.Errorf("Prev at the start = row %d Pos %d, want 0 0", got, lf.Pos())
	}
}

// Seek clamps past-the-end positions to the last record and negative ones to
// "before the first record", from where Next returns record 0.
func TestSeek(t *testing.T) {
	lf := fiveRows()
	for _, tc := range []struct{ seek, pos, next int }{
		{2, 2, 3},
		{4, 4, -1},
		{99, 4, -1},
		{-1, 0, 0},
		{-50, 0, 0},
	} {
		lf.Seek(tc.seek)
		if lf.Pos() != tc.pos {
			t.Errorf("Seek(%d): Pos = %d, want %d", tc.seek, lf.Pos(), tc.pos)
		}
		if got := rowOf(t, lf.Get()); got != tc.pos {
			t.Errorf("Seek(%d): Get = row %d, want %d", tc.seek, got, tc.pos)
		}
		if got := rowOf(t, lf.Next()); got != tc.next {
			t.Errorf("Seek(%d): Next = row %d, want %d", tc.seek, got, tc.next)
		}
	}
}

// RecordAt clamps its index to the log and leaves the playback position alone.
func TestRecordAtClamps(t *testing.T) {
	lf := fiveRows()
	lf.Seek(2)
	for _, tc := range []struct{ i, want int }{{-3, 0}, {0, 0}, {3, 3}, {4, 4}, {10, 4}} {
		if got := rowOf(t, lf.RecordAt(tc.i)); got != tc.want {
			t.Errorf("RecordAt(%d) = row %d, want %d", tc.i, got, tc.want)
		}
	}
	if lf.Pos() != 2 {
		t.Errorf("Pos = %d after RecordAt, want 2", lf.Pos())
	}
}

// An empty log reports EOF from every accessor and zero times.
func TestEmptyLog(t *testing.T) {
	lf := FromRows(nil, nil)
	if lf.Len() != 0 || !lf.Start().IsZero() || !lf.End().IsZero() {
		t.Errorf("Len/Start/End = %d/%v/%v", lf.Len(), lf.Start(), lf.End())
	}
	for name, r := range map[string]Record{
		"Get": lf.Get(), "Next": lf.Next(), "Prev": lf.Prev(), "RecordAt": lf.RecordAt(0),
	} {
		if !r.EOF {
			t.Errorf("%s on an empty log is not EOF", name)
		}
	}
	lf.Seek(3)
	if !lf.Get().EOF || lf.Pos() != 0 {
		t.Errorf("after Seek on empty log: Get EOF %v, Pos %d", lf.Get().EOF, lf.Pos())
	}
}

// Length is the time between the first and last record.
func TestLength(t *testing.T) {
	bl := fiveRows().(*BaseLogfile)
	if got := bl.Length(); got != 400*time.Millisecond {
		t.Errorf("Length = %v, want 400ms", got)
	}
	if got := FromRows(nil, nil).(*BaseLogfile).Length(); got != 0 {
		t.Errorf("empty Length = %v, want 0", got)
	}
}

// Close empties the log, but records handed out before it stay readable.
func TestClose(t *testing.T) {
	lf := fiveRows()
	kept := lf.RecordAt(3)
	lf.Close()

	if lf.Len() != 0 || lf.Columns() != nil || lf.Column("rpm") != nil {
		t.Errorf("after Close Len/Columns/Column = %d/%v/%v", lf.Len(), lf.Columns(), lf.Column("rpm"))
	}
	if !lf.Next().EOF || !lf.Get().EOF || !lf.RecordAt(0).EOF {
		t.Error("closed log still returns records")
	}
	if !lf.Start().IsZero() || !lf.End().IsZero() {
		t.Error("closed log still reports times")
	}
	if v, ok := kept.Value("boost"); !ok || v != 1.25 {
		t.Errorf("record kept across Close: boost = %v %v, want 1.25 true", v, ok)
	}
}

// RecordAt and Column can be read from other goroutines while playback moves
// the position.
func TestRecordAtConcurrentWithPlayback(t *testing.T) {
	lf := fiveRows()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			if v, ok := lf.RecordAt(i % 5).Value("rpm"); !ok || v != float64(800+(i%5)*100) {
				t.Errorf("RecordAt(%d) rpm = %v %v", i%5, v, ok)
				return
			}
			_ = lf.Column("boost")[i%5]
		}
	}()
	for range 200 {
		lf.Seek(-1)
		for !lf.Next().EOF {
		}
	}
	wg.Wait()
}
