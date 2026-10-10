package datalogger

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecusim"
	"github.com/roffe/txlogger/pkg/wbl/aem"
)

// newTestTxbridge is a TxBridge wired to fake without running Start.
func newTestTxbridge(t *testing.T, cfg Config, fake *fakeTxbridge) *TxBridge {
	c, _ := NewTxbridge(cfg, nopWriter{})
	c.tb = fake
	t.Cleanup(c.secondTicker.Stop)
	return c
}

func raw(s string) tbCall          { return tbCall{raw: true, data: []byte(s)} }
func cmd(c byte, d ...byte) tbCall { return tbCall{cmd: c, data: d} }
func sameCall(a, b tbCall) bool {
	return a.raw == b.raw && a.cmd == b.cmd && bytes.Equal(a.data, b.data)
}
func sameCalls(a, b []tbCall) bool  { return slices.EqualFunc(a, b, sameCall) }
func callsString(c []tbCall) string { return fmt.Sprintf("%+v", c) }

// frame builds a txbridge log frame: little-endian ms timestamp then payload.
func frame(ts uint32, b ...[]byte) []byte {
	out := binary.LittleEndian.AppendUint32(nil, ts)
	for _, p := range b {
		out = append(out, p...)
	}
	return out
}

// setECU selects the ECU with its id character, then overrides the dongle's
// read delay with 1000/Rate ms clamped to 1..255; no rate keeps the default.
func TestTxbridgeSetECU(t *testing.T) {
	tests := []struct {
		rate int
		want []tbCall
	}{
		{50, []tbCall{raw("7"), cmd('D', 20)}},
		{2000, []tbCall{raw("7"), cmd('D', 1)}},
		{1, []tbCall{raw("7"), cmd('D', 255)}},
		{0, []tbCall{raw("7")}},
	}
	for _, tt := range tests {
		fake := newFakeTxbridge(nil)
		c := newTestTxbridge(t, Config{Rate: tt.rate}, fake)
		if err := c.setECU("7"); err != nil {
			t.Fatal(err)
		}
		if got := fake.callList(); !sameCalls(got, tt.want) {
			t.Errorf("rate %d: calls %s, want %s", tt.rate, callsString(got), callsString(tt.want))
		}
	}
	fake := newFakeTxbridge(nil)
	fake.cmdErr = errors.New("link down")
	if err := newTestTxbridge(t, Config{Rate: 25}, fake).setECU("8"); !errors.Is(err, fake.cmdErr) {
		t.Fatalf("setECU = %v", err)
	}
}

// The broadcast collect command lists 11-bit ids little-endian; an empty list
// clears collection.
func TestSendBroadcastCollect(t *testing.T) {
	fake := newFakeTxbridge(nil)
	if err := sendBroadcastCollect(fake, t7BroadcastIDs); err != nil {
		t.Fatal(err)
	}
	if err := sendBroadcastCollect(fake, nil); err != nil {
		t.Fatal(err)
	}
	want := []tbCall{cmd('b', 0xA0, 0x01, 0x80, 0x02, 0xA0, 0x03), cmd('b')}
	if got := fake.callList(); !sameCalls(got, want) {
		t.Fatalf("calls %s", callsString(got))
	}
}

// configureT5Symbols sends 'd' with a little-endian {offset u32, length u16}
// per symbol and returns the payload size the log frames will carry.
func TestConfigureT5Symbols(t *testing.T) {
	fake := newFakeTxbridge(nil)
	msgs := newMsgLog(t)
	c := newTestTxbridge(t, Config{OnMessage: msgs.add, Symbols: []*symbol.Symbol{
		{Name: "Rpm", SramOffset: 0x1000, Length: 2},
		{Name: "X", SramOffset: 0x12345, Length: 300},
	}}, fake)
	n, err := c.configureT5Symbols()
	if err != nil || n != 302 {
		t.Fatalf("configureT5Symbols = %d %v", n, err)
	}
	want := cmd('d', 0x00, 0x10, 0, 0, 0x02, 0x00, 0x45, 0x23, 0x01, 0x00, 0x2C, 0x01)
	if got := fake.callList(); !sameCalls(got, []tbCall{want}) || !msgs.has("Symbol list configured") {
		t.Fatalf("calls %s, messages %v", callsString(got), msgs.all())
	}
	fake.cmdErr = errors.New("link down")
	if n, err := c.configureT5Symbols(); n != -1 || !errors.Is(err, fake.cmdErr) {
		t.Fatalf("configureT5Symbols = %d %v", n, err)
	}
}

