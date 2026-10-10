package wbl

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
)

// Sample readings for each wideband, as its reader receives them.
var (
	// AEM UEGO serial line, AFR x10 in ASCII: λ 1.03.
	aemLine = []byte("10.3\r\n")
	// Innovate ISP2 packet (LM-2): normal, stoich 14.7, λ 0.991.
	isp2Packet = []byte{0xB2, 0x87, 0x43, 0x13, 0x03, 0x6B, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	// PLX iMFD packet: wideband (address 0) instance 0, raw 120 = λ 1.00.
	plxPacket = []byte{0x80, 0x00, 0x00, 0x00, 0x01, 0x38, 0x40}
	// Zeitronix ZT-2 packet: AFR x10 = 147, λ 1.00.
	ztPacket = []byte{0, 1, 2, 147, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	// STAG data frame (status work, free air) with lambda raw 0x9D24.
	stagFrame = []byte{
		0x32, 0x00, 0x00, 0x1F, 0xE4, 0x00, 0x02, 0x00,
		0x00, 0x09, 0x83, 0xAE, 0x00, 0x00, 0x9D, 0x24,
		0x00, 0xC2, 0x00, 0x0A, 0x6A, 0x00, 0x41, 0x11,
		0x00, 0x04, 0x00, 0x00, 0x00, 0x78, 0x01, 0x2C,
		0x01, 0xF4, 0x58,
	}
)

// logRec records the WBL log callback and lets a test wait for a line.
type logRec struct {
	mu    sync.Mutex
	lines []string
	added chan struct{}
}

func newLogRec() *logRec { return &logRec{added: make(chan struct{}, 1)} }

func (l *logRec) add(s string) {
	l.mu.Lock()
	l.lines = append(l.lines, s)
	l.mu.Unlock()
	select {
	case l.added <- struct{}{}:
	default:
	}
}

func (l *logRec) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

func (l *logRec) has(sub string) bool {
	return slices.ContainsFunc(l.all(), func(s string) bool { return strings.Contains(s, sub) })
}

// wait blocks until a line containing sub is logged, failing after 5 seconds.
func (l *logRec) wait(t *testing.T, sub string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for !l.has(sub) {
		select {
		case <-l.added:
		case <-timeout:
			t.Fatalf("no log line containing %q in %q", sub, l.all())
		}
	}
}

// waitLambda polls until p reads want (within 1e-9), failing after 5 seconds.
func waitLambda(t *testing.T, p LambdaProvider, want float64) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for math.Abs(p.GetLambda()-want) > 1e-9 {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("lambda = %v, want %v", p.GetLambda(), want)
		}
	}
}

// fakeTxbridge is a bus adapter with the txbridge command surface: it
// records commands and hands out one test-fed subscription channel, closed
// when the subscribing context ends like the real adapter's.
type fakeTxbridge struct {
	cmdErr error
	ch     chan *serialcommand.SerialCommand

	mu   sync.Mutex
	cmds []serialcommand.SerialCommand
	subs [][]byte
}

func newFakeTxbridge() *fakeTxbridge {
	return &fakeTxbridge{ch: make(chan *serialcommand.SerialCommand, 16)}
}

func (f *fakeTxbridge) Open(context.Context, *gocan.Bus) error  { return nil }
func (f *fakeTxbridge) Send(context.Context, gocan.Frame) error { return nil }
func (f *fakeTxbridge) Close() error                            { return nil }

func (f *fakeTxbridge) Command(cmd byte, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, serialcommand.SerialCommand{Command: cmd, Data: slices.Clone(data)})
	return f.cmdErr
}

func (f *fakeTxbridge) Subscribe(ctx context.Context, cmds ...byte) <-chan *serialcommand.SerialCommand {
	f.mu.Lock()
	f.subs = append(f.subs, slices.Clone(cmds))
	f.mu.Unlock()
	context.AfterFunc(ctx, func() { close(f.ch) })
	return f.ch
}

