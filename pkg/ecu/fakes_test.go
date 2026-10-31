package ecu

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

// Fake ECUs and adapters for driving the package's CAN code without hardware.
// Every fake answers from inside Send, so replies are on the bus before Send
// returns; code that subscribes before sending (Bus.Request, gmlan exchanges,
// t5can.request) sees them.

// openBus opens dev on a fresh bus that is closed when the test ends.
func openBus(t *testing.T, dev gocan.Adapter) *gocan.Bus {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), dev)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	return bus
}

// testCtx is a context bounded well past any single exchange, so a hung
// exchange fails the test instead of the whole run.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// failOpen is an adapter that cannot be opened.
type failOpen struct{}

var errNoAdapter = errors.New("adapter unplugged")

func (failOpen) Open(context.Context, *gocan.Bus) error  { return errNoAdapter }
func (failOpen) Send(context.Context, gocan.Frame) error { return nil }
func (failOpen) Close() error                            { return nil }

// sendFails wraps another adapter and fails every Send of a frame that
// match accepts with errSendFailed.
type sendFails struct {
	gocan.Adapter
	match func(gocan.Frame) bool
}

var errSendFailed = errors.New("adapter write failed")

func (s *sendFails) Send(ctx context.Context, f gocan.Frame) error {
	if s.match(f) {
		return errSendFailed
	}
	return s.Adapter.Send(ctx, f)
}

// hookAdapter wraps another adapter: hook sees every sent frame first and
// answers it itself by returning true, or lets it through to the wrapped one.
type hookAdapter struct {
	gocan.Adapter
	bus  *gocan.Bus
	hook func(bus *gocan.Bus, f gocan.Frame) bool
}

func (h *hookAdapter) Open(ctx context.Context, bus *gocan.Bus) error {
	h.bus = bus
	return h.Adapter.Open(ctx, bus)
}

func (h *hookAdapter) Send(ctx context.Context, f gocan.Frame) error {
	if h.hook(h.bus, f) {
		return nil
	}
	return h.Adapter.Send(ctx, f)
}

// t5Console answers the T5 ASCII console the way the ECU does when it lists
// its symbol table: every C4 character is acknowledged with C6 00, and once
// "S\r" has been typed each C6 00 ack from the tester is answered with the
// next table byte as C6 00 <byte>. The byte is delivered once from a
// goroutine, the way an adapter's reader hands frames to the bus, unless
// sync is set.
type t5Console struct {
	table   []byte
	sync    bool // deliver the table byte inside Send, once
	echo    byte // first byte of the table byte frames; 0 means 0xC6
	status  byte // second byte of the table byte frames
	noReply bool // never answer C4 characters

	mu    sync.Mutex
	bus   *gocan.Bus
	line  []byte
	armed bool
	pos   int
}

func (c *t5Console) Open(_ context.Context, bus *gocan.Bus) error {
	c.bus = bus
	return nil
}

func (c *t5Console) Close() error { return nil }

func (c *t5Console) Send(_ context.Context, f gocan.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f.ID != 0x05 {
		return nil
	}
	switch f.Data[0] {
	case 0xC4:
		if c.noReply {
			return nil
		}
		c.line = append(c.line, f.Data[1])
		if f.Data[1] == '\r' {
			c.armed = string(c.line) == "S\r"
			c.line = c.line[:0]
		}
		c.bus.Deliver(gocan.NewFrame(0x0C, []byte{0xC6, 0x00, 0, 0, 0, 0, 0, 0}))
	case 0xC6:
		if !c.armed || c.pos >= len(c.table) {
			return nil
		}
		echo := c.echo
		if echo == 0 {
			echo = 0xC6
		}
		reply := gocan.NewFrame(0x0C, []byte{echo, c.status, c.table[c.pos], 0, 0, 0, 0, 0})
		c.pos++
		if c.sync {
			c.bus.Deliver(reply)
			return nil
		}
		go c.bus.Deliver(reply)
	}
	return nil
}

// t7SeedKey is the pair the T7 fake checks keys against.
var t7SeedKey = t7kwp.KnownSeedKeys[0]

