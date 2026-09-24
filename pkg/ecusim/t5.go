package ecusim

import (
	"context"
	"encoding/binary"
	"strconv"
	"sync"
	"time"

	"github.com/roffe/gocan/v2"
)

// T5 is a fake Trionic 5 behind a virtual CAN adapter, speaking the raw
// command frames on 0x05 answered on 0x0C: C7 six-byte RAM reads, C4 ASCII
// console characters (the W<addr><byte> write command), A5 set-address plus
// offset-prefixed data frames for block writes, and C1 call-address, which
// emulates the datalogger's gather stub when pointed at it.
type T5 struct {
	Latency time.Duration // reply delay; 0 = reply synchronously inside Send
	Log     func(string)  // optional wire trace

	mu      sync.Mutex
	bus     *gocan.Bus
	ram     map[uint32]byte
	cmd     []byte // ASCII console line being typed via C4
	winAddr uint32 // block write window set by A5
	winLen  int
}

// Gather stub layout, mirrored from the T5 fast logger.
const (
	t5StubAddr  = 0x7700
	t5TableAddr = 0x7740
	t5BufAddr   = 0x7800
)

// Poke writes bytes into the fake memory at addr.
func (s *T5) Poke(addr uint32, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ram == nil {
		s.ram = make(map[uint32]byte)
	}
	for i, v := range b {
		s.ram[addr+uint32(i)] = v
	}
}

// Peek reads n bytes of the fake memory at addr.
func (s *T5) Peek(addr uint32, n int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peek(addr, n)
}

func (s *T5) peek(addr uint32, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = s.ram[addr+uint32(i)]
	}
	return out
}

func (s *T5) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	if s.ram == nil {
		s.ram = make(map[uint32]byte)
	}
	return nil
}

func (s *T5) Close() error { return nil }

func (s *T5) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Log != nil {
		s.Log("<- " + f.String())
	}
	if f.ID != 0x05 {
		return nil
	}
	d := f.Data
	switch d[0] {
	case 0xC7: // read the six bytes ending at the address, highest first
		addr := uint32(d[3])<<8 | uint32(d[4])
		out := []byte{0xC7, 0x00}
		for i := range uint32(6) {
			out = append(out, s.ram[addr-i])
		}
		s.reply(out)
	case 0xC4: // one console character
		s.cmd = append(s.cmd, d[1])
		if d[1] == '\r' {
			s.console(string(s.cmd[:len(s.cmd)-1]))
			s.cmd = s.cmd[:0]
		}
		s.reply([]byte{0xC6, 0x00})
	case 0xA5: // set block write window (length 0 arms the stream call)
		s.winAddr, s.winLen = binary.BigEndian.Uint32(d[1:5]), int(d[5])
		s.reply([]byte{0xA5, 0x00})
	case 0xC1: // call address: no reply
		if binary.BigEndian.Uint32(d[1:5]) == t5StubAddr {
			s.gather()
		}
	default: // block data: offset then up to 7 bytes
		if s.winLen == 0 {
			return nil
		}
		off := int(d[0])
		for i, b := range d[1 : 1+min(7, s.winLen-off)] {
			s.ram[s.winAddr+uint32(off+i)] = b
		}
		s.reply([]byte{d[0], 0x00})
	}
	return nil
}

// console runs one ASCII command line; only W<addr:4><byte:2> is understood.
func (s *T5) console(line string) {
	if len(line) != 7 || line[0] != 'W' {
		return
	}
	addr, err1 := strconv.ParseUint(line[1:5], 16, 16)
	val, err2 := strconv.ParseUint(line[5:7], 16, 8)
	if err1 == nil && err2 == nil {
		s.ram[uint32(addr)] = byte(val)
	}
}

// gather emulates the CPU32 stub uploaded by the fast logger: walk the
// descriptor table {count, pad, N*{addr.w, len.b, pad.b}} and pack the listed
// bytes contiguously into the output buffer.
func (s *T5) gather() {
	count := int(s.ram[t5TableAddr])
	out := uint32(t5BufAddr)
	for i := range count {
		e := uint32(t5TableAddr + 2 + 4*i)
		addr := uint32(s.ram[e])<<8 | uint32(s.ram[e+1])
		for _, b := range s.peek(addr, int(s.ram[e+2])) {
			s.ram[out] = b
			out++
		}
	}
}

func (s *T5) reply(b []byte) { deliver(s.bus, s.Latency, s.Log, gocan.NewFrame(0x0C, pad8(b))) }
