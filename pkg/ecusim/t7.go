package ecusim

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
)

// Values the broadcast frames carry (see decodeT7Broadcast in the datalogger).
const (
	BroadcastRPM   = 1500
	BroadcastPedal = 12
	BroadcastGear  = 3
	BroadcastSpeed = 500 // 0.1 km/h units
)

// T7 is a fake Trionic 7 behind a virtual CAN adapter. It speaks the KWP2000
// dialect the datalogger uses: start/stop communication, security access,
// dynamicallyDefineLocalIdentifier (by symbol number), readDataByLocalIdentifier
// F0, readMemoryByAddress, writeDataByAddress and testerPresent. Everything
// else gets a serviceNotSupported negative response.
//
// All exported fields are read at Open.
type T7 struct {
	SeedKey   t7kwp.SeedKey // security access pair; zero = t7kwp.KnownSeedKeys[0]
	Broadcast time.Duration // period of the 0x1A0/0x280/0x3A0 broadcast frames; 0 = silent
	Latency   time.Duration // reply delay; 0 = reply synchronously inside Send
	Log       func(string)  // optional wire trace

	mu       sync.Mutex
	bus      *gocan.Bus
	symbols  map[int][]byte  // symbol number -> live value bytes
	ram      map[uint32]byte // sparse memory for 0x23 / 0x3D
	defined  []int           // F0 register: symbol numbers in definition order
	req      []byte          // request reassembly buffer
	pending  []gocan.Frame   // response chunks waiting for 0x266 acks
	seed     uint16
	unlocked bool
}

// SetSymbol sets the live value bytes of a symbol number.
func (s *T7) SetSymbol(number int, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.symbols == nil {
		s.symbols = make(map[int][]byte)
	}
	s.symbols[number] = append([]byte(nil), value...)
}

// Peek reads n bytes of the fake memory at addr.
func (s *T7) Peek(addr uint32, n int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, n)
	for i := range out {
		out[i] = s.ram[addr+uint32(i)]
	}
	return out
}

func (s *T7) Open(ctx context.Context, bus *gocan.Bus) error {
	s.bus = bus
	if s.ram == nil {
		s.ram = make(map[uint32]byte)
	}
	if s.SeedKey == (t7kwp.SeedKey{}) {
		s.SeedKey = t7kwp.KnownSeedKeys[0]
	}
	if s.Broadcast > 0 {
		go s.broadcast(ctx)
	}
	return nil
}

func (s *T7) Close() error { return nil }

func (s *T7) broadcast(ctx context.Context) {
	t := time.NewTicker(s.Broadcast)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.bus.Deliver(gocan.NewFrame(0x1A0, []byte{0, BroadcastRPM >> 8, BroadcastRPM & 0xFF, 0, 0, BroadcastPedal, 0, 0}))
			s.bus.Deliver(gocan.NewFrame(0x280, []byte{0, BroadcastGear, 0, 0, 0, 0, 0, 0}))
			s.bus.Deliver(gocan.NewFrame(0x3A0, []byte{0, 0, 0, BroadcastSpeed >> 8, BroadcastSpeed & 0xFF, 0, 0, 0}))
		}
	}
}

func (s *T7) Send(_ context.Context, f gocan.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Log != nil {
		s.Log("<- " + f.String())
	}
	switch f.ID {
	case t7kwp.INIT_MSG_ID:
		if f.Data[1] == t7kwp.START_COM_REQ { // 3F 81 00 11 <req id>: sid sits in byte 1 here
			s.deliver(gocan.NewFrame(t7kwp.INIT_RESP_ID, []byte{0xC0, 0xA1, 0x03, 0xC1, 0xEA, 0x8F, 0x02, 0x58}))
		}
	case t7kwp.REQ_MSG_ID:
		if f.Data[0]&0x40 != 0 {
			s.req = s.req[:0]
		}
		s.req = append(s.req, f.Data[2:8]...)
		if f.Data[0]&0x80 != 0 { // sender wants this chunk confirmed
			s.deliver(gocan.NewFrame(t7kwp.REQ_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, f.Data[0] & 0xBF, 0, 0, 0, 0}))
		}
		if f.Data[0]&0x3F == 0 && len(s.req) > 0 && len(s.req) > int(s.req[0]) {
			msg := s.req[:1+s.req[0]]
			s.req = s.req[:0]
			s.pending = nil
			s.respond(s.handle(msg[1], msg[2:]))
		}
	case t7kwp.RESP_CHUNK_CONF_ID: // tester acked a response chunk, send the next
		if len(s.pending) > 0 {
			next := s.pending[0]
			s.pending = s.pending[1:]
			s.deliver(next)
		}
	}
	return nil
}

