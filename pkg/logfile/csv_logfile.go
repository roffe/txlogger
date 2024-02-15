package logfile

import (
	"encoding/csv"
	"os"
	"strconv"
	"time"

	"github.com/roffe/txlogger/pkg/datalogger"
)

type CSVLogfile struct {
	records []*Record
	length  int
	pos     int
}

func NewFromCSVLogfile(filename string) (Logfile, error) {
	rec, err := parseCSVLogfile(filename)
	if err != nil {
		return nil, err
	}
	csvlog := &CSVLogfile{
		records: rec,
		length:  len(rec),
		pos:     -1,
	}

	return csvlog, nil
}

func (l *CSVLogfile) Next() *Record {
	if l.pos+1 > l.length-1 || l.pos+1 < 0 {
		return nil
	}
	l.pos++
	return l.records[l.pos]
}

func (l *CSVLogfile) Prev() *Record {
	if l.pos-1 < 0 {
		return nil
	}
	l.pos--
	return l.records[l.pos]
}

func (l *CSVLogfile) Seek(pos int) *Record {
	if pos < 0 || pos >= l.length {
		return nil
	}
	l.pos = pos
	return l.records[pos]
}

func (l *CSVLogfile) Pos() int {
	return l.pos
}

func (l *CSVLogfile) SeekTime(time.Time) *Record {
	return nil
}

func (l *CSVLogfile) Len() int {
	return l.length
}

func (l *CSVLogfile) Start() time.Time {
	if l.length > 0 {
		return l.records[0].Time
	}
	return time.Time{}
}

func (l *CSVLogfile) End() time.Time {
	if l.length > 0 {
		return l.records[l.length-1].Time
	}
	return time.Time{}
}

func parseCSVLogfile(filename string) ([]*Record, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	var recs []*Record

	for i := 1; i < len(records); i++ {
		ts, err := time.Parse(datalogger.ISONICO, records[i][0])
		if err != nil {
			return nil, err
		}
		rec := NewRecord(ts)

		for j := 1; j < len(records[i]); j++ {
			val, err := strconv.ParseFloat(records[i][j], 64)
			if err != nil {
				return nil, err
			}
			rec.SetValue(records[0][j], val)
		}

		if i < len(records)-1 {
			ts2, err := time.Parse(datalogger.ISONICO, records[i+1][0])
			if err != nil {
				return nil, err
			}
			rec.DelayTillNext = ts2.Sub(ts).Milliseconds()
		}

		recs = append(recs, rec)
	}

	return recs, nil
}
