package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/t5can"
)

// ---------------------------------------------------------------------------
// T5 gather fast-logger, generic-CAN-adapter variant.
//
// Same trick as the txbridge gather path (see txbridgelogger_t5.go and
// txbridge/t5_gather.s): upload a tiny CPU32 stub + descriptor table into the
// certified-free SRAM tail, then per cycle call the stub (C1) so it packs all
// logged symbols into one contiguous buffer at 0x7800, and read that back with
// packed 6-byte C7 reads instead of one round-trip per symbol.
//
// On a plain CAN adapter we drive the raw T5 commands ourselves rather than
// letting a dongle do it.
// ---------------------------------------------------------------------------

// Verified 52-byte CPU32 gather routine (see txbridge/t5_gather.s). Reads the
// descriptor table at 0x7740 and packs the listed symbols into 0x7800.
var t5GatherStub = []byte{
	0x48, 0xE7, 0xC0, 0xE0, 0x30, 0x7C, 0x77, 0x40, 0x32, 0x7C, 0x78, 0x00,
	0x70, 0x00, 0x10, 0x18, 0x67, 0x1C, 0x53, 0x40, 0x52, 0x88, 0x34, 0x58,
	0x72, 0x00, 0x12, 0x18, 0x52, 0x88, 0x4A, 0x01, 0x67, 0x08, 0x53, 0x41,
	0x12, 0xDA, 0x51, 0xC9, 0xFF, 0xFC, 0x51, 0xC8, 0xFF, 0xEA, 0x4C, 0xDF,
	0x07, 0x03, 0x4E, 0x75,
}

const (
	t5GatherStubAddr = 0x7700 // stub entry (called via C1 each cycle)
	t5GatherBufAddr  = 0x7800 // packed output buffer
	t5GatherTableOff = 0x40   // table base = 0x7740 = stub + 0x40
)

// buildT5GatherImage builds the SRAM image uploaded at t5GatherStubAddr:
// stub (padded to 0x40) + descriptor table { count, pad, N*{addr.w, len.b, pad.b} }.
// Shared by the txbridge gather path and the generic fast-logger.
func buildT5GatherImage(symbols []*symbol.Symbol) ([]byte, error) {
	image := make([]byte, t5GatherTableOff)
	copy(image, t5GatherStub)
	image = append(image, byte(len(symbols)), 0x00)
	for _, sym := range symbols {
		if sym.SramOffset >= 0x8000 || sym.Length == 0 || sym.Length > 255 {
			return nil, fmt.Errorf("symbol %s not gather-representable (addr %X len %d)", sym.Name, sym.SramOffset, sym.Length)
		}
		image = append(image, byte(sym.SramOffset>>8), byte(sym.SramOffset), byte(sym.Length), 0x00)
	}
	if len(image) > 255 {
		return nil, fmt.Errorf("gather image too large: %d bytes", len(image))
	}
	return image, nil
}

type T5FastClient struct {
	*BaseLogger
}

func NewT5Fast(cfg Config, lw LogWriter) (IClient, error) {
	return &T5FastClient{BaseLogger: NewBaseLogger(cfg, lw)}, nil
}

