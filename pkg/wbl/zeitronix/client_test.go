package zeitronix

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// packet builds a ZT-2 packet: header 00 01 02, the AFR byte (AFR x10), EGT,
// RPM and MAP little endian, then TPS, USER1 and the two config registers.
func packet(afr byte, egt, rpm, mapv uint16) []byte {
	return []byte{
		0x00, 0x01, 0x02, afr,
		byte(egt), byte(egt >> 8),
		byte(rpm), byte(rpm >> 8),
		byte(mapv), byte(mapv >> 8),
		0x10, 0x20, 0x30, 0x40,
	}
}

// feedAll runs stream through z.feed from step 0 and returns the final step.
func feedAll(z *Zeitronix, stream []byte) int {
	cmd := make([]byte, 14)
	step := 0
	for _, b := range stream {
		step = z.feed(cmd, step, b)
	}
	return step
}

// eventually polls cond until it holds or waitTimeout passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		<-tick.C
	}
}

// feed walks the header bytes 00 01 02 and the eleven data bytes, returning
// to step 0 after the 14th byte.
func TestFeedSteps(t *testing.T) {
	z, _ := NewZeitronixClient("test", func(string) {})
	cmd := make([]byte, 14)
	step := 0
	for i, b := range packet(147, 0, 0, 0) {
		step = z.feed(cmd, step, b)
		want := i + 1
		if i == 13 {
			want = 0
		}
		if step != want {
			t.Fatalf("after byte %d step = %d, want %d", i, step, want)
		}
	}
}

// feed finds the 00 01 02 header in a noisy stream: junk is skipped, a 00
// that breaks a header starts the next one, header bytes inside the data are
// plain data, an unfinished packet is not applied and the latest complete
// packet wins.
func TestFeedStreams(t *testing.T) {
	clean := packet(147, 850, 3000, 120)
	const cleanStr = "Lambda: 1.000, EGT: 850, RPM: 3000, MAP: 120"
	cat := func(parts ...[]byte) []byte { return slices.Concat(parts...) }

	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"clean packet", clean, cleanStr},
		{"junk before header", cat([]byte{0xFF, 0x01, 0x02, 0x93}, clean), cleanStr},
		{"header broken by 00", cat([]byte{0x00, 0x01}, clean), cleanStr},
		{"header broken by junk", cat([]byte{0x00, 0x01, 0x7F}, clean), cleanStr},
		{"header bytes in data", packet(0x00, 0x0201, 0x0100, 0x0002), "Lambda: 0.000, EGT: 513, RPM: 256, MAP: 2"},
		{"unfinished packet", clean[:13], "Lambda: 0.000, EGT: 0, RPM: 0, MAP: 0"},
		{"latest packet wins", cat(clean, packet(100, 900, 6500, 250)), "Lambda: 0.680, EGT: 900, RPM: 6500, MAP: 250"},
		{"lean limit", packet(255, 0xFFFF, 0, 0), "Lambda: 1.735, EGT: 65535, RPM: 0, MAP: 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			z, _ := NewZeitronixClient("test", func(string) {})
			feedAll(z, tt.in)
			if got := z.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// SetData converts the AFR byte (AFR x10, 147 = stoichiometric) to lambda and
// rejects short packets or a wrong header without changing the reading.
func TestSetData(t *testing.T) {
	z, _ := NewZeitronixClient("test", func(string) {})
	if err := z.SetData(packet(147, 850, 3000, 120)); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	if got := z.GetLambda(); got != 1 {
		t.Errorf("lambda = %v, want 1 for AFR byte 147", got)
	}
	const want = "Lambda: 1.000, EGT: 850, RPM: 3000, MAP: 120"

	badHeader := packet(100, 1, 2, 3)
	badHeader[2] = 0x03
	for _, tc := range []struct {
		name string
		in   []byte
		err  string
	}{
		{"short", packet(100, 1, 2, 3)[:13], "invalid data length"},
		{"empty", nil, "invalid data length"},
		{"bad header", badHeader, "invalid data format"},
	} {
		err := z.SetData(tc.in)
		if err == nil || err.Error() != tc.err {
			t.Errorf("%s: SetData error = %v, want %q", tc.name, err, tc.err)
		}
		if got := z.String(); got != want {
			t.Errorf("%s: reading changed to %q", tc.name, got)
		}
	}
}

// startHandler runs z.serialHandler in the background and returns a channel
// closed when it returns.
func startHandler(ctx context.Context, z *Zeitronix) <-chan struct{} {
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		z.serialHandler(ctx)
	}()
	return exited
}

