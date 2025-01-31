package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

type T7Client struct {
	symbolChan chan []*symbol.Symbol
	updateChan chan *WriteRequest
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
		updateChan: make(chan *WriteRequest, 10),
		readChan:   make(chan *ReadRequest, 10),
		quitChan:   make(chan struct{}),
		sysvars:    NewThreadSafeMap(),
		lw:         lw,
	}, nil
}

func (c *T7Client) Close() {
	c.closeOnce.Do(func() {
		close(c.quitChan)
		time.Sleep(200 * time.Millisecond)
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
	c.ErrorCounter(c.errCount)
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
	go c.startBroadcastListener(bctx, cl)

	c.OnMessage("Watching for broadcast messages")
	<-time.After(350 * time.Millisecond)
	order := c.sysvars.Keys()
	sort.StringSlice(order).Sort()
	c.OnMessage(fmt.Sprintf("Found %s", order))

	if len(order) == 0 {
		bcancel()
	}

	// Wideband lambda
	cfg := &WBLConfig{
		WBLType:  c.Config.WidebandConfig.Type,
		Port:     c.Config.WidebandConfig.Port,
		Log:      c.OnMessage,
		Txbridge: txbridge,
	}

	c.lamb, err = NewWBL(ctx, cl, cfg)
	if err != nil {
		return fmt.Errorf("failed to create wideband lambda: %w", err)
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
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

	if txbridge {
		if err := cl.SendFrame(adapter.SystemMsg, []byte("7"), gocan.Outgoing); err != nil {
			return err
		}
	}

	kwp := kwp2000.New(cl)
	err = retry.Do(func() error {
		if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {

			return retry.Unrecoverable(errors.New("failed to start session"))
		}
		defer func() {
			kwp.StopSession(ctx)
			time.Sleep(50 * time.Millisecond)
		}()

		c.OnMessage("Connected to ECU")

		granted, err := kwp.RequestSecurityAccess(ctx, false)
		if err != nil {
			return errors.New("failed to request security access")
		}

		if !granted {
			c.OnMessage("Security access not granted!")
		} else {
			c.OnMessage("Security access granted")
		}

		if err := kwp.ClearDynamicallyDefineLocalId(ctx); err != nil {
			return errors.New("failed to clear dynamic register")
		}
		c.OnMessage("Cleared dynamic register")

		dpos := 0
		for _, sym := range c.Symbols {
			if sym.Skip {
				continue
			}
			//			log.Println("Defining", sym.Name, dpos)
			if err := kwp.DynamicallyDefineLocalIdRequest(ctx, dpos, sym); err != nil {
				return errors.New("failed to define dynamic register")
			}
			dpos++
			time.Sleep(12 * time.Millisecond)
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

		tx := cl.Subscribe(ctx, adapter.SystemMsgDataResponse, adapter.SystemMsgError)
		defer tx.Close()

		if txbridge {
			//			log.Println("stopped timer, using txbridge")
			t.Stop()
			if err := cl.SendFrame(adapter.SystemMsg, []byte("r"), gocan.Outgoing); err != nil {
				return err
			}
		}

		//buf := bytes.NewBuffer(nil)
		var firstTime time.Time
		var firstTimestamp uint32
		var databuff []byte
		var currtimestamp uint32
		for {
			select {
			case <-c.quitChan:
				c.OnMessage("Stopped logging..")
				return nil
			case <-secondTicker.C:
				c.FpsCounter(cps)
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
					toRead := min(235, read.Length)
					read.Length -= toRead
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
					read.Address += uint32(toRead)
					payload, err := cmd.MarshalBinary()
					if err != nil {
						c.onError(err)
						continue
					}
					frame := gocan.NewFrame(adapter.SystemMsg, payload, gocan.Outgoing)
					resp, err := cl.SendAndPoll(ctx, frame, 3*time.Second, adapter.SystemMsgDataRequest)
					if err != nil {
						read.Complete(err)
						continue
					}
					read.Data = append(read.Data, resp.Data()...)
					if read.Length > 0 {
						c.readChan <- read
					} else {
						read.Complete(nil)
					}
					continue
				}

				data, err := kwp.ReadMemoryByAddress(ctx, int(read.Address), int(read.Length))
				if err != nil {
					read.Complete(err)
					continue
				}
				read.Data = data
				read.Complete(nil)
			case write := <-c.updateChan:
				if txbridge {
					toRead := min(235, write.Length)
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

					write.Data = write.Data[toRead:] // remove the data we just sent
					write.Address += uint32(toRead)
					write.Length -= toRead

					payload, err := cmd.MarshalBinary()
					if err != nil {
						write.Complete(err)
						continue
					}

					frame := gocan.NewFrame(adapter.SystemMsg, payload, gocan.Outgoing)

					resp, err := cl.SendAndPoll(ctx, frame, 1*time.Second, adapter.SystemMsgWriteResponse, adapter.SystemMsgError)
					if err != nil {
						write.Complete(err)
						continue
					}

					if resp.Identifier() == adapter.SystemMsgError {
						write.Complete(fmt.Errorf("error response"))
						continue
					}

					if write.Length > 0 {
						select {
						case c.updateChan <- write:
						default:
							log.Println("kisskorv updateChan full")
						}
						continue
					}
					write.Complete(nil)
					continue
				}

				write.Complete(kwp.WriteDataByAddress(ctx, write.Address, write.Data))

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
					c.CaptureCounter(count)
				}
			case msg, ok := <-tx.C():
				if !ok {
					return retry.Unrecoverable(errors.New("txbridge recv channel closed"))
				}
				if msg.Identifier() == adapter.SystemMsgError {
					data := msg.Data()
					switch data[0] {
					case 0x31:
						c.onError(fmt.Errorf("read timeout"))
					case 0x06:
						c.onError(fmt.Errorf("invalid sequence"))
					}
					continue
				}

				databuff = msg.Data()
				if len(databuff) != int(expectedPayloadSize+4) {
					c.onError(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize+4, len(databuff)))
					log.Printf("unexpected data %X", databuff)
					continue
					//return retry.Unrecoverable(fmt.Errorf("expected %d bytes, got %d", expectedPayloadSize, len(databuff)))
				}

				r := bytes.NewReader(databuff)

				binary.Read(r, binary.LittleEndian, &currtimestamp)

				if firstTime.IsZero() {
					firstTime = time.Now()
					firstTimestamp = currtimestamp
				}

				timeStamp := calculateCompensatedTimestamp(firstTime, firstTimestamp, currtimestamp)

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

					if err := ebus.Publish(va.Name, va.Float64()); err != nil {
						c.onError(err)
					}
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
					c.CaptureCounter(count)
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

func calculateOptimalReadSize(remainingBytes uint32) uint32 {
	const (
		maxReadSize          = 235 // Maximum bytes we can request at once
		singleByteThreshold  = 1
		multiBytePayloadSize = 6 // Number of bytes per payload for multi-byte reads
		minEfficientRead     = 7 // Minimum size for an efficient read
	)

	// If only 1 byte left, just read it directly
	if remainingBytes <= singleByteThreshold {
		return remainingBytes
	}

	// Calculate initial optimal read based on complete payloads
	maxPayloads := maxReadSize / multiBytePayloadSize
	optimalSize := uint32(maxPayloads * multiBytePayloadSize)

	if remainingBytes <= optimalSize {
		return remainingBytes
	}

	// If reading optimalSize would leave a small inefficient remainder,
	// reduce this read size to ensure the next read is efficient
	remainderAfterRead := remainingBytes - optimalSize
	if remainderAfterRead > 0 && remainderAfterRead < minEfficientRead {
		// Calculate how many complete payloads we need to shave off
		payloadsToReduce := (minEfficientRead - remainderAfterRead + multiBytePayloadSize - 1) / multiBytePayloadSize
		return optimalSize - (payloadsToReduce * multiBytePayloadSize)
	}

	return optimalSize
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

func calculateCompensatedTimestamp(firstTime time.Time, firstTimestamp, currentTimestamp uint32) time.Time {
	return firstTime.Add(time.Duration(currentTimestamp-firstTimestamp) * time.Millisecond)
}

func calculateCompensatedTimestampZ(firstTime time.Time, firstTimestamp, currentTimestamp uint32) time.Time {
	// Calculate elapsed milliseconds since the first reading
	elapsedMs := currentTimestamp - firstTimestamp

	// Calculate the compensated timestamp
	compensatedTime := firstTime.Add(time.Duration(elapsedMs) * time.Millisecond)

	// Calculate drift between actual system time and compensated time
	actualTime := time.Now()
	driftDuration := actualTime.Sub(compensatedTime)

	// Convert drift to milliseconds for easier reading
	driftMs := float64(driftDuration.Nanoseconds()) / float64(time.Millisecond)

	// Log if drift is more than 100ms

	log.Printf("Timestamp drift: %.2fms (System: %v, Compensated: %v, ECU elapsed: %dms)",
		driftMs,
		actualTime.Format("15:04:05.000"),
		compensatedTime.Format("15:04:05.000"),
		elapsedMs)

	return compensatedTime
}
