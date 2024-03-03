package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecumaster"
	"github.com/roffe/txlogger/pkg/kwp2000"
	"golang.org/x/sync/errgroup"
)

func AirDemToStringT7(v float64) string {
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
		return "Stall Limit"
	case 25:
		return "Special Mode"
	case 26:
		return "Reverse Limit (Auto)"
	case 27:
		return "Misfire diagnose"
	case 28:
		return "Brake Management"
	case 29:
		return "Diff Prot (Automatic)"
	case 30:
		return "Not used"
	case 31:
		return "Max Vehicle Speed"
	case 40:
		return "LDA Request"
	case 41:
		return "Min Load"
	case 42:
		return "Dash Pot"
	case 50:
		return "Knock Airmass Limit"
	case 51:
		return "Max Engine Speed"
	case 52:
		return "Max Air for Lambda 1"
	case 53:
		return "Max Turbo Speed"
	case 54:
		return "N.A"
	case 55:
		return "Faulty APC valve"
	case 60:
		return "Emission Limitation"
	case 70:
		return "Safety Switch Limit"
	default:
		return "Unknown"
	}
}

type T7Client struct {
	symbolChan chan []*symbol.Symbol
	updateChan chan *RamUpdate
	readChan   chan *ReadRequest

	quitChan chan struct{}
	sysvars  *ThreadSafeMap

	lamb LambdaProvider

	lw LogWriter

	Config
}

func NewT7(dl Logger, cfg Config, lw LogWriter) (Provider, error) {
	return &T7Client{
		Config:     cfg,
		symbolChan: make(chan []*symbol.Symbol, 1),
		updateChan: make(chan *RamUpdate, 1),
		readChan:   make(chan *ReadRequest, 1),
		quitChan:   make(chan struct{}, 2),
		sysvars: &ThreadSafeMap{
			values: map[string]string{
				"ActualIn.n_Engine": "0",   // comes from 0x1A0
				"Out.X_AccPedal":    "0.0", // comes from 0x1A0
				"In.v_Vehicle":      "0.0", // comes from 0x3A0
				"Out.ST_LimpHome":   "0",   // comes from 0x280
			},
		},
		lw: lw,
	}, nil
}

func (c *T7Client) Close() {
	close(c.quitChan)
	time.Sleep(200 * time.Millisecond)
}

func (c *T7Client) SetSymbols(symbols []*symbol.Symbol) error {
	select {
	case c.symbolChan <- symbols:
	default:
		return fmt.Errorf("pending")
	}
	return nil
}

func (c *T7Client) SetRAM(address uint32, data []byte) error {
	upd := NewRamUpdate(address, data)
	c.updateChan <- upd
	return upd.Wait()
}

func (c *T7Client) GetRAM(address uint32, length uint32) ([]byte, error) {
	req := NewReadRequest(address, length)
	c.readChan <- req
	return req.Data, req.Wait()
}

