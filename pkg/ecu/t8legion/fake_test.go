package t8legion

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

const (
	t8Size  = 0x100000 // Trionic 8 main flash
	mcpSize = 0x40100  // MCP flash including the 256 byte shadow
)

// t8Partitions are the Trionic 8 main flash partitions as the Legion loader
// numbers them: partition n (1..9) spans t8Partitions[n-1]..t8Partitions[n].
var t8Partitions = []int{0x000000, 0x004000, 0x006000, 0x008000, 0x020000, 0x040000, 0x060000, 0x080000, 0x0C0000, 0x100000}

// legionKey is the per-byte XOR the client applies to block data before
// TransferData, repeating every six bytes from the start of each block.
var legionKey = [6]byte{0x39, 0x68, 0x77, 0x6D, 0x47, 0x39}

// partitionRange returns the flash range of partition p (1..9) of device dev.
func partitionRange(dev byte, p int) (start, end int) {
	if dev == EcuByte_MCP {
		if p == 9 {
			return 0x40000, 0x40100
		}
		return (p - 1) * 0x8000, p * 0x8000
	}
	return t8Partitions[p-1], t8Partitions[p]
}

func ffBytes(n int) []byte { return bytes.Repeat([]byte{0xFF}, n) }

func allFF(b []byte) bool {
	for _, v := range b {
		if v != 0xFF {
			return false
		}
	}
	return true
}

type demandReq struct {
	cmd  Command
	wish uint16
}

type readReq struct {
	pci  byte
	addr int
	n    int
}

type eraseReq struct {
	dev   byte
	mask  uint16
	frame [8]byte
}

// fakeLegion is a Trionic 8 behind a virtual CAN adapter, requests on reqID
// and replies on respID. Until the client executes the uploaded loader at
// 0x102400 it acts as the stock GMLAN firmware: initiateDiagnosticOperation,
// disableNormalCommunication, reportProgrammedState, programmingMode,
// securityAccess (checked with t8sec), requestDownload and TransferData into
// RAM. After that it acts as the Legion loader: ping, IDemand (02 A5 ...),
// readDataByLocalIdentifier (xx 21 ...) with FF-block skipping, the erase
// request (dev 34 01 ...) with its status poll on pci 0xF0, XOR-coded block
// writes with a checksum-16, and exit (01 20). Replies are delivered from
// inside Send, like pkg/ecusim.
//
// Anything the loader would choke on (a consecutive frame out of sequence,
// a block write outside flash, a malformed erase mask) is recorded in
// violations, which fail the test.
type fakeLegion struct {
	mu            sync.Mutex
	bus           *gocan.Bus
	name          string
	reqID, respID uint32

	// stock firmware
	ignore      map[byte]bool // stock services left unanswered
	seed        [2]byte
	seedLevel   byte
	unlocked    bool
	accessLevel byte   // security level that was granted
	downloadPCI []byte // PCI byte of every accepted requestDownload
	ram         map[uint32]byte
	executed    []uint32 // TransferData $80 start addresses
	stockLog    []string // stock requests in order, e.g. "A5 01"

	// TransferData in progress
	tdAddr uint32
	tdLeft int
	tdSeq  byte
	tdBuf  []byte

	// Legion loader
	running      bool
	flash        map[byte][]byte
	device       byte   // device of the last erase; block writes go there
	md5          []byte // last md5 computed by IDemand 2/3, read back on pci 7
	pending      [][]byte
	ifl          uint16
	mcpRunning   bool
	crc          [4]byte
	adc          [2]byte
	demandStatus map[Command][]byte  // scripted IDemand status bytes, then 1
	erasePolls   []int               // status byte of each 0xF0 poll after an erase (-1 = garbled reply), then 1; nil = [device]
	polls        []int               // what is left of erasePolls for the erase in progress
	blockReplies map[uint32][][]byte // scripted replies (nil = silence) to a finished block write at an address; the block is not programmed
	tdReject     map[uint32]int      // times a TransferData to an address is refused
	demands      []demandReq
	reads        []readReq
	erases       []eraseReq
	transfers    []uint32 // TransferData start addresses

	sent       []gocan.Frame
	violations []string

	// hook, when set, sees every frame on reqID first. Returning true means
	// it handled the frame and replies are delivered. It runs with mu held.
	hook func(f gocan.Frame) (replies [][]byte, handled bool)
}

