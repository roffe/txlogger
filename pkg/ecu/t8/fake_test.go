package t8

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
	"github.com/roffe/txlogger/pkg/ecu/t8util"
)

// fakeT8 is a Trionic 8 behind a virtual CAN adapter. Until legion is set it
// speaks GMLAN over ISO-TP on 0x7E0/0x7E8, with UUDT replies ($AA, $A9) on
// 0x5E8, and also answers OBD functional requests on 0x7DF; once legion is
// set it also answers the legion bootloader (ping, IDemand,
// ReadDataByLocalIdentifier, erase, TransferData, exit) on the same IDs,
// reading and writing flash, and no longer answers GMLAN. The stock firmware
// takes the legion upload (requestDownload, TransferData into RAM) and starts
// it on the execute at 0x102400; the legion's exit drops back to the stock
// firmware. With cim set, a second node (see newFakeCIM) answers GMLAN on its
// own IDs. It answers from inside Send, so every reply is on the bus before
// Send returns.
type fakeT8 struct {
	mu   sync.Mutex
	bus  *gocan.Bus
	sent []gocan.Frame // every frame the tester sent, to any node

	rx, tx uint32                          // request and response IDs of this node
	key    func([]byte, byte) (byte, byte) // seed/key algorithm of this node
	cim    *fakeT8                         // CIM on the same bus, nil when not needed

	dids   map[byte][]byte // $1A answers; unknown DIDs get requestOutOfRange
	nrc    map[byte]byte   // service -> negative response code to answer with
	dpids  map[byte][]byte // $AA answers: the UUDT frame sent on 0x5E8
	dtcs   [][]byte        // $A9 answers: UUDT frames on 0x5E8, terminator appended
	writes []didWrite      // $3B requests in order
	devctl [][]byte        // $AE requests (command and code) in order

	seed uint16

	// While married the stock firmware refuses to rewrite the VIN ($3B $90).
	// Device control $60 and then $6E, both carrying 00 and pin, virginize
	// it, and so does a restart from the legion with the virgin NVDM in
	// flash.
	married bool
	pin     string
	pinSeen bool // $60 came with the right PIN

	log []string // requests and test markers in order, see note

	req     []byte // multi-frame request reassembly
	reqLen  int
	pending []gocan.Frame // response consecutive frames released by flow control

	legion     bool
	flash      []byte            // 1 MiB flash image
	mcpVersion string            // MCP firmware string at 0x810C
	md5        []byte            // result of the last md5 demand
	md5Lie     map[uint16]int    // partition -> wrong md5 answers left
	dropWrites bool              // acknowledge TransferData without storing it
	erases     []uint64          // partition masks of erase commands
	erasePolls int               // status polls since the last erase command
	transfers  []int             // TransferData block addresses
	demands    map[byte][]uint16 // IDemand command -> wishes
}

type didWrite struct {
	pid  byte
	data []byte
}

func newFakeT8() *fakeT8 {
	return &fakeT8{
		rx:      0x7E0,
		tx:      0x7E8,
		key:     t8sec.CalculateAccessKey,
		dids:    map[byte][]byte{},
		nrc:     map[byte]byte{},
		dpids:   map[byte][]byte{},
		md5Lie:  map[uint16]int{},
		demands: map[byte][]uint16{},
	}
}

// newFakeCIM returns the CIM node a fakeT8 hosts through its cim field: GMLAN
// on 0x245/0x645 with the CIM seed/key algorithm.
func newFakeCIM() *fakeT8 {
	c := newFakeT8()
	c.rx, c.tx, c.key = 0x245, 0x645, t8sec.CalculateKeyForCIM
	return c
}

func (s *fakeT8) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	if s.cim != nil {
		s.cim.bus = bus
	}
	return nil
}

func (s *fakeT8) Close() error { return nil }

func (s *fakeT8) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, f)
	d := f.Data[:f.Length]
	switch {
	case f.ID == s.rx:
		if len(d) == 8 && d[0] == 0xEF && d[1] == 0xBE {
			s.note("ping") // whether or not the legion is there to answer
		}
		if s.legion && s.handleLegion(d) {
			return nil
		}
		s.frame(d)
	case f.ID == 0x7DF: // OBD functional request, answered by the ECU
		s.frame(d)
	case s.cim != nil && f.ID == s.cim.rx:
		s.cim.mu.Lock()
		defer s.cim.mu.Unlock()
		s.cim.frame(d)
	}
	return nil // functional 0x101 tester present wants no answer
}

