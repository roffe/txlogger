package ecusim

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
)

// kwpChunks splits a KWP message (length byte first) into 0x240 payloads
// the way the tester does: six bytes each after an 0xA1 marker, byte 0
// counting down the chunks still to come, 0x40 on the first chunk and 0x80
// on every chunk that wants a confirmation (all but the last).
func kwpChunks(msg []byte) [][]byte {
	n := (len(msg) + 5) / 6
	var out [][]byte
	for i := range n {
		b0 := byte(n - 1 - i)
		if i == 0 {
			b0 |= 0x40
		}
		if i != n-1 {
			b0 |= 0x80
		}
		out = append(out, append([]byte{b0, 0xA1}, msg[i*6:min(i*6+6, len(msg))]...))
	}
	return out
}

// kwp sends a KWP message to the sim, acknowledging every response chunk
// on 0x266, and returns the response payload without its length byte.
func (h *harness) kwp(msg ...byte) []byte {
	h.t.Helper()
	chunks := kwpChunks(msg)
	var resp []gocan.Frame
	for i, c := range chunks {
		resp = h.send(t7kwp.REQ_MSG_ID, c...)
		if i < len(chunks)-1 {
			assertFrames(h.t, resp, fr(t7kwp.REQ_CHUNK_CONF_ID, 0x40, 0xA1, 0x3F, c[0]&0xBF))
		}
	}
	if len(resp) != 1 || resp[0].ID != 0x258 {
		h.t.Fatalf("response to % X: %v", msg, resp)
	}
	f := resp[0]
	raw := append([]byte(nil), f.Data[2:]...)
	for f.Data[0]&0x3F != 0 {
		next := h.send(t7kwp.RESP_CHUNK_CONF_ID, 0x40, 0xA1, 0x3F, f.Data[0]&0xBF)
		if len(next) != 1 || next[0].ID != 0x258 {
			h.t.Fatalf("ack of %02X answered with %v", f.Data[0], next)
		}
		f = next[0]
		raw = append(raw, f.Data[2:]...)
	}
	if int(raw[0]) >= len(raw) {
		h.t.Fatalf("response length %d exceeds the %d bytes received", raw[0], len(raw)-1)
	}
	return raw[1 : 1+raw[0]]
}

func assertBytes(t *testing.T, what string, got []byte, want ...byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s = % X, want % X", what, got, want)
	}
}

// unlockT7 unlocks the sim at level 5 through the gocan T7 client, whose
// key calculation is independent of the sim's, then drops the exchange
// from the harness so the next send sees only its own replies.
func unlockT7(t *testing.T, h *harness, sk t7kwp.SeedKey) {
	t.Helper()
	c := t7kwp.New(h.bus)
	c.SetResponseID(0x258)
	if ok, err := c.SecurityAccess(testCtx(t), 0x05, sk); err != nil || !ok {
		t.Fatalf("SecurityAccess = %v, %v", ok, err)
	}
	h.drain()
}

// startCommunication on 0x220 is answered on 0x238 with the fixed reply
// naming 0x258 as the response identifier; other 0x220 frames are ignored.
func TestT7StartCommunication(t *testing.T) {
	h := newHarness(t, &T7{})
	got := h.send(t7kwp.INIT_MSG_ID, 0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00)
	assertFrames(t, got, fr(t7kwp.INIT_RESP_ID, 0xC0, 0xA1, 0x03, 0xC1, 0xEA, 0x8F, 0x02, 0x58))
	if got := h.send(t7kwp.INIT_MSG_ID, 0x3F, 0x82, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00); len(got) != 0 {
		t.Fatalf("stop on 0x220 answered with %v", got)
	}
}

