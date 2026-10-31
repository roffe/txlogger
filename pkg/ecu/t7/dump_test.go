package t7

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roffe/gocan/v2/t7kwp"
)

// openSession initialises c and unlocks the ECU, as DumpECU does before
// reading.
func openSession(t *testing.T, c *Client) {
	t.Helper()
	if ok, err := c.KnockKnock(testCtx(t)); err != nil || !ok {
		t.Fatalf("KnockKnock = %v, %v", ok, err)
	}
}

// readECU without the fast path reads with 0x2C F0 03 + 0x21 F0 in chunks
// of dumpChunkSize, ending with a short one, and reports progress from the
// negative total up to the full length.
func TestReadECUStandard(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{mem: patternMem(0x1000)}
	c := newClient(t, e, rec)
	openSession(t, c)

	const addr, length = 0x100, 0x300
	got, err := c.readECU(testCtx(t), addr, length, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, e.mem[addr:addr+length]) {
		t.Fatal("read data does not match ECU memory")
	}
	want := []span{{0x100, 0xF5}, {0x1F5, 0xF5}, {0x2EA, 0xF5}, {0x3DF, 0x21}}
	if !slices.Equal(e.reads, want) {
		t.Fatalf("read setups %X, want %X", e.reads, want)
	}
	if first, last := rec.progress[0], rec.progress[len(rec.progress)-1]; first != -length || last != length {
		t.Fatalf("progress runs %v..%v, want %v..%v", first, last, -length, length)
	}
	if !rec.hasMsg("Done, took") {
		t.Fatalf("messages %q lack the completion line", rec.msgs)
	}
}

// A chunk that fails once is requested again at the same address, and the
// result has no gap.
func TestReadECURetriesChunk(t *testing.T) {
	rec := &recorder{}
	failed := false
	e := &fakeT7{mem: patternMem(0x400)}
	e.fault = func(sid byte, p []byte) []byte {
		if sid == t7kwp.DYNAMICALLY_DEFINE_IDENTIFIER && u24(p[4:]) == 0xF5 && !failed {
			failed = true
			return []byte{0x7F, sid, t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT}
		}
		return nil
	}
	c := newClient(t, e, rec)
	openSession(t, c)

	got, err := c.readECU(testCtx(t), 0, 0x200, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, e.mem[:0x200]) {
		t.Fatal("read data does not match ECU memory")
	}
	if want := []span{{0, 0xF5}, {0xF5, 0xF5}, {0x1EA, 0x16}}; !slices.Equal(e.reads, want) {
		t.Fatalf("read setups %X, want %X", e.reads, want)
	}
	if n := e.count("2C"); n != 4 {
		t.Fatalf("sent %d read setups, want 4 (one refused, sent again)", n)
	}
	if !rec.hasMsg("retrying") {
		t.Fatalf("messages %q do not mention the retry", rec.msgs)
	}
}

// A chunk that keeps failing ends the read with an error naming its
// position, after three attempts.
func TestReadECUGivesUp(t *testing.T) {
	e := &fakeT7{mem: patternMem(0x400)}
	e.fault = func(sid byte, p []byte) []byte {
		if sid == t7kwp.DYNAMICALLY_DEFINE_IDENTIFIER && u24(p[4:]) == 0xF5 {
			return []byte{0x7F, sid, t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT}
		}
		return nil
	}
	c := newClient(t, e, nil)
	openSession(t, c)

	got, err := c.readECU(testCtx(t), 0, 0x200, false)
	if err == nil || got != nil {
		t.Fatalf("readECU = %d bytes, %v; want an error", len(got), err)
	}
	if !strings.Contains(err.Error(), "pos: 0xF5") || !errors.Is(err, t7kwp.ErrSubFunctionNotSupportedOrInvalidFormat) {
		t.Fatalf("err = %v, want the failing position and the ECU's reason", err)
	}
	if n := e.count("2C"); n != 4 {
		t.Fatalf("sent %d read setups, want 1 good + 3 attempts", n)
	}
}

