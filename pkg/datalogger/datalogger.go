package datalogger

import (
	"fmt"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
)

var (
	ErrToManyErrors = fmt.Errorf("too many errors, aborting logging")
)

const ISO8601 = "2006-01-02T15:04:05.999-0700"
const ISONICO = "2006-01-02 15:04:05,999"
const EXTERNALWBLSYM = "Lambda.External"

type LogWriter interface {
	Write(sysvars *ThreadSafeMap, vars []*symbol.Symbol, ts time.Time, sysvarOrder []string) error
	Close() error
}

type IClient interface {
	Start() error
	SetRAM(address uint32, data []byte) error
	GetRAM(address uint32, length uint32) ([]byte, error)
	Close()
}

type Consumer interface {
	SetValue(string, float64)
}

type Config struct {
	FilenamePrefix string
	ECU            string
	Device         gocan.Adapter
	Symbols        []*symbol.Symbol
	Rate           int
	OnMessage      func(string)
	CaptureCounter func(int)
	ErrorCounter   func(int)
	FpsCounter     func(int)
	LogFormat      string
	LogPath        string
	WidebandConfig WidebandConfig
}

type Client struct {
	cfg Config
	IClient
}

type WidebandConfig struct {
	Type                   string
	Port                   string
	MinimumVoltageWideband float64
	MaximumVoltageWideband float64
	Low                    float64
	High                   float64
}

func New(cfg Config) (IClient, string, error) {
	datalogger := &Client{
		cfg: cfg,
	}

	filename, lw, err := NewWriter(cfg)
	if err != nil {
		return nil, "", err
	}

	cfg.OnMessage(fmt.Sprintf("Logging to %s", filename))

	if cfg.Device.Name() == "txbridge wifi" || cfg.Device.Name() == "txbridge bluetooth" {
		dc, err := NewTxbridge(cfg, lw)
		if err != nil {
			return nil, "", err
		}
		return dc, filename, nil
	}

	switch cfg.ECU {
	case "T5":
		datalogger.IClient, err = NewT5(cfg, lw)
		if err != nil {
			return nil, "", err
		}
	case "T7":
		datalogger.IClient, err = NewT7(cfg, lw)
		if err != nil {
			return nil, "", err
		}
	case "T8":
		datalogger.IClient, err = NewT8(cfg, lw)
		if err != nil {
			return nil, "", err
		}
	default:
		return nil, "", fmt.Errorf("%s not supported yet", cfg.ECU)
	}

	return datalogger, filename, nil
}

func (d *Client) Start() error {
	d.cfg.ErrorCounter(0)
	d.cfg.CaptureCounter(0)
	d.cfg.FpsCounter(0)
	return d.IClient.Start()
}
