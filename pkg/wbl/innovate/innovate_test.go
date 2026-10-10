package innovate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProcessBytes(t *testing.T) {
	// packet builds an ISP2 packet of n words: header, one lambda sub-packet
	// (normal, stoich 14.7, λ 0.991, from LM2.cs' sample) and zeroed aux words.
	packet := func(n int) []byte {
		p := []byte{0xB2, 0x80 | byte(n), 0x43, 0x13, 0x03, 0x6B}
		return append(p, make([]byte, (n-2)*2)...)
	}
	tests := []struct {
		name string
		in   []byte
	}{
		{"LM-2", packet(7)},
		{"chained MTS, 11 words", packet(11)},
		{"lambda word lost, then a good packet", append(packet(7)[:4], packet(7)...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := NewISP2Client("test", func(string) {})
			c.SetData(tt.in)
			if got := c.GetLambda(); got != 0.991 {
				t.Errorf("lambda = %v, want 0.991", got)
			}
		})
	}
}

// statusWord builds the first word of an ISP2 lambda sub-packet: the 3-bit
// function (status) code and the AFR multiplier in tenths (147 = 14.7).
func statusWord(function, afrx10 uint8) [2]byte {
	return [2]byte{0x42 | function<<2 | afrx10>>7, afrx10 & 0x7F}
}

// lambdaWord builds the lambda word for λ given in thousandths (991 = 0.991).
func lambdaWord(milli uint16) [2]byte {
	raw := milli - ISP2_LAMBDA_OFFSET
	return [2]byte{byte(raw>>7) & 0x3F, byte(raw) & 0x7F}
}

// isp2Packet builds an ISP2 packet of n (>= 2) words after the header: one
// lambda sub-packet followed by zeroed aux words. The word count's bit 7 goes
// in the high header byte, bits 6-0 in the low one.
func isp2Packet(n int, status, lambda [2]byte) []byte {
	p := []byte{0xB2 | byte(n>>7)&0x01, 0x80 | byte(n)&0x7F, status[0], status[1], lambda[0], lambda[1]}
	return append(p, make([]byte, (n-2)*2)...)
}

func newTestClient(t *testing.T) *ISP2Client {
	t.Helper()
	c, err := NewISP2Client("test", func(string) {})
	if err != nil {
		t.Fatalf("NewISP2Client: %v", err)
	}
	return c
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// processBytes finds packets however the stream is chunked, skips noise and
// truncated packets, and leaves the newest packet's lambda in place.
func TestProcessBytesChunked(t *testing.T) {
	normal := statusWord(ISP2_NORMAL, 147)
	p991 := isp2Packet(7, normal, lambdaWord(991))
	p1200 := isp2Packet(7, normal, lambdaWord(1200))
	split := func(p []byte, at ...int) [][]byte {
		var out [][]byte
		prev := 0
		for _, i := range at {
			out = append(out, p[prev:i])
			prev = i
		}
		return append(out, p[prev:])
	}
	var oneByOne [][]byte
	for i := range p991 {
		oneByOne = append(oneByOne, p991[i:i+1])
	}
	tests := []struct {
		name   string
		chunks [][]byte
		want   float64
	}{
		{"one byte per read", oneByOne, 0.991},
		{"header split from body", split(p991, 2), 0.991},
		{"split inside a word", split(p991, 1, 5, 9), 0.991},
		{"noise before the header", [][]byte{{0x12, 0x80, 0x34, 0x7F}, p991}, 0.991},
		{"two packets in one read, newest wins", [][]byte{append(append([]byte{}, p991...), p1200...)}, 1.2},
		{"packet cut after its status word, next packet read", [][]byte{p991[:4], p1200}, 1.2},
		{"129-word chain uses the length bit in the high header byte", [][]byte{isp2Packet(129, normal, lambdaWord(1200))}, 1.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			for _, ch := range tt.chunks {
				c.SetData(ch)
			}
			if got := c.GetLambda(); got != tt.want {
				t.Errorf("lambda = %v, want %v", got, tt.want)
			}
		})
	}
}

// Nothing is decoded until the whole packet has arrived.
func TestProcessBytesWaitsForWholePacket(t *testing.T) {
	c := newTestClient(t)
	p := isp2Packet(7, statusWord(ISP2_WARMING, 147), lambdaWord(991))
	c.SetData(p[:len(p)-1])
	if c.GetStatus() != ISP2_NORMAL || c.GetAFRMultiplier() != 0 || c.GetAFR() != 0 {
		t.Fatalf("partial packet decoded: status %d, multiplier %v, AFR %v", c.GetStatus(), c.GetAFRMultiplier(), c.GetAFR())
	}
	c.SetData(p[len(p)-1:])
	if c.GetStatus() != ISP2_WARMING {
		t.Errorf("status = %d after the last byte, want %d", c.GetStatus(), ISP2_WARMING)
	}
}

