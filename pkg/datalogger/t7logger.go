package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/ebus"
)

type T7Client struct {
	*BaseLogger
}

func NewT7(cfg Config, lw LogWriter) (IClient, error) {
	return &T7Client{BaseLogger: NewBaseLogger(cfg, lw)}, nil
}

// t7BroadcastIDs are the T7 CAN broadcast frames decodeT7Broadcast understands.
// The txbridge path hands this list to the dongle ('b' command) so it collects and
// folds them into the log stream; generic adapters subscribe to them live below.
var t7BroadcastIDs = []uint16{0x1A0, 0x280, 0x3A0 /*, 0x5C0*/}

// t7BroadcastSymbols are the symbol names decodeT7Broadcast can produce. On the
// txbridge path, requested symbols in this set are sourced from the folded trailer
// instead of the KWP F0 read.
var t7BroadcastSymbols = map[string]struct{}{
	"ActualIn.n_Engine":  {},
	"Out.X_AccPedal":     {},
	"Out.X_ActualGear":   {},
	"Out.ST_BrakeLight":  {},
	"In.ST_ClutchBrake1": {},
	"In.v_Vehicle":       {},
	// "ActualIn.T_Engine": {}, // enable together with 0x5C0 in t7BroadcastIDs
}

// decodeT7Broadcast decodes one raw T7 broadcast frame into sysvars (plus a few
// ebus-only status bits). Shared by the live listener (generic adapters) and the
// txbridge path, which feeds the same bytes from the folded 'r' trailer. Guards on
// length because the folded frames carry their own dlc.
func decodeT7Broadcast(id uint16, data []byte, sysvars *ThreadSafeMap) {
	switch id {
	case 0x1A0:
		if len(data) < 6 {
			return
		}
		rpm := binary.BigEndian.Uint16(data[1:3])
		sysvars.Set("ActualIn.n_Engine", float64(rpm))
		sysvars.Set("Out.X_AccPedal", float64(data[5]))
	case 0x280:
		if len(data) < 5 {
			return
		}
		sysvars.Set("Out.X_ActualGear", float64(data[1]))
		sysvars.Set("Out.ST_BrakeLight", float64(data[2]&0x02>>1))
		sysvars.Set("In.ST_ClutchBrake1", float64(data[2]&0x08>>3))
		ebus.Publish("LIMP", float64(data[3]&0x01))
		ebus.Publish("CRUISE", float64(data[4]&0x20>>5))
		ebus.Publish("CEL", float64(data[4]&0x80>>7))
	case 0x3A0:
		if len(data) < 5 {
			return
		}
		speed := uint16(data[4]) | uint16(data[3])<<8
		sysvars.Set("In.v_Vehicle", float64(speed)*0.1)
	case 0x5C0:
		// 0x5C0 COTE_ECS: Data[1]=coolant. byte=(u8)(V+40)
		if len(data) < 2 {
			return
		}
		sysvars.Set("ActualIn.T_Engine", float64(data[1])-40)
	}
}

func t7broadcastListener(ctx context.Context, cl *gocan.Bus, sysvars *ThreadSafeMap) {
	ids := make([]uint32, len(t7BroadcastIDs))
	for i, id := range t7BroadcastIDs {
		ids[i] = uint32(id)
	}
	broadcast := cl.Subscribe(ctx, ids...)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-broadcast:
			if !ok {
				return
			}
			decodeT7Broadcast(uint16(msg.ID), msg.Bytes(), sysvars)
		}
	}
}

