package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

type T7Client struct {
	BaseLogger
}

func NewT7(cfg Config, lw LogWriter) (IClient, error) {
	return &T7Client{BaseLogger: NewBaseLogger(cfg, lw)}, nil
}

func t7broadcastListener(ctx context.Context, cl *gocan.Client, sysvars *ThreadSafeMap) {
	sub := cl.Subscribe(ctx, 0x1A0, 0x280, 0x3A0)
	var speed uint16
	var rpm uint16
	var throttle float64
	var realSpeed float64
	defer sub.Close()
	log.Println("Started T7 broadcast listener")
	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping T7 broadcast listener")
			return
		case msg := <-sub.Chan():
			switch msg.Identifier {
			case 0x1A0:
				rpm = binary.BigEndian.Uint16(msg.Data[1:3])
				throttle = float64(msg.Data[5])
				sysvars.Set("ActualIn.n_Engine", float64(rpm))
				sysvars.Set("Out.X_AccPedal", throttle)
			case 0x280:
				if msg.Data[3]&0x01 == 1 {
					ebus.Publish("LIMP", 1)
				} else {
					ebus.Publish("LIMP", 0)
				}
				if msg.Data[4]&0x20 == 0x20 {
					ebus.Publish("CRUISE", 1)
				} else {
					ebus.Publish("CRUISE", 0)
				}
				if msg.Data[4]&0x80 == 0x80 {
					ebus.Publish("CEL", 1)
				} else {
					ebus.Publish("CEL", 0)
				}
			case 0x3A0:
				speed = uint16(msg.Data[4]) | uint16(msg.Data[3])<<8
				realSpeed = float64(speed) / 10
				sysvars.Set("In.v_Vehicle", realSpeed)
			}
		}
	}
}

