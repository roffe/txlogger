package datalogger

import (
	"context"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2/data/binding"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
)

const ISO8601 = "2006-01-02T15:04:05.999-0700"
const ISONICO = "2006-01-02 15:04:05,999"
const EXTERNALWBLSYM = "Lambda.External"

type LogWriter interface {
	Write(sysvars *ThreadSafeMap, vars []*symbol.Symbol, ts time.Time, sysvarOrder []string) error
	Close() error
}

type Provider interface {
	Start() error
	SetRAM(address uint32, data []byte) error
	GetRAM(address uint32, length uint32) ([]byte, error)
	SetSymbols(symbols []*symbol.Symbol) error
	Close()
}

type Consumer interface {
	SetValue(string, float64)
}

type LambdaProvider interface {
	GetLambda() float64
	Start(context.Context)
	Stop()
	PrettyPrint() string
}

type Config struct {
	ECU            string
	Lambda         string
	Device         gocan.Adapter
	Symbols        []*symbol.Symbol
	Rate           int
	OnMessage      func(string)
	CaptureCounter binding.Int
	ErrorCounter   binding.Int
	FpsCounter     binding.Int
	LogFormat      string
	LogPath        string
}

type Client struct {
	cfg Config
	Provider
}

type ReadRequest struct {
	Address  uint32
	Length   uint32
	respChan chan error
	Data     []byte
	left     uint32
}

func NewReadRequest(address uint32, length uint32) *ReadRequest {
	return &ReadRequest{
		Address:  address,
		Length:   length,
		left:     length,
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
	defer func() { recover() }()
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
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout")
	}
}

type RamUpdate struct {
	Address  uint32
	Data     []byte
	left     uint32
	respChan chan error
}

func NewRamUpdate(address uint32, data []byte) *RamUpdate {
	return &RamUpdate{
		Address:  address,
		Data:     data,
		left:     uint32(len(data)),
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

func New(cfg Config) (Provider, error) {
	datalogger := &Client{
		cfg: cfg,
	}
	var err error

	filename, lw, err := NewWriter(cfg)
	if err != nil {
		return nil, err
	}

	switch cfg.ECU {
	case "T5":
		datalogger.Provider, err = NewT5(cfg, lw)
		if err != nil {
			return nil, err
		}
	case "T7":
		datalogger.Provider, err = NewT7(cfg, lw)
		if err != nil {
			return nil, err
		}
	case "T8":
		datalogger.Provider, err = NewT8(cfg, lw)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s not supported yet", cfg.ECU)
	}

	cfg.OnMessage(fmt.Sprintf("Logging to %s", filename))

	return datalogger, nil
}

func (d *Client) Start() error {
	d.cfg.ErrorCounter.Set(0)
	d.cfg.CaptureCounter.Set(0)
	d.cfg.FpsCounter.Set(0)
	return d.Provider.Start()
}

//func (d *Client) Close() {
//	if d.p != nil {
//		d.p.Close()
//	}
//}

/*
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

func (d *Client) SetSymbols(symbols []*symbol.Symbol) error {
	if d.p == nil {
		return fmt.Errorf("no provider")
	}
	return d.p.SetSymbols(symbols)
}
*/