// frame handles one ISO-TP frame addressed to this node.
func (s *fakeT8) frame(d []byte) {
	switch d[0] >> 4 {
	case 0: // single frame
		if n := int(d[0]); n > 0 && n < len(d) {
			s.reply(s.handle(d[1 : 1+n]))
		}
	case 1: // first frame of a multi-frame request
		sid := d[2]
		if code, ok := s.nrc[sid]; ok {
			s.reply([]byte{0x7F, sid, code})
			return
		}
		s.reqLen = int(d[0]&0x0F)<<8 | int(d[1])
		s.req = append(s.req[:0], d[2:]...)
		s.deliver(s.tx, []byte{0x30, 0x00, 0x00})
	case 2: // consecutive frame
		s.req = append(s.req, d[1:]...)
		if s.reqLen > 0 && len(s.req) >= s.reqLen {
			msg := s.req[:s.reqLen]
			s.reqLen = 0
			s.reply(s.handle(msg))
		}
	case 3: // flow control: release our consecutive frames
		frames := s.pending
		s.pending = nil
		for _, p := range frames {
			s.bus.Deliver(p)
		}
	}
}

// handle answers one GMLAN request with its response payload; nil sends
// nothing on 0x7E8.
func (s *fakeT8) handle(msg []byte) []byte {
	sid, p := msg[0], msg[1:]
	s.note("% X", msg[:min(len(msg), 2)])
	if s.legion && sid != 0x36 {
		return nil // the legion speaks no GMLAN, its block writes aside
	}
	if code, ok := s.nrc[sid]; ok {
		return []byte{0x7F, sid, code}
	}
	nrc := func(code byte) []byte { return []byte{0x7F, sid, code} }
	switch sid {
	case 0x04, 0x10, 0x20, 0x28, 0x3E, 0xA5: // clear DTCs, diagnostic session, normal mode, ...
		return []byte{sid + 0x40}
	case 0xA2: // reportProgrammedState: fully programmed
		return []byte{0xE2, 0x00}
	case 0x27: // securityAccess with the T8 seed/key
		level := p[0]
		if level%2 == 1 {
			s.seed = 0x1234
			return []byte{0x67, level, byte(s.seed >> 8), byte(s.seed)}
		}
		hi, lo := s.key([]byte{byte(s.seed >> 8), byte(s.seed)}, level-1)
		if len(p) < 3 || p[1] != hi || p[2] != lo {
			return nrc(0x35)
		}
		return []byte{0x67, level}
	case 0x1A: // readDataByIdentifier
		data, ok := s.dids[p[0]]
		if !ok {
			return nrc(0x31)
		}
		return append([]byte{0x5A, p[0]}, data...)
	case 0x3B: // writeDataByIdentifier
		if s.married && p[0] == 0x90 {
			return nrc(0x22) // conditionsNotCorrect
		}
		s.writes = append(s.writes, didWrite{p[0], append([]byte(nil), p[1:]...)})
		return []byte{0x7B, p[0]}
	case 0xAE: // deviceControl
		s.devctl = append(s.devctl, append([]byte(nil), p...))
		if p[0] == 0x60 || p[0] == 0x6E { // PIN commands: 00 and the four PIN characters
			if !bytes.Equal(p[1:], append([]byte{0x00}, s.pin...)) {
				return nrc(0x35) // invalidKey
			}
			if p[0] == 0x60 {
				s.pinSeen = true
			} else if s.pinSeen {
				s.married = false
			}
		}
		return []byte{0xEE, p[0]}
	case 0x34: // requestDownload of the legion
		return []byte{0x74}
	case 0xAA: // readDataByPacketIdentifier: the packet comes as UUDT
		frame, ok := s.dpids[p[1]]
		if !ok {
			return nrc(0x31)
		}
		s.deliver(0x5E8, frame)
		return nil
	case 0xA9: // readDiagnosticInformation: pending, then the UUDT stream
		s.reply([]byte{0x7F, 0xA9, 0x78})
		for _, d := range s.dtcs {
			s.deliver(0x5E8, d)
		}
		s.deliver(0x5E8, []byte{0x81, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return nil
	case 0x36: // transferData: 36 <mode> <addr32> <data>
		addr := int(binary.BigEndian.Uint32(p[1:5]))
		if p[0] == 0x80 { // downloadAndExecute: run what was uploaded
			s.legion = s.legion || addr == 0x102400
			return []byte{0x76}
		}
		if !s.legion { // the legion upload into RAM, not kept
			return []byte{0x76}
		}
		// a legion write: <coded block> <checksum16>
		s.transfers = append(s.transfers, addr)
		if !s.dropWrites {
			for i := range 0x80 {
				s.flash[addr+i] = t8util.CodeByte(p[5+i], i%6)
			}
		}
		return []byte{0x76}
	}
	return nrc(0x11)
}

// handleLegion answers the legion bootloader frames, which are not ISO-TP.
func (s *fakeT8) handleLegion(d []byte) bool {
	switch {
	case len(d) == 8 && d[0] == 0xEF && d[1] == 0xBE: // ping
		s.deliver(0x7E8, []byte{0xDE, 0xAD, 0xF0, 0x0F, 0x00, 0x00, 0x00, 0x00})
	case len(d) == 2 && d[0] == 0x01 && d[1] == 0x20: // exit: restart into the stock firmware
		s.note("exit")
		s.legion = false
		if s.flash != nil && bytes.Equal(s.flash[0x4000:0x8000], virginNVDM) {
			s.married = false
		}
		s.deliver(0x7E8, []byte{0x01, 0x60})
	case len(d) == 8 && d[0] == 0x02 && d[1] == 0xA5: // IDemand
		cmd, wish := d[2], uint16(d[6])<<8|uint16(d[7])
		s.note("demand %02X", cmd)
		s.demands[cmd] = append(s.demands[cmd], wish)
		if cmd == 2 {
			s.md5 = t8util.GetPartitionMD5(s.flash, 6, int(wish))
			if s.md5Lie[wish] > 0 {
				s.md5Lie[wish]--
				s.md5[0] ^= 0xFF
			}
		}
		s.deliver(0x7E8, []byte{0x02, 0xE5, cmd, 0x01, 0x00, 0x00, 0x00, 0x00})
	case len(d) == 8 && d[0] == 0x06 && d[1] == 0x34 && d[2] == 0x01: // erase: mask in bytes 5 and 6
		mask := uint64(d[6]) | uint64(d[5])<<8
		s.note("erase %#x", mask)
		s.erases = append(s.erases, mask)
		s.erasePolls = 0
		for p := 1; p <= 9; p++ {
			if mask&(1<<(p-1)) != 0 {
				for i := legionParts[p-1]; i < legionParts[p]; i++ {
					s.flash[i] = 0xFF
				}
			}
		}
	case len(d) == 8 && (d[0] == 0x05 || d[0] == 0x06 || d[0] == 0x07 || d[0] == 0xF0) && d[1] == 0x21: // ReadDataByLocalIdentifier
		s.note("read %02X", d[0])
		s.legionRead(d[0], int(binary.BigEndian.Uint32(d[3:7])), int(d[2]))
	default:
		return false
	}
	return true
}

// virginNVDM is the NVDM (0x4000..0x8000) of a virgin T8.
var virginNVDM = t8util.GetVirginNVDM()[0x4000:0x8000]

// note appends an entry to the request log. Callers hold mu.
func (s *fakeT8) note(format string, args ...any) {
	s.log = append(s.log, fmt.Sprintf(format, args...))
}

// legionParts are the T8 flash partition boundaries the legion erases by.
var legionParts = []int{0x000000, 0x004000, 0x006000, 0x008000, 0x020000, 0x040000, 0x060000, 0x080000, 0x0C0000, 0x100000}

// legionRead answers a legion memory read. pci selects the source: 6 the
// flash, 7 the last md5, 5 the MCP, F0 the erase status.
func (s *fakeT8) legionRead(pci byte, addr, length int) {
	if pci == 0xF0 {
		status := byte(1)
		if s.erasePolls == 0 && len(s.erases) > 0 {
			status = 6 // first poll after an erase: the device being erased
		}
		s.erasePolls++
		s.deliver(0x7E8, []byte{0x00, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, status})
		return
	}
	data := make([]byte, length)
	switch pci {
	case 6:
		if run := s.erasedBlocks(addr, length); run > 0 {
			// the loader tags erased blocks instead of sending them
			s.deliver(0x7E8, []byte{0x00, 0x61, 0x00, byte(run), 0xFF, 0xFF, 0xFF, 0xFF})
			return
		}
		copy(data, s.flash[addr:])
	case 7:
		copy(data, s.md5)
	case 5:
		if addr == 0x8100 {
			copy(data[0x0C:], s.mcpVersion)
		}
	}
	s.deliver(0x7E8, append([]byte{0x10, byte(length + 2), 0x61, 0x00}, data[:4]...))
	s.pending = nil
	seq := byte(0x21)
	for pos := 4; pos < length; pos += 7 {
		chunk := make([]byte, 7)
		copy(chunk, data[pos:])
		s.pending = append(s.pending, gocan.NewFrame(0x7E8, append([]byte{seq}, chunk...)))
		seq = 0x20 | (seq+1)&0x0F
	}
}

// erasedBlocks counts the all-0xFF blocks of size length from addr, at most
// 255.
func (s *fakeT8) erasedBlocks(addr, length int) int {
	n := 0
	for a := addr; a+length <= len(s.flash) && n < 255; a += length {
		if !bytes.Equal(s.flash[a:a+length], bytes.Repeat([]byte{0xFF}, length)) {
			break
		}
		n++
	}
	return n
}

// reply frames a GMLAN response payload as ISO-TP on the node's response ID.
func (s *fakeT8) reply(payload []byte) {
	if payload == nil {
		return
	}
	if len(payload) <= 7 {
		s.deliver(s.tx, append([]byte{byte(len(payload))}, payload...))
		return
	}
	s.pending = nil
	seq := byte(0x21)
	for pos := 6; pos < len(payload); pos += 7 {
		s.pending = append(s.pending, gocan.NewFrame(s.tx, append([]byte{seq}, payload[pos:min(pos+7, len(payload))]...)))
		seq = 0x20 | (seq+1)&0x0F
	}
	s.deliver(s.tx, append([]byte{0x10 | byte(len(payload)>>8), byte(len(payload))}, payload[:6]...))
}

// deliver puts one frame on the bus, zero padded to 8 bytes.
func (s *fakeT8) deliver(id uint32, data []byte) {
	b := make([]byte, 8)
	copy(b, data)
	s.bus.Deliver(gocan.NewFrame(id, b))
}

// sentTo returns the payloads of the frames sent to id.
func (s *fakeT8) sentTo(id uint32) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]byte
	for _, f := range s.sent {
		if f.ID == id {
			out = append(out, append([]byte(nil), f.Data[:f.Length]...))
		}
	}
	return out
}

