package logfile

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

var _ Logfile = (*CSVLogfile)(nil)

type CSVLogfile struct {
	BaseLogfile
}

func NewFromCSVLogfile(reader io.Reader) (Logfile, error) {
	c := &CSVLogfile{}
	c.pos = -1
	// start := time.Now()
	if err := c.parseCSVLogfile(reader); err != nil {
		return nil, err
	}
	// log.Printf("Parsed %d records in %s", len(rec), time.Since(start))
	return c, nil
}

// parseCSVLogfile reads row by row and skips malformed rows and values
// instead of aborting: real-world logs regularly contain a truncated or
// corrupted line (interrupted writes) and one bad line must not make the
// rest of the log unreadable.
func (l *CSVLogfile) parseCSVLogfile(reader io.Reader) error {
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return err
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed line
		}
		if len(row) == 0 {
			continue
		}
		ts, err := time.Parse(ISONICO, row[0])
		if err != nil {
			continue
		}
		rec := NewRecord(ts)
		for j := 1; j < len(row) && j < len(header); j++ {
			val, err := strconv.ParseFloat(row[j], 64)
			if err != nil {
				continue // skip unparsable value, keep the rest of the row
			}
			rec.SetValue(header[j], val)
		}
		l.records = append(l.records, rec)
	}
	if len(l.records) == 0 {
		return fmt.Errorf("no valid records in log")
	}

	for i := 0; i < len(l.records)-1; i++ {
		l.records[i].DelayTillNext = l.records[i+1].Time.Sub(l.records[i].Time).Milliseconds()
	}

	l.length = len(l.records)
	l.end = l.length - 1

	return nil
}
