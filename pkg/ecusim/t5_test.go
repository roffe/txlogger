package ecusim

import (
	"bytes"
	"testing"
)

// Poke and Peek work before Open, unset memory reads as zero, and Open
// keeps what was poked.
func TestT5PokePeek(t *testing.T) {
	s := &T5{}
	s.Poke(0x1000, []byte{1, 2, 3})
	if got := s.Peek(0x0FFF, 5); !bytes.Equal(got, []byte{0, 1, 2, 3, 0}) {
		t.Fatalf("Peek = % X", got)
	}
	newHarness(t, s)
	if got := s.Peek(0x1000, 3); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("Peek after Open = % X", got)
	}
}

// C7 answers with the six bytes ending at the 16-bit address in bytes 3-4,
// highest address first.
func TestT5ReadRAM(t *testing.T) {
	s := &T5{}
	h := newHarness(t, s)
	s.Poke(0x1000, []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16})
	got := h.send(0x05, 0xC7, 0x00, 0x00, 0x10, 0x05, 0x00, 0x00, 0x00)
	assertFrames(t, got, fr(0x0C, 0xC7, 0x00, 0x15, 0x14, 0x13, 0x12, 0x11, 0x10))
}

// typeLine sends line one character per C4 frame and checks every
// character is acknowledged with C6 00.
func typeLine(t *testing.T, h *harness, line string) {
	t.Helper()
	for i := range len(line) {
		got := h.send(0x05, 0xC4, line[i], 0, 0, 0, 0, 0, 0)
		assertFrames(t, got, fr(0x0C, 0xC6, 0x00))
	}
}

// The console runs W<addr:4 hex><byte:2 hex> when the carriage return
// arrives, not before.
func TestT5ConsoleWrite(t *testing.T) {
	s := &T5{}
	h := newHarness(t, s)
	typeLine(t, h, "W12345A")
	if got := s.Peek(0x1234, 1)[0]; got != 0 {
		t.Fatalf("byte written before the carriage return: %02X", got)
	}
	typeLine(t, h, "\r")
	if got := s.Peek(0x1234, 1)[0]; got != 0x5A {
		t.Fatalf("RAM[1234] = %02X, want 5A", got)
	}
	typeLine(t, h, "wABCD01\rW00FFc3\r") // the command letter is case sensitive, hex is not
	if got := s.Peek(0xABCD, 1)[0]; got != 0 {
		t.Errorf("lower case w wrote RAM[ABCD] = %02X", got)
	}
	if got := s.Peek(0x00FF, 1)[0]; got != 0xC3 {
		t.Errorf("RAM[00FF] = %02X, want C3", got)
	}
}

// Console lines that are not a well formed W command are acknowledged and
// otherwise ignored.
func TestT5ConsoleIgnoresMalformed(t *testing.T) {
	for _, line := range []string{"", "W1234", "W12345A6", "X12345A", "WZZZZ5A", "W1234ZZ"} {
		s := &T5{}
		h := newHarness(t, s)
		typeLine(t, h, line+"\r")
		if len(s.ram) != 0 {
			t.Errorf("%q wrote memory: %v", line, s.ram)
		}
	}
}

// A5 sets a block write window; data frames carry an offset and up to
// seven bytes, are clipped to the window and acknowledged with the offset.
func TestT5BlockWrite(t *testing.T) {
	s := &T5{}
	h := newHarness(t, s)
	assertFrames(t, h.send(0x05, 0xA5, 0x00, 0x00, 0x30, 0x00, 10, 0, 0), fr(0x0C, 0xA5, 0x00))
	assertFrames(t, h.send(0x05, 0x00, 1, 2, 3, 4, 5, 6, 7), fr(0x0C, 0x00, 0x00))
	assertFrames(t, h.send(0x05, 0x07, 8, 9, 10, 0xEE, 0xEE, 0xEE, 0xEE), fr(0x0C, 0x07, 0x00))
	if got := s.Peek(0x3000, 11); !bytes.Equal(got, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 0}) {
		t.Fatalf("window = % X", got)
	}
}

// Without a write window (none set yet, or an A5 with length 0 arming the
// stream call) data frames are ignored and unanswered.
func TestT5DataWithoutWindow(t *testing.T) {
	s := &T5{}
	h := newHarness(t, s)
	if got := h.send(0x05, 0x00, 1, 2, 3, 4, 5, 6, 7); len(got) != 0 {
		t.Fatalf("data before A5 answered with %v", got)
	}
	assertFrames(t, h.send(0x05, 0xA5, 0x00, 0x00, 0x77, 0x00, 0, 0, 0), fr(0x0C, 0xA5, 0x00))
	if got := h.send(0x05, 0x00, 1, 2, 3, 4, 5, 6, 7); len(got) != 0 {
		t.Fatalf("data after arming A5 answered with %v", got)
	}
	if len(s.ram) != 0 {
		t.Fatalf("memory written: %v", s.ram)
	}
}

// C1 at the gather stub walks the descriptor table {count, pad,
// N*{addr.w, len.b, pad.b}} and packs the listed bytes into the output
// buffer, without answering. C1 anywhere else does nothing.
func TestT5Gather(t *testing.T) {
	s := &T5{}
	h := newHarness(t, s)
	s.Poke(0x1000, []byte{0xAA, 0xBB, 0xCC})
	s.Poke(0x2000, []byte{0xDD, 0xEE})
	s.Poke(t5TableAddr, []byte{2, 0, 0x10, 0x00, 3, 0, 0x20, 0x00, 2, 0})

	if got := h.send(0x05, 0xC1, 0x00, 0x00, 0x12, 0x34, 0, 0, 0); len(got) != 0 {
		t.Fatalf("C1 answered with %v", got)
	}
	if got := s.Peek(t5BufAddr, 5); !bytes.Equal(got, make([]byte, 5)) {
		t.Fatalf("C1 to another address gathered % X", got)
	}

	if got := h.send(0x05, 0xC1, 0x00, 0x00, 0x77, 0x00, 0, 0, 0); len(got) != 0 {
		t.Fatalf("C1 answered with %v", got)
	}
	if got := s.Peek(t5BufAddr, 6); !bytes.Equal(got, []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0}) {
		t.Fatalf("gather buffer = % X", got)
	}
}

// Frames on other identifiers are ignored.
func TestT5IgnoresOtherIDs(t *testing.T) {
	h := newHarness(t, &T5{})
	if got := h.send(0x06, 0xC7, 0, 0, 0x10, 0x00, 0, 0, 0); len(got) != 0 {
		t.Fatalf("answered %v", got)
	}
}
