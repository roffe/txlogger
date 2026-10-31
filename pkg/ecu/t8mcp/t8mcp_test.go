package t8mcp

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

const mcpSize = 0x40100

// codeKey is the XOR key Legion codes transfer blocks with.
var codeKey = [6]byte{0x39, 0x68, 0x77, 0x6D, 0x47, 0x39}

// fakeLegion is a T8 whose main processor already runs the Legion
// bootloader (so Bootstrap only pings it) with an MCP flash behind it.
// It answers every request from inside Send, so replies are on the bus
// before Send returns. The MCP flash is kept in natural byte order.
type fakeLegion struct {
	// knobs, set before use
	secondaryStatus byte // status for "start secondary bootloader"; 0 means ready (1)
	badFullMD5      bool // report a wrong md5 for the whole image (partition 0)
	dropWrites      bool // acknowledge transfer blocks without storing them
	silent          bool // never answer anything
	ignoreExit      bool // do not answer the exit request
	silentReads     bool // do not answer MCP flash reads

	mu        sync.Mutex
	bus       *gocan.Bus
	flash     []byte
	ops       []string // high level operations, in order (no flash reads or block writes)
	frames    int      // frames sent to the ECU
	reads     int      // MCP flash read requests
	writes    int      // transfer blocks received
	csumErrs  int      // transfer blocks whose checksum did not match
	eraseMask []uint64
	married   bool

	md5Part    int      // partition of the last md5 demand
	pending    [][]byte // consecutive frames sent on the next flow control
	blockAddr  int
	blockData  []byte
	blockWant  int // consecutive frames still expected for the current transfer
	eraseState byte
}

func newFakeLegion(flash []byte) *fakeLegion {
	return &fakeLegion{flash: slices.Clone(flash)}
}

func (l *fakeLegion) Open(_ context.Context, bus *gocan.Bus) error {
	l.bus = bus
	return nil
}

func (l *fakeLegion) Close() error { return nil }

func (l *fakeLegion) reply(data ...byte) {
	l.bus.Deliver(gocan.NewFrame(0x7E8, data))
}

