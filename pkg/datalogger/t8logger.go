package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecu"
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

type T8Client struct {
	symbolChan chan []*symbol.Symbol
	writeChan  chan *WriteRequest
	readChan   chan *ReadRequest

	quitChan chan struct{}
	sysvars  *ThreadSafeMap

	lamb LambdaProvider

	lw LogWriter

	errCount     int
	errPerSecond int

	closeOnce sync.Once
	Config
}

func NewT8(cfg Config, lw LogWriter) (IClient, error) {
	return &T8Client{
		Config:     cfg,
		symbolChan: make(chan []*symbol.Symbol, 1),
		writeChan:  make(chan *WriteRequest, 1),
		readChan:   make(chan *ReadRequest, 1),
		quitChan:   make(chan struct{}, 2),
		sysvars:    NewThreadSafeMap(),
		lw:         lw,
	}, nil
}

func (c *T8Client) Close() {
	c.closeOnce.Do(func() {
		close(c.quitChan)
		time.Sleep(150 * time.Millisecond)
	})
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
	c.writeChan <- upd
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

const T8ChunkSize = 235

const lastPresentInterval = 3500 * time.Millisecond

func (c *T8Client) onError(err error) {
	c.errCount++
	c.errPerSecond++
	c.ErrorCounter.SetText("Err: " + strconv.Itoa(c.errCount))
	c.OnMessage(err.Error())
}

func (c *T8Client) Start() error {
	defer c.lw.Close()

	var txbridge bool
	if c.Config.Device.Name() == "txbridge" {
		txbridge = true
	}

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

	order := c.sysvars.Keys()

	// Wideband lambda setup
	cfg := &WBLConfig{
		WBLType:  c.Config.WidebandConfig.Type,
		Port:     c.Config.WidebandConfig.Port,
		Log:      c.OnMessage,
		Txbridge: txbridge,
	}

	c.lamb, err = NewWBL(ctx, cl, cfg)
	if err != nil {
		return err
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	// implement sym.Skip from T7 logger here

	// sort order
	sort.StringSlice(order).Sort()

	c.ErrorCounter.SetText("Err: 0")

	errPerSecond := 0

	cps := 0
	count := 0
	retries := 0

	if txbridge {
		if err := cl.SendFrame(adapter.SystemMsg, []byte("8"), gocan.Outgoing); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	err = retry.Do(func() error {
		gm := gmlan.New(cl, 0x7e0, 0x7e8)

		if err := gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
			return err
		}

		if err := gm.RequestSecurityAccess(ctx, 0xFD, 1, ecu.CalculateT8AccessKey); err != nil {
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
					c.FpsCounter.SetText("Cps: " + strconv.Itoa(cps))
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

			var expectedPayloadSize uint16
			for _, sym := range c.Symbols {
				expectedPayloadSize += sym.Length
			}

			lastPresent := time.Now()

			testerPresent := func() {
				if time.Since(lastPresent) > lastPresentInterval {
					if err := gm.TesterPresentNoResponseAllowed(); err != nil {
						c.onError(fmt.Errorf("failed to send tester present: %w", err))
					}
					lastPresent = time.Now()
				}
			}

			tx := cl.Subscribe(ctx, adapter.SystemMsgDataResponse, adapter.SystemMsgError)
			defer tx.Close()

			if txbridge {
				//log.Println("stopped timer, using txbridge")
				t.Stop()
				if err := cl.SendFrame(adapter.SystemMsg, []byte("r"), gocan.Outgoing); err != nil {
					return err
				}
			}

			var firstTime time.Time
			var firstTimestamp uint32
			var currtimestamp uint32
			for {
				select {
				case <-c.quitChan:
					c.OnMessage("Finished logging")
					return nil
				case <-gctx.Done():
					return nil
				case symbols := <-c.symbolChan:
					c.Symbols = symbols
					expectedPayloadSize = 0
					for _, sym := range c.Symbols {
						expectedPayloadSize += sym.Length
					}
					c.OnMessage("Reconfiguring symbols..")
					if err := clearDynamicallyDefinedRegister(ctx, gm); err != nil {
						return err
					}
					c.OnMessage("Cleared dynamic register")
					if len(c.Symbols) > 0 {
						for _, sym := range c.Symbols {
							if err := setUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Number)); err != nil {
								return err
							}
						}
						c.OnMessage("Configured dynamic register")
					}
				case read := <-c.readChan:
					if txbridge {
						if err := c.handleReadTxbridge(ctx, cl, read); err != nil {
							read.Complete(err)
						}
						continue
					}

					chunkSize = uint32(math.Min(float64(read.left), T8ChunkSize))
					log.Printf("Reading RAM 0x%X %d", read.Address, chunkSize)
					data, err := gm.ReadMemoryByAddress(ctx, read.Address, chunkSize)
					if err != nil {
						read.Complete(err)
						continue
					}
					read.Data = append(read.Data, data...)
					read.left -= chunkSize
					read.Address += chunkSize
					if read.left > 0 {
						c.readChan <- read
						continue
					}
					read.Complete(nil)
					time.Sleep(12 * time.Millisecond)
				case upd := <-c.writeChan:
					if txbridge {
						if err := c.handleWriteTxbridge(ctx, cl, upd); err != nil {
							upd.Complete(err)
						}
						continue
					}
					chunkSize = uint32(math.Min(float64(upd.Length), T8ChunkSize))
					log.Printf("Updating RAM 0x%X %d", upd.Address, chunkSize)
					if err := gm.WriteDataByAddress(ctx, upd.Address, upd.Data[:chunkSize]); err != nil {
						upd.Complete(err)
						continue
					}

					upd.Length -= chunkSize
					upd.Address += chunkSize
					upd.Data = upd.Data[chunkSize:]

					if upd.Length > 0 {
						c.writeChan <- upd
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
						c.onError(fmt.Errorf("failed to read data: %w", err))
						continue
					}
					if len(databuff) != int(expectedPayloadSize) {
						return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
					}
					r := bytes.NewReader(databuff)

					for _, va := range c.Symbols {
						if err := va.Read(r); err != nil {
							c.onError(fmt.Errorf("failed to set data: %w", err))
							break
						}
						if err := ebus.Publish(va.Name, va.Float64()); err != nil {
							c.onError(err)
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
						c.onError(fmt.Errorf("failed to write log: %w", err))
					}
					cps++
					count++
					if count%10 == 0 {
						c.CaptureCounter.SetText("Cap: " + strconv.Itoa(count))
					}
					testerPresent()
				case msg := <-tx.C():
					if msg == nil {
						return retry.Unrecoverable(errors.New("nil message received"))
					}

					if msg.Identifier() == adapter.SystemMsgError {
						c.onError(errors.New("read timeout"))
						continue
					}

					databuff := msg.Data()
					if len(databuff) != int(expectedPayloadSize+4) {
						return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize+4, len(databuff)))
					}

					r := bytes.NewReader(databuff)
					binary.Read(r, binary.LittleEndian, &currtimestamp)

					if firstTime.IsZero() {
						firstTime = time.Now()
						firstTimestamp = currtimestamp
					}

					timeStamp := calculateCompensatedTimestamp(firstTime, firstTimestamp, currtimestamp)

					for _, va := range c.Symbols {
						if err := va.Read(r); err != nil {
							c.onError(fmt.Errorf("failed to set data: %w", err))
							break
						}
						if err := ebus.Publish(va.Name, va.Float64()); err != nil {
							c.onError(fmt.Errorf("failed to publish data: %w", err))
						}
					}

					if r.Len() > 0 {
						c.OnMessage(fmt.Sprintf("%d leftover bytes!", r.Len()))
					}

					if c.lamb != nil {
						ebus.Publish(EXTERNALWBLSYM, c.lamb.GetLambda())
						c.sysvars.Set(EXTERNALWBLSYM, c.lamb.GetLambda())
					}

					if err := c.lw.Write(c.sysvars, c.Symbols, timeStamp, order); err != nil {
						c.onError(fmt.Errorf("failed to write log: %w", err))
					}
					cps++
					count++
					if count%10 == 0 {
						c.CaptureCounter.SetText("Cap: " + strconv.Itoa(count))
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
			retries++
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
	)
	return err
}

func (c *T8Client) handleWriteTxbridge(ctx context.Context, cl *gocan.Client, write *WriteRequest) error {
	toRead := min(write.Length, T8ChunkSize)
	log.Printf("Writing RAM $%X:%d", write.Address, toRead)
	cmd := gocan.SerialCommand{
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

	payload, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}
	frame := gocan.NewFrame(adapter.SystemMsg, payload, gocan.Outgoing)
	resp, err := cl.SendAndPoll(ctx, frame, 1*time.Second, adapter.SystemMsgWriteResponse, adapter.SystemMsgError)
	if err != nil {
		return err
	}
	if resp.Identifier() == adapter.SystemMsgError {
		return fmt.Errorf("error: %X", resp.Data())
	}
	write.Address += uint32(toRead)
	write.Length -= toRead
	write.Data = write.Data[toRead:]

	if write.Length > 0 {
		c.writeChan <- write
	} else {
		write.Complete(nil)
	}
	return nil
}

func (c *T8Client) handleReadTxbridge(ctx context.Context, cl *gocan.Client, read *ReadRequest) error {
	toRead := min(T8ChunkSize, read.Length)
	log.Printf("Reading RAM $%X:%d", read.Address, toRead)
	cmd := gocan.SerialCommand{
		Command: 'R',
		Data: []byte{
			byte(read.Address),
			byte(read.Address >> 8),
			byte(read.Address >> 16),
			byte(read.Address >> 24),
			byte(toRead),
		},
	}
	payload, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}

	frame := gocan.NewFrame(adapter.SystemMsg, payload, gocan.Outgoing)
	resp, err := cl.SendAndPoll(ctx, frame, 4*time.Second, adapter.SystemMsgDataRequest)
	if err != nil {
		return err
	}
	read.Address += uint32(toRead)
	read.Length -= toRead
	read.Data = append(read.Data, resp.Data()...)
	if read.Length > 0 {
		c.readChan <- read
	} else {
		read.Complete(nil)
	}
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
