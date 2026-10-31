package t7

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/model"
)

// The package registers itself as "Trionic 7": 500 kbit/s, a receive filter
// that includes the fast-upload stream id except on OBD dongles (or fast dumps
// starve on adapters with a hardware filter), and a manual reset so flashing
// never resets an ECU with the ignition on.
func TestRegistration(t *testing.T) {
	info := ecu.Info("Trionic 7")
	if info.CANRate != 500 {
		t.Errorf("CANRate = %v, want 500", info.CANRate)
	}
	if !info.ManualReset {
		t.Error("ManualReset = false: a CAN reset with the ignition on puts the throttle body in limp mode")
	}
	replies := []uint32{t7kwp.INIT_RESP_ID, 0x258, t7kwp.RESP_CHUNK_CONF_ID, t7kwp.REQ_CHUNK_CONF_ID}
	for adapter, want := range map[string][]uint32{
		"CANusb":     append(slices.Clone(replies), t7kwp.FAST_UP_ID),
		"OBDLink SX": replies,
	} {
		filter := info.CANFilter(adapter)
		for _, id := range want {
			if !slices.Contains(filter, id) {
				t.Errorf("%s filter %X lacks 0x%03X", adapter, filter, id)
			}
		}
		if len(filter) != len(want) {
			t.Errorf("%s filter %X, want only %X", adapter, filter, want)
		}
	}

	bus, err := gocan.OpenAdapter(context.Background(), &fakeT7{})
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c, err := ecu.New(bus, &ecu.Config{Name: "Trionic 7"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*Client); !ok {
		t.Fatalf("ecu.New returned %T, want *t7.Client", c)
	}
}

// Marrying and recovery are T8 operations; the T7 client refuses them.
func TestUnsupportedOperations(t *testing.T) {
	e := &fakeT7{}
	c := newClient(t, e, nil)
	if err := c.MarryECU(testCtx(t), "x"); err == nil || err.Error() != "not supported" {
		t.Errorf("MarryECU err = %v, want \"not supported\"", err)
	}
	if err := c.RecoverECU(testCtx(t), nil); err == nil || err.Error() != "not supported" {
		t.Errorf("RecoverECU err = %v, want \"not supported\"", err)
	}
	if len(e.trace) != 0 {
		t.Errorf("refused operations talked to the ECU: %q", e.trace)
	}
}

// DataInitialization opens the session once and reuses it: the ECU ignores a
// second startCommunication while a session is live. StopSession drops the
// cache, so the next operation opens a fresh session.
func TestDataInitializationReusesSession(t *testing.T) {
	ctx := testCtx(t)
	e := &fakeT7{}
	c := newClient(t, e, nil)

	for range 2 {
		if err := c.DataInitialization(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.count("init"); n != 1 {
		t.Fatalf("startCommunication sent %d times, want 1 (trace %q)", n, e.trace)
	}

	if err := c.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.lastInit.IsZero() {
		t.Fatal("StopSession kept the init cache")
	}
	if err := c.DataInitialization(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"init", "82", "init"}; !slices.Equal(e.trace, want) {
		t.Fatalf("trace %q, want %q", e.trace, want)
	}
}

// A session left live by an earlier tester swallows startCommunication.
// DataInitialization then sends stopCommunication to end it and retries,
// reporting the first failure through OnError.
func TestDataInitializationEndsStaleSession(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{phantom: true}
	c := newClient(t, e, rec)

	if err := c.DataInitialization(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"init ignored", "82", "init"}; !slices.Equal(e.trace, want) {
		t.Fatalf("trace %q, want %q", e.trace, want)
	}
	if len(rec.errs) != 1 {
		t.Fatalf("OnError called %d times, want once for the first failed attempt: %v", len(rec.errs), rec.errs)
	}
	if c.lastInit.IsZero() {
		t.Fatal("successful init not cached")
	}
}

// An ECU that never answers fails DataInitialization once the context ends,
// and nothing is cached.
func TestDataInitializationSilentECU(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c := newClient(t, &fakeT7{silent: true}, nil)

	err := c.DataInitialization(ctx)
	if err == nil || !strings.Contains(err.Error(), "Data initialization failed") {
		t.Fatalf("err = %v, want a data initialization failure", err)
	}
	if !c.lastInit.IsZero() {
		t.Fatal("failed init was cached")
	}
}

// KnockKnock gains security access at level 05 with the first stock pair
// the ECU accepts, sending key = ((seed<<2) ^ XOR) - SUB.
func TestKnockKnockStockPair(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{seed: 0xBEEF}
	c := newClient(t, e, rec)

	ok, err := c.KnockKnock(testCtx(t))
	if err != nil || !ok {
		t.Fatalf("KnockKnock = %v, %v", ok, err)
	}
	// ((0xBEEF<<2) ^ 0x8142) - 0x2356 in 16 bits: 0xFBBC ^ 0x8142 = 0x7AFE, - 0x2356 = 0x57A8
	if want := []uint16{0x57A8}; !slices.Equal(e.keys, want) {
		t.Fatalf("keys sent %04X, want %04X", e.keys, want)
	}
	if want := []string{"init", "27 05", "27 06"}; !slices.Equal(e.trace, want) {
		t.Fatalf("trace %q, want %q", e.trace, want)
	}
	if !rec.hasMsg("Security access obtained (XOR 8142 SUB 2356)") {
		t.Fatalf("messages %q lack the pair that worked", rec.msgs)
	}
}

// A custom pair from the config is tried before the stock pairs, so a
// re-patched ECU is unlocked with a single exchange.
func TestKnockKnockCustomPairFirst(t *testing.T) {
	custom := ecu.SeedKey{XOR: 0x03E3, Sub: 0x2356}
	rec := &recorder{}
	e := &fakeT7{seedKey: custom}
	cfg := rec.config()
	cfg.SeedKey = &custom
	bus, err := gocan.OpenAdapter(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c := New(bus, cfg).(*Client)

	ok, err := c.KnockKnock(testCtx(t))
	if err != nil || !ok {
		t.Fatalf("KnockKnock = %v, %v", ok, err)
	}
	// ((0x1234<<2) ^ 0x03E3) - 0x2356 = 0x27DD
	if want := []uint16{0x27DD}; !slices.Equal(e.keys, want) {
		t.Fatalf("keys sent %04X, want %04X: the custom pair goes first", e.keys, want)
	}
	if !rec.hasMsg("Using custom seed/key XOR 03E3 SUB 2356") {
		t.Fatalf("messages %q do not announce the custom pair", rec.msgs)
	}
}

// A custom pair the ECU rejects falls back to the stock pairs, and the
// rejection is reported through OnError.
func TestKnockKnockFallsBackToStock(t *testing.T) {
	skipWaits(t)
	wrong := ecu.SeedKey{XOR: 0x1111, Sub: 0x2222}
	rec := &recorder{}
	e := &fakeT7{}
	cfg := rec.config()
	cfg.SeedKey = &wrong
	bus, err := gocan.OpenAdapter(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c := New(bus, cfg).(*Client)

	ok, err := c.KnockKnock(testCtx(t))
	if err != nil || !ok {
		t.Fatalf("KnockKnock = %v, %v", ok, err)
	}
	// seed 0x1234: custom ((0x48D0 ^ 0x1111) - 0x2222) = 0x379F, then stock
	// ((0x48D0 ^ 0x8142) - 0x2356) = 0xA63C
	want := []uint16{0x379F, 0xA63C}
	if !slices.Equal(e.keys, want) {
		t.Fatalf("keys sent %04X, want %04X", e.keys, want)
	}
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), "Invalid key") {
		t.Fatalf("OnError got %v, want one invalid-key error", rec.errs)
	}
}

// When the ECU refuses every pair KnockKnock returns false with an error,
// after reporting each refusal through OnError.
func TestKnockKnockAllRefused(t *testing.T) {
	skipWaits(t)
	stock := t7kwp.KnownSeedKeys
	t.Cleanup(func() { t7kwp.KnownSeedKeys = stock })
	t7kwp.KnownSeedKeys = []t7kwp.SeedKey{{XOR: 0x1111, Sub: 0x2222}}

	rec := &recorder{}
	e := &fakeT7{seedKey: stock[0]}
	c := newClient(t, e, rec)

	ok, err := c.KnockKnock(testCtx(t))
	if ok || err == nil || err.Error() != "/!\\ Failed to obtain security access" {
		t.Fatalf("KnockKnock = %v, %v; want false and the security access error", ok, err)
	}
	if want := []uint16{0x379F}; !slices.Equal(e.keys, want) {
		t.Fatalf("keys sent %04X, want %04X", e.keys, want)
	}
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0].Error(), "Invalid key") {
		t.Fatalf("OnError got %v, want one invalid-key error", rec.errs)
	}
	if e.unlocked {
		t.Fatal("ECU unlocked")
	}
}

// Cancelling the context during the back-off after a refused key ends
// KnockKnock with the context error instead of trying the next pair.
func TestKnockKnockCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(testCtx(t))
	wrong := ecu.SeedKey{XOR: 0x1111, Sub: 0x2222}
	e := &fakeT7{}
	cfg := (&recorder{}).config()
	cfg.SeedKey = &wrong
	cfg.OnError = func(error) { cancel() }
	bus, err := gocan.OpenAdapter(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c := New(bus, cfg).(*Client)

	ok, err := c.KnockKnock(ctx)
	if ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("KnockKnock = %v, %v; want false, context.Canceled", ok, err)
	}
	if want := []uint16{0x379F}; !slices.Equal(e.keys, want) {
		t.Fatalf("keys sent %04X, want only the custom key %04X", e.keys, want)
	}
}

