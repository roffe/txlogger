package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

func (c *TxBridge) t7(pctx context.Context, cl *gocan.Client) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	go t7broadcastListener(bctx, cl, c.sysvars)

	c.OnMessage("Watching for broadcast messages")
	<-time.After(1550 * time.Millisecond)
	found := c.sysvars.Keys()
	c.OnMessage(fmt.Sprintf("Found %s", found))

	if len(found) == 0 {
		bcancel()
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
	}

	for _, sym := range c.Symbols {
		if c.sysvars.Exists(sym.Name) {
			log.Println("Skipping", sym.Name, "in broadcast")
			sym.Number = -1
			continue
		}
	}

	channels := c.buildChannels()

	kwp := kwp2000.New(cl)
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

	tx := cl.Subscribe(ctx, gocan.SystemMsgDataResponse)
	defer tx.Close()

	if err := c.startLogging(cl); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	adConverter := NewWBLInterpolator(c.WidebandConfig)

	router := map[string]func(s *symbol.Symbol) bool{
		"IgnKnk.fi_Offset": func(s *symbol.Symbol) bool {
			data := s.Bytes()
			if len(data) != 8 {
				return false
			}

			ioffCyl1 := int16(binary.BigEndian.Uint16(data[0:2]))
			ioffCyl2 := int16(binary.BigEndian.Uint16(data[2:4]))
			ioffCyl3 := int16(binary.BigEndian.Uint16(data[4:6]))
			ioffCyl4 := int16(binary.BigEndian.Uint16(data[6:8]))

			ebus.Publish("IgnKnk.fi_Offset.Cyl1", float64(ioffCyl1)/10)
			ebus.Publish("IgnKnk.fi_Offset.Cyl2", float64(ioffCyl2)/10)
			ebus.Publish("IgnKnk.fi_Offset.Cyl3", float64(ioffCyl3)/10)
			ebus.Publish("IgnKnk.fi_Offset.Cyl4", float64(ioffCyl4)/10)
			return true
		},
	}

	if c.WidebandConfig.ADScanner {
		router[c.WidebandConfig.ADScannerSymbol] = func(s *symbol.Symbol) bool {
			lambda := adConverter(s.Int())
			c.sysvars.Set(LAMBDAADSCANNER, lambda)
			ebus.Publish(LAMBDAADSCANNER, lambda)
			return true
		}
	}

	go func() {
		defer cl.Close()
		defer func() {
			_ = kwp.StopSession(ctx)
			time.Sleep(75 * time.Millisecond)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.quitChan:
				c.OnMessage("Stop logging")
				return
			case <-c.secondTicker.C:
				c.FpsCounter(c.capturePerSecond)
				if c.errPerSecond > 5 {
					c.OnMessage("too many errors, aborting logging")
					return
				}
				c.resetPerSecond()
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
					c.OnMessage("txbridge recv channel closed")
					return
				}
				if msg.DLC() != int(expectedPayloadSize+4) {
					c.onError()
					c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, msg.DLC()))
					// log.Printf("unexpected data %X", msg.Data)
					continue
					// return fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff))
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
					if va.Number == -1 {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
						continue
					}
					if err := va.Read(r); err != nil {
						log.Printf("data ex %d %X len %d", expectedPayloadSize, msg.Data, msg.DLC())
						c.onError()
						c.OnMessage(err.Error())
						break
					}

					if fn, ok := router[va.Name]; ok && fn(va) {
						continue
					}

					ebus.Publish(va.Name, va.Float64())
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
			}
		}
	}()
	return cl.Wait(ctx)
}
