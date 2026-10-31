package datalogger

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/serialcommand"
	"github.com/roffe/txlogger/pkg/ebus"
)

// Shared fixtures for the logger tests: an in-memory LogWriter, a recorder for
// OnMessage, an adapter wrapper that can intercept frames on their way to a
// pkg/ecusim ECU, and a fake txbridge dongle.

// recRow is one Write call seen by recWriter.
type recRow struct {
	ts    time.Time
	names []string
	vals  map[string]float64
	text  map[string]string
}

// recWriter keeps every row it is handed. Write returns err when set and
// calls onWrite (with the 1-based write count) before returning.
type recWriter struct {
	mu      sync.Mutex
	rows    []recRow
	err     error
	onWrite func(n int)
	notify  chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func newRecWriter() *recWriter {
	return &recWriter{notify: make(chan struct{}, 1), closed: make(chan struct{})}
}

func (w *recWriter) Write(ts time.Time, channels []Channel) error {
	row := recRow{ts: ts, vals: map[string]float64{}, text: map[string]string{}}
	for i := range channels {
		row.names = append(row.names, channels[i].Name)
		row.vals[channels[i].Name] = channels[i].Value()
		row.text[channels[i].Name] = channels[i].String()
	}
	w.mu.Lock()
	w.rows = append(w.rows, row)
	n, err, hook := len(w.rows), w.err, w.onWrite
	w.mu.Unlock()
	select {
	case w.notify <- struct{}{}:
	default:
	}
	if hook != nil {
		hook(n)
	}
	return err
}

func (w *recWriter) Close() error {
	w.once.Do(func() { close(w.closed) })
	return nil
}

func (w *recWriter) snapshot() []recRow {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.rows)
}

// waitFor blocks until a row satisfying ok has been written and returns it.
func (w *recWriter) waitFor(t *testing.T, d time.Duration, ok func(recRow) bool) recRow {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		for _, r := range w.snapshot() {
			if ok(r) {
				return r
			}
		}
		select {
		case <-w.notify:
		case <-deadline.C:
			t.Fatalf("no matching row after %v (%d rows written)", d, len(w.snapshot()))
		}
	}
}

// waitRows blocks until at least n rows have been written.
func (w *recWriter) waitRows(t *testing.T, n int, d time.Duration) []recRow {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		if rows := w.snapshot(); len(rows) >= n {
			return rows
		}
		select {
		case <-w.notify:
		case <-deadline.C:
			t.Fatalf("only %d of %d rows after %v", len(w.snapshot()), n, d)
		}
	}
}

// msgLog records OnMessage output and the latest error count.
type msgLog struct {
	mu     sync.Mutex
	msgs   []string
	notify chan struct{}
	errs   atomic.Int64
}

func newMsgLog(t *testing.T) *msgLog {
	m := &msgLog{notify: make(chan struct{}, 1)}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("messages:\n%s", strings.Join(m.all(), "\n"))
		}
	})
	return m
}

func (m *msgLog) add(s string) {
	m.mu.Lock()
	m.msgs = append(m.msgs, s)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
}

func (m *msgLog) all() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.msgs)
}

func (m *msgLog) has(sub string) bool {
	return slices.ContainsFunc(m.all(), func(s string) bool { return strings.Contains(s, sub) })
}

// wait blocks until a message containing sub has been seen.
func (m *msgLog) wait(t *testing.T, sub string, d time.Duration) {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for !m.has(sub) {
		select {
		case <-m.notify:
		case <-deadline.C:
			t.Fatalf("no message containing %q after %v", sub, d)
		}
	}
}

// testCfg is a logging config writing CSV into a temp dir at 50 Hz with no
// wideband, reporting messages and errors to msgs.
func testCfg(t *testing.T, ecu string, dev gocan.Adapter, syms []*symbol.Symbol, msgs *msgLog) Config {
	return Config{
		FilenamePrefix: "test",
		ECU:            ecu,
		Device:         dev,
		Symbols:        syms,
		Rate:           50,
		OnMessage:      msgs.add,
		CaptureCounter: func(int) {},
		ErrorCounter:   func(n int) { msgs.errs.Store(int64(n)) },
		FpsCounter:     func(int) {},
		LogFormat:      "CSV",
		LogPath:        t.TempDir(),
		WidebandConfig: WidebandConfig{Name: "None"},
	}
}