// serialHandler assembles packets split across reads (and skips empty reads);
// cancelling the context stops it and closes the port.
func TestSerialHandlerSplitReads(t *testing.T) {
	t.Parallel()
	rec := newLogRec()
	z, _ := NewZeitronixClient(missingPort(t), rec.log)
	f := newFakePort()
	z.p = f
	ctx, cancel := context.WithCancel(context.Background())
	exited := startHandler(ctx, z)

	p1 := packet(147, 850, 3000, 120)
	p2 := packet(120, 700, 2500, 95)
	f.feed(p1[:2])
	f.feed(nil)
	f.feed(p1[2:9])
	f.feed(append(slices.Clone(p1[9:]), p2[:5]...))
	f.feed(p2[5:])
	const want = "Lambda: 0.816, EGT: 700, RPM: 2500, MAP: 95"
	eventually(t, "the second packet", func() bool { return z.String() == want })

	cancel()
	waitClosed(t, exited, "serialHandler to return after cancel")
	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
	if z.getPort() != nil {
		t.Error("port still set after serialHandler returned")
	}
	if lines := rec.all(); len(lines) != 0 {
		t.Errorf("logged %q, want nothing", lines)
	}
}

// A read error is logged, the port is closed and the handler keeps trying to
// reopen it until Stop.
func TestSerialHandlerReconnectsAfterReadError(t *testing.T) {
	t.Parallel()
	port := missingPort(t)
	rec := newLogRec()
	z, _ := NewZeitronixClient(port, rec.log)
	f := newFakePort()
	z.p = f
	exited := startHandler(context.Background(), z)

	f.fail(errors.New("device gone"))
	rec.waitFor(t, "Zeitronix: reconnect failed: ")
	lines := rec.all()
	if len(lines) < 3 || lines[0] != "Zeitronix read error: device gone" || lines[1] != "Zeitronix: reconnecting to "+port {
		t.Errorf("logs = %q, want the read error, then a reconnect to %s", lines, port)
	}
	if n := f.closeCount(); n != 1 {
		t.Errorf("dropped port closed %d times, want 1", n)
	}

	z.Stop()
	waitClosed(t, exited, "serialHandler to return after Stop")
	if n := f.closeCount(); n != 1 {
		t.Errorf("dropped port closed %d times after Stop, want 1", n)
	}
}

// Stop makes the pending read fail, which the handler takes as shutdown: it
// returns without logging a read error or reconnecting.
func TestStopDuringReadIsNotAnError(t *testing.T) {
	t.Parallel()
	rec := newLogRec()
	z, _ := NewZeitronixClient(missingPort(t), rec.log)
	f := newFakePort()
	z.p = f
	exited := startHandler(context.Background(), z)

	f.feed(packet(147, 0, 0, 0))
	eventually(t, "a packet", func() bool { return z.GetLambda() == 1 })
	z.Stop()
	waitClosed(t, exited, "serialHandler to return after Stop")

	if got, want := rec.all(), []string{"Closing Zeitronix client"}; !slices.Equal(got, want) {
		t.Errorf("logs = %q, want %q", got, want)
	}
	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
}

