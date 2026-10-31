package datalogger

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/ecusim"
	"github.com/roffe/txlogger/pkg/wbl/aem"
)

// The ECU reads the address from dataBuf[4:7] and the length from dataBuf[3],
// see case APPT_DBMA in SetUpDynamicallyDefinedRegister (APP_Apptool.c).
func TestDBMAPayload(t *testing.T) {
	got := dbmaPayload(0x123456, 4)
	want := []byte{0xF0, APPT_DBMA, 0x00, 0x04, 0x12, 0x34, 0x56}
	if !bytes.Equal(got, want) {
		t.Errorf("got % X, want % X", got, want)
	}
}

func t8Gmlan(t *testing.T, sim gocan.Adapter) (*gmlan.Client, context.Context) {
	t.Helper()
	cl := openBus(t, sim)
	return gmlan.NewWithOpts(cl, gmlan.WithCanID(0x7E0), gmlan.WithRecvID(0x7E8), gmlan.WithDefaultTimeout(200*time.Millisecond)), context.Background()
}

// After the logging init (security access + register clear), a register
// entry defined by memory address makes the 0x18 read return those bytes.
func TestT8DefineRegisterByAddress(t *testing.T) {
	sim := &ecusim.T8{}
	gm, ctx := t8Gmlan(t, sim)
	var msgs []string
	if err := initT8Logging(ctx, gm, nil, func(s string) { msgs = append(msgs, s) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(msgs, "|") != "Cleared dynamic register|Configured dynamic register" {
		t.Fatalf("messages %q", msgs)
	}
	if err := gm.WriteDataByAddress(ctx, 0x100400, []byte{0xAA, 0xBB, 0xCC}); err != nil {
		t.Fatal(err)
	}
	if err := setupDynamicallyDefinedRegisterByAddress(ctx, gm, 0x100400, 3); err != nil {
		t.Fatal(err)
	}
	got, err := gm.ReadDataByIdentifier(ctx, 0x18)
	if err != nil || !bytes.Equal(got, []byte{0xAA, 0xBB, 0xCC}) {
		t.Fatalf("register read % X %v", got, err)
	}
}

// Define-by-address refuses addresses beyond 24 bits and zero lengths
// without talking to the ECU.
func TestT8DefineRegisterByAddressValidation(t *testing.T) {
	gm, ctx := t8Gmlan(t, newHook(&ecusim.T8{}, "", func(*gocan.Bus, gocan.Frame) bool {
		t.Error("frame sent for an invalid definition")
		return true
	}))
	if err := setupDynamicallyDefinedRegisterByAddress(ctx, gm, 0x1000000, 1); err == nil || !strings.Contains(err.Error(), "24 bit") {
		t.Fatalf("wide address: %v", err)
	}
	if err := setupDynamicallyDefinedRegisterByAddress(ctx, gm, 0x100000, 0); err == nil || !strings.Contains(err.Error(), "length") {
		t.Fatalf("zero length: %v", err)
	}
}

// Register commands the ECU refuses (no security access yet) come back as
// errors naming the step that failed.
func TestT8RegisterCommandsReportECURefusal(t *testing.T) {
	gm, ctx := t8Gmlan(t, &ecusim.T8{})
	if err := clearDynamicallyDefinedRegister(ctx, gm); err == nil || !strings.HasPrefix(err.Error(), "ClearDynamicallyDefinedRegister:") {
		t.Fatalf("clear: %v", err)
	}
	if err := setUpDynamicallyDefinedRegisterBySymbol(ctx, gm, 42); err == nil || !strings.HasPrefix(err.Error(), "SetUpDynamicallyDefinedRegisterBySymbol:") {
		t.Fatalf("define by symbol: %v", err)
	}
	if err := setupDynamicallyDefinedRegisterByAddress(ctx, gm, 0x100000, 2); err == nil || !strings.HasPrefix(err.Error(), "SetUpDynamicallyDefinedRegisterByAddress:") {
		t.Fatalf("define by address: %v", err)
	}
}

// T8 GetRAM only serves SRAM (above 0x100000) and refuses anything that ends
// at or below it without queueing a request.
func TestT8GetRAMRejectsNonSRAM(t *testing.T) {
	cl, _ := NewT8(testCfg(t, "T8", &ecusim.T8{}, nil, newMsgLog(t)), nopWriter{})
	t8 := cl.(*T8Client)
	defer t8.secondTicker.Stop()
	for _, r := range [][2]uint32{{0, 4}, {0x0FFF00, 0x100}, {0x00FFFF, 1}} {
		if _, err := t8.GetRAM(r[0], r[1]); err == nil || !strings.Contains(err.Error(), "not in SRAM") {
			t.Fatalf("GetRAM(%X, %d) = %v", r[0], r[1], err)
		}
	}
	if len(t8.readChan) != 0 {
		t.Fatal("request queued")
	}
}

// Like the base logger, T8 GetRAM answers "busy" when a request is already
// queued rather than blocking the caller.
func TestT8GetRAMBusy(t *testing.T) {
	cl, _ := NewT8(testCfg(t, "T8", &ecusim.T8{}, nil, newMsgLog(t)), nopWriter{})
	t8 := cl.(*T8Client)
	defer t8.secondTicker.Stop()
	t8.readChan <- NewReadDataRequest(0x100000, 1)
	done := make(chan error, 1)
	go func() {
		_, err := t8.GetRAM(0x100000, 4)
		done <- err
	}()
	t.Cleanup(func() { // unblock a stuck GetRAM so it does not outlive the test
		for range 2 {
			select {
			case r := <-t8.readChan:
				r.Complete(errors.New("test over"))
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
	select {
	case err := <-done:
		if err == nil || err.Error() != "busy" {
			t.Fatalf("GetRAM = %v, want busy", err)
		}
	case <-time.After(time.Second):
		t.Fatal("GetRAM blocked")
	}
}

// RAM transfers larger than one 245-byte GMLAN request are split into chunks
// and reassembled.
func TestT8LargeRAMTransfer(t *testing.T) {
	sim := &ecusim.T8{}
	sim.SetSymbol(100, []byte{0, 1})
	rec := newRecWriter()
	cl, _ := NewT8(testCfg(t, "T8", sim, []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 100, Length: 2}}, newMsgLog(t)), rec)
	runLogger(t, cl)
	rec.waitRows(t, 1, 10*time.Second)

	data := make([]byte, 600)
	for i := range data {
		data[i] = byte(i * 7)
	}
	if err := cl.SetRAM(0x100200, data); err != nil {
		t.Fatal(err)
	}
	if got := sim.Peek(0x100200, len(data)); !bytes.Equal(got, data) {
		t.Fatal("ECU RAM does not hold the written data")
	}
	got, err := cl.GetRAM(0x100200, uint32(len(data)))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("GetRAM returned %d bytes, %v", len(got), err)
	}
	n := len(rec.snapshot())
	rec.waitRows(t, n+2, 10*time.Second) // logging resumes
}

// traceLog collects a sim's wire trace.
type traceLog struct {
	mu     sync.Mutex
	lines  []string
	notify chan struct{}
}

func newTraceLog() *traceLog { return &traceLog{notify: make(chan struct{}, 1)} }

func (l *traceLog) add(s string) {
	l.mu.Lock()
	l.lines = append(l.lines, s)
	l.mu.Unlock()
	select {
	case l.notify <- struct{}{}:
	default:
	}
}

func (l *traceLog) wait(t *testing.T, sub string, d time.Duration) {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		l.mu.Lock()
		found := false
		for _, s := range l.lines {
			if strings.Contains(s, sub) {
				found = true
				break
			}
		}
		l.mu.Unlock()
		if found {
			return
		}
		select {
		case <-l.notify:
		case <-deadline.C:
			t.Fatalf("no %q in the wire trace after %v", sub, d)
		}
	}
}

// With no symbols selected the T8 logger logs nothing but keeps the
// diagnostic session alive with periodic tester present frames on 0x101.
func TestT8NoSymbolsSendsTesterPresent(t *testing.T) {
	trace := newTraceLog()
	sim := &ecusim.T8{Log: trace.add}
	probe := (&gocan.Frame{ID: 0x101, Length: 3, Data: [8]byte{0xFE, 0x01, 0x3E}}).String()
	rec := newRecWriter()
	cl, _ := NewT8(testCfg(t, "T8", sim, nil, newMsgLog(t)), rec)
	runLogger(t, cl)
	trace.wait(t, "<- "+probe, 6*time.Second)
	if len(rec.snapshot()) != 0 {
		t.Fatal("rows logged without symbols")
	}
}

// A payload of the wrong size is skipped as a bad sample; repeated ones trip
// the error-rate abort.
func TestT8PayloadSizeMismatch(t *testing.T) {
	sim := &ecusim.T8{}
	sim.SetSymbol(100, []byte{1, 2, 3})
	msgs := newMsgLog(t)
	rec := newRecWriter()
	cl, _ := NewT8(testCfg(t, "T8", sim, []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 100, Length: 2}}, msgs), rec)
	if err := runLogger(t, cl).wait(t, 10*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	if !msgs.has("expected 2 bytes, got 3") || !msgs.has("too many errors, aborting logging") || len(rec.snapshot()) != 0 {
		t.Fatalf("%d rows, messages %v", len(rec.snapshot()), msgs.all())
	}
}

// A fatal adapter error ends the session with that error.
func TestT8FatalAdapterError(t *testing.T) {
	sim := &ecusim.T8{}
	sim.SetSymbol(100, []byte{0, 1})
	hook := newHook(sim, "", nil)
	rec := newRecWriter()
	cl, _ := NewT8(testCfg(t, "T8", hook, []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 100, Length: 2}}, newMsgLog(t)), rec)
	s := runLogger(t, cl)
	rec.waitRows(t, 2, 10*time.Second)
	unplugged := errors.New("usb unplugged")
	hook.fatal(unplugged)
	if err := s.wait(t, 5*time.Second); !errors.Is(err, unplugged) {
		t.Fatalf("Start = %v", err)
	}
	waitClosed(t, rec.closed, time.Second, "log writer close")
}

