package ecusim

import (
	"bytes"
	"testing"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/ecu/t8sec"
)

// gmlan sends a GMLAN request to the T8 sim as ISO-TP (a single frame, or
// a first frame plus consecutive frames paced by the sim's flow control)
// and returns the response payload, sending flow control for a multi-frame
// response.
func (h *harness) gmlan(msg ...byte) []byte {
	h.t.Helper()
	var resp []gocan.Frame
	if len(msg) <= 7 {
		resp = h.send(0x7E0, append([]byte{byte(len(msg))}, msg...)...)
	} else {
		fc := h.send(0x7E0, append([]byte{0x10 | byte(len(msg)>>8), byte(len(msg))}, msg[:6]...)...)
		if len(fc) != 1 || fc[0].Data[0] != 0x30 {
			h.t.Fatalf("first frame answered with %v", fc)
		}
		bs := fc[0].Data[1]
		seq := byte(0x21)
		for pos := 6; pos < len(msg); pos += 7 {
			resp = h.send(0x7E0, append([]byte{seq}, msg[pos:min(pos+7, len(msg))]...)...)
			seq = 0x20 | (seq+1)&0x0F
			if pos+7 < len(msg) {
				if bs == 1 {
					assertFrames(h.t, resp, fr(0x7E8, 0x30, 0x01, 0x00))
				} else if len(resp) != 0 {
					h.t.Fatalf("consecutive frame answered with %v", resp)
				}
			}
		}
	}
	if len(resp) != 1 || resp[0].ID != 0x7E8 {
		h.t.Fatalf("response to % X: %v", msg, resp)
	}
	d := resp[0].Data
	if d[0]>>4 == 0 {
		return append([]byte(nil), d[1:1+d[0]]...)
	}
	if d[0]>>4 != 1 {
		h.t.Fatalf("response starts with %s", resp[0])
	}
	total := int(d[0]&0x0F)<<8 | int(d[1])
	out := append([]byte(nil), d[2:]...)
	cfs := h.send(0x7E0, 0x30, 0x00, 0x00)
	seq := byte(0x21)
	for _, cf := range cfs {
		if cf.Data[0] != seq {
			h.t.Fatalf("consecutive frame %s, want sequence %02X", cf, seq)
		}
		out = append(out, cf.Data[1:]...)
		seq = 0x20 | (seq+1)&0x0F
	}
	if len(out) < total {
		h.t.Fatalf("got %d of %d response bytes", len(out), total)
	}
	return out[:total]
}

// unlockT8 runs securityAccess level FD with the T8 algorithm.
func unlockT8(t *testing.T, h *harness) {
	t.Helper()
	seed := h.gmlan(0x27, 0xFD)
	hi, lo := t8sec.CalculateAccessKey(seed[2:4], 0xFD)
	assertBytes(t, "send key", h.gmlan(0x27, 0xFE, hi, lo), 0x67, 0xFE)
}

// The session services answer positively and anything the sim does not
// implement gets the matching negative response.
func TestT8Services(t *testing.T) {
	h := newHarness(t, &T8{})
	tests := []struct {
		name string
		msg  []byte
		want []byte
	}{
		{"initiateDiagnosticOperation", []byte{0x10, 0x02}, []byte{0x50}},
		{"returnToNormalMode", []byte{0x20}, []byte{0x60}},
		{"testerPresent", []byte{0x3E}, []byte{0x7E}},
		{"register locked", []byte{0x3B, 0x17, 0xF0, 0x04}, []byte{0x7F, 0x3B, 0x33}},
		{"unknown write identifier", []byte{0x3B, 0x99, 0x01}, []byte{0x7F, 0x3B, 0x31}},
		{"read another identifier", []byte{0x1A, 0x90}, []byte{0x7F, 0x1A, 0x31}},
		{"unsupported service", []byte{0x11, 0x01}, []byte{0x7F, 0x11, 0x11}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertBytes(t, "response", h.gmlan(tc.msg...), tc.want...)
		})
	}
}

