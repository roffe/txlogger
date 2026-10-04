package ecu

import (
	"context"
	"errors"
	"log"
	"sort"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/model"
)

type Client interface {
	ReadDTC(context.Context) ([]dtc.DTC, error)
	PrintECUInfo(context.Context) error
	Info(context.Context) ([]model.HeaderResult, error)
	DumpECU(context.Context) ([]byte, error)
	FlashECU(context.Context, []byte) error
	RecoverECU(context.Context, []byte) error
	EraseECU(context.Context) error
	MarryECU(context.Context, string) error
	ResetECU(context.Context) error
}

// SeedKey is a SecurityAccess (KWP2000 service 0x27) algorithm:
// key = ((seed<<2) ^ XOR) - Sub. Stock firmware uses one of a handful of known
// pairs; a locked or re-patched ECU can use any other. Read the pair out of a
// binary with the T7 Seed/Key patcher (pkg/widgets/seedkey).
type SeedKey = t7kwp.SeedKey

type Config struct {
	Name string
	// SeedKey, when set, is tried before the known-good pairs.
	SeedKey    *SeedKey
	OnProgress func(float64)
	OnError    func(error)
	OnMessage  func(string)
}

func LoadConfig(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{
			Name: "Unknown ECU",
		}
	}

	if cfg.OnProgress == nil {
		cfg.OnProgress = func(f float64) {
			log.Println(f)
		}
	}

	if cfg.OnError == nil {
		cfg.OnError = func(err error) {
			log.Println(err)
		}
	}

	if cfg.OnMessage == nil {
		cfg.OnMessage = func(msg string) {
			log.Println(msg)
		}
	}

	return cfg
}

var ecuMap = map[string]*EcuInfo{}

// EcuInfo registers a flashing/diagnostics client (canflasher). Name is the
// flasher's own name ("Trionic 7"), not the Profile name.
type EcuInfo struct {
	Name    string
	NewFunc func(c *gocan.Bus, cfg *Config) Client
	CANRate float64
	Filter  []uint32
	// ManualReset: resetting over CAN with the ignition on puts the throttle
	// body in limp mode, so flash and dump leave the reset to the user and
	// the reset button asks first.
	ManualReset bool
}

func Register(t *EcuInfo) {
	if _, found := ecuMap[t.Name]; found {
		panic("ECU already registered: " + t.Name)
	}
	ecuMap[t.Name] = t
}

func New(c *gocan.Bus, cfg *Config) (Client, error) {
	if ecu, found := ecuMap[cfg.Name]; found {
		return ecu.NewFunc(c, cfg), nil
	}
	return nil, errors.New("unknown ECU")
}

func List() (ecus []string) {
	for k := range ecuMap {
		ecus = append(ecus, k)
	}
	sort.Strings(ecus)
	return
}

// Info returns the named client's registration, zero if unknown.
func Info(ecuName string) EcuInfo {
	if e, found := ecuMap[ecuName]; found {
		return *e
	}
	return EcuInfo{}
}