func (l *fakeLegion) Send(_ context.Context, f gocan.Frame) error {
	if f.ID != 0x7E0 || l.silent {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frames++
	d := f.Data

	if l.blockWant > 0 {
		l.blockData = append(l.blockData, d[1:8]...)
		if l.blockWant--; l.blockWant == 0 {
			l.storeBlock()
			l.reply(0x01, 0x76)
		}
		return nil
	}

	switch {
	case f.Length == 1 && d[0] == 0x30: // flow control: send the rest of a long reply
		for _, p := range l.pending {
			l.reply(p...)
		}
		l.pending = nil
	case bytes.Equal(d[:], []byte{0xEF, 0xBE, 0x00, 0x00, 0x00, 0x00, 0x33, 0x66}):
		l.ops = append(l.ops, "ping")
		l.reply(0xDE, 0xAD, 0xF0, 0x0F)
	case d[0] == 0x02 && d[1] == 0xA5:
		l.demand(d[2], int(d[6])<<8|int(d[7]))
	case d[1] == 0x21:
		l.readLocal(d[0], int(binary.BigEndian.Uint32(d[3:7])), int(d[2]))
	case d[0] == 0x10 && d[2] == 0x36: // TransferData
		// the first 6 bytes (service, subfunction, address) came in this
		// frame, the rest follows 7 bytes per consecutive frame
		rest := int(d[1]) - 6
		l.blockAddr = int(binary.BigEndian.Uint32(d[4:8]))
		l.blockData = nil
		l.blockWant = (rest + 6) / 7
		l.reply(0x30, 0x00)
	case d[1] == 0x34 && d[2] == 0x01: // erase, d[0] is the device
		mask := uint64(d[6]) | uint64(d[5])<<8
		l.ops = append(l.ops, fmt.Sprintf("erase %d %#x", d[0], mask))
		l.eraseMask = append(l.eraseMask, mask)
		for p := 0; p < 9; p++ {
			if mask&(1<<p) == 0 {
				continue
			}
			start, end := partRange(p + 1)
			for i := start; i < end; i++ {
				l.flash[i] = 0xFF
			}
		}
		l.eraseState = d[0]
	case f.Length == 2 && d[0] == 0x01 && d[1] == 0x20: // exit
		l.ops = append(l.ops, "exit")
		if !l.ignoreExit {
			l.reply(0x01, 0x60)
		}
	}
	return nil
}

// partRange is the MCP flash range of partition p (1-8 are 32 KiB, 9 is the
// shadow area, 0 the whole image).
func partRange(p int) (int, int) {
	switch {
	case p == 0:
		return 0, mcpSize
	case p == 9:
		return 0x40000, mcpSize
	default:
		return (p - 1) * 0x8000, p * 0x8000
	}
}

func (l *fakeLegion) demand(cmd byte, wish int) {
	status := byte(1)
	switch cmd {
	case 0:
		l.ops = append(l.ops, "latency")
	case 3:
		l.ops = append(l.ops, fmt.Sprintf("md5 %d", wish))
		l.md5Part = wish
	case 4:
		l.ops = append(l.ops, "secondary")
		if l.secondaryStatus != 0 {
			status = l.secondaryStatus
		}
	case 5:
		l.ops = append(l.ops, "marry")
		l.married = true
	}
	l.reply(0x03, 0xE5, cmd, status, 0, 0, 0, 0)
}

func (l *fakeLegion) readLocal(pci byte, addr, length int) {
	var data []byte
	skip := byte(0)
	switch pci {
	case 0xF0: // erase status
		l.ops = append(l.ops, "erase status")
		data = []byte{0, 0, 0, l.eraseState}
		if l.eraseState > 1 {
			l.eraseState = 1
		}
	case 0x07: // md5 of the last demanded partition
		start, end := partRange(l.md5Part)
		sum := md5.Sum(l.flash[start:end])
		if l.md5Part == 0 && l.badFullMD5 {
			sum[0] ^= 0xFF
		}
		data = sum[:]
	case 0x05: // MCP flash
		l.reads++
		if l.silentReads {
			return
		}
		data = bytes.Repeat([]byte{0xFF}, length)
		if addr < len(l.flash) {
			copy(data, l.flash[addr:])
		}
		if length == 0x80 && bytes.Count(data, []byte{0xFF}) == length {
			skip = 1
		}
	default:
		return
	}
	if length <= 4 {
		l.reply(append([]byte{0x06, 0x61, 0x00, 0x00}, data[:4]...)...)
		return
	}
	l.reply(append([]byte{0x10, byte(length + 2), 0x61, skip}, data[:4]...)...)
	if skip > 0 {
		return
	}
	seq := byte(0x21)
	for rest := data[4:]; len(rest) > 0; {
		n := min(7, len(rest))
		frame := make([]byte, 8)
		frame[0] = seq
		copy(frame[1:], rest[:n])
		l.pending = append(l.pending, frame)
		rest = rest[n:]
		if seq++; seq > 0x2F {
			seq = 0x20
		}
	}
}

// storeBlock checks and decodes a 0x80-byte transfer block and writes it to
// flash.
func (l *fakeLegion) storeBlock() {
	l.writes++
	coded := l.blockData[:0x80]
	var sum int16
	for _, b := range coded {
		sum += int16(b)
	}
	if l.blockData[0x80] != byte(sum>>8) || l.blockData[0x81] != byte(sum) {
		l.csumErrs++
		return
	}
	if l.dropWrites {
		return
	}
	for i, b := range coded {
		l.flash[l.blockAddr+i] = b ^ codeKey[i%6]
	}
}

// legionState is a copy of what a fakeLegion saw and holds.
type legionState struct {
	flash      []byte
	ops        []string
	frames     int
	reads      int
	writes     int
	csumErrs   int
	eraseMasks []uint64
	married    bool
}

func (l *fakeLegion) snapshot() legionState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return legionState{
		flash:      slices.Clone(l.flash),
		ops:        slices.Clone(l.ops),
		frames:     l.frames,
		reads:      l.reads,
		writes:     l.writes,
		csumErrs:   l.csumErrs,
		eraseMasks: slices.Clone(l.eraseMask),
		married:    l.married,
	}
}

// opIndex returns the index of op in ops at or after from, or -1.
func opIndex(ops []string, from int, op string) int {
	if from < 0 || from > len(ops) {
		return -1
	}
	if i := slices.Index(ops[from:], op); i >= 0 {
		return from + i
	}
	return -1
}