// A response goes out as 0x258 chunks: the first carries the length byte
// and five payload bytes, later ones six, byte 0 counts down the chunks
// still to come with 0x40 on the first and 0x80 on all. Only the first is
// sent unprompted; each 0x266 ack releases the next one.
func TestT7ResponseFraming(t *testing.T) {
	s := &T7{}
	h := newHarness(t, s)
	data := []byte{0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9}
	unlockT7(t, h, t7kwp.KnownSeedKeys[0])
	h.kwp(append([]byte{0x0F, 0x3D, 0x01, 0x20, 0x00, 10}, data...)...)

	got := h.send(t7kwp.REQ_MSG_ID, 0x40, 0xA1, 0x05, 0x23, 0x01, 0x20, 0x00, 10)
	assertFrames(t, got, fr(0x258, 0xC2, 0xA1, 0x0E, 0x63, 0x01, 0x20, 0x00, 0xD0))
	ack := func() []gocan.Frame { return h.send(t7kwp.RESP_CHUNK_CONF_ID, 0x40, 0xA1, 0x3F, 0x82) }
	assertFrames(t, ack(), fr(0x258, 0x81, 0xA1, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6))
	assertFrames(t, ack(), fr(0x258, 0x80, 0xA1, 0xD7, 0xD8, 0xD9))
	if got := ack(); len(got) != 0 {
		t.Fatalf("ack after the last chunk answered with %v", got)
	}
}

// A request split over several 0x240 chunks has every chunk flagged 0x80
// confirmed on 0x270 and is answered once the last chunk is in.
func TestT7ChunkedRequest(t *testing.T) {
	s := &T7{}
	h := newHarness(t, s)
	got := h.send(t7kwp.REQ_MSG_ID, 0xC1, 0xA1, 0x08, 0x3D, 0x00, 0x10, 0x00, 0x04)
	assertFrames(t, got, fr(t7kwp.REQ_CHUNK_CONF_ID, 0x40, 0xA1, 0x3F, 0x81))
	got = h.send(t7kwp.REQ_MSG_ID, 0x00, 0xA1, 0xDE, 0xAD, 0xBE, 0xEF)
	assertFrames(t, got, fr(0x258, 0xC0, 0xA1, 0x04, 0x7D, 0x00, 0x10, 0x00))
	assertBytes(t, "memory", s.Peek(0x1000, 4), 0xDE, 0xAD, 0xBE, 0xEF)
}

// A new first chunk (0x40) discards a half received request.
func TestT7RequestRestart(t *testing.T) {
	h := newHarness(t, &T7{})
	h.send(t7kwp.REQ_MSG_ID, 0xC1, 0xA1, 0x08, 0x3D, 0x00, 0x10, 0x00, 0x04)
	assertBytes(t, "tester present", h.kwp(0x01, 0x3E), 0x7E)
}

// writeDataByAddress (3D) and readMemoryByAddress (23) work on the sparse
// memory, both echoing the 24-bit address.
func TestT7Memory(t *testing.T) {
	s := &T7{}
	h := newHarness(t, s)
	assertBytes(t, "write", h.kwp(0x07, 0x3D, 0x0F, 0x12, 0x34, 0x02, 0xCA, 0xFE), 0x7D, 0x0F, 0x12, 0x34)
	assertBytes(t, "Peek", s.Peek(0x0F1234, 3), 0xCA, 0xFE, 0x00)
	assertBytes(t, "read", h.kwp(0x05, 0x23, 0x0F, 0x12, 0x33, 0x04), 0x63, 0x0F, 0x12, 0x33, 0x00, 0xCA, 0xFE, 0x00)
}

// securityAccess hands out an odd seed and grants access (67 06 34) only
// to the key the gocan T7 client derives with the sim's pair, the first
// known pair unless one is configured; any other key gets invalidKey
// (7F 27 35) and leaves the sim locked.
func TestT7SecurityAccess(t *testing.T) {
	custom := t7kwp.SeedKey{XOR: 0x1234, Sub: 0x0101}
	tests := []struct {
		name string
		sim  t7kwp.SeedKey
		key  t7kwp.SeedKey
		ok   bool
	}{
		{"default pair", t7kwp.SeedKey{}, t7kwp.KnownSeedKeys[0], true},
		{"default pair rejects the older pair", t7kwp.SeedKey{}, t7kwp.KnownSeedKeys[1], false},
		{"custom pair", custom, custom, true},
		{"custom pair rejects the default", custom, t7kwp.KnownSeedKeys[0], false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &T7{SeedKey: tc.sim}
			h := newHarness(t, s)
			seed := h.kwp(0x02, 0x27, 0x05)
			if len(seed) != 4 || seed[0] != 0x67 || seed[1] != 0x05 || seed[3]&1 == 0 {
				t.Fatalf("seed response % X", seed)
			}

			c := t7kwp.New(h.bus)
			c.SetResponseID(0x258)
			ok, err := c.SecurityAccess(testCtx(t), 0x05, tc.key)
			want := fr(0x258, 0xC0, 0xA1, 0x03, 0x7F, 0x27, t7kwp.INVALID_KEY)
			if tc.ok {
				want = fr(0x258, 0xC0, 0xA1, 0x03, 0x67, 0x06, t7kwp.SECURITY_ACCESS_ALLOWED)
				if err != nil || !ok {
					t.Fatalf("SecurityAccess = %v, %v", ok, err)
				}
			} else if ok || !errors.Is(err, t7kwp.ErrInvalidKey) {
				t.Fatalf("SecurityAccess = %v, %v; want an invalid key error", ok, err)
			}
			// the client's seed request and key, each answered by one frame
			if got := h.drain(); len(got) != 2 || got[1] != want {
				t.Fatalf("replies %v, want the key answered with %s", got, want)
			}
			if s.unlocked != tc.ok {
				t.Fatalf("unlocked = %v", s.unlocked)
			}
		})
	}
}

