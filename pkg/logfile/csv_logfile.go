package logfile

import (
	"encoding/csv"
	"os"
	"strconv"
	"time"

	"github.com/roffe/txlogger/pkg/datalogger"
)

var _ Logfile = (*CSVLogfile)(nil)

type CSVLogfile struct {
	BaseLogfile
}

func NewFromCSVLogfile(filename string) (Logfile, error) {
	c := &CSVLogfile{}
	c.pos = -1
	// start := time.Now()
	if err := c.parseCSVLogfile(filename); err != nil {
		return nil, err
	}
	// log.Printf("Parsed %d records in %s", len(rec), time.Since(start))
	return c, nil
}

func (l *CSVLogfile) parseCSVLogfile(filename string) error {
	f, err := os.Open(filename)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return err
	}
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	for i := 1; i < len(records); i++ {
		ts, err := time.Parse(datalogger.ISONICO, records[i][0])
		if err != nil {
			return err
		}
		rec := NewRecord(ts)

		for j := 1; j < len(records[i]); j++ {
			val, err := strconv.ParseFloat(records[i][j], 64)
			if err != nil {
				return err
			}

			//if records[0][j] == "Lambda.External" {
			//	if val > 1.5 {
			//		val = 1.5
			//	} else if val < 0.5 {
			//		val = 0.5
			//	}
			//}
			rec.SetValue(records[0][j], val)
		}

		if i < len(records)-1 {
			ts2, err := time.Parse(datalogger.ISONICO, records[i+1][0])
			if err != nil {
				return err
			}
			rec.DelayTillNext = ts2.Sub(ts).Milliseconds()
		}

		l.records = append(l.records, rec)
	}

	l.length = len(l.records)
	l.end = l.length - 1

	return nil
}
