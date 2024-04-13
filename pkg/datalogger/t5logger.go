package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

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

func NewT5(cfg Config, lw LogWriter) (Provider, error) {
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
	vars := NewThreadSafeMap()
	order := make([]string, len(c.Symbols))
	for n, s := range c.Symbols {
		log.Println(s.String())
		order[n] = s.Name
		s.Correctionfactor = 0.2
	}
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
					c.onError(err)
					continue
				}
				val := converto(sym)
				if err := ebus.Publish(sym.Name, val); err != nil {
					c.onError(err)
				}
				vars.Set(sym.Name, val)
			}
			count++
			cps++
			if count%15 == 0 {
				c.CaptureCounter.Set(count)
			}

			if err := c.lw.Write(vars, []*symbol.Symbol{}, ts, order); err != nil {
				c.onError(err)
			}
		}
	}
}

func (c *T5Client) onError(err error) {
	c.errCount++
	c.errPerSecond++
	c.ErrorCounter.Set(c.errCount)
	c.OnMessage(err.Error())
}

func converto(sym *symbol.Symbol) float64 {
	var retval float64
	correctionForMapsensor := 1.0
	switch sym.Name {
	case "P_medel", "P_Manifold10", "P_Manifold", "Max_tryck", "Regl_tryck":
		// inlet manifold pressure
		retval = ConvertByteStringToDouble(sym.Bytes())
		retval *= correctionForMapsensor
		retval *= 0.01
		retval -= 1

	case "Lufttemp":
		retval = ConvertByteStringToDouble(sym.Bytes())
		if retval > 128 {
			retval = -(256 - retval)
		}

	case "Kyl_temp":
		retval = ConvertByteStringToDouble(sym.Bytes())
		if retval > 128 {
			retval = -(256 - retval)
		}

	case "Rpm":
		retval = ConvertByteStringToDouble(sym.Bytes())
		retval *= 10 // factor 10

	case "AD_sond":
		// should average, no the realtime panel does that
		retval = ConvertByteStringToDouble(sym.Bytes())
		// fix
		// retval = ConvertToAFR(retval)

	case "AD_EGR":
		retval = ConvertByteStringToDouble(sym.Bytes())
		// fix
		// retval = ConvertToWidebandAFR(retval)
		//retval = ConvertToAFR(retval)

	case "Pgm_status":
		// now what, just pass it on in a seperate structure
		// fix
		// retval = ConvertByteStringToDoubleStatus(sym.Bytes())

	case "Insptid_ms10":
		retval = ConvertByteStringToDouble(sym.Bytes())
		retval /= 10

	case "Lacc_mangd", "Acc_mangd", "Lret_mangd", "Ret_mangd":
		retval = ConvertByteStringToDouble(sym.Bytes())
		// 4 values in one variable, one for each cylinder

	case "Ign_angle":
		retval = ConvertByteStringToDouble(sym.Bytes())
		if retval > 32000 {
			retval = -(65536 - retval)
		}
		retval /= 10

	case "Knock_offset1", "Knock_offset2", "Knock_offset3", "Knock_offset4", "Knock_offset1234":
		retval = ConvertByteStringToDouble(sym.Bytes())
		if retval > 32000 {
			retval = -(65536 - retval)
		}
		retval /= 10

	case "Medeltrot":
		retval = ConvertByteStringToDouble(sym.Bytes())
		retval -= 34
		//TODO: should substract trot_min from this value?
	case "Apc_decrese":
		retval = ConvertByteStringToDouble(sym.Bytes())
		retval *= correctionForMapsensor
		retval *= 0.01 // to bar!

	case "P_fak", "I_fak", "D_fak":
		retval = ConvertByteStringToDouble(sym.Bytes())
		if retval > 32000 {
			retval = -(65535 - retval)
		}

	case "PWM_ut10":
		retval = ConvertByteStringToDouble(sym.Bytes())

	case "Knock_count_cyl1", "Knock_count_cyl2", "Knock_count_cyl3", "Knock_count_cyl4":
		retval = ConvertByteStringToDouble(sym.Bytes())

	case "Knock_average":
		retval = ConvertByteStringToDouble(sym.Bytes())

	case "Bil_hast":
		retval = ConvertByteStringToDouble(sym.Bytes())

	case "TQ":
		retval = ConvertByteStringToDouble(sym.Bytes())
		retval *= correctionForMapsensor
	default:
		retval = ConvertByteStringToDouble(sym.Bytes())

	}
	return retval
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

func ConvertByteStringToDoubleStatus(ecudata []byte) float64 {
	var retval float64
	// Iterate over the bytes in ecudata and accumulate the result
	for i := 0; i < len(ecudata); i++ {
		// Multiply the current byte by the appropriate power of 256 and add it to the result
		retval += float64(ecudata[i]) * math.Pow(256, float64(i))
	}
	return retval
}

func ConvertByteStringToDoubleStatus2(ecudata []byte) float64 {
	var retval float64 = 0
	switch len(ecudata) {
	case 4:
		retval = float64(ecudata[3])*256*256*256 +
			float64(ecudata[2])*256*256 +
			float64(ecudata[1])*256 +
			float64(ecudata[0])
	case 5:
		retval = float64(ecudata[4])*256*256*256*256 +
			float64(ecudata[3])*256*256*256 +
			float64(ecudata[2])*256*256 +
			float64(ecudata[1])*256 +
			float64(ecudata[0])
	case 6:
		retval = float64(ecudata[5])*256*256*256*256*256 +
			float64(ecudata[4])*256*256*256*256 +
			float64(ecudata[3])*256*256*256 +
			float64(ecudata[2])*256*256 +
			float64(ecudata[1])*256 +
			float64(ecudata[0])
	case 7:
		retval = float64(ecudata[6])*256*256*256*256*256*256 +
			float64(ecudata[5])*256*256*256*256*256 +
			float64(ecudata[4])*256*256*256*256 +
			float64(ecudata[3])*256*256*256 +
			float64(ecudata[2])*256*256 +
			float64(ecudata[1])*256 +
			float64(ecudata[0])
	case 8:
		retval = float64(ecudata[7])*256*256*256*256*256*256*256 +
			float64(ecudata[6])*256*256*256*256*256*256 +
			float64(ecudata[5])*256*256*256*256*256 +
			float64(ecudata[4])*256*256*256*256 +
			float64(ecudata[3])*256*256*256 +
			float64(ecudata[2])*256*256 +
			float64(ecudata[1])*256 +
			float64(ecudata[0])
	}
	return retval
}
