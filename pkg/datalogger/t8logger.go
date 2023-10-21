package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/widgets"
	"golang.org/x/sync/errgroup"
)

func NewT8(cfg Config) (*T8Client, error) {
	return &T8Client{
		quitChan: make(chan struct{}, 2),
		Config:   cfg,
		sysvars: &ThreadSafeMap{
			values: make(map[string]string),
		},
		subs: make(map[string][]*func(float64)),
	}, nil
}

type T8Client struct {
	quitChan chan struct{}
	Config
	sysvars *ThreadSafeMap
	db      Dashboard

	subs map[string][]*func(float64)
}

func (c *T8Client) Close() {
	close(c.quitChan)
	time.Sleep(200 * time.Millisecond)
}

func (c *T8Client) Subscribe(name string, cb *func(float64)) {
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

func (c *T8Client) Unsubscribe(name string, cb *func(float64)) {
	for i, f := range c.subs[name] {
		if f == cb {
			c.subs[name] = append(c.subs[name][:i], c.subs[name][i+1:]...)
			return
		}
	}
}

func (c *T8Client) AttachDashboard(db Dashboard) {
	c.db = db
}

func (c *T8Client) DetachDashboard(db Dashboard) {
	c.db = nil
}

func (c *T8Client) Start() error {
	file, filename, err := createLog("t8l")
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
		c.Dev,
	)
	if err != nil {
		return err
	}
	defer cl.Close()

	count := 0
	errCount := 0
	c.ErrorCounter.Set(errCount)

	errPerSecond := 0
	c.ErrorPerSecondCounter.Set(errPerSecond)

	// cps := 0
	retries := 0

	err = retry.Do(func() error {
		gm := gmlan.New(cl, 0x7e0, 0x7e8)

		if err := ClearDynamicallyDefinedRegister(ctx, gm); err != nil {
			return err
		}
		c.OnMessage("Cleared dynamic register")

		for _, sym := range c.Config.Variables {
			if err := SetUpDynamicallyDefinedRegisterBySymbol(ctx, gm, uint16(sym.Value)); err != nil {
				return err
			}
			//c.OnMessage(fmt.Sprintf("Configured dynamic register %d: %s %d", i, sym.Name, sym.Value))
		}
		c.OnMessage("Configured dynamic register")

		secondTicker := time.NewTicker(time.Second)
		defer secondTicker.Stop()

		t := time.NewTicker(time.Second / time.Duration(c.Freq))
		defer t.Stop()

		//first := true

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
					ts := time.Now()
					data, err := gm.ReadDataByIdentifier(ctx, 0x18)
					if err != nil {
						errCount++
						errPerSecond++
						c.ErrorCounter.Set(errCount)
						c.OnMessage(fmt.Sprintf("Failed to read data: %v", err))
						continue
					}
					r := bytes.NewReader(data)
					if err != nil {
						return err
					}
					for _, va := range c.Variables {
						if err := va.Read(r); err != nil {
							c.OnMessage(fmt.Sprintf("Failed to read %s: %v", va.Name, err))
							break
						}
						if c.db != nil {
							c.db.SetValue(va.Name, va.GetFloat64())
						}
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
					c.produceLogLine(file, c.Variables, ts)
					//cps++
					count++
					if count%10 == 0 {
						c.CaptureCounter.Set(count)
					}
					//if count%30 == 0 {
					//	gm.TesterPresentNoResponseAllowed()
					//}
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

func (c *T8Client) produceLogLine(file io.Writer, vars []*kwp2000.VarDefinition, ts time.Time) {
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

func ClearDynamicallyDefinedRegister(ctx context.Context, gm *gmlan.Client) error {
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x04}); err != nil {
		return fmt.Errorf("ClearDynamicallyDefinedRegister: %w", err)
	}
	return nil
}

func SetUpDynamicallyDefinedRegisterBySymbol(ctx context.Context, gm *gmlan.Client, symbol uint16) error {
	/* payload
	byte[0] = register id
	byte[1] type
		0x03 = Define by memory address
		0x04 = Clear dynamic defined register
		0x80 = Define by symbol position
	byte[5] symbol id high byte
	byte[6]	symbol id low byte
	*/
	payload := []byte{0xF0, 0x80, 0x00, 0x00, 0x00, byte(symbol >> 8), byte(symbol)}
	if err := gm.WriteDataByIdentifier(ctx, 0x17, payload); err != nil {
		return fmt.Errorf("SetUpDynamicallyDefinedRegisterBySymbol: %w", err)
	}
	return nil
}
