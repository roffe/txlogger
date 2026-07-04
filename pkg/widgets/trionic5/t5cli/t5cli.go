// Package t5cli is a widget wrapping the T5 serial-monitor-over-CAN CLI.
//
// The Trionic 5 ECU exposes an ASCII serial monitor behind two CAN commands:
//
//	C4 <ch>  feed ONE input character into the monitor (gated by flag 0x3720)
//	C6       pull ONE output character back out          (gated by flag 0x3722)
//
// Requests go out on id 0x05, replies arrive on id 0x0C as [0xC6, 0x00, char].
// So a "line" is sent char-by-char as C4 frames terminated by CR, and the
// response is drained one char at a time with C6 polls.
package t5cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
)

const (
	reqID  = 0x05 // we send commands here
	respID = 0x0C // ECU replies here
	cmdC4  = 0xC4 // feed one ascii char to the monitor
	cmdC6  = 0xC6 // pull one output char from the monitor
)

const (
	charTimeout  = 250 * time.Millisecond // wait for a C4 char's ack/echo
	pollTimeout  = 120 * time.Millisecond // wait for a C6 output char (timeout => FIFO empty)
	flushTimeout = 40 * time.Millisecond  // shorter probe when flushing residual output
	maxDrain     = 1 << 32                // safety cap on output chars (S dumps the whole symbol table, ~11 KB)
	historySize  = 15                     // commands kept for arrow-up/down recall
	maxRows      = 300                    // output TextGrid rows kept before trimming the top
)

// Symbol is one entry of the ECU symbol table downloaded with the 'S' command,
// kept in RAM for the session so /symbol can look one up by name, number or address.
type Symbol struct {
	Num  int // ordinal in the table (0-based)
	Name string
	Addr uint16
	Len  uint16
}

var _ fyne.Widget = (*Widget)(nil)

type Widget struct {
	widget.BaseWidget

	output *widget.TextGrid
	// scroll *container.Scroll
	input *historyEntry

	connectBtn    *widget.Button
	disconnectBtn *widget.Button

	getAdapter func() (gocan.Adapter, error)

	// client, symbols and verbose are only touched by the worker goroutine.
	client  *gocan.Client
	symbols []Symbol
	verbose bool

	lines  chan string
	ctx    context.Context
	cancel context.CancelFunc
}

func New(getAdapter func() (gocan.Adapter, error)) *Widget {
	w := &Widget{
		getAdapter: getAdapter,
		lines:      make(chan string, 4),
		output:     widget.NewTextGrid(),
		input:      newHistoryEntry(),
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.ExtendBaseWidget(w)

	// TextGrid only creates its internal scroller in CreateRenderer, so calling
	// its ScrollToBottom before the widget is rendered panics. Scroll ourselves.
	w.output.Scroll = fyne.ScrollBoth
	// w.scroll = container.NewScroll(w.output)

	w.connectBtn = widget.NewButtonWithIcon("Connect", theme.LoginIcon(), func() {
		w.submit("/connect")
	})
	w.disconnectBtn = widget.NewButtonWithIcon("Disconnect", theme.LogoutIcon(), func() {
		w.submit("/disconnect")
	})
	w.disconnectBtn.Disable()

	w.input.PlaceHolder = "type a command, '?' for help — Up/Down for history"
	w.input.OnSubmitted = func(line string) {
		w.input.record(line)
		w.input.SetText("")
		w.submit(line)
	}

	w.output.Append(fmt.Sprintf("T5 serial monitor over CAN (req=0x%02X resp=0x%02X). Type '?' for help.", reqID, respID))

	go w.worker()
	return w
}

// Close stops the worker (which closes the CAN client) — wire it to the
// containing window's OnClose.
func (w *Widget) Close() {
	w.cancel()
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(
		container.NewHBox(w.connectBtn, w.disconnectBtn),
		w.input,
		nil,
		nil,
		w.output,
	))
}

// submit queues one line for the worker; called from the fyne thread.
func (w *Widget) submit(line string) {
	select {
	case w.lines <- line:
	default:
		w.print("  busy — command dropped")
	}
}

