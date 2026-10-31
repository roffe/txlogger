package z22se

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

// partitions are the main flash partition boundaries the Legion loader
// erases and hashes by: partition n (1..9) spans partitions[n-1]..partitions[n].
var partitions = []int{0x000000, 0x004000, 0x006000, 0x008000, 0x020000, 0x040000, 0x060000, 0x080000, 0x0C0000, 0x100000}

// legionKey is the XOR the client applies to block data before
// TransferData, repeating every six bytes.
var legionKey = [6]byte{0x39, 0x68, 0x77, 0x6D, 0x47, 0x39}

// fakeZ22 is a Z22SE behind a virtual CAN adapter, requests on 0x7E0 and
// replies on 0x7E8. It answers GMLAN readDataByIdentifier ($1A) and
// securityAccess ($27, checked with t8sec) as ISO-TP, and, while legion is
// set, the Legion loader: ping, exit, IDemand (latency, md5), the 0x21
// reads with erased-block skipping, the erase command with its status poll
// and the XOR coded TransferData block writes. It answers from inside
// Send, so every reply is on the bus before Send returns.
type fakeZ22 struct {
	mu   sync.Mutex
	bus  *gocan.Bus
	sent []gocan.Frame // every frame the tester sent

	dids       map[byte][]byte // $1A answers; unknown DIDs get requestOutOfRange
	seed       uint16
	keys       [][]byte // $27 key requests: level, high, low
	rejectKeys bool     // answer every key with invalidKey

	legion     bool
	flash      []byte
	md5        []byte // result of the last md5 demand
	md5Lie     bool   // corrupt every md5 the loader reports
	dropWrites bool   // acknowledge block writes without programming them
	exitNRC    byte   // non-zero: refuse exit with this negative response code
	exits      int
	demands    []string // "cmd:wish" in order
	erases     []uint16 // partition masks of erase commands
	polls      int      // erase status polls since the last erase command
	writes     []int    // block write addresses
	violations []string

	td      []byte // TransferData block being received
	tdAddr  int
	tdLen   int
	pending []gocan.Frame // consecutive frames released by flow control
}

func newFake(img []byte) *fakeZ22 {
	return &fakeZ22{dids: map[byte][]byte{}, flash: slices.Clone(img)}
}

func (s *fakeZ22) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	return nil
}

func (s *fakeZ22) Close() error { return nil }

func (s *fakeZ22) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, f)
	if f.ID != 0x7E0 {
		return nil // functional tester present on 0x101 wants no answer
	}
	d := f.Data[:f.Length]
	switch {
	case d[0] == 0x30: // flow control from the tester
		frames := s.pending
		s.pending = nil
		for _, p := range frames {
			s.bus.Deliver(p)
		}
	case s.legion && s.loader(d):
	case len(d) >= 2 && d[0] < 8:
		s.gmlan(d[1 : 1+int(d[0])])
	}
	return nil
}

// gmlan answers a single frame GMLAN request.
func (s *fakeZ22) gmlan(msg []byte) {
	sid, p := msg[0], msg[1:]
	switch sid {
	case 0x1A:
		data, ok := s.dids[p[0]]
		if !ok {
			s.isotp([]byte{0x7F, 0x1A, 0x31})
			return
		}
		s.isotp(append([]byte{0x5A, p[0]}, data...))
	case 0x27:
		level := p[0]
		if level%2 == 1 {
			s.seed = 0x4A21
			s.isotp([]byte{0x67, level, byte(s.seed >> 8), byte(s.seed)})
			return
		}
		s.keys = append(s.keys, slices.Clone(p))
		hi, lo := t8sec.CalculateAccessKey([]byte{byte(s.seed >> 8), byte(s.seed)}, level-1)
		if s.rejectKeys || len(p) < 3 || p[1] != hi || p[2] != lo {
			s.isotp([]byte{0x7F, 0x27, 0x35})
			return
		}
		s.isotp([]byte{0x67, level})
	}
	// anything else, like the bootstrap's diagnostic session, goes unanswered
}

