package stag

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"testing"
	"time"
)

// dataSample is a 0xE4 data frame from STAG.cs: status work, sensor in free
// air (lambda 40.228, oxygen 19.4%).
var dataSample = []byte{
	0x32, 0x00, 0x00, 0x1F, 0xE4, 0x00, 0x02, 0x00,
	0x00, 0x09, 0x83, 0xAE, 0x00, 0x00, 0x9D, 0x24,
	0x00, 0xC2, 0x00, 0x0A, 0x6A, 0x00, 0x41, 0x11,
	0x00, 0x04, 0x00, 0x00, 0x00, 0x78, 0x01, 0x2C,
	0x01, 0xF4, 0x58,
}

// Requests the client answers each reply type with.
var (
	reqAfter80 = []byte{0x32, 0x00, 0x00, 0x03, 0x03, 0x00, 0x38}
	reqAfter83 = []byte{0x32, 0x00, 0x00, 0x03, 0x6D, 0x00, 0xA2}
	reqPoll    = []byte{0x32, 0x00, 0x00, 0x03, 0x64, 0x00, 0x99}
)

// frame builds a STAG frame: 0x32, two zero bytes, the length of payload
// plus checksum, the payload, and the sum of all earlier bytes.
func frame(payload ...byte) []byte {
	f := append([]byte{0x32, 0x00, 0x00, byte(len(payload) + 1)}, payload...)
	var sum byte
	for _, b := range f {
		sum += b
	}
	return append(f, sum)
}

