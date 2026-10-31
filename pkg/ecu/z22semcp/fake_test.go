package z22semcp

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
)

const mcpSize = 0x40100 // MCP flash including the 256 byte shadow

// partitionRange returns the MCP flash range of partition p (1..9): eight
// 32 KiB partitions and the shadow.
func partitionRange(p int) (start, end int) {
	if p == 9 {
		return 0x40000, 0x40100
	}
	return (p - 1) * 0x8000, p * 0x8000
}

// legionKey is the XOR the client applies to block data before
// TransferData, repeating every six bytes.
var legionKey = [6]byte{0x39, 0x68, 0x77, 0x6D, 0x47, 0x39}

// fakeMCP is a Z22SE whose Legion loader is reached on 0x7E0/0x7E8 and
// relays to the MCP once the secondary loader is started (IDemand 4). It
// answers the ping, exit, IDemand (latency, secondary loader, MCP md5),
// the 0x21 reads of the MCP flash (pci 5) with erased-block skipping, the
// md5 read-back (pci 7), the erase command with its status poll and the
// XOR coded TransferData block writes. Stock firmware (legion unset)
// answers nothing. Replies are delivered from inside Send.
type fakeMCP struct {
	mu   sync.Mutex
	bus  *gocan.Bus
	sent []gocan.Frame

	legion       bool
	secondaryNAK bool // refuse to start the secondary loader
	secondary    bool // the secondary loader is running
	flash        []byte
	md5          []byte
	md5Lie       bool
	dropWrites   bool
	exitNRC      byte
	exits        int
	log          []string // "demand cmd:wish", "read pci", "erase mask", "write addr" in order
	erases       []uint16
	polls        int
	violations   []string

	td      []byte
	tdAddr  int
	tdLen   int
	pending []gocan.Frame
}

func newFake(img []byte) *fakeMCP {
	return &fakeMCP{flash: slices.Clone(img)}
}

func (s *fakeMCP) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	return nil
}

func (s *fakeMCP) Close() error { return nil }

func (s *fakeMCP) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, f)
	if f.ID != 0x7E0 || !s.legion {
		return nil
	}
	d := f.Data[:f.Length]
	switch {
	case d[0] == 0x30: // flow control: release the rest of a read
		frames := s.pending
		s.pending = nil
		for _, p := range frames {
			s.bus.Deliver(p)
		}
	case d[0]>>4 == 2 && s.tdLen > 0:
		s.consecutive(d)
	case len(d) == 8 && d[0] == 0xEF && d[1] == 0xBE: // ping
		s.reply(0xDE, 0xAD, 0xF0, 0x0F)
	case len(d) == 2 && d[0] == 0x01 && d[1] == 0x20: // exit
		s.exits++
		if s.exitNRC != 0 {
			s.reply(0x03, 0x7F, 0x20, s.exitNRC)
			return nil
		}
		s.legion = false
		s.reply(0x01, 0x60)
	case len(d) == 8 && d[0] == 0x02 && d[1] == 0xA5:
		s.demand(d[2], int(d[6])<<8|int(d[7]))
	case len(d) == 8 && d[1] == 0x21:
		s.read(d[0], int(binary.BigEndian.Uint32(d[3:7])), int(d[2]))
	case len(d) == 8 && d[1] == 0x34 && d[2] == 0x01:
		s.erase(d)
	case len(d) == 8 && d[0] == 0x10 && d[2] == 0x36: // TransferData first frame: 10 len 36 00 addr32
		s.tdAddr, s.tdLen, s.td = int(binary.BigEndian.Uint32(d[4:8])), int(d[1])-6, nil
		s.reply(0x30, 0x00, 0x00)
	default:
		s.violations = append(s.violations, "unexpected frame "+f.String())
	}
	return nil
}

