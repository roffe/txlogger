package datalogger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/wbl"
)

type BaseLogger struct {
	lamb wbl.LambdaProvider
	lw   LogWriter

	sysvars *ThreadSafeMap

	//symbolChan chan []*symbol.Symbol
	readChan  chan *DataRequest
	writeChan chan *DataRequest
	quitChan  chan struct{}

	cps          int
	captureCount int
	errCount     int
	errPerSecond int

	closeOnce sync.Once

	txbridge bool

	secondTicker *time.Ticker

	firstTime      time.Time
	firstTimestamp uint32
	currtimestamp  uint32

	Config
}

func NewBaseLogger(cfg Config, lw LogWriter) BaseLogger {
	return BaseLogger{
		lw:           lw,
		Config:       cfg,
		sysvars:      NewThreadSafeMap(),
		writeChan:    make(chan *DataRequest, 1),
		readChan:     make(chan *DataRequest, 1),
		quitChan:     make(chan struct{}),
		txbridge:     strings.HasPrefix(cfg.Device.Name(), "txbridge"),
		secondTicker: time.NewTicker(time.Second),
	}
}

func (bl *BaseLogger) Close() {
	bl.closeOnce.Do(func() {
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

func (bl *BaseLogger) onError() {
	bl.errCount++
	bl.errPerSecond++
	bl.ErrorCounter(bl.errCount)
}

func (bl *BaseLogger) calculateCompensatedTimestamp() time.Time {
	return bl.firstTime.Add(time.Duration(bl.currtimestamp-bl.firstTimestamp) * time.Millisecond)
}

func (bl *BaseLogger) setupWBL(ctx context.Context, cl *gocan.Client) error {
	cfg := &wbl.WBLConfig{
		WBLType:  bl.Config.WidebandConfig.Type,
		Port:     bl.Config.WidebandConfig.Port,
		Log:      bl.OnMessage,
		Txbridge: bl.txbridge,
	}
	var err error
	lamb, err := wbl.New(ctx, cl, cfg)
	if err != nil {
		return fmt.Errorf("failed to create wideband lambda: %w", err)
	}
	bl.lamb = lamb
	return nil
}