// sentWith reports whether a frame to 0x7E0 started with prefix.
func (s *fakeT8) sentWith(prefix ...byte) bool {
	for _, d := range s.sentTo(0x7E0) {
		if bytes.HasPrefix(d, prefix) {
			return true
		}
	}
	return false
}

func (s *fakeT8) lock() func() {
	s.mu.Lock()
	return s.mu.Unlock
}

// recorder collects what a client reports through its ecu.Config.
type recorder struct {
	mu       sync.Mutex
	msgs     []string
	errs     []error
	progress []float64
}

func (r *recorder) config() *ecu.Config {
	return &ecu.Config{
		Name:       "Trionic 8",
		OnMessage:  func(s string) { r.mu.Lock(); r.msgs = append(r.msgs, s); r.mu.Unlock() },
		OnError:    func(err error) { r.mu.Lock(); r.errs = append(r.errs, err); r.mu.Unlock() },
		OnProgress: func(f float64) { r.mu.Lock(); r.progress = append(r.progress, f); r.mu.Unlock() },
	}
}

func (r *recorder) hasMessage(s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.msgs {
		if m == s {
			return true
		}
	}
	return false
}

// newClient opens dev on a bus closed at the end of the test and returns a
// T8 client on it reporting to a recorder.
func newClient(t *testing.T, dev gocan.Adapter) (*Client, *recorder) {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), dev)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	rec := new(recorder)
	return New(bus, rec.config()).(*Client), rec
}

// skipWaits makes MarryECU's fixed delays return at once, still honouring a
// cancelled context.
func skipWaits(t *testing.T) {
	old := wait
	wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	t.Cleanup(func() { wait = old })
}

// testCtx bounds a test well past any single exchange.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