// print appends one (possibly multi-line) entry to the output grid.
// Safe to call from any goroutine; fyne.Do preserves submission order.
func (w *Widget) print(s string) {
	fyne.Do(func() {
		w.output.Append(s)
		if n := len(w.output.Rows); n > maxRows {
			w.output.Rows = w.output.Rows[n-maxRows:]
			w.output.Refresh()
		}
		// w.scroll.ScrollToBottom()
		w.output.ScrollToBottom()
	})
}

func (w *Widget) printf(format string, a ...any) {
	w.print(fmt.Sprintf(format, a...))
}

// worker serialises command execution: the monitor has a single command
// mailbox, so lines run strictly one at a time.
func (w *Widget) worker() {
	defer func() {
		if w.client != nil {
			w.client.Close()
		}
	}()
	for {
		select {
		case <-w.ctx.Done():
			return
		case line := <-w.lines:
			w.handle(line)
		}
	}
}

// connect lazily opens the CAN client on the first command that needs it.
func (w *Widget) connect() error {
	if w.client != nil {
		return nil
	}
	dev, err := w.getAdapter()
	if err != nil {
		return err
	}
	w.print("connecting to " + dev.Name() + "...")
	cl, err := gocan.NewWithOpts(w.ctx, dev, gocan.WithEventFunc(func(e gocan.Event) {
		w.print(e.String())
	}))
	if err != nil {
		return err
	}
	w.client = cl
	w.print("  connected")
	w.setConnected(true)
	return nil
}

// disconnect closes the CAN client; the next command (or Connect) reopens it.
func (w *Widget) disconnect() {
	if w.client == nil {
		w.print("  not connected")
		return
	}
	w.client.Close()
	w.client = nil
	w.print("  disconnected")
	w.setConnected(false)
}

func (w *Widget) setConnected(connected bool) {
	fyne.Do(func() {
		if connected {
			w.connectBtn.Disable()
			w.disconnectBtn.Enable()
		} else {
			w.connectBtn.Enable()
			w.disconnectBtn.Disable()
		}
	})
}

// handle runs one input line.
func (w *Widget) handle(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	w.print("t5> " + line)
	switch {
	case trimmed == "?" || trimmed == "help":
		w.print(helpText)
		return
	case trimmed == "/v":
		w.verbose = !w.verbose
		w.printf("  verbose = %v", w.verbose)
		return
	case trimmed == "/symbol" || strings.HasPrefix(trimmed, "/symbol "):
		w.searchSymbols(strings.TrimSpace(strings.TrimPrefix(trimmed, "/symbol")))
		return
	case trimmed == "/connect":
		if w.client != nil {
			w.print("  already connected")
			return
		}
		if err := w.connect(); err != nil {
			w.printf("  connect: %v", err)
		}
		return
	case trimmed == "/disconnect":
		w.disconnect()
		return
	}

	// everything below talks to the ECU
	if err := w.connect(); err != nil {
		w.printf("  connect: %v", err)
		return
	}
	switch {
	case strings.HasPrefix(trimmed, "/raw"):
		w.sendRaw(trimmed)
	case strings.HasPrefix(trimmed, "/poll"):
		n := 0 // 0 => drain until empty
		if f := strings.Fields(trimmed); len(f) > 1 {
			n, _ = strconv.Atoi(f[1])
		}
		w.printf("  rx: %s", render(w.drain(n)))
	default:
		w.sendCommand(line)
	}
}

// sendCommand types a command line into the monitor: each character (and a
// terminating CR) as its own C4 frame, then drains the response with C6 polls.
//
// The monitor emits one '>' prompt char per input character as an ack. We wait
// for each (which also paces the single command mailbox) but they are not the
// response, so they are dropped — only the drained output, minus framing, is
// shown.
func (w *Widget) sendCommand(line string) {
	w.flush() // clear any residual output from a previous stateful stream (e.g. S dump)
	for _, b := range append([]byte(line), '\r') {
		w.txWait([]byte{cmdC4, b}, charTimeout) // ack/echo; ignored unless verbose
	}
	records := w.streamResponse()
	if strings.TrimSpace(line) == "S" { // S = symbol-table dump: keep it in RAM for /symbol
		n := w.parseSymbols(records)
		w.printf("  parsed %d symbols into RAM (search with /symbol <name|number|addr>)", n)
	}
}

