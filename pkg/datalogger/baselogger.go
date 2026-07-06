package datalogger

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ebus"
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

	r *relayserver.Client

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

	if cfg.RemoteMode == 1 {
		if err := bl.runRelay(); err != nil {
			log.Println(err.Error())
		}
	}
	return bl
}

func (bl *BaseLogger) Close() {
	bl.closeOnce.Do(func() {
		if bl.r != nil {
			bl.r.Close()
		}
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

// update capture counters and emit the per-frame tick so live consumers can
// sample every symbol with this frame's real timestamp.
func (bl *BaseLogger) onCapture(t time.Time) {
	bl.captureCount++
	bl.capturePerSecond++
	if bl.captureCount%15 == 0 {
		bl.CaptureCounter(bl.captureCount)
	}
	ebus.PublishFrame(t)
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

// appendExtraSysvars appends the wideband and AD scanner pseudo-symbol names to
// a sysvar order slice when they are active, in the same order they are
// written to the log.
func (bl *BaseLogger) appendExtraSysvars(order []string) []string {
	if bl.lamb != nil {
		order = append(order, EXTERNALWBLSYM)
	}
	if bl.WidebandConfig.ADScanner && bl.WidebandConfig.Name == "ECU" {
		order = append(order, LAMBDAADSCANNER)
	}
	return order
}

// buildChannels assembles the standard log layout: every current sysvar
// (broadcast/derived values plus the active wideband / AD scanner pseudo-symbols)
// becomes an asynchronous sysvar channel, followed by every polled symbol
// (Number >= 0) as a symbol channel. Symbols with a negative number are either
// replaced by a broadcast/derived sysvar or sourced elsewhere and are not log
// columns. Column order is whatever the channel slice yields; the writer is
// consistent across the header and every row because it iterates this slice.
func (bl *BaseLogger) buildChannels() []Channel {
	order := bl.appendExtraSysvars(bl.sysvars.Keys())
	channels := make([]Channel, 0, len(order)+len(bl.Symbols))
	for _, name := range order {
		channels = append(channels, newSysvarChannel(bl.sysvars, name))
	}
	for _, sym := range bl.Symbols {
		if sym.Number < 0 {
			continue
		}
		channels = append(channels, newSymbolChannel(sym))
	}
	return channels
}

func (bl *BaseLogger) setupWBL(ctx context.Context, cl *gocan.Bus) error {
	cfg := &wbl.WBLConfig{
		WBLType:  bl.Config.WidebandConfig.Name,
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