// session runs a logger's Start in the background and stops it at cleanup.
type session struct {
	cl       IClient
	finished chan struct{}
	err      error
}

func runLogger(t *testing.T, cl IClient) *session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s := &session{cl: cl, finished: make(chan struct{})}
	go func() {
		s.err = cl.Start(ctx)
		close(s.finished)
	}()
	t.Cleanup(func() {
		cl.Close()
		cancel()
		select {
		case <-s.finished:
		case <-time.After(5 * time.Second):
			t.Error("logger did not stop")
		}
	})
	return s
}

// wait returns Start's result, failing the test if it takes longer than d.
func (s *session) wait(t *testing.T, d time.Duration) error {
	t.Helper()
	select {
	case <-s.finished:
		return s.err
	case <-time.After(d):
		t.Fatalf("Start did not return within %v", d)
		return nil
	}
}

// hookAdapter wraps a fake ECU adapter. A non-empty name is reported as the
// adapter name; onSend sees every frame first and swallows it by returning
// true. fatal kills the bus the way a dying adapter does.
type hookAdapter struct {
	gocan.Adapter
	name   string
	onSend func(bus *gocan.Bus, f gocan.Frame) bool

	bus    *gocan.Bus
	opened chan struct{}
}

func newHook(ecu gocan.Adapter, name string, onSend func(*gocan.Bus, gocan.Frame) bool) *hookAdapter {
	return &hookAdapter{Adapter: ecu, name: name, onSend: onSend, opened: make(chan struct{})}
}

func (h *hookAdapter) Name() string { return h.name }

func (h *hookAdapter) Open(ctx context.Context, bus *gocan.Bus) error {
	h.bus = bus
	close(h.opened)
	return h.Adapter.Open(ctx, bus)
}

func (h *hookAdapter) Send(ctx context.Context, f gocan.Frame) error {
	if h.onSend != nil && h.onSend(h.bus, f) {
		return nil
	}
	return h.Adapter.Send(ctx, f)
}

func (h *hookAdapter) fatal(err error) {
	<-h.opened
	h.bus.Fatal(err)
}

// emit raises an adapter event on the bus.
func (h *hookAdapter) emit(e gocan.Event) {
	<-h.opened
	h.bus.Emit(e)
}

// tbCall is one Raw or Command call made on fakeTxbridge.
type tbCall struct {
	raw  bool
	cmd  byte
	data []byte
}