func newFake() *fakeLegion {
	return &fakeLegion{
		name:         "canusb",
		reqID:        0x7E0,
		respID:       0x7E8,
		ignore:       map[byte]bool{},
		seed:         [2]byte{0x4A, 0x21},
		ram:          map[uint32]byte{},
		flash:        map[byte][]byte{EcuByte_T8: ffBytes(t8Size), EcuByte_MCP: ffBytes(mcpSize)},
		device:       EcuByte_T8,
		demandStatus: map[Command][]byte{},
		blockReplies: map[uint32][][]byte{},
		tdReject:     map[uint32]int{},
	}
}

func (s *fakeLegion) Name() string { return s.name }

func (s *fakeLegion) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	return nil
}

func (s *fakeLegion) Close() error { return nil }

func (s *fakeLegion) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, f)
	if f.ID != s.reqID {
		return nil // tester present on 0x101 wants no answer
	}
	if s.hook != nil {
		if replies, ok := s.hook(f); ok {
			for _, r := range replies {
				s.reply(r...)
			}
			return nil
		}
	}
	d := f.Data
	switch {
	case d[0]>>4 == 2:
		s.consecutive(d)
	case d[0] == 0x10 && d[2] == 0x36:
		s.transferStart(d)
	case s.running:
		s.loader(f)
	default:
		s.stock(d)
	}
	return nil
}

func (s *fakeLegion) reply(b ...byte) {
	out := make([]byte, 8)
	copy(out, b)
	s.bus.Deliver(gocan.NewFrame(s.respID, out))
}

func (s *fakeLegion) violate(format string, args ...any) {
	s.violations = append(s.violations, fmt.Sprintf(format, args...))
}

func (s *fakeLegion) stock(d [8]byte) {
	if s.ignore[d[1]] {
		return
	}
	switch {
	case d[0] == 0x02 && d[1] == 0x10:
		s.stockLog = append(s.stockLog, fmt.Sprintf("10 %02X", d[2]))
		s.reply(0x01, 0x50)
	case d[0] == 0x01 && d[1] == 0x28:
		s.stockLog = append(s.stockLog, "28")
		s.reply(0x01, 0x68)
	case d[0] == 0x01 && d[1] == 0xA2:
		s.stockLog = append(s.stockLog, "A2")
		s.reply(0x02, 0xE2, 0x00)
	case d[0] == 0x02 && d[1] == 0xA5:
		s.stockLog = append(s.stockLog, fmt.Sprintf("A5 %02X", d[2]))
		if d[2] == 0x01 || d[2] == 0x02 {
			s.reply(0x01, 0xE5)
		}
	case d[0] == 0x02 && d[1] == 0x27:
		s.stockLog = append(s.stockLog, fmt.Sprintf("27 %02X", d[2]))
		s.seedLevel = d[2]
		s.reply(0x04, 0x67, d[2], s.seed[0], s.seed[1])
	case d[0] == 0x04 && d[1] == 0x27:
		s.stockLog = append(s.stockLog, fmt.Sprintf("27 %02X", d[2]))
		hi, lo := t8sec.CalculateAccessKey(s.seed[:], s.seedLevel)
		if d[2] != s.seedLevel+1 || d[3] != hi || d[4] != lo {
			s.reply(0x03, 0x7F, 0x27, 0x35)
			return
		}
		s.unlocked, s.accessLevel = true, s.seedLevel
		s.reply(0x02, 0x67, d[2])
	case (d[0] == 0x05 || d[0] == 0x06) && d[1] == 0x34:
		s.stockLog = append(s.stockLog, fmt.Sprintf("34 (%02X)", d[0]))
		if !s.unlocked {
			s.reply(0x03, 0x7F, 0x34, 0x33)
			return
		}
		s.downloadPCI = append(s.downloadPCI, d[0])
		s.reply(0x01, 0x74)
	case d[0] == 0x06 && d[1] == 0x36 && d[2] == 0x80:
		addr := binary.BigEndian.Uint32(d[3:7])
		s.stockLog = append(s.stockLog, fmt.Sprintf("36 80 %06X", addr))
		s.executed = append(s.executed, addr)
		if addr == 0x102400 {
			s.running = true
		}
		s.reply(0x01, 0x76)
	}
	// anything else, like the Legion ping, is ignored by the stock firmware
}

func (s *fakeLegion) transferStart(d [8]byte) {
	addr := binary.BigEndian.Uint32(d[4:8])
	s.transfers = append(s.transfers, addr)
	if n := s.tdReject[addr]; n > 0 {
		s.tdReject[addr] = n - 1
		s.reply(0x03, 0x7F, 0x36, 0x22)
		return
	}
	if !s.running && len(s.downloadPCI) == 0 {
		s.reply(0x03, 0x7F, 0x36, 0x22)
		return
	}
	s.tdAddr, s.tdLeft, s.tdSeq, s.tdBuf = addr, int(d[1])-6, 0x21, nil
	s.reply(0x30, 0x00, 0x00)
}