// isotp sends a GMLAN response, as a single frame when it fits and
// otherwise as a first frame with the rest held for flow control.
func (s *fakeZ22) isotp(payload []byte) {
	if len(payload) <= 7 {
		s.reply(append([]byte{byte(len(payload))}, payload...)...)
		return
	}
	s.pending = nil
	seq := byte(0x21)
	for pos := 6; pos < len(payload); pos += 7 {
		s.pending = append(s.pending, frame8(append([]byte{seq}, payload[pos:min(pos+7, len(payload))]...)...))
		seq = 0x20 | (seq+1)&0x0F
	}
	s.reply(append([]byte{0x10 | byte(len(payload)>>8), byte(len(payload))}, payload[:6]...)...)
}

// loader answers a Legion loader frame and reports whether it was one.
func (s *fakeZ22) loader(d []byte) bool {
	switch {
	case d[0]>>4 == 2 && s.tdLen > 0:
		s.consecutive(d)
	case len(d) == 8 && d[0] == 0xEF && d[1] == 0xBE: // ping
		s.reply(0xDE, 0xAD, 0xF0, 0x0F)
	case len(d) == 2 && d[0] == 0x01 && d[1] == 0x20: // exit
		s.exits++
		if s.exitNRC != 0 {
			s.reply(0x03, 0x7F, 0x20, s.exitNRC)
			return true
		}
		s.legion = false
		s.reply(0x01, 0x60)
	case len(d) == 8 && d[0] == 0x02 && d[1] == 0xA5: // IDemand
		cmd, wish := d[2], int(d[6])<<8|int(d[7])
		s.demands = append(s.demands, fmt.Sprintf("%d:%d", cmd, wish))
		if cmd == 2 {
			start, end := 0, len(s.flash)
			if wish >= 1 && wish <= 9 {
				start, end = partitions[wish-1], partitions[wish]
			}
			sum := md5.Sum(s.flash[start:end])
			s.md5 = sum[:]
			if s.md5Lie {
				s.md5[0] ^= 0xFF
			}
		}
		s.reply(0x02, 0xE5, cmd, 0x01)
	case len(d) == 8 && d[1] == 0x21: // ReadDataByLocalIdentifier
		s.read(d[0], int(binary.BigEndian.Uint32(d[3:7])), int(d[2]))
	case len(d) == 8 && d[1] == 0x34 && d[2] == 0x01: // erase: device, mask check bytes, mask
		if d[0] != 6 || d[3] != ^d[5] || d[4] != ^d[6] {
			s.violations = append(s.violations, fmt.Sprintf("erase request % X", d))
		}
		mask := uint16(d[5])<<8 | uint16(d[6])
		s.erases = append(s.erases, mask)
		s.polls = 0
		for p := 1; p <= 9; p++ {
			if mask&(1<<(p-1)) != 0 {
				copy(s.flash[partitions[p-1]:partitions[p]], bytes.Repeat([]byte{0xFF}, partitions[p]-partitions[p-1]))
			}
		}
	case len(d) == 8 && d[0] == 0x10 && d[2] == 0x36: // TransferData first frame: 10 len 36 00 addr32
		s.tdAddr, s.tdLen, s.td = int(binary.BigEndian.Uint32(d[4:8])), int(d[1])-6, nil
		s.reply(0x30, 0x00, 0x00)
	default:
		return false
	}
	return true
}