func (c *T7Client) Start(pctx context.Context) error {
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
		return fmt.Errorf("failed to create t7 client: %w", err)
	}
	defer cl.Close()

	// Drive everything below off the client's context so a fatal adapter error
	// (or Close) cancels the polling loop and aborts in-flight requests directly,
	// instead of relying on cl.Wait returning and the deferred cancel bouncing back.
	ctx = cl.Context()

	checkBroadcast := true
	if strings.Contains(cl.AdapterName(), "OBDLink") || strings.Contains(cl.AdapterName(), "STN") || strings.Contains(cl.AdapterName(), "ELM") {
		checkBroadcast = false
	}

	if checkBroadcast {
		bctx, bcancel := context.WithCancel(ctx)
		defer bcancel()
		go t7broadcastListener(bctx, cl, c.sysvars)

		c.OnMessage("Watching for broadcast messages")
		<-time.After(1550 * time.Millisecond)
		if found := c.sysvars.Keys(); len(found) > 0 {
			c.OnMessage(fmt.Sprintf("Found: %s", strings.Join(found, ", ")))
		} else {
			c.OnMessage("No broadcast messages found, stopping broadcast listener")
			bcancel()
		}
	}
	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
	}

	for _, sym := range c.Symbols {
		if c.sysvars.Exists(sym.Name) {
			log.Println("Skipping", sym.Name, "in broadcast")
			sym.Number = -1
			continue
		}
	}

	// Broadcast/derived values resolved above become async sysvar channels;
	// the remaining symbols (Number >= 0) are polled each tick.
	channels := c.buildChannels()

	kwp := t7kwp.New(cl)
	kwp.SetSeedKey(c.SeedKey) // custom pair extracted from the loaded binary, if any

	adConverter := NewWBLInterpolator(c.WidebandConfig)

	if err := initT7logging(ctx, kwp, c.Symbols, c.OnMessage); err != nil {
		return fmt.Errorf("failed to init t7 logging: %w", err)
	}

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

	//lastPresent := time.Now()
	//testerPresent := func() {
	//	if time.Since(lastPresent) > lastPresentInterval {
	//		if err := kwp.TesterPresent(ctx); err != nil {
	//			c.onError()
	//			c.OnMessage(err.Error())
	//			return
	//		}
	//		lastPresent = time.Now()
	//	}
	//}

	/*
		wheelSlipFN := func(s string, v float64) {
			switch s {
			case "In.v_Vehicle", "In.v_Vehicle3":
				// Front wheel speed, if this is higher than rear wheel speed, we have wheel slip
				rearSpeed := c.sysvars.Get("In.v_Vehicle2")
				if rearSpeed > 0 {
					slip := (v - rearSpeed) / rearSpeed
					c.sysvars.Set("WheelSlip."+s, slip)
					ebus.Publish("WheelSlip."+s, slip)
				}
			}
		}
		// In.v_Vehicle Left front wheel speed
		// In.v_Vehicle2 Vehicle speed, measured on the rear wheel
		// In.v_Vehicle3 Right front wheel speed

		specialFN := map[string]func(string, float64){
			"In.v_Vehicle":  wheelSlipFN,
			"In.v_Vehicle2": wheelSlipFN,
			"In.v_Vehicle3": wheelSlipFN,
		}
	*/

	router := map[string]func(s *symbol.Symbol) bool{
		"IgnKnk.fi_Offset": func(s *symbol.Symbol) bool {
			data := s.Bytes()
			if len(data) != 8 {
				return false
			}

			ioffCyl1 := int16(binary.BigEndian.Uint16(data[0:2]))
			ioffCyl2 := int16(binary.BigEndian.Uint16(data[2:4]))
			ioffCyl3 := int16(binary.BigEndian.Uint16(data[4:6]))
			ioffCyl4 := int16(binary.BigEndian.Uint16(data[6:8]))

			ebus.Publish("IgnKnk.fi_Offset.Cyl1", float64(ioffCyl1)/10)
			ebus.Publish("IgnKnk.fi_Offset.Cyl2", float64(ioffCyl2)/10)
			ebus.Publish("IgnKnk.fi_Offset.Cyl3", float64(ioffCyl3)/10)
			ebus.Publish("IgnKnk.fi_Offset.Cyl4", float64(ioffCyl4)/10)
			return true
		},
	}

	if c.WidebandConfig.ADScanner {
		router[c.WidebandConfig.ADScannerSymbol] = func(s *symbol.Symbol) bool {
			lambda := adConverter(s.Int())
			c.sysvars.Set(LAMBDAADSCANNER, lambda)
			ebus.Publish(LAMBDAADSCANNER, lambda)
			return true
		}
	}

	/*
		if c.lamb != nil {
			lambdbaChan := newFunctionChannel(EXTERNALWBLSYM, func() float64 {
				lambda := c.lamb.GetLambda()
				ebus.Publish(EXTERNALWBLSYM, lambda)
				return lambda
			})
			channels = append(channels, lambdbaChan)
		}
	*/

	go func() {
		defer cl.Close()
		defer func() {
			_ = kwp.StopSession(ctx)
			time.Sleep(50 * time.Millisecond)
		}()

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
				}

				r := bytes.NewReader(databuff)
				for _, va := range c.Symbols {

					if va.Number < 0 {
						ebus.Publish(va.Name, c.sysvars.Get(va.Name))
						continue
					}

					if err := va.Read(r); err != nil {
						log.Printf("data ex %d %X len %d", expectedPayloadSize, databuff, len(databuff))
						c.onError()
						c.OnMessage(err.Error())
						break
					}

					if fn, ok := router[va.Name]; ok && fn(va) {
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

				if err := c.lw.Write(timeStamp, channels); err != nil {
					c.onError()
					c.OnMessage("failed to write log: " + err.Error())
				}
				c.onCapture(timeStamp)
			}
		}
	}()
	return cl.Wait(ctx)
}

func initT7logging(ctx context.Context, kwp *t7kwp.Client, symbols []*symbol.Symbol, onMessage func(string)) error {
	if err := kwp.StartSession(ctx, t7kwp.INIT_MSG_ID, t7kwp.INIT_RESP_ID); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	onMessage("Connected to ECU")

	granted, err := kwp.RequestSecurityAccess(ctx, false)
	if err != nil {
		return errors.New("failed to request security access")
	}

	if !granted {
		return errors.New("security access not granted")
	} else {
		onMessage("Security access granted")
	}

	// For some fucked up reason this clears DTC's and resets adaptation!!!
	// Did we stumble on a bug in Trionic 7 ECU's firmware?
	if err := kwp.ClearDynamicallyDefineLocalId(ctx); err != nil {
		return fmt.Errorf("failed to clear dynamic register: %w", err)
	}
	onMessage("Cleared dynamic register")

	index := 0
	for _, sym := range symbols {
		if sym.Number < 0 {
			continue
		}
		onMessage("Defining " + sym.Name)

		// log.Println("Define:", sym.String())

		if err := kwp.DynamicallyDefineLocalIdBySymbolNumber(ctx, index, sym.Number); err != nil {
			return fmt.Errorf("failed to define dynamic register: %w", err)
		}
		index++
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

/*
func newDisplProtADConverterT7(wbl WidebandConfig) func(float64) float64 {
	return func(value float64) float64 {
		voltage := (value / 1023) * (wbl.MaximumVoltageWideband - wbl.MinimumVoltageWideband)
		voltage = clamp(voltage, wbl.MinimumVoltageWideband, wbl.MaximumVoltageWideband)
		steepness := (wbl.High - wbl.Low) / (wbl.MaximumVoltageWideband - wbl.MinimumVoltageWideband)
		return wbl.Low + (steepness * (voltage - wbl.MinimumVoltageWideband))
	}
}
*/
