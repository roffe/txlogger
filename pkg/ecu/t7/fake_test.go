package t7

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/ecu"
)

// fakeT7 is a Trionic 7 behind a virtual adapter, speaking the KWP2000-over-CAN
// framing the client uses: startCommunication on 0x220/0x238, requests
// reassembled from 0x240 frames, responses chunked on 0x258 with every chunk
// after the first released by the tester's 0x266 ack, and the fast-transfer
// firmware extension on 0x7C0/0x7C1. Like the real ECU it ignores a second
// startCommunication while a session is live, ignores requests without a
// session, and only erases after EOLProgrammingStart under security access.
// Flash writes AND into the old contents, as programming can only clear bits.
//
// It answers from inside Send, so replies are on the bus before Send returns.
type fakeT7 struct {
	seedKey    t7kwp.SeedKey   // pair keys are checked against; zero = t7kwp.KnownSeedKeys[0]
	seed       uint16          // seed handed out; zero = 0x1234
	phantom    bool            // start with a live session left by another tester
	silent     bool            // never answer startCommunication
	fastBlock  int             // > 0: the fast transfer extension is present with this block size
	headers    map[byte][]byte // readECUIdentification answers
	dtcs       [][3]byte       // readDTCByStatus answers: code hi, code lo, status
	mem        []byte          // what memory reads return
	eolResult  byte            // requestRoutineResults status byte
	brokenFast func(addr int) bool

	// fault sees every complete request first; a non-nil reply replaces the
	// normal answer, and noReply makes the ECU stay silent.
	fault func(sid byte, p []byte) []byte

	mu       sync.Mutex
	bus      *gocan.Bus
	req      []byte
	pending  []gocan.Frame
	session  bool
	unlocked bool
	eol      bool
	erased   bool
	flash    []byte
	download bool // requestDownload accepted, transferData may follow
	writePtr int
	readAddr int
	readLen  int

	// fast download state
	armed     bool
	fastAdr   int
	fastBuf   []byte
	fastCount int

	trace      []string        // requests in arrival order, see label
	keys       []uint16        // keys received with 0x27
	downloads  []span          // requestDownload address and length
	blocks     []span          // transferData blocks as written
	reads      []span          // 0x2C F0 03 read setups
	fastUps    []int           // fast upload start addresses
	fastBlocks []span          // committed fast download blocks
	piWrites   map[byte][]byte // writeDataByLocalIdentifier
}

type span struct{ addr, n int }

// noReply is a fault reply that leaves a request unanswered.
var noReply = []byte{}

func (e *fakeT7) Open(_ context.Context, bus *gocan.Bus) error {
	e.bus = bus
	if e.seedKey == (t7kwp.SeedKey{}) {
		e.seedKey = t7kwp.KnownSeedKeys[0]
	}
	if e.seed == 0 {
		e.seed = 0x1234
	}
	e.session = e.phantom
	e.flash = make([]byte, 0x80000) // the old firmware, all zero bits
	e.piWrites = map[byte][]byte{}
	return nil
}

func (e *fakeT7) Close() error { return nil }

func (e *fakeT7) Send(_ context.Context, f gocan.Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch f.ID {
	case t7kwp.INIT_MSG_ID:
		if f.Data[1] != t7kwp.START_COM_REQ {
			return nil
		}
		if e.silent || e.session {
			e.trace = append(e.trace, "init ignored")
			return nil
		}
		e.trace = append(e.trace, "init")
		e.session = true
		e.bus.Deliver(gocan.NewFrame(t7kwp.INIT_RESP_ID, []byte{0xC0, 0xA1, 0x03, 0xC1, 0xEA, 0x8F, 0x02, 0x58}))
	case t7kwp.REQ_MSG_ID:
		if f.Data[0]&0x40 != 0 {
			e.req = e.req[:0]
		}
		e.req = append(e.req, f.Data[2:f.Length]...)
		if f.Data[0]&0x3F == 0 && len(e.req) > 0 && len(e.req) > int(e.req[0]) {
			msg := append([]byte(nil), e.req[1:1+e.req[0]]...)
			e.req = e.req[:0]
			e.pending = nil
			if !e.session {
				e.trace = append(e.trace, "no session "+label(msg))
				return nil
			}
			e.trace = append(e.trace, label(msg))
			if e.fault != nil {
				if r := e.fault(msg[0], msg[1:]); r != nil {
					if len(r) > 0 {
						e.respond(r)
					}
					return nil
				}
			}
			e.respond(e.handle(msg[0], msg[1:]))
		}
	case t7kwp.RESP_CHUNK_CONF_ID:
		if len(e.pending) > 0 {
			next := e.pending[0]
			e.pending = e.pending[1:]
			e.bus.Deliver(next)
		}
	case t7kwp.FAST_DOWN_ID:
		if !e.armed {
			return nil
		}
		for _, c := range f.Data[:f.Length] {
			if e.fastCount < len(e.fastBuf) {
				e.fastBuf[e.fastCount] = c
				e.fastCount++
			}
		}
	}
	return nil
}