// readFrameHeader strips the dongle's little-endian ms timestamp and places
// each frame relative to the first one's arrival time by the dongle clock.
func TestReadFrameHeader(t *testing.T) {
	c := newTestTxbridge(t, Config{}, newFakeTxbridge(nil))
	before := time.Now()
	r, t1, err := c.readFrameHeader(frame(10_000, []byte{0xAB}))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := r.ReadByte(); b != 0xAB || r.Len() != 0 {
		t.Fatal("reader not positioned after the timestamp")
	}
	if t1.Before(before) || t1.After(time.Now()) {
		t.Fatalf("first frame at %v", t1)
	}
	_, t2, err := c.readFrameHeader(frame(10_140))
	if err != nil {
		t.Fatal(err)
	}
	if d := t2.Sub(t1); d != 140*time.Millisecond {
		t.Fatalf("second frame %v after the first, want 140ms", d)
	}
	if _, _, err := c.readFrameHeader([]byte{1, 2}); err == nil {
		t.Fatal("short frame accepted")
	}
}

// perSecond reports the frame rate and aborts on more than five errors in
// the last second or five seconds without data; otherwise it resets the
// per-second counters.
func TestTxbridgePerSecond(t *testing.T) {
	var fps []int
	c := newTestTxbridge(t, Config{FpsCounter: func(n int) { fps = append(fps, n) }}, newFakeTxbridge(nil))
	c.capturePerSecond = 40
	c.errPerSecond.Store(5)
	if msg := c.perSecond(time.Now()); msg != "" || c.capturePerSecond != 0 || c.errPerSecond.Load() != 0 {
		t.Fatalf("healthy second: %q, counters %d/%d", msg, c.capturePerSecond, c.errPerSecond.Load())
	}
	c.errPerSecond.Store(6)
	if msg := c.perSecond(time.Now()); msg != "too many errors, aborting logging" {
		t.Fatalf("error second: %q", msg)
	}
	c.errPerSecond.Store(0)
	if msg := c.perSecond(time.Now().Add(-dataTimeout - time.Second)); msg != "no data for 5s, aborting logging" {
		t.Fatalf("stale second: %q", msg)
	}
	if !slices.Equal(fps, []int{40, 0, 0}) {
		t.Fatalf("fps %v", fps)
	}
}

// RAM reads go out as 'R' <addr u32 LE> <len> in chunks of at most the
// given size, requeued until the request is complete.
func TestHandleReadTxbridge(t *testing.T) {
	fake := newFakeTxbridge(nil)
	data := make([]byte, 500)
	for i := range data {
		data[i] = byte(i)
	}
	fake.poke(0x1000, data)
	c := newTestTxbridge(t, Config{}, fake)
	req := NewReadDataRequest(0x1000, 500)
	for r := req; r != nil; {
		c.handleReadTxbridge(context.Background(), r, 234, time.Second)
		select {
		case r = <-c.readChan:
		default:
			r = nil
		}
	}
	if err := req.Wait(); err != nil || !bytes.Equal(req.Data, data) {
		t.Fatalf("read %d bytes, %v", len(req.Data), err)
	}
	want := []tbCall{
		cmd('R', 0x00, 0x10, 0, 0, 234),
		cmd('R', 0xEA, 0x10, 0, 0, 234),
		cmd('R', 0xD4, 0x11, 0, 0, 32),
	}
	if got := fake.reqList(); !sameCalls(got, want) {
		t.Fatalf("requests %s", callsString(got))
	}

	fake.reqErr = errors.New("timeout")
	failed := NewReadDataRequest(0x2000, 4)
	c.handleReadTxbridge(context.Background(), failed, 234, time.Second)
	if err := failed.Wait(); !errors.Is(err, fake.reqErr) {
		t.Fatalf("failed read: %v", err)
	}
}