// dataFrame returns dataSample with the status, lambda (in thousandths) and
// oxygen (in tenths of a percent) replaced and the checksum fixed up.
func dataFrame(status byte, lambda uint32, oxygen uint16) []byte {
	payload := slices.Clone(dataSample[4 : len(dataSample)-1])
	payload[6-4] = status
	binary.BigEndian.PutUint32(payload[12-4:], lambda)
	binary.BigEndian.PutUint16(payload[16-4:], oxygen)
	return frame(payload...)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Every request the client sends ends in the sum of its earlier bytes, the
// same checksum rule the STAG's replies follow, and the frame builder used by
// these tests reproduces the captured STAG.cs data frame byte for byte.
func TestRequestChecksums(t *testing.T) {
	for _, req := range [][]byte{initRequest, reqAfter80, reqAfter83, reqPoll, dataSample} {
		var sum byte
		for _, b := range req[:len(req)-1] {
			sum += b
		}
		if sum != req[len(req)-1] {
			t.Errorf("% X: checksum %02X, want %02X", req, req[len(req)-1], sum)
		}
	}
	if got := frame(dataSample[4 : len(dataSample)-1]...); !bytes.Equal(got, dataSample) {
		t.Errorf("frame() = % X, want the STAG.cs sample % X", got, dataSample)
	}
}

// The parser returns each complete frame with a valid checksum; it skips
// junk before the start byte, drops frames with a bad checksum without
// losing the next one, and recovers after a truncated frame swallows part of
// the following frame.
func TestParserStreams(t *testing.T) {
	other := dataFrame(0x02, 1000, 209)
	badSum := slices.Clone(dataSample)
	badSum[len(badSum)-1]++

	cat := func(parts ...[]byte) []byte { return slices.Concat(parts...) }
	tests := []struct {
		name string
		in   []byte
		want [][]byte
	}{
		{"one frame", dataSample, [][]byte{dataSample}},
		{"back to back", cat(dataSample, other), [][]byte{dataSample, other}},
		{"junk before start byte", cat([]byte{0x01, 0x07, 0xFF, 0x00}, dataSample), [][]byte{dataSample}},
		{"bad checksum then good frame", cat(badSum, other), [][]byte{other}},
		{"short reply frame", frame(0x80), [][]byte{frame(0x80)}},
		{"truncated frame", dataSample[:20], nil},
		{"truncated frame then two frames", cat(dataSample[:10], dataSample, other), [][]byte{other}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p parser
			var got [][]byte
			for _, b := range tt.in {
				if f := p.feed(b); f != nil {
					got = append(got, slices.Clone(f))
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d frames % X, want %d", len(got), got, len(tt.want))
			}
			for i := range got {
				if !bytes.Equal(got[i], tt.want[i]) {
					t.Errorf("frame %d = % X, want % X", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// SetData logs each sensor status change once and only takes lambda and
// oxygen from frames reporting status work; in any other status the last
// working reading is kept.
func TestSetDataStatus(t *testing.T) {
	rec := newLogRec()
	a, _ := NewSTAGClient("test", rec.log)

	steps := []struct {
		status byte
		lambda uint32
		oxygen uint16
		log    string // new log line, "" for none
		want   float64
		str    string
	}{
		{0x00, 1000, 209, "Stag: status_sleep", 0, "Lambda: 0.0000, Oxygen: 0.000"},
		{0x01, 1000, 209, "Stag: status_warming", 0, "Lambda: 0.0000, Oxygen: 0.000"},
		{0x02, 1000, 209, "Stag: status_work", 1.0, "Lambda: 1.0000, Oxygen: 20.900"},
		{0x02, 850, 0, "", 0.85, "Lambda: 0.8500, Oxygen: 0.000"},
		{0x03, 1200, 100, "Stag: status_breakdown", 0.85, "Lambda: 0.8500, Oxygen: 0.000"},
		{0x07, 1200, 100, "", 0.85, "Lambda: 0.8500, Oxygen: 0.000"},
		{0x02, 0x00009D24, 0x00C2, "Stag: status_work", 40.228, "Lambda: 40.2280, Oxygen: 19.400"},
	}
	for i, s := range steps {
		before := len(rec.all())
		if err := a.SetData(dataFrame(s.status, s.lambda, s.oxygen)); err != nil {
			t.Fatalf("step %d: SetData: %v", i, err)
		}
		lines := rec.all()[before:]
		switch {
		case s.log == "" && len(lines) != 0:
			t.Errorf("step %d (status %02X): logged %q, want nothing", i, s.status, lines)
		case s.log != "" && !slices.Equal(lines, []string{s.log}):
			t.Errorf("step %d (status %02X): logged %q, want %q", i, s.status, lines, s.log)
		}
		if got := a.GetLambda(); !near(got, s.want) {
			t.Errorf("step %d (status %02X): lambda = %v, want %v", i, s.status, got, s.want)
		}
		if got := a.String(); got != s.str {
			t.Errorf("step %d: String() = %q, want %q", i, got, s.str)
		}
	}
}

// SetData rejects frames too short to hold the oxygen bytes without touching
// the reading.
func TestSetDataTooShort(t *testing.T) {
	a, _ := NewSTAGClient("test", func(string) {})
	err := a.SetData(dataSample[:17])
	if err == nil || err.Error() != "stag: data frame too short: 17 bytes" {
		t.Errorf("SetData(17 bytes) error = %v, want too short", err)
	}
	if a.GetLambda() != 0 {
		t.Errorf("lambda = %v after a rejected frame, want 0", a.GetLambda())
	}
}

// processPacket answers each reply type with the next request of the
// handshake (0x80 -> 03, 0x83 -> 6D, 0xF0 -> poll), stores 0xE4 data frames
// and polls again even when the data frame is unusable, and ignores unknown
// or headerless frames.
func TestProcessPacket(t *testing.T) {
	tests := []struct {
		name   string
		in     []byte
		want   []byte // request written, nil for none
		lambda float64
		log    string
	}{
		{"init reply", frame(0x80), reqAfter80, 0, ""},
		{"0x83 reply", frame(0x83, 0x01), reqAfter83, 0, ""},
		{"0xF0 reply", frame(0xF0), reqPoll, 0, ""},
		{"data frame", dataSample, reqPoll, 40.228, "Stag: status_work"},
		{"short data frame", frame(0xE4, 0x00, 0x02), reqPoll, 0, "stag: data frame too short: 8 bytes"},
		{"unknown reply", frame(0x55), nil, 0, ""},
		{"no command byte", []byte{0x32, 0x00, 0x00, 0x00}, nil, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := newLogRec()
			a, _ := NewSTAGClient("test", rec.log)
			f := newFakePort()
			a.sp = f

			a.processPacket(tt.in)

			var got [][]byte
			for len(f.writes) > 0 {
				got = append(got, <-f.writes)
			}
			switch {
			case tt.want == nil && len(got) != 0:
				t.Errorf("wrote % X, want nothing", got)
			case tt.want != nil && (len(got) != 1 || !bytes.Equal(got[0], tt.want)):
				t.Errorf("wrote % X, want % X", got, tt.want)
			}
			if got := a.GetLambda(); !near(got, tt.lambda) {
				t.Errorf("lambda = %v, want %v", got, tt.lambda)
			}
			if lines := rec.all(); tt.log == "" && len(lines) != 0 || tt.log != "" && !slices.Equal(lines, []string{tt.log}) {
				t.Errorf("logged %q, want %q", lines, tt.log)
			}
		})
	}
}

// Once the port has been closed, requests are dropped instead of being
// written to the stale port, and no port is opened to send them.
func TestSendRequestAfterPortClosed(t *testing.T) {
	a, _ := NewSTAGClient("test", func(string) {})
	f := newFakePort()
	a.sp = f
	a.closePort()

	a.processPacket(frame(0x80))

	if n := len(f.writes); n != 0 {
		t.Errorf("wrote %d requests to the closed port, want 0", n)
	}
	if a.getPort() != nil {
		t.Error("sendRequest opened a port")
	}
}

// Once the context is done or the client is stopped, reconnect gives up
// without trying to open the port.
func TestReconnectAfterShutdown(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		rec := newLogRec()
		a, _ := NewSTAGClient(missingPort(t), rec.log)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if a.reconnect(ctx) {
			t.Error("reconnect = true after cancel")
		}
		if lines := rec.all(); len(lines) != 0 {
			t.Errorf("logs = %q, want no reconnect attempt", lines)
		}
	})
	t.Run("stopped", func(t *testing.T) {
		rec := newLogRec()
		a, _ := NewSTAGClient(missingPort(t), rec.log)
		a.Stop()
		if a.reconnect(context.Background()) {
			t.Error("reconnect = true after Stop")
		}
		if got, want := rec.all(), []string{"Stopping Stag serial client"}; !slices.Equal(got, want) {
			t.Errorf("logs = %q, want %q (no reconnect attempt)", got, want)
		}
	})
}

// startSession runs a.session on f in the background and returns a channel
// closed when it returns.
func startSession(ctx context.Context, a *STAG, f *fakePort) <-chan struct{} {
	a.sp = f
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		a.session(ctx, f)
	}()
	return exited
}

// A session opens with the init request and walks the handshake: each reply,
// even when split across reads and preceded by junk, is answered with the
// next request, data frames update the reading, and a read error ends the
// session with the error logged.
func TestSessionHandshake(t *testing.T) {
	t.Parallel()
	rec := newLogRec()
	a, _ := NewSTAGClient("test", rec.log)
	f := newFakePort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exited := startSession(ctx, a, f)
	defer f.Close()

	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("first request = % X, want init % X", got, initRequest)
	}

	r80 := frame(0x80)
	f.feed(r80[:3])
	f.feed(r80[3:])
	if got := nextWrite(t, f); !bytes.Equal(got, reqAfter80) {
		t.Fatalf("after 0x80 reply wrote % X, want % X", got, reqAfter80)
	}
	f.feed(frame(0x83, 0x01))
	if got := nextWrite(t, f); !bytes.Equal(got, reqAfter83) {
		t.Fatalf("after 0x83 reply wrote % X, want % X", got, reqAfter83)
	}
	f.feed(frame(0xF0))
	if got := nextWrite(t, f); !bytes.Equal(got, reqPoll) {
		t.Fatalf("after 0xF0 reply wrote % X, want % X", got, reqPoll)
	}

	data := slices.Concat([]byte{0x00, 0xFF}, dataFrame(0x02, 1000, 209))
	for chunk := range slices.Chunk(data, 5) {
		f.feed(chunk)
	}
	f.feed(nil) // a read that returns no bytes
	if got := nextWrite(t, f); !bytes.Equal(got, reqPoll) {
		t.Fatalf("after data frame wrote % X, want % X", got, reqPoll)
	}
	if got := a.GetLambda(); got != 1 {
		t.Errorf("lambda = %v, want 1", got)
	}

	f.fail(errors.New("device gone"))
	waitClosed(t, exited, "session to end on read error")
	if !slices.Contains(rec.all(), "device gone") {
		t.Errorf("logs %q do not report the read error", rec.all())
	}
}

// When no valid frame arrives for replyTimeout the session logs once per
// stall, drops any partial frame and resends the init request every
// replyTimeout; a valid frame ends the stall so the next one is logged again.
func TestSessionRestartsHandshakeWhenRepliesStop(t *testing.T) {
	t.Parallel()
	const stallMsg = "Stag: no reply, restarting handshake"
	rec := newLogRec()
	a, _ := NewSTAGClient("test", rec.log)
	f := newFakePort()
	ctx, cancel := context.WithCancel(context.Background())
	exited := startSession(ctx, a, f)
	defer func() {
		cancel()
		waitClosed(t, exited, "session to end")
		f.Close()
	}()

	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("first request = % X, want init", got)
	}
	first := time.Now()
	f.feed(dataSample[:10]) // the reply breaks off

	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("after a stall wrote % X, want init", got)
	}
	if waited := time.Since(first); waited < replyTimeout/2 {
		t.Errorf("handshake restarted after %v, want about %v", waited, replyTimeout)
	}
	if n := rec.count(stallMsg); n != 1 {
		t.Errorf("stall logged %d times, want 1", n)
	}
	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("second stall wrote % X, want init", got)
	}
	if n := rec.count(stallMsg); n != 1 {
		t.Errorf("continued stall logged %d times, want 1", n)
	}

	// The partial frame from before the stall must not swallow this one.
	f.feed(dataSample)
	if got := nextWrite(t, f); !bytes.Equal(got, reqPoll) {
		t.Fatalf("after data frame wrote % X, want poll", got)
	}
	if got := a.GetLambda(); !near(got, 40.228) {
		t.Errorf("lambda = %v, want 40.228", got)
	}

	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("after replies stopped again wrote % X, want init", got)
	}
	if n := rec.count(stallMsg); n != 2 {
		t.Errorf("new stall after a good frame: logged %d times in total, want 2", n)
	}
}