// parseSymbols rebuilds the in-RAM symbol table from a freshly streamed S dump.
// Each table record is "<4hex addr><4hex len><name>"; the leading version line
// and the trailing "END" don't match the hex prefix and are skipped.
func (w *Widget) parseSymbols(records [][]byte) int {
	w.symbols = w.symbols[:0]
	for _, rec := range records {
		if len(rec) < 8 || !isHex(rec[:8]) {
			continue
		}
		addr, _ := strconv.ParseUint(string(rec[:4]), 16, 16)
		ln, _ := strconv.ParseUint(string(rec[4:8]), 16, 16)
		w.symbols = append(w.symbols, Symbol{
			Num:  len(w.symbols),
			Name: string(rec[8:]),
			Addr: uint16(addr),
			Len:  uint16(ln),
		})
	}
	return len(w.symbols)
}

// matchSymbols returns every loaded symbol whose name contains q (case-insensitive),
// or whose number (q as decimal) or address (q as hex) equals q.
func (w *Widget) matchSymbols(q string) []Symbol {
	ql := strings.ToLower(q)
	num, errNum := strconv.Atoi(q)
	addr, errAddr := strconv.ParseUint(q, 16, 16)
	var hits []Symbol
	for _, s := range w.symbols {
		if strings.Contains(strings.ToLower(s.Name), ql) ||
			(errNum == nil && s.Num == num) ||
			(errAddr == nil && s.Addr == uint16(addr)) {
			hits = append(hits, s)
		}
	}
	return hits
}

// searchSymbols matches q against every loaded symbol's name (case-insensitive
// substring), number (decimal) and address (hex), printing the hits.
func (w *Widget) searchSymbols(q string) {
	if len(w.symbols) == 0 {
		w.print("  no symbols loaded — run 'S' to download the table first")
		return
	}
	if q == "" {
		w.printf("  %d symbols loaded. usage: /symbol <name | number | hex-addr>", len(w.symbols))
		return
	}
	hits := w.matchSymbols(q)
	if len(hits) == 0 {
		w.printf("  no symbol matches %q", q)
		return
	}
	for _, s := range hits {
		w.printf("  #%-3d  %04X  %04X  %s", s.Num, s.Addr, s.Len, s.Name)
	}
	if len(hits) > 1 {
		w.printf("  (%d matches)", len(hits))
	}
}

// flush discards any monitor output still queued in the ECU's FIFO — e.g. the
// tail of a previous, stateful S dump (it pipelines a lookahead char and only
// stops at "END") — so each command starts from a clean FIFO.
func (w *Widget) flush() {
	for range maxDrain {
		if _, err := w.txWait([]byte{cmdC6}, flushTimeout); err != nil {
			return // empty
		}
	}
}

// streamResponse drains the monitor's output with C6 polls and prints it live,
// one CR/LF-delimited record per line, until the FIFO is empty. Streaming matters
// because S returns the whole symbol table (~11 KB) at one CAN frame per char.
// Stray '>' prompts and the CR/LF framing are dropped; a record that starts with
// 8 hex digits (the symbol table's <addr><len>) is spaced out for readability.
func (w *Widget) streamResponse() [][]byte {
	var rec []byte
	var records [][]byte
	emit := func() {
		if len(rec) > 0 {
			w.print("  " + fmtRecord(rec))
			records = append(records, append([]byte(nil), rec...)) // copy: rec is reused
			rec = rec[:0]
		}
	}
	for range maxDrain {
		reply, err := w.txWait([]byte{cmdC6}, pollTimeout)
		if err != nil {
			break // empty FIFO
		}
		ch, ok := outChar(reply)
		if !ok {
			continue
		}
		switch ch {
		case '\r', '\n':
			emit()
		case '>': // stray per-char prompt echo
		default:
			rec = append(rec, ch)
		}
	}
	emit()
	if len(records) == 0 {
		w.print("  ok")
	}
	return records
}