func (c *T7Client) Start() error {
	/* 	file, filename, err := createLog(c.LogPath, "t7l")
	   	if err != nil {
	   		return err
	   	}
	   	defer file.Close()
	   	defer file.Sync()
	   	c.OnMessage(fmt.Sprintf("Logging to %s%s", c.LogPath, filename)) */
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

	sub := cl.Subscribe(ctx, 0x1A0, 0x280, 0x3A0)
	go func() {
		defer sub.Close()
		for msg := range sub.C() {
			switch msg.Identifier() {
			case 0x1A0:
				rpm := binary.BigEndian.Uint16(msg.Data()[1:3])
				throttle := int(msg.Data()[5])
				c.sysvars.Set("ActualIn.n_Engine", strconv.Itoa(int(rpm)))
				c.sysvars.Set("Out.X_AccPedal", strconv.Itoa(throttle)+",0")
				ebus.Publish("ActualIn.n_Engine", float64(rpm))
				ebus.Publish("Out.X_AccPedal", float64(throttle))
			case 0x280:
				data := msg.Data()[4]
				if data&0x20 == 0x20 {
					ebus.Publish("CRUISE", 1)
				} else {
					ebus.Publish("CRUISE", 0)
				}
				if data&0x80 == 0x80 {
					ebus.Publish("CEL", 1)
				} else {
					ebus.Publish("CEL", 0)
				}
				data2 := msg.Data()[3]
				if data2&0x01 == 0x01 {
					ebus.Publish("LIMP", 1)
				} else {
					ebus.Publish("LIMP", 0)
				}
			case 0x3A0:
				speed := uint16(msg.Data()[4]) | uint16(msg.Data()[3])<<8
				realSpeed := float64(speed) / 10
				c.sysvars.Set("In.v_Vehicle", strconv.FormatFloat(realSpeed, 'f', 1, 64))
				ebus.Publish("In.v_Vehicle", realSpeed)
			}
		}
	}()

	kwp := kwp2000.New(cl)

	count := 0
	errCount := 0
	c.ErrorCounter.Set(errCount)

	errPerSecond := 0
	//c.ErrorPerSecondCounter.Set(errPerSecond)

	//cps := 0
	retries := 0

	err = retry.Do(func() error {
		if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
			if retries == 0 {
				return retry.Unrecoverable(err)
			}
			return err
		}
		defer func() {
			kwp.StopSession(ctx)
			time.Sleep(50 * time.Millisecond)
		}()

		c.OnMessage("Connected to ECU")

		granted, err := kwp.RequestSecurityAccess(ctx, false)
		if err != nil {
			return err
		}

		if !granted {
			c.OnMessage("Security access not granted!")
		} else {
			c.OnMessage("Security access granted")
		}

		if err := kwp.ClearDynamicallyDefineLocalId(ctx); err != nil {
			return err
		}
		c.OnMessage("Cleared dynamic register")

		for i, sym := range c.Symbols {
			if err := kwp.DynamicallyDefineLocalIdRequest(ctx, i, sym); err != nil {
				return err
			}
			time.Sleep(5 * time.Millisecond)
		}
		c.OnMessage("Configured dynamic register")

		secondTicker := time.NewTicker(time.Second)
		defer secondTicker.Stop()

		t := time.NewTicker(time.Second / time.Duration(c.Rate))
		defer t.Stop()

		errg, gctx := errgroup.WithContext(ctx)

		cps := 0
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
					if errPerSecond > 5 {
						errPerSecond = 0
						return fmt.Errorf("too many errors")
					}
					errPerSecond = 0
				}
			}
		})
		errg.Go(func() error {
			var timeStamp time.Time
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
					if err := kwp.ClearDynamicallyDefineLocalId(ctx); err != nil {
						return err
					}
					c.OnMessage("Cleared dynamic register")
					if len(c.Symbols) > 0 {
						for i, sym := range c.Symbols {
							if err := kwp.DynamicallyDefineLocalIdRequest(ctx, i, sym); err != nil {
								return err
							}
							time.Sleep(5 * time.Millisecond)
						}
						c.OnMessage("Configured dynamic register")
					}
				case read := <-c.readChan:
					data, err := kwp.ReadMemoryByAddress(ctx, int(read.Address), int(read.Length))
					if err != nil {
						read.Complete(err)
					}
					read.Data = data
					read.Complete(nil)
				case upd := <-c.updateChan:
					upd.Complete(kwp.WriteDataByAddress(ctx, upd.Address, upd.Data))
				case <-t.C:
					timeStamp = time.Now()

					if len(c.Symbols) == 0 {
						if err := kwp.TesterPresent(ctx); err != nil {
							errCount++
							errPerSecond++
							c.ErrorCounter.Set(errCount)
							c.OnMessage(err.Error())
						}
						continue
					}

					data, err := kwp.ReadDataByIdentifier(ctx, 0xF0)
					if err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(err.Error())
						continue
					}
					r := bytes.NewReader(data)
					for _, va := range c.Symbols {
						if err := va.Read(r); err != nil {
							errCount++
							errPerSecond++
							c.ErrorCounter.Set(errCount)
							if err == io.EOF {
								return fmt.Errorf("EOF reading symbol %s", va.Name)
							}
							c.OnMessage(err.Error())
							break
						}
						// Set value on dashboards
						ebus.Publish(va.Name, va.Float64())
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
					count++
					cps++
					if count%10 == 0 {
						c.CaptureCounter.Set(count)
					}

				}
			}
		})
		//c.OnMessage(fmt.Sprintf("Live logging at %d fps", c.Rate))
		return errg.Wait()
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(4),
		retry.OnRetry(func(n uint, err error) {
			retries++
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	return err
}