// fakeTxbridge emulates the txbridge dongle's host command surface around a
// fake ECU: CAN frames go to ecu (dropped when nil), Raw/Command calls are
// recorded, 'R'/'W' requests are served from a sparse RAM map and the
// autonomous log stream is whatever the test pushes.
type fakeTxbridge struct {
	ecu gocan.Adapter

	mu        sync.Mutex
	calls     []tbCall
	reqs      []tbCall
	subs      map[*tbSub]struct{}
	ram       map[uint32]byte
	writeErr  bool  // answer 'W' with an 'e' frame
	reqErr    error // fail every Request
	cmdErr    error // fail every Command
	started   chan struct{}
	stopped   chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

type tbSub struct {
	cmds []byte
	ch   chan *serialcommand.SerialCommand
}

func newFakeTxbridge(ecu gocan.Adapter) *fakeTxbridge {
	return &fakeTxbridge{
		ecu:     ecu,
		subs:    map[*tbSub]struct{}{},
		ram:     map[uint32]byte{},
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (f *fakeTxbridge) Name() string { return "txbridge wifi" }

func (f *fakeTxbridge) Open(ctx context.Context, bus *gocan.Bus) error {
	if f.ecu != nil {
		return f.ecu.Open(ctx, bus)
	}
	return nil
}

func (f *fakeTxbridge) Send(ctx context.Context, fr gocan.Frame) error {
	if f.ecu != nil {
		return f.ecu.Send(ctx, fr)
	}
	return nil
}

func (f *fakeTxbridge) Close() error { return nil }

func (f *fakeTxbridge) Command(cmd byte, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, tbCall{cmd: cmd, data: slices.Clone(data)})
	return f.cmdErr
}

func (f *fakeTxbridge) Raw(data []byte) error {
	f.mu.Lock()
	f.calls = append(f.calls, tbCall{raw: true, data: slices.Clone(data)})
	f.mu.Unlock()
	switch string(data) {
	case "r":
		f.startOnce.Do(func() { close(f.started) })
	case "s":
		f.stopOnce.Do(func() { close(f.stopped) })
	}
	return nil
}

func (f *fakeTxbridge) Subscribe(ctx context.Context, cmds ...byte) <-chan *serialcommand.SerialCommand {
	s := &tbSub{cmds: cmds, ch: make(chan *serialcommand.SerialCommand, 64)}
	f.mu.Lock()
	f.subs[s] = struct{}{}
	f.mu.Unlock()
	context.AfterFunc(ctx, func() { f.dropSub(s) })
	return s.ch
}

func (f *fakeTxbridge) dropSub(s *tbSub) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.subs[s]; ok {
		delete(f.subs, s)
		close(s.ch)
	}
}

// closeSubs ends every subscription, as the adapter does when its link dies.
func (f *fakeTxbridge) closeSubs() {
	f.mu.Lock()
	subs := make([]*tbSub, 0, len(f.subs))
	for s := range f.subs {
		subs = append(subs, s)
	}
	f.mu.Unlock()
	for _, s := range subs {
		f.dropSub(s)
	}
}

func (f *fakeTxbridge) Request(_ context.Context, cmd byte, data []byte, _ ...byte) (*serialcommand.SerialCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, tbCall{cmd: cmd, data: slices.Clone(data)})
	if f.reqErr != nil {
		return nil, f.reqErr
	}
	addr := binary.LittleEndian.Uint32(data)
	n := int(data[4])
	switch cmd {
	case 'R':
		out := make([]byte, n)
		for i := range out {
			out[i] = f.ram[addr+uint32(i)]
		}
		return serialcommand.NewSerialCommand('R', out), nil
	case 'W':
		if f.writeErr {
			return serialcommand.NewSerialCommand('e', []byte{0x01}), nil
		}
		for i, b := range data[5 : 5+n] {
			f.ram[addr+uint32(i)] = b
		}
		return serialcommand.NewSerialCommand('W', nil), nil
	}
	return nil, errors.New("unsupported request")
}

// push delivers one autonomous frame (e.g. an 'r' log frame) to subscribers.
func (f *fakeTxbridge) push(cmd byte, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for s := range f.subs {
		if slices.Contains(s.cmds, cmd) {
			s.ch <- serialcommand.NewSerialCommand(cmd, data)
		}
	}
}

func (f *fakeTxbridge) callList() []tbCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeTxbridge) reqList() []tbCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reqs)
}

func (f *fakeTxbridge) poke(addr uint32, b []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, v := range b {
		f.ram[addr+uint32(i)] = v
	}
}

func (f *fakeTxbridge) peek(addr uint32, n int) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, n)
	for i := range out {
		out[i] = f.ram[addr+uint32(i)]
	}
	return out
}

// waitClosed fails the test if ch is not closed within d.
func waitClosed(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// ebusValues collects everything published on topic for the rest of the test.
type ebusValues struct {
	mu     sync.Mutex
	vals   []float64
	notify chan struct{}
}

func collectEbus(t *testing.T, topic string) *ebusValues {
	e := &ebusValues{notify: make(chan struct{}, 1)}
	t.Cleanup(ebus.SubscribeFunc(topic, func(v float64) {
		e.mu.Lock()
		e.vals = append(e.vals, v)
		e.mu.Unlock()
		select {
		case e.notify <- struct{}{}:
		default:
		}
	}))
	return e
}

func (e *ebusValues) all() []float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.vals)
}

func (e *ebusValues) last() (float64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.vals) == 0 {
		return 0, false
	}
	return e.vals[len(e.vals)-1], true
}

// waitValue blocks until want has been published.
func (e *ebusValues) waitValue(t *testing.T, want float64, d time.Duration) {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for !slices.Contains(e.all(), want) {
		select {
		case <-e.notify:
		case <-deadline.C:
			t.Fatalf("%v never published (got %v)", want, e.all())
		}
	}
}
