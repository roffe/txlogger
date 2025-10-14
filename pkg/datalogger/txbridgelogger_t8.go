package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/gocan/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/ebus"
	"golang.org/x/sync/errgroup"
)

func (c *TxBridge) t8(ctx context.Context, cl *gocan.Client) error {
	order := c.sysvars.Keys()
	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	sort.StringSlice(order).Sort()

	gm := gmlan.New(cl, 0x7e0, 0x7e8)

	err := retry.Do(func() error {
		if err := initT8Logging(ctx, gm, c.Symbols, c.OnMessage); err != nil {
			return fmt.Errorf("failed to init t8 logging: %w", err)
		}
		defer gm.ReturnToNormalMode(ctx)

		t := time.NewTicker(time.Second / time.Duration(c.Rate))
		defer t.Stop()

		errg, gctx := errgroup.WithContext(ctx)

		errg.Go(func() error {
			var expectedPayloadSize uint16
			for _, sym := range c.Symbols {
				expectedPayloadSize += sym.Length
			}
			lastPresent := time.Now()

			testerPresent := func() {
				if time.Since(lastPresent) > lastPresentInterval {
					//log.Println("sending tester present")
					if err := gm.TesterPresentNoResponseAllowed(); err != nil {
						c.onError()
						c.OnMessage("Failed to send tester present: " + err.Error())
					}
					lastPresent = time.Now()
				}
			}

			tx := cl.Subscribe(ctx, gocan.SystemMsgDataResponse)
			defer tx.Close()

			messages := cl.Subscribe(ctx, gocan.SystemMsg)
			defer messages.Close()

			if err := c.startLogging(cl); err != nil {
				return fmt.Errorf("error starting logging: %w", err)
			}

			for {
				select {
				case msg := <-messages.Chan():
					c.OnMessage(string(msg.Data))
				case err := <-cl.Err():
					if gocan.IsRecoverable(err) {
						c.onError()
						c.OnMessage(err.Error())
						continue
					}
					return retry.Unrecoverable(err)
				case <-c.quitChan:
					c.OnMessage("Finished logging")
					return nil
				case <-gctx.Done():
					log.Println("gctx done")
					return nil
				case <-c.secondTicker.C:
					c.FpsCounter(c.cps)
					c.cps = 0
					if c.errPerSecond > 10 {
						return errors.New("too many errors, reconnecting")
					}
					c.errPerSecond = 0
					continue
				case read := <-c.readChan:
					if err := c.handleReadTxbridge(ctx, cl, read); err != nil {
						read.Complete(err)
					}
					continue
				case upd := <-c.writeChan:
					log.Printf("Updating RAM 0x%X", upd.Address)
					if err := c.handleWriteTxbridge(ctx, cl, upd); err != nil {
						upd.Complete(err)
					}
					continue
				case msg, ok := <-tx.Chan():
					if !ok {
						return retry.Unrecoverable(errors.New("txbridge recv channel closed"))
					}

					if msg.Length() != int(expectedPayloadSize+4) {
						return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize+4, msg.Length()))
					}

					r := bytes.NewReader(msg.Data)
					binary.Read(r, binary.LittleEndian, &c.currtimestamp)

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
						if err := ebus.Publish(va.Name, va.Float64()); err != nil {
							c.onError()
							c.OnMessage("failed to publish data: " + err.Error())
						}
					}

					if r.Len() > 0 {
						c.OnMessage(fmt.Sprintf("%d leftover bytes!", r.Len()))
					}

					if c.lamb != nil {
						lambda := c.lamb.GetLambda()
						c.sysvars.Set(EXTERNALWBLSYM, lambda)
						if err := ebus.Publish(EXTERNALWBLSYM, lambda); err != nil {
							c.onError()
							c.OnMessage(err.Error())
						}
					}

					if err := c.lw.Write(c.sysvars, c.Symbols, timeStamp, order); err != nil {
						c.onError()
						c.OnMessage("failed to write log: " + err.Error())
					}
					c.cps++
					c.captureCount++
					if c.captureCount%15 == 0 {
						c.CaptureCounter(c.captureCount)
					}
					testerPresent()
				}
			}
		})
		return errg.Wait()
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(4),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
	)
	return err
}

func (c *TxBridge) handleReadTxbridge(ctx context.Context, cl *gocan.Client, read *DataRequest) error {
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
	payload, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}

	frame := gocan.NewFrame(gocan.SystemMsg, payload, gocan.Outgoing)
	resp, err := cl.SendAndWait(ctx, frame, 4*time.Second, gocan.SystemMsgDataRequest)
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

func (c *TxBridge) handleWriteTxbridge(ctx context.Context, cl *gocan.Client, write *DataRequest) error {
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

	payload, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}
	frame := gocan.NewFrame(gocan.SystemMsg, payload, gocan.Outgoing)
	resp, err := cl.SendAndWait(ctx, frame, 1*time.Second, gocan.SystemMsgWriteResponse, gocan.SystemMsgError)
	if err != nil {
		return err
	}
	if resp.Identifier == gocan.SystemMsgError {
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
