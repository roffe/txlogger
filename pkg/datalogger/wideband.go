package datalogger

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/txlogger/pkg/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
)

type WBLConfig struct {
	WBLType  string
	Port     string
	Log      func(string)
	Txbridge bool
}

func NewWBL(ctx context.Context, cl *gocan.Client, cfg *WBLConfig) (LambdaProvider, error) {
	if cfg.Log == nil {
		cfg.Log = func(str string) {
			log.Println(str)
		}
	}

	switch cfg.WBLType {
	case "ECU", "None":
		return nil, nil

	case ecumaster.ProductString:
		wblClient := ecumaster.NewLambdaToCAN(cl)
		if err := wblClient.Start(ctx); err != nil {
			return nil, err
		}
		return wblClient, nil

	case innovate.ProductString:
		wblClient, err := innovate.NewISP2Client(cfg.Port, cfg.Log)
		if err != nil {
			return nil, err
		}
		if cfg.Txbridge {
			if err := cl.SendFrame(adapter.SystemMsg, []byte{'w', 1, 'i', 'i'}, gocan.Outgoing); err != nil {
				return nil, err
			}
		}
		wblClient.Start(ctx)
		if cfg.Txbridge {
			wblSub := cl.Subscribe(ctx, adapter.SystemMsgWBLReading)
			//defer wblSub.Close()
			go func() {
				for msg := range wblSub.C() {
					wblClient.SetData(msg.Data())
				}
			}()
		}
		return wblClient, nil

	case aem.ProductString:
		wblClient, err := aem.NewAEMuegoClient(cfg.Port, cfg.Log)
		if err != nil {
			return nil, err
		}

		if cfg.Port == "txbridge" {
			cfg.Log("Starting AEM txbridge client")
			if err := cl.SendFrame(adapter.SystemMsg, []byte{'w', 1, 'a', 'a'}, gocan.Outgoing); err != nil {
				return nil, err
			}

			wblSub := cl.Subscribe(ctx, adapter.SystemMsgWBLReading)
			// defer wblSub.Close()
			go func() {
				for msg := range wblSub.C() {
					// create a float from the 2 first bytes in the message
					lambda := float64(binary.BigEndian.Uint16(msg.Data()[0:2])) / 100
					wblClient.SetLambda(lambda)
				}
			}()
		} else if cfg.Port == "CAN" {
			cfg.Log("Starting AEM CAN client")
			wblSub := cl.Subscribe(ctx, 0x180)
			go func() {
				// defer wblSub.Close()
				for msg := range wblSub.C() {
					wblClient.SetData(msg.Data())
				}
			}()
		} else {
			cfg.Log("Starting AEM serial client")
			if err := wblClient.Start(ctx); err != nil {
				return nil, err
			}
		}
		return wblClient, nil
	default:
		return nil, fmt.Errorf("unknown WBL type: %s", cfg.WBLType)
	}
}