// startRun runs a.run in the background and returns a channel closed when
// it returns.
func startRun(ctx context.Context, a *STAG) <-chan struct{} {
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		a.run(ctx)
	}()
	return exited
}

// When the port drops, run closes it, logs the error and keeps trying to
// reopen it until Stop; the dropped port is closed exactly once.
func TestRunReconnectsAfterPortError(t *testing.T) {
	t.Parallel()
	port := missingPort(t)
	rec := newLogRec()
	a, _ := NewSTAGClient(port, rec.log)
	f := newFakePort()
	a.sp = f
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exited := startRun(ctx, a)

	if got := nextWrite(t, f); !bytes.Equal(got, initRequest) {
		t.Fatalf("first request = % X, want init", got)
	}
	f.fail(errors.New("device gone"))
	rec.waitFor(t, "Stag: reconnect failed: ")

	lines := rec.all()
	if len(lines) < 3 || lines[0] != "device gone" || lines[1] != "Stag: reconnecting to "+port {
		t.Errorf("logs = %q, want the read error, then a reconnect to %s", lines, port)
	}
	if n := f.closeCount(); n != 1 {
		t.Errorf("dropped port closed %d times, want 1", n)
	}
	if a.getPort() != nil {
		t.Error("dropped port still set")
	}

	a.Stop()
	waitClosed(t, exited, "run to return after Stop")
	if n := f.closeCount(); n != 1 {
		t.Errorf("dropped port closed %d times after Stop, want 1", n)
	}
}