// With no port open the handler goes straight to reconnecting and gives up
// when the context is cancelled during the retry wait.
func TestSerialHandlerWithoutPortReconnectsUntilCancel(t *testing.T) {
	t.Parallel()
	port := missingPort(t)
	rec := newLogRec()
	z, _ := NewZeitronixClient(port, rec.log)
	ctx, cancel := context.WithCancel(context.Background())
	exited := startHandler(ctx, z)

	rec.waitFor(t, "Zeitronix: reconnect failed: ")
	if lines := rec.all(); lines[0] != "Zeitronix: reconnecting to "+port {
		t.Errorf("first log = %q, want a reconnect to %s", lines[0], port)
	}
	cancel()
	waitClosed(t, exited, "serialHandler to return after cancel")
}

// A handler started with an already cancelled context returns without
// reading and still closes the port Start opened for it.
func TestSerialHandlerCancelledBeforeStart(t *testing.T) {
	rec := newLogRec()
	z, _ := NewZeitronixClient(missingPort(t), rec.log)
	f := newFakePort()
	z.p = f
	f.feed(packet(147, 850, 3000, 120))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	z.serialHandler(ctx)

	if n := len(f.events); n != 1 {
		t.Errorf("handler read from the port after cancel (%d queued reads left, want 1)", n)
	}
	if z.GetLambda() != 0 {
		t.Errorf("lambda = %v, want 0: a packet was decoded after cancel", z.GetLambda())
	}
	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
	if lines := rec.all(); len(lines) != 0 {
		t.Errorf("logs = %q, want nothing", lines)
	}
}

// Once the context is done or the client is stopped, reconnect gives up
// without trying to open the port.
func TestReconnectAfterShutdown(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		rec := newLogRec()
		z, _ := NewZeitronixClient(missingPort(t), rec.log)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if z.reconnect(ctx) {
			t.Error("reconnect = true after cancel")
		}
		if lines := rec.all(); len(lines) != 0 {
			t.Errorf("logs = %q, want no reconnect attempt", lines)
		}
	})
	t.Run("stopped", func(t *testing.T) {
		rec := newLogRec()
		z, _ := NewZeitronixClient(missingPort(t), rec.log)
		z.Stop()
		if z.reconnect(context.Background()) {
			t.Error("reconnect = true after Stop")
		}
		if got, want := rec.all(), []string{"Closing Zeitronix client"}; !slices.Equal(got, want) {
			t.Errorf("logs = %q, want %q (no reconnect attempt)", got, want)
		}
	})
}

// Start reports a port that cannot be opened instead of starting the reader.
func TestStartMissingPort(t *testing.T) {
	z, _ := NewZeitronixClient(missingPort(t), func(string) {})
	if err := z.Start(context.Background()); err == nil {
		z.Stop()
		t.Fatal("Start on a missing port succeeded")
	}
	if z.getPort() != nil {
		t.Error("port set after a failed open")
	}
}

// closePort logs a failing Close and forgets the port, so later calls do
// nothing; Stop runs once however often it is called.
func TestCloseAndStopAreIdempotent(t *testing.T) {
	rec := newLogRec()
	z, _ := NewZeitronixClient("test", rec.log)
	f := newFakePort()
	f.closeErr = errors.New("busy")
	z.p = f

	z.closePort()
	z.closePort()
	z.Stop()
	z.Stop()

	if n := f.closeCount(); n != 1 {
		t.Errorf("port closed %d times, want 1", n)
	}
	want := []string{"Zeitronix: busy", "Closing Zeitronix client"}
	if got := rec.all(); !slices.Equal(got, want) {
		t.Errorf("logs = %q, want %q", got, want)
	}
	select {
	case <-z.done:
	default:
		t.Error("done not closed by Stop")
	}
}

// A new client keeps its port name and reports no reading, and the product
// string matches the WBL type name saved in user settings.
func TestNewClient(t *testing.T) {
	z, err := NewZeitronixClient("COM4", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if z.Port != "COM4" {
		t.Errorf("Port = %q", z.Port)
	}
	if got := z.String(); got != "Lambda: 0.000, EGT: 0, RPM: 0, MAP: 0" {
		t.Errorf("String() = %q", got)
	}
	if ProductString != "Zeitronix ZT-2" {
		t.Errorf("ProductString = %q", ProductString)
	}
}
