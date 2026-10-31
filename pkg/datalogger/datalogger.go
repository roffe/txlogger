package datalogger

import (
	"context"
	"fmt"
	"log"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/debug"
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
	SeedKey                   *t7kwp.SeedKey // T7: custom SecurityAccess pair, tried first
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

// loggers builds the logging client for each ECU that can be logged over a
// plain CAN adapter; txbridgeECUs is the txbridge counterpart. A new ECU's
// logger is one entry in each.
var loggers = map[string]func(Config, LogWriter) (IClient, error){
	"T5": func(cfg Config, lw LogWriter) (IClient, error) {
		if cfg.ExperimentalT5FastLogging {
			debug.Log("Using experimental T5 fast logger")
			return NewT5Fast(cfg, lw)
		}
		return NewT5(cfg, lw)
	},
	"T7": NewT7,
	"T8": NewT8,
}

func New(cfg Config) (IClient, string, error) {
	log.Println("RemoteMode", cfg.RemoteMode)

	devName := gocan.AdapterName(cfg.Device)
	txbridge := devName == "txbridge wifi" || devName == "txbridge bluetooth"
	newLogger, ok := loggers[cfg.ECU]
	if txbridge {
		_, ok = txbridgeECUs[cfg.ECU]
	}
	if !ok && cfg.RemoteMode != 2 {
		return nil, "", fmt.Errorf("%s not supported yet", cfg.ECU)
	}

	filename, lw, err := NewWriter(cfg)
	if err != nil {
		return nil, "", err
	}

	cfg.OnMessage(fmt.Sprintf("Logging to %s", filename))

	datalogger := &Client{
		cfg: cfg,
	}
	switch {
	case cfg.RemoteMode == 2:
		datalogger.IClient, err = NewRemote(cfg, lw)
	case txbridge:
		datalogger.IClient, err = NewTxbridge(cfg, lw)
	default:
		datalogger.IClient, err = newLogger(cfg, lw)
	}
	if err != nil {
		lw.Close()
		return nil, "", err
	}
	return datalogger, filename, nil
}

func (d *Client) Start(ctx context.Context) error {
	d.cfg.ErrorCounter(0)
	d.cfg.CaptureCounter(0)
	d.cfg.FpsCounter(0)
	return d.IClient.Start(ctx)
}
