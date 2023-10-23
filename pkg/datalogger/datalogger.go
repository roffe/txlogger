package datalogger

import (
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

const ISO8601 = "2006-01-02T15:04:05.999-0700"

type DataLoggerClient interface {
	SetValue(name string, value float64)
}

type DataClient interface {
	Start() error
	Close()
	Attach(DataLoggerClient)
	Detach(DataLoggerClient)
}

type Config struct {
	ECU                   string
	Dev                   gocan.Adapter
	Variables             []*kwp2000.VarDefinition
	Freq                  int
	OnMessage             func(string)
	CaptureCounter        binding.Int
	ErrorCounter          binding.Int
	ErrorPerSecondCounter binding.Int
}

func New(cfg Config) (DataClient, error) {
	switch cfg.ECU {
	case "T7":
		return NewT7(cfg)
	case "T8":
		return NewT8(cfg)
	default:
		return nil, fmt.Errorf("%s not supported yet", cfg.ECU)
	}
}

type DataLogger struct {
	cfg      Config
	sysvars  *ThreadSafeMap
	dbs      []DataLoggerClient
	quitChan chan struct{}
	mu       sync.Mutex
}

func NewDataLogger(cfg Config) *DataLogger {
	return &DataLogger{
		cfg:      cfg,
		sysvars:  &ThreadSafeMap{values: make(map[string]string)},
		quitChan: make(chan struct{}, 2),
	}
}

func (d *DataLogger) Start() error {
	return nil
}

func (d *DataLogger) Close() {
	close(d.quitChan)
	time.Sleep(100 * time.Millisecond)
}

func (d *DataLogger) Attach(db DataLoggerClient) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, dbz := range d.dbs {
		if db == dbz {
			log.Println("Dropping")
			return
		}
	}
	d.dbs = append(d.dbs, db)
}

func (d *DataLogger) Detach(db DataLoggerClient) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, dbz := range d.dbs {
		if db == dbz {
			d.dbs = append(d.dbs[:i], d.dbs[i+1:]...)
			return
		}
	}
}

func (d *DataLogger) setDbValue(name string, value float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, db := range d.dbs {
		db.SetValue(name, value)
	}
}
