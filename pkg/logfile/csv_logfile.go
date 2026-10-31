package logfile

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// NewFromCSVLogfile reads row by row and skips malformed rows and values
// instead of aborting: real-world logs regularly contain a truncated or
// corrupted line (interrupted writes) and one bad line must not make the
// rest of the log unreadable.
func NewFromCSVLogfile(reader io.Reader) (Logfile, error) {
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = true

	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	t := newTable()
	cols := make([]int, len(header)) // header field -> column, 0 is the time
	for j := 1; j < len(header); j++ {
		cols[j] = t.column(header[j])
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) == 0 {
			continue // skip malformed line
		}
		ts, err := time.Parse(ISONICO, row[0])
		if err != nil {
			continue
		}
		n := t.addRow(ts)
		for j := 1; j < len(row) && j < len(cols); j++ {
			val, err := strconv.ParseFloat(row[j], 64)
			if err != nil {
				continue // skip unparsable value, keep the rest of the row
			}
			t.cols[cols[j]][n] = val
		}
	}
	if len(t.times) == 0 {
		return nil, fmt.Errorf("no valid records in log")
	}
	return newBaseLogfile(t), nil
}
