package datalogger

import (
	"fmt"

	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

const ISO8601 = "2006-01-02T15:04:05.999-0700"

type Dashboard interface {
	SetValue(name string, value float64)
}

type DataClient interface {
	Start() error
	Close()
	AttachDashboard(Dashboard)
	DetachDashboard(Dashboard)
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
