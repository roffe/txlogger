package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/kwp2000"
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

	kwp := kwp2000.New(cl)
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
				rctx, rcancel := context.WithTimeout(ctx, 3*time.Second)
				resp, err := c.tb.Request(rctx, cmd.Command, cmd.Data, 'R')
				rcancel()
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

				rctx, rcancel := context.WithTimeout(ctx, 1*time.Second)
				resp, err := c.tb.Request(rctx, cmd.Command, cmd.Data, 'W', 'e')
				rcancel()
				if err != nil {
					write.Complete(err)
					continue
				}

				if resp.Command == 'e' {
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

					if fn, ok := router[va.Name]; ok && fn(va) {
						continue
					}

					ebus.Publish(va.Name, va.Float64())
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