// openBus opens a bus on adapter, closed when the test ends.
func openBus(t *testing.T, ctx context.Context, adapter gocan.Adapter) *gocan.Bus {
	t.Helper()
	cl, err := gocan.OpenAdapter(ctx, adapter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

// "ECU" and "None" mean no external wideband: New returns no provider and
// no error.
func TestNewNoWideband(t *testing.T) {
	for _, typ := range []string{"ECU", "None"} {
		p, err := New(context.Background(), nil, &WBLConfig{WBLType: typ, Log: func(string) {}})
		if p != nil || err != nil {
			t.Errorf("New(%q) = %v, %v; want nil, nil", typ, p, err)
		}
	}
}

// An unknown wideband type is an error.
func TestNewUnknownType(t *testing.T) {
	p, err := New(context.Background(), nil, &WBLConfig{WBLType: "Bosch LA4", Log: func(string) {}})
	if p != nil || err == nil || err.Error() != "unknown WBL type: Bosch LA4" {
		t.Errorf("New = %v, %v; want unknown WBL type error", p, err)
	}
}

// A wideband on the txbridge port without the txbridge adapter is skipped
// with a message telling the user to fix the configuration.
func TestNewTxbridgePortWithoutTxbridge(t *testing.T) {
	logs := newLogRec()
	p, err := New(context.Background(), nil, &WBLConfig{WBLType: zeitronix.ProductString, Port: "txbridge", Log: logs.add})
	if p != nil || err != nil {
		t.Fatalf("New = %v, %v; want nil, nil", p, err)
	}
	want := []string{"please correct your WBL configuration. txbridge port is selected but not using txbridge adapter"}
	if got := logs.all(); !slices.Equal(got, want) {
		t.Errorf("logs = %q, want %q", got, want)
	}
}

// Without a Log callback New installs one that writes to the standard
// logger.
func TestNewDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})
	cfg := &WBLConfig{WBLType: aem.ProductString, Port: "txbridge"}
	if p, err := New(context.Background(), nil, cfg); p != nil || err != nil {
		t.Fatalf("New = %v, %v; want nil, nil", p, err)
	}
	if cfg.Log == nil {
		t.Fatal("cfg.Log left nil")
	}
	if got := buf.String(); !strings.Contains(got, "txbridge port is selected but not using txbridge adapter") {
		t.Errorf("standard log = %q, want the configuration message", got)
	}
}

// Through a txbridge, New arms the dongle's reader for the wideband with
// command 'w' <type>, subscribes to its 'w' readings and feeds them to the
// driver; a reading the driver rejects is logged and the next one is used.
func TestNewTxbridgeReadings(t *testing.T) {
	for _, c := range []struct {
		typ     string
		code    byte
		msgs    [][]byte
		want    float64
		badLog  string // logged for the first, rejected message
		doneLog string // logged when the subscription closes
	}{
		{aem.ProductString, 'a', [][]byte{[]byte("1x"), []byte("10.3")}, 1.03, "could not decode WBL value", "wbl reading channel closed"},
		{innovate.ProductString, 'i', [][]byte{isp2Packet}, 0.991, "", "wbl channel closed"},
		{plx.ProductString, 'p', [][]byte{{0x80, 0x00, 0x00, 0x40}, plxPacket}, 1.0, "invalid data length: 2", "wbl channel closed"},
		{zeitronix.ProductString, 'z', [][]byte{ztPacket[:3], ztPacket}, 1.0, "invalid data length", "wbl channel closed"},
	} {
		t.Run(c.typ, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tb := newFakeTxbridge()
			cl := openBus(t, context.Background(), tb)
			logs := newLogRec()
			p, err := New(ctx, cl, &WBLConfig{WBLType: c.typ, Port: "txbridge", Txbridge: true, Log: logs.add})
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range c.msgs {
				tb.ch <- &serialcommand.SerialCommand{Command: 'w', Data: m}
			}
			// The reader drains what was sent before the subscription closes.
			cancel()
			logs.wait(t, c.doneLog)
			if got := p.GetLambda(); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("lambda = %v, want %v", got, c.want)
			}
			if c.badLog != "" && !logs.has(c.badLog) {
				t.Errorf("logs = %q, want %q for the rejected reading", logs.all(), c.badLog)
			}
			tb.mu.Lock()
			defer tb.mu.Unlock()
			wantCmds := []serialcommand.SerialCommand{{Command: 'w', Data: []byte{c.code}}}
			if !slices.EqualFunc(tb.cmds, wantCmds, func(a, b serialcommand.SerialCommand) bool {
				return a.Command == b.Command && bytes.Equal(a.Data, b.Data)
			}) {
				t.Errorf("commands = %v, want %v", tb.cmds, wantCmds)
			}
			if !slices.EqualFunc(tb.subs, [][]byte{{'w'}}, bytes.Equal) {
				t.Errorf("subscriptions = %q, want [w]", tb.subs)
			}
		})
	}
}

