package datalogger

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/symbol"
)

const ISO8601 = "2006-01-02T15:04:05.999-0700"

type Provider interface {
	Start() error
	Close()
}

type Setter interface {
	SetValue(string, float64)
}

type Logger interface {
	Provider
	Attach(Setter)
	Detach(Setter)
	Setter
}

type Config struct {
	ECU                   string
	Device                gocan.Adapter
	Symbols               []*symbol.Symbol
	Freq                  int
	OnMessage             func(string)
	CaptureCounter        binding.Int
	ErrorCounter          binding.Int
	ErrorPerSecondCounter binding.Int
}

type Client struct {
	cfg Config
	dbs []Setter
	mu  sync.Mutex
	dlc Provider
}

func New(cfg Config) (Logger, error) {
	dl := &Client{
		cfg: cfg,
	}
	var err error
	switch cfg.ECU {
	case "T7":
		dl.dlc, err = NewT7(dl, cfg)
		if err != nil {
			return nil, err
		}
	case "T8":
		dl.dlc, err = NewT8(dl, cfg)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s not supported yet", cfg.ECU)
	}

	return dl, nil
}

func (d *Client) Start() error {
	return d.dlc.Start()
}

func (d *Client) Close() {
	if d.dlc != nil {
		d.dlc.Close()
	}
}

func (d *Client) Attach(db Setter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, dbz := range d.dbs {
		if db == dbz {
			return
		}
	}
	d.dbs = append(d.dbs, db)
}

func (d *Client) Detach(db Setter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, dbz := range d.dbs {
		if db == dbz {
			d.dbs = append(d.dbs[:i], d.dbs[i+1:]...)
			return
		}
	}
}

func (d *Client) SetValue(name string, value float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, db := range d.dbs {
		db.SetValue(name, value)
	}
}
