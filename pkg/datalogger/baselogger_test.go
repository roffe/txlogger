package datalogger

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
)

// fakeLambda is a wideband that always reads lambda.
type fakeLambda struct{ lambda float64 }

func (f *fakeLambda) GetLambda() float64          { return f.lambda }
func (f *fakeLambda) Start(context.Context) error { return nil }
func (f *fakeLambda) Stop()                       {}
func (f *fakeLambda) String() string              { return "fake" }

func newTestBase(t *testing.T, cfg Config) *BaseLogger {
	bl := NewBaseLogger(cfg, nopWriter{})
	t.Cleanup(bl.secondTicker.Stop)
	return bl
}

// onCapture counts every frame, reports the running total every 15th frame
// and publishes the frame timestamp on the ebus frame topic.
func TestOnCapture(t *testing.T) {
	var reported []int
	cfg := Config{CaptureCounter: func(n int) { reported = append(reported, n) }}
	bl := newTestBase(t, cfg)
	frames := collectEbus(t, ebus.TOPIC_FRAME)

	t0 := time.UnixMilli(1_760_000_000_000)
	for i := range 31 {
		bl.onCapture(t0.Add(time.Duration(i) * time.Millisecond))
	}
	if !slices.Equal(reported, []int{15, 30}) {
		t.Fatalf("CaptureCounter got %v, want [15 30]", reported)
	}
	if bl.captureCount != 31 || bl.capturePerSecond != 31 {
		t.Fatalf("counts %d/%d", bl.captureCount, bl.capturePerSecond)
	}
	published := frames.all()
	for i := range 31 {
		if want := float64(t0.Add(time.Duration(i) * time.Millisecond).UnixMilli()); !slices.Contains(published, want) {
			t.Fatalf("frame %d timestamp %v not published", i, want)
		}
	}
}

// onError reports every error immediately; resetPerSecond clears only the
// per-second counts the abort check uses.
func TestOnErrorAndResetPerSecond(t *testing.T) {
	var reported []int
	bl := newTestBase(t, Config{
		CaptureCounter: func(int) {},
		ErrorCounter:   func(n int) { reported = append(reported, n) },
	})
	bl.onError()
	bl.onError()
	bl.onCapture(time.Now())
	if !slices.Equal(reported, []int{1, 2}) || bl.errPerSecond.Load() != 2 {
		t.Fatalf("reported %v, per second %d", reported, bl.errPerSecond.Load())
	}
	bl.resetPerSecond()
	if bl.errPerSecond.Load() != 0 || bl.capturePerSecond != 0 || bl.errCount.Load() != 2 || bl.captureCount != 1 {
		t.Fatalf("after reset: errs %d/%d captures %d/%d", bl.errPerSecond.Load(), bl.errCount.Load(), bl.capturePerSecond, bl.captureCount)
	}
}

// appendExtraSysvars adds the external wideband column when a wideband is
// running, the AD scanner column only when the ECU source has the scanner on,
// and script outputs not already present.
func TestAppendExtraSysvars(t *testing.T) {
	tests := []struct {
		name string
		lamb bool
		wb   WidebandConfig
		want []string
	}{
		{"nothing", false, WidebandConfig{Name: "None"}, []string{"base"}},
		{"external wideband", true, WidebandConfig{Name: ecumaster.ProductString}, []string{"base", EXTERNALWBLSYM}},
		{"ECU AD scanner", false, WidebandConfig{Name: "ECU", ADScanner: true}, []string{"base", LAMBDAADSCANNER}},
		{"AD scanner needs ECU source", false, WidebandConfig{Name: "None", ADScanner: true}, []string{"base"}},
	}
	for _, tt := range tests {
		bl := newTestBase(t, Config{WidebandConfig: tt.wb})
		if tt.lamb {
			bl.lamb = &fakeLambda{}
		}
		if got := bl.appendExtraSysvars([]string{"base"}); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}

	bl := newTestBase(t, Config{})
	bl.lw = &scriptWriter{lw: nopWriter{}, scripts: []*luaScript{
		{outputs: []string{"RPM.RoC", "base"}},
		{outputs: []string{"Boost.Error"}},
	}}
	if got := bl.appendExtraSysvars([]string{"base"}); !slices.Equal(got, []string{"base", "RPM.RoC", "Boost.Error"}) {
		t.Errorf("script outputs: got %v", got)
	}
}

