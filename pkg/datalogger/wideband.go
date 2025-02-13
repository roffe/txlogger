package datalogger

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
		return newECUMaster(ctx, cl)
	case innovate.ProductString:
		return newInnovate(ctx, cl, cfg)
	case aem.ProductString:
		return newAEM(ctx, cl, cfg)
	case plx.ProductString:
		return newPLX(ctx, cl, cfg)
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
		if err := cl.SendFrame(gocan.SystemMsg, []byte{'w', 1, 'i', 'i'}, gocan.Outgoing); err != nil {
			return nil, err
		}
	}
	wblClient.Start(ctx)
	if cfg.Txbridge {
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)
		go func() {
			ch := wblSub.Chan()
			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						cfg.Log("wbl channel closed")
						return
					}
					wblClient.SetData(msg.Data())
				}
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
	if cfg.Port == "txbridge" {
		cfg.Log("Starting AEM txbridge client")
		if err := cl.SendFrame(gocan.SystemMsg, []byte{'w', 1, 'a', 'a'}, gocan.Outgoing); err != nil {
			return nil, err
		}
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)
		go func() {
			ch := wblSub.Chan()
			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						cfg.Log("wbl reading channel closed")
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
			}
		}()
	} else if cfg.Port == "CAN" {
		cfg.Log("Starting AEM CAN client")
		wblSub := cl.Subscribe(ctx, 0x180)
		go func() {
			// defer wblSub.Close()
			ch := wblSub.Chan()
			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						cfg.Log("wbl channel closed")
						return
					}
					wblClient.SetData(msg.Data())
				}
			}
		}()
	} else {
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
	if cfg.Txbridge || cfg.Port == "txbridge" {
		if err := cl.SendFrame(gocan.SystemMsg, []byte{'w', 1, 'p', 'p'}, gocan.Outgoing); err != nil {
			return nil, err
		}
		wblSub := cl.Subscribe(ctx, gocan.SystemMsgWBLReading)

		go func() {
			ch := wblSub.Chan()
			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						cfg.Log("wbl channel closed")
						return
					}
					if err := wblClient.Parse(msg.Data()); err != nil {
						cfg.Log(err.Error())
						log.Println(err)
					}
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
