package ecusim

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

// T8 is a fake Trionic 8 behind a virtual CAN adapter: GMLAN over ISO-TP on
// 0x7E0/0x7E8. It answers what the datalogger uses: initiateDiagnosticOperation,
// securityAccess (level FD, checked with t8sec), writeDataByIdentifier 0x17
// (dynamic register clear / define by symbol number / define by address) and
// 0x15 (write by address), readDataByIdentifier 0x18, readMemoryByAddress,
// returnToNormalMode and testerPresent. Anything else gets serviceNotSupported.
type T8 struct {
	Latency time.Duration // reply delay; 0 = reply synchronously inside Send
	Log     func(string)  // optional wire trace

	mu       sync.Mutex
	bus      *gocan.Bus
	symbols  map[int][]byte
	ram      map[uint32]byte
	defined  []func() []byte // 0x18 register entries, evaluated at read time
	req      []byte          // ISO-TP request reassembly
	reqLen   int
	bs       byte          // block size we announced for the request in flight
	pending  []gocan.Frame // response consecutive frames waiting for the tester's flow control
	seed     uint16
	unlocked bool
}

// SetSymbol sets the live value bytes of a symbol number.
func (s *T8) SetSymbol(number int, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.symbols == nil {
		s.symbols = make(map[int][]byte)
	}
	s.symbols[number] = append([]byte(nil), value...)
}

// Peek reads n bytes of the fake memory at addr.
func (s *T8) Peek(addr uint32, n int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, n)
	for i := range out {
		out[i] = s.ram[addr+uint32(i)]
	}
	return out
}

func (s *T8) Open(_ context.Context, bus *gocan.Bus) error {
	s.bus = bus
	if s.ram == nil {
		s.ram = make(map[uint32]byte)
	}
	return nil
}

func (s *T8) Close() error { return nil }

func (s *T8) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Log != nil {
		s.Log("<- " + f.String())
	}
	if f.ID != 0x7E0 { // tester present goes to 0x101 and wants no answer
		return nil
	}
	d := f.Data
	switch d[0] >> 4 {
	case 0: // single frame
		s.reply(s.handle(d[1 : 1+int(d[0])]))
	case 1: // first frame: announce our block size and wait for the rest
		s.reqLen = int(d[0]&0x0F)<<8 | int(d[1])
		s.req = append(s.req[:0], d[2:8]...)
		s.bs = 0
		if s.req[0] == gmlan.WRITE_DATA_BY_IDENTIFIER && s.req[1] == 0x15 {
			s.bs = 1 // the T8 acks every frame of a write-by-address
		}
		s.deliver(gocan.NewFrame(0x7E8, pad8([]byte{0x30, s.bs, 0x00})))
	case 2: // consecutive frame
		s.req = append(s.req, d[1:8]...)
		if len(s.req) >= s.reqLen {
			s.reply(s.handle(s.req[:s.reqLen]))
		} else if s.bs == 1 {
			s.deliver(gocan.NewFrame(0x7E8, pad8([]byte{0x30, 0x01, 0x00})))
		}
	case 3: // flow control from the tester: release our consecutive frames
		s.deliver(s.pending...)
		s.pending = nil
	}
	return nil
}

// handle answers one GMLAN request and returns the response payload, either
// positive (service|0x40 ...) or negative (7F sid nrc).
func (s *T8) handle(msg []byte) []byte {
	sid, p := msg[0], msg[1:]
	nrc := func(code byte) []byte { return []byte{0x7F, sid, code} }
	switch sid {
	case gmlan.INITIATE_DIAGNOSTIC_OPERATION:
		return []byte{0x50}
	case gmlan.RETURN_TO_NORMAL_MODE, 0x3E:
		return []byte{sid | 0x40}
	case gmlan.SECURITY_ACCESS:
		level := p[0]
		if level%2 == 1 { // seed request
			s.seed = uint16(rand.Uint32()) | 1
			return []byte{0x67, level, byte(s.seed >> 8), byte(s.seed)}
		}
		hi, lo := t8sec.CalculateAccessKey([]byte{byte(s.seed >> 8), byte(s.seed)}, level-1)
		if p[1] != hi || p[2] != lo {
			return nrc(0x35)
		}
		s.unlocked = true
		return []byte{0x67, level}
	case gmlan.WRITE_DATA_BY_IDENTIFIER:
		switch p[0] {
		case 0x17: // dynamic register
			if !s.unlocked {
				return nrc(0x33)
			}
			if p[1] != 0xF0 {
				return nrc(0x31)
			}
			switch p[2] {
			case 0x04:
				s.defined = nil
			case 0x80: // by symbol number
				n := int(p[6])<<8 | int(p[7])
				s.defined = append(s.defined, func() []byte { return s.symbols[n] })
			case 0x03: // by memory address
				addr, length := uint32(p[5])<<16|uint32(p[6])<<8|uint32(p[7]), int(p[4])
				s.defined = append(s.defined, func() []byte { return s.peek(addr, length) })
			default:
				return nrc(0x12)
			}
			return []byte{0x7B, 0x17}
		case 0x15: // write by address
			addr := uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
			for i, b := range p[5 : 5+int(p[4])] {
				s.ram[addr+uint32(i)] = b
			}
			return []byte{0x7B, 0x15}
		}
		return nrc(0x31)
	case gmlan.READ_DATA_BY_IDENTIFIER:
		if p[0] != 0x18 {
			return nrc(0x31)
		}
		out := []byte{0x5A, 0x18}
		for _, get := range s.defined {
			out = append(out, get()...)
		}
		return out
	case gmlan.READ_MEMORY_BY_ADDRESS:
		addr := uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
		return append(append([]byte{0x63}, p[:3]...), s.peek(addr, int(p[3])<<8|int(p[4]))...)
	}
	return nrc(0x11)
}

func (s *T8) peek(addr uint32, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = s.ram[addr+uint32(i)]
	}
	return out
}

// reply frames a payload as ISO-TP: a single frame when it fits, otherwise a
// first frame now and consecutive frames once the tester sends flow control.
func (s *T8) reply(payload []byte) {
	if len(payload) <= 7 {
		s.deliver(gocan.NewFrame(0x7E8, pad8(append([]byte{byte(len(payload))}, payload...))))
		return
	}
	s.pending = nil
	seq := byte(0x21)
	for pos := 6; pos < len(payload); pos += 7 {
		s.pending = append(s.pending, gocan.NewFrame(0x7E8, pad8(append([]byte{seq}, payload[pos:min(pos+7, len(payload))]...))))
		seq = 0x20 | (seq+1)&0x0F
	}
	s.deliver(gocan.NewFrame(0x7E8, pad8(append([]byte{0x10 | byte(len(payload)>>8), byte(len(payload))}, payload[:6]...))))
}

func (s *T8) deliver(frames ...gocan.Frame) { deliver(s.bus, s.Latency, s.Log, frames...) }