// mcpImage is a natural-order MCP image: partitions 1-3 and the shadow area
// programmed, the rest erased, firmware version "MCPTEST_01" at 0x810C.
func mcpImage() []byte {
	img := bytes.Repeat([]byte{0xFF}, mcpSize)
	for i := 0; i < 0x18000; i++ {
		img[i] = byte(i ^ i>>8 ^ 0x5A)
	}
	copy(img, []byte{0x00, 0x08, 0x20, 0x00})
	copy(img[0x810C:], "MCPTEST_01")
	for i := 0x40000; i < mcpSize; i++ {
		img[i] = byte(i)
	}
	return img
}

// withNewPartition3 returns img with partition 3 (0x10000-0x17FFF)
// reprogrammed, two of its blocks left erased.
func withNewPartition3(img []byte) []byte {
	out := slices.Clone(img)
	for i := 0x10000; i < 0x18000; i++ {
		out[i] = ^out[i] + byte(i>>4)
	}
	for i := 0x14000; i < 0x14100; i++ {
		out[i] = 0xFF
	}
	return out
}

func swapPairs(b []byte) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b); i += 2 {
		out[i], out[i+1] = b[i+1], b[i]
	}
	return out
}

type recorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *recorder) has(prefix string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.msgs, func(m string) bool { return strings.HasPrefix(m, prefix) })
}

func (r *recorder) find(prefix string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.msgs {
		if strings.HasPrefix(m, prefix) {
			return m
		}
	}
	return ""
}

// newClient opens dev on a fresh bus and returns an MCP client on it whose
// messages land in the returned recorder.
func newClient(t *testing.T, dev gocan.Adapter) (*Client, *recorder) {
	t.Helper()
	bus, err := gocan.OpenAdapter(context.Background(), dev)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	rec := &recorder{}
	cfg := &ecu.Config{
		Name:       "Trionic 8 MCP",
		OnProgress: func(float64) {},
		OnError:    func(error) {},
		OnMessage: func(m string) {
			rec.mu.Lock()
			rec.msgs = append(rec.msgs, m)
			rec.mu.Unlock()
		},
	}
	return New(bus, cfg).(*Client), rec
}