// The status word's AFR multiplier scales λ into AFR; λ is clamped to the
// 0.5-1.5 range the gauges display.
func TestLambdaAndAFR(t *testing.T) {
	tests := []struct {
		name       string
		status     [2]byte
		lambda     [2]byte
		multiplier float64
		wantLambda float64
		wantAFR    float64
	}{
		{"LM2.cs sample, gasoline", [2]byte{0x43, 0x13}, [2]byte{0x03, 0x6B}, 14.7, 0.991, 14.5677},
		{"methanol stoich", statusWord(ISP2_NORMAL, 64), lambdaWord(1000), 6.4, 1.0, 6.4},
		{"E85 rich", statusWord(ISP2_NORMAL, 98), lambdaWord(800), 9.8, 0.8, 7.84},
		{"richest reading", statusWord(ISP2_NORMAL, 147), [2]byte{0x00, 0x00}, 14.7, 0.5, 7.35},
		{"exactly 1.5", statusWord(ISP2_NORMAL, 147), lambdaWord(1500), 14.7, 1.5, 22.05},
		{"leaner than 1.5 is clamped", statusWord(ISP2_NORMAL, 147), [2]byte{0x7F, 0x7F}, 14.7, 1.5, 22.05},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			c.SetData(isp2Packet(7, tt.status, tt.lambda))
			if got := c.GetAFRMultiplier(); !near(got, tt.multiplier) {
				t.Errorf("AFR multiplier = %v, want %v", got, tt.multiplier)
			}
			if got := c.GetLambda(); !near(got, tt.wantLambda) {
				t.Errorf("lambda = %v, want %v", got, tt.wantLambda)
			}
			if got := c.GetAFR(); math.Abs(got-tt.wantAFR) > 1e-6 {
				t.Errorf("AFR = %v, want %v", got, tt.wantAFR)
			}
		})
	}
}

// The status word's function bits select the status; when λ is not valid
// GetLambda returns a fixed placeholder per status instead of the raw value.
func TestStatus(t *testing.T) {
	tests := []struct {
		status uint8
		lambda float64
	}{
		{ISP2_NORMAL, 0.991},
		{ISP2_O2, 1.5},
		{ISP2_CALIBRATING, 0.502},
		{ISP2_NEED_CALIBRATION, 0.503},
		{ISP2_WARMING, 0.504},
		{ISP2_HEATER_CALIBRATING, 0.505},
		{ISP2_LAMBDA_ERROR_CODE, 0.506},
		{ISP2_RESERVED, 0.507},
	}
	for _, tt := range tests {
		code := fmt.Sprintf("(%03b)", tt.status)
		t.Run(code, func(t *testing.T) {
			c := newTestClient(t)
			c.SetData(isp2Packet(7, statusWord(tt.status, 147), lambdaWord(991)))
			if got := c.GetStatus(); got != tt.status {
				t.Errorf("status = %d, want %d", got, tt.status)
			}
			if got := c.GetLambda(); got != tt.lambda {
				t.Errorf("lambda = %v, want %v", got, tt.lambda)
			}
			if got := c.GetLambdaStatus(); !strings.HasSuffix(got, code) {
				t.Errorf("lambda status = %q, want it to end in %s", got, code)
			}
		})
	}
}

// A header-only packet changes nothing and a packet with only the status word
// updates status and multiplier but keeps the last λ; neither desyncs the stream.
func TestShortPackets(t *testing.T) {
	c := newTestClient(t)
	c.SetData(isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(991)))

	c.SetData([]byte{0xB2, 0x80})
	if got := c.GetLambda(); got != 0.991 {
		t.Errorf("lambda = %v after a header-only packet, want 0.991", got)
	}

	sw := statusWord(ISP2_NORMAL, 64)
	c.SetData([]byte{0xB2, 0x81, sw[0], sw[1]})
	if got := c.GetAFRMultiplier(); !near(got, 6.4) {
		t.Errorf("AFR multiplier = %v after a status-only packet, want 6.4", got)
	}
	if got := c.GetLambda(); got != 0.991 {
		t.Errorf("lambda = %v after a status-only packet, want 0.991", got)
	}

	c.SetData(isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(1200)))
	if got := c.GetLambda(); got != 1.2 {
		t.Errorf("lambda = %v for the packet after the short ones, want 1.2", got)
	}
}

// A packet truncated after its first header byte must not cost the next
// packet: its header starts right behind the stray byte.
func TestStrayHeaderByteBeforePacket(t *testing.T) {
	c := newTestClient(t)
	c.SetData(append([]byte{0xB2}, isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(991))...))
	if got := c.GetLambda(); got != 0.991 {
		t.Errorf("lambda = %v, want 0.991", got)
	}
}