// A cancelled context stops the read before any request goes out.
func TestReadECUCancelled(t *testing.T) {
	e := &fakeT7{mem: patternMem(0x400)}
	c := newClient(t, e, nil)
	openSession(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.readECU(ctx, 0, 0x200, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(e.reads) != 0 {
		t.Fatalf("sent read setups %X after cancellation", e.reads)
	}
}

// An ECU that refuses readDataByLocalIdentifier F0 (as the EOL RAM loop does
// right after a flash, answering 7F 21 12) must fail the read, not yield a
// short dump.
func TestReadECURefusedData(t *testing.T) {
	e := &fakeT7{mem: patternMem(0x400)}
	e.fault = func(sid byte, _ []byte) []byte {
		if sid == t7kwp.READ_DATA_BY_IDENTIFIER {
			return []byte{0x7F, sid, t7kwp.SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT}
		}
		return nil
	}
	c := newClient(t, e, nil)
	openSession(t, c)

	got, err := c.readECU(testCtx(t), 0, 0x200, false)
	if err == nil {
		t.Fatalf("readECU returned %d of 0x200 bytes (% X) and no error", len(got), got)
	}
}

// DumpECU on an ECU with the fast-transfer extension reads all 512 KB with
// fast uploads of fastDumpChunk bytes, never touching the KWP read path, and
// closes the session.
func TestDumpECUFast(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{mem: patternMem(0x80000), fastBlock: 0x2000}
	c := newClient(t, e, rec)

	got, err := c.DumpECU(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, e.mem) {
		t.Fatal("dump does not match ECU memory")
	}
	var want []int
	for a := 0; a < 0x80000; a += fastDumpChunk {
		want = append(want, a)
	}
	if !slices.Equal(e.fastUps, want) {
		t.Fatalf("fast uploads at %X, want %X", e.fastUps, want)
	}
	if len(e.reads) != 0 {
		t.Fatalf("KWP reads %X used although fast upload worked", e.reads)
	}
	if !rec.hasMsg("ECU supports fast transfer (8192 byte blocks)") {
		t.Fatalf("messages %q lack the probe result", rec.msgs)
	}
	if last := e.trace[len(e.trace)-1]; last != "82" {
		t.Fatalf("last request %q, want stopCommunication", last)
	}
}

// When a fast chunk keeps failing (an adapter that drops stream frames), the
// read carries on with the KWP path from exactly where the fast path stopped.
func TestReadECUFastFallsBack(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{mem: patternMem(0x10000), fastBlock: 0x2000, brokenFast: func(addr int) bool { return addr == 0x8000 }}
	c := newClient(t, e, rec)
	openSession(t, c)

	got, err := c.readECU(testCtx(t), 0, 0x10000, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, e.mem) {
		t.Fatal("read data does not match ECU memory")
	}
	if want := []int{0, 0x8000, 0x8000, 0x8000}; !slices.Equal(e.fastUps, want) {
		t.Fatalf("fast uploads at %X, want %X", e.fastUps, want)
	}
	if len(e.reads) == 0 || e.reads[0].addr != 0x8000 {
		t.Fatalf("KWP read starts at %X, want 0x8000", e.reads)
	}
	if !rec.hasMsg("Fast read failed at 0x8000") {
		t.Fatalf("messages %q do not report the fallback", rec.msgs)
	}
}

// A context cancelled during the fast read ends the read with the context's
// error instead of falling back.
func TestReadECUFastCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(testCtx(t))
	defer cancel()
	e := &fakeT7{mem: patternMem(0x10000), fastBlock: 0x2000}
	e.fault = func(sid byte, p []byte) []byte {
		if sid == t7kwp.START_ROUTINE_BY_IDENTIFIER && p[0] == t7kwp.RLI_FAST_UPLOAD {
			cancel()
			return noReply
		}
		return nil
	}
	c := newClient(t, e, nil)
	openSession(t, c)

	if _, err := c.readECU(ctx, 0, 0x10000, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(e.reads) != 0 {
		t.Fatalf("fell back to KWP reads %X after cancellation", e.reads)
	}
}

// probeFast reports 0 for stock firmware (negative response) and for a
// probe that goes unanswered, saying so in the latter case.
func TestProbeFast(t *testing.T) {
	ctx := testCtx(t)
	c := newClient(t, &fakeT7{}, nil)
	openSession(t, c)
	if n := c.probeFast(ctx); n != 0 {
		t.Fatalf("stock ECU: probeFast = %d, want 0", n)
	}

	rec := &recorder{}
	e := &fakeT7{fastBlock: 0x2000, fault: func(sid byte, p []byte) []byte {
		if sid == t7kwp.START_ROUTINE_BY_IDENTIFIER && p[0] == t7kwp.RLI_FAST_INFO {
			return noReply
		}
		return nil
	}}
	c = newClient(t, e, rec)
	openSession(t, c)
	if n := c.probeFast(ctx); n != 0 {
		t.Fatalf("silent probe: probeFast = %d, want 0", n)
	}
	if !rec.hasMsg("Fast transfer probe failed") {
		t.Fatalf("messages %q do not report the failed probe", rec.msgs)
	}
}

// DumpECU refuses to read when the session cannot be opened.
func TestDumpECUNoSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	e := &fakeT7{silent: true}
	_, err := newClient(t, e, nil).DumpECU(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to authenticate") {
		t.Fatalf("err = %v, want an authentication failure", err)
	}
	for _, s := range e.trace {
		if strings.Contains(s, "27") {
			t.Fatalf("trace %q: security access attempted without a session", e.trace)
		}
	}
}