// With the ECU wideband source and AD scanner on, the configured input
// symbol is converted to lambda and logged in its own column.
func TestT8ADScannerColumn(t *testing.T) {
	sim := &ecusim.T8{}
	sim.SetSymbol(7, []byte{0x01, 0xF4}) // AD 500
	msgs := newMsgLog(t)
	cfg := testCfg(t, "T8", sim, []*symbol.Symbol{{Name: "AD_sond", Number: 7, Length: 2, Correctionfactor: 1}}, msgs)
	cfg.WidebandConfig = WidebandConfig{
		Name: "ECU", ADScanner: true, ADScannerSymbol: "AD_sond",
		SupportPoints: []int{0, 1000}, LambdaValues: []float64{0.5, 1.5},
	}
	rec := newRecWriter()
	cl, _ := NewT8(cfg, rec)
	runLogger(t, cl)
	row := rec.waitFor(t, 10*time.Second, func(r recRow) bool { return r.vals[LAMBDAADSCANNER] != 0 })
	if row.vals[LAMBDAADSCANNER] != 1.0 || row.text["AD_sond"] != "500" {
		t.Fatalf("row %v", row.text)
	}
}

// An external CAN wideband is sampled every frame into the
// Lambda.External column.
func TestT8ExternalWidebandColumn(t *testing.T) {
	sim := &ecusim.T8{}
	sim.SetSymbol(100, []byte{0, 1})
	// The AEM UEGO broadcasts on extended id 0x180, lambda*10000 in bytes 0-1;
	// send one alongside every request the logger makes.
	wbFrame := gocan.NewExtendedFrame(0x180, []byte{0x26, 0x8E, 0, 0, 0, 0, 0, 0})
	hook := newHook(sim, "", func(bus *gocan.Bus, f gocan.Frame) bool {
		if f.ID == 0x7E0 {
			bus.Deliver(wbFrame)
		}
		return false
	})
	msgs := newMsgLog(t)
	cfg := testCfg(t, "T8", hook, []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 100, Length: 2}}, msgs)
	cfg.WidebandConfig = WidebandConfig{Name: aem.ProductString, Port: "CAN"}
	rec := newRecWriter()
	cl, _ := NewT8(cfg, rec)
	runLogger(t, cl)
	row := rec.waitFor(t, 10*time.Second, func(r recRow) bool { return r.vals[EXTERNALWBLSYM] != 0 })
	if math.Abs(row.vals[EXTERNALWBLSYM]-0.987) > 1e-9 || row.text[EXTERNALWBLSYM] != "0.987" {
		t.Fatalf("row %v", row.text)
	}
}

// Start fails before logging when the wideband cannot be set up or the ECU
// does not answer the diagnostic session request.
func TestT8StartFailures(t *testing.T) {
	t.Run("wideband", func(t *testing.T) {
		cfg := testCfg(t, "T8", &ecusim.T8{}, nil, newMsgLog(t))
		cfg.WidebandConfig = WidebandConfig{Name: "Bogus 3000"}
		cl, _ := NewT8(cfg, newRecWriter())
		if err := runLogger(t, cl).wait(t, 5*time.Second); err == nil || !strings.Contains(err.Error(), "failed to create wideband lambda") {
			t.Fatalf("Start = %v", err)
		}
	})
	t.Run("silent ECU", func(t *testing.T) {
		silent := newHook(&ecusim.T8{}, "", func(*gocan.Bus, gocan.Frame) bool { return true })
		cl, _ := NewT8(testCfg(t, "T8", silent, nil, newMsgLog(t)), newRecWriter())
		if err := runLogger(t, cl).wait(t, 5*time.Second); err == nil || !strings.Contains(err.Error(), "failed to init t8 logging") {
			t.Fatalf("Start = %v", err)
		}
	})
}
