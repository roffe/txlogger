package datalogger

import (
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/symbol"
)

const ISO8601 = "2006-01-02T15:04:05.999-0700"

type Provider interface {
	Start() error
	SetRAM(address uint32, data []byte) error
	GetRAM(address uint32, length uint32) ([]byte, error)
	Close()
}

type Consumer interface {
	SetValue(string, float64)
}

type Logger interface {
	Provider
	Attach(Consumer)
	Detach(Consumer)
	Consumer
}

type Config struct {
	ECU            string
	Device         gocan.Adapter
	Symbols        []*symbol.Symbol
	Rate           int
	OnMessage      func(string)
	CaptureCounter binding.Int
	ErrorCounter   binding.Int
	//ErrorPerSecondCounter binding.Int
}

type Client struct {
	cfg Config
	dbs []Consumer
	mu  sync.Mutex
	p   Provider
}

type ReadRequest struct {
	Address  uint32
	Length   uint32
	respChan chan error
	Data     []byte
}

func NewReadRequest(address uint32, length uint32) *ReadRequest {
	return &ReadRequest{
		Address:  address,
		Length:   length,
		respChan: make(chan error, 1),
	}
}

func (r *ReadRequest) String() string {
	return fmt.Sprintf("%08X: %d", r.Address, r.Length)
}

func (r *ReadRequest) Len() int {
	return int(r.Length)
}

func (r *ReadRequest) Complete(err error) {
	select {
	case r.respChan <- err:
	default:
		log.Println("ReadRequest respChan full")
	}
	close(r.respChan)
}

func (r *ReadRequest) Wait() error {
	if r.respChan == nil {
		return fmt.Errorf("respChan is nil")
	}
	select {
	case err := <-r.respChan:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout")
	}
}

type RamUpdate struct {
	Address  uint32
	Data     []byte
	respChan chan error
}

func NewRamUpdate(address uint32, data []byte) *RamUpdate {
	return &RamUpdate{
		Address:  address,
		Data:     data,
		respChan: make(chan error, 1),
	}
}

func (r *RamUpdate) String() string {
	return fmt.Sprintf("%08X: % X", r.Address, r.Data)
}

func (r *RamUpdate) Len() int {
	return len(r.Data)
}

func (r *RamUpdate) Complete(err error) {
	select {
	case r.respChan <- err:
	default:
		log.Println("RamUpdate respChan full")
	}
	close(r.respChan)
}

func (r *RamUpdate) Wait() error {
	if r.respChan == nil {
		return fmt.Errorf("respChan is nil")
	}
	select {
	case err := <-r.respChan:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout")
	}
}

func New(cfg Config) (Logger, error) {
	datalogger := &Client{
		cfg: cfg,
	}
	var err error
	switch cfg.ECU {
	case "T7":
		datalogger.p, err = NewT7(datalogger, cfg)
		if err != nil {
			return nil, err
		}
	case "T8":
		datalogger.p, err = NewT8(datalogger, cfg)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s not supported yet", cfg.ECU)
	}
	return datalogger, nil
}

func (d *Client) Start() error {
	return d.p.Start()
}

func (d *Client) Close() {
	if d.p != nil {
		d.p.Close()
	}
}

func (d *Client) Attach(db Consumer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, dbz := range d.dbs {
		if db == dbz {
			return
		}
	}
	d.dbs = append(d.dbs, db)
}

func (d *Client) Detach(db Consumer) {
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

func (d *Client) SetRAM(address uint32, data []byte) error {
	if d.p == nil {
		return fmt.Errorf("no provider")
	}
	return d.p.SetRAM(address, data)
}

func (d *Client) GetRAM(address uint32, length uint32) ([]byte, error) {
	if d.p == nil {
		return nil, fmt.Errorf("no provider")
	}
	return d.p.GetRAM(address, length)
}
