package datalogger

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// decodeT7Broadcast turns the 0x1A0/0x280/0x3A0/0x5C0 broadcast frames into
// rpm, pedal, gear, brake/clutch bits, vehicle speed and coolant sysvars,
// ignoring frames too short to carry their fields.
func TestDecodeT7Broadcast(t *testing.T) {
	tests := []struct {
		name string
		id   uint16
		data []byte
		want map[string]float64
	}{
		{"engine", 0x1A0, []byte{0, 0x05, 0xDC, 0, 0, 33, 0, 0}, map[string]float64{"ActualIn.n_Engine": 1500, "Out.X_AccPedal": 33}},
		{"gear brake clutch", 0x280, []byte{0, 4, 0x0A, 0, 0}, map[string]float64{"Out.X_ActualGear": 4, "Out.ST_BrakeLight": 1, "In.ST_ClutchBrake1": 1}},
		{"gear no pedals", 0x280, []byte{0, 2, 0xF5, 0, 0}, map[string]float64{"Out.X_ActualGear": 2, "Out.ST_BrakeLight": 0, "In.ST_ClutchBrake1": 0}},
		{"speed", 0x3A0, []byte{0, 0, 0, 0x01, 0xF4}, map[string]float64{"In.v_Vehicle": 50}},
		{"coolant", 0x5C0, []byte{0, 130}, map[string]float64{"ActualIn.T_Engine": 90}},
		{"coolant below zero", 0x5C0, []byte{0, 10}, map[string]float64{"ActualIn.T_Engine": -30}},
		{"short engine", 0x1A0, []byte{0, 1, 2, 3, 4}, nil},
		{"short gear", 0x280, []byte{0, 1, 2, 3}, nil},
		{"short speed", 0x3A0, []byte{0, 1, 2, 3}, nil},
		{"short coolant", 0x5C0, []byte{0}, nil},
		{"unknown id", 0x123, []byte{1, 2, 3, 4, 5, 6, 7, 8}, nil},
	}
	for _, tt := range tests {
		sv := NewThreadSafeMap()
		decodeT7Broadcast(tt.id, tt.data, sv)
		keys := sv.Keys()
		if len(keys) != len(tt.want) {
			t.Errorf("%s: set %v, want %v", tt.name, keys, tt.want)
			continue
		}
		for k, v := range tt.want {
			if got := sv.Get(k); got != v {
				t.Errorf("%s: %s = %v, want %v", tt.name, k, got, v)
			}
		}
	}
}

// The 0x280 frame's limp-home, cruise and check-engine bits go to the ebus
// only, not the log.
func TestDecodeT7BroadcastStatusBits(t *testing.T) {
	limp, cruise, cel := collectEbus(t, "LIMP"), collectEbus(t, "CRUISE"), collectEbus(t, "CEL")
	sv := NewThreadSafeMap()
	decodeT7Broadcast(0x280, []byte{0, 1, 0, 0x01, 0xA0}, sv)
	decodeT7Broadcast(0x280, []byte{0, 1, 0, 0xFE, 0x5F}, sv)
	for name, e := range map[string]*ebusValues{"LIMP": limp, "CRUISE": cruise, "CEL": cel} {
		if got := e.all(); !slices.Equal(got, []float64{1, 0}) {
			t.Errorf("%s published %v, want [1 0]", name, got)
		}
		if sv.Exists(name) {
			t.Errorf("%s stored as a sysvar", name)
		}
	}
}

// FCutToStringT7 names every documented fuel cut reason and "Unknown"
// otherwise.
func TestFCutToStringT7(t *testing.T) {
	tests := map[float64]string{
		0:  "No fuelcut",
		1:  "Ignition key turned off",
		2:  "Accelerator pedal pressed during start",
		3:  "RPM limiter (engine speed guard)",
		4:  "Throttle block adaption active 1st time",
		5:  "Airmass limit (pressure guard)",
		6:  "Airmass limit (pressure guard)",
		7:  "Immobilizer code incorrect",
		8:  "Current to h-bridge to high during throttle limphome",
		9:  "Torque to high during throttle limphome",
		10: "Unknown",
		11: "Tampering protection of throttle",
		12: "Error on all ignition trigger outputs",
		13: "ECU not correctly programmed",
		14: "To high rpm in throttle limp home, pedal potentiometer fault",
		15: "Torque master fuel cut request",
		16: "TCM requests fuelcut to smoothen gear shift",
		20: "Application conditions for fuel cut",
		21: "Unknown",
		-1: "Unknown",
	}
	for v, want := range tests {
		if got := FCutToStringT7(v); got != want {
			t.Errorf("FCutToStringT7(%v) = %q, want %q", v, got, want)
		}
	}
}