// label names a request for the trace: the service id, plus the first
// parameter for services where that selects what is done.
func label(msg []byte) string {
	switch msg[0] {
	case t7kwp.ECU_RESET, t7kwp.READ_VEHICLE_IDENTIFICATION, t7kwp.SECURITY_ACCESS,
		t7kwp.START_ROUTINE_BY_IDENTIFIER, t7kwp.REQUEST_ROUTINE_RESULTS_BY_IDENTIFIER,
		t7kwp.WRITE_DATA_BY_IDENTIFIER, t7kwp.READ_DTC_BY_STATUS:
		if len(msg) > 1 {
			return fmt.Sprintf("%02X %02X", msg[0], msg[1])
		}
	}
	return fmt.Sprintf("%02X", msg[0])
}

func u24(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }

func (e *fakeT7) handle(sid byte, p []byte) []byte {
	nrc := func(code byte) []byte { return []byte{0x7F, sid, code} }
	switch sid {
	case t7kwp.STOP_COM_REQ:
		e.session, e.unlocked = false, false
		return []byte{0xC2}
	case t7kwp.TESTER_PRESENT:
		return []byte{0x7E}
	case t7kwp.ECU_RESET:
		e.session, e.unlocked = false, false
		return []byte{0x51, 0x81}
	case t7kwp.SECURITY_ACCESS:
		if p[0]%2 == 1 {
			return []byte{0x67, p[0], byte(e.seed >> 8), byte(e.seed)}
		}
		key := uint16(p[1])<<8 | uint16(p[2])
		e.keys = append(e.keys, key)
		if key != (e.seed<<2^e.seedKey.XOR)-e.seedKey.Sub {
			return nrc(t7kwp.INVALID_KEY)
		}
		e.unlocked = true
		return []byte{0x67, p[0], 0x34}
	case t7kwp.READ_VEHICLE_IDENTIFICATION:
		v, ok := e.headers[p[0]]
		if !ok {
			return nrc(t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		return append([]byte{0x5A, p[0]}, v...)
	case t7kwp.READ_DTC_BY_STATUS:
		out := []byte{0x58, byte(len(e.dtcs))}
		for _, d := range e.dtcs {
			out = append(out, d[:]...)
		}
		return out
	case t7kwp.DYNAMICALLY_DEFINE_IDENTIFIER: // F0 03 00 len adr[3]: read setup
		if len(p) < 7 || p[0] != 0xF0 || p[1] != 0x03 {
			return nrc(t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		e.readLen, e.readAddr = int(p[3]), u24(p[4:])
		e.reads = append(e.reads, span{e.readAddr, e.readLen})
		return []byte{0x6C, 0xF0}
	case t7kwp.READ_DATA_BY_IDENTIFIER:
		if p[0] != 0xF0 || e.readLen == 0 {
			return nrc(t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		return append([]byte{0x61, 0xF0}, e.mem[e.readAddr:e.readAddr+e.readLen]...)
	case t7kwp.START_ROUTINE_BY_IDENTIFIER:
		return e.routine(p[0], p[1:])
	case t7kwp.REQUEST_ROUTINE_RESULTS_BY_IDENTIFIER:
		return []byte{0x73, p[0], e.eolResult}
	case t7kwp.REQUEST_DOWNLOAD: // adr[3] fmt len[3]
		if !e.erased {
			return nrc(t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
		}
		e.writePtr, e.download = u24(p), true
		e.downloads = append(e.downloads, span{e.writePtr, u24(p[4:])})
		return []byte{0x74}
	case t7kwp.TRANSFER_DATA:
		if !e.download {
			return nrc(t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
		}
		e.program(e.writePtr, p)
		e.blocks = append(e.blocks, span{e.writePtr, len(p)})
		e.writePtr += len(p)
		return []byte{0x76}
	case t7kwp.REQUEST_TRANSFER_EXIT:
		e.download = false
		return []byte{0x77}
	case t7kwp.WRITE_DATA_BY_IDENTIFIER:
		e.piWrites[p[0]] = append([]byte(nil), p[1:]...)
		return []byte{0x7B, p[0]}
	}
	return nrc(t7kwp.SERVICE_NOT_SUPPORTED)
}

func (e *fakeT7) routine(id byte, args []byte) []byte {
	nrc := func(code byte) []byte { return []byte{0x7F, t7kwp.START_ROUTINE_BY_IDENTIFIER, code} }
	ok := []byte{0x71, id}
	switch id {
	case t7kwp.RLI_EOL_START:
		if !e.unlocked {
			return nrc(t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
		}
		e.eol = true
		return ok
	case t7kwp.RLI_ERASE:
		if !e.eol {
			return nrc(t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
		}
		for i := range e.flash {
			e.flash[i] = 0xFF
		}
		e.erased = true
		return ok
	case t7kwp.RLI_END_EOL:
		for _, id := range []byte{0x90, 0x99, 0x98} {
			if _, ok := e.piWrites[id]; !ok {
				return nrc(t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
			}
		}
		return ok
	case t7kwp.RLI_FAST_INFO:
		if e.fastBlock == 0 {
			break
		}
		return []byte{0x71, id, 1, byte(e.fastBlock >> 8), byte(e.fastBlock)}
	case t7kwp.RLI_FAST_UPLOAD:
		if e.fastBlock == 0 {
			break
		}
		adr, n := u24(args), u24(args[3:])
		e.fastUps = append(e.fastUps, adr)
		data := e.mem[adr : adr+n]
		for i := 0; i < n; i += 8 {
			if i == 16 && e.brokenFast != nil && e.brokenFast(adr) {
				continue // the adapter lost a stream frame
			}
			var buf [8]byte
			copy(buf[:], data[i:])
			e.bus.Deliver(gocan.NewFrame(t7kwp.FAST_UP_ID, buf[:]))
		}
		s := t7kwp.FastSum(data)
		return []byte{0x71, id, byte(s >> 24), byte(s >> 16), byte(s >> 8), byte(s)}
	case t7kwp.RLI_FAST_DOWNLOAD:
		if e.fastBlock == 0 || !e.erased {
			break
		}
		e.fastAdr, e.armed, e.fastCount = u24(args), true, 0
		e.fastBuf = make([]byte, u24(args[3:]))
		return ok
	case t7kwp.RLI_FAST_COMMIT:
		if !e.armed {
			return nrc(t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
		}
		e.armed = false
		if e.fastCount != len(e.fastBuf) || t7kwp.FastSum(e.fastBuf) != binary.BigEndian.Uint32(args) {
			return nrc(t7kwp.BLOCK_TRANSFER_DATA_CHECKSUM_ERROR)
		}
		e.program(e.fastAdr, e.fastBuf)
		e.fastBlocks = append(e.fastBlocks, span{e.fastAdr, len(e.fastBuf)})
		return ok
	}
	return nrc(t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
}

// program writes data to flash the way flash cells take it: bits only clear.
func (e *fakeT7) program(addr int, data []byte) {
	for i, b := range data {
		e.flash[addr+i] &= b
	}
}

// respond chunks payload as the T7 does: length byte plus five payload bytes
// in the first frame, six in each following one, byte 0 counting down the
// frames still to come with 0x40 on the first and 0x80 on all. The first
// frame goes out now, the rest on the tester's acks.
func (e *fakeT7) respond(payload []byte) {
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

// count returns how many requests in the trace carry label l.
func (e *fakeT7) count(l string) int {
	n := 0
	for _, s := range e.trace {
		if s == l {
			n++
		}
	}
	return n
}

// recorder collects what a Client reports through its ecu.Config callbacks.
// They run on the caller's goroutine.
type recorder struct {
	msgs     []string
	errs     []error
	progress []float64
}

func (r *recorder) config() *ecu.Config {
	return &ecu.Config{
		Name:       "Trionic 7",
		OnMessage:  func(s string) { r.msgs = append(r.msgs, s) },
		OnError:    func(err error) { r.errs = append(r.errs, err) },
		OnProgress: func(f float64) { r.progress = append(r.progress, f) },
	}
}

// hasMsg reports whether any message contains sub.
func (r *recorder) hasMsg(sub string) bool {
	for _, m := range r.msgs {
		if bytes.Contains([]byte(m), []byte(sub)) {
			return true
		}
	}
	return false
}

// newClient opens e on a fresh bus and returns a Client reporting to rec.
func newClient(t *testing.T, e *fakeT7, rec *recorder) *Client {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	if rec == nil {
		rec = &recorder{}
	}
	return New(bus, rec.config()).(*Client)
}

// skipWaits makes the package's fixed delays return at once, still honouring
// a cancelled context.
func skipWaits(t *testing.T) {
	old := wait
	wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	t.Cleanup(func() { wait = old })
}

// testCtx bounds a test well past any single operation, so a hung exchange
// fails the test instead of the run.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// patternMem is memory with a position-dependent pattern, so a shifted or
// repeated chunk shows.
func patternMem(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*131 + i>>8)
	}
	return b
}

type piField = struct {
	id   byte
	data []byte
}

func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

// testBinTop is the PI top-of-program of testBin: the ROM checksum covers
// [0, testBinTop).
const testBinTop = 0x2000

// testBin returns a 512 KB Intel-order T7 image: the magic, a 256-byte data
// run at 0x1000, flash-algorithm bytes at 0x7C000 that a flash must leave
// alone, a PI area whose 0xFB matches the ROM checksum, and 0xFF everywhere
// else, so a flash writes only a few segments.
func testBin() []byte {
	bin := bytes.Repeat([]byte{0xFF}, 0x80000)
	copy(bin, []byte{0xFF, 0xFF, 0xEF, 0xFC})
	copy(bin[0x1000:0x1100], patternMem(0x100))
	copy(bin[0x7C000:0x7C100], patternMem(0x100))
	var sum uint32
	for p := 0; p < testBinTop; p += 4 {
		sum += binary.BigEndian.Uint32(bin[p:])
	}
	writePI(bin, []piField{
		{0x90, []byte("YS3DD58N512345678")},
		{0x92, []byte("123456789012345")},
		{0xFB, be32(sum)},
		{0xFE, be32(testBinTop)},
		{0xFD, be32(0)},
		{0xF9, []byte("1")},
	})
	return bin
}
