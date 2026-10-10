package ecusim

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
)

// harness opens a real gocan bus on a sim and records every frame the sim
// puts on it.
type harness struct {
	t   *testing.T
	bus *gocan.Bus
	rx  <-chan gocan.Frame
}

func newHarness(t *testing.T, sim gocan.Adapter) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	bus, err := gocan.OpenAdapter(ctx, sim)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	h := &harness{t: t, bus: bus, rx: bus.SubscribeN(ctx, 1024)}
	t.Cleanup(func() {
		_ = bus.Close()
		cancel()
	})
	return h
}

// send writes one frame to the sim and returns every frame it answered
// with. With Latency 0 the sims reply from inside Send, so all replies are
// buffered by the time Send returns and an empty result means silence.
func (h *harness) send(id uint32, data ...byte) []gocan.Frame {
	h.t.Helper()
	if err := h.bus.Send(context.Background(), gocan.NewFrame(id, data)); err != nil {
		h.t.Fatal(err)
	}
	return h.drain()
}

// drain returns the frames already delivered and not yet read.
func (h *harness) drain() []gocan.Frame {
	var out []gocan.Frame
	for {
		select {
		case f := <-h.rx:
			out = append(out, f)
		default:
			return out
		}
	}
}

// fr builds the 8-byte frame a sim sends: data zero padded to 8 bytes.
func fr(id uint32, data ...byte) gocan.Frame {
	return gocan.NewFrame(id, pad8(data))
}

// assertFrames fails unless got is exactly want, in order.
func assertFrames(t *testing.T, got []gocan.Frame, want ...gocan.Frame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d frames %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frame %d = %s, want %s", i, got[i], want[i])
		}
	}
}

// testCtx bounds a client call so a stuck exchange fails instead of hanging.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// trace collects the lines a sim writes to its Log func.
type trace struct {
	mu    sync.Mutex
	lines []string
}

func (tr *trace) log(s string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lines = append(tr.lines, s)
}

func (tr *trace) get() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return append([]string(nil), tr.lines...)
}

// pad8 zero pads to a full CAN payload and never truncates or aliases.
func TestPad8(t *testing.T) {
	for _, in := range [][]byte{nil, {1}, {1, 2, 3, 4, 5, 6, 7, 8}} {
		got := pad8(in)
		if len(got) != 8 {
			t.Fatalf("pad8(% X) has %d bytes", in, len(got))
		}
		if !bytes.Equal(got[:len(in)], in) || !bytes.Equal(got[len(in):], make([]byte, 8-len(in))) {
			t.Fatalf("pad8(% X) = % X", in, got)
		}
		if len(in) > 0 {
			got[0] = 0xEE
			if in[0] == 0xEE {
				t.Fatal("pad8 aliases its input")
			}
		}
	}
}

// deliver hands frames to the bus in order, synchronously without latency
// and from another goroutine no sooner than the latency otherwise, tracing
// each one as "-> frame".
func TestDeliver(t *testing.T) {
	frames := []gocan.Frame{fr(0x100, 1), fr(0x100, 2), fr(0x100, 3)}
	for _, latency := range []time.Duration{0, 5 * time.Millisecond} {
		h := newHarness(t, &T5{})
		tr := &trace{}
		start := time.Now()
		deliver(h.bus, latency, tr.log, frames...)
		var got []gocan.Frame
		if latency == 0 {
			got = h.drain()
		} else {
			ctx := testCtx(t)
			for len(got) < len(frames) {
				select {
				case f := <-h.rx:
					got = append(got, f)
				case <-ctx.Done():
					t.Fatalf("latency %v: got %d of %d frames", latency, len(got), len(frames))
				}
			}
			if el := time.Since(start); el < latency {
				t.Errorf("frames arrived after %v, before the %v latency", el, latency)
			}
		}
		assertFrames(t, got, frames...)
		lines := tr.get()
		if len(lines) != len(frames) {
			t.Fatalf("latency %v: trace %q", latency, lines)
		}
		for i, l := range lines {
			if l != "-> "+frames[i].String() {
				t.Errorf("trace line %d = %q, want %q", i, l, "-> "+frames[i].String())
			}
		}
	}
}

// A nil trace func is allowed.
func TestDeliverNilTrace(t *testing.T) {
	h := newHarness(t, &T5{})
	deliver(h.bus, 0, nil, fr(0x10, 0xAA))
	assertFrames(t, h.drain(), fr(0x10, 0xAA))
}

// The sims trace what they receive as "<- frame" and what they send as
// "-> frame", in wire order.
func TestSimTrace(t *testing.T) {
	tr := &trace{}
	h := newHarness(t, &T5{Log: tr.log})
	req := gocan.NewFrame(0x05, []byte{0xC4, 'X'})
	if err := h.bus.Send(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	want := []string{"<- " + req.String(), "-> " + fr(0x0C, 0xC6, 0x00).String()}
	if got := tr.get(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("trace = %q, want %q", got, want)
	}
}
