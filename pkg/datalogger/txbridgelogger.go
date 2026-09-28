package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
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
		}
		return c.t5(ctx, cl, c.ExperimentalT5FastLogging)
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

// perSecond runs the once-a-second bookkeeping of the txbridge loops and
// returns why the session should abort, or "" to keep logging.
func (c *TxBridge) perSecond(lastData time.Time) string {
	c.FpsCounter(c.capturePerSecond)
	if c.errPerSecond > 5 {
		return "too many errors, aborting logging"
	}
	if time.Since(lastData) > dataTimeout {
		return "no data for 5s, aborting logging"
	}
	c.resetPerSecond()
	return ""
}

// readFrameHeader consumes the dongle's little-endian ms timestamp from a log
// frame and returns the reader positioned at the symbol payload plus the frame's
// host-compensated time.
func (c *TxBridge) readFrameHeader(data []byte) (*bytes.Reader, time.Time, error) {
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.LittleEndian, &c.currtimestamp); err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read timestamp: %w", err)
	}
	if c.firstTime.IsZero() {
		c.firstTime = time.Now()
		c.firstTimestamp = c.currtimestamp
	}
	return r, c.calculateCompensatedTimestamp(), nil
}

// handleReadTxbridge serves one chunk (at most chunk bytes) of a RAM read via
// the dongle's 'R' command, re-queueing the request until it is complete so
// log frames interleave with long transfers.
func (c *TxBridge) handleReadTxbridge(ctx context.Context, read *DataRequest, chunk uint32, timeout time.Duration) {
	n := min(chunk, read.Length)
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := c.tb.Request(rctx, 'R', append(binary.LittleEndian.AppendUint32(nil, read.Address), byte(n)), 'R')
	if err != nil {
		read.Complete(err)
		return
	}
	read.Address += n
	read.Length -= n
	read.Data = append(read.Data, resp.Data...)
	if read.Length > 0 {
		requeue(c.readChan, read)
		return
	}
	read.Complete(nil)
}

// handleWriteTxbridge is the 'W' counterpart of handleReadTxbridge.
func (c *TxBridge) handleWriteTxbridge(ctx context.Context, write *DataRequest, chunk uint32, timeout time.Duration) {
	n := min(chunk, write.Length)
	data := append(binary.LittleEndian.AppendUint32(nil, write.Address), byte(n))
	data = append(data, write.Data[:n]...)
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := c.tb.Request(rctx, 'W', data, 'W', 'e')
	if err != nil {
		write.Complete(err)
		return
	}
	if resp.Command == 'e' {
		write.Complete(fmt.Errorf("error response: % 02X", resp.Data))
		return
	}
	write.Address += n
	write.Length -= n
	write.Data = write.Data[n:]
	if write.Length > 0 {
		requeue(c.writeChan, write)
		return
	}
	write.Complete(nil)
}

// requeue puts a partially served request back for its next chunk. The logger
// loop is the channel's only consumer, so fail the request rather than block it
// if a new request took the slot meanwhile.
func requeue(ch chan *DataRequest, req *DataRequest) {
	select {
	case ch <- req:
	default:
		req.Complete(errors.New("request queue full"))
	}
}
