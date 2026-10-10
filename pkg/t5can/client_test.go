package t5can

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecusim"
)

// scriptT5 records every frame sent to it and answers each one with the
// frames reply returns for it; no frames means the ECU stays silent.
type scriptT5 struct {
	bus   *gocan.Bus
	reply func(gocan.Frame) []gocan.Frame

	mu   sync.Mutex
	sent []gocan.Frame
}

func (e *scriptT5) Open(_ context.Context, bus *gocan.Bus) error { e.bus = bus; return nil }
func (e *scriptT5) Close() error                                 { return nil }

func (e *scriptT5) Send(_ context.Context, f gocan.Frame) error {
	e.mu.Lock()
	e.sent = append(e.sent, f)
	e.mu.Unlock()
	if e.reply != nil {
		for _, r := range e.reply(f) {
			e.bus.Deliver(r)
		}
	}
	return nil
}

func (e *scriptT5) frames() []gocan.Frame {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]gocan.Frame(nil), e.sent...)
}

// frame is a reply on replyID carrying exactly b.
func frame(b ...byte) gocan.Frame { return gocan.NewFrame(replyID, b) }

// ackAll answers like a healthy T5: byte 0 echoed (C6 for a C4 console
// character) followed by status 0x00.
func ackAll(f gocan.Frame) []gocan.Frame {
	echo := f.Data[0]
	if echo == 0xC4 {
		echo = 0xC6
	}
	return []gocan.Frame{frame(echo, respOK, 0, 0, 0, 0, 0, 0)}
}

// newScripted opens a bus on a scriptT5 and returns a client with a short
// reply timeout so silent-ECU cases finish quickly.
func newScripted(t *testing.T, reply func(gocan.Frame) []gocan.Frame) (*Client, *scriptT5, *gocan.Bus) {
	t.Helper()
	ecu := &scriptT5{reply: reply}
	bus, err := gocan.OpenAdapter(context.Background(), ecu)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	c := NewClient(bus)
	c.defaultTimeout = 50 * time.Millisecond
	return c, ecu, bus
}

// newSim opens a bus on the ecusim fake T5, which models RAM and the C7, C4
// and A5 commands independently of this package.
func newSim(t *testing.T) (*Client, *ecusim.T5) {
	t.Helper()
	sim := &ecusim.T5{}
	bus, err := gocan.OpenAdapter(context.Background(), sim)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	return NewClient(bus), sim
}

func pattern(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = seed + byte(i*7)
	}
	return b
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ReadRam returns length bytes starting at address, for lengths that are and
// are not a multiple of the 6-byte read window.
func TestReadRamReturnsMemory(t *testing.T) {
	c, sim := newSim(t)
	mem := pattern(40, 0x10)
	sim.Poke(0x1000, mem)
	for _, n := range []uint32{1, 5, 6, 7, 12, 13, 40} {
		got, err := c.ReadRam(ctxT(t), 0x1000, n)
		if err != nil {
			t.Fatalf("len %d: %v", n, err)
		}
		if !bytes.Equal(got, mem[:n]) {
			t.Errorf("len %d: got % X, want % X", n, got, mem[:n])
		}
	}
}

// ReadRam sends one C7 frame per 6 bytes, addressed 5 past the start of each
// window, and reverses the six reply bytes (the ECU sends the highest
// address first).
func TestReadRamWire(t *testing.T) {
	c, ecu, _ := newScripted(t, func(f gocan.Frame) []gocan.Frame {
		return []gocan.Frame{frame(0xC7, respOK, 6, 5, 4, 3, 2, 1)}
	})
	got, err := c.ReadRam(ctxT(t), 0x1234, 13)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1}; !bytes.Equal(got, want) {
		t.Errorf("data: got % X, want % X", got, want)
	}
	want := []gocan.Frame{
		gocan.NewFrame(canID, []byte{0xC7, 0x00, 0x00, 0x12, 0x39}),
		gocan.NewFrame(canID, []byte{0xC7, 0x00, 0x00, 0x12, 0x3F}),
		gocan.NewFrame(canID, []byte{0xC7, 0x00, 0x00, 0x12, 0x45}),
	}
	if sent := ecu.frames(); fmt.Sprint(sent) != fmt.Sprint(want) {
		t.Errorf("frames:\ngot  %v\nwant %v", sent, want)
	}
}

