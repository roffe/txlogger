package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecumaster"
	"github.com/roffe/txlogger/pkg/innovate"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

type T7Client struct {
	symbolChan chan []*symbol.Symbol
	updateChan chan *RamUpdate
	readChan   chan *ReadRequest

	quitChan chan struct{}
	sysvars  *ThreadSafeMap

	lamb LambdaProvider

	lw LogWriter

	closeOnce sync.Once

	errCount     int
	errPerSecond int

	Config
}

func NewT7(cfg Config, lw LogWriter) (IClient, error) {
	return &T7Client{
		Config:     cfg,
		symbolChan: make(chan []*symbol.Symbol, 1),
		updateChan: make(chan *RamUpdate, 1),
		readChan:   make(chan *ReadRequest, 1),
		quitChan:   make(chan struct{}),
		sysvars:    NewThreadSafeMap(),
		lw:         lw,
	}, nil
}

func (c *T7Client) Close() {
	c.closeOnce.Do(func() {
		close(c.quitChan)
		//time.Sleep(200 * time.Millisecond)
	})
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

func (c *T7Client) startBroadcastListener(ctx context.Context, cl *gocan.Client) {
	sub := cl.Subscribe(ctx, 0x1A0, 0x280, 0x3A0)
	var speed uint16
	var rpm uint16
	var throttle float64
	var realSpeed float64
	go func() {
		<-c.quitChan
		sub.Close()
	}()

	for msg := range sub.C() {
		switch msg.Identifier() {
		case 0x1A0:
			rpm = binary.BigEndian.Uint16(msg.Data()[1:3])
			throttle = float64(msg.Data()[5])
			c.sysvars.Set("ActualIn.n_Engine", float64(rpm))
			c.sysvars.Set("Out.X_AccPedal", throttle)
		case 0x280:
			data := msg.Data()[4]
			data2 := msg.Data()[3]
			if data2&0x01 == 1 {
				ebus.Publish("LIMP", 1)
			} else {
				ebus.Publish("LIMP", 0)
			}
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
		case 0x3A0:
			speed = uint16(msg.Data()[4]) | uint16(msg.Data()[3])<<8
			realSpeed = float64(speed) / 10
			c.sysvars.Set("In.v_Vehicle", realSpeed)
		}
	}

	c.OnMessage("Stopped broadcast listener")
}

func (c *T7Client) onError(err error) {
	c.errCount++
	c.errPerSecond++
	c.ErrorCounter.Set(c.errCount)
	c.OnMessage(err.Error())
}

func (c *T7Client) Start() error {
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

	//for _, sym := range c.Symbols {
	//	if c.sysvars.Exists(sym.Name) {
	//		c.sysvars.Delete(sym.Name)
	//		continue
	//	}
	//}
	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	go c.startBroadcastListener(bctx, cl)

	c.OnMessage("Watching for broadcast messages")
	<-time.After(300 * time.Millisecond)
	order := c.sysvars.Keys()
	sort.StringSlice(order).Sort()
	c.OnMessage(fmt.Sprintf("Found %s", order))

	if len(order) == 0 {
		bcancel()
	}

	switch c.Config.WidebandConfig.Type {
	case "ECU":
	case ecumaster.ProductString:
		c.lamb = ecumaster.NewLambdaToCAN(cl)
		c.lamb.Start(ctx)
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	case innovate.ProductString:
		wblClient, err := innovate.NewISP2Client(c.Config.WidebandConfig.Port, c.Config.OnMessage)
		if err != nil {
			return err
		}
		c.lamb = wblClient
		c.lamb.Start(ctx)
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)

		if txbridge {
			wblSub := cl.Subscribe(ctx, 0x124)
			defer wblSub.Close()
			go func() {
				for msg := range wblSub.C() {
					if msg.Identifier() == 0x124 {
						wblClient.SetData(msg.Data())
					}
				}
			}()
		}

	}

	for _, sym := range c.Symbols {
		if c.sysvars.Exists(sym.Name) {
			//			log.Println("Skipping", sym.Name)
			sym.Skip = true
		}
	}
	cps := 0
	count := 0
	retries := 0

	if err := cl.SendFrame(0x123, []byte("7"), gocan.Outgoing); err != nil {
		return err
	}

	kwp := kwp2000.New(cl)
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

		dpos := 0
		for _, sym := range c.Symbols {
			if sym.Skip {
				continue
			}
			//			log.Println("Defining", sym.Name, dpos)
			if err := kwp.DynamicallyDefineLocalIdRequest(ctx, dpos, sym); err != nil {
				return err
			}
			dpos++
			time.Sleep(10 * time.Millisecond)
		}
		c.OnMessage("Configured dynamic register")

		secondTicker := time.NewTicker(1000 * time.Millisecond)
		defer secondTicker.Stop()

		t := time.NewTicker(time.Second / time.Duration(c.Rate))
		defer t.Stop()

		var timeStamp time.Time
		var expectedPayloadSize uint16
		for _, sym := range c.Symbols {
			if sym.Skip {
				continue
			}
			expectedPayloadSize += sym.Length
		}

		lastPresent := time.Now()
		testerPresent := func() {
			if time.Since(lastPresent) > lastPresentInterval {
				if err := kwp.TesterPresent(ctx); err != nil {
					c.onError(err)
					return
				}
				lastPresent = time.Now()
			}
		}

		tx := cl.Subscribe(ctx, 0x123)
		defer tx.Close()

		if txbridge {
			log.Println("stopped timer, using txbridge")
			t.Stop()
			if err := cl.SendFrame(0x123, []byte("r"), gocan.Outgoing); err != nil {
				return err
			}
		}

		//buf := bytes.NewBuffer(nil)
		var databuff []byte
		for {
			select {
			case <-c.quitChan:
				c.OnMessage("Stopped logging..")
				return nil
			case <-secondTicker.C:
				c.FpsCounter.Set(cps)
				if c.errPerSecond > 5 {
					c.errPerSecond = 0
					return fmt.Errorf("too many errors per second")
				}
				cps = 0
				c.errPerSecond = 0
			case symbols := <-c.symbolChan:
				c.Symbols = symbols
				c.OnMessage("Reconfiguring symbols..")
				if err := kwp.ClearDynamicallyDefineLocalId(ctx); err != nil {
					return err
				}
				c.OnMessage("Cleared dynamic register")
				if len(c.Symbols) > 0 {
					expectedPayloadSize = 0
					dpos := 0
					for _, sym := range c.Symbols {
						if c.sysvars.Exists(sym.Name) {
							sym.Skip = true
							continue
						}
						if err := kwp.DynamicallyDefineLocalIdRequest(ctx, dpos, sym); err != nil {
							return err
						}
						dpos++
						expectedPayloadSize += sym.Length
						time.Sleep(5 * time.Millisecond)
					}
					c.OnMessage("Configured dynamic register")
				}
			case read := <-c.readChan:
				if txbridge {
					if err := cl.SendFrame(0x123, []byte("s"), gocan.Outgoing); err != nil {
						c.onError(err)
						continue
					}
					time.Sleep(42 * time.Millisecond)
				}
				// log.Printf("Reading %X %d", read.Address, read.Length)
				data, err := kwp.ReadMemoryByAddress(ctx, int(read.Address), int(read.Length))
				if err != nil {
					read.Complete(err)
					if txbridge {
						if err := cl.SendFrame(0x123, []byte("r"), gocan.Outgoing); err != nil {
							c.onError(err)
							continue
						}
					}
					continue
				}
				read.Data = data
				if txbridge {
					if err := cl.SendFrame(0x123, []byte("r"), gocan.Outgoing); err != nil {
						c.onError(err)
						continue
					}
				}
				read.Complete(nil)
			case upd := <-c.updateChan:
				if txbridge {
					if err := cl.SendFrame(0x123, []byte("s"), gocan.Outgoing); err != nil {
						c.onError(err)
						continue
					}
					time.Sleep(42 * time.Millisecond)
				}
				upd.Complete(kwp.WriteDataByAddress(ctx, upd.Address, upd.Data))
				if txbridge {
					if err := cl.SendFrame(0x123, []byte("r"), gocan.Outgoing); err != nil {
						c.onError(err)
						continue
					}
				}
			case <-t.C:
				timeStamp = time.Now()
				if len(c.Symbols) == 0 {
					testerPresent()
					continue
				}

				databuff, err = kwp.ReadDataByIdentifier(ctx, 0xF0)
				if err != nil {
					c.onError(err)
					continue
				}

				if len(databuff) != int(expectedPayloadSize) {
					c.onError(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
					continue
					//return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
				}

				r := bytes.NewReader(databuff)
				for _, va := range c.Symbols {
					if va.Skip {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
						continue
					}
					if err := va.Read(r); err != nil {
						log.Printf("data ex %d %X len %d", expectedPayloadSize, databuff, len(databuff))
						c.onError(err)
						break
					}
					if va.Name == "DisplProt.AD_Scanner" {
						value := va.Float64()
						voltage := (value / 1023) * (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
						voltage = clamp(voltage, c.WidebandConfig.MinimumVoltageWideband, c.WidebandConfig.MaximumVoltageWideband)
						steepness := (c.WidebandConfig.HighAFR - c.WidebandConfig.LowAFR) / (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
						afr := c.WidebandConfig.LowAFR + (steepness * (voltage - c.WidebandConfig.MinimumVoltageWideband))
						ebus.Publish(va.Name, afr)
						continue
					}

					ebus.Publish(va.Name, va.Float64())
				}

				if r.Len() > 0 {
					c.OnMessage(fmt.Sprintf("%d leftover bytes!", r.Len()))
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

				//produceTxLogLine(file, c.sysvars, c.Symbols, timeStamp, order)
				if err := c.lw.Write(c.sysvars, c.Symbols, timeStamp, order); err != nil {
					c.onError(fmt.Errorf("failed to write log: %w", err))
				}
				count++
				cps++
				if count%15 == 0 {
					c.CaptureCounter.Set(count)
				}
			case msg := <-tx.C():
				timeStamp = time.Now()
				databuff = msg.Data()
				if len(databuff) != int(expectedPayloadSize) {
					c.onError(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
					log.Printf("%02X", databuff)
					continue
					//return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
				}

				r := bytes.NewReader(databuff)
				for _, va := range c.Symbols {
					if va.Skip {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
						continue
					}
					if err := va.Read(r); err != nil {
						log.Printf("data ex %d %X len %d", expectedPayloadSize, databuff, len(databuff))
						c.onError(err)
						break
					}
					if va.Name == "DisplProt.AD_Scanner" {
						value := va.Float64()
						voltage := (value / 1023) * (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
						voltage = clamp(voltage, c.WidebandConfig.MinimumVoltageWideband, c.WidebandConfig.MaximumVoltageWideband)
						steepness := (c.WidebandConfig.HighAFR - c.WidebandConfig.LowAFR) / (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
						afr := c.WidebandConfig.LowAFR + (steepness * (voltage - c.WidebandConfig.MinimumVoltageWideband))
						ebus.Publish(va.Name, afr)
						continue
					}

					ebus.Publish(va.Name, va.Float64())
				}

				if r.Len() > 0 {
					c.OnMessage(fmt.Sprintf("%d leftover bytes!", r.Len()))
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

				//produceTxLogLine(file, c.sysvars, c.Symbols, timeStamp, order)
				if err := c.lw.Write(c.sysvars, c.Symbols, timeStamp, order); err != nil {
					c.onError(fmt.Errorf("failed to write log: %w", err))
				}
				count++
				cps++
				if count%15 == 0 {
					c.CaptureCounter.Set(count)
				}
			}
		}

		//c.OnMessage(fmt.Sprintf("Live logging at %d fps", c.Rate))
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(3),
		retry.OnRetry(func(n uint, err error) {
			retries++
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	return err
}

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
		return "Crankcase vent error"
	case 55:
		return "Faulty APC valve"
	case 60:
		return "Emission Limitation"
	case 61:
		return "Engine Tipin"
	case 62:
		return "Engine Tipout"
	case 70:
		return "Safety Switch"
	case 71:
		return "O2 Sens fault, E85"
	case 80:
		return "Cold engine temp"
	case 81:
		return "Overheating"
	default:
		return "Unknown"
	}
}

func FCutToStringT7(value float64) string {
	switch value {
	case 0:
		return "No fuelcut"
	case 1:
		return "Ignition key turned off"
	case 2:
		return "Accelerator pedal pressed during start"
	case 3:
		return "RPM limiter (engine speed guard)"
	case 4:
		return "Throttle block adaption active 1st time"
	case 5, 6:
		return "Airmass limit (pressure guard)"
	case 7:
		return "Immobilizer code incorrect"
	case 8:
		return "Current to h-bridge to high during throttle limphome"
	case 9:
		return "Torque to high during throttle limphome"
	case 11:
		return "Tampering protection of throttle"
	case 12:
		return "Error on all ignition trigger outputs"
	case 13:
		return "ECU not correctly programmed"
	case 14:
		return "To high rpm in throttle limp home, pedal potentiometer fault"
	case 15:
		return "Torque master fuel cut request"
	case 16:
		return "TCM requests fuelcut to smoothen gear shift"
	case 20:
		return "Application conditions for fuel cut"
	default:
		return "Unknown"
	}
}
