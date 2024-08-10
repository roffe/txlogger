package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/t5can"
)

type T5Client struct {
	symbolChan chan []*symbol.Symbol
	updateChan chan *RamUpdate
	readChan   chan *ReadRequest
	quitChan   chan struct{}
	closeOnce  sync.Once
	lw         LogWriter
	Config

	errCount     int
	errPerSecond int
}

func NewT5(cfg Config, lw LogWriter) (IClient, error) {
	return &T5Client{
		Config:     cfg,
		lw:         lw,
		symbolChan: make(chan []*symbol.Symbol, 1),
		updateChan: make(chan *RamUpdate, 1),
		readChan:   make(chan *ReadRequest, 1),
		quitChan:   make(chan struct{}),
	}, nil
}

func (c *T5Client) Close() {
	c.closeOnce.Do(func() {
		close(c.quitChan)
	})
}

func (c *T5Client) SetRAM(address uint32, data []byte) error {
	upd := NewRamUpdate(address, data)
	c.updateChan <- upd
	return upd.Wait()
}

func (c *T5Client) GetRAM(address uint32, length uint32) ([]byte, error) {
	req := NewReadRequest(address, length)
	c.readChan <- req
	return req.Data, req.Wait()
}

func (c *T5Client) SetSymbols(symbols []*symbol.Symbol) error {
	select {
	case c.symbolChan <- symbols:
	default:
		return fmt.Errorf("pending")
	}
	return nil
}

func (c *T5Client) Start() error {
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

	secondTicker := time.NewTicker(1000 * time.Millisecond)
	defer secondTicker.Stop()

	t := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer t.Stop()
	cps := 0
	t5 := t5can.NewClient(cl)
	count := 0
	sysvars := NewThreadSafeMap()
	order := make([]string, len(c.Symbols))
	for n, s := range c.Symbols {
		log.Println(s.String())
		order[n] = s.Name
		s.Correctionfactor = 0.1
	}

	err = retry.Do(func() error {
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
				_ = symbols
			case read := <-c.readChan:
				_ = read
			case upd := <-c.updateChan:
				_ = upd
			case <-t.C:
				ts := time.Now()
				for _, sym := range c.Symbols {
					resp, err := t5.ReadRam(ctx, sym.SramOffset, uint32(sym.Length))
					if err != nil {
						c.onError(err)
						continue
					}

					r := bytes.NewReader(resp)
					if err := sym.Read(r); err != nil {
						return err
					}
					val := c.converto(sym.Name, sym.Bytes())
					sysvars.Set(sym.Name, val)
					if err := ebus.Publish(sym.Name, val); err != nil {
						c.onError(err)
					}
				}
				count++
				cps++
				if count%15 == 0 {
					c.CaptureCounter.Set(count)
				}
				if err := c.lw.Write(sysvars, []*symbol.Symbol{}, ts, order); err != nil {
					return err
				}
			}
		}
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(3),
		retry.OnRetry(func(n uint, err error) {
			c.onError(fmt.Errorf("retry %d: %w", n, err))
		}),
		retry.LastErrorOnly(true),
	)
	return err
}

func (c *T5Client) onError(err error) {
	c.errCount++
	c.errPerSecond++
	c.ErrorCounter.Set(c.errCount)
	c.OnMessage(err.Error())
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
		// retval = ConvertByteStringToDoubleStatus(data)
		return 0
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
	for i := 0; i < len(ecudata); i++ {
		// Multiply the current byte by the appropriate power of 256 and add it to the result
		retval += float64(ecudata[i]) * math.Pow(256, float64(i))
	}
	return retval
}