func (c *T7Client) Start() error {
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventHandler := func(e gocan.Event) {
		c.OnMessage(e.String())
		if e.Type == gocan.EventTypeError {
			c.onError()
		}
	}

	cl, err := gocan.NewWithOpts(ctx, c.Device, gocan.WithEventHandler(eventHandler))
	if err != nil {
		return fmt.Errorf("failed to create t7 client: %w", err)
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
	go t7broadcastListener(bctx, cl, c.sysvars)

	c.OnMessage("Watching for broadcast messages")
	<-time.After(550 * time.Millisecond)
	order := c.sysvars.Keys()
	sort.StringSlice(order).Sort()
	c.OnMessage(fmt.Sprintf("Found %s", order))

	if len(order) == 0 {
		bcancel()
	}

	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	for _, sym := range c.Symbols {
		if c.sysvars.Exists(sym.Name) {
			// log.Println("Skipping", sym.Name)
			sym.Number = -1
		}
		if sym.Number < 1000 {
			order = append(order, sym.Name)
		}
	}

	kwp := kwp2000.New(cl)

	adConverter := newDisplProtADConverterT7(c.WidebandConfig)

	if err := initT7logging(ctx, kwp, c.Symbols, c.OnMessage); err != nil {
		return fmt.Errorf("failed to init t7 logging: %w", err)
	}
	defer func() {
		kwp.StopSession(ctx)
		time.Sleep(50 * time.Millisecond)
	}()

	t := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer t.Stop()

	var timeStamp time.Time
	var expectedPayloadSize uint16
	for _, sym := range c.Symbols {
		if sym.Number < 0 {
			continue
		}
		expectedPayloadSize += sym.Length
	}

	lastPresent := time.Now()
	testerPresent := func() {
		if time.Since(lastPresent) > lastPresentInterval {
			if err := kwp.TesterPresent(ctx); err != nil {
				c.onError()
				c.OnMessage(err.Error())
				return
			}
			lastPresent = time.Now()
		}
	}

	go func() {
		if err := cl.Wait(); err != nil {
			c.OnMessage(err.Error())
			cancel()
		}
	}()

	eventChan := cl.Event()

	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-eventChan:
			c.OnMessage(e.String())
			if e.Type == gocan.EventTypeError {
				c.onError()
			}
		case <-c.quitChan:
			c.OnMessage("Stop logging")
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
			data, err := kwp.ReadMemoryByAddress(ctx, int(read.Address), int(read.Length))
			if err != nil {
				read.Complete(err)
				continue
			}
			read.Data = data
			read.Complete(nil)
		case write := <-c.writeChan:
			toWrite := min(36, write.Length)

			if err := kwp.WriteDataByAddress(ctx, write.Address, write.Data[:toWrite]); err != nil {
				write.Complete(err)
				continue
			}
			write.Data = write.Data[toWrite:]
			write.Address += uint32(toWrite)
			write.Length -= toWrite
			if write.Length > 0 {
				select {
				case c.writeChan <- write:
				default:
					log.Println("kisskorv updateChan full")
				}
				continue
			}
			write.Complete(nil)

		case <-t.C:
			timeStamp = time.Now()
			if len(c.Symbols) == 0 {
				testerPresent()
				continue
			}

			databuff, err := kwp.ReadDataByIdentifier(ctx, 0xF0)
			if err != nil {
				c.onError()
				c.OnMessage(err.Error())
				continue
			}

			if len(databuff) != int(expectedPayloadSize) {
				c.onError()
				c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
				continue
				//return fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff))
			}

			r := bytes.NewReader(databuff)
			for _, va := range c.Symbols {
				if va.Number < 0 {
					if va.Number <= -1000 {
						if ca, ok := cl.Adapter().(gocan.ADCCapable); ok {
							adcNumber := -va.Number - 1000
							val, err := ca.GetADCValue(ctx, adcNumber)
							if err != nil {
								c.onError()
								c.OnMessage(err.Error())
								continue
							}
							c.sysvars.Set(va.Name, float64(val))
							if err := ebus.Publish(va.Name, float64(val)); err != nil {
								c.onError()
								c.OnMessage(err.Error())
							}
							continue
						}
					} else {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
					}
					continue
				}
				if err := va.Read(r); err != nil {
					log.Printf("data ex %d %X len %d", expectedPayloadSize, databuff, len(databuff))
					c.onError()
					c.OnMessage(err.Error())
					break
				}
				if va.Name == "DisplProt.AD_Scanner" {
					ebus.Publish(va.Name, adConverter(va.Float64()))
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

			if err := c.lw.Write(c.sysvars, c.Symbols, timeStamp, order); err != nil {
				c.onError()
				c.OnMessage("failed to write log: " + err.Error())
			}
			c.captureCount++
			c.cps++
			if c.captureCount%15 == 0 {
				c.CaptureCounter(c.captureCount)
			}
		}
	}
}

func initT7logging(ctx context.Context, kwp *kwp2000.Client, symbols []*symbol.Symbol, onMessage func(string)) error {
	if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
		return errors.New("failed to start session")
	}
	onMessage("Connected to ECU")

	granted, err := kwp.RequestSecurityAccess(ctx, false)
	if err != nil {
		return errors.New("failed to request security access")
	}

	if !granted {
		onMessage("Security access not granted!")
	} else {
		onMessage("Security access granted")
	}

	if err := kwp.ClearDynamicallyDefineLocalId(ctx); err != nil {
		return errors.New("failed to clear dynamic register")
	}
	onMessage("Cleared dynamic register")

	dpos := 0
	for _, sym := range symbols {
		if sym.Number < 0 {
			continue
		}
		onMessage("Defining " + sym.Name)
		if err := kwp.DynamicallyDefineLocalIdRequest(ctx, dpos, sym); err != nil {
			return errors.New("failed to define dynamic register")
		}
		dpos++
		time.Sleep(12 * time.Millisecond)
	}
	onMessage("Configured dynamic register")
	return nil
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

func newDisplProtADConverterT7(wbl WidebandConfig) func(float64) float64 {
	return func(value float64) float64 {
		voltage := (value / 1023) * (wbl.MaximumVoltageWideband - wbl.MinimumVoltageWideband)
		voltage = clamp(voltage, wbl.MinimumVoltageWideband, wbl.MaximumVoltageWideband)
		steepness := (wbl.High - wbl.Low) / (wbl.MaximumVoltageWideband - wbl.MinimumVoltageWideband)
		return wbl.Low + (steepness * (voltage - wbl.MinimumVoltageWideband))

	}
}
