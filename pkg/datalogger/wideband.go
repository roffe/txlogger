package datalogger

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
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
					if msg == nil {
						cfg.Log("wbl nil message")
						return
					}
					// create a float from the message
					f, err := strconv.ParseFloat(string(msg.Data()), 64)
					if err != nil {
						cfg.Log("could not decode WBL value")
						continue
					}

					//lambda := float64(binary.BigEndian.Uint16(msg.Data()[0:2])) / 100
					wblClient.SetLambda(f / 10)
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

	case plx.ProductString:
		wblClient, err := plx.NewIMFDClient(cfg.Port, nil, cfg.Log)
		if err != nil {
			return nil, err
		}
		if cfg.Txbridge {
			if err := cl.SendFrame(adapter.SystemMsg, []byte{'w', 1, 'p', 'p'}, gocan.Outgoing); err != nil {
				return nil, err
			}
			wblSub := cl.Subscribe(ctx, adapter.SystemMsgWBLReading)

			go func() {
				for msg := range wblSub.C() {
					if msg == nil {
						cfg.Log("wbl nil message")
						return
					}
					// log.Printf("plx: %X\n", msg.Data())
					if err := wblClient.Parse(msg.Data()); err != nil {
						cfg.Log(err.Error())
						log.Println(err)
					}
				}
			}()
		} else {
			if err := wblClient.Start(ctx); err != nil {
				return nil, err
			}
		}
		return wblClient, nil
	default:
		return nil, fmt.Errorf("unknown WBL type: %s", cfg.WBLType)
	}
}
