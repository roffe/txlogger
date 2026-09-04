package datalogger

import (
	"context"
	"errors"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/debug"
)

// dataTimeout aborts a txbridge logging session if no log frame arrives for
// this long. The txbridge loggers wait passively on the autonomous stream, so
// without this a dead stream would hang the session forever instead of erroring.
const dataTimeout = 5 * time.Second

var _ IClient = (*TxBridge)(nil)

// txbridgeAdapter is the host-side command surface of the native txbridge
// adapter (gocan/v2/adapters/txbridge): framed serial commands next to the
// regular CAN traffic.
type txbridgeAdapter interface {
	Command(cmd byte, data []byte) error
	Raw(data []byte) error
	Subscribe(ctx context.Context, cmds ...byte) <-chan *serialcommand.SerialCommand
	Request(ctx context.Context, cmd byte, data []byte, reply ...byte) (*serialcommand.SerialCommand, error)
}

type TxBridge struct {
	*BaseLogger
	tb txbridgeAdapter
}

func NewTxbridge(cfg Config, lw LogWriter) (*TxBridge, error) {
	return &TxBridge{
		BaseLogger: NewBaseLogger(cfg, lw),
	}, nil
}

func (c *TxBridge) Start(ctx context.Context) error {
	c.ErrorCounter(0)
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	eventHandler := func(e gocan.Event) {
		c.OnMessage(e.String())
		if e.Type == gocan.EventTypeError {
			c.onError()
		}
	}

	cl, err := gocan.OpenAdapter(ctx, c.Device, gocan.WithEventFunc(eventHandler))
	if err != nil {
		return err
	}
	defer cl.Close()

	tb, ok := cl.Adapter().(txbridgeAdapter)
	if !ok {
		return errors.New("txbridge logging needs the txbridge adapter")
	}
	c.tb = tb

	// Drive everything below (incl. the per-ECU loops, which derive their ctx
	// from this one) off the client's context so a fatal adapter error or Close
	// cancels logging and aborts in-flight requests directly.
	ctx = cl.Context()

	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	switch c.ECU {
	case "T5":
		if err := c.setECU("5"); err != nil {
			return err
		}
		if c.ExperimentalT5FastLogging {
			debug.Log("Using experimental T5 fast logger")
			return c.t5new(ctx, cl)
		}
		return c.t5(ctx, cl)
	case "T7":
		if err := c.setECU("7"); err != nil {
			return err
		}
		return c.t7(ctx, cl)
	case "T8":
		if err := c.setECU("8"); err != nil {
			return err
		}
		return c.t8(ctx, cl)
	default:
		return errors.New("unknown ECU type: " + c.ECU)
	}
}

func (c *TxBridge) setECU(ecuType string) error {
	if err := c.tb.Raw([]byte(ecuType)); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	// Setting the ECU above applies a per-ECU default delay. Override it with the
	// configured rate now: delayTime is the firmware's ms between reads = 1000/Hz.
	// Framed command: 'D' <len=1> <delay> <checksum=delay>.
	if c.Rate > 0 {
		delay := 1000 / c.Rate
		if delay < 1 {
			delay = 1
		} else if delay > 255 {
			delay = 255
		}
		if err := c.tb.Command('D', []byte{byte(delay)}); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// sendBroadcastCollect tells the dongle which CAN broadcast ids to cache and fold
// into the 'r' log responses (empty clears). The per-ECU id lists live with each
// ECU's broadcast decoder (e.g. t7BroadcastIDs).
func sendBroadcastCollect(tb txbridgeAdapter, ids []uint16) error {
	data := make([]byte, 0, len(ids)*2)
	for _, id := range ids {
		data = append(data, byte(id), byte(id>>8)) // 11-bit ids, little-endian
	}
	return tb.Command('b', data)
}

func (c *TxBridge) startLogging() error {
	return c.tb.Raw([]byte("r"))
}

// stopLogging tells the dongle to stop its autonomous read loop. Send this before
// ending the ECU session (StopSession / ReturnToNormalMode): otherwise the dongle
// keeps issuing reads against an ended session and work() logs a spurious timeout.
func (c *TxBridge) stopLogging() error {
	return c.tb.Raw([]byte("s"))
}
