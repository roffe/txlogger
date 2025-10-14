package datalogger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecu"
	"golang.org/x/sync/errgroup"
)

type T8Client struct {
	BaseLogger
}

func NewT8(cfg Config, lw LogWriter) (IClient, error) {
	return &T8Client{BaseLogger: NewBaseLogger(cfg, lw)}, nil
}

func (c *T8Client) GetRAM(address, length uint32) ([]byte, error) {
	if address+length <= 0x100000 {
		return nil, fmt.Errorf("GetRAM: address not in SRAM: $%X", address)
	}
	req := NewReadDataRequest(address, length)
	c.readChan <- req
	return req.Data, req.Wait()
}

const T8ReadChunkSize = 245
const T8WriteChunkSize = 245
const lastPresentInterval = 2500 * time.Millisecond

func (c *T8Client) Start() error {
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := gocan.NewWithOpts(ctx, c.Device)
	if err != nil {
		return err
	}
	defer cl.Close()

	order := c.sysvars.Keys()

	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	// implement sym.Skip from T7 logger here

	// sort order
	sort.StringSlice(order).Sort()

	gm := gmlan.New(cl, 0x7e0, 0x7e8)

	err = retry.Do(func() error {
		if err := initT8Logging(ctx, gm, c.Symbols, c.OnMessage); err != nil {
			return fmt.Errorf("failed to init t8 logging: %w", err)
		}
		defer gm.ReturnToNormalMode(ctx)

		t := time.NewTicker(time.Second / time.Duration(c.Rate))
		defer t.Stop()

		errg, gctx := errgroup.WithContext(ctx)

		errg.Go(func() error {
			var timeStamp time.Time
			var chunkSize uint32

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

			messages := cl.Subscribe(ctx, gocan.SystemMsg)
			defer messages.Close()

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
				case read := <-c.readChan:
					for read.left > 0 {
						chunkSize = uint32(math.Min(float64(read.left), T8ReadChunkSize))
						log.Printf("Reading RAM 0x%X %d", read.Address, chunkSize)
						data, err := gm.ReadMemoryByAddress(ctx, read.Address, chunkSize)
						if err != nil {
							read.Complete(err)
							continue
						}
						read.Data = append(read.Data, data...)
						read.left -= chunkSize
						read.Address += chunkSize
					}
					read.Complete(nil)
				case upd := <-c.writeChan:
					chunkSize = uint32(math.Min(float64(upd.Length), T8WriteChunkSize))
					log.Printf("Updating RAM 0x%X %d", upd.Address, chunkSize)
					if err := gm.WriteDataByAddress(ctx, upd.Address, upd.Data[:chunkSize]); err != nil {
						upd.Complete(err)
						continue
					}
					upd.Address += chunkSize
					upd.Length -= chunkSize
					upd.Data = upd.Data[chunkSize:]
					if upd.Length > 0 {
						c.writeChan <- upd
						t.Reset(time.Second / time.Duration(c.Rate))
						continue
					}
					upd.Complete(nil)
					time.Sleep(12 * time.Millisecond)
				case <-t.C:
					timeStamp = time.Now()
					if len(c.Symbols) == 0 {
						testerPresent()
						continue
					}
					databuff, err := gm.ReadDataByIdentifier(ctx, 0x18)
					if err != nil {
						c.onError()
						c.OnMessage("failed to read data: " + err.Error())
						continue
					}
					if len(databuff) != int(expectedPayloadSize) {
						return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
					}
					r := bytes.NewReader(databuff)

					for _, va := range c.Symbols {
						if err := va.Read(r); err != nil {
							c.onError()
							c.OnMessage("failed to set data: " + err.Error())
							break
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
						ebus.Publish(EXTERNALWBLSYM, c.lamb.GetLambda())
						c.sysvars.Set(EXTERNALWBLSYM, c.lamb.GetLambda())
					}

					//produceTxLogLine(file, c.sysvars, c.Symbols, timeStamp, order)
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

func initT8Logging(ctx context.Context, gm *gmlan.Client, symbols []*symbol.Symbol, onMessage func(string)) error {
	if err := gm.InitiateDiagnosticOperation(ctx, 0x03); err != nil {
		return err
	}

	if err := gm.RequestSecurityAccess(ctx, 0xFD, 1, ecu.CalculateT8AccessKey); err != nil {
		return err
	}

	if err := clearDynamicallyDefinedRegister(ctx, gm); err != nil {
		return err
	}
	onMessage("Cleared dynamic register")

	for _, sym := range symbols {
		if err := setUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Number)); err != nil {
			return err
		}
		//onMessage(fmt.Sprintf("Configured dynamic register %d: %s %d", i, sym.Name, sym.Value))
	}
	onMessage("Configured dynamic register")
	return nil
}

func clearDynamicallyDefinedRegister(ctx context.Context, gm *gmlan.Client) error {
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x04}); err != nil {
		return fmt.Errorf("ClearDynamicallyDefinedRegister: %w", err)
	}
	return nil
}

func setUpDynamicallyDefinedRegisterBySymbol(ctx context.Context, gm *gmlan.Client, symbol uint16) error {
	/* payload
	byte[0] = register id
	byte[1] type
		0x03 = Define by memory address
		0x04 = Clear dynamic defined register
		0x80 = Define by symbol position
	byte[5] symbol id high byte
	byte[6]	symbol id low byte
	*/
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x80, 0x00, 0x00, 0x00, byte(symbol >> 8), byte(symbol)}); err != nil {
		return fmt.Errorf("SetUpDynamicallyDefinedRegisterBySymbol: %w", err)
	}
	return nil
}

func AirDemToStringT8(v float64) string {
	switch v {
	case 10:
		return "PedalMap"
	case 11:
		return "Cruise Control"
	case 12:
		return "Idle Control"
	case 20:
		return "Max Engine Torque"
	case 21:
		return "Traction Control"
	case 22:
		return "Manual Gearbox Limit"
	case 23:
		return "Automatic Gearbox Lim"
	case 24:
		return "Stall Limit (Automatic)"
	case 25:
		return "Hardcoded Limit"
	case 26:
		return "Reverse Limit (Automatic)"
	case 27:
		return "Max Vehicle speed"
	case 28:
		return "Brake Management"
	case 29:
		return "System Action"
	case 30:
		return "Max Engine Speed"
	case 40:
		return "Min Load"
	case 50:
		return "Knock Airmass Limit"
	case 52:
		return "Max Turbo Speed"
	default:
		return "Unknown"
	}
}