// buildChannels puts every sysvar (plus active extras) first as sysvar
// channels, then the polled symbols in order, skipping symbols with a
// negative number (sourced from broadcasts or elsewhere).
func TestBuildChannels(t *testing.T) {
	syms := []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: -1, Length: 2},
		{Name: "MAF.m_AirInlet", Number: 10, Length: 2, Correctionfactor: 1},
		{Name: "Out.PWM_BoostCntrl", Number: 0, Length: 2, Correctionfactor: 0.1},
	}
	bl := newTestBase(t, Config{Symbols: syms, WidebandConfig: WidebandConfig{Name: "ECU", ADScanner: true}})
	bl.sysvars.Set("ActualIn.n_Engine", 900)
	bl.sysvars.Set("In.v_Vehicle", 55.5)
	bl.lamb = &fakeLambda{}

	chans := bl.buildChannels()
	names := make([]string, len(chans))
	for i := range chans {
		names[i] = chans[i].Name
	}
	head := slices.Clone(names[:4])
	slices.Sort(head)
	if !slices.Equal(head, []string{"ActualIn.n_Engine", "In.v_Vehicle", LAMBDAADSCANNER, EXTERNALWBLSYM}) {
		t.Fatalf("sysvar channels %v", names[:4])
	}
	if !slices.Equal(names[4:], []string{"MAF.m_AirInlet", "Out.PWM_BoostCntrl"}) {
		t.Fatalf("symbol channels %v", names[4:])
	}
	_ = syms[2].SetData([]byte{0x01, 0x2C}) // 300 * 0.1
	if got := chans[5].String(); got != "30.0" {
		t.Fatalf("symbol channel = %q", got)
	}
	if got := chans[slices.Index(names, "In.v_Vehicle")].String(); got != "55.50" {
		t.Fatalf("sysvar channel = %q", got)
	}
}

// The AD scanner hook converts the configured input symbol's AD value to
// lambda through the support point table, stores and publishes it, and
// ignores every other symbol.
func TestADScannerFunc(t *testing.T) {
	wb := WidebandConfig{
		Name: "ECU", ADScanner: true, ADScannerSymbol: "AD_EGR",
		SupportPoints: []int{0, 1000}, LambdaValues: []float64{0.5, 1.5},
	}
	bl := newTestBase(t, Config{WidebandConfig: wb})
	pub := collectEbus(t, LAMBDAADSCANNER)
	scan := bl.adScannerFunc()

	if scan("Rpm", 500) {
		t.Fatal("non-scanner symbol handled")
	}
	if !scan("AD_EGR", 250) {
		t.Fatal("scanner symbol not handled")
	}
	if got := bl.sysvars.Get(LAMBDAADSCANNER); got != 0.75 {
		t.Fatalf("sysvar = %v", got)
	}
	if got, _ := pub.last(); got != 0.75 {
		t.Fatalf("published %v", got)
	}

	wb.ADScanner = false
	off := newTestBase(t, Config{WidebandConfig: wb}).adScannerFunc()
	if off("AD_EGR", 250) {
		t.Fatal("scanner disabled but symbol handled")
	}
}

// publishExternalWBL samples a running wideband into the sysvars and the ebus
// and does nothing without one.
func TestPublishExternalWBL(t *testing.T) {
	bl := newTestBase(t, Config{})
	bl.publishExternalWBL()
	if bl.sysvars.Exists(EXTERNALWBLSYM) {
		t.Fatal("published without a wideband")
	}
	pub := collectEbus(t, EXTERNALWBLSYM)
	bl.lamb = &fakeLambda{lambda: 1.023}
	bl.publishExternalWBL()
	if bl.sysvars.Get(EXTERNALWBLSYM) != 1.023 {
		t.Fatalf("sysvar = %v", bl.sysvars.Get(EXTERNALWBLSYM))
	}
	if got, _ := pub.last(); got != 1.023 {
		t.Fatalf("published %v", got)
	}
}

// setupWBL leaves no wideband for the None/ECU sources, refuses unknown types
// and starts a CAN wideband that then feeds publishExternalWBL.
func TestSetupWBL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl, err := gocan.OpenAdapter(ctx, &gocan.Loopback{})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	for _, name := range []string{"None", "ECU"} {
		bl := newTestBase(t, Config{WidebandConfig: WidebandConfig{Name: name}})
		if err := bl.setupWBL(ctx, cl); err != nil || bl.lamb != nil {
			t.Fatalf("%s: err %v lamb %v", name, err, bl.lamb)
		}
	}

	bl := newTestBase(t, Config{WidebandConfig: WidebandConfig{Name: "Bogus 3000"}})
	if err := bl.setupWBL(ctx, cl); err == nil || !strings.Contains(err.Error(), "failed to create wideband lambda") {
		t.Fatalf("unknown type: %v", err)
	}

	msgs := newMsgLog(t)
	bl = newTestBase(t, Config{OnMessage: msgs.add, WidebandConfig: WidebandConfig{Name: aem.ProductString, Port: "CAN"}})
	if err := bl.setupWBL(ctx, cl); err != nil {
		t.Fatal(err)
	}
	defer bl.lamb.Stop()
	// AEM UEGO CAN frame (extended id 0x180): lambda*10000 in bytes 0-1.
	frame := gocan.NewExtendedFrame(0x180, []byte{0x26, 0x8E, 0x00, 0x00, 0x8C, 0x00, 0x00, 0x00})
	waitLambda(t, ctx, cl, bl, frame, 0.987)
	bl.publishExternalWBL()
	if got := bl.sysvars.Get(EXTERNALWBLSYM); math.Abs(got-0.987) > 1e-9 {
		t.Fatalf("sysvar = %v", got)
	}
	if got := bl.appendExtraSysvars(nil); !slices.Equal(got, []string{EXTERNALWBLSYM}) {
		t.Fatalf("extra sysvars %v", got)
	}
}