// Txbridge wideband readers fail when the bus adapter is not a txbridge, or
// when arming the dongle's reader fails.
func TestNewTxbridgeErrors(t *testing.T) {
	armErr := errors.New("txbridge port not open")
	for _, typ := range []string{aem.ProductString, innovate.ProductString, plx.ProductString, zeitronix.ProductString} {
		t.Run(typ, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := func() *WBLConfig {
				return &WBLConfig{WBLType: typ, Port: "txbridge", Txbridge: true, Log: func(string) {}}
			}

			p, err := New(ctx, openBus(t, ctx, &gocan.Loopback{}), cfg())
			if p != nil || err == nil || err.Error() != "txbridge WBL needs the txbridge adapter" {
				t.Errorf("on a loopback adapter: New = %v, %v; want the txbridge adapter error", p, err)
			}

			tb := newFakeTxbridge()
			tb.cmdErr = armErr
			p, err = New(ctx, openBus(t, ctx, tb), cfg())
			if p != nil || !errors.Is(err, armErr) {
				t.Errorf("arm failure: New = %v, %v; want %v", p, err, armErr)
			}
		})
	}
}

// An AEM on CAN reads the extended 0x180 frame off the bus; a standard
// frame with the same id is not the AEM and is ignored.
func TestNewAEMOnCAN(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, context.Background(), &gocan.Loopback{})
	logs := newLogRec()
	p, err := New(ctx, cl, &WBLConfig{WBLType: aem.ProductString, Port: "CAN", Log: logs.add})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []gocan.Frame{
		gocan.NewExtendedFrame(0x180, []byte{0x26, 0x8E, 0x04, 0xD2, 0x8A, 0x00, 0x80, 0x00}), // λ 0.987
		gocan.NewFrame(0x180, []byte{0x3A, 0x98, 0x00, 0x00, 0x8A, 0x00, 0x80, 0x00}),         // λ 1.5, standard id
	} {
		if err := cl.Send(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	logs.wait(t, "wbl channel closed")
	if got := p.GetLambda(); math.Abs(got-0.987) > 1e-9 {
		t.Errorf("lambda = %v, want 0.987", got)
	}
	if !logs.has("Starting AEM CAN client") {
		t.Errorf("logs = %q, want the CAN client start message", logs.all())
	}
}

// A LAMBDA to CAN module is read off the bus, from 0x664/0x665.
func TestNewECUMaster(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := openBus(t, context.Background(), &gocan.Loopback{})
	p, err := New(ctx, cl, &WBLConfig{WBLType: ecumaster.ProductString, Log: func(string) {}})
	if err != nil {
		t.Fatal(err)
	}
	if err := cl.Send(ctx, gocan.NewFrame(0x664, []byte{0x05, 0x00, 0x80, 0xC8, 0x03, 0xDB, 0x28, 0x00})); err != nil {
		t.Fatal(err)
	}
	waitLambda(t, p, 0.987)
}

// Serial widebands report a port that cannot be opened.
func TestNewSerialPortError(t *testing.T) {
	port := filepath.Join(t.TempDir(), "missing")
	for _, typ := range []string{aem.ProductString, plx.ProductString, zeitronix.ProductString, stag.ProductString} {
		p, err := New(context.Background(), nil, &WBLConfig{WBLType: typ, Port: port, Log: func(string) {}})
		if p != nil || err == nil {
			t.Errorf("New(%q) on a missing port = %v, %v; want an error", typ, p, err)
		}
	}
}

// The Innovate reader reports a port that cannot be opened like the other
// serial widebands.
func TestNewInnovateSerialPortError(t *testing.T) {
	port := filepath.Join(t.TempDir(), "missing")
	p, err := New(context.Background(), nil, &WBLConfig{WBLType: innovate.ProductString, Port: port, Log: func(string) {}})
	if p != nil || err == nil {
		t.Errorf("New on a missing port = %v, %v; want an error", p, err)
	}
}
