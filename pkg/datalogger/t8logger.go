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
	"github.com/roffe/txlogger/pkg/ecumaster"
	"golang.org/x/sync/errgroup"
)

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
		return "Special Mode"
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

type T8Client struct {
	symbolChan chan []*symbol.Symbol
	updateChan chan *RamUpdate
	readChan   chan *ReadRequest

	quitChan chan struct{}
	sysvars  *ThreadSafeMap

	lamb LambdaProvider

	lw LogWriter

	Config
}

func NewT8(dl Logger, cfg Config, lw LogWriter) (Provider, error) {
	return &T8Client{
		Config:     cfg,
		symbolChan: make(chan []*symbol.Symbol, 1),
		updateChan: make(chan *RamUpdate, 1),
		readChan:   make(chan *ReadRequest, 0),
		quitChan:   make(chan struct{}, 2),
		sysvars: &ThreadSafeMap{
			values: make(map[string]string),
		},
		lw: lw,
	}, nil
}

func (c *T8Client) Close() {
	close(c.quitChan)
	time.Sleep(150 * time.Millisecond)
}

func (c *T8Client) SetSymbols(symbols []*symbol.Symbol) error {
	select {
	case c.symbolChan <- symbols:
	default:
		return fmt.Errorf("pending")
	}
	return nil
}

func (c *T8Client) SetRAM(address uint32, data []byte) error {
	upd := NewRamUpdate(address, data)
	c.updateChan <- upd
	return upd.Wait()
}

func (c *T8Client) GetRAM(address, length uint32) ([]byte, error) {
	//c.OnMessage(fmt.Sprintf("GetRAM %X %d", address, length))
	if address+length <= 0x100000 {
		return nil, fmt.Errorf("GetRAM: address not in SRAM: $%X", address)
	}
	req := NewReadRequest(address, length)
	c.readChan <- req
	return req.Data, req.Wait()
}

const T8ChunkSize = 0x10

