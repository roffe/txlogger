package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/t5can"
)

type T5Client struct {
	BaseLogger
}

func NewT5(cfg Config, lw LogWriter) (IClient, error) {
	return &T5Client{BaseLogger: NewBaseLogger(cfg, lw)}, nil
}

func (c *T5Client) Start() error {
	c.ErrorCounter(0)
	defer c.secondTicker.Stop()
	defer c.lw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := gocan.NewWithOpts(ctx, c.Device)
	if err != nil {
		return err
	}
	defer cl.Close()

	t := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer t.Stop()
	t5 := t5can.NewClient(cl)

	order := make([]string, len(c.Symbols))
	for n, s := range c.Symbols {
		//		log.Println(s.String())
		order[n] = s.Name
		s.Correctionfactor = 0.1
	}

	if err := c.setupWBL(ctx, cl); err != nil {
		return err
	}

	if c.lamb != nil {
		defer c.lamb.Stop()
		order = append(order, EXTERNALWBLSYM)
	}

	var expectedPayloadSize uint16
	if c.txbridge {
		if err := cl.Send(gocan.SystemMsg, []byte("5"), gocan.Outgoing); err != nil {
			return err
		}

		var symbollist []byte
		for _, sym := range c.Symbols {
			symbollist = binary.LittleEndian.AppendUint32(symbollist, sym.SramOffset)
			symbollist = binary.LittleEndian.AppendUint16(symbollist, sym.Length)
			expectedPayloadSize += sym.Length

			// deletelog.Printf("Symbol: %s, offset: %X, length: %d\n", sym.Name, sym.SramOffset, sym.Length)
		}
		cmd := &serialcommand.SerialCommand{
			Command: 'd',
			Data:    symbollist,
		}
		payload, err := cmd.MarshalBinary()
		if err != nil {
			return err
		}
		if err := cl.Send(gocan.SystemMsg, payload, gocan.Outgoing); err != nil {
			return err
		}
		c.OnMessage("Symbol list configured")
	}

	err = retry.Do(func() error {
		tx := cl.Subscribe(ctx, gocan.SystemMsgDataResponse)
		defer tx.Close()

		messages := cl.Subscribe(ctx, gocan.SystemMsg)
		defer messages.Close()

		if c.txbridge {
			t.Stop()
			if err := cl.Send(gocan.SystemMsg, []byte("r"), gocan.Outgoing); err != nil {
				return err
			}
		}

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
				c.OnMessage("Stopped logging..")
				return nil
			case <-c.secondTicker.C:
				c.FpsCounter(c.cps)
				if c.errPerSecond > 5 {
					c.errPerSecond = 0
					return fmt.Errorf("too many errors per second")
				}
				c.cps = 0
				c.errPerSecond = 0
			case symbols := <-c.symbolChan:
				_ = symbols
			case read := <-c.readChan:
				if c.txbridge {
					// log.Println(read.Length)
					toRead := min(234, read.Length)
					read.Length -= toRead
					cmd := serialcommand.SerialCommand{
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
						c.onError()
						c.OnMessage(err.Error())
						continue
					}
					frame := gocan.NewFrame(gocan.SystemMsg, payload, gocan.Outgoing)
					resp, err := cl.SendAndWait(ctx, frame, 300*time.Millisecond, gocan.SystemMsgDataRequest)
					if err != nil {
						read.Complete(err)
						continue
					}
					read.Data = append(read.Data, resp.Data...)
					if read.Length > 0 {
						c.readChan <- read
					} else {
						read.Complete(nil)
					}
					continue
				}
				data, err := t5.ReadRam(ctx, read.Address, read.Length)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}
				read.Data = data
				read.Complete(nil)
			case upd := <-c.writeChan:
				upd.Complete(fmt.Errorf("not implemented"))
			case <-t.C:
				ts := time.Now()
				for _, sym := range c.Symbols {
					resp, err := t5.ReadRam(ctx, sym.SramOffset, uint32(sym.Length))
					if err != nil {
						c.onError()
						c.OnMessage(err.Error())
						continue
					}
					r := bytes.NewReader(resp)
					if err := sym.Read(r); err != nil {
						return err
					}
					val := c.converto(sym.Name, sym.Bytes())
					c.sysvars.Set(sym.Name, val)
					if err := ebus.Publish(sym.Name, val); err != nil {
						c.onError()
						c.OnMessage(err.Error())
					}
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

				if err := c.lw.Write(c.sysvars, []*symbol.Symbol{}, ts, order); err != nil {
					return err
				}
				c.captureCount++
				c.cps++
				if c.captureCount%15 == 0 {
					c.CaptureCounter(c.captureCount)
				}
			case msg, ok := <-tx.Chan():
				if !ok {
					return retry.Unrecoverable(errors.New("txbridge sub closed"))
				}

				if msg.Length() != int(expectedPayloadSize+4) {
					c.onError()
					c.OnMessage(fmt.Sprintf("expected %d bytes, got %d", expectedPayloadSize+4, msg.Length()))
					continue
				}

				r := bytes.NewReader(msg.Data)
				binary.Read(r, binary.LittleEndian, &c.currtimestamp)

				if c.firstTime.IsZero() {
					c.firstTime = time.Now()
					c.firstTimestamp = c.currtimestamp
				}

				timeStamp := c.calculateCompensatedTimestamp()

				for _, sym := range c.Symbols {
					if err := sym.Read(r); err != nil {
						return err
					}
					val := c.converto(sym.Name, sym.Bytes())
					c.sysvars.Set(sym.Name, val)
					if err := ebus.Publish(sym.Name, val); err != nil {
						c.onError()
						c.OnMessage(err.Error())
					}
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					if err := ebus.Publish(EXTERNALWBLSYM, lambda); err != nil {
						c.onError()
						c.OnMessage(err.Error())
					}
				}

				if err := c.lw.Write(c.sysvars, []*symbol.Symbol{}, timeStamp, order); err != nil {
					return err
				}

				c.captureCount++
				c.cps++
				if c.captureCount%15 == 0 {
					c.CaptureCounter(c.captureCount)
				}
			}
		}
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(3),
		retry.OnRetry(func(n uint, err error) {
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	return err
}

const (
	correctionForMapsensor = 1.0
)

func (c *T5Client) converto(name string, data []byte) float64 {
	switch name {
	case "P_medel", "P_Manifold10", "P_Manifold", "Max_tryck", "Regl_tryck":
		// inlet manifold pressure
		return ConvertByteStringToDouble(data)*correctionForMapsensor*0.01 - 1
	case "Lambdaint":
		return ConvertByteStringToDouble(data)
	case "Lufttemp", "Kyl_temp":
		retval := ConvertByteStringToDouble(data)
		if retval > 128 {
			retval = -(256 - retval)
		}
		return retval
	case "Rpm":
		return ConvertByteStringToDouble(data) * 10
	case "AD_sond":
		// should average, no the realtime panel does that
		return ConvertByteStringToDouble(data)
		// fix
		// retval = ConvertToAFR(retval)
	case "AD_EGR":
		value := ConvertByteStringToDouble(data)
		voltage := (value / 255) * (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
		voltage = clamp(voltage, c.WidebandConfig.MinimumVoltageWideband, c.WidebandConfig.MaximumVoltageWideband)
		steepness := (c.WidebandConfig.HighAFR - c.WidebandConfig.LowAFR) / (c.WidebandConfig.MaximumVoltageWideband - c.WidebandConfig.MinimumVoltageWideband)
		return c.WidebandConfig.LowAFR + (steepness * (voltage - c.WidebandConfig.MinimumVoltageWideband))
		// return ((lambAt5v-lambAt0v)/255)*ConvertByteStringToDouble(data) + lambAt0v
	case "Pgm_status":
		// now what, just pass it on in a seperate structure
		// fix
		return ConvertByteStringToDoubleStatus(data)
	case "Insptid_ms10":
		// return value using multiplication instead of division
		return ConvertByteStringToDouble(data) * 0.1
		//return ConvertByteStringToDouble(data) / 10

	case "Lacc_mangd", "Acc_mangd", "Lret_mangd", "Ret_mangd":
		// 4 values in one variable, one for each cylinder
		return ConvertByteStringToDouble(data)
	case "Ign_angle":
		retval := ConvertByteStringToDouble(data)
		if retval > 32000 {
			retval = -(65536 - retval)
		}
		return retval / 10
	case "Knock_offset1", "Knock_offset2", "Knock_offset3", "Knock_offset4", "Knock_offset1234":
		retval := ConvertByteStringToDouble(data)
		if retval > 32000 {
			retval = -(65536 - retval)
		}
		return retval / 10
	case "Medeltrot":
		//TODO: should substract trot_min from this value?
		return ConvertByteStringToDouble(data) - 34
	case "Apc_decrese":
		return ConvertByteStringToDouble(data) * correctionForMapsensor * 0.01
	case "P_fak", "I_fak", "D_fak":
		retval := ConvertByteStringToDouble(data)
		if retval > 32000 {
			retval = -(65535 - retval)
		}
		return retval
	case "PWM_ut10":
		return ConvertByteStringToDouble(data)
	case "Knock_count_cyl1", "Knock_count_cyl2", "Knock_count_cyl3", "Knock_count_cyl4":
		return ConvertByteStringToDouble(data)
	case "Knock_average":
		return ConvertByteStringToDouble(data)
	case "Bil_hast":
		return ConvertByteStringToDouble(data)
	case "TQ":
		return ConvertByteStringToDouble(data) * correctionForMapsensor
	case "Batt_volt":
		return ConvertByteStringToDouble(data) * 0.1
	default:
		return ConvertByteStringToDouble(data)
	}
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func ConvertByteStringToDouble(ecudata []byte) float64 {
	var retval float64
	// Iterate over the bytes in ecudata and accumulate the result
	for i := 0; i < len(ecudata); i++ {
		// Multiply the current byte by the appropriate power of 256 and add it to the result
		retval += float64(ecudata[i]) * math.Pow(256, float64(len(ecudata)-i-1))
	}
	return retval
}

func ConvertByteToI16(ecudata []byte) int16 {
	var retval int16
	// Iterate over the bytes in ecudata and accumulate the result
	for i := 0; i < len(ecudata); i++ {
		// Multiply the current byte by the appropriate power of 256 and add it to the result
		retval += int16(ecudata[i]) * int16(math.Pow(256, float64(len(ecudata)-i-1)))
	}
	return retval
}

func ConvertByteStringToDoubleStatus(ecudata []byte) float64 {
	var retval float64
	// Iterate over the bytes in ecudata and accumulate the result
	for i := range len(ecudata) {
		// Multiply the current byte by the appropriate power of 256 and add it to the result
		retval += float64(ecudata[i]) * math.Pow(256, float64(i))
	}
	return retval
}

func ConvertByteStringToDoubleStatus2(ecudata []byte) float64 {
	var retval float64 = 0

	switch len(ecudata) {
	case 4:
		retval = float64(ecudata[3]) * 256 * 256 * 256
		retval += float64(ecudata[2]) * 256 * 256
		retval += float64(ecudata[1]) * 256
		retval += float64(ecudata[0])
	case 5:
		retval = float64(ecudata[4]) * 256 * 256 * 256 * 256
		retval += float64(ecudata[3]) * 256 * 256 * 256
		retval += float64(ecudata[2]) * 256 * 256
		retval += float64(ecudata[1]) * 256
		retval += float64(ecudata[0])
	case 6:
		retval = float64(ecudata[5]) * 256 * 256 * 256 * 256 * 256
		retval += float64(ecudata[4]) * 256 * 256 * 256 * 256
		retval += float64(ecudata[3]) * 256 * 256 * 256
		retval += float64(ecudata[2]) * 256 * 256
		retval += float64(ecudata[1]) * 256
		retval += float64(ecudata[0])
	case 7:
		retval = float64(ecudata[6]) * 256 * 256 * 256 * 256 * 256 * 256
		retval += float64(ecudata[5]) * 256 * 256 * 256 * 256 * 256
		retval += float64(ecudata[4]) * 256 * 256 * 256 * 256
		retval += float64(ecudata[3]) * 256 * 256 * 256
		retval += float64(ecudata[2]) * 256 * 256
		retval += float64(ecudata[1]) * 256
		retval += float64(ecudata[0])
	case 8:
		retval = float64(ecudata[7]) * 256 * 256 * 256 * 256 * 256 * 256 * 256
		retval += float64(ecudata[6]) * 256 * 256 * 256 * 256 * 256 * 256
		retval += float64(ecudata[5]) * 256 * 256 * 256 * 256 * 256
		retval += float64(ecudata[4]) * 256 * 256 * 256 * 256
		retval += float64(ecudata[3]) * 256 * 256 * 256
		retval += float64(ecudata[2]) * 256 * 256
		retval += float64(ecudata[1]) * 256
		retval += float64(ecudata[0])
	}

	return retval
}