// securityAccess hands out an odd seed and accepts only the t8sec key for
// it; a wrong key gets invalidKey and leaves the ECU locked.
func TestT8SecurityAccess(t *testing.T) {
	for _, good := range []bool{true, false} {
		s := &T8{}
		h := newHarness(t, s)
		seed := h.gmlan(0x27, 0xFD)
		if len(seed) != 4 || seed[0] != 0x67 || seed[1] != 0xFD || seed[3]&1 == 0 {
			t.Fatalf("seed response % X", seed)
		}
		hi, lo := t8sec.CalculateAccessKey(seed[2:4], 0xFD)
		if !good {
			lo++
		}
		got := h.gmlan(0x27, 0xFE, hi, lo)
		if good {
			assertBytes(t, "key response", got, 0x67, 0xFE)
		} else {
			assertBytes(t, "key response", got, 0x7F, 0x27, 0x35)
		}
		if s.unlocked != good {
			t.Fatalf("good key %v: unlocked = %v", good, s.unlocked)
		}
	}
}

// The 0x17 register is cleared with 04, filled by symbol number (80) and
// by memory address (03), and read with 1A 18 as the entries evaluated at
// read time, in definition order.
func TestT8DynamicRegister(t *testing.T) {
	s := &T8{}
	h := newHarness(t, s)
	unlockT8(t, h)
	s.SetSymbol(0x0102, []byte{0x11, 0x22})
	assertBytes(t, "write memory", h.gmlan(0x3B, 0x15, 0x00, 0x10, 0x00, 0x04, 0xA1, 0xA2, 0xA3, 0xA4), 0x7B, 0x15)

	assertBytes(t, "clear", h.gmlan(0x3B, 0x17, 0xF0, 0x04), 0x7B, 0x17)
	assertBytes(t, "by symbol", h.gmlan(0x3B, 0x17, 0xF0, 0x80, 0x00, 0x00, 0x00, 0x01, 0x02), 0x7B, 0x17)
	assertBytes(t, "by address", h.gmlan(0x3B, 0x17, 0xF0, 0x03, 0x00, 0x04, 0x00, 0x10, 0x00), 0x7B, 0x17)
	assertBytes(t, "read", h.gmlan(0x1A, 0x18), 0x5A, 0x18, 0x11, 0x22, 0xA1, 0xA2, 0xA3, 0xA4)

	s.SetSymbol(0x0102, []byte{0x33})
	h.gmlan(0x3B, 0x15, 0x00, 0x10, 0x00, 0x01, 0xB1)
	assertBytes(t, "read after change", h.gmlan(0x1A, 0x18), 0x5A, 0x18, 0x33, 0xB1, 0xA2, 0xA3, 0xA4)

	assertBytes(t, "another register", h.gmlan(0x3B, 0x17, 0xF1, 0x04), 0x7F, 0x3B, 0x31)
	assertBytes(t, "unknown define mode", h.gmlan(0x3B, 0x17, 0xF0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00), 0x7F, 0x3B, 0x12)

	assertBytes(t, "clear again", h.gmlan(0x3B, 0x17, 0xF0, 0x04), 0x7B, 0x17)
	assertBytes(t, "read after clear", h.gmlan(0x1A, 0x18), 0x5A, 0x18)
}

// A first frame is answered with flow control announcing block size 0,
// except for a write-by-address (3B 15), which the T8 acks frame by frame
// with block size 1 until the last consecutive frame gets the response.
func TestT8RequestFlowControl(t *testing.T) {
	s := &T8{}
	h := newHarness(t, s)
	assertFrames(t, h.send(0x7E0, 0x10, 0x09, 0x3B, 0x17, 0xF0, 0x04, 0x00, 0x00), fr(0x7E8, 0x30, 0x00, 0x00))

	assertFrames(t, h.send(0x7E0, 0x10, 0x14, 0x3B, 0x15, 0x00, 0x20, 0x00, 0x0E), fr(0x7E8, 0x30, 0x01, 0x00))
	assertFrames(t, h.send(0x7E0, 0x21, 1, 2, 3, 4, 5, 6, 7), fr(0x7E8, 0x30, 0x01, 0x00))
	assertFrames(t, h.send(0x7E0, 0x22, 8, 9, 10, 11, 12, 13, 14), fr(0x7E8, 0x02, 0x7B, 0x15))
	assertBytes(t, "memory", s.Peek(0x2000, 15), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 0)
}