// t7Fake is a Trionic 7 speaking the KWP2000-over-CAN framing: the session
// start on 0x220/0x238, requests reassembled from 0x240 frames and responses
// chunked on 0x258 with every chunk after the first released by the tester's
// 0x266 ack. Complete requests go to handle; when it returns nil the fake
// answers stopCommunication and securityAccess (seed 0x1234, keys checked
// with t7SeedKey) itself and rejects anything else as serviceNotSupported.
type t7Fake struct {
	silent bool // ignore the session start, as an absent ECU would
	handle func(sid byte, p []byte) []byte

	mu      sync.Mutex
	bus     *gocan.Bus
	req     []byte
	pending []gocan.Frame
	sids    []byte // services requested, in order
}

func (e *t7Fake) Open(_ context.Context, bus *gocan.Bus) error {
	e.bus = bus
	return nil
}

func (e *t7Fake) Close() error { return nil }

func (e *t7Fake) Send(_ context.Context, f gocan.Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch f.ID {
	case t7kwp.INIT_MSG_ID:
		if !e.silent && f.Data[1] == t7kwp.START_COM_REQ {
			e.bus.Deliver(gocan.NewFrame(t7kwp.INIT_RESP_ID, []byte{0xC0, 0xA1, 0x03, 0xC1, 0xEA, 0x8F, 0x02, 0x58}))
		}
	case t7kwp.REQ_MSG_ID:
		if f.Data[0]&0x40 != 0 {
			e.req = e.req[:0]
		}
		e.req = append(e.req, f.Data[2:8]...)
		if f.Data[0]&0x80 != 0 {
			e.bus.Deliver(gocan.NewFrame(t7kwp.REQ_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, f.Data[0] & 0xBF, 0, 0, 0, 0}))
		}
		if f.Data[0]&0x3F == 0 && len(e.req) > 0 && len(e.req) > int(e.req[0]) {
			msg := append([]byte(nil), e.req[1:1+e.req[0]]...)
			e.req = e.req[:0]
			e.pending = nil
			e.sids = append(e.sids, msg[0])
			e.respond(e.answer(msg[0], msg[1:]))
		}
	case t7kwp.RESP_CHUNK_CONF_ID:
		if len(e.pending) > 0 {
			next := e.pending[0]
			e.pending = e.pending[1:]
			e.bus.Deliver(next)
		}
	}
	return nil
}

func (e *t7Fake) answer(sid byte, p []byte) []byte {
	if e.handle != nil {
		if r := e.handle(sid, p); r != nil {
			return r
		}
	}
	switch sid {
	case t7kwp.STOP_COM_REQ:
		return []byte{sid | 0x40}
	case t7kwp.SECURITY_ACCESS:
		const seed = 0x1234
		if p[0]%2 == 1 {
			return []byte{0x67, p[0], seed >> 8, seed & 0xFF}
		}
		want := (uint16(seed)<<2 ^ t7SeedKey.XOR) - t7SeedKey.Sub
		if uint16(p[1])<<8|uint16(p[2]) != want {
			return []byte{0x7F, sid, t7kwp.INVALID_KEY}
		}
		return []byte{0x67, p[0], 0x34}
	}
	return []byte{0x7F, sid, t7kwp.SERVICE_NOT_SUPPORTED}
}

// respond chunks payload as the T7 does: length byte plus five payload bytes
// in the first frame, six in each following one, byte 0 counting down the
// frames still to come with 0x40 on the first and 0x80 on all.
func (e *t7Fake) respond(payload []byte) {
	msg := append([]byte{byte(len(payload))}, payload...)
	n := (len(msg) + 5) / 6
	for i := range n {
		chunk := make([]byte, 8)
		chunk[0] = 0x80 | byte(n-1-i)
		if i == 0 {
			chunk[0] |= 0x40
		}
		chunk[1] = 0xA1
		copy(chunk[2:], msg[i*6:min(i*6+6, len(msg))])
		e.pending = append(e.pending, gocan.NewFrame(0x258, chunk))
	}
	first := e.pending[0]
	e.pending = e.pending[1:]
	e.bus.Deliver(first)
}

func (e *t7Fake) services() []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]byte(nil), e.sids...)
}

