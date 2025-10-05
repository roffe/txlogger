package wbl

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
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

func New(ctx context.Context, cl *gocan.Client, cfg *WBLConfig) (LambdaProvider, error) {
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
	case zeitronix.ProductString:
		return newZeitronix(ctx, cl, cfg)
	default:
		return nil, fmt.Errorf("unknown WBL type: %s", cfg.WBLType)
	}
}

func newECUMaster(ctx context.Context, cl *gocan.Client) (LambdaProvider, error) {
	wblClient := ecumaster.NewLambdaToCAN(cl)
	if err := wblClient.Start(ctx); err != nil {
		return nil, err
	}
	return wblClient, nil
}

func newInnovate(ctx context.Context, cl *gocan.Client, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := innovate.NewISP2Client(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}
	if cfg.Txbridge {
		if err := cl.Send(gocan.SystemMsg, []byte{'w', 1, 'i', 'i'}, gocan.Outgoing); err != nil {
			return nil, err
		}
	}
	wblClient.Start(ctx)
	if cfg.Txbridge {
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)
		go func() {
			ch := wblSub.Chan()
			defer cfg.Log("wbl channel closed")
			for msg := range ch {
				wblClient.SetData(msg.Data)
			}

		}()
	}
	return wblClient, nil
}

func newAEM(ctx context.Context, cl *gocan.Client, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := aem.NewAEMuegoClient(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}
	switch cfg.Port {
	case "txbridge":
		cfg.Log("Starting AEM txbridge client")
		if err := cl.Send(gocan.SystemMsg, []byte{'w', 1, 'a', 'a'}, gocan.Outgoing); err != nil {
			return nil, err
		}
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)
		go func() {
			ch := wblSub.Chan()
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
		wblSub := cl.Subscribe(ctx, 0x180)
		go func() {
			// defer wblSub.Close()
			ch := wblSub.Chan()
			defer cfg.Log("wbl channel closed")
			for msg := range ch {
				// We should only get extended frames here
				if !msg.Extended {
					continue
				}
				wblClient.SetData(msg.Data)
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

func newPLX(ctx context.Context, cl *gocan.Client, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := plx.NewIMFDClient(cfg.Port, nil, cfg.Log)
	if err != nil {
		return nil, err
	}
	if cfg.Txbridge && cfg.Port == "txbridge" {
		if err := cl.Send(gocan.SystemMsg, []byte{'w', 1, 'p', 'p'}, gocan.Outgoing); err != nil {
			return nil, err
		}
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)

		go func() {
			ch := wblSub.Chan()
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

func newZeitronix(ctx context.Context, cl *gocan.Client, cfg *WBLConfig) (LambdaProvider, error) {
	wblClient, err := zeitronix.NewZeitronixClient(cfg.Port, cfg.Log)
	if err != nil {
		return nil, err
	}
	if cfg.Txbridge && cfg.Port == "txbridge" {
		if err := cl.Send(gocan.SystemMsg, []byte{'w', 1, 'z', 'z'}, gocan.Outgoing); err != nil {
			return nil, err
		}
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)

		go func() {
			ch := wblSub.Chan()
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