func (s *fakeMCP) demand(cmd byte, wish int) {
	s.log = append(s.log, fmt.Sprintf("demand %d:%d", cmd, wish))
	status := byte(1)
	switch cmd {
	case 3:
		if !s.secondary {
			s.violations = append(s.violations, "MCP md5 before the secondary loader")
		}
		start, end := 0, mcpSize
		if wish >= 1 && wish <= 9 {
			start, end = partitionRange(wish)
		}
		sum := md5.Sum(s.flash[start:end])
		s.md5 = sum[:]
		if s.md5Lie {
			s.md5[0] ^= 0xFF
		}
	case 4:
		if s.secondaryNAK {
			status = 0
		} else {
			s.secondary = true
		}
	}
	s.reply(0x02, 0xE5, cmd, status)
}

func (s *fakeMCP) erase(d []byte) {
	if d[0] != 5 || d[3] != ^d[5] || d[4] != ^d[6] || !s.secondary {
		s.violations = append(s.violations, fmt.Sprintf("erase request % X", d))
	}
	mask := uint16(d[5])<<8 | uint16(d[6])
	s.log = append(s.log, fmt.Sprintf("erase %04X", mask))
	s.erases = append(s.erases, mask)
	s.polls = 0
	for p := 1; p <= 9; p++ {
		if mask&(1<<(p-1)) != 0 {
			start, end := partitionRange(p)
			copy(s.flash[start:end], bytes.Repeat([]byte{0xFF}, end-start))
		}
	}
}

func (s *fakeMCP) consecutive(d []byte) {
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
	if len(block) != 130 || sum != binary.BigEndian.Uint16(block[128:130]) || s.tdAddr+128 > mcpSize {
		s.violations = append(s.violations, fmt.Sprintf("bad block write at %X", s.tdAddr))
		s.reply(0x03, 0x7F, 0x36, 0x77)
		return
	}
	s.log = append(s.log, fmt.Sprintf("write %X", s.tdAddr))
	if !s.dropWrites {
		for i, b := range block[:128] {
			s.flash[s.tdAddr+i] &= b ^ legionKey[i%6]
		}
	}
	s.reply(0x01, 0x76)
}

func (s *fakeMCP) read(pci byte, addr, n int) {
	if pci == 0xF0 {
		status := byte(1)
		if s.polls == 0 && len(s.erases) > 0 {
			status = 5
		}
		s.polls++
		s.reply(0x10, 0x06, 0x61, 0x00, 0x00, 0x00, 0x00, status)
		return
	}
	var data []byte
	skip := 0
	switch pci {
	case 5:
		if !s.secondary {
			s.violations = append(s.violations, "MCP read before the secondary loader")
		}
		s.log = append(s.log, "read 5")
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

func (s *fakeMCP) reply(b ...byte) { s.bus.Deliver(frame8(b...)) }

func (s *fakeMCP) with(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// entries returns the log lines starting with prefix.
func (s *fakeMCP) entries(prefix string) []string {
	var out []string
	s.with(func() {
		for _, l := range s.log {
			if strings.HasPrefix(l, prefix) {
				out = append(out, l)
			}
		}
	})
	return out
}

// recorder collects what a client reports through its ecu.Config.
type recorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *recorder) config() *ecu.Config {
	return &ecu.Config{
		Name:       "Z22SE MCP",
		OnMessage:  func(s string) { r.mu.Lock(); r.msgs = append(r.msgs, s); r.mu.Unlock() },
		OnError:    func(error) {},
		OnProgress: func(float64) {},
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

// newClient opens f on a bus closed at the end of the test and returns an
// MCP client on it. The test fails if the client sent anything the fake
// loader would choke on.
func newClient(t *testing.T, f *fakeMCP) (*Client, *recorder) {
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

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// mcpImage returns a mostly erased MCP image with data in partitions 1
// and 3 and in the shadow.
func mcpImage() []byte {
	b := bytes.Repeat([]byte{0xFF}, mcpSize)
	for i := 0; i < 0x100; i++ {
		b[i] = byte(i)
	}
	for i := 0x10000; i < 0x10100; i++ {
		b[i] = byte(i * 5)
	}
	for i := 0x40000; i < 0x40010; i++ {
		b[i] = byte(i)
	}
	return b
}
