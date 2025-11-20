package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
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
		return err
	}
	defer cl.Close()

	t := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer t.Stop()
	t5 := t5can.NewClient(cl)

	order := make([]string, len(c.Symbols))
	for n, s := range c.Symbols {
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

	tx := cl.Subscribe(ctx, gocan.SystemMsgDataResponse)
	defer tx.Close()

	converto := newT5Converter(c.WidebandConfig)

	go func() {
		defer cl.Close()
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
				data, err := t5.ReadRam(ctx, read.Address, read.Length)
				if err != nil {
					c.onError()
					c.OnMessage(err.Error())
					continue
				}
				read.Data = data
				read.Complete(nil)
			case upd := <-c.writeChan:
				if err := t5.WriteRam(ctx, upd.Address, upd.Data); err != nil {
					upd.Complete(err)
					break
				}
				upd.Complete(nil)
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
						c.OnMessage("failed to read symbol " + sym.Name + ": " + err.Error())
						return
					}
					val := converto(sym.Name, sym.Bytes())
					c.sysvars.Set(sym.Name, val)
					ebus.Publish(sym.Name, val)
				}

				if c.lamb != nil {
					lambda := c.lamb.GetLambda()
					c.sysvars.Set(EXTERNALWBLSYM, lambda)
					ebus.Publish(EXTERNALWBLSYM, lambda)
				}

				if err := c.lw.Write(c.sysvars, []*symbol.Symbol{}, ts, order); err != nil {
					c.OnMessage("failed to write log: " + err.Error())
					return
				}
				c.onCapture()
			}
		}
	}()
	return cl.Wait(ctx)
}

const (
	correctionForMapsensor = 1.0
)

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
	switch len(ecudata) {
	case 0:
		return 0
	case 4:
		return float64(binary.LittleEndian.Uint32(ecudata))
	case 8:
		return float64(binary.LittleEndian.Uint64(ecudata))
	}

	n := min(len(ecudata), 8)

	var u uint64
	for i := 0; i < n; i++ {
		u |= uint64(ecudata[i]) << (8 * uint(i))
	}
	return float64(u)
}

func newT5Converter(wb WidebandConfig) func(string, []byte) float64 {
	return func(name string, data []byte) float64 {
		switch name {
		case "P_medel", "P_Manifold10", "P_Manifold", "Max_tryck", "Regl_tryck": // inlet manifold pressure
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
			voltage := (value / 255) * (wb.MaximumVoltageWideband - wb.MinimumVoltageWideband)
			voltage = clamp(voltage, wb.MinimumVoltageWideband, wb.MaximumVoltageWideband)
			steepness := (wb.High - wb.Low) / (wb.MaximumVoltageWideband - wb.MinimumVoltageWideband)
			return wb.Low + (steepness * (voltage - wb.MinimumVoltageWideband))
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
}
