package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/symbol"
	"golang.org/x/sync/errgroup"
)

type T7Client struct {
	dl Logger

	updateChan chan *RamUpdate
	readChan   chan *ReadRequest

	quitChan chan struct{}
	sysvars  *ThreadSafeMap

	Config
}

func NewT7(dl Logger, cfg Config) (Provider, error) {
	return &T7Client{
		dl:         dl,
		updateChan: make(chan *RamUpdate, 1),
		readChan:   make(chan *ReadRequest, 1),
		quitChan:   make(chan struct{}, 2),
		Config:     cfg,
		sysvars: &ThreadSafeMap{
			values: map[string]string{
				"ActualIn.n_Engine": "0",   // comes from 0x1A0
				"Out.X_AccPedal":    "0.0", // comes from 0x1A0
				"In.v_Vehicle":      "0.0", // comes from 0x3A0
				"Out.ST_LimpHome":   "0",   // comes from 0x280
			},
		},
	}, nil
}

func (c *T7Client) Close() {
	close(c.quitChan)
	time.Sleep(200 * time.Millisecond)
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
	file, filename, err := createLog("t7l")
	if err != nil {
		return err
	}
	defer file.Close()
	defer file.Sync()
	c.OnMessage(fmt.Sprintf("Logging to %s", filename))

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

	go func() {
		sub := cl.Subscribe(ctx, 0x1A0, 0x280, 0x3A0)
		for msg := range sub {
			switch msg.Identifier() {
			case 0x1A0:
				rpm := binary.BigEndian.Uint16(msg.Data()[1:3])
				throttle := int(msg.Data()[5])
				c.sysvars.Set("ActualIn.n_Engine", strconv.Itoa(int(rpm)))
				c.sysvars.Set("Out.X_AccPedal", strconv.Itoa(throttle)+",0")
				c.dl.SetValue("ActualIn.n_Engine", float64(rpm))
				c.dl.SetValue("Out.X_AccPedal", float64(throttle))
			case 0x280:
				data := msg.Data()[4]
				if data&0x20 == 0x20 {
					c.dl.SetValue("CRUISE", 1)
				} else {
					c.dl.SetValue("CRUISE", 0)
				}
				if data&0x80 == 0x80 {
					c.dl.SetValue("CEL", 1)
				} else {
					c.dl.SetValue("CEL", 0)
				}
				data2 := msg.Data()[3]
				if data2&0x01 == 0x01 {
					c.dl.SetValue("LIMP", 1)
				} else {
					c.dl.SetValue("LIMP", 0)
				}
			case 0x3A0:
				speed := uint16(msg.Data()[4]) | uint16(msg.Data()[3])<<8
				realSpeed := float64(speed) / 10
				c.sysvars.Set("In.v_Vehicle", strconv.FormatFloat(realSpeed, 'f', 1, 64))
				c.dl.SetValue("In.v_Vehicle", realSpeed)
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
			return fmt.Errorf("ClearDynamicallyDefineLocalId: %w", err)
		}

		for i, v := range c.Symbols {
			if err := kwp.DynamicallyDefineLocalIdRequest(ctx, i, v); err != nil {
				return err
			}
			time.Sleep(5 * time.Millisecond)
		}

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
					//log.Println("cps:", cps)
					//cps = 0
					//c.ErrorPerSecondCounter.Set(errPerSecond)
					if errPerSecond > 5 {
						errPerSecond = 0
						return fmt.Errorf("too many errors")
					}
					errPerSecond = 0
				}
			}
		})
		var timeStamp time.Time
		errg.Go(func() error {
			for {
				select {
				case <-c.quitChan:
					c.OnMessage("Stop logging...")
					return nil
				case <-gctx.Done():
					return nil
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
					data, err := kwp.ReadDataByIdentifier(ctx, 0xF0)
					if err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(err.Error())
						continue
					}
					timeStamp = time.Now()
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
						c.dl.SetValue(va.Name, va.Float64())
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
					c.produceLogLine(file, c.Symbols, timeStamp)
					count++
					//cps++
					if count%10 == 0 {
						c.CaptureCounter.Set(count)
					}
				}
			}
		})
		c.OnMessage(fmt.Sprintf("Live logging at %d fps", c.Rate))
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

func (c *T7Client) produceLogLine(file io.Writer, vars []*symbol.Symbol, ts time.Time) {
	file.Write([]byte(ts.Format("02-01-2006 15:04:05.999") + "|"))
	c.sysvars.Lock()
	for k, v := range c.sysvars.values {
		file.Write([]byte(k + "=" + strings.Replace(v, ".", ",", 1) + "|"))
	}
	c.sysvars.Unlock()
	for _, va := range vars {
		val := va.StringValue()
		file.Write([]byte(va.Name + "=" + strings.Replace(val, ".", ",", 1) + "|"))
	}
	file.Write([]byte("IMPORTANTLINE=0|\n"))
}