// ReadRam of zero bytes returns an empty slice without touching the bus.
func TestReadRamZeroLength(t *testing.T) {
	c, ecu, _ := newScripted(t, ackAll)
	got, err := c.ReadRam(ctxT(t), 0x1000, 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("got % X, %v", got, err)
	}
	if n := len(ecu.frames()); n != 0 {
		t.Fatalf("sent %d frames, want 0", n)
	}
}

// request ignores empty frames, frames with the wrong echo byte and frames on
// other IDs while waiting for the reply.
func TestRequestSkipsUnrelatedFrames(t *testing.T) {
	c, _, _ := newScripted(t, func(f gocan.Frame) []gocan.Frame {
		return []gocan.Frame{
			gocan.NewFrame(replyID, nil),
			frame(0xC6, respOK, 9, 9, 9, 9, 9, 9),
			gocan.NewFrame(replyID+1, []byte{0xC7, respOK, 8, 8, 8, 8, 8, 8}),
			frame(0xC7, respOK, 6, 5, 4, 3, 2, 1),
		}
	})
	got, err := c.ReadRam(ctxT(t), 0x1000, 6)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{1, 2, 3, 4, 5, 6}; !bytes.Equal(got, want) {
		t.Fatalf("got % X, want % X", got, want)
	}
}

// A single byte at a 16-bit address is written by typing the console command
// W<addr:4><byte:2>\r one C4 frame per character.
func TestWriteRamSingleByteUsesConsole(t *testing.T) {
	c, ecu, _ := newScripted(t, ackAll)
	if err := c.WriteRam(ctxT(t), 0x1234, []byte{0xAB}); err != nil {
		t.Fatal(err)
	}
	var typed []byte
	for _, f := range ecu.frames() {
		if f.ID != canID || f.Length != 2 || f.Data[0] != 0xC4 {
			t.Fatalf("unexpected frame %v", f)
		}
		typed = append(typed, f.Data[1])
	}
	if string(typed) != "W1234AB\r" {
		t.Fatalf("typed %q, want %q", typed, "W1234AB\r")
	}

	sc, sim := newSim(t)
	if err := sc.WriteRam(ctxT(t), 0x1234, []byte{0xAB}); err != nil {
		t.Fatal(err)
	}
	if got := sim.Peek(0x1234, 1); got[0] != 0xAB {
		t.Fatalf("memory = %#x, want 0xAB", got[0])
	}
}

