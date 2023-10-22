package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/widgets"
	"golang.org/x/sync/errgroup"
)

type T7Client struct {
	quitChan chan struct{}
	Config
	sysvars *ThreadSafeMap
	dbs     []Dashboard

	subs map[string][]*func(float64)
	mu   sync.Mutex
}

func NewT7(cfg Config) (*T7Client, error) {
	return &T7Client{
		quitChan: make(chan struct{}, 2),
		Config:   cfg,
		sysvars: &ThreadSafeMap{
			values: map[string]string{
				"ActualIn.n_Engine": "0",   // comes from 0x1A0
				"Out.X_AccPedal":    "0.0", // comes from 0x1A0
				"In.v_Vehicle":      "0.0", // comes from 0x3A0
				"Out.ST_LimpHome":   "0",   // comes from 0x280
			},
		},
		subs: make(map[string][]*func(float64)),
	}, nil
}

func (c *T7Client) Subscribe(name string, cb *func(float64)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	subs, found := c.subs[name]
	if !found {
		c.subs[name] = []*func(float64){cb}
		return
	}
	for _, f := range subs {
		if f == cb {
			return
		}
	}
	subs = append(subs, cb)
	c.subs[name] = subs
}

func (c *T7Client) Unsubscribe(name string, cb *func(float64)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, f := range c.subs[name] {
		if f == cb {
			c.subs[name] = append(c.subs[name][:i], c.subs[name][i+1:]...)
			return
		}
	}
}

func (c *T7Client) Close() {
	close(c.quitChan)
	time.Sleep(200 * time.Millisecond)
}

func (c *T7Client) AttachDashboard(db Dashboard) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.dbs {
		if d == db {
			log.Println("Dropping")
			return
		}
	}
	c.dbs = append(c.dbs, db)
}

func (c *T7Client) DetachDashboard(db Dashboard) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, d := range c.dbs {
		if d == db {
			c.dbs = append(c.dbs[:i], c.dbs[i+1:]...)
			return
		}
	}
}

func (c *T7Client) setDbValue(name string, value float64) {
	for _, db := range c.dbs {
		db.SetValue(name, value)
	}
}