func (s *fakeLegion) consecutive(d [8]byte) {
	if s.tdLeft == 0 {
		s.violate("consecutive frame %02X outside a transfer", d[0])
		return
	}
	if d[0] != s.tdSeq {
		s.violate("consecutive frame %02X, want %02X", d[0], s.tdSeq)
		s.tdLeft = 0
		return
	}
	s.tdSeq++
	if s.tdSeq > 0x2F {
		s.tdSeq = 0x20
	}
	n := min(7, s.tdLeft)
	s.tdBuf = append(s.tdBuf, d[1:1+n]...)
	s.tdLeft -= n
	if s.tdLeft > 0 {
		return
	}
	if q := s.blockReplies[s.tdAddr]; len(q) > 0 {
		s.blockReplies[s.tdAddr] = q[1:]
		if q[0] != nil {
			s.reply(q[0]...)
		}
		return
	}
	if !s.running {
		for i, b := range s.tdBuf {
			s.ram[s.tdAddr+uint32(i)] = b
		}
		s.reply(0x01, 0x76)
		return
	}
	s.reply(s.programBlock()...)
}

// programBlock decodes a finished 128 byte block write, checks its
// checksum-16 and programs it into the flash of the current device.
func (s *fakeLegion) programBlock() []byte {
	if len(s.tdBuf) != 130 {
		s.violate("block write of %d bytes at %X", len(s.tdBuf), s.tdAddr)
		return []byte{0x03, 0x7F, 0x36, 0x79}
	}
	coded := s.tdBuf[:128]
	var sum int
	for _, b := range coded {
		sum += int(b)
	}
	if uint16(sum) != binary.BigEndian.Uint16(s.tdBuf[128:130]) {
		return []byte{0x03, 0x7F, 0x36, 0x77}
	}
	mem := s.flash[s.device]
	addr := int(s.tdAddr)
	if addr+128 > len(mem) {
		s.violate("block write at %X outside device %d flash", addr, s.device)
		return []byte{0x03, 0x7F, 0x36, 0x74}
	}
	for i, b := range coded {
		mem[addr+i] &= b ^ legionKey[i%6] // flash can only clear bits
	}
	return []byte{0x01, 0x76}
}

func (s *fakeLegion) loader(f gocan.Frame) {
	d := f.Data
	switch {
	case d[0] == 0xEF && d[1] == 0xBE:
		s.reply(0xDE, 0xAD, 0xF0, 0x0F)
	case d[0] == 0x01 && d[1] == 0x20:
		s.running = false
		s.reply(0x01, 0x60)
	case d[0] == 0x30:
		for _, cf := range s.pending {
			s.reply(cf...)
		}
		s.pending = nil
	case d[0] == 0x02 && d[1] == 0xA5:
		s.demand(d)
	case d[1] == 0x21:
		s.read(d)
	case d[1] == 0x34 && d[2] == 0x01:
		s.erase(d)
	default:
		s.violate("loader got unexpected frame %s", f.String())
	}
}

func (s *fakeLegion) demand(d [8]byte) {
	cmd := Command(d[2])
	wish := binary.BigEndian.Uint16(d[6:8])
	s.demands = append(s.demands, demandReq{cmd, wish})
	status := byte(1)
	if q := s.demandStatus[cmd]; len(q) > 0 {
		status, s.demandStatus[cmd] = q[0], q[1:]
	}
	out := []byte{0x33, 0x55, byte(cmd), status, 0, 0, 0, 0}
	if status == 1 {
		switch cmd {
		case SetInterFrameLatency:
			s.ifl = wish
		case GetCRC32:
			copy(out[4:], s.crc[:])
		case GetTrionic8MD5:
			s.md5 = s.partitionMD5(EcuByte_T8, int(wish))
		case GetTrionic8MCPMD5:
			s.md5 = s.partitionMD5(EcuByte_MCP, int(wish))
		case StartSecondaryBootloader:
			s.mcpRunning = true
		case ReadADCPin:
			out[4], out[5] = s.adc[0], s.adc[1]
		}
	}
	s.reply(out...)
}

func (s *fakeLegion) partitionMD5(dev byte, p int) []byte {
	mem := s.flash[dev]
	start, end := 0, len(mem)
	if p >= 1 && p <= 9 {
		start, end = partitionRange(dev, p)
	}
	sum := md5.Sum(mem[start:end])
	return sum[:]
}