// RAM writes go out as 'W' <addr u32 LE> <len> <data> in chunks; an 'e'
// answer fails the request.
func TestHandleWriteTxbridge(t *testing.T) {
	fake := newFakeTxbridge(nil)
	c := newTestTxbridge(t, Config{}, fake)
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(255 - i)
	}
	req := NewWriteDataRequest(0x3000, data)
	for r := req; r != nil; {
		c.handleWriteTxbridge(context.Background(), r, 128, time.Second)
		select {
		case r = <-c.writeChan:
		default:
			r = nil
		}
	}
	if err := req.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := fake.peek(0x3000, 300); !bytes.Equal(got, data) {
		t.Fatal("dongle RAM does not hold the written data")
	}
	reqs := fake.reqList()
	if len(reqs) != 3 {
		t.Fatalf("%d requests, want 3", len(reqs))
	}
	for i, want := range []struct {
		addr uint32
		n    int
	}{{0x3000, 128}, {0x3080, 128}, {0x3100, 44}} {
		r := reqs[i]
		if r.cmd != 'W' || binary.LittleEndian.Uint32(r.data) != want.addr || int(r.data[4]) != want.n || len(r.data) != 5+want.n {
			t.Fatalf("request %d: %c % X", i, r.cmd, r.data[:5])
		}
	}

	fake.writeErr = true
	rejected := NewWriteDataRequest(0x3000, []byte{1})
	c.handleWriteTxbridge(context.Background(), rejected, 128, time.Second)
	if err := rejected.Wait(); err == nil || err.Error() != "error response: 01" {
		t.Fatalf("rejected write: %v", err)
	}
	fake.writeErr, fake.reqErr = false, errors.New("timeout")
	failed := NewWriteDataRequest(0x3000, []byte{1})
	c.handleWriteTxbridge(context.Background(), failed, 128, time.Second)
	if err := failed.Wait(); !errors.Is(err, fake.reqErr) {
		t.Fatalf("failed write: %v", err)
	}
}

// A device named like a txbridge that lacks the dongle command surface, or
// an ECU the dongle cannot log, fails Start.
func TestTxbridgeStartFailures(t *testing.T) {
	notTxbridge := newHook(&gocan.Loopback{}, "txbridge wifi", nil)
	cl, _, err := New(testCfg(t, "T7", notTxbridge, nil, newMsgLog(t)))
	if err != nil {
		t.Fatal(err)
	}
	if err := runLogger(t, cl).wait(t, 5*time.Second); err == nil || !strings.Contains(err.Error(), "needs the txbridge adapter") {
		t.Fatalf("Start = %v", err)
	}

	c, _ := NewTxbridge(testCfg(t, "T9", newFakeTxbridge(nil), nil, newMsgLog(t)), newRecWriter())
	if err := runLogger(t, c).wait(t, 5*time.Second); err == nil || err.Error() != "unknown ECU type: T9" {
		t.Fatalf("Start = %v", err)
	}
}

// startTxbridge starts a txbridge logger and waits for it to start the
// dongle's log stream.
func startTxbridge(t *testing.T, cfg Config, fake *fakeTxbridge, rec *recWriter) *session {
	t.Helper()
	c, _ := NewTxbridge(cfg, rec)
	s := runLogger(t, c)
	select {
	case <-fake.started:
	case <-s.finished:
		t.Fatalf("Start returned %v before logging", s.err)
	case <-time.After(10 * time.Second):
		t.Fatal("logging never started")
	}
	return s
}

