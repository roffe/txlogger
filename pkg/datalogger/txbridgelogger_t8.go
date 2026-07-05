package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan/v2/pkg/serialcommand"
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

		adConverter := NewWBLInterpolator(c.WidebandConfig)

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
				c.FpsCounter(c.capturePerSecond)
				if c.errPerSecond > 5 {
					c.OnMessage("too many errors, aborting logging")
					return
				}
				if time.Since(lastData) > dataTimeout {
					c.OnMessage("no data for 5s, aborting logging")
					return
				}
				c.resetPerSecond()
			case read := <-c.readChan:
				if err := c.handleReadTxbridge(ctx, read); err != nil {
					read.Complete(err)
				}
			case upd := <-c.writeChan:
				log.Printf("Updating RAM 0x%X", upd.Address)
				if err := c.handleWriteTxbridge(ctx, upd); err != nil {
					upd.Complete(err)
				}
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

				r := bytes.NewReader(msg.Data)
				if err := binary.Read(r, binary.LittleEndian, &c.currtimestamp); err != nil {
					c.onError()
					c.OnMessage("failed to read timestamp: " + err.Error())
					continue
				}

				if c.firstTime.IsZero() {
					c.firstTime = time.Now()
					c.firstTimestamp = c.currtimestamp
				}

				timeStamp := c.calculateCompensatedTimestamp()

				for _, va := range c.Symbols {
					if err := va.Read(r); err != nil {
						c.onError()
						c.OnMessage("failed to read symbol data: " + err.Error())
						break
					}
					ebus.Publish(va.Name, va.Float64())

					if c.WidebandConfig.ADScanner && va.Name == c.WidebandConfig.ADScannerSymbol {
						lambda := adConverter(va.Int())
						c.sysvars.Set(LAMBDAADSCANNER, lambda)
						ebus.Publish(LAMBDAADSCANNER, lambda)
					}
				}

				if r.Len() > 0 {
					c.OnMessage(fmt.Sprintf("%d leftover bytes!", r.Len()))
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

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

func (c *TxBridge) handleReadTxbridge(ctx context.Context, read *DataRequest) error {
	toRead := min(235, read.Length)
	// log.Printf("Reading RAM $%X:%d", read.Address, toRead)
	cmd := serialcommand.SerialCommand{
		Command: 'R',
		Data: []byte{
			byte(read.Address),
			byte(read.Address >> 8),
			byte(read.Address >> 16),
			byte(read.Address >> 24),
			byte(toRead),
		},
	}
	rctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	resp, err := c.tb.Request(rctx, cmd.Command, cmd.Data, 'R')
	if err != nil {
		return err
	}
	read.Address += uint32(toRead)
	read.Length -= toRead
	read.Data = append(read.Data, resp.Data...)
	if read.Length > 0 {
		c.readChan <- read
	} else {
		read.Complete(nil)
	}
	return nil
}

func (c *TxBridge) handleWriteTxbridge(ctx context.Context, write *DataRequest) error {
	toWrite := min(write.Length, 235)
	// log.Printf("Writing RAM $%X:%d", write.Address, toWrite)
	cmd := serialcommand.SerialCommand{
		Command: 'W',
		Data: []byte{
			byte(write.Address),
			byte(write.Address >> 8),
			byte(write.Address >> 16),
			byte(write.Address >> 24),
			byte(toWrite),
		},
	}

	cmd.Data = append(cmd.Data, write.Data[:toWrite]...)

	rctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	resp, err := c.tb.Request(rctx, cmd.Command, cmd.Data, 'W', 'e')
	if err != nil {
		return err
	}
	if resp.Command == 'e' {
		return fmt.Errorf("error: %X", resp.Data)
	}
	write.Address += uint32(toWrite)
	write.Length -= toWrite
	write.Data = write.Data[toWrite:]

	if write.Length > 0 {
		c.writeChan <- write
	} else {
		write.Complete(nil)
	}
	return nil
}