// The F0 register needs security access; it is cleared with DM_CDDLI,
// filled by symbol number and read back with 21 F0 as the live symbol
// values in definition order.
func TestT7DynamicRegister(t *testing.T) {
	s := &T7{}
	h := newHarness(t, s)
	s.SetSymbol(0x0102, []byte{0x11, 0x22})
	s.SetSymbol(7, []byte{0x33})

	assertBytes(t, "locked", h.kwp(0x04, 0x2C, 0xF0, 0x04, 0x00), 0x7F, 0x2C, t7kwp.SECURITY_ACCESS_DENIED_OR_REQUESTED)
	unlockT7(t, h, t7kwp.KnownSeedKeys[0])

	assertBytes(t, "clear", h.kwp(0x04, 0x2C, 0xF0, 0x04, 0x00), 0x6C, 0xF0)
	assertBytes(t, "empty read", h.kwp(0x02, 0x21, 0xF0), 0x61, 0xF0)
	assertBytes(t, "define 7", h.kwp(0x08, 0x2C, 0xF0, 0x03, 0x00, 0x00, 0x80, 0x00, 0x07), 0x6C, 0xF0)
	assertBytes(t, "define 0102", h.kwp(0x08, 0x2C, 0xF0, 0x03, 0x01, 0x00, 0x80, 0x01, 0x02), 0x6C, 0xF0)
	assertBytes(t, "read", h.kwp(0x02, 0x21, 0xF0), 0x61, 0xF0, 0x33, 0x11, 0x22)

	s.SetSymbol(7, []byte{0x44, 0x55}) // live values: the next read sees the change
	assertBytes(t, "read after change", h.kwp(0x02, 0x21, 0xF0), 0x61, 0xF0, 0x44, 0x55, 0x11, 0x22)

	assertBytes(t, "clear again", h.kwp(0x04, 0x2C, 0xF0, 0x04, 0x00), 0x6C, 0xF0)
	assertBytes(t, "read after clear", h.kwp(0x02, 0x21, 0xF0), 0x61, 0xF0)
}

// Requests the sim cannot answer get the matching negative response.
func TestT7NegativeResponses(t *testing.T) {
	s := &T7{}
	h := newHarness(t, s)
	unlockT7(t, h, t7kwp.KnownSeedKeys[0])
	tests := []struct {
		name string
		msg  []byte
		want []byte
	}{
		{"define another register", []byte{0x04, 0x2C, 0xF1, 0x04, 0x00}, []byte{0x7F, 0x2C, t7kwp.REQUEST_OUT_OF_RANGE}},
		{"define by address", []byte{0x08, 0x2C, 0xF0, 0x03, 0x00, 0x02, 0x0F, 0x12, 0x34}, []byte{0x7F, 0x2C, t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT}},
		{"read another identifier", []byte{0x02, 0x21, 0x90}, []byte{0x7F, 0x21, t7kwp.REQUEST_OUT_OF_RANGE}},
		{"unsupported service", []byte{0x02, 0x11, 0x01}, []byte{0x7F, 0x11, t7kwp.SERVICE_NOT_SUPPORTED}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertBytes(t, "response", h.kwp(tc.msg...), tc.want...)
		})
	}

	// a defined symbol the sim has no value for
	h.kwp(0x08, 0x2C, 0xF0, 0x03, 0x00, 0x00, 0x80, 0x00, 0x09)
	assertBytes(t, "read unknown symbol", h.kwp(0x02, 0x21, 0xF0), 0x7F, 0x21, t7kwp.REQUEST_OUT_OF_RANGE)
}