// WriteRam of nothing sends nothing.
func TestWriteRamEmpty(t *testing.T) {
	c, ecu, _ := newScripted(t, ackAll)
	if err := c.WriteRam(ctxT(t), 0x1000, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(ecu.frames()); n != 0 {
		t.Fatalf("sent %d frames, want 0", n)
	}
}

// Multi-byte writes, and single bytes above 0xFFFF, land in memory through the
// A5 block protocol, split into windows of at most 133 bytes.
func TestWriteRamBlocksReachMemory(t *testing.T) {
	c, sim := newSim(t)
	data := pattern(300, 0x21)
	if err := c.WriteRam(ctxT(t), 0x7700, data); err != nil {
		t.Fatal(err)
	}
	if got := sim.Peek(0x7700, len(data)); !bytes.Equal(got, data) {
		t.Fatalf("memory mismatch:\ngot  % X\nwant % X", got, data)
	}
	if err := c.WriteRam(ctxT(t), 0x12345, []byte{0x5A}); err != nil {
		t.Fatal(err)
	}
	if got := sim.Peek(0x12345, 1); got[0] != 0x5A {
		t.Fatalf("memory at 0x12345 = %#x, want 0x5A", got[0])
	}
}

// Each block starts with A5 <addr:4> <len> 00 00 and streams its bytes in
// frames of <offset> + 7 data bytes; the next block's address follows on from
// the previous one, and a short final chunk is zero padded.
func TestWriteRamBlockWire(t *testing.T) {
	c, ecu, _ := newScripted(t, ackAll)
	data := pattern(300, 0x01)
	const base = 0x00012345
	if err := c.WriteRam(ctxT(t), base, data); err != nil {
		t.Fatal(err)
	}
	sent := ecu.frames()
	if len(sent) != 3+19+19+5 {
		t.Fatalf("sent %d frames, want 46", len(sent))
	}
	var got []byte
	i := 0
	for _, blk := range []struct {
		addr uint32
		n    int
	}{{base, 133}, {base + 133, 133}, {base + 266, 34}} {
		hdr := sent[i]
		i++
		want := gocan.NewFrame(canID, []byte{0xA5, byte(blk.addr >> 24), byte(blk.addr >> 16), byte(blk.addr >> 8), byte(blk.addr), byte(blk.n), 0, 0})
		if hdr != want {
			t.Fatalf("block header: got %v, want %v", hdr, want)
		}
		for off := 0; off < blk.n; off += chunkSize {
			f := sent[i]
			i++
			if f.ID != canID || f.Length != 8 || int(f.Data[0]) != off {
				t.Fatalf("data frame at offset %d: %v", off, f)
			}
			n := min(chunkSize, blk.n-off)
			got = append(got, f.Data[1:1+n]...)
			for _, pad := range f.Data[1+n:] {
				if pad != 0 {
					t.Fatalf("offset %d: padding not zeroed: %v", off, f)
				}
			}
		}
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("streamed data mismatch:\ngot  % X\nwant % X", got, data)
	}
}

// WriteRam2 types one console W command per byte at consecutive addresses.
func TestWriteRam2(t *testing.T) {
	c, ecu, _ := newScripted(t, ackAll)
	if err := c.WriteRam2(ctxT(t), 0x2000, []byte{0x01, 0xFE, 0x7F}); err != nil {
		t.Fatal(err)
	}
	var typed []byte
	for _, f := range ecu.frames() {
		if f.Data[0] != 0xC4 {
			t.Fatalf("unexpected frame %v", f)
		}
		typed = append(typed, f.Data[1])
	}
	if want := "W200001\rW2001FE\rW20027F\r"; string(typed) != want {
		t.Fatalf("typed %q, want %q", typed, want)
	}

	sc, sim := newSim(t)
	if err := sc.WriteRam2(ctxT(t), 0x3000, []byte{9, 8, 7}); err != nil {
		t.Fatal(err)
	}
	if got := sim.Peek(0x3000, 3); !bytes.Equal(got, []byte{9, 8, 7}) {
		t.Fatalf("memory = % X", got)
	}
}

// Failed replies surface as errors naming the step, and the transfer stops at
// the frame that failed.
func TestErrors(t *testing.T) {
	silent := func(gocan.Frame) []gocan.Frame { return nil }
	only := func(cmd byte, r ...gocan.Frame) func(gocan.Frame) []gocan.Frame {
		return func(f gocan.Frame) []gocan.Frame {
			if f.Data[0] == cmd {
				return r
			}
			return ackAll(f)
		}
	}
	tests := []struct {
		name   string
		reply  func(gocan.Frame) []gocan.Frame
		run    func(context.Context, *Client) error
		msg    string // substring of the error
		cause  error  // errors.Is target, if any
		frames int    // frames sent before giving up
	}{
		{
			name:   "read error code",
			reply:  only(0xC7, frame(0xC7, 0x04, 0, 0, 0, 0, 0, 0)),
			run:    func(ctx context.Context, c *Client) error { _, err := c.ReadRam(ctx, 0x1000, 12); return err },
			msg:    "read error at 0x1005: code 0x04",
			frames: 1,
		},
		{
			name:   "read short reply",
			reply:  only(0xC7, frame(0xC7, respOK, 1, 2)),
			run:    func(ctx context.Context, c *Client) error { _, err := c.ReadRam(ctx, 0x1000, 6); return err },
			msg:    "short response: got 4 bytes",
			frames: 1,
		},
		{
			name:   "read no reply",
			reply:  silent,
			run:    func(ctx context.Context, c *Client) error { _, err := c.ReadRam(ctx, 0x1000, 6); return err },
			msg:    "ReadRam[1]",
			cause:  context.DeadlineExceeded,
			frames: 1,
		},
		{
			name:   "set-address NACK",
			reply:  only(0xA5, frame(0xA5, 0x01)),
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, make([]byte, 10)) },
			msg:    "set-address NACK: 0x01",
			frames: 1,
		},
		{
			name:   "set-address short reply",
			reply:  only(0xA5, frame(0xA5)),
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, make([]byte, 10)) },
			msg:    "set-address short response: 1 bytes",
			frames: 1,
		},
		{
			name:   "set-address no reply",
			reply:  silent,
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, make([]byte, 10)) },
			msg:    "set-address send failed",
			cause:  context.DeadlineExceeded,
			frames: 1,
		},
		{
			name:   "data NACK at second chunk",
			reply:  only(7, frame(7, 0x02)),
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, make([]byte, 20)) },
			msg:    "data NACK at offset 7: 0x02",
			frames: 3,
		},
		{
			name:   "data short reply",
			reply:  only(0, frame(0)),
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, make([]byte, 20)) },
			msg:    "data short response at offset 0: 1 bytes",
			frames: 2,
		},
		{
			name:   "data no reply",
			reply:  only(0),
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, make([]byte, 20)) },
			msg:    "data send failed at offset 0",
			cause:  context.DeadlineExceeded,
			frames: 2,
		},
		{
			name:   "console no reply",
			reply:  silent,
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam(ctx, 0x1000, []byte{1}) },
			cause:  context.DeadlineExceeded,
			frames: 1,
		},
		{
			name:   "WriteRam2 stops at first failure",
			reply:  silent,
			run:    func(ctx context.Context, c *Client) error { return c.WriteRam2(ctx, 0x1000, []byte{1, 2, 3}) },
			cause:  context.DeadlineExceeded,
			frames: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ecu, _ := newScripted(t, tt.reply)
			err := tt.run(ctxT(t), c)
			if err == nil {
				t.Fatal("expected an error")
			}
			if tt.msg != "" && !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("error %q does not contain %q", err, tt.msg)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Errorf("error %v is not %v", err, tt.cause)
			}
			if n := len(ecu.frames()); n != tt.frames {
				t.Errorf("sent %d frames, want %d", n, tt.frames)
			}
		})
	}
}

