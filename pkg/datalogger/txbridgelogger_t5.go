package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ebus"
)

func (c *TxBridge) t5(pctx context.Context, cl *gocan.Bus) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	// T5 decodes every value into sysvars (see newT5Converter), so all columns
	// are sysvar channels.
	channels := make([]Channel, 0, len(c.Symbols)+2)
	for _, s := range c.Symbols {
		s.Correctionfactor = 0.1
		channels = append(channels, newSysvarChannel(c.sysvars, s.Name))
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
	}
	for _, name := range c.appendExtraSysvars(nil) {
		channels = append(channels, newSysvarChannel(c.sysvars, name))
	}

	expectedPayloadSize, err := c.configureT5Symbols()
	if err != nil {
		return fmt.Errorf("error configuring symbols: %w", err)
	}

	tx := c.tb.Subscribe(ctx, 'r')

	if err := c.startLogging(); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	converto := newT5Converter()
	adscannerConverter := NewWBLInterpolator(c.WidebandConfig)

	go func() {
		defer cl.Close()
		defer func() {
			_ = c.stopLogging() // stop the dongle's read loop before closing the connection
			time.Sleep(50 * time.Millisecond)
		}()
		lastData := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.quitChan:
				c.OnMessage("Stopped logging..")
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
				toWrite := min(128, write.Length)
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

				write.Data = write.Data[toWrite:] // remove the data we just sent
				write.Address += uint32(toWrite)
				write.Length -= toWrite

				rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
				resp, err := c.tb.Request(rctx, cmd.Command, cmd.Data, 'W', 'e')
				rcancel()
				if err != nil {
					write.Complete(err)
					continue
				}

				if resp.Command == 'e' {
					write.Complete(fmt.Errorf("error response: % 02X", resp.Data))
					continue
				}

				if write.Length > 0 {
					select {
					case c.writeChan <- write:
					default:
						log.Println("writeChan full")
					}
					continue
				}
				write.Complete(nil)
				continue
			case msg, ok := <-tx:
				if !ok {
					c.OnMessage("txbridge sub closed")
					return
				}
				lastData = time.Now()

				if len(msg.Data) != (expectedPayloadSize + 4) {
					c.onError()
					c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, len(msg.Data)))
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
						c.OnMessage("failed to read symbol " + sym.Name + ": " + err.Error())
						return
					}
					val := converto(sym.Name, sym.Bytes())
					if c.WidebandConfig.ADScanner && sym.Name == c.WidebandConfig.ADScannerSymbol {
						lambda := adscannerConverter(int(val))
						c.sysvars.Set(LAMBDAADSCANNER, lambda)
						ebus.Publish(LAMBDAADSCANNER, lambda)
					}
					c.sysvars.Set(sym.Name, val)
					ebus.Publish(sym.Name, val)
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

				if err := c.lw.Write(timeStamp, channels); err != nil {
					c.OnMessage("failed to write log: " + err.Error())
					return
				}
				c.onCapture(timeStamp)
			}
		}
	}()
	return cl.Wait(ctx)
}

// enableT5Gather uploads the gather stub + descriptor table to ECU SRAM (using the
// same A5 address-command + 7-byte index-frame write the bootloader uses) and then
// enables gather mode on the dongle. The table is built from c.Symbols (the same
// list sent via 'd'); the C4 'W' write can't be used here (it needs an ECU
// write-enable we never set), so we drive the raw CAN write ourselves while the
// dongle is idle and forwards the 0xC replies back to us.
//
// The stub/table layout (t5GatherStub, buildT5GatherImage) is shared with the
// generic-adapter fast logger in t5fastlogger.go.
func (c *TxBridge) enableT5Gather(ctx context.Context, cl *gocan.Bus) error {
	image, err := buildT5GatherImage(c.Symbols)
	if err != nil {
		return err
	}

	if err := c.uploadT5SRAM(ctx, cl, t5GatherStubAddr, image); err != nil {
		return fmt.Errorf("gather upload: %w", err)
	}
	if err := c.tb.Raw([]byte("g")); err != nil {
		return err
	}
	c.OnMessage(fmt.Sprintf("T5 gather enabled (%d symbols, %d byte image)", len(c.Symbols), len(image)))
	return nil
}

