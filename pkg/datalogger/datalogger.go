package datalogger

import (
	"context"
	"fmt"
	"log"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

var ErrToManyErrors = fmt.Errorf("too many errors, aborting logging")

const (
	EXTERNALWBLSYM  = "Lambda.External"
	LAMBDAADSCANNER = "Lambda.ADScanner"
)

type LogWriter interface {
	Write(ts time.Time, channels []Channel) error
	Close() error
}

type IClient interface {
	// Start runs the logging session until ctx is cancelled, Close is
	// called or a fatal error occurs.
	Start(ctx context.Context) error
	SetRAM(address uint32, data []byte) error
	GetRAM(address uint32, length uint32) ([]byte, error)
	Close()
}

type Config struct {
	FilenamePrefix            string
	ECU                       string
	Device                    gocan.Adapter
	Symbols                   []*symbol.Symbol
	Rate                      int
	OnMessage                 func(string)
	CaptureCounter            func(int)
	ErrorCounter              func(int)
	FpsCounter                func(int)
	LogFormat                 string
	LogPath                   string
	WidebandConfig            WidebandConfig
	RemoteMode                int
	ExperimentalT5FastLogging bool
	SeedKey                   *kwp2000.SeedKey // T7: custom SecurityAccess pair, tried first
}

type Client struct {
	cfg Config
	IClient
}

type WidebandConfig struct {
	Name            string
	Port            string
	ADScanner       bool
	ADScannerSymbol string
	SupportPoints   []int
	LambdaValues    []float64
}

func New(cfg Config) (IClient, string, error) {
	log.Println("RemoteMode", cfg.RemoteMode)
	datalogger := &Client{
		cfg: cfg,
	}

	filename, lw, err := NewWriter(cfg)
	if err != nil {
		return nil, "", err
	}

	cfg.OnMessage(fmt.Sprintf("Logging to %s", filename))

	if cfg.RemoteMode == 2 {
		datalogger.IClient, err = NewRemote(cfg, lw)
		if err != nil {
			return nil, "", err
		}
		return datalogger, filename, nil
	}

	devName := gocan.AdapterName(cfg.Device)
	if devName == "txbridge wifi" || devName == "txbridge bluetooth" {
		dc, err := NewTxbridge(cfg, lw)
		if err != nil {
			return nil, "", err
		}
		return dc, filename, nil
	}

	switch cfg.ECU {
	case "T5":
		if cfg.ExperimentalT5FastLogging {
			debug.Log("Using experimental T5 fast logger")
			datalogger.IClient, err = NewT5Fast(cfg, lw)
		} else {
			datalogger.IClient, err = NewT5(cfg, lw)
		}
		if err != nil {
			return nil, "", err
		}
	case "T7":
		datalogger.IClient, err = NewT7(cfg, lw)
		// datalogger.IClient, err = NewRemote(cfg, lw)
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

func (d *Client) Start(ctx context.Context) error {
	d.cfg.ErrorCounter(0)
	d.cfg.CaptureCounter(0)
	d.cfg.FpsCounter(0)
	return d.IClient.Start(ctx)
}