func (c *T5FastClient) Start(pctx context.Context) error {
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	eventHandler := func(e gocan.Event) {
		c.OnMessage(e.String())
		if e.Type == gocan.EventTypeError {
			c.onError()
		}
	}

	cl, err := gocan.OpenAdapter(ctx, c.Device, gocan.WithEventFunc(eventHandler))
	if err != nil {
		return err
	}
	defer cl.Close()
	ctx = cl.Context()

	t := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer t.Stop()
	t5 := t5can.NewClient(cl)

	// T5 decodes every value into sysvars (see newT5Converter), so all columns
	// are sysvar channels.
	channels := make([]Channel, 0, len(c.Symbols)+2)
	gatherLen := uint32(0)
	for _, s := range c.Symbols {
		s.Correctionfactor = 0.1
		gatherLen += uint32(s.Length)
		channels = append(channels, newSysvarChannel(c.sysvars, s.Name))
	}

	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
	}
	for _, name := range c.appendExtraSysvars(nil) {
		channels = append(channels, newSysvarChannel(c.sysvars, name))
	}

	// Upload the gather stub + descriptor table and arm the stream-call once.
	image, err := buildT5GatherImage(c.Symbols)
	if err != nil {
		return err
	}
	if err := t5.WriteRam(ctx, t5GatherStubAddr, image); err != nil {
		return fmt.Errorf("gather upload: %w", err)
	}
	if err := armT5Gather(ctx, cl); err != nil {
		return fmt.Errorf("gather arm: %w", err)
	}
	c.OnMessage(fmt.Sprintf("T5 fast-logger enabled (%d symbols, %d byte payload)", len(c.Symbols), len(image)))

	converto := newT5Converter()
	adscannerConverter := NewWBLInterpolator(c.WidebandConfig)

	go func() {
		defer cl.Close()
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
				c.resetPerSecond()
			case read := <-c.readChan:
				data, err := t5.ReadRam(ctx, read.Address, read.Length)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}
				read.Data = data
				read.Complete(nil)
			case write := <-c.writeChan:
				if err := t5.WriteRam2(ctx, write.Address, write.Data); err != nil {
					write.Complete(err)
					break
				}
				write.Complete(nil)
			case <-t.C:
				ts := time.Now()

				// Call the stub (C1, no reply) to pack symbols into 0x7800, then
				// read the whole packed buffer in one go.
				if err := callT5Gather(ctx, cl); err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}
				// C1 runs the stub with interrupts masked; the ECU only frees its
				// command mailbox at the end of that ISR, so a C7 sent right behind
				// C1 is dropped. Wait a touch before reading.
				time.Sleep(2 * time.Millisecond) // ponytail: fixed gap, tune if reads come back stale
				packed, err := t5.ReadRam(ctx, t5GatherBufAddr, gatherLen)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}

				off := 0
				for _, sym := range c.Symbols {
					end := off + int(sym.Length)
					if err := sym.Read(bytes.NewReader(packed[off:end])); err != nil {
						c.OnMessage("failed to read symbol " + sym.Name + ": " + err.Error())
						return
					}
					off = end
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

				if err := c.lw.Write(ts, channels); err != nil {
					c.OnMessage("failed to write log: " + err.Error())
					return
				}
				c.onCapture(ts)
			}
		}
	}()
	return cl.Wait(ctx)
}

// armT5Gather sets the ECU's stream-call flag (A5 @ stub addr, len 0). The flag
// persists, so we only arm once; C1 then calls the stub each cycle.
func armT5Gather(ctx context.Context, cl *gocan.Bus) error {
	arm := []byte{0xA5, t5GatherStubAddr >> 24, t5GatherStubAddr >> 16, t5GatherStubAddr >> 8, t5GatherStubAddr & 0xFF, 0x00, 0x00, 0x00}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	resp, err := cl.Request(rctx, gocan.NewFrame(0x5, arm), 0xC)
	if err != nil {
		return err
	}
	if resp.Length < 2 || resp.Data[0] != 0xA5 || resp.Data[1] != 0x00 {
		return fmt.Errorf("arm rejected: % 02X", resp.Data)
	}
	return nil
}

// callT5Gather issues the C1 "call address" command for the stub. C1 has no reply.
func callT5Gather(ctx context.Context, cl *gocan.Bus) error {
	c1 := []byte{0xC1, t5GatherStubAddr >> 24, t5GatherStubAddr >> 16, t5GatherStubAddr >> 8, t5GatherStubAddr & 0xFF, 0x00, 0x00, 0x00}
	return cl.Send(ctx, gocan.NewFrame(0x5, c1))
}
