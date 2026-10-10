package t5

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

// fakeT5 is a Trionic 5 behind a virtual CAN adapter. Until the client jumps
// to the uploaded loader it acts as the stock firmware (A5 + data frames load
// RAM with 16-bit addresses, C7 reads RAM, C1 jumps). After C1 to 0x5000 it
// acts as MyBooty: C0 erase, C2 reset, C7 flash read, C8 checksum, C9 chip
// id and A5 + data frames to program flash. Replies are delivered from
// inside Send, like pkg/ecusim, and are [echo, status, data..., 0x08 fill].
//
// Anything a real ECU would not survive (a C7 read outside flash, which
// restarts MyBooty, or a C8 over an odd byte count, which hangs it) is
// recorded in violations and left unanswered.
type fakeT5 struct {
	mu sync.Mutex

	bus        *gocan.Bus
	booty      bool // MyBooty is running
	mem        []byte
	flashStart uint32 // 0x40000 for 256 kB chips, 0x60000 for 128 kB
	maker, dev byte   // C9 manufacturer and device id

	winAddr uint32
	winLen  int
	winBuf  []byte

	sent       []gocan.Frame
	violations []string

	// hook, when set, sees every frame on 0x05 first. Returning true means
	// it handled the frame: reply is delivered when non-nil, otherwise the
	// ECU stays silent.
	hook func(d [8]byte) (reply []byte, handled bool)

	// sendErr, when set, can fail a frame on 0x05 at the adapter (a write
	// that never reached the bus) before anything else sees it.
	sendErr func(d [8]byte) error
}

type chip struct{ maker, dev byte }

var (
	amd28F010   = chip{0x01, 0xA7} // 256 kB, T5.5 box
	intel28F512 = chip{0x89, 0xB8} // 128 kB, T5.2 box
)

// newFakeT5 returns a box with the given flash chip, erased, running the
// stock firmware.
func newFakeT5(c chip) *fakeT5 {
	f := &fakeT5{mem: make([]byte, 0x80000), maker: c.maker, dev: c.dev, flashStart: 0x40000}
	if flashSizeFromChip(c.dev) == 128 {
		f.flashStart = 0x60000
	}
	for i := f.flashStart; i < 0x80000; i++ {
		f.mem[i] = 0xFF
	}
	return f
}

// load writes img into flash ending at 0x7FFFF, like a programmed chip.
func (f *fakeT5) load(img []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy(f.mem[0x80000-len(img):], img)
}

func (f *fakeT5) flash(addr uint32, n int) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return bytes.Clone(f.mem[addr : addr+uint32(n)])
}

func (f *fakeT5) setBooty(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.booty = v
}

func (f *fakeT5) setHook(h func(d [8]byte) ([]byte, bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hook = h
}

// frames returns the frames the client sent whose first byte is cmd.
func (f *fakeT5) frames(cmd byte) []gocan.Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []gocan.Frame
	for _, fr := range f.sent {
		if fr.Data[0] == cmd {
			out = append(out, fr)
		}
	}
	return out
}

// commands returns the first byte of every frame sent, in order.
func (f *fakeT5) commands() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.sent))
	for i, fr := range f.sent {
		out[i] = fr.Data[0]
	}
	return out
}

func (f *fakeT5) Open(_ context.Context, bus *gocan.Bus) error {
	f.bus = bus
	return nil
}

func (f *fakeT5) Close() error { return nil }

func (f *fakeT5) Send(_ context.Context, fr gocan.Frame) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fr.ID != 0x05 {
		return nil
	}
	if f.sendErr != nil {
		if err := f.sendErr(fr.Data); err != nil {
			return err
		}
	}
	f.sent = append(f.sent, fr)
	d := fr.Data
	if f.hook != nil {
		if reply, ok := f.hook(d); ok {
			if reply != nil {
				f.reply(reply...)
			}
			return nil
		}
	}
	if f.booty {
		f.bootyCmd(d)
	} else {
		f.firmwareCmd(d)
	}
	return nil
}

