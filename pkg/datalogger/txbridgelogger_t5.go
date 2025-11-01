package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/t5can"
)

func (c *TxBridge) t5(pctx context.Context, cl *gocan.Client) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	order := make([]string, len(c.Symbols))
	for n, s := range c.Symbols {
		order[n] = s.Name
		s.Correctionfactor = 0.1
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	t5 := t5can.NewClient(cl)
	expectedPayloadSize, err := c.configureT5Symbols(cl)
	if err != nil {
		return fmt.Errorf("error configuring symbols: %w", err)
	}

	tx := cl.Subscribe(ctx, gocan.SystemMsgDataResponse)
	defer tx.Close()

	messages := cl.Subscribe(ctx, gocan.SystemMsg)
	defer messages.Close()

	if err := c.startLogging(cl); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	converto := newT5Converter(c.WidebandConfig)

	go func() {
		if err := cl.Wait(); err != nil {
			c.OnMessage(err.Error())
			cancel()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-messages.Chan():
			c.OnMessage(string(msg.Data))
		case <-c.quitChan:
			c.OnMessage("Stopped logging..")
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
			toRead := min(234, read.Length)
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
			resp, err := cl.SendAndWait(ctx, frame, 300*time.Millisecond, gocan.SystemMsgDataRequest)
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
		case upd := <-c.writeChan:
			if err := t5.WriteRam(ctx, upd.Address, upd.Data); err != nil {
				upd.Complete(err)
				break
			}
			upd.Complete(nil)
		case msg, ok := <-tx.Chan():
			if !ok {
				return errors.New("txbridge sub closed")
			}

			if msg.Length() != int(expectedPayloadSize+4) {
				c.onError()
				c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, msg.Length()))
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

			for _, sym := range c.Symbols {
				if err := sym.Read(r); err != nil {
					return err
				}
				val := converto(sym.Name, sym.Bytes())
				c.sysvars.Set(sym.Name, val)
				if err := ebus.Publish(sym.Name, val); err != nil {
					c.onError()
					c.OnMessage(err.Error())
				}
			}

			if c.lamb != nil {
				lambda := c.lamb.GetLambda()
				c.sysvars.Set(EXTERNALWBLSYM, lambda)
				if err := ebus.Publish(EXTERNALWBLSYM, lambda); err != nil {
					c.onError()
					c.OnMessage(err.Error())
				}
			}

			if err := c.lw.Write(c.sysvars, []*symbol.Symbol{}, timeStamp, order); err != nil {
				return err
			}

			c.captureCount++
			c.cps++
			if c.captureCount%15 == 0 {
				c.CaptureCounter(c.captureCount)
			}
		}
	}
}

func (c *TxBridge) configureT5Symbols(cl *gocan.Client) (int, error) {
	var expectedPayloadSize uint16
	var symbollist []byte
	for _, sym := range c.Symbols {
		symbollist = binary.LittleEndian.AppendUint32(symbollist, sym.SramOffset)
		symbollist = binary.LittleEndian.AppendUint16(symbollist, sym.Length)
		expectedPayloadSize += sym.Length
		// deletelog.Printf("Symbol: %s, offset: %X, length: %d\n", sym.Name, sym.SramOffset, sym.Length)
	}
	cmd := &serialcommand.SerialCommand{
		Command: 'd',
		Data:    symbollist,
	}
	payload, err := cmd.MarshalBinary()
	if err != nil {
		return -1, err
	}
	if err := cl.Send(gocan.SystemMsg, payload, gocan.Outgoing); err != nil {
		return -1, err
	}
	c.OnMessage("Symbol list configured")
	return int(expectedPayloadSize), nil
}
