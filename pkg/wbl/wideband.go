package wbl

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
)

type LambdaProvider interface {
	GetLambda() float64
	Start(context.Context) error
	Stop()
	String() string
}

type WBLConfig struct {
	WBLType  string
	Port     string
	Log      func(string)
	Txbridge bool
}

// txbridge WBL config and readings travel as the dongle's 'w' serial
// commands, reached through the native txbridge adapter's methods.
func New(ctx context.Context, cl *gocan.Bus, cfg *WBLConfig) (LambdaProvider, error) {
	if cfg.Log == nil {
		cfg.Log = func(str string) {
			log.Println(str)
		}
	}

	if cfg.WBLType == "ECU" || cfg.WBLType == "None" {
		return nil, nil
	}

	if !cfg.Txbridge && cfg.Port == "txbridge" {
		cfg.Log("please correct your WBL configuration. txbridge port is selected but not using txbridge adapter")
		return nil, nil
	}

	switch cfg.WBLType {
	case ecumaster.ProductString:
		return newECUMaster(ctx, cl)
	case innovate.ProductString:
		return newInnovate(ctx, cl, cfg)
	case aem.ProductString:
		return newAEM(ctx, cl, cfg)
	case plx.ProductString:
		return newPLX(ctx, cl, cfg)
	case stag.ProductString:
		return newSTAG(ctx, cfg)
	case zeitronix.ProductString:
		return newZeitronix(ctx, cl, cfg)
	default:
		return nil, fmt.Errorf("unknown WBL type: %s", cfg.WBLType)
	}
}

// txbridgeAdapter is the slice of the native txbridge adapter's command
// surface the WBL readers need.
type txbridgeAdapter interface {
	Command(cmd byte, data []byte) error
	Subscribe(ctx context.Context, cmds ...byte) <-chan *serialcommand.SerialCommand
}

// txbridgeWBL arms the dongle's wideband reader (command 'w' <type>) and
// returns the reading subscription.
func txbridgeWBL(ctx context.Context, cl *gocan.Bus, wblType byte) (<-chan *serialcommand.SerialCommand, error) {
	tb, ok := cl.Adapter().(txbridgeAdapter)
	if !ok {
		return nil, fmt.Errorf("txbridge WBL needs the txbridge adapter")
	}
	if err := tb.Command('w', []byte{wblType}); err != nil {
		return nil, err
	}
	return tb.Subscribe(ctx, 'w'), nil
}

func newECUMaster(ctx context.Context, cl *gocan.Bus) (LambdaProvider, error) {
	wblClient := ecumaster.NewLambdaToCAN(cl)
	if err := wblClient.Start(ctx); err != nil {
		return nil, err
	}
	return wblClient, nil
}

func newInnovate(ctx context.Context, cl *gocan.Bus, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := innovate.NewISP2Client(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}
	var ch <-chan *serialcommand.SerialCommand
	if cfg.Txbridge {
		var err error
		if ch, err = txbridgeWBL(ctx, cl, 'i'); err != nil {
			return nil, err
		}
	}
	wblClient.Start(ctx)
	if cfg.Txbridge {
		go func() {
			defer cfg.Log("wbl channel closed")
			for msg := range ch {
				wblClient.SetData(msg.Data)
			}
		}()
	}
	return wblClient, nil
}

func newAEM(ctx context.Context, cl *gocan.Bus, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := aem.NewAEMuegoClient(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}
	switch cfg.Port {
	case "txbridge":
		cfg.Log("Starting AEM txbridge client")
		ch, err := txbridgeWBL(ctx, cl, 'a')
		if err != nil {
			return nil, err
		}
		go func() {
			defer cfg.Log("wbl reading channel closed")
			for msg := range ch {
				// create a float from the message
				f, err := strconv.ParseFloat(string(msg.Data), 64)
				if err != nil {
					cfg.Log("could not decode WBL value")
					continue
				}
				//lambda := float64(binary.BigEndian.Uint16(msg.Data()[0:2])) / 100
				wblClient.SetLambda(f / 10)
			}

		}()
	case "CAN":
		cfg.Log("Starting AEM CAN client")
		ch := cl.Subscribe(ctx, 0x180)
		go func() {
			defer cfg.Log("wbl channel closed")
			for msg := range ch {
				// We should only get extended frames here
				if !msg.Extended {
					continue
				}
				wblClient.SetData(msg.Bytes())
			}
		}()
	default:
		cfg.Log("Starting AEM serial client")
		if err := wblClient.Start(ctx); err != nil {
			return nil, err
		}
	}
	return wblClient, nil
}

func newPLX(ctx context.Context, cl *gocan.Bus, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := plx.NewIMFDClient(cfg.Port, nil, cfg.Log)
	if err != nil {
		return nil, err
	}
	if cfg.Txbridge && cfg.Port == "txbridge" {
		ch, err := txbridgeWBL(ctx, cl, 'p')
		if err != nil {
			return nil, err
		}
		go func() {
			defer cfg.Log("wbl channel closed")
			for msg := range ch {
				if err := wblClient.Parse(msg.Data); err != nil {
					cfg.Log(err.Error())
				}
			}
		}()
	} else {
		if err := wblClient.Start(ctx); err != nil {
			return nil, err
		}
	}
	return wblClient, nil
}

func newZeitronix(ctx context.Context, cl *gocan.Bus, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := zeitronix.NewZeitronixClient(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}
	if cfg.Txbridge && cfg.Port == "txbridge" {
		ch, err := txbridgeWBL(ctx, cl, 'z')
		if err != nil {
			return nil, err
		}
		go func() {
			defer cfg.Log("wbl channel closed")
			for msg := range ch {
				if err := wblClient.SetData(msg.Data); err != nil {
					cfg.Log(err.Error())
				}
			}
		}()
	} else {
		if err := wblClient.Start(ctx); err != nil {
			return nil, err
		}
	}
	return wblClient, nil
}

func newSTAG(ctx context.Context, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := stag.NewSTAGClient(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}

	cfg.Log("Starting STAG serial client")
	if err := wblClient.Start(ctx); err != nil {
		return nil, err
	}

	return wblClient, nil
}
