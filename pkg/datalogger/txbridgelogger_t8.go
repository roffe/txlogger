package datalogger

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/ebus"
)

func (c *TxBridge) t8(pctx context.Context, cl *gocan.Bus) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	if c.lamb != nil {
		defer c.lamb.Stop()
	}

	channels := c.buildChannels()

	gm := gmlan.New(cl, 0x7e0, 0x7e8)

	if err := initT8Logging(ctx, gm, c.Symbols, c.OnMessage); err != nil {
		return fmt.Errorf("failed to init t8 logging: %w", err)
	}

	t := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer t.Stop()

	var expectedPayloadSize uint16
	for _, sym := range c.Symbols {
		expectedPayloadSize += sym.Length
	}
	lastPresent := time.Now()

	testerPresent := func() {
		if time.Since(lastPresent) > lastPresentInterval {
			if err := gm.TesterPresentNoResponseAllowed(); err != nil {
				c.onError()
				c.OnMessage("Failed to send tester present: " + err.Error())
			}
			lastPresent = time.Now()
		}
	}

	tx := c.tb.Subscribe(ctx, 'r')

	if err := c.startLogging(); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	go func() {
		defer cl.Close()

		adScanner := c.adScannerFunc()

		defer func() {
			_ = c.stopLogging() // stop the dongle's read loop before ending the session
			_ = gm.ReturnToNormalMode(ctx)
			time.Sleep(100 * time.Millisecond)
		}()

		lastData := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.quitChan:
				c.OnMessage("Finished logging")
				return
			case <-c.secondTicker.C:
				if msg := c.perSecond(lastData); msg != "" {
					c.OnMessage(msg)
					return
				}
			case read := <-c.readChan:
				c.handleReadTxbridge(ctx, read, 235, 4*time.Second)
			case upd := <-c.writeChan:
				log.Printf("Updating RAM 0x%X", upd.Address)
				c.handleWriteTxbridge(ctx, upd, 235, 1*time.Second)
			case msg, ok := <-tx:
				if !ok {
					c.OnMessage("txbridge recv channel closed")
					return
				}
				lastData = time.Now()
				if len(msg.Data) != int(expectedPayloadSize+4) {
					c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, len(msg.Data)))
					return
				}

				r, timeStamp, err := c.readFrameHeader(msg.Data)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}

				for _, va := range c.Symbols {
					if err := va.Read(r); err != nil {
						c.onError()
						c.OnMessage("failed to read symbol data: " + err.Error())
						break
					}
					ebus.Publish(va.Name, va.Float64())
					adScanner(va.Name, va.Int())
				}

				if r.Len() > 0 {
					c.OnMessage(fmt.Sprintf("%d leftover bytes!", r.Len()))
				}

				c.publishExternalWBL()

				if err := c.lw.Write(timeStamp, channels); err != nil {
					c.onError()
					c.OnMessage("failed to write log: " + err.Error())
				}
				c.onCapture(timeStamp)
				testerPresent()
			}
		}
	}()
	return cl.Wait(ctx)
}