func (f *fakeT5) firmwareCmd(d [8]byte) {
	switch d[0] {
	case 0xA5: // only the low 16 address bits are used by the firmware
		f.winAddr, f.winLen = uint32(d[3])<<8|uint32(d[4]), int(d[5])
		f.reply(0xA5, 0x00)
	case 0xC1: // jump, never answered
		if binary.BigEndian.Uint32(d[1:5]) == 0x5000 {
			f.booty = true
		}
	case 0xC7:
		f.read(uint32(d[3])<<8 | uint32(d[4]))
	default:
		if d[0] < 0x80 && f.winLen > 0 {
			off := int(d[0])
			for i := 0; i < 7 && off+i < f.winLen; i++ {
				f.mem[f.winAddr+uint32(off+i)] = d[1+i]
			}
			f.reply(d[0], 0x00)
		}
	}
}

func (f *fakeT5) bootyCmd(d [8]byte) {
	switch d[0] {
	case 0xA5:
		f.winAddr, f.winLen = binary.BigEndian.Uint32(d[1:5]), int(d[5])
		f.winBuf = bytes.Repeat([]byte{0xFF}, f.winLen)
		f.reply(0xA5, 0x00)
	case 0xC0:
		for i := f.flashStart; i < 0x80000; i++ {
			f.mem[i] = 0xFF
		}
		f.reply(0xC0, 0x00)
	case 0xC2:
		f.booty = false
		f.reply(0xC2, 0x00)
	case 0xC7:
		addr := binary.BigEndian.Uint32(d[1:5])
		if addr < f.flashStart+5 || addr > 0x7FFFF {
			f.violations = append(f.violations, fmt.Sprintf("C7 read at 0x%X outside flash restarts MyBooty", addr))
			f.booty = false
			return
		}
		f.read(addr)
	case 0xC8:
		f.checksum()
	case 0xC9:
		f.reply(0xC9, 0x00, 0x00, 0x04, 0x00, 0x00, f.maker, f.dev)
	default:
		if d[0] >= 0x80 {
			f.reply(d[0], 0x09)
			return
		}
		off := int(d[0])
		if off >= f.winLen {
			f.violations = append(f.violations, fmt.Sprintf("data frame offset 0x%02X outside window of %d", off, f.winLen))
			f.reply(d[0], 0x09)
			return
		}
		n := min(7, f.winLen-off)
		copy(f.winBuf[off:], d[1:1+n])
		if off+n < f.winLen {
			f.reply(d[0], 0x00)
			return
		}
		// Last frame of the window: program it, like Flash_Programming.
		if f.winAddr < f.flashStart || f.winAddr+uint32(f.winLen) > 0x80000 {
			f.reply(d[0], 0x01, byte(f.winAddr>>24), byte(f.winAddr>>16), byte(f.winAddr>>8), byte(f.winAddr))
			return
		}
		for i, b := range f.winBuf {
			a := f.winAddr + uint32(i)
			if f.mem[a]&b != b { // flash can only clear bits
				f.reply(d[0], 0x01, byte(a>>24), byte(a>>16), byte(a>>8), byte(a))
				return
			}
			f.mem[a] = b
		}
		f.reply(d[0], 0x00)
	}
}

// read answers C7 with the six bytes ending at addr, highest address first.
func (f *fakeT5) read(addr uint32) {
	out := []byte{0xC7, 0x00}
	for i := range uint32(6) {
		out = append(out, f.mem[addr-i])
	}
	f.reply(out...)
}

// checksum answers C8 like MyBooty: sum [ROM_Offset..Code_End] from the
// flash footer and compare with the last four bytes of flash.
func (f *fakeT5) checksum() {
	romOffset, ok1 := f.footerHex(ROMoffset)
	codeEnd, ok2 := f.footerHex(CodeEnd)
	if !ok1 || !ok2 || codeEnd <= romOffset || codeEnd > 0x7FFFB {
		f.reply(0xC8, 0x01)
		return
	}
	if (codeEnd-romOffset+1)%2 != 0 {
		f.violations = append(f.violations, "C8 over an odd byte count hangs MyBooty")
		return
	}
	var sum uint32
	for _, b := range f.mem[romOffset : codeEnd+1] {
		sum += uint32(b)
	}
	if sum != binary.BigEndian.Uint32(f.mem[0x7FFFC:]) {
		f.reply(0xC8, 0x01)
		return
	}
	f.reply(0xC8, 0x00, byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum))
}