// String reports multiplier, AFR, λ and the status text.
func TestString(t *testing.T) {
	c := newTestClient(t)
	c.SetData([]byte{0xB2, 0x87, 0x43, 0x13, 0x03, 0x6B, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	want := "AFR stoich: 14.7, AFR: 14.568, λ: 0.991 - Lambda valid and Aux data valid, normal operation (000)"
	if got := c.String(); got != want {
		t.Errorf("String() = %q\nwant         %q", got, want)
	}
}

// logRecorder collects log lines from the client, which may log from its
// reader goroutine.
type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (l *logRecorder) log(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, s)
}

func (l *logRecorder) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func (l *logRecorder) count(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.lines {
		if strings.Contains(s, substr) {
			n++
		}
	}
	return n
}

// waitFor polls cond until it holds, failing the test after five seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !cond() {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for %s", what)
		case <-tick.C:
		}
	}
}

// With the "txbridge" port the bridge feeds bytes through SetData, so Start
// opens nothing.
func TestStartTxbridge(t *testing.T) {
	var rec logRecorder
	c, _ := NewISP2Client("txbridge", rec.log)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if c.getPort() != nil || rec.count("") != 0 {
		t.Errorf("Start opened a port or logged %q", rec.all())
	}
	c.SetData(isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(991)))
	if got := c.GetLambda(); got != 0.991 {
		t.Errorf("lambda = %v, want 0.991", got)
	}
}

// Start fails straight away when the port can't be opened.
func TestStartMissingPort(t *testing.T) {
	c, _ := NewISP2Client(filepath.Join(t.TempDir(), "ttyUSB9"), func(string) {})
	err := c.Start(context.Background())
	if !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "failed to open serial port: ") {
		t.Fatalf("Start error = %v, want a failed to open serial port error wrapping does-not-exist", err)
	}
	if c.getPort() != nil {
		t.Error("port stored after a failed open")
	}
}

// Stop runs once: one log line, and the shutdown signals are set.
func TestStopIdempotent(t *testing.T) {
	var rec logRecorder
	c, _ := NewISP2Client("test", rec.log)
	c.Stop()
	c.Stop()
	if n := rec.count("Stopping ISP2 client"); n != 1 {
		t.Errorf("logged Stopping %d times, want 1", n)
	}
	if !c.closed.Load() {
		t.Error("closed flag not set")
	}
	select {
	case <-c.done:
	default:
		t.Error("done channel not closed")
	}
}

// reconnect returns false without reopening anything once the context ends or
// the client is stopped, whether that happens before the first attempt or
// while it waits to retry a port that won't open.
func TestReconnectGivesUp(t *testing.T) {
	tests := []struct {
		name   string
		before func(c *ISP2Client, cancel context.CancelFunc)
		during func(c *ISP2Client, cancel context.CancelFunc)
	}{
		{name: "context already done", before: func(_ *ISP2Client, cancel context.CancelFunc) { cancel() }},
		{name: "already stopped", before: func(c *ISP2Client, _ context.CancelFunc) { c.Stop() }},
		{name: "stopped while waiting to retry", during: func(c *ISP2Client, _ context.CancelFunc) { c.Stop() }},
		{name: "context done while waiting to retry", during: func(_ *ISP2Client, cancel context.CancelFunc) { cancel() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rec logRecorder
			c, _ := NewISP2Client(filepath.Join(t.TempDir(), "ttyUSB9"), rec.log)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.before != nil {
				tt.before(c, cancel)
			}
			res := make(chan bool, 1)
			go func() { res <- c.reconnect(ctx) }()
			if tt.during != nil {
				waitFor(t, "a failed reconnect attempt", func() bool { return rec.count("isp2: reconnect failed") > 0 })
				tt.during(c, cancel)
			}
			select {
			case ok := <-res:
				if ok {
					t.Error("reconnect = true, want false")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("reconnect did not return")
			}
			if tt.before != nil && rec.count("reconnecting") != 0 {
				t.Error("reconnect attempted after shutdown")
			}
		})
	}
}

// run returns without reading once the context is done or the client stopped.
func TestRunReturnsOnShutdown(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprintf("stopped=%v", stop), func(t *testing.T) {
			var rec logRecorder
			c, _ := NewISP2Client("test", rec.log)
			ctx, cancel := context.WithCancel(context.Background())
			if stop {
				defer cancel()
				c.Stop()
			} else {
				cancel()
			}
			done := make(chan struct{})
			go func() { c.run(ctx); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("run did not return")
			}
			if rec.count("reconnecting") != 0 {
				t.Error("run tried to reconnect after shutdown")
			}
		})
	}
}

// Feeding bytes from one goroutine while another reads the values is safe.
func TestConcurrentSetDataAndGetters(t *testing.T) {
	c := newTestClient(t)
	p := isp2Packet(7, statusWord(ISP2_NORMAL, 147), lambdaWord(991))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			c.SetData(p[:5])
			c.SetData(p[5:])
		}
	}()
	for {
		select {
		case <-done:
			if got := c.GetLambda(); got != 0.991 {
				t.Errorf("lambda = %v, want 0.991", got)
			}
			return
		default:
			_ = c.GetLambda()
			_ = c.GetAFR()
			_ = c.GetStatus()
			_ = c.GetAFRMultiplier()
			_ = c.GetLambdaStatus()
			_ = c.String()
		}
	}
}