// Stop ends run without a reconnect attempt, even though closing the port
// makes the pending read fail.
func TestRunStopDoesNotReconnect(t *testing.T) {
	t.Parallel()
	rec := newLogRec()
	a, _ := NewSTAGClient(missingPort(t), rec.log)
	f := newFakePort()
	a.sp = f
	exited := startRun(context.Background(), a)

	nextWrite(t, f) // session running
	a.Stop()
	waitClosed(t, exited, "run to return after Stop")

	if rec.hasPrefix("Stag: reconnecting") {
		t.Errorf("logs %q: Stop triggered a reconnect", rec.all())
	}
	if rec.count("Stopping Stag serial client") != 1 {
		t.Errorf("logs %q, want the stop message once", rec.all())
	}
	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
}

// Cancelling the context ends run and closes the port without reconnecting.
func TestRunExitsOnCancel(t *testing.T) {
	t.Parallel()
	rec := newLogRec()
	a, _ := NewSTAGClient(missingPort(t), rec.log)
	f := newFakePort()
	a.sp = f
	ctx, cancel := context.WithCancel(context.Background())
	exited := startRun(ctx, a)

	nextWrite(t, f)
	cancel()
	waitClosed(t, exited, "run to return after cancel")

	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
	if a.getPort() != nil {
		t.Error("port still set after run returned")
	}
	if rec.hasPrefix("Stag: reconnecting") {
		t.Errorf("logs %q: cancel triggered a reconnect", rec.all())
	}
}