// Txbridge T5: the logger selects the ECU and rate, sends the symbol list,
// starts the stream and decodes each frame's timestamp and symbol bytes into
// a log row; wrong-size frames are counted and skipped, RAM access goes
// through 'R'/'W', and Close stops the stream.
func TestTxbridgeT5Logging(t *testing.T) {
	fake := newFakeTxbridge(nil)
	msgs := newMsgLog(t)
	rec := newRecWriter()
	s := startTxbridge(t, testCfg(t, "T5", fake, t5Symbols(), msgs), fake, rec)

	want := []tbCall{raw("5"), cmd('D', 20), cmd('d', 0x00, 0x10, 0, 0, 2, 0, 0x10, 0x10, 0, 0, 1, 0), raw("r")}
	if got := fake.callList(); !sameCalls(got, want) {
		t.Fatalf("setup calls %s", callsString(got))
	}

	fake.push('r', frame(5000, []byte{0x01, 0x2C}, []byte{123}))
	fake.push('r', frame(5001, []byte{0x01}))
	fake.push('r', frame(5040, []byte{0x00, 0x96}, []byte{140}))
	rows := rec.waitRows(t, 2, 5*time.Second)
	if rows[0].text["Rpm"] != "3000" || rows[0].text["Batt_volt"] != "12.30" || rows[1].text["Rpm"] != "1500" {
		t.Fatalf("rows %v / %v", rows[0].text, rows[1].text)
	}
	if d := rows[1].ts.Sub(rows[0].ts); d != 40*time.Millisecond {
		t.Fatalf("rows %v apart, want 40ms", d)
	}
	if !msgs.has("expected 7 bytes, got 5") || msgs.errs.Load() != 1 {
		t.Fatalf("errors %d, messages %v", msgs.errs.Load(), msgs.all())
	}

	fake.poke(0x2000, []byte{9, 8, 7})
	if got, err := s.cl.GetRAM(0x2000, 3); err != nil || !bytes.Equal(got, []byte{9, 8, 7}) {
		t.Fatalf("GetRAM = % X %v", got, err)
	}
	if err := s.cl.SetRAM(0x2100, []byte{1, 2}); err != nil || !bytes.Equal(fake.peek(0x2100, 2), []byte{1, 2}) {
		t.Fatalf("SetRAM: %v", err)
	}

	s.cl.Close()
	if err := s.wait(t, 5*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	waitClosed(t, fake.stopped, time.Second, "stop command")
	waitClosed(t, rec.closed, time.Second, "log writer close")
}

// A txbridge T5 session ends when the stream subscription closes or the log
// cannot be written.
func TestTxbridgeT5SessionEnds(t *testing.T) {
	t.Run("stream closed", func(t *testing.T) {
		fake := newFakeTxbridge(nil)
		msgs := newMsgLog(t)
		s := startTxbridge(t, testCfg(t, "T5", fake, t5Symbols(), msgs), fake, newRecWriter())
		fake.closeSubs()
		if err := s.wait(t, 5*time.Second); err != nil {
			t.Fatalf("Start = %v", err)
		}
		if !msgs.has("txbridge sub closed") {
			t.Fatalf("messages %v", msgs.all())
		}
	})
	t.Run("log write error", func(t *testing.T) {
		fake := newFakeTxbridge(nil)
		msgs := newMsgLog(t)
		rec := newRecWriter()
		rec.err = errors.New("disk full")
		s := startTxbridge(t, testCfg(t, "T5", fake, t5Symbols(), msgs), fake, rec)
		fake.push('r', frame(1, []byte{0, 1}, []byte{2}))
		if err := s.wait(t, 5*time.Second); err != nil {
			t.Fatalf("Start = %v", err)
		}
		if !msgs.has("failed to write log: disk full") {
			t.Fatalf("messages %v", msgs.all())
		}
	})
}

// With the txbridge wideband port the dongle is told to read the wideband
// ('w' + type) and its readings fill the Lambda.External column.
func TestTxbridgeWidebandColumn(t *testing.T) {
	fake := newFakeTxbridge(nil)
	cfg := testCfg(t, "T5", fake, t5Symbols(), newMsgLog(t))
	cfg.WidebandConfig = WidebandConfig{Name: aem.ProductString, Port: "txbridge"}
	rec := newRecWriter()
	startTxbridge(t, cfg, fake, rec)
	if got := fake.callList(); !sameCall(got[0], cmd('w', 'a')) {
		t.Fatalf("first call %+v, want the wideband setup", got[0])
	}
	fake.push('w', []byte("9.87")) // lambda*10 as text
	deadline := time.Now().Add(5 * time.Second)
	for ts := uint32(1); ; ts++ {
		n := len(rec.snapshot())
		fake.push('r', frame(ts, []byte{0, 1}, []byte{2}))
		row := rec.waitRows(t, n+1, 5*time.Second)[n]
		if math.Abs(row.vals[EXTERNALWBLSYM]-0.987) < 1e-9 {
			if row.text[EXTERNALWBLSYM] != "0.987" {
				t.Fatalf("formatted as %q", row.text[EXTERNALWBLSYM])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Lambda.External = %v", row.vals[EXTERNALWBLSYM])
		}
	}
}

func manyT5Symbols(n int) []*symbol.Symbol {
	out := make([]*symbol.Symbol, n)
	for i := range out {
		out[i] = &symbol.Symbol{Name: fmt.Sprintf("S%d", i), SramOffset: uint32(0x1000 + i), Length: 1}
	}
	return out
}

// In gather mode the txbridge logger uploads the gather image into ECU SRAM
// over CAN (re-arming at each block) before enabling gather on the dongle
// and starting the stream.
func TestTxbridgeT5Gather(t *testing.T) {
	sim := &ecusim.T5{}
	fake := newFakeTxbridge(sim)
	syms := manyT5Symbols(30) // 186 byte image: two upload blocks
	msgs := newMsgLog(t)
	cfg := testCfg(t, "T5", fake, syms, msgs)
	cfg.ExperimentalT5FastLogging = true
	startTxbridge(t, cfg, fake, newRecWriter())

	image, _ := buildT5GatherImage(syms)
	if got := sim.Peek(t5GatherStubAddr, len(image)); !bytes.Equal(got, image) {
		t.Fatalf("SRAM\n% X\nwant\n% X", got, image)
	}
	calls := fake.callList()
	tail := calls[len(calls)-2:]
	if !sameCalls(tail, []tbCall{raw("g"), raw("r")}) || calls[len(calls)-3].cmd != 'd' {
		t.Fatalf("calls %s", callsString(calls))
	}
	if !msgs.has("T5 gather enabled (30 symbols, 186 byte image)") {
		t.Fatalf("messages %v", msgs.all())
	}
}

// A rejected arm (engine running) fails the upload at once and is never
// re-sent; a lost acknowledgement is retried.
func TestUploadT5SRAMRetries(t *testing.T) {
	image := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	t.Run("rejected arm not retried", func(t *testing.T) {
		var log frameLog
		bus := openBus(t, newHook(&ecusim.T5{}, "", func(b *gocan.Bus, f gocan.Frame) bool {
			log.add(f)
			if f.ID == 0x05 && f.Data[0] == 0xA5 {
				b.Deliver(gocan.NewFrame(0x0C, []byte{0xA5, 0x01, 0, 0, 0, 0, 0, 0}))
				return true
			}
			return false
		}))
		c := newTestTxbridge(t, Config{}, newFakeTxbridge(nil))
		err := c.uploadT5SRAM(context.Background(), bus, 0x7700, image)
		if err == nil || !strings.Contains(err.Error(), "engine must be off to arm") {
			t.Fatalf("upload = %v", err)
		}
		if n := len(log.withFirstByte(0x05, 0xA5)); n != 1 {
			t.Fatalf("arm sent %d times", n)
		}
	})
	t.Run("lost ack retried", func(t *testing.T) {
		sim := &ecusim.T5{}
		var log frameLog
		dropped := false
		bus := openBus(t, newHook(sim, "", func(_ *gocan.Bus, f gocan.Frame) bool {
			log.add(f)
			if f.ID == 0x05 && f.Data[0] == 0xA5 && !dropped {
				dropped = true // first arm vanishes
				return true
			}
			return false
		}))
		c := newTestTxbridge(t, Config{}, newFakeTxbridge(nil))
		if err := c.uploadT5SRAM(context.Background(), bus, 0x7700, image); err != nil {
			t.Fatal(err)
		}
		if n := len(log.withFirstByte(0x05, 0xA5)); n != 2 {
			t.Fatalf("arm sent %d times, want 2", n)
		}
		if got := sim.Peek(0x7700, len(image)); !bytes.Equal(got, image) {
			t.Fatalf("SRAM % X", got)
		}
	})
}

func t7TxbridgeSymbols() []*symbol.Symbol {
	return []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: 1, Length: 2, Correctionfactor: 1},
		{Name: "MAF.m_AirInlet", Number: 100, Length: 2, Correctionfactor: 1},
		{Name: "IgnKnk.fi_Offset", Number: 200, Length: 8},
	}
}