// waitLambda sends frame on the bus until the wideband reads want.
func waitLambda(t *testing.T, ctx context.Context, cl *gocan.Bus, bl *BaseLogger, frame gocan.Frame, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for math.Abs(bl.lamb.GetLambda()-want) > 1e-9 {
		if time.Now().After(deadline) {
			t.Fatalf("lambda = %v, want %v", bl.lamb.GetLambda(), want)
		}
		if err := cl.Send(ctx, frame); err != nil {
			t.Fatal(err)
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-ctx.Done():
		}
	}
}

// The loggers stop their wideband (lamb.Stop) while its reader goroutine may
// still be running; for the LAMBDA to CAN module that must be race free.
// Run with -race to see the failure.
func TestSetupWBLLambdaToCANStopIsRaceFree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl, err := gocan.OpenAdapter(ctx, &gocan.Loopback{})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	bl := newTestBase(t, Config{OnMessage: func(string) {}, WidebandConfig: WidebandConfig{Name: ecumaster.ProductString}})
	if err := bl.setupWBL(ctx, cl); err != nil {
		t.Fatal(err)
	}
	// LAMBDA to CAN status frame: lambda*1000 in bytes 4-5.
	waitLambda(t, ctx, cl, bl, gocan.NewFrame(0x664, []byte{0x05, 0x00, 0x80, 0xC8, 0x03, 0xDB, 0x00, 0x00}), 0.987)
	bl.lamb.Stop()
}

// A wideband set to read through the txbridge port is ignored, with a
// message, when the adapter is not a txbridge.
func TestSetupWBLTxbridgePortOnPlainAdapter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl, err := gocan.OpenAdapter(ctx, &gocan.Loopback{})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	msgs := newMsgLog(t)
	bl := newTestBase(t, Config{OnMessage: msgs.add, WidebandConfig: WidebandConfig{Name: aem.ProductString, Port: "txbridge"}})
	if err := bl.setupWBL(ctx, cl); err != nil || bl.lamb != nil {
		t.Fatalf("err %v lamb %v", err, bl.lamb)
	}
	if !msgs.has("txbridge port is selected but not using txbridge adapter") {
		t.Fatalf("messages %v", msgs.all())
	}
}

// SetRAM and GetRAM refuse with "busy" instead of blocking when a request is
// already queued for the logger loop.
func TestRAMRequestsBusy(t *testing.T) {
	bl := newTestBase(t, Config{})
	bl.writeChan <- NewWriteDataRequest(0, []byte{1})
	bl.readChan <- NewReadDataRequest(0, 1)
	if err := bl.SetRAM(0x10, []byte{1}); err == nil || err.Error() != "busy" {
		t.Fatalf("SetRAM = %v", err)
	}
	if _, err := bl.GetRAM(0x10, 1); err == nil || err.Error() != "busy" {
		t.Fatalf("GetRAM = %v", err)
	}
}

// GetRAM and SetRAM hand the request to the logger loop and return what the
// loop completed it with.
func TestRAMRequestsServedByLoop(t *testing.T) {
	bl := newTestBase(t, Config{})
	var wg sync.WaitGroup
	wg.Go(func() {
		r := <-bl.readChan
		r.Data = []byte{0xDE, 0xAD}
		r.Complete(nil)
		w := <-bl.writeChan
		if w.Address != 0x20 || !slices.Equal(w.Data, []byte{9, 8}) {
			t.Errorf("write request %+v", w)
		}
		w.Complete(nil)
	})
	got, err := bl.GetRAM(0x10, 2)
	if err != nil || !slices.Equal(got, []byte{0xDE, 0xAD}) {
		t.Fatalf("GetRAM = % X %v", got, err)
	}
	if err := bl.SetRAM(0x20, []byte{9, 8}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

// Close may be called repeatedly; the quit channel is closed once.
func TestBaseLoggerCloseIdempotent(t *testing.T) {
	bl := newTestBase(t, Config{})
	bl.Close()
	bl.Close()
	select {
	case <-bl.quitChan:
	default:
		t.Fatal("quit channel not closed")
	}
}