// A run started with an already cancelled context returns at once and still
// closes the port Start opened for it.
func TestRunCancelledBeforeStart(t *testing.T) {
	rec := newLogRec()
	a, _ := NewSTAGClient(missingPort(t), rec.log)
	f := newFakePort()
	a.sp = f
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.run(ctx)
	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
	if len(f.writes) != 0 || len(rec.all()) != 0 {
		t.Errorf("run did work after cancel: %d writes, logs %q", len(f.writes), rec.all())
	}
}

// With no port open, run goes straight to reconnecting and gives up when
// the context is cancelled during the retry wait.
func TestRunWithoutPortReconnectsUntilCancel(t *testing.T) {
	t.Parallel()
	port := missingPort(t)
	rec := newLogRec()
	a, _ := NewSTAGClient(port, rec.log)
	ctx, cancel := context.WithCancel(context.Background())
	exited := startRun(ctx, a)

	rec.waitFor(t, "Stag: reconnect failed: ")
	if lines := rec.all(); lines[0] != "Stag: reconnecting to "+port {
		t.Errorf("first log = %q, want a reconnect to %s", lines[0], port)
	}
	cancel()
	waitClosed(t, exited, "run to return after cancel")
}

// Start reports a port that cannot be opened instead of starting the reader.
func TestStartMissingPort(t *testing.T) {
	rec := newLogRec()
	a, _ := NewSTAGClient(missingPort(t), rec.log)
	if err := a.Start(context.Background()); err == nil {
		a.Stop()
		t.Fatal("Start on a missing port succeeded")
	}
	if a.getPort() != nil {
		t.Error("port set after a failed open")
	}
}

// closePort logs a failing Close and forgets the port, so later calls do
// nothing; Stop runs once however often it is called.
func TestCloseAndStopAreIdempotent(t *testing.T) {
	rec := newLogRec()
	a, _ := NewSTAGClient("test", rec.log)
	f := newFakePort()
	f.closeErr = errors.New("busy")
	a.sp = f

	a.closePort()
	a.closePort()
	a.Stop()
	a.Stop()

	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
	want := []string{"busy", "Stopping Stag serial client"}
	if got := rec.all(); !slices.Equal(got, want) {
		t.Errorf("logs = %q, want %q", got, want)
	}
	if !a.closed.Load() {
		t.Error("closed flag not set by Stop")
	}
	select {
	case <-a.done:
	default:
		t.Error("done not closed by Stop")
	}
}

// A new client reports no reading until a data frame arrives, and the
// product string matches the WBL type name saved in user settings.
func TestNewClientString(t *testing.T) {
	a, err := NewSTAGClient("COM3", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.String(); got != "Lambda: 0.0000, Oxygen: 0.000" {
		t.Errorf("String() = %q", got)
	}
	if ProductString != "Stag AFR" {
		t.Errorf("ProductString = %q", ProductString)
	}
}
