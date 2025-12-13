package datalogger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/wbl"
	"github.com/roffe/txlogger/relayserver"
)

type BaseLogger struct {
	lamb wbl.LambdaProvider
	lw   LogWriter

	sysvars *ThreadSafeMap

	readChan  chan *DataRequest
	writeChan chan *DataRequest
	quitChan  chan struct{}

	capturePerSecond int
	captureCount     int
	errPerSecond     int
	errCount         int

	closeOnce sync.Once

	secondTicker *time.Ticker

	firstTime      time.Time
	firstTimestamp uint32
	currtimestamp  uint32

	//r *relayserver.Client

	Config
}

func NewBaseLogger(cfg Config, lw LogWriter) *BaseLogger {
	bl := &BaseLogger{
		lw:           lw,
		Config:       cfg,
		sysvars:      NewThreadSafeMap(),
		writeChan:    make(chan *DataRequest, 1),
		readChan:     make(chan *DataRequest, 1),
		quitChan:     make(chan struct{}),
		secondTicker: time.NewTicker(time.Second),
	}
	//if err := bl.connectRelay(); err != nil {
	//	log.Println(err.Error())
	//}
	return bl
}

func (bl *BaseLogger) connectRelay() error {
	c, err := relayserver.NewClient("localhost:9000")
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}
	bl.OnMessage("Connected to relay server")

	if err := c.JoinSession("1337"); err != nil {
		return fmt.Errorf("join session error: %w", err)
	}

	//bl.r = c

	return nil
}

func (bl *BaseLogger) Close() {
	bl.closeOnce.Do(func() {
		//if bl.r != nil {
		//	bl.r.Close()
		//}
		close(bl.quitChan)
		time.Sleep(150 * time.Millisecond)
	})
}

func (bl *BaseLogger) SetRAM(address uint32, data []byte) error {
	req := NewWriteDataRequest(address, data)
	select {
	case bl.writeChan <- req:
	default:
		return fmt.Errorf("busy")
	}
	return req.Wait()
}

func (bl *BaseLogger) GetRAM(address uint32, length uint32) ([]byte, error) {
	req := NewReadDataRequest(address, length)
	select {
	case bl.readChan <- req:
	default:
		return nil, fmt.Errorf("busy")
	}
	return req.Data, req.Wait()
}

// update capture counters
func (bl *BaseLogger) onCapture() {
	bl.captureCount++
	bl.capturePerSecond++
	if bl.captureCount%15 == 0 {
		bl.CaptureCounter(bl.captureCount)
	}
}

func (bl *BaseLogger) onError() {
	bl.errCount++
	bl.errPerSecond++
	bl.ErrorCounter(bl.errCount)
}

func (bl *BaseLogger) resetPerSecond() {
	bl.capturePerSecond = 0
	bl.errPerSecond = 0
}

func (bl *BaseLogger) calculateCompensatedTimestamp() time.Time {
	return bl.firstTime.Add(time.Duration(bl.currtimestamp-bl.firstTimestamp) * time.Millisecond)
}

func (bl *BaseLogger) setupWBL(ctx context.Context, cl *gocan.Client) error {
	cfg := &wbl.WBLConfig{
		WBLType:  bl.Config.WidebandConfig.Type,
		Port:     bl.Config.WidebandConfig.Port,
		Log:      bl.OnMessage,
		Txbridge: strings.HasPrefix(cl.AdapterName(), "txbridge"),
	}
	var err error
	lamb, err := wbl.New(ctx, cl, cfg)
	if err != nil {
		return fmt.Errorf("failed to create wideband lambda: %w", err)
	}
	bl.lamb = lamb
	return nil
}