// stopCommunication and testerPresent get their positive responses.
func TestT7SessionServices(t *testing.T) {
	h := newHarness(t, &T7{})
	assertBytes(t, "stop", h.kwp(0x01, t7kwp.STOP_COM_REQ), 0xC2)
	assertBytes(t, "tester present", h.kwp(0x01, t7kwp.TESTER_PRESENT), 0x7E)
}

// The gocan T7 KWP client runs a session against the sim: start, security
// access, memory write and read, the F0 register and stop.
func TestT7Client(t *testing.T) {
	s := &T7{}
	bus := newHarness(t, s).bus
	s.SetSymbol(42, []byte{0x12, 0x34, 0x56, 0x78})
	ctx := testCtx(t)
	c := t7kwp.New(bus)
	if err := c.StartSession(ctx, t7kwp.INIT_MSG_ID, t7kwp.INIT_RESP_ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.SecurityAccess(ctx, 0x05, t7kwp.KnownSeedKeys[0]); err != nil || !ok {
		t.Fatalf("SecurityAccess = %v, %v", ok, err)
	}
	data := bytes.Repeat([]byte{0xA5, 0x5A, 0x01}, 10)
	if err := c.WriteDataByAddress(ctx, 0x00F000, data); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadMemoryByAddress(ctx, 0x00F000, len(data))
	if err != nil {
		t.Fatal(err)
	}
	assertBytes(t, "ReadMemoryByAddress", got, data...)
	if err := c.ClearDynamicallyDefineLocalId(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.DynamicallyDefineLocalIdBySymbolNumber(ctx, 0, 42); err != nil {
		t.Fatal(err)
	}
	got, err = c.ReadDataByIdentifier(ctx, 0xF0)
	if err != nil {
		t.Fatal(err)
	}
	assertBytes(t, "ReadDataByIdentifier(F0)", got, 0x12, 0x34, 0x56, 0x78)
	if err := c.TesterPresent(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
}

// With Broadcast set the sim sends the 0x1A0 (rpm, pedal), 0x280 (gear)
// and 0x3A0 (speed) frames carrying the Broadcast* values.
func TestT7Broadcast(t *testing.T) {
	h := newHarness(t, &T7{Broadcast: time.Millisecond})
	ctx := testCtx(t)
	want := map[uint32]gocan.Frame{
		0x1A0: fr(0x1A0, 0, BroadcastRPM>>8, BroadcastRPM&0xFF, 0, 0, BroadcastPedal, 0, 0),
		0x280: fr(0x280, 0, BroadcastGear),
		0x3A0: fr(0x3A0, 0, 0, 0, BroadcastSpeed>>8, BroadcastSpeed&0xFF),
	}
	seen := map[uint32]bool{}
	for len(seen) < len(want) {
		select {
		case f := <-h.rx:
			w, ok := want[f.ID]
			if !ok || f != w {
				t.Fatalf("broadcast %s, want one of %v", f, want)
			}
			seen[f.ID] = true
		case <-ctx.Done():
			t.Fatalf("saw only %v", seen)
		}
	}
}

// Replies with Latency set arrive from another goroutine after the delay.
func TestT7Latency(t *testing.T) {
	h := newHarness(t, &T7{Latency: 3 * time.Millisecond})
	start := time.Now()
	if err := h.bus.Send(context.Background(), gocan.NewFrame(t7kwp.INIT_MSG_ID, []byte{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40})); err != nil {
		t.Fatal(err)
	}
	select {
	case f := <-h.rx:
		if f.ID != t7kwp.INIT_RESP_ID {
			t.Fatalf("got %s", f)
		}
		if el := time.Since(start); el < 3*time.Millisecond {
			t.Fatalf("reply after %v", el)
		}
	case <-testCtx(t).Done():
		t.Fatal("no reply")
	}
}
