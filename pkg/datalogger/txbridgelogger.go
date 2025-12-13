package datalogger

import (
	"context"
	"errors"
	"time"

	"github.com/roffe/gocan"
)

var _ IClient = (*TxBridge)(nil)

type TxBridge struct {
	*BaseLogger
}

func NewTxbridge(cfg Config, lw LogWriter) (*TxBridge, error) {
	return &TxBridge{
		BaseLogger: NewBaseLogger(cfg, lw),
	}, nil
}

func (c *TxBridge) Start() error {
	c.ErrorCounter(0)
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	eventHandler := func(e gocan.Event) {
		c.OnMessage(e.String())
		if e.Type == gocan.EventTypeError {
			c.onError()
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := gocan.NewWithOpts(ctx, c.Device, gocan.WithEventHandler(eventHandler))
	if err != nil {
		return err
	}
	defer cl.Close()

	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	switch c.Config.ECU {
	case "T5":
		if err := c.setECU(cl, "5"); err != nil {
			return err
		}
		return c.t5(ctx, cl)
	case "T7":
		if err := c.setECU(cl, "7"); err != nil {
			return err
		}
		return c.t7(ctx, cl)
	case "T8":
		if err := c.setECU(cl, "8"); err != nil {
			return err
		}
		return c.t8(ctx, cl)
	default:
		return errors.New("unknown ECU type: " + c.Config.ECU)
	}
}

func (c *TxBridge) setECU(cl *gocan.Client, ecuType string) error {
	if err := cl.Send(gocan.SystemMsg, []byte(ecuType), gocan.Outgoing); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	return nil
}

func (c *TxBridge) startLogging(cl *gocan.Client) error {
	return cl.Send(gocan.SystemMsg, []byte("r"), gocan.Outgoing)
}