func (s *fakeLegion) read(d [8]byte) {
	s.pending = nil
	pci, n, addr := d[0], int(d[2]), int(binary.BigEndian.Uint32(d[3:7]))
	s.reads = append(s.reads, readReq{pci, addr, n})
	if pci == 0xF0 {
		st := 1
		if len(s.polls) > 0 {
			st, s.polls = s.polls[0], s.polls[1:]
		}
		if st < 0 {
			s.reply(0x7E)
			return
		}
		s.reply(0x10, 0x06, 0x61, 0x00, 0x00, 0x00, 0x00, byte(st))
		return
	}
	src := s.flash[pci]
	if pci == 7 {
		src = s.md5
	}
	if addr+n > len(src) {
		s.reply(0x03, 0x7F, 0x21, 0x31)
		return
	}
	data := src[addr : addr+n]
	skip := 0
	if pci != 7 && n == 0x80 {
		for a := addr; a+0x80 <= len(src) && skip < 0xFF && allFF(src[a:a+0x80]); a += 0x80 {
			skip++
		}
	}
	first := append([]byte{0x10, byte(n + 2), 0x61, byte(skip)}, data[:min(4, n)]...)
	seq := byte(0x21)
	for i := 4; i < n; i += 7 {
		s.pending = append(s.pending, append([]byte{seq}, data[i:min(i+7, n)]...))
		seq++
		if seq > 0x2F {
			seq = 0x20
		}
	}
	s.reply(first...)
}

func (s *fakeLegion) erase(d [8]byte) {
	dev, hi, lo := d[0], d[5], d[6]
	if d[3] != ^hi || d[4] != ^lo || d[7] != 0 {
		s.violate("erase request % X has bad mask check bytes", d[:])
	}
	mask := uint16(hi)<<8 | uint16(lo)
	s.erases = append(s.erases, eraseReq{dev, mask, d})
	s.device = dev
	mem := s.flash[dev]
	for p := 1; p <= 9; p++ {
		if mask&(1<<(p-1)) == 0 {
			continue
		}
		start, end := partitionRange(dev, p)
		copy(mem[start:end], ffBytes(end-start))
	}
	s.polls = append([]int(nil), s.erasePolls...)
	if s.erasePolls == nil {
		s.polls = []int{int(dev)}
	}
}

// framesOn returns the frames sent on id.
func (s *fakeLegion) framesOn(id uint32) []gocan.Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []gocan.Frame
	for _, f := range s.sent {
		if f.ID == id {
			out = append(out, f)
		}
	}
	return out
}

func (s *fakeLegion) with(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

func (s *fakeLegion) mem(dev byte, start, end int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.flash[dev][start:end])
}

func (s *fakeLegion) ramBytes(addr uint32, n int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, n)
	for i := range out {
		out[i] = s.ram[addr+uint32(i)]
	}
	return out
}

// events collects what the client reports through its ecu.Config callbacks.
type events struct {
	mu       sync.Mutex
	messages []string
	errs     []error
	progress []float64
}

func (e *events) message(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = append(e.messages, s)
}

func (e *events) error(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, err)
}

func (e *events) prog(v float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.progress = append(e.progress, v)
}

func (e *events) hasMessage(sub string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.messages {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

func (e *events) errors() []error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]error(nil), e.errs...)
}

func (e *events) progressValues() []float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]float64(nil), e.progress...)
}

// newTestClient opens a bus on f and returns a Legion client on its IDs
// whose callbacks are recorded in the returned events. The test fails if
// the client did anything the fake loader would choke on.
func newTestClient(t *testing.T, f *fakeLegion) (*Client, *events) {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = bus.Close()
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, v := range f.violations {
			t.Errorf("loader protocol violation: %s", v)
		}
	})
	ev := &events{}
	cl := New(bus, &ecu.Config{Name: "Trionic 8", OnMessage: ev.message, OnError: ev.error, OnProgress: ev.prog}, f.reqID, f.respID)
	return cl, ev
}

// runningClient returns a client already talking to a running Legion
// loader, so tests can skip the bootstrap.
func runningClient(t *testing.T) (*Client, *fakeLegion, *events) {
	t.Helper()
	f := newFake()
	f.running = true
	cl, ev := newTestClient(t, f)
	cl.legionRunning = true
	return cl, f, ev
}

// frame8 pads b to a full 8 byte CAN payload.
func frame8(b ...byte) []byte {
	out := make([]byte, 8)
	copy(out, b)
	return out
}

// TestMain makes the package's fixed delays return at once (still honouring a
// cancelled context), so the erase polling runs at full speed.
func TestMain(m *testing.M) {
	wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	os.Exit(m.Run())
}

// testCtx bounds a client call so a stuck exchange ends with an error
// instead of hanging the run.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}