// Info reads every T7Headers field in order with 0x1A, trims NUL padding,
// appends the level-3 seed with the two XOR interpretations without sending a
// key, and closes the session afterwards.
func TestInfo(t *testing.T) {
	e := &fakeT7{
		seed: 0x1234,
		headers: map[byte][]byte{
			0x97: []byte("B235R 2.3T\x00\x00"),
			0x95: []byte("EU0AF01C.55P"),
			0x94: []byte("5387998"),
			0x92: []byte("123456789012345"),
			0x90: []byte("YS3DD58N512345678"),
			0x91: []byte("5382019"),
			0x98: []byte("txlogger EOL\x00"),
			0x99: []byte("260809"),
		},
	}
	c := newClient(t, e, nil)

	res, err := c.Info(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	row := func(id uint8, desc, value string) model.HeaderResult {
		return model.HeaderResult{Header: model.Header{Desc: desc, ID: id}, Value: value}
	}
	want := []model.HeaderResult{
		row(0x97, "Engine type", "B235R 2.3T"),
		row(0x95, "Software version:", "EU0AF01C.55P"),
		row(0x94, "Software P/N", "5387998"),
		row(0x92, "Immobiilizer code", "123456789012345"),
		row(0x90, "Chassis ID/VIN", "YS3DD58N512345678"),
		row(0x91, "Box Hardware P/N", "5382019"),
		row(0x98, "Tester info", "txlogger EOL"),
		row(0x99, "Software date", "260809"),
		// seed 12 34: 0x1234 + 0x4B and 0x3412 + 0x4B
		row(0, "L3 seed (hi lo)", "12 34"),
		row(0, "XOR (BE seed)", "XOR 127F SUB 2356"),
		row(0, "XOR (byteswapped seed)", "XOR 345D SUB 2356"),
	}
	if !slices.Equal(res, want) {
		t.Fatalf("Info =\n%v\nwant\n%v", res, want)
	}
	if want := []string{"init", "1A 97", "1A 95", "1A 94", "1A 92", "1A 90", "1A 91", "1A 98", "1A 99", "27 03", "82"}; !slices.Equal(e.trace, want) {
		t.Fatalf("trace %q, want %q", e.trace, want)
	}
}

// A header the ECU refuses fails Info, and the session is still closed.
func TestInfoHeaderError(t *testing.T) {
	e := &fakeT7{headers: map[byte][]byte{0x97: []byte("B235R")}}
	c := newClient(t, e, nil)

	_, err := c.Info(testCtx(t))
	if err == nil || !strings.Contains(err.Error(), "ECU info failed") {
		t.Fatalf("err = %v, want an ECU info failure", err)
	}
	if last := e.trace[len(e.trace)-1]; last != "82" {
		t.Fatalf("last request %q, want stopCommunication", last)
	}
	if err := c.PrintECUInfo(testCtx(t)); err == nil {
		t.Fatal("PrintECUInfo hid the Info error")
	}
}

// A refused level-3 seed request does not fail Info; it shows as an "n/a"
// row carrying the reason.
func TestInfoL3SeedRefused(t *testing.T) {
	headers := map[byte][]byte{}
	for _, h := range T7Headers {
		headers[byte(h.ID)] = []byte("x")
	}
	e := &fakeT7{headers: headers, fault: func(sid byte, p []byte) []byte {
		if sid == t7kwp.SECURITY_ACCESS && p[0] == t7kwp.HIGH_PRIORITY {
			return []byte{0x7F, sid, t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR}
		}
		return nil
	}}
	c := newClient(t, e, nil)

	res, err := c.Info(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	last := res[len(res)-1]
	if len(res) != len(T7Headers)+1 || last.Desc != "L3 seed" || !strings.HasPrefix(last.Value, "n/a (") {
		t.Fatalf("last row %+v (of %d), want an n/a L3 seed row", last, len(res))
	}
	if err := c.PrintECUInfo(testCtx(t)); err != nil {
		t.Fatalf("PrintECUInfo: %v", err)
	}
}

// GetHeader wraps a refused 0x1A read.
func TestGetHeaderError(t *testing.T) {
	c := newClient(t, &fakeT7{}, nil)
	ctx := testCtx(t)
	if err := c.DataInitialization(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := c.GetHeader(ctx, 0x90)
	if !errors.Is(err, t7kwp.ErrSubFunctionNotSupportedOrInvalidFormat) || !strings.Contains(err.Error(), "failed getting header") {
		t.Fatalf("err = %v", err)
	}
}

// ReadDTC asks for all stored DTCs (status 0x02 leaves the storage-state
// bits clear), tags them as T7 codes with their raw status, and closes the
// session afterwards.
func TestReadDTC(t *testing.T) {
	e := &fakeT7{dtcs: [][3]byte{{0x01, 0x10, 0x61}, {0x05, 0x00, 0x24}, {0x11, 0x30, 0x42}}}
	c := newClient(t, e, nil)

	got, err := c.ReadDTC(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []dtc.DTC{
		{ECU: symbol.ECU_T7, Code: "P0110", Status: 0x61},
		{ECU: symbol.ECU_T7, Code: "P0500", Status: 0x24},
		{ECU: symbol.ECU_T7, Code: "P1130", Status: 0x42},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ReadDTC = %+v, want %+v", got, want)
	}
	if want := []string{"init", "18 02", "82"}; !slices.Equal(e.trace, want) {
		t.Fatalf("trace %q, want %q", e.trace, want)
	}
	if !c.lastInit.IsZero() {
		t.Fatal("init cache kept after the session was stopped")
	}
}

// An ECU with no stored DTCs yields an empty, non-nil list.
func TestReadDTCNone(t *testing.T) {
	got, err := newClient(t, &fakeT7{}, nil).ReadDTC(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("ReadDTC = %#v, want an empty list", got)
	}
}

// A refused DTC read is returned as an error.
func TestReadDTCError(t *testing.T) {
	e := &fakeT7{fault: func(sid byte, _ []byte) []byte {
		if sid == t7kwp.READ_DTC_BY_STATUS {
			return []byte{0x7F, sid, t7kwp.SERVICE_NOT_SUPPORTED}
		}
		return nil
	}}
	_, err := newClient(t, e, nil).ReadDTC(testCtx(t))
	if !errors.Is(err, t7kwp.ErrServiceNotSupported) {
		t.Fatalf("err = %v, want serviceNotSupported", err)
	}
}

// ResetECU opens a session and sends ECUReset 0x11 0x01; a refusal is an
// error.
func TestResetECU(t *testing.T) {
	e := &fakeT7{}
	if err := newClient(t, e, nil).ResetECU(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"init", "11 01"}; !slices.Equal(e.trace, want) {
		t.Fatalf("trace %q, want %q", e.trace, want)
	}

	e = &fakeT7{fault: func(sid byte, _ []byte) []byte {
		if sid == t7kwp.ECU_RESET {
			return []byte{0x7F, sid, t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR}
		}
		return nil
	}}
	if err := newClient(t, e, nil).ResetECU(testCtx(t)); !errors.Is(err, t7kwp.ErrConditionsNotCorrectOrRequestSequenceError) {
		t.Fatalf("err = %v, want conditionsNotCorrect", err)
	}
}