func (c *T7Client) Start() error {
	file, filename, err := createLog("t7l")
	if err != nil {
		return err
	}
	defer file.Close()
	defer file.Sync()
	c.OnMessage(fmt.Sprintf("Logging to %s", filename))

	/*
		csvFilename := fmt.Sprintf("logs/log-%s.csv", time.Now().Format("2006-01-02-15-04-05"))
		csv, err := os.OpenFile(csvFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer csv.Close()

		csvHeader := []string{"Date"}
		for _, va := range c.Variables {
			csvHeader = append(csvHeader, va.Name)
		}
		fmt.Fprintln(csv, strings.Join(csvHeader, ","))
	*/
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := gocan.NewWithOpts(
		ctx,
		c.Dev,
	)
	if err != nil {
		return err
	}
	defer cl.Close()

	go func() {
		sub := cl.Subscribe(ctx, 0x1A0, 0x280, 0x3A0)
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-sub:
				switch msg.Identifier() {
				case 0x1A0:
					rpm := binary.BigEndian.Uint16(msg.Data()[1:3])
					throttle := int(msg.Data()[5])
					c.sysvars.Set("ActualIn.n_Engine", strconv.Itoa(int(rpm)))
					c.sysvars.Set("Out.X_AccPedal", strconv.Itoa(throttle)+",0")

					c.setDbValue("ActualIn.n_Engine", float64(rpm))
					c.setDbValue("Out.X_AccPedal", float64(throttle))

					if subs, found := c.subs["ActualIn.n_Engine"]; found {
						for _, sub := range subs {
							(*sub)(float64(rpm))
						}
					}
					if subs, found := c.subs["Out.X_AccPedal"]; found {
						for _, sub := range subs {
							(*sub)(float64(throttle))
						}
					}
				case 0x280:
					data := msg.Data()[4]
					if data&0x20 == 0x20 {
						c.setDbValue("CRUISE", 1)
					} else {
						c.setDbValue("CRUISE", 0)
					}
					if data&0x80 == 0x80 {
						c.setDbValue("CEL", 1)
					} else {
						c.setDbValue("CEL", 0)
					}
					data2 := msg.Data()[3]
					if data2&0x01 == 0x01 {
						c.setDbValue("LIMP", 1)
					} else {
						c.setDbValue("LIMP", 0)
					}

				case 0x3A0:
					speed := uint16(msg.Data()[4]) | uint16(msg.Data()[3])<<8
					realSpeed := float64(speed) / 10
					c.sysvars.Set("In.v_Vehicle", strconv.FormatFloat(realSpeed, 'f', 1, 64))
					c.setDbValue("In.v_Vehicle", realSpeed)
				}
			}
		}
	}()

	kwp := kwp2000.New(cl)

	count := 0
	errCount := 0
	c.ErrorCounter.Set(errCount)

	errPerSecond := 0
	c.ErrorPerSecondCounter.Set(errPerSecond)

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

		for i, v := range c.Variables {
			//c.onMessage(fmt.Sprintf("%d %s %s %d %X", i, v.Name, v.Method, v.Value, v.Type))
			if err := kwp.DynamicallyDefineLocalIdRequest(ctx, i, v); err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest: %w", err)
			}
			time.Sleep(5 * time.Millisecond)
		}

		secondTicker := time.NewTicker(time.Second)
		defer secondTicker.Stop()

		t := time.NewTicker(time.Second / time.Duration(c.Freq))
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
					c.ErrorPerSecondCounter.Set(errPerSecond)
					if errPerSecond > 10 {
						errPerSecond = 0
						return fmt.Errorf("too many errors")
					}
					errPerSecond = 0
				}
			}
		})

		errg.Go(func() error {
			for {
				select {
				case <-c.quitChan:
					c.OnMessage("Stop logging...")
					return nil
				case <-gctx.Done():
					return nil
				case <-t.C:
					//start := time.Now()
					ts := time.Now()
					data, err := kwp.ReadDataByLocalIdentifier(ctx, 0xF0)
					if err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(fmt.Sprintf("Failed to read data: %v", err))
						continue
					}

					r := bytes.NewReader(data)
					for _, va := range c.Variables {
						if err := va.Read(r); err != nil {
							c.OnMessage(fmt.Sprintf("Failed to read %s: %v", va.Name, err))
							break
						}

						// Set value on dashboards
						c.setDbValue(va.Name, va.GetFloat64())

						if subs, found := c.subs[va.Name]; found {
							for _, sub := range subs {
								(*sub)(va.GetFloat64())
							}
						}

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
					//c.produceCSVLine(csv, c.Variables)
					c.produceLogLine(file, c.Variables, ts)
					count++
					//cps++
					if count%10 == 0 {
						c.CaptureCounter.Set(count)
					}
				}
			}
		})

		c.OnMessage(fmt.Sprintf("Live logging at %d fps", c.Freq))

		return errg.Wait()
	},
		retry.DelayType(retry.FixedDelay),
		retry.Delay(1500*time.Millisecond),
		retry.Attempts(4),
		retry.OnRetry(func(n uint, err error) {
			retries++
			c.OnMessage(fmt.Sprintf("Retry %d: %v", n, err))
		}),
	)
	return err
}

/*
func (c *T7Client) produceCSVLine(file io.Writer, vars []*kwp2000.VarDefinition) {
	var values []string
	for _, va := range vars {
		values = append(values, va.StringValue())
	}
	fmt.Fprintln(file, time.Now().Format("2006-01-02 15:04:05")+","+strings.Join(values, ","))

}
*/

func (c *T7Client) produceLogLine(file io.Writer, vars []*kwp2000.VarDefinition, ts time.Time) {
	file.Write([]byte(ts.Format("02-01-2006 15:04:05.999") + "|"))
	c.sysvars.Lock()
	for k, v := range c.sysvars.values {
		file.Write([]byte(k + "=" + strings.Replace(v, ".", ",", 1) + "|"))
	}
	c.sysvars.Unlock()
	for _, va := range vars {
		val := va.StringValue()
		file.Write([]byte(va.Name + "=" + strings.Replace(val, ".", ",", 1) + "|"))
		if va.Widget != nil {
			va.Widget.(*widgets.VarDefinitionWidgetEntry).SetValue(val)
		}
	}
	file.Write([]byte("IMPORTANTLINE=0|\n"))
	//c.Sink.Push(&sink.Message{
	//	Data: []byte(time.Now().Format(ISO8601) + "|" + strings.Join(ms, ",")),
	//})
}
