package logfile

import (
	"path"
	"strings"
	"time"
)

type Logfile interface {
	Next() *Record
	Prev() *Record
	Seek(int) *Record
	Pos() int
	Len() int
	Start() time.Time
	End() time.Time
}

func NewRecord(time time.Time) *Record {
	return &Record{
		Time:   time,
		Values: make(map[string]float64),
	}
}

type Record struct {
	Time          time.Time
	DelayTillNext int64
	Values        map[string]float64
	//mu            sync.Mutex
}

func (r *Record) SetValue(key string, value float64) {
	//r.mu.Lock()
	//defer r.mu.Unlock()
	r.Values[key] = value
}

func Open(filename string) (Logfile, error) {
	switch strings.ToLower(path.Ext(filename)) {
	case ".csv":
		return NewFromCSVLogfile(filename)
	case ".t7l", ".t8l":
		fallthrough
	default:
		return NewFromTxLogfile(filename)
	}
}