func testCtx(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// The package registers its client as "Trionic 8 MCP" on the 500 kbit bus,
// listening for the main ECU's 0x7E8 replies.
func TestRegistered(t *testing.T) {
	info := ecu.Info("Trionic 8 MCP")
	if info.NewFunc == nil {
		t.Fatal("Trionic 8 MCP is not registered")
	}
	if info.CANRate != 500 || !slices.Equal(info.Filter, []uint32{0x7E8}) {
		t.Errorf("CANRate %v Filter %X, want 500 [7E8]", info.CANRate, info.Filter)
	}
	bus, err := gocan.OpenAdapter(context.Background(), newFakeLegion(mcpImage()))
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c, err := ecu.New(bus, &ecu.Config{Name: "Trionic 8 MCP"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*Client); !ok {
		t.Errorf("ecu.New returned %T, want *t8mcp.Client", c)
	}
}

// Operations the MCP has no use for return at once without touching the
// bus: marrying, recovery and DTC reading fail, erase and info printing
// are no-ops.
func TestUnsupportedOperations(t *testing.T) {
	dev := newFakeLegion(mcpImage())
	c, _ := newClient(t, dev)
	ctx := testCtx(t, 5*time.Second)

	if err := c.MarryECU(ctx, "VIN"); err == nil {
		t.Error("MarryECU succeeded")
	}
	if err := c.RecoverECU(ctx, nil); err == nil {
		t.Error("RecoverECU succeeded")
	}
	if dtcs, err := c.ReadDTC(ctx); err == nil || dtcs != nil {
		t.Errorf("ReadDTC = %v, %v; want nil and an error", dtcs, err)
	}
	if err := c.EraseECU(ctx); err != nil {
		t.Errorf("EraseECU: %v", err)
	}
	if err := c.PrintECUInfo(ctx); err != nil {
		t.Errorf("PrintECUInfo: %v", err)
	}
	if st := dev.snapshot(); st.frames != 0 {
		t.Errorf("%d frames sent, want none", st.frames)
	}
}

// Info pings Legion, sets its frame latency, starts the MCP's secondary
// bootloader and reports the MCP firmware version read from 0x810C.
func TestInfo(t *testing.T) {
	dev := newFakeLegion(mcpImage())
	c, rec := newClient(t, dev)

	res, err := c.Info(testCtx(t, 10*time.Second))
	if err != nil || res != nil {
		t.Fatalf("Info = %v, %v; want nil, nil", res, err)
	}
	if !rec.has("MCP Firmware information: MCPTEST_01") {
		t.Errorf("messages %q lack the firmware version", rec.msgs)
	}
	if ops := dev.snapshot().ops; !slices.Equal(ops, []string{"ping", "latency", "secondary"}) {
		t.Errorf("ops %q, want ping, latency, secondary", ops)
	}
}

// When the MCP's secondary bootloader does not start, Info, DumpECU and
// FlashECU stop right there and Info reports the failed start. Legion
// retries the start until the context ends, so going on past it is caught
// by the message each next step logs before it touches the bus (version,
// "Dumping MCP", "Determine Partition Mask").
func TestSecondaryBootloaderFails(t *testing.T) {
	test.NewTempApp(t)
	for _, op := range []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{"Info", func(ctx context.Context, c *Client) error { _, err := c.Info(ctx); return err }},
		{"DumpECU", func(ctx context.Context, c *Client) error { _, err := c.DumpECU(ctx); return err }},
		{"FlashECU", func(ctx context.Context, c *Client) error {
			return c.FlashECU(ctx, withNewPartition3(mcpImage()))
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			dev := newFakeLegion(mcpImage())
			dev.secondaryStatus = 0xFF
			c, rec := newClient(t, dev)

			err := op.run(testCtx(t, 400*time.Millisecond), c)
			if err == nil {
				t.Fatalf("%s succeeded without the secondary bootloader", op.name)
			}
			if op.name == "Info" && err.Error() != "failed to start secondary bootloader" {
				t.Errorf("Info error %q, want failed to start secondary bootloader", err)
			}
			if rec.has("MCP Firmware information") || rec.has("Dumping MCP") || rec.has("Determine Partition Mask") {
				t.Errorf("messages %q: went on past the failed start", rec.msgs)
			}
			st := dev.snapshot()
			if opIndex(st.ops, 0, "secondary") < 0 {
				t.Errorf("ops %q: secondary bootloader never requested", st.ops)
			}
			for _, o := range st.ops {
				if o != "ping" && o != "latency" && o != "secondary" {
					t.Errorf("ops %q: went on past the failed start", st.ops)
					break
				}
			}
			if st.reads != 0 {
				t.Errorf("%d flash reads after the failed start", st.reads)
			}
		})
	}
}

// Info fails, without reporting a firmware version, when the MCP flash
// read for the version string goes unanswered.
func TestInfoVersionReadFails(t *testing.T) {
	dev := newFakeLegion(mcpImage())
	dev.silentReads = true
	c, rec := newClient(t, dev)

	if _, err := c.Info(testCtx(t, 10*time.Second)); err == nil {
		t.Fatal("Info succeeded without the firmware version")
	}
	if rec.has("MCP Firmware information") {
		t.Errorf("messages %q report a firmware version", rec.msgs)
	}
	if st := dev.snapshot(); st.reads != 1 {
		t.Errorf("%d flash reads, want the one version read", st.reads)
	}
}

// Info, DumpECU and FlashECU all fail when no Legion bootloader answers
// and the ECU does not take the bootstrap either.
func TestOperationsFailWithoutLegion(t *testing.T) {
	test.NewTempApp(t)
	for _, op := range []struct {
		name string
		run  func(context.Context, *Client) error
	}{
		{"Info", func(ctx context.Context, c *Client) error { _, err := c.Info(ctx); return err }},
		{"DumpECU", func(ctx context.Context, c *Client) error {
			bin, err := c.DumpECU(ctx)
			if bin != nil {
				t.Errorf("DumpECU returned %d bytes", len(bin))
			}
			return err
		}},
		{"FlashECU", func(ctx context.Context, c *Client) error { return c.FlashECU(ctx, mcpImage()) }},
	} {
		t.Run(op.name, func(t *testing.T) {
			dev := newFakeLegion(mcpImage())
			dev.silent = true
			c, _ := newClient(t, dev)
			if err := op.run(testCtx(t, 150*time.Millisecond), c); err == nil {
				t.Fatalf("%s succeeded with a silent ECU", op.name)
			}
			if c.legion.IsRunning() {
				t.Error("legion reported running")
			}
		})
	}
}

// DumpECU reads the whole 0x40100-byte MCP flash and returns it once its
// md5 matches the one the bootloader computes.
func TestDumpECU(t *testing.T) {
	img := mcpImage()
	dev := newFakeLegion(img)
	c, rec := newClient(t, dev)

	bin, err := c.DumpECU(testCtx(t, 30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, img) {
		t.Fatalf("dump (%#x bytes) differs from the MCP flash", len(bin))
	}
	if ops := dev.snapshot().ops; !slices.Equal(ops, []string{"ping", "latency", "secondary", "md5 0"}) {
		t.Errorf("ops %q, want ping, latency, secondary, md5 0", ops)
	}
	sum := md5.Sum(img)
	if want := fmt.Sprintf("Remote md5 : %X", sum); !rec.has(want) {
		t.Errorf("messages lack %q", want)
	}
}

// DumpECU discards the dump when the bootloader's md5 disagrees with the
// data received.
func TestDumpECUMD5Mismatch(t *testing.T) {
	dev := newFakeLegion(mcpImage())
	dev.badFullMD5 = true
	c, _ := newClient(t, dev)

	bin, err := c.DumpECU(testCtx(t, 30*time.Second))
	if err == nil || err.Error() != "md5 Verification failed" {
		t.Fatalf("DumpECU error %v, want md5 Verification failed", err)
	}
	if bin != nil {
		t.Errorf("DumpECU returned %d bytes with a failed verification", len(bin))
	}
}

// FlashECU rejects images that are not exactly 0x40100 bytes before
// talking to the ECU.
func TestFlashECURejectsWrongSize(t *testing.T) {
	for _, n := range []int{0, 0x40000, 0x40101, 0x100000} {
		dev := newFakeLegion(mcpImage())
		c, _ := newClient(t, dev)
		err := c.FlashECU(testCtx(t, 5*time.Second), make([]byte, n))
		if err == nil || err.Error() != "err: Invalid T8 MCP file size" {
			t.Errorf("%#x bytes: error %v, want invalid file size", n, err)
		}
		if st := dev.snapshot(); st.frames != 0 {
			t.Errorf("%#x bytes: %d frames sent, want none", n, st.frames)
		}
	}
}

// FlashECU erases and rewrites only the partitions whose md5 differs,
// verifies them and marries the MCP. A byteswapped image is written in
// natural byte order.
func TestFlashECU(t *testing.T) {
	for _, c := range []struct {
		name string
		bin  func(newImg []byte) []byte
	}{
		{"natural image", func(b []byte) []byte { return b }},
		{"byteswapped image", swapPairs},
	} {
		t.Run(c.name, func(t *testing.T) {
			test.NewTempApp(t)
			old := mcpImage()
			want := withNewPartition3(old)
			dev := newFakeLegion(old)
			cl, rec := newClient(t, dev)

			if err := cl.FlashECU(testCtx(t, 30*time.Second), c.bin(want)); err != nil {
				t.Fatal(err)
			}
			st := dev.snapshot()
			if !bytes.Equal(st.flash, want) {
				t.Error("MCP flash does not hold the new image")
			}
			if !slices.Equal(st.eraseMasks, []uint64{0x4}) {
				t.Errorf("erase masks %#x, want [0x4] (partition 3)", st.eraseMasks)
			}
			if st.csumErrs != 0 {
				t.Errorf("%d transfer blocks had a bad checksum", st.csumErrs)
			}
			// 256 blocks in partition 3, two of them left erased
			if st.writes != 254 {
				t.Errorf("%d blocks written, want 254", st.writes)
			}
			// compare md5, erase, verify md5, then marry as the last step
			compare := opIndex(st.ops, 0, "md5 3")
			erase := opIndex(st.ops, compare, "erase 5 0x4")
			verify := opIndex(st.ops, erase, "md5 3")
			marry := opIndex(st.ops, verify, "marry")
			if compare < 0 || erase < 0 || verify < 0 || marry != len(st.ops)-1 {
				t.Errorf("ops %q: want md5 3, erase, md5 3, marry last", st.ops)
			}
			if !rec.has("Verifying md5: sucess") || !rec.has("MCP Firmware information: MCPTEST_01") {
				t.Errorf("messages %q lack verification or firmware version", rec.msgs)
			}
		})
	}
}

// FlashECU leaves the boot partition alone unless the boot preference is
// set; with it set, a changed boot partition is written together with the
// shadow area. An image identical to the ECU is not erased at all.
func TestFlashECUBootPartition(t *testing.T) {
	old := mcpImage()
	newBoot := slices.Clone(old)
	for i := 0x100; i < 0x200; i++ {
		newBoot[i] ^= 0xA5
	}
	for _, c := range []struct {
		name      string
		boot      bool
		bin       []byte
		wantMask  []uint64
		wantFlash []byte
	}{
		{"identical image", false, old, nil, old},
		{"identical image, boot set", true, old, nil, old},
		{"changed boot, boot unset", false, newBoot, nil, old},
		{"changed boot, boot set", true, newBoot, []uint64{0x101}, newBoot},
	} {
		t.Run(c.name, func(t *testing.T) {
			app := test.NewTempApp(t)
			app.Preferences().SetBool(settings.PrefsBoot, c.boot)
			dev := newFakeLegion(old)
			cl, rec := newClient(t, dev)

			if err := cl.FlashECU(testCtx(t, 30*time.Second), c.bin); err != nil {
				t.Fatal(err)
			}
			st := dev.snapshot()
			if !slices.Equal(st.eraseMasks, c.wantMask) {
				t.Errorf("erase masks %#x, want %#x", st.eraseMasks, c.wantMask)
			}
			if !bytes.Equal(st.flash, c.wantFlash) {
				t.Error("MCP flash content is wrong")
			}
			if c.wantMask == nil {
				if !rec.has("Noting to flash") {
					t.Errorf("messages %q lack the nothing-to-flash notice", rec.msgs)
				}
				if st.married {
					t.Error("MCP married without a flash")
				}
			} else if !st.married {
				t.Error("MCP not married after the flash")
			}
		})
	}
}

// FlashECU fails, without marrying, when the partitions read back with a
// different md5 than the image.
func TestFlashECUVerifyFails(t *testing.T) {
	test.NewTempApp(t)
	old := mcpImage()
	dev := newFakeLegion(old)
	dev.dropWrites = true
	c, _ := newClient(t, dev)

	err := c.FlashECU(testCtx(t, 30*time.Second), withNewPartition3(old))
	if err == nil || err.Error() != "failed md5 verification" {
		t.Fatalf("FlashECU error %v, want failed md5 verification", err)
	}
	if dev.snapshot().married {
		t.Error("MCP married after a failed verification")
	}
}

// ResetECU tells a running Legion to exit and does nothing when Legion
// was never started.
func TestResetECU(t *testing.T) {
	t.Run("not running", func(t *testing.T) {
		dev := newFakeLegion(mcpImage())
		c, _ := newClient(t, dev)
		if err := c.ResetECU(testCtx(t, 5*time.Second)); err != nil {
			t.Fatal(err)
		}
		if st := dev.snapshot(); st.frames != 0 {
			t.Errorf("%d frames sent, want none", st.frames)
		}
	})
	t.Run("running", func(t *testing.T) {
		dev := newFakeLegion(mcpImage())
		c, _ := newClient(t, dev)
		ctx := testCtx(t, 10*time.Second)
		if _, err := c.Info(ctx); err != nil {
			t.Fatal(err)
		}
		if err := c.ResetECU(ctx); err != nil {
			t.Fatal(err)
		}
		if ops := dev.snapshot().ops; ops[len(ops)-1] != "exit" {
			t.Errorf("ops %q, want exit last", ops)
		}
	})
	t.Run("exit unanswered", func(t *testing.T) {
		dev := newFakeLegion(mcpImage())
		dev.ignoreExit = true
		c, _ := newClient(t, dev)
		ctx := testCtx(t, 10*time.Second)
		if _, err := c.Info(ctx); err != nil {
			t.Fatal(err)
		}
		if err := c.ResetECU(ctx); err == nil || !strings.HasPrefix(err.Error(), "LegionExit") {
			t.Fatalf("ResetECU error %v, want a LegionExit error", err)
		}
	})
}
