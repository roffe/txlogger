package datalogger

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/ebus"
)

func (c *TxBridge) t7(pctx context.Context, cl *gocan.Bus) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	if c.lamb != nil {
		defer c.lamb.Stop()
	}

	// Tell the dongle to collect the T7 broadcast frames and fold them into the log
	// stream, then source those symbols from the folded trailer (decodeT7Broadcast)
	// instead of the KWP F0 read, rather than parsing a live broadcast flood here.
	c.OnMessage("Enabling T7 broadcast collection")
	if err := sendBroadcastCollect(c.tb, t7BroadcastIDs); err != nil {
		return fmt.Errorf("failed to set broadcast collect: %w", err)
	}
	for _, sym := range c.Symbols {
		if _, ok := t7BroadcastSymbols[sym.Name]; ok {
			log.Println("Skipping", sym.Name, "in broadcast")
			sym.Number = -1
		}
	}

	channels := c.buildChannels()

	kwp := t7kwp.New(cl)
	kwp.SetSeedKey(c.SeedKey) // custom pair extracted from the loaded binary, if any
	if err := initT7logging(ctx, kwp, c.Symbols, c.OnMessage); err != nil {
		return fmt.Errorf("failed to init t7 logging: %w", err)
	}

	var expectedPayloadSize uint16
	for _, sym := range c.Symbols {
		if sym.Number < 0 {
			continue
		}
		expectedPayloadSize += sym.Length
	}

	tx := c.tb.Subscribe(ctx, 'r')

	if err := c.startLogging(); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	publishSpecial := c.t7SpecialPublisher()

	go func() {
		defer cl.Close()
		defer func() {
			_ = c.stopLogging() // stop the dongle's read loop before ending the session
			_ = kwp.StopSession(ctx)
			time.Sleep(75 * time.Millisecond)
		}()
		lastData := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.quitChan:
				c.OnMessage("Stop logging")
				return
			case <-c.secondTicker.C:
				if msg := c.perSecond(lastData); msg != "" {
					c.OnMessage(msg)
					return
				}
			case read := <-c.readChan:
				c.handleReadTxbridge(ctx, read, 245, 3*time.Second)
			case write := <-c.writeChan:
				c.handleWriteTxbridge(ctx, write, 245, 1*time.Second)
			case msg, ok := <-tx:
				if !ok {
					c.OnMessage("txbridge recv channel closed")
					return
				}
				lastData = time.Now()
				// timestamp(4) + fixed symbol payload, then a variable broadcast trailer
				// the dongle folded in (see decodeT7Broadcast), so this is a lower bound.
				if len(msg.Data) < int(expectedPayloadSize+4) {
					c.onError()
					c.OnMessage(fmt.Sprintf("expected at least %d bytes, got %d", expectedPayloadSize+4, len(msg.Data)))
					// log.Printf("unexpected data %X", msg.Data)
					continue
				}

				r, timeStamp, err := c.readFrameHeader(msg.Data)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}

				// Read the fixed symbol payload first; broadcast (-1) symbols carry no
				// bytes here — they come from the trailer parsed just below.
				readErr := false
				for _, va := range c.Symbols {
					if va.Number == -1 {
						continue
					}
					if err := va.Read(r); err != nil {
						log.Printf("data ex %d %X len %d", expectedPayloadSize, msg.Data, len(msg.Data))
						c.onError()
						c.OnMessage(err.Error())
						readErr = true
						break
					}

					if !publishSpecial(va) {
						ebus.Publish(va.Name, va.Float64())
					}
				}
				if readErr {
					continue // r is misaligned; skip the trailer for this frame
				}

				// Drain the folded broadcast trailer: [idHi, idLo, dlc, data...] per
				// collected frame -> update sysvars via the shared T7 decoder.
				var bcbuf [8]byte
				for r.Len() >= 3 {
					idHi, _ := r.ReadByte()
					idLo, _ := r.ReadByte()
					dlc, _ := r.ReadByte()
					if dlc > 8 || r.Len() < int(dlc) {
						c.OnMessage("malformed broadcast trailer")
						break
					}
					if _, err := io.ReadFull(r, bcbuf[:dlc]); err != nil {
						break
					}
					decodeT7Broadcast(uint16(idHi)<<8|uint16(idLo), bcbuf[:dlc], c.sysvars)
				}

				// Publish the broadcast-sourced symbols from the freshly updated sysvars.
				for _, va := range c.Symbols {
					if va.Number == -1 {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
					}
				}

				c.publishExternalWBL()

				if err := c.lw.Write(timeStamp, channels); err != nil {
					c.onError()
					c.OnMessage("failed to write log: " + err.Error())
				}
				c.onCapture(timeStamp)
			}
		}
	}()
	return cl.Wait(ctx)
}
