package datalogger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan"
	"github.com/roffe/t7logger/pkg/kwp2000"
	"github.com/roffe/t7logger/pkg/sink"
)

type Client struct {
	dev            gocan.Adapter
	variables      *kwp2000.VarDefinitionList
	quitChan       chan struct{}
	onMessage      func(string)
	k              *kwp2000.Client
	captureCounter binding.Int
	errorCounter   binding.Int
	freq           int
	sink           *sink.Manager
}

type Config struct {
	Dev            gocan.Adapter
	Variables      *kwp2000.VarDefinitionList
	Freq           int
	OnMessage      func(string)
	CaptureCounter binding.Int
	ErrorCounter   binding.Int
	Sink           *sink.Manager
}

func New(cfg Config) *Client {
	return &Client{
		variables:      cfg.Variables,
		dev:            cfg.Dev,
		quitChan:       make(chan struct{}, 1),
		onMessage:      cfg.OnMessage,
		captureCounter: cfg.CaptureCounter,
		errorCounter:   cfg.ErrorCounter,
		freq:           cfg.Freq,
		sink:           cfg.Sink,
	}
}

func (c *Client) Close() {
	c.quitChan <- struct{}{}
	time.Sleep(200 * time.Millisecond)
}

func (c *Client) Start() error {
	filename := fmt.Sprintf("log-%s.t7l", time.Now().Format("2006-01-02-15-04-05"))
	c.onMessage(fmt.Sprintf("Logging to %s", filename))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	ctx := context.Background()

	cl, err := gocan.New(ctx, c.dev)
	if err != nil {
		return err
	}
	defer cl.Close()

	c.k = kwp2000.New(cl)
	if err := c.k.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
		return err
	}

	if err := c.defVars(ctx); err != nil {
		return err
	}

	count := 0
	errCount := 0
	c.errorCounter.Set(errCount)

	c.onMessage(fmt.Sprintf("Starting live logging at %d fps", c.freq))
	//empty update queue
	select {
	case <-c.variables.Update():
	default:
	}

	//print(strings.Repeat("\r\n", c.variables.Len()))

	t := time.NewTicker(time.Second / time.Duration(c.freq))
	//t := time.NewTicker(82 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-c.variables.Update():
			if err := c.defVars(ctx); err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
		case <-c.quitChan:
			c.onMessage("Stop logging...")
			//if err := c.k.StopSession(ctx, kwp2000.INIT_MSG_ID); err != nil {
			//	log.Println(err)
			//}
			//time.Sleep(50 * time.Millisecond)
			return nil
		case <-t.C:
			data, err := c.k.ReadDataByLocalIdentifier(ctx, 0xF0)
			if err != nil {
				errCount++
				c.errorCounter.Set(errCount)
				c.onMessage(fmt.Sprintf("Failed to read data: %v", err))
				continue
			}
			r := bytes.NewReader(data)
			//print(strings.Repeat("\033[A", c.variables.Len()))
			for _, va := range c.variables.Get() {
				if err := va.Read(r); err != nil {
					c.onMessage(fmt.Sprintf("Failed to read %s: %v", va.Name, err))
					break
				}
			}

			if r.Len() > 0 {
				left := r.Len()
				leftovers := make([]byte, r.Len())
				n, err := r.Read(leftovers)
				if err != nil {
					c.onMessage(fmt.Sprintf("Failed to read leftovers: %v", err))
				}
				c.onMessage(fmt.Sprintf("leftovers %d: %X", left, leftovers[:n]))
			}

			c.produceLogLine(file, c.variables.Get())
			count++
			if err := c.captureCounter.Set(count); err != nil {
				c.onMessage(fmt.Sprintf("Failed to set capture counter: %v", err))
			}
		}
	}

}

func (c *Client) defVars(ctx context.Context) error {
	//c.onMessage("Defining DynamicallyDefineLocalId's...")
	for i, v := range c.variables.Get() {
		//c.onMessage(fmt.Sprintf("%d %s %s %d %X", i, v.Name, v.Method, v.Value, v.Type))
		if err := c.k.DynamicallyDefineLocalIdRequest(ctx, i, v); err != nil {
			return fmt.Errorf("DynamicallyDefineLocalIdRequest: %w", err)
		}
	}
	return nil
}

var out strings.Builder

func (c *Client) produceLogLine(file io.Writer, vars []*kwp2000.VarDefinition) {
	out.WriteString("|")

	var ms []string
	for _, va := range vars {
		out.WriteString(va.T7L() + "|")
		ms = append(ms, va.Tuple())
	}
	msg := time.Now().Format("02-01-2006 15:04:05.999") + out.String() + "IMPORTANTLINE=0|"
	fmt.Fprintln(file, msg)

	c.sink.Push(&sink.Message{
		Data: []byte(strings.Join(ms, ",")),
	})

	out.Reset()
}