// t8Fake is a Trionic 8 speaking GMLAN over ISO-TP on 0x7E0/0x7E8, plus
// functional requests on 0x7DF and the no-response tester present on 0x101.
// Requests go to handle; when it returns nil the fake answers
// initiateDiagnosticOperation, returnToNormalMode, testerPresent and
// securityAccess (seed 0x4321, keys checked with t8sec) itself and rejects
// anything else as serviceNotSupported. uudt frames are delivered on 0x5E8
// right after the answer to a readDiagnosticInformation request.
type t8Fake struct {
	handle func(req []byte) []byte
	uudt   [][]byte

	mu            sync.Mutex
	bus           *gocan.Bus
	pending       []gocan.Frame
	reqs          [][]byte // requests received, in order
	reqIDs        []uint32 // CAN id each request arrived on
	testerPresent int      // frames on 0x101
}

func (e *t8Fake) Open(_ context.Context, bus *gocan.Bus) error {
	e.bus = bus
	return nil
}

func (e *t8Fake) Close() error { return nil }

func (e *t8Fake) Send(_ context.Context, f gocan.Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	d := f.Data
	switch {
	case f.ID == 0x101:
		e.testerPresent++
	case f.ID == 0x7DF || f.ID == 0x7E0 && d[0]>>4 == 0: // single frame request
		req := append([]byte(nil), d[1:1+int(d[0])]...)
		e.reqs = append(e.reqs, req)
		e.reqIDs = append(e.reqIDs, f.ID)
		e.reply(e.answer(req))
		if req[0] == 0xA9 {
			for _, u := range e.uudt {
				e.bus.Deliver(gocan.NewFrame(0x5E8, u))
			}
		}
	case f.ID == 0x7E0 && d[0]>>4 == 3: // flow control: release consecutive frames
		for _, cf := range e.pending {
			e.bus.Deliver(cf)
		}
		e.pending = nil
	}
	return nil
}

func (e *t8Fake) answer(req []byte) []byte {
	if e.handle != nil {
		if r := e.handle(req); r != nil {
			return r
		}
	}
	sid := req[0]
	switch sid {
	case 0x10, 0x20, 0x3E:
		return []byte{sid | 0x40}
	case 0x27:
		const seed = 0x4321
		level := req[1]
		if level%2 == 1 {
			return []byte{0x67, level, seed >> 8, seed & 0xFF}
		}
		hi, lo := t8sec.CalculateAccessKey([]byte{seed >> 8, seed & 0xFF}, level-1)
		if req[2] != hi || req[3] != lo {
			return []byte{0x7F, sid, 0x35}
		}
		return []byte{0x67, level}
	}
	return []byte{0x7F, sid, 0x11}
}

// reply frames payload as ISO-TP: a single frame when it fits, otherwise a
// first frame now and consecutive frames once the tester sends flow control.
func (e *t8Fake) reply(payload []byte) {
	if len(payload) <= 7 {
		e.bus.Deliver(gocan.NewFrame(0x7E8, pad8(append([]byte{byte(len(payload))}, payload...))))
		return
	}
	e.pending = nil
	seq := byte(0x21)
	for pos := 6; pos < len(payload); pos += 7 {
		e.pending = append(e.pending, gocan.NewFrame(0x7E8, pad8(append([]byte{seq}, payload[pos:min(pos+7, len(payload))]...))))
		seq = 0x20 | (seq+1)&0x0F
	}
	e.bus.Deliver(gocan.NewFrame(0x7E8, append([]byte{0x10 | byte(len(payload)>>8), byte(len(payload))}, payload[:6]...)))
}

// pad8 zero-pads b to the 8 bytes a T8 always sends.
func pad8(b []byte) []byte {
	out := make([]byte, 8)
	copy(out, b)
	return out
}

// requestIDs returns the CAN id of each request received, in order.
func (e *t8Fake) requestIDs() []uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]uint32(nil), e.reqIDs...)
}

// requests returns the service ids received, in order.
func (e *t8Fake) requests() []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]byte, len(e.reqs))
	for i, r := range e.reqs {
		out[i] = r[0]
	}
	return out
}