// uploadT5SRAM writes data into ECU SRAM via the stock A5 arm + 7-byte index frames
// (cf. pkg/ecu/t5/bootloader.go). Each index frame is acked on 0xC with
// [offset, 0x00]. Must run while the dongle is idle (pre-logging) so the 0xC
// replies are forwarded to the host.
//
// The index byte is the write offset within the armed block and MUST stay <= 0x7F
// (the ECU routes a first byte > 0x7F to the command table, not the write path),
// so chunk into blocks, re-arming at each block's base so the offset restarts at 0.
func (c *TxBridge) uploadT5SRAM(ctx context.Context, cl *gocan.Bus, address uint32, data []byte) error {
	const maxBlock = 112 // 16 frames, max offset 105 (0x69) — well under 0x7F
	for blkStart := 0; blkStart < len(data); blkStart += maxBlock {
		blkEnd := min(blkStart+maxBlock, len(data))
		block := data[blkStart:blkEnd]
		blkAddr := address + uint32(blkStart)

		// Retry only on a lost 0xC ack (timeout): index writes are absolute
		// (base+index) and re-arming engine-off is idempotent, so re-sending the
		// same frame is safe. A received-but-wrong reply is NOT retried — for A5
		// it means the arm was *rejected* (RPM-gated: engine running, or bad addr),
		// which also de-arms (CLR.B 0x3724); re-sending A5 can't beat the gate and
		// is exactly what must never happen on a live engine (see Trionic5.md).
		arm := []byte{0xA5, byte(blkAddr >> 24), byte(blkAddr >> 16), byte(blkAddr >> 8), byte(blkAddr), byte(len(block)), 0x00, 0x00}
		if err := retry.Do(func() error {
			rctx, rcancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer rcancel()
			resp, err := cl.Request(rctx, gocan.NewFrame(0x5, arm), 0xC)
			if err != nil {
				return err
			}
			if resp.Length < 2 || resp.Data[0] != 0xA5 || resp.Data[1] != 0x00 {
				return retry.Unrecoverable(fmt.Errorf("rejected (engine must be off to arm): % 02X", resp.Data))
			}
			return nil
		}, retry.Context(ctx), retry.LastErrorOnly(true), retry.Attempts(3)); err != nil {
			return fmt.Errorf("arm @%X: %w", blkAddr, err)
		}

		for off := 0; off < len(block); off += 7 {
			frame := make([]byte, 8)
			frame[0] = byte(off)
			for i := 0; i < 7 && off+i < len(block); i++ {
				frame[1+i] = block[off+i]
			}
			if err := retry.Do(func() error {
				rctx, rcancel := context.WithTimeout(ctx, 250*time.Millisecond)
				defer rcancel()
				resp, err := cl.Request(rctx, gocan.NewFrame(0x5, frame), 0xC)
				if err != nil {
					return err
				}
				if resp.Length < 2 || resp.Data[0] != byte(off) || resp.Data[1] != 0x00 {
					return retry.Unrecoverable(fmt.Errorf("rejected: % 02X", resp.Data))
				}
				return nil
			}, retry.Context(ctx), retry.LastErrorOnly(true), retry.Attempts(3)); err != nil {
				return fmt.Errorf("data @%X+%d: %w", blkAddr, off, err)
			}
		}
	}
	return nil
}