// The T7 special publisher splits the 8-byte IgnKnk.fi_Offset into four
// signed per-cylinder offsets in degrees, routes the AD scanner input through
// the lambda table and leaves every other symbol to the caller.
func TestT7SpecialPublisher(t *testing.T) {
	bl := newTestBase(t, Config{WidebandConfig: WidebandConfig{
		Name: "ECU", ADScanner: true, ADScannerSymbol: "DisplProt.AD_Scanner",
		SupportPoints: []int{0, 1000}, LambdaValues: []float64{0.5, 1.5},
	}})
	publish := bl.t7SpecialPublisher()
	cyl := make([]*ebusValues, 4)
	for i, name := range ignOffsetCylNames {
		cyl[i] = collectEbus(t, name)
	}

	ign := &symbol.Symbol{Name: "IgnKnk.fi_Offset", Length: 8}
	_ = ign.SetData([]byte{0x00, 0x0A, 0xFF, 0xF6, 0x00, 0x00, 0x01, 0x00})
	if !publish(ign) {
		t.Fatal("ignition offsets not handled")
	}
	for i, want := range []float64{1, -1, 0, 25.6} {
		if got, ok := cyl[i].last(); !ok || got != want {
			t.Errorf("cylinder %d offset %v, want %v", i+1, got, want)
		}
	}

	short := &symbol.Symbol{Name: "IgnKnk.fi_Offset", Length: 4}
	_ = short.SetData([]byte{0, 1, 0, 2})
	if publish(short) {
		t.Fatal("4-byte offset symbol handled")
	}

	ad := &symbol.Symbol{Name: "DisplProt.AD_Scanner", Length: 2}
	_ = ad.SetData([]byte{0x01, 0xF4}) // 500
	if !publish(ad) || bl.sysvars.Get(LAMBDAADSCANNER) != 1.0 {
		t.Fatalf("AD scanner lambda %v", bl.sysvars.Get(LAMBDAADSCANNER))
	}

	other := &symbol.Symbol{Name: "MAF.m_AirInlet", Length: 2}
	_ = other.SetData([]byte{0, 1})
	if publish(other) {
		t.Fatal("ordinary symbol handled")
	}
}

// The broadcast listener decodes subscribed broadcast frames into the
// sysvars until its context ends.
func TestT7BroadcastListener(t *testing.T) {
	cl := openBus(t, &gocan.Loopback{})
	sv := NewThreadSafeMap()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		t7broadcastListener(ctx, cl, sv)
		close(done)
	}()
	frame := gocan.NewFrame(0x3A0, []byte{0, 0, 0, 0x03, 0xE8, 0, 0, 0})
	deadline := time.Now().Add(5 * time.Second)
	for sv.Get("In.v_Vehicle") != 100 {
		if time.Now().After(deadline) {
			t.Fatal("broadcast frame not decoded")
		}
		_ = cl.Send(ctx, gocan.NewFrame(0x123, []byte{1})) // not subscribed
		_ = cl.Send(ctx, frame)
		select {
		case <-time.After(5 * time.Millisecond):
		case <-done:
			t.Fatal("listener stopped early")
		}
	}
	if sv.Exists("ActualIn.n_Engine") || len(sv.Keys()) != 1 {
		t.Fatalf("unexpected sysvars %v", sv.Keys())
	}
	cancel()
	waitClosed(t, done, 5*time.Second, "listener exit")
}

// obd wraps a fake T7 in an adapter named like an OBDLink, which the T7
// logger does not listen for broadcasts on.
func obd(sim gocan.Adapter) *hookAdapter { return newHook(sim, "OBDLink SX", nil) }

// On OBDLink/STN/ELM adapters the T7 logger skips the broadcast listener and
// polls every symbol, including those a broadcast could have served.
func TestT7OBDAdapterPollsEverything(t *testing.T) {
	sim := &ecusim.T7{Broadcast: 10 * time.Millisecond}
	sim.SetSymbol(1, []byte{0x0B, 0xB8}) // 3000, not the broadcast's 1500
	sim.SetSymbol(100, []byte{0x12, 0x34})
	syms := []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: 1, Length: 2, Correctionfactor: 1},
		{Name: "MAF.m_AirInlet", Number: 100, Length: 2, Correctionfactor: 1},
	}
	msgs := newMsgLog(t)
	rec := newRecWriter()
	cl, _ := NewT7(testCfg(t, "T7", obd(sim), syms, msgs), rec)
	runLogger(t, cl)
	row := rec.waitRows(t, 3, 10*time.Second)[2]
	if !slices.Equal(row.names, []string{"ActualIn.n_Engine", "MAF.m_AirInlet"}) {
		t.Fatalf("columns %v", row.names)
	}
	if row.text["ActualIn.n_Engine"] != "3000" || row.text["MAF.m_AirInlet"] != "4660" {
		t.Fatalf("row %v", row.text)
	}
	if msgs.has("Watching for broadcast messages") || syms[0].Number != 1 {
		t.Fatal("listened for broadcasts on an OBD adapter")
	}
}