// footerHex parses a footer field as hex. Entries are walked backwards from
// just below the stored checksum as [len][id][value, reversed].
func (f *fakeT5) footerHex(id byte) (uint32, bool) {
	for p := 0x7FFFB; p > 0x7FF00; {
		n, typ := int(f.mem[p]), f.mem[p-1]
		p -= 2
		if n == 0xFF {
			return 0, false
		}
		if typ == id {
			var s strings.Builder
			for i := 0; i < n; i++ {
				s.WriteByte(f.mem[p-i])
			}
			v, err := strconv.ParseUint(s.String(), 16, 32)
			return uint32(v), err == nil
		}
		p -= n
	}
	return 0, false
}

func (f *fakeT5) reply(b ...byte) {
	out := bytes.Repeat([]byte{0x08}, 8)
	copy(out, b)
	f.bus.Deliver(gocan.NewFrame(0x0C, out))
}

// events collects what the client reports through its ecu.Config callbacks.
type events struct {
	mu       sync.Mutex
	messages []string
	errs     []error
	progress []float64
}

func (e *events) message(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = append(e.messages, s)
}

func (e *events) error(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, err)
}

func (e *events) prog(v float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.progress = append(e.progress, v)
}

func (e *events) hasMessage(sub string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.messages {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

func (e *events) errors() []error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]error(nil), e.errs...)
}

// lastProgress returns the start of the most recent progress range (a
// negative value announces the total) and the last value reported.
func (e *events) lastProgress() (start, last float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, v := range e.progress {
		if v < 0 {
			start = v
		}
		last = v
	}
	return start, last
}

// newClient opens a bus on dev and returns a T5 client on it whose callbacks
// are recorded in the returned events. When dev is a fakeT5 the test fails
// if the client did anything a real ECU would not survive.
func newClient(t *testing.T, dev gocan.Adapter) (*Client, *events) {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), dev)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Close() })
	if f, ok := dev.(*fakeT5); ok {
		t.Cleanup(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, v := range f.violations {
				t.Errorf("ECU protocol violation: %s", v)
			}
		})
	}
	ev := &events{}
	cl := New(bus, &ecu.Config{Name: "Trionic 5", OnMessage: ev.message, OnError: ev.error, OnProgress: ev.prog}).(*Client)
	return cl, ev
}

// newBootyClient returns a client already talking to MyBooty on a box with
// chip c, so tests can skip the bootloader upload.
func newBootyClient(t *testing.T, c chip) (*Client, *fakeT5, *events) {
	t.Helper()
	f := newFakeT5(c)
	f.booty = true
	cl, ev := newClient(t, f)
	cl.bootloaded = true
	return cl, f, ev
}

// buildBin returns a bin of size bytes for a box whose image starts at
// romOffset: code bytes in the first 0x200 bytes, a few distinct bytes in
// the reset vector area and a footer with identifiers, ROM_Offset, Code_End
// and the stored checksum.
func buildBin(t *testing.T, size int, romOffset uint32) []byte {
	t.Helper()
	bin := bytes.Repeat([]byte{0xFF}, size)
	for i := 0; i < 0x200; i++ {
		bin[i] = byte(i*7 + 3)
	}
	var entries []byte
	for _, e := range []struct {
		id  byte
		val string
	}{
		{Partnumber, "4780656"},
		{SoftwareID, "4782506"},
		{Dataname, "A55L41VQ.29H"},
		{EngineType, "9000 2.3T B234R"},
		{ImmoCode, "QQ12345"},
		{ROMend, "07FFFF"},
		{ROMoffset, fmt.Sprintf("%06X", romOffset)},
		{CodeEnd, fmt.Sprintf("%06X", romOffset+0x1FF)},
	} {
		entries = append(entries, footerEntry(e.id, e.val)...)
	}
	copy(bin[size-4-len(entries):], entries)
	var sum uint32
	for _, b := range bin[:0x200] {
		sum += uint32(b)
	}
	binary.BigEndian.PutUint32(bin[size-4:], sum)
	return bin
}

// echoMismatch is a reply hook result that fails the client's echo check,
// a transport-level error the client is expected to retry.
var echoMismatch = []byte{0xEE, 0x00}
