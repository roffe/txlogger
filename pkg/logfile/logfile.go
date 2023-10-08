package logfile

import "time"

type Logfile interface {
	Next() *Record
	Prev() *Record
	Seek(int) *Record
	Pos() int
	Len() int
	Start() time.Time
	End() time.Time
}

type Record struct {
	Time          time.Time
	DelayTillNext int64
	Values        []*RecordValue
}

type RecordValue struct {
	Key   string
	Value float64
}