// drain pulls output chars with C6 until the FIFO is empty (a poll timeout, i.e.
// no reply, signals empty) or maxDrain. n>0 caps it at n chars instead. It does
// NOT stop at a LF: monitor streams like S are multi-line/multi-phase, and
// stopping early would strand the rest in the FIFO to bleed into the next command.
func (w *Widget) drain(n int) []byte {
	var out []byte
	limit := maxDrain
	if n > 0 {
		limit = n
	}
	for range limit {
		reply, err := w.txWait([]byte{cmdC6}, pollTimeout)
		if err != nil {
			break // empty FIFO
		}
		if ch, ok := outChar(reply); ok {
			out = append(out, ch)
		}
	}
	return out
}

// sendRaw: "/raw 05 c7 00 00 37 24" — send an arbitrary frame and print the reply.
func (w *Widget) sendRaw(line string) {
	fields := strings.Fields(line)[1:]
	if len(fields) < 2 {
		w.print("  usage: /raw <id> <b0> <b1> ...   (hex), e.g. /raw 05 c7 00 00 37 24")
		return
	}
	id, err := strconv.ParseUint(fields[0], 16, 32)
	if err != nil {
		w.printf("  bad id %q: %v", fields[0], err)
		return
	}
	data := make([]byte, 0, len(fields)-1)
	for _, f := range fields[1:] {
		b, err := strconv.ParseUint(f, 16, 8)
		if err != nil {
			w.printf("  bad byte %q: %v", f, err)
			return
		}
		data = append(data, byte(b))
	}
	frame := gocan.NewFrame(uint32(id), data, gocan.Outgoing)
	reply, err := w.client.SendAndWait(w.ctx, frame, charTimeout, respID)
	if err != nil {
		w.printf("  sent, no reply (%v)", err)
		return
	}
	w.printf("  rx: %s", frame2str(reply))
}

// txWait sends a frame on reqID and waits for one reply on respID.
func (w *Widget) txWait(data []byte, timeout time.Duration) (*gocan.CANFrame, error) {
	frame := gocan.NewFrame(reqID, data, gocan.Outgoing)
	reply, err := w.client.SendAndWait(w.ctx, frame, timeout, respID)
	if err == nil && w.verbose {
		w.print("    " + frame2str(reply))
	}
	return reply, err
}

// outChar returns the monitor output byte carried in a reply frame [0xC6, 0x00, char].
func outChar(f *gocan.CANFrame) (byte, bool) {
	if len(f.Data) >= 3 {
		return f.Data[2], true
	}
	return 0, false
}

// fmtRecord renders one monitor record. Symbol-table entries are "<4hex addr>
// <4hex len><name>"; split those into columns. Everything else prints as-is.
func fmtRecord(rec []byte) string {
	if len(rec) >= 8 && isHex(rec[:8]) {
		return fmt.Sprintf("%s %s  %s", rec[:4], rec[4:8], sanitize(rec[8:]))
	}
	return sanitize(rec)
}

func isHex(b []byte) bool {
	for _, x := range b {
		if !(x >= '0' && x <= '9' || x >= 'A' && x <= 'F' || x >= 'a' && x <= 'f') {
			return false
		}
	}
	return len(b) > 0
}

