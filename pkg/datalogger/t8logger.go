package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/ecu"
	"golang.org/x/sync/errgroup"
)

type T8Client struct {
	dl Logger

	symbolChan chan []*symbol.Symbol
	updateChan chan *RamUpdate
	readChan   chan *ReadRequest

	quitChan chan struct{}
	sysvars  *ThreadSafeMap

	Config
}

func NewT8(dl Logger, cfg Config) (Provider, error) {
	return &T8Client{
		Config:     cfg,
		dl:         dl,
		symbolChan: make(chan []*symbol.Symbol, 1),
		updateChan: make(chan *RamUpdate, 1),
		readChan:   make(chan *ReadRequest, 1),
		quitChan:   make(chan struct{}, 2),
		sysvars: &ThreadSafeMap{
			values: make(map[string]string),
		},
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
	//upd := NewRamUpdate(address, data)
	//c.updateChan <- upd
	//return upd.Wait()
	return nil
}

func (c *T8Client) GetRAM(address, length uint32) ([]byte, error) {
	if address+length >= 0x100000 && address+length <= (0x100000+32768) {
		req := NewReadRequest(address, length)
		c.readChan <- req
		return req.Data, req.Wait()
	}
	return nil, fmt.Errorf("GetRAM: address out of range: $%X", address)
}

func (c *T8Client) Start() error {
	file, filename, err := createLog(c.LogPath, "t8l")
	if err != nil {
		return err
	}
	defer file.Close()
	defer file.Sync()
	c.OnMessage(fmt.Sprintf("Logging to %s", filename))

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

	count := 0
	errCount := 0
	c.ErrorCounter.Set(errCount)

	errPerSecond := 0
	//c.ErrorPerSecondCounter.Set(errPerSecond)

	// cps := 0
	retries := 0

	err = retry.Do(func() error {
		gm := gmlan.New(cl, 0x7e0, 0x7e8)

		if err := gm.RequestSecurityAccess(ctx, 0xFD, 0, ecu.CalculateT8AccessKey); err != nil {
			return err
		}

		if err := ClearDynamicallyDefinedRegister(ctx, gm); err != nil {
			return err
		}
		c.OnMessage("Cleared dynamic register")

		for _, sym := range c.Symbols {
			if err := SetUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Number)); err != nil {
				return err
			}
			//c.OnMessage(fmt.Sprintf("Configured dynamic register %d: %s %d", i, sym.Name, sym.Value))
		}
		c.OnMessage("Configured dynamic register")

		secondTicker := time.NewTicker(time.Second)
		defer secondTicker.Stop()

		t := time.NewTicker(time.Second / time.Duration(c.Rate))
		defer t.Stop()

		//first := true

		errg, gctx := errgroup.WithContext(ctx)

		errg.Go(func() error {
			for {
				select {
				case <-c.quitChan:
					return nil
				case <-gctx.Done():
					return nil
				case <-secondTicker.C:
					//log.Println("cps:", cps)
					//cps = 0
					//c.ErrorPerSecondCounter.Set(errPerSecond)
					if errPerSecond > 10 {
						errPerSecond = 0
						return fmt.Errorf("too many errors")
					}
					errPerSecond = 0
				}
			}
		})

		errg.Go(func() error {
			var timeStamp time.Time

			order := make([]string, len(c.sysvars.values))
			for k := range c.sysvars.values {
				order = append(order, k)
			}
			// sort order
			sort.StringSlice(order).Sort()

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
					if err := ClearDynamicallyDefinedRegister(ctx, gm); err != nil {
						return err
					}
					if len(c.Symbols) > 0 {
						c.OnMessage("Cleared dynamic register")
						for _, sym := range c.Symbols {
							if err := SetUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Number)); err != nil {
								return err
							}
						}
						c.OnMessage("Configured dynamic register")
					}
				case read := <-c.readChan:
					readAddr := read.Address
					left := read.Length
					for left > 0 {
						var chunk uint32
						if left > 0xF0 {
							chunk = 0xF0
						} else {
							chunk = left
						}
						data, err := gm.ReadMemoryByAddress(ctx, readAddr, chunk)
						if err != nil {
							errCount++
							errPerSecond++
							c.ErrorCounter.Set(errCount)
							read.Complete(err)
						}
						read.Data = append(read.Data, data...)
						left -= chunk
						readAddr += chunk
					}
					read.Complete(nil)

				case <-t.C:
					timeStamp = time.Now()
					if len(c.Symbols) == 0 {
						if err := gm.TesterPresentNoResponseAllowed(); err != nil {
							errCount++
							errPerSecond++
							c.ErrorCounter.Set(errCount)
							c.OnMessage(fmt.Sprintf("Failed to send tester present: %v", err))
						}
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
					if err != nil {
						return err
					}
					for _, va := range c.Symbols {
						if err := va.Read(r); err != nil {
							errCount++
							errPerSecond++
							c.ErrorCounter.Set(errCount)
							c.OnMessage(fmt.Sprintf("Failed to read %s: %v", va.Name, err))
							break
						}
						c.dl.SetValue(va.Name, va.Float64())
					}
					if r.Len() > 0 {
						left := r.Len()
						leftovers := make([]byte, r.Len())
						n, err := r.Read(leftovers)
						if err != nil {
							c.OnMessage(fmt.Sprintf("Failed to read leftovers: %v", err))
						}
						c.OnMessage(fmt.Sprintf("Leftovers %d: %X", left, leftovers[:n]))
					}
					produceLogLine(file, c.sysvars, c.Symbols, timeStamp, order)
					//cps++
					count++
					if count%10 == 0 {
						c.CaptureCounter.Set(count)
					}
					//if count%30 == 0 {
					//	gm.TesterPresentNoResponseAllowed()
					//}
				}
			}
		})

		c.OnMessage(fmt.Sprintf("Live logging at %d fps", c.Rate))

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

func ClearDynamicallyDefinedRegister(ctx context.Context, gm *gmlan.Client) error {
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x04}); err != nil {
		return fmt.Errorf("ClearDynamicallyDefinedRegister: %w", err)
	}
	return nil
}

func SetUpDynamicallyDefinedRegisterBySymbol(ctx context.Context, gm *gmlan.Client, symbol uint16) error {
	/* payload
	byte[0] = register id
	byte[1] type
		0x03 = Define by memory address
		0x04 = Clear dynamic defined register
		0x80 = Define by symbol position
	byte[5] symbol id high byte
	byte[6]	symbol id low byte
	*/
	payload := []byte{0xF0, 0x80, 0x00, 0x00, 0x00, byte(symbol >> 8), byte(symbol)}
	if err := gm.WriteDataByIdentifier(ctx, 0x17, payload); err != nil {
		return fmt.Errorf("SetUpDynamicallyDefinedRegisterBySymbol: %w", err)
	}
	return nil
}