// consecutive collects a block write; once complete it checks the
// checksum-16 and programs the decoded block.
func (s *fakeZ22) consecutive(d []byte) {
	s.td = append(s.td, d[1:]...)
	if len(s.td) < s.tdLen {
		return
	}
	block := s.td[:s.tdLen]
	s.tdLen = 0
	var sum uint16
	for _, b := range block[:128] {
		sum += uint16(b)
	}
	if len(block) != 130 || sum != binary.BigEndian.Uint16(block[128:130]) {
		s.violations = append(s.violations, fmt.Sprintf("bad block write at %X", s.tdAddr))
		s.reply(0x03, 0x7F, 0x36, 0x77)
		return
	}
	s.writes = append(s.writes, s.tdAddr)
	if !s.dropWrites {
		for i, b := range block[:128] {
			s.flash[s.tdAddr+i] &= b ^ legionKey[i%6] // flash can only clear bits
		}
	}
	s.reply(0x01, 0x76)
}

// read answers a loader read: pci 6 flash, 7 the last md5, F0 the erase
// status (the device on the first poll after an erase, then 1 = done).
func (s *fakeZ22) read(pci byte, addr, n int) {
	if pci == 0xF0 {
		status := byte(1)
		if s.polls == 0 && len(s.erases) > 0 {
			status = 6
		}
		s.polls++
		s.reply(0x10, 0x06, 0x61, 0x00, 0x00, 0x00, 0x00, status)
		return
	}
	var data []byte
	skip := 0
	switch pci {
	case 6:
		data = s.flash[addr : addr+n]
		for a := addr; n == 0x80 && a+n <= len(s.flash) && skip < 0xFF && allFF(s.flash[a:a+n]); a += n {
			skip++
		}
	case 7:
		data = s.md5[:n]
	default:
		s.violations = append(s.violations, fmt.Sprintf("read of pci %02X", pci))
		return
	}
	s.pending = nil
	seq := byte(0x21)
	for i := 4; i < n; i += 7 {
		s.pending = append(s.pending, frame8(append([]byte{seq}, data[i:min(i+7, n)]...)...))
		seq = 0x20 | (seq+1)&0x0F
	}
	s.reply(append([]byte{0x10, byte(n + 2), 0x61, byte(skip)}, data[:4]...)...)
}

func allFF(b []byte) bool {
	for _, v := range b {
		if v != 0xFF {
			return false
		}
	}
	return true
}

func frame8(b ...byte) gocan.Frame {
	out := make([]byte, 8)
	copy(out, b)
	return gocan.NewFrame(0x7E8, out)
}

func (s *fakeZ22) reply(b ...byte) { s.bus.Deliver(frame8(b...)) }

func (s *fakeZ22) with(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
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
		Name:       "Z22SE",
		OnMessage:  func(s string) { r.mu.Lock(); r.msgs = append(r.msgs, s); r.mu.Unlock() },
		OnError:    func(err error) { r.mu.Lock(); r.errs = append(r.errs, err); r.mu.Unlock() },
		OnProgress: func(f float64) { r.mu.Lock(); r.progress = append(r.progress, f); r.mu.Unlock() },
	}
}

func (r *recorder) hasMessage(prefix string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.msgs {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// newClient opens f on a bus closed at the end of the test and returns a
// Z22SE client on it reporting to a recorder. The test fails if the client
// sent anything the fake loader would choke on.
func newClient(t *testing.T, f *fakeZ22) (*Client, *recorder) {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = bus.Close()
		f.with(func() {
			for _, v := range f.violations {
				t.Errorf("loader protocol violation: %s", v)
			}
		})
	})
	rec := new(recorder)
	return New(bus, rec.config()).(*Client), rec
}

// testCtx bounds a client call well past any single exchange.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// flashImage returns a mostly erased 1 MiB image with data in the boot,
// NVDM and application partitions.
func flashImage() []byte {
	b := bytes.Repeat([]byte{0xFF}, 0x100000)
	for i := 0; i < 0x100; i++ {
		b[i] = byte(i)
	}
	for i := 0x4000; i < 0x4080; i++ {
		b[i] = byte(i * 3)
	}
	for i := 0x20000; i < 0x20200; i++ {
		b[i] = byte(i * 7)
	}
	b[0xFFFFF] = 0x5A
	return b
}