func (c *T8Client) Start() error {
	defer c.lw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := gocan.NewWithOpts(
		ctx,
		c.Device,
	)
	if err != nil {
		return err
	}
	defer cl.Close()

	order := make([]string, len(c.sysvars.values))
	for k := range c.sysvars.values {
		order = append(order, k)
	}
	// sort order
	sort.StringSlice(order).Sort()

	switch c.Config.Lambda {
	case "ECU":
	case ecumaster.ProductString:
		c.lamb = ecumaster.NewLambdaToCAN(cl)
		c.lamb.Start(ctx)
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	count := 0
	errCount := 0
	c.ErrorCounter.Set(errCount)

	errPerSecond := 0
	//c.ErrorPerSecondCounter.Set(errPerSecond)

	cps := 0
	retries := 0

	lastPresentInterval := 3500 * time.Millisecond

	err = retry.Do(func() error {
		gm := gmlan.New(cl, 0x7e0, 0x7e8)

		if err := gm.RequestSecurityAccess(ctx, 0xFD, 0, ecu.CalculateT8AccessKey); err != nil {
			return err
		}

		if err := clearDynamicallyDefinedRegister(ctx, gm); err != nil {
			return err
		}
		c.OnMessage("Cleared dynamic register")

		for _, sym := range c.Symbols {
			if err := setUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Number)); err != nil {
				return err
			}
			//c.OnMessage(fmt.Sprintf("Configured dynamic register %d: %s %d", i, sym.Name, sym.Value))
		}
		c.OnMessage("Configured dynamic register")

		secondTicker := time.NewTicker(time.Second)
		defer secondTicker.Stop()

		t := time.NewTicker(time.Second / time.Duration(c.Rate))
		defer t.Stop()

		errg, gctx := errgroup.WithContext(ctx)

		errg.Go(func() error {
			for {
				select {
				case <-c.quitChan:
					return nil
				case <-gctx.Done():
					return nil
				case <-secondTicker.C:
					c.FpsCounter.Set(cps)
					cps = 0
					if errPerSecond > 10 {
						return errors.New("too many errors, reconnecting")
					}
					errPerSecond = 0
				}
			}
		})

		errg.Go(func() error {
			var timeStamp time.Time
			var chunkSize uint32
			lastPresent := time.Now()

			testerPresent := func() {
				if time.Since(lastPresent) > lastPresentInterval {
					if err := gm.TesterPresentNoResponseAllowed(); err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(fmt.Sprintf("Failed to send tester present: %v", err))
					}
					lastPresent = time.Now()
				}
			}
			buf := bytes.NewBuffer(nil)
		outer:
			for {
				select {
				case <-c.quitChan:
					c.OnMessage("Stopped logging..")
					return nil
				case <-gctx.Done():
					return nil
				case symbols := <-c.symbolChan:
					c.Symbols = symbols
					c.OnMessage("Reconfiguring symbols..")
					if err := clearDynamicallyDefinedRegister(ctx, gm); err != nil {
						return err
					}
					if len(c.Symbols) > 0 {
						c.OnMessage("Cleared dynamic register")
						for _, sym := range c.Symbols {
							if err := setUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Number)); err != nil {
								return err
							}
						}
						c.OnMessage("Configured dynamic register")
					}
				case read := <-c.readChan:
					chunkSize = uint32(math.Min(float64(read.left), T8ChunkSize))
					log.Printf("Reading RAM 0x%X %d", read.Address, chunkSize)
					data, err := gm.ReadMemoryByAddress(ctx, read.Address, chunkSize)
					if err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						read.Complete(err)
						continue outer
					}
					read.Data = append(read.Data, data...)
					read.left -= chunkSize
					read.Address += chunkSize
					if time.Since(lastPresent) > lastPresentInterval {
						gm.TesterPresentNoResponseAllowed()
						lastPresent = time.Now()
					}
					if read.left > 0 {
						go func() {
							//time.Sleep(2 * time.Millisecond)
							c.readChan <- read
						}()
					} else {
						read.Complete(nil)
					}
				case upd := <-c.updateChan:
					//					log.Printf("Updating RAM 0x%X", upd.Address)
					chunkSize = uint32(math.Min(float64(upd.left), 0x06))
					if err := gm.WriteDataByAddress(ctx, upd.Address, upd.Data[:chunkSize]); err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						upd.Complete(err)
						continue outer
					}
					upd.left -= chunkSize
					upd.Address += chunkSize
					upd.Data = upd.Data[chunkSize:]
					testerPresent()
					if upd.left > 0 {
						c.updateChan <- upd
						continue outer
					}
					upd.Complete(nil)
				case <-t.C:
					timeStamp = time.Now()
					if len(c.Symbols) == 0 {
						testerPresent()
						continue
					}
					data, err := gm.ReadDataByIdentifier(ctx, 0x18)
					if err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(fmt.Sprintf("Failed to read data: %v", err))
						continue
					}
					r := bytes.NewReader(data)
					for _, va := range c.Symbols {
						buf.Reset()
						buf.Write(va.Bytes())
						if err := va.Read(r); err != nil {
							errCount++
							errPerSecond++
							c.ErrorCounter.Set(errCount)
							c.OnMessage(fmt.Sprintf("Failed to read %s: %v", va.Name, err))
							break
						}
						if !bytes.Equal(va.Bytes(), buf.Bytes()) {
							ebus.Publish(va.Name, va.Float64())
						}
					}
					if r.Len() > 0 {
						left := r.Len()
						leftovers := make([]byte, r.Len())
						n, err := r.Read(leftovers)
						if err != nil {
							c.OnMessage(fmt.Sprintf("Failed to read leftovers: %v", err))
							continue
						}
						c.OnMessage(fmt.Sprintf("Leftovers %d: %X", left, leftovers[:n]))
					}

					if c.lamb != nil {
						value := fmt.Sprintf("%.2f", c.lamb.GetLambda())
						ebus.Publish(EXTERNALWBLSYM, c.lamb.GetLambda())
						c.sysvars.Set(EXTERNALWBLSYM, value)
					}

					//produceTxLogLine(file, c.sysvars, c.Symbols, timeStamp, order)
					if err := c.lw.Write(c.sysvars, c.Symbols, timeStamp, order); err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(fmt.Sprintf("Failed to write log: %v", err))
					}
					cps++
					count++
					if count%10 == 0 {
						c.CaptureCounter.Set(count)
					}
					testerPresent()
				}
			}
		})

		//c.OnMessage(fmt.Sprintf("Live logging at %d fps", c.Rate))

		return errg.Wait()

	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(4),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			retries++
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
	)
	return err
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