func sanitize(b []byte) string {
	out := make([]byte, len(b))
	for i, x := range b {
		if x >= 0x20 && x < 0x7F {
			out[i] = x
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func render(b []byte) string {
	if len(b) == 0 {
		return "(no output)"
	}
	var hex, asc strings.Builder
	for _, x := range b {
		fmt.Fprintf(&hex, "%02X ", x)
		if x >= 0x20 && x < 0x7F {
			asc.WriteByte(x)
		} else {
			asc.WriteByte('.')
		}
	}
	return fmt.Sprintf("%-*s %q", 3*len(b), strings.TrimRight(hex.String(), " "), asc.String())
}

func frame2str(f *gocan.CANFrame) string {
	return fmt.Sprintf("0x%02X | %s", f.Identifier, render(f.Data))
}

// historyEntry is a single-line Entry with an in-memory command history
// (last historySize entries) recalled with the Up/Down arrow keys.
type historyEntry struct {
	widget.Entry
	history []string
	pos     int // index into history; len(history) == the fresh empty line
}

func newHistoryEntry() *historyEntry {
	e := &historyEntry{}
	e.ExtendBaseWidget(e)
	return e
}

// record remembers a submitted line, skipping blanks and immediate repeats,
// and resets the recall position to the fresh line.
func (e *historyEntry) record(line string) {
	if line != "" && (len(e.history) == 0 || e.history[len(e.history)-1] != line) {
		e.history = append(e.history, line)
		if len(e.history) > historySize {
			e.history = e.history[len(e.history)-historySize:]
		}
	}
	e.pos = len(e.history)
}

func (e *historyEntry) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyUp:
		if e.pos > 0 {
			e.pos--
			e.recall(e.history[e.pos])
		}
	case fyne.KeyDown:
		if e.pos < len(e.history) {
			e.pos++
			if e.pos == len(e.history) {
				e.recall("")
			} else {
				e.recall(e.history[e.pos])
			}
		}
	default:
		e.Entry.TypedKey(ev)
	}
}

func (e *historyEntry) recall(t string) {
	e.SetText(t)
	e.CursorColumn = len([]rune(t))
	e.Refresh()
}

const helpText = `
T5 serial monitor — type a command and press Enter.
Commands are tunneled over CAN: each character is sent as a C4 frame on id 0x05,
the reply (one output char) comes back on id 0x0C, drained with C6 polls.

Up/Down arrows recall the last 15 commands (this session only).

TOOL COMMANDS
  ?                 this help
  /connect          open the CAN connection (also happens on the first command)
  /disconnect       close the CAN connection
  /v                toggle verbose (print every raw reply frame)
  /symbol <q>       search the symbol table (downloaded by S) by name (substring),
                    number (decimal), or address (hex). No arg => count + usage.
  /raw <id> <b>...  send a raw frame (hex) and print the reply
                    e.g. /raw 05 c7 00 00 37 24   (C7 read of 0x3724)
  /poll [n]         pull n output chars via C6 (no n => drain until empty)

COMMANDS (typed directly; sent char-by-char + CR, output drained via C6)
  A..O <addr><len>  Define a logging channel. 15 slots A-O. <addr>=4 hex digits
                    (<= 7FFF), <len>=4 hex digits. e.g. "A12340004" reads 4 bytes
                    from 0x1234 when streamed. Stores addr->0x3E9C.. / len->0x375E..
  R<x>              Arm a stream for channel x and set its bit in mask 0x3762:
                    RA->bit0  RB->bit1  RC->bit2  RD->bit3  (other R? ->bit4)
  W<addr><data>     Write one byte. <addr>=4 hex (<= 7FFF), <data>=2 hex.
                    e.g. "W1234AB" writes 0xAB to 0x1234.
  S                 Print the whole symbol table: one "<addr> <len> <name>" record
                    per line (~513 symbols). Slow (~10s, one CAN frame per char),
                    streamed live as it arrives, and parsed into RAM for /symbol.
  s                 Retrieve the binary version string (e.g. "A55790VL.17D").
  #                 No-op in this firmware: clears bit 6 of the monitor mode byte
                    0x373E, a bit nothing else sets or reads.

NOTES
  - C4 input is gated by ECU flag 0x3720, C6 output by 0x3722. These are managed
    internally by the monitor; over CAN the input flag must already be enabled or
    nothing will echo. If "rx: (no output)" for everything, input is likely off.
  - Reads/streams emit each data byte as TWO ASCII hex chars + CRLF.
  - Single command mailbox: chars are paced by waiting for each frame's reply.`
