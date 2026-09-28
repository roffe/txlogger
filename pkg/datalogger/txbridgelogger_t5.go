package datalogger

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan/v2"
)

// t5 logs via the dongle's autonomous T5 read loop. gather first uploads the
// gather stub/table (enableT5Gather) so the dongle reads one packed buffer.
func (c *TxBridge) t5(pctx context.Context, cl *gocan.Bus, gather bool) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	if c.lamb != nil {
		defer c.lamb.Stop()
	}
	channels := c.t5Channels()

	expectedPayloadSize, err := c.configureT5Symbols()
	if err != nil {
		return fmt.Errorf("error configuring symbols: %w", err)
	}

	if gather {
		if err := c.enableT5Gather(ctx, cl); err != nil {
			return fmt.Errorf("error enabling gather: %w", err)
		}
	}

	tx := c.tb.Subscribe(ctx, 'r')

	if err := c.startLogging(); err != nil {
		return fmt.Errorf("error starting logging: %w", err)
	}

	publish := c.t5Publisher()

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
				if msg := c.perSecond(lastData); msg != "" {
					c.OnMessage(msg)
					return
				}
			case read := <-c.readChan:
				c.handleReadTxbridge(ctx, read, 234, 3*time.Second)
			case write := <-c.writeChan:
				c.handleWriteTxbridge(ctx, write, 128, 5*time.Second)
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

				r, timeStamp, err := c.readFrameHeader(msg.Data)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}

				for _, sym := range c.Symbols {
					if err := sym.Read(r); err != nil {
						c.OnMessage("failed to read symbol " + sym.Name + ": " + err.Error())
						return
					}
					publish(sym)
				}
				c.publishExternalWBL()

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
