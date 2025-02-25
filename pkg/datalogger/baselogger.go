package datalogger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
)

type BaseLogger struct {
	lamb LambdaProvider
	lw   LogWriter

	sysvars *ThreadSafeMap

	symbolChan chan []*symbol.Symbol
	readChan   chan *ReadRequest
	writeChan  chan *WriteRequest
	quitChan   chan struct{}

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
		symbolChan:   make(chan []*symbol.Symbol, 1),
		writeChan:    make(chan *WriteRequest, 1),
		readChan:     make(chan *ReadRequest, 1),
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
	upd := NewRamUpdate(address, data)
	bl.writeChan <- upd
	return upd.Wait()
}

func (bl *BaseLogger) GetRAM(address uint32, length uint32) ([]byte, error) {
	req := NewReadRequest(address, length)
	bl.readChan <- req
	return req.Data, req.Wait()
}

func (bl *BaseLogger) SetSymbols(symbols []*symbol.Symbol) error {
	select {
	case bl.symbolChan <- symbols:
	default:
		return fmt.Errorf("pending")
	}
	return nil
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
	// Wideband lambda
	cfg := &WBLConfig{
		WBLType:  bl.Config.WidebandConfig.Type,
		Port:     bl.Config.WidebandConfig.Port,
		Log:      bl.OnMessage,
		Txbridge: bl.txbridge,
	}
	var err error
	bl.lamb, err = NewWBL(ctx, cl, cfg)
	if err != nil {
		return fmt.Errorf("failed to create wideband lambda: %w", err)
	}
	return nil
}