var (
	trailer1A0 = []byte{0x01, 0xA0, 8, 0, 0x05, 0xDC, 0, 0, 33, 0, 0} // 1500 rpm, pedal 33
	trailer3A0 = []byte{0x03, 0xA0, 8, 0, 0, 0, 0x01, 0xF4, 0, 0, 0}  // 50 km/h
)

// Txbridge T7: the dongle is told to fold the broadcast frames into the
// stream, broadcast symbols are taken from that trailer instead of the KWP
// read, the payload is decoded (with the per-cylinder ignition split) and a
// malformed trailer or short frame is reported without ending the session.
func TestTxbridgeT7Logging(t *testing.T) {
	fake := newFakeTxbridge(&ecusim.T7{})
	syms := t7TxbridgeSymbols()
	msgs := newMsgLog(t)
	rec := newRecWriter()
	rpm, cyl1 := collectEbus(t, "ActualIn.n_Engine"), collectEbus(t, "IgnKnk.fi_Offset.Cyl1")
	s := startTxbridge(t, testCfg(t, "T7", fake, syms, msgs), fake, rec)

	want := []tbCall{raw("7"), cmd('D', 20), cmd('b', 0xA0, 0x01, 0x80, 0x02, 0xA0, 0x03), raw("r")}
	if got := fake.callList(); !sameCalls(got, want) {
		t.Fatalf("setup calls %s", callsString(got))
	}
	if syms[0].Number != -1 || syms[1].Number != 100 {
		t.Fatal("broadcast symbol still polled")
	}

	payload := [][]byte{{0x12, 0x34}, {0x00, 0x0A, 0, 0, 0, 0, 0, 0}}
	fake.push('r', frame(100, append(payload, trailer1A0, trailer3A0)...))
	fake.push('r', frame(101, []byte{1}))
	fake.push('r', frame(102, append(payload, []byte{0x01, 0xA0, 9, 0})...))
	rows := rec.waitRows(t, 2, 5*time.Second)
	if rows[0].text["MAF.m_AirInlet"] != "4660" || rows[1].text["MAF.m_AirInlet"] != "4660" {
		t.Fatalf("rows %v / %v", rows[0].text, rows[1].text)
	}
	rpm.waitValue(t, 1500, time.Second)
	if got, _ := cyl1.last(); got != 1 {
		t.Fatalf("cylinder 1 offset %v", got)
	}
	if !msgs.has("expected at least 14 bytes, got 5") || !msgs.has("malformed broadcast trailer") {
		t.Fatalf("messages %v", msgs.all())
	}
	fake.poke(0xF00010, []byte{3, 4})
	if got, err := s.cl.GetRAM(0xF00010, 2); err != nil || !bytes.Equal(got, []byte{3, 4}) {
		t.Fatalf("GetRAM = % X %v", got, err)
	}
	if err := s.cl.SetRAM(0xF00020, []byte{9}); err != nil || fake.peek(0xF00020, 1)[0] != 9 {
		t.Fatalf("SetRAM: %v", err)
	}

	s.cl.Close()
	if err := s.wait(t, 5*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	waitClosed(t, fake.stopped, time.Second, "stop command")
}

// Broadcast-sourced symbols the user selected are still log columns on the
// txbridge path, filled from the folded trailer (as the generic T7 logger
// logs them from its broadcast sysvars).
func TestTxbridgeT7LogsBroadcastSymbols(t *testing.T) {
	fake := newFakeTxbridge(&ecusim.T7{})
	rec := newRecWriter()
	startTxbridge(t, testCfg(t, "T7", fake, t7TxbridgeSymbols(), newMsgLog(t)), fake, rec)
	payload := [][]byte{{0x12, 0x34}, {0x00, 0x0A, 0, 0, 0, 0, 0, 0}}
	fake.push('r', frame(100, append(payload, trailer1A0)...))
	fake.push('r', frame(140, append(payload, trailer1A0)...))
	row := rec.waitRows(t, 2, 5*time.Second)[1]
	if got, ok := row.text["ActualIn.n_Engine"]; !ok || got != "1500" {
		t.Fatalf("ActualIn.n_Engine column %q (present %v), columns %v", got, ok, row.names)
	}
}

// Txbridge T8: after the GMLAN init over CAN the logger starts the stream,
// decodes each frame into a row (feeding the AD scanner) and stops the
// stream on Close.
func TestTxbridgeT8Logging(t *testing.T) {
	fake := newFakeTxbridge(&ecusim.T8{})
	syms := []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: 100, Length: 2, Correctionfactor: 1},
		{Name: "AD_sond", Number: 7, Length: 2, Correctionfactor: 1},
	}
	msgs := newMsgLog(t)
	cfg := testCfg(t, "T8", fake, syms, msgs)
	cfg.WidebandConfig = WidebandConfig{Name: "ECU", ADScanner: true, ADScannerSymbol: "AD_sond", SupportPoints: []int{0, 1000}, LambdaValues: []float64{0.5, 1.5}}
	rec := newRecWriter()
	s := startTxbridge(t, cfg, fake, rec)
	if got := fake.callList(); !sameCalls(got, []tbCall{raw("8"), cmd('D', 20), raw("r")}) {
		t.Fatalf("setup calls %s", callsString(got))
	}
	fake.push('r', frame(7, []byte{0x03, 0x20}, []byte{0x01, 0xF4}))
	row := rec.waitRows(t, 1, 5*time.Second)[0]
	if row.text["ActualIn.n_Engine"] != "800" || row.vals[LAMBDAADSCANNER] != 1 {
		t.Fatalf("row %v", row.text)
	}
	fake.poke(0x100010, []byte{5, 6})
	if got, err := s.cl.GetRAM(0x100010, 2); err != nil || !bytes.Equal(got, []byte{5, 6}) {
		t.Fatalf("GetRAM = % X %v", got, err)
	}
	if err := s.cl.SetRAM(0x100020, []byte{7}); err != nil || fake.peek(0x100020, 1)[0] != 7 {
		t.Fatalf("SetRAM: %v", err)
	}
	s.cl.Close()
	if err := s.wait(t, 5*time.Second); err != nil {
		t.Fatalf("Start = %v", err)
	}
	waitClosed(t, fake.stopped, time.Second, "stop command")
}