// A request whose adapter dies mid-wait reports the adapter's error rather
// than a timeout.
func TestRequestReportsBusFailure(t *testing.T) {
	unplugged := errors.New("adapter unplugged")
	var bus *gocan.Bus
	c, _, b := newScripted(t, func(gocan.Frame) []gocan.Frame {
		bus.Fatal(unplugged)
		return nil
	})
	bus = b
	c.defaultTimeout = 5 * time.Second
	_, err := c.ReadRam(ctxT(t), 0x1000, 6)
	if !errors.Is(err, unplugged) {
		t.Fatalf("got %v, want %v", err, unplugged)
	}
}

// Calls on a closed bus fail with gocan.ErrClosed.
func TestClosedBus(t *testing.T) {
	c, ecu, bus := newScripted(t, ackAll)
	bus.Close()
	if _, err := c.ReadRam(ctxT(t), 0x1000, 6); !errors.Is(err, gocan.ErrClosed) {
		t.Errorf("ReadRam: got %v, want ErrClosed", err)
	}
	if err := c.WriteRam(ctxT(t), 0x1000, make([]byte, 8)); !errors.Is(err, gocan.ErrClosed) {
		t.Errorf("WriteRam: got %v, want ErrClosed", err)
	}
	if n := len(ecu.frames()); n != 0 {
		t.Errorf("sent %d frames on a closed bus", n)
	}
}

// sendBlock refuses empty and over-long blocks before sending anything.
func TestSendBlockRejectsBadSize(t *testing.T) {
	c, ecu, _ := newScripted(t, ackAll)
	for _, n := range []int{0, maxBlock + 1} {
		err := c.sendBlock(ctxT(t), 0x1000, make([]byte, n), maxBlock)
		if err == nil || !strings.Contains(err.Error(), "invalid block size") {
			t.Errorf("size %d: got %v", n, err)
		}
	}
	if n := len(ecu.frames()); n != 0 {
		t.Errorf("sent %d frames", n)
	}
}