// Without broadcast traffic the T7 logger drops the listener after its
// watch window and polls the would-be broadcast symbols.
func TestT7NoBroadcastFallsBackToPolling(t *testing.T) {
	sim := &ecusim.T7{}
	sim.SetSymbol(1, []byte{0x03, 0x20}) // 800
	syms := []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 1, Length: 2, Correctionfactor: 1}}
	msgs := newMsgLog(t)
	rec := newRecWriter()
	cl, _ := NewT7(testCfg(t, "T7", sim, syms, msgs), rec)
	runLogger(t, cl)
	row := rec.waitRows(t, 2, 10*time.Second)[1]
	if !msgs.has("No broadcast messages found") || row.text["ActualIn.n_Engine"] != "800" {
		t.Fatalf("row %v, messages %v", row.text, msgs.all())
	}
}

// A custom seed/key pair from the loaded binary unlocks an ECU that answers
// to none of the stock pairs.
func TestT7CustomSeedKey(t *testing.T) {
	custom := t7kwp.SeedKey{XOR: 0x1234, Sub: 0x0FED}
	sim := &ecusim.T7{SeedKey: custom}
	sim.SetSymbol(100, []byte{0, 7})
	msgs := newMsgLog(t)
	cfg := testCfg(t, "T7", obd(sim), []*symbol.Symbol{{Name: "MAF.m_AirInlet", Number: 100, Length: 2, Correctionfactor: 1}}, msgs)
	cfg.SeedKey = &custom
	rec := newRecWriter()
	cl, _ := NewT7(cfg, rec)
	runLogger(t, cl)
	if row := rec.waitRows(t, 1, 10*time.Second)[0]; row.text["MAF.m_AirInlet"] != "7" {
		t.Fatalf("row %v", row.text)
	}
	if !msgs.has("Security access granted") {
		t.Fatalf("messages %v", msgs.all())
	}
}

// RAM writes longer than one 36-byte KWP write are split and requeued until
// all of it has landed; reads return the full range.
func TestT7LargeRAMTransfer(t *testing.T) {
	sim := &ecusim.T7{}
	sim.SetSymbol(100, []byte{0, 1})
	rec := newRecWriter()
	cl, _ := NewT7(testCfg(t, "T7", obd(sim), []*symbol.Symbol{{Name: "MAF.m_AirInlet", Number: 100, Length: 2}}, newMsgLog(t)), rec)
	runLogger(t, cl)
	rec.waitRows(t, 1, 10*time.Second)

	data := make([]byte, 80)
	for i := range data {
		data[i] = byte(i + 1)
	}
	if err := cl.SetRAM(0xF00200, data); err != nil {
		t.Fatal(err)
	}
	if got := sim.Peek(0xF00200, len(data)); !bytes.Equal(got, data) {
		t.Fatalf("ECU RAM % X", got)
	}
	got, err := cl.GetRAM(0xF00200, uint32(len(data)))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("GetRAM = % X %v", got, err)
	}
}

// A payload of the wrong size is an error every tick; with more than five a
// second the T7 logger gives up and ends the session.
func TestT7PayloadSizeMismatchAborts(t *testing.T) {
	sim := &ecusim.T7{}
	sim.SetSymbol(100, []byte{1, 2, 3}) // three bytes where two are expected
	msgs := newMsgLog(t)
	rec := newRecWriter()
	cl, _ := NewT7(testCfg(t, "T7", obd(sim), []*symbol.Symbol{{Name: "MAF.m_AirInlet", Number: 100, Length: 2}}, msgs), rec)
	if err := runLogger(t, cl).wait(t, 10*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	if !msgs.has("expected 2 bytes, got 3") || !msgs.has("too many errors, aborting logging") {
		t.Fatalf("messages %v", msgs.all())
	}
	if len(rec.snapshot()) != 0 {
		t.Fatal("bad payload was logged")
	}
}

// A failing log writer counts as an error per frame on T7, so the session
// keeps going until the error rate trips the abort.
func TestT7LogWriteErrorsAbort(t *testing.T) {
	sim := &ecusim.T7{}
	sim.SetSymbol(100, []byte{0, 1})
	msgs := newMsgLog(t)
	rec := newRecWriter()
	rec.err = errors.New("disk full")
	cl, _ := NewT7(testCfg(t, "T7", obd(sim), []*symbol.Symbol{{Name: "MAF.m_AirInlet", Number: 100, Length: 2}}, msgs), rec)
	if err := runLogger(t, cl).wait(t, 10*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	if !msgs.has("failed to write log: disk full") || !msgs.has("too many errors") || len(rec.snapshot()) <= 5 {
		t.Fatalf("%d rows, messages %v", len(rec.snapshot()), msgs.all())
	}
}

// An ECU that does not answer the session start fails Start with an init
// error and closes the log.
func TestT7InitFailure(t *testing.T) {
	silent := newHook(&ecusim.T7{}, "OBDLink SX", func(*gocan.Bus, gocan.Frame) bool { return true })
	rec := newRecWriter()
	cl, _ := NewT7(testCfg(t, "T7", silent, nil, newMsgLog(t)), rec)
	err := runLogger(t, cl).wait(t, 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "failed to init t7 logging") {
		t.Fatalf("Start = %v", err)
	}
	waitClosed(t, rec.closed, time.Second, "log writer close")
}
