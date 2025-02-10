package logfile

import (
	"time"
)

type BaseLogfile struct {
	records []Record
	length  int
	pos     int
	end     int
}

func (l *BaseLogfile) Get() Record {
	return l.records[l.pos]
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
	return l.records[l.pos]
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
	return l.records[l.pos]
}

func (l *BaseLogfile) Seek(pos int) Record {
	l.pos = pos
	if l.pos >= l.end {
		l.pos = l.end
	}
	if l.pos < 0 {
		l.pos = 0
	}
	return l.records[l.pos]
}

func (l *BaseLogfile) Pos() int {
	return max(l.pos, 0)
}

func (l *BaseLogfile) Len() int {
	return l.length
}

func (l *BaseLogfile) Start() time.Time {
	if l.length > 0 {
		return l.records[0].Time
	}
	return time.Time{}
}

func (l *BaseLogfile) End() time.Time {
	if l.length > 0 {
		return l.records[l.end].Time
	}
	return time.Time{}
}

func (l *BaseLogfile) Length() time.Duration {
	if l.length > 0 {
		return l.records[l.end].Time.Sub(l.records[0].Time)
	}
	return 0
}

func (l *BaseLogfile) Close() {
	l.records = nil
	l.length = 0
	l.pos = -1
}