// handle answers one KWP request and returns the positive response payload
// (service|0x40 followed by data) or a negative one (7F sid nrc).
func (s *T7) handle(sid byte, p []byte) []byte {
	nrc := func(code byte) []byte { return []byte{0x7F, sid, code} }
	switch sid {
	case t7kwp.STOP_COM_REQ, t7kwp.TESTER_PRESENT:
		return []byte{sid | 0x40}
	case t7kwp.SECURITY_ACCESS:
		level := p[0]
		if level%2 == 1 { // seed request
			s.seed = uint16(rand.Uint32()) | 1
			return []byte{0x67, level, byte(s.seed >> 8), byte(s.seed)}
		}
		key := uint16(p[1])<<8 | uint16(p[2])
		if key != (s.seed<<2^s.SeedKey.XOR)-s.SeedKey.Sub {
			return nrc(t7kwp.INVALID_KEY)
		}
		s.unlocked = true
		return []byte{0x67, level, t7kwp.SECURITY_ACCESS_ALLOWED}
	case t7kwp.DYNAMICALLY_DEFINE_IDENTIFIER:
		if !s.unlocked {
			return nrc(t7kwp.SECURITY_ACCESS_DENIED_OR_REQUESTED)
		}
		if p[0] != 0xF0 {
			return nrc(t7kwp.REQUEST_OUT_OF_RANGE)
		}
		switch {
		case p[1] == t7kwp.DM_CDDLI:
			s.defined = nil
		case p[1] == t7kwp.DM_DBMA && p[4] == 0x80: // define by symbol number
			s.defined = append(s.defined, int(p[5])<<8|int(p[6]))
		default:
			return nrc(t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
		}
		return []byte{0x6C, 0xF0}
	case t7kwp.READ_DATA_BY_IDENTIFIER:
		if p[0] != 0xF0 {
			return nrc(t7kwp.REQUEST_OUT_OF_RANGE)
		}
		out := []byte{0x61, 0xF0}
		for _, n := range s.defined {
			v, ok := s.symbols[n]
			if !ok {
				return nrc(t7kwp.REQUEST_OUT_OF_RANGE)
			}
			out = append(out, v...)
		}
		return out
	case t7kwp.READ_MEMORY_BY_ADDRESS:
		addr := uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
		out := append([]byte{0x63}, p[:3]...)
		for i := range int(p[3]) {
			out = append(out, s.ram[addr+uint32(i)])
		}
		return out
	case t7kwp.WRITE_DATA_BY_ADDRESS:
		addr := uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
		for i, b := range p[4 : 4+int(p[3])] {
			s.ram[addr+uint32(i)] = b
		}
		return append([]byte{0x7D}, p[:3]...)
	}
	return nrc(t7kwp.SERVICE_NOT_SUPPORTED)
}

// respond frames a KWP payload the way the ECU does: the first frame carries
// the length byte and 5 payload bytes, following frames 6 each, byte 0 counting
// down the frames still to come with 0x40 on the first and 0x80 on all. The
// first frame goes out now; the rest wait for the tester's 0x266 acks.
func (s *T7) respond(payload []byte) {
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
		s.pending = append(s.pending, gocan.NewFrame(0x258, chunk))
	}
	first := s.pending[0]
	s.pending = s.pending[1:]
	s.deliver(first)
}

func (s *T7) deliver(f gocan.Frame) { deliver(s.bus, s.Latency, s.Log, f) }
