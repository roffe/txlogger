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
	"github.com/roffe/gocan/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

func (c *TxBridge) t7(ctx context.Context, cl *gocan.Client) error {
	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	go t7broadcastListener(bctx, cl, c.sysvars, c.quitChan)

	c.OnMessage("Watching for broadcast messages")
	<-time.After(550 * time.Millisecond)
	order := c.sysvars.Keys()
	sort.StringSlice(order).Sort()
	c.OnMessage(fmt.Sprintf("Found %s", order))

	if len(order) == 0 {
		bcancel()
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	for _, sym := range c.Symbols {
		if c.sysvars.Exists(sym.Name) {
			// log.Println("Skipping", sym.Name)
			sym.Number = -1
		}
		if sym.Number < 1000 {
			order = append(order, sym.Name)
		}
	}

	kwp := kwp2000.New(cl)
	err := retry.Do(func() error {
		if err := initT7logging(ctx, kwp, c.Symbols, c.OnMessage); err != nil {
			return fmt.Errorf("failed to init t7 logging: %w", err)
		}
		defer func() {
			kwp.StopSession(ctx)
			time.Sleep(50 * time.Millisecond)
		}()

		var expectedPayloadSize uint16
		for _, sym := range c.Symbols {
			if sym.Number < 0 {
				continue
			}
			expectedPayloadSize += sym.Length
		}

		tx := cl.Subscribe(ctx, gocan.SystemMsgDataResponse)
		defer tx.Close()

		messages := cl.Subscribe(ctx, gocan.SystemMsg)
		defer messages.Close()

		if err := c.startLogging(cl); err != nil {
			return fmt.Errorf("error starting logging: %w", err)
		}

		adConverter := newDisplProtADConverterT7(c.WidebandConfig)

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
				c.OnMessage("Stop logging")
				return nil
			case <-c.secondTicker.C:
				c.FpsCounter(c.cps)
				if c.errPerSecond > 5 {
					c.errPerSecond = 0
					return fmt.Errorf("too many errors per second")
				}
				c.cps = 0
				c.errPerSecond = 0
			case read := <-c.readChan:
				toRead := min(245, read.Length)
				read.Length -= toRead
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
				read.Address += uint32(toRead)
				payload, err := cmd.MarshalBinary()
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}
				frame := gocan.NewFrame(gocan.SystemMsg, payload, gocan.Outgoing)
				resp, err := cl.SendAndWait(ctx, frame, 3*time.Second, gocan.SystemMsgDataRequest)
				if err != nil {
					read.Complete(err)
					continue
				}
				read.Data = append(read.Data, resp.Data...)
				if read.Length > 0 {
					c.readChan <- read
				} else {
					read.Complete(nil)
				}
				continue
			case write := <-c.writeChan:
				toRead := min(245, write.Length)
				cmd := serialcommand.SerialCommand{
					Command: 'W',
					Data: []byte{
						byte(write.Address),
						byte(write.Address >> 8),
						byte(write.Address >> 16),
						byte(write.Address >> 24),
						byte(toRead),
					},
				}
				cmd.Data = append(cmd.Data, write.Data[:toRead]...)

				write.Data = write.Data[toRead:] // remove the data we just sent
				write.Address += uint32(toRead)
				write.Length -= toRead

				payload, err := cmd.MarshalBinary()
				if err != nil {
					write.Complete(err)
					continue
				}

				frame := gocan.NewFrame(gocan.SystemMsg, payload, gocan.Outgoing)

				resp, err := cl.SendAndWait(ctx, frame, 1*time.Second, gocan.SystemMsgWriteResponse)
				if err != nil {
					write.Complete(err)
					continue
				}

				if resp.Identifier == gocan.SystemMsgError {
					write.Complete(fmt.Errorf("error response"))
					continue
				}

				if write.Length > 0 {
					select {
					case c.writeChan <- write:
					default:
						log.Println("kisskorv updateChan full")
					}
					continue
				}
				write.Complete(nil)
				continue
			case msg, ok := <-tx.Chan():
				if !ok {
					return retry.Unrecoverable(errors.New("txbridge recv channel closed"))
				}
				if msg.Length() != int(expectedPayloadSize+4) {
					c.onError()
					c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, msg.Length()))
					//log.Printf("unexpected data %X", msg.Data)
					continue
					//return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
				}

				r := bytes.NewReader(msg.Data)

				binary.Read(r, binary.LittleEndian, &c.currtimestamp)

				if c.firstTime.IsZero() {
					c.firstTime = time.Now()
					c.firstTimestamp = c.currtimestamp
				}

				timeStamp := c.calculateCompensatedTimestamp()

				for _, va := range c.Symbols {
					if va.Number == -1 {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
						continue
					}
					if err := va.Read(r); err != nil {
						log.Printf("data ex %d %X len %d", expectedPayloadSize, msg.Data, msg.Length())
						c.onError()
						c.OnMessage(err.Error())
						break
					}
					if va.Name == "DisplProt.AD_Scanner" {
						//value := va.Float64()
						//voltage := (value / 1023) * (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
						//voltage = clamp(voltage, c.WidebandConfig.MinimumVoltageWideband, c.WidebandConfig.MaximumVoltageWideband)
						//steepness := (c.WidebandConfig.High - c.WidebandConfig.Low) / (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
						//result := c.WidebandConfig.Low + (steepness * (voltage - c.WidebandConfig.MinimumVoltageWideband))
						ebus.Publish(va.Name, adConverter(va.Float64()))
						continue
					}

					if err := ebus.Publish(va.Name, va.Float64()); err != nil {
						c.onError()
						c.OnMessage(err.Error())
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
				c.captureCount++
				c.cps++
				if c.captureCount%15 == 0 {
					c.CaptureCounter(c.captureCount)
				}
			}
		}
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(3),
		retry.OnRetry(func(n uint, err error) {
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	return err
}