// t5new is a test variant of t5 that enables the gather fast-logger. Identical to
// t5 except for the enableT5Gather call before startLogging. Flip the dispatch in
// txbridgelogger.go (c.t5 -> c.t5new) to try it; merge into t5 once validated.
func (c *TxBridge) t5new(pctx context.Context, cl *gocan.Bus) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	channels := make([]Channel, 0, len(c.Symbols)+2)
	for _, s := range c.Symbols {
		s.Correctionfactor = 0.1
		channels = append(channels, newSysvarChannel(c.sysvars, s.Name))
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
	}
	for _, name := range c.appendExtraSysvars(nil) {
		channels = append(channels, newSysvarChannel(c.sysvars, name))
	}

	expectedPayloadSize, err := c.configureT5Symbols() // configure the symbol list in the dongle and get the expected payload size
	if err != nil {
		return fmt.Errorf("error configuring symbols: %w", err)
	}

	// --- the only difference from t5(): build+upload the gather stub/table and enable.
	if err := c.enableT5Gather(ctx, cl); err != nil {
		return fmt.Errorf("error enabling gather: %w", err)
	}

	tx := c.tb.Subscribe(ctx, 'r')

	if err := c.startLogging(); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	converto := newT5Converter()
	adscannerConverter := NewWBLInterpolator(c.WidebandConfig)

	go func() {
		defer cl.Close()
		defer func() {
			_ = c.stopLogging()
			time.Sleep(50 * time.Millisecond)
		}()
		lastData := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.quitChan:
				c.OnMessage("Stopped logging..")
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
				toWrite := min(128, write.Length)
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

				write.Data = write.Data[toWrite:]
				write.Address += uint32(toWrite)
				write.Length -= toWrite

				rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
				resp, err := c.tb.Request(rctx, cmd.Command, cmd.Data, 'W', 'e')
				rcancel()
				if err != nil {
					write.Complete(err)
					continue
				}

				if resp.Command == 'e' {
					write.Complete(fmt.Errorf("error response: % 02X", resp.Data))
					continue
				}

				if write.Length > 0 {
					select {
					case c.writeChan <- write:
					default:
						log.Println("writeChan full")
					}
					continue
				}
				write.Complete(nil)
				continue
			case msg, ok := <-tx:
				if !ok {
					c.OnMessage("txbridge sub closed")
					return
				}
				lastData = time.Now()

				if len(msg.Data) != (expectedPayloadSize + 4) {
					c.onError()
					c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, len(msg.Data)))
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
						c.OnMessage("failed to read symbol " + sym.Name + ": " + err.Error())
						return
					}
					val := converto(sym.Name, sym.Bytes())
					if c.WidebandConfig.ADScanner && sym.Name == c.WidebandConfig.ADScannerSymbol {
						lambda := adscannerConverter(int(val))
						c.sysvars.Set(LAMBDAADSCANNER, lambda)
						ebus.Publish(LAMBDAADSCANNER, lambda)
					}
					c.sysvars.Set(sym.Name, val)
					ebus.Publish(sym.Name, val)
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

				if err := c.lw.Write(timeStamp, channels); err != nil {
					c.OnMessage("failed to write log: " + err.Error())
					return
				}
				c.onCapture(timeStamp)
			}
		}
	}()
	return cl.Wait(ctx)
}

func (c *TxBridge) configureT5Symbols() (int, error) {
	var expectedPayloadSize uint16
	var symbollist []byte
	for _, sym := range c.Symbols {
		symbollist = binary.LittleEndian.AppendUint32(symbollist, sym.SramOffset)
		symbollist = binary.LittleEndian.AppendUint16(symbollist, sym.Length)
		expectedPayloadSize += sym.Length
		// deletelog.Printf("Symbol: %s, offset: %X, length: %d\n", sym.Name, sym.SramOffset, sym.Length)
	}
	if err := c.tb.Command('d', symbollist); err != nil {
		return -1, err
	}
	c.OnMessage("Symbol list configured")
	return int(expectedPayloadSize), nil
}

func (c *TxBridge) calculateExpectedPayloadSize() int {
	var expectedPayloadSize uint16
	for _, sym := range c.Symbols {
		expectedPayloadSize += sym.Length
	}
	return int(expectedPayloadSize)
}