// One wrong-size frame is a bad sample on txbridge T8 too: it is counted and
// skipped and logging carries on, as on the generic T8 logger and the
// txbridge T5/T7 loggers.
func TestTxbridgeT8SkipsBadFrame(t *testing.T) {
	fake := newFakeTxbridge(&ecusim.T8{})
	msgs := newMsgLog(t)
	rec := newRecWriter()
	logged := make(chan struct{})
	rec.onWrite = func(n int) {
		if n == 1 {
			close(logged)
		}
	}
	s := startTxbridge(t, testCfg(t, "T8", fake, []*symbol.Symbol{{Name: "ActualIn.n_Engine", Number: 100, Length: 2, Correctionfactor: 1}}, msgs), fake, rec)
	fake.push('r', frame(1, []byte{1}))
	fake.push('r', frame(2, []byte{0x03, 0x20}))
	select {
	case <-logged:
	case <-s.finished:
		t.Fatalf("session ended on a bad frame; messages %v", msgs.all())
	case <-time.After(5 * time.Second):
		t.Fatal("good frame after the bad one was not logged")
	}
	if got := rec.snapshot()[0].text["ActualIn.n_Engine"]; got != "800" {
		t.Fatalf("logged %q, want the good frame's 800", got)
	}
	if msgs.errs.Load() != 1 {
		t.Fatalf("errors %d, want 1", msgs.errs.Load())
	}
}