// A long response is a first frame carrying the 12-bit length and six
// bytes, then consecutive frames of seven bytes numbered 21..2F, 20, 21...
// released only by the tester's flow control.
func TestT8ResponseFraming(t *testing.T) {
	s := &T8{}
	h := newHarness(t, s)
	data := make([]byte, 120)
	for i := range data {
		data[i] = byte(i + 1)
	}
	h.gmlan(append([]byte{0x3B, 0x15, 0x00, 0x30, 0x00, byte(len(data))}, data...)...)

	got := h.send(0x7E0, 0x06, 0x23, 0x00, 0x30, 0x00, 0x00, byte(len(data)))
	assertFrames(t, got, fr(0x7E8, 0x10, 124, 0x63, 0x00, 0x30, 0x00, 1, 2))
	cfs := h.send(0x7E0, 0x30, 0x00, 0x00)
	if len(cfs) != 17 {
		t.Fatalf("got %d consecutive frames, want 17", len(cfs))
	}
	seqs := []byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F, 0x20, 0x21}
	var payload []byte
	for i, cf := range cfs {
		if cf.ID != 0x7E8 || cf.Data[0] != seqs[i] {
			t.Fatalf("consecutive frame %d = %s, want sequence %02X", i, cf, seqs[i])
		}
		payload = append(payload, cf.Data[1:]...)
	}
	assertBytes(t, "payload", payload[:118], data[2:]...)
	assertBytes(t, "padding", payload[118:], make([]byte, 1)...)
	if got := h.send(0x7E0, 0x30, 0x00, 0x00); len(got) != 0 {
		t.Fatalf("second flow control released %v", got)
	}
}

// Frames on other identifiers, like the functional tester present on
// 0x101, are ignored.
func TestT8IgnoresOtherIDs(t *testing.T) {
	h := newHarness(t, &T8{})
	if got := h.send(0x101, 0xFE, 0x01, 0x3E); len(got) != 0 {
		t.Fatalf("answered %v", got)
	}
}

// The gocan GMLAN client runs the datalogger's requests against the sim:
// security access, register setup and read, memory write and read.
func TestT8Client(t *testing.T) {
	s := &T8{}
	bus := newHarness(t, s).bus
	s.SetSymbol(77, []byte{0xCA, 0xFE})
	ctx := testCtx(t)
	gm := gmlan.New(bus, 0x7E0, 0x7E8)
	if err := gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
		t.Fatal(err)
	}
	if err := gm.RequestSecurityAccess(ctx, 0xFD, 0, t8sec.CalculateAccessKey); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte{0x01, 0x02, 0x03}, 9)
	if err := gm.WriteDataByAddress(ctx, 0x004000, data); err != nil {
		t.Fatal(err)
	}
	got, err := gm.ReadMemoryByAddress(ctx, 0x004000, uint32(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	assertBytes(t, "ReadMemoryByAddress", got, data...)
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x04}); err != nil {
		t.Fatal(err)
	}
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x80, 0x00, 0x00, 0x00, 0x00, 77}); err != nil {
		t.Fatal(err)
	}
	if err := gm.WriteDataByIdentifier(ctx, 0x17, []byte{0xF0, 0x03, 0x00, 0x05, 0x00, 0x40, 0x00}); err != nil {
		t.Fatal(err)
	}
	got, err = gm.ReadDataByIdentifier(ctx, 0x18)
	if err != nil {
		t.Fatal(err)
	}
	assertBytes(t, "ReadDataByIdentifier(18)", got, 0xCA, 0xFE, 0x01, 0x02, 0x03, 0x01, 0x02)
	if err := gm.TesterPresentResponseRequired(ctx); err != nil {
		t.Fatal(err)
	}
	if err := gm.ReturnToNormalMode(ctx); err != nil {
		t.Fatal(err)
	}
}
