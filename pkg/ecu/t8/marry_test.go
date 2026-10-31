package t8

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/ecu/t8/t8file"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

const ecuVIN, cimVIN = "YS3FB49F741000001", "YS3FH41U861000002"

// nvdmImage returns flashImage with an NVDM (0x4000..0x8000) holding one
// flash block at 0x4100, listed first in the low block table, that stores
// pin, and data in its last byte.
func nvdmImage(t *testing.T, pin string) []byte {
	t.Helper()
	img := flashImage()
	copy(img[0x4000:0x8000], bytes.Repeat([]byte{0xFF}, 0x4000))
	copy(img[0x400E:], []byte{0x41, 0x00, 0x00, 0x00, 0x00, 0x01}) // block address, type 1
	img[0x7FFF] = 0x00                                             // keeps the last NVDM block from reading as erased
	plain := make([]byte, 0x130)
	copy(plain[0x40:], pin)
	copy(plain[0x50:], "Cryp") // marks it an NVDM record
	for i, b := range plain {
		img[0x4100+i] = (b ^ 0xA4) - 0x53 // the inverse of FlashBlock.DecodeBlock
	}
	th := new(t8file.T8Header)
	th.DecodeExtraInfo(img)
	if th.PIN() != pin {
		t.Fatalf("test NVDM decodes to PIN %q, want %q", th.PIN(), pin)
	}
	return img
}

// marriedECU returns a fake T8 running its stock firmware with img in flash,
// married to a car (its VIN cannot be rewritten) and unlocked by pin, next to
// a CIM.
func marriedECU(img []byte, pin string) *fakeT8 {
	ecu := newFakeT8()
	ecu.flash = slices.Clone(img)
	ecu.mcpVersion = "MCP 4.3.21"
	ecu.married = true
	ecu.pin = pin
	ecu.dids[0x90] = []byte(ecuVIN)
	ecu.cim = newFakeCIM()
	ecu.cim.dids[0x90] = []byte(cimVIN)
	return ecu
}

// strandedECU is marriedECU with the legion left running by an earlier
// session: the stock firmware is silent until the legion exits. MarryECU's
// first CheckIfCanChangeVIN fails on the unanswered security access, and
// the first bootstrap finds the legion alive and uploads nothing, which
// spares these tests one loader upload.
func strandedECU(img []byte, pin string) *fakeT8 {
	ecu := marriedECU(img, pin)
	ecu.legion = true
	return ecu
}

// noteWaits makes MarryECU's delays return at once and records each one as
// "wait <duration>" in the ECU's request log, so the log shows where they
// fall between the requests.
func noteWaits(t *testing.T, ecu *fakeT8) {
	old := wait
	wait = func(ctx context.Context, d time.Duration) error {
		defer ecu.lock()()
		ecu.note("wait %v", d)
		return ctx.Err()
	}
	t.Cleanup(func() { wait = old })
}

// requests returns the ECU's request log with each run of block transfers,
// flash reads and erase status polls folded into one entry: their count
// depends on the flash content and the loader, not on MarryECU.
func requests(ecu *fakeT8) []string {
	defer ecu.lock()()
	var out []string
	for _, e := range ecu.log {
		switch e {
		case "36 00", "read 06", "read F0":
			if len(out) > 0 && out[len(out)-1] == e {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// checkLog compares the ECU's request log with want, line by line.
func checkLog(t *testing.T, ecu *fakeT8, want []string) {
	t.Helper()
	got := requests(ecu)
	if !slices.Equal(got, want) {
		t.Errorf("ECU requests differ:\n got %q\nwant %q", got, want)
	}
}

// cat joins request sequences.
func cat(parts ...[]string) []string { return slices.Concat(parts...) }

var (
	// checkVIN is CheckIfCanChangeVIN: unlock, read the VIN, write it back.
	checkVIN = []string{"27 01", "27 02", "1A 90", "3B 90"}
	// bootstrap is what the legion bootstrap sends to the stock firmware:
	// pings that go unanswered, the programming session preflight, the
	// loader upload, the jump to 0x102400, a ping, and the inter frame
	// latency.
	bootstrap = []string{"ping", "ping", "ping", "10 02", "28", "A2", "A5 01", "A5 03", "27 01", "27 02", "34 00", "36 00", "36 80", "ping", "demand 00"}
	// readPIN is MarryECU on a strandedECU up to the PIN: the security
	// access of CheckIfCanChangeVIN goes unanswered, the legion answers its
	// ping, the NVDM is read and the legion exits.
	readPIN = []string{"27 01", "ping", "demand 00", "read 06", "exit"}
	// pinUnlock is MarryECU entering the PIN read from the NVDM after the
	// 30 s security access delay.
	pinUnlock = []string{"wait 30s", "27 01", "27 02", "AE 60", "AE 6E"}
	// marry is the marry proper on the ECU side: clear DTCs (the 04 comes in
	// on 0x7DF), read the VIN, write the CIM's VIN, wait out the CIM, then
	// unlock and set oil quality, top speed and E85.
	marry = []string{
		"wait 25ms", "10 02", "04", "20", "wait 200ms", "1A 90", "3B 90", "wait 1s",
		"wait 1s", "wait 1s", "wait 1s", "wait 1s", "wait 1s",
		"27 01", "27 02", "1A 90", "3B 25", "3B 02", "AE 18", "20",
	}
)

// pinCode is the device control code MarryECU sends for a PIN: 00 and its
// four characters.
func pinCode(pin string) []byte { return append([]byte{0x00}, pin...) }

func devctlOf(ecu *fakeT8) [][]byte {
	defer ecu.lock()()
	return slices.Clone(ecu.devctl)
}

func checkWrites(t *testing.T, node string, got []didWrite, want ...didWrite) {
	t.Helper()
	if !slices.EqualFunc(got, want, func(a, b didWrite) bool { return a.pid == b.pid && bytes.Equal(a.data, b.data) }) {
		t.Errorf("%s writes = %+v, want %+v", node, got, want)
	}
}

// marriedWrites are the ECU writes of a marry after an accepted
// CheckIfCanChangeVIN: the VIN written back once, the CIM's VIN, oil
// quality 50% and speed limiter 240 km/h.
var marriedWrites = []didWrite{
	{0x90, []byte(ecuVIN)},
	{0x90, []byte(cimVIN)},
	{0x25, []byte{0x00, 0x00, 0x32, 0x00}},
	{0x02, []byte{0x09, 0x60}},
}

func checkNoFlashChanges(t *testing.T, ecu *fakeT8) {
	t.Helper()
	defer ecu.lock()()
	if len(ecu.erases) != 0 || len(ecu.transfers) != 0 {
		t.Errorf("erases %#x transfers %#x, want no flash changes", ecu.erases, ecu.transfers)
	}
}

// flashReadAddrs returns the addresses of the legion's T8 flash reads.
func flashReadAddrs(ecu *fakeT8) []int {
	var out []int
	for _, d := range ecu.sentTo(0x7E0) {
		if len(d) == 8 && d[0] == 0x06 && d[1] == 0x21 {
			out = append(out, int(d[3])<<24|int(d[4])<<16|int(d[5])<<8|int(d[6]))
		}
	}
	return out
}

func (r *recorder) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.msgs)
}

// MarryECU on an ECU whose VIN cannot be changed starts the legion, reads the
// NVDM, leaves the legion, waits out the 30 s security access delay and
// enters the PIN stored in the NVDM with device controls $60 and $6E. When
// that unlocks the VIN it marries the ECU to the CIM.
func TestMarryECUPinFromNVDM(t *testing.T) {
	test.NewTempApp(t)
	img := nvdmImage(t, "4711")
	ecu := marriedECU(img, "4711")
	noteWaits(t, ecu)
	c, rec := newClient(t, ecu)

	if err := c.MarryECU(testCtx(t), "1234"); err != nil {
		t.Fatal(err)
	}

	checkLog(t, ecu, cat(checkVIN, bootstrap, []string{"read 06", "exit"}, pinUnlock, checkVIN, marry))
	if got, want := devctlOf(ecu), [][]byte{
		append([]byte{0x60}, pinCode("4711")...),
		append([]byte{0x6E}, pinCode("4711")...),
		{0x18, 0x00, 0x0A},
	}; !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("ECU device controls = % X, want % X", got, want)
	}
	// the NVDM is read up to its last block, and nothing past it
	if reads := flashReadAddrs(ecu); !slices.Contains(reads, 0x4000) || slices.Max(reads) != 0x7F80 {
		t.Errorf("flash reads at %#x, want 0x4000 up to 0x7F80", reads)
	}
	checkWrites(t, "ECU", writesOf(ecu), marriedWrites...)
	checkWrites(t, "CIM", writesOf(ecu.cim), didWrite{0x60, []byte("1234")}, didWrite{0x63, []byte{}})
	checkNoFlashChanges(t, ecu)
	for _, m := range []string{
		"Virginize ECU by pin stored in ECU",
		"PIN stored in ECU: 4711, security access delay 30s",
		"Current VIN CIM: " + cimVIN,
		"Done",
	} {
		if !rec.hasMessage(m) {
			t.Errorf("message %q missing from %q", m, rec.messages())
		}
	}
	if rec.hasMessage("Virginize by pin stored in ECU failed, trying by flashing T8 NVDM") {
		t.Error("tried to flash the NVDM after the PIN unlocked the ECU")
	}
}

// MarryECU gives up with an error, without touching the flash or the CIM,
// when the PIN does not unlock the ECU and flashing the NVDM is not allowed.
func TestMarryECUNVDMNotAllowed(t *testing.T) {
	test.NewTempApp(t) // settings.PrefsNvdm unset
	img := nvdmImage(t, "4711")
	ecu := strandedECU(img, "0000") // the stored PIN is not the one that unlocks
	noteWaits(t, ecu)
	c, rec := newClient(t, ecu)

	err := c.MarryECU(testCtx(t), "1234")
	if err == nil || err.Error() != "System partition not unlocked, try again with checked this option" {
		t.Fatalf("MarryECU = %v, want the system partition error", err)
	}
	checkLog(t, ecu, cat(readPIN, pinUnlock, checkVIN))
	if got, want := devctlOf(ecu), [][]byte{
		append([]byte{0x60}, pinCode("4711")...),
		append([]byte{0x6E}, pinCode("4711")...),
	}; !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("ECU device controls = % X, want % X", got, want)
	}
	if !bytes.Equal(ecu.flashCopy(), img) {
		t.Error("flash changed")
	}
	if n := len(ecu.sentTo(0x245)); n != 0 {
		t.Errorf("%d frames sent to the CIM", n)
	}
	if w := writesOf(ecu); len(w) != 0 {
		t.Errorf("ECU writes = %+v, want none", w)
	}
	if !rec.hasMessage("Virginize by pin stored in ECU failed, trying by flashing T8 NVDM") {
		t.Errorf("messages = %q", rec.messages())
	}
}

// virginBlocks returns the addresses of the 0x80 byte blocks of the virgin
// NVDM that are not erased, which is what a write of it transfers.
func virginBlocks() []int {
	var out []int
	for a := 0; a < len(virginNVDM); a += 0x80 {
		if !bytes.Equal(virginNVDM[a:a+0x80], bytes.Repeat([]byte{0xFF}, 0x80)) {
			out = append(out, 0x4000+a)
		}
	}
	return out
}

// MarryECU with flashing the NVDM allowed, when the PIN does not unlock the
// ECU, starts the legion again, erases the NVDM partitions (2 and 3), writes
// the virgin NVDM into them, verifies them, marries the MCP, leaves the
// legion and marries the now virgin ECU to the CIM.
func TestMarryECUFlashNVDM(t *testing.T) {
	app := test.NewTempApp(t)
	app.Preferences().SetBool(settings.PrefsNvdm, true)
	img := nvdmImage(t, "4711")
	ecu := strandedECU(img, "0000")
	noteWaits(t, ecu)
	c, rec := newClient(t, ecu)

	if err := c.MarryECU(testCtx(t), "1234"); err != nil {
		t.Fatal(err)
	}

	checkLog(t, ecu, cat(
		readPIN, pinUnlock, checkVIN,
		bootstrap,
		[]string{"erase 0x6", "read F0", "36 00", "demand 02", "read 07", "demand 02", "read 07", "demand 05", "read 05", "wait 200ms", "exit", "wait 1s"},
		marry,
	))
	// device 6, 34 01, ~mask, mask with its bytes swapped
	if !ecu.sentWith(0x06, 0x34, 0x01, 0xFF, 0xF9, 0x00, 0x06, 0x00) {
		t.Errorf("no erase command for mask 0x6 in % X", ecu.sentTo(0x7E0))
	}
	unlock := ecu.lock()
	transfers := slices.Clone(ecu.transfers)
	unlock()
	if got, want := transfers, virginBlocks(); !slices.Equal(got, want) {
		t.Errorf("written blocks = %#x, want %#x", got, want)
	}
	if got := ecu.demandsOf(2); !slices.Equal(got, []uint16{2, 3}) {
		t.Errorf("md5 demands = %v, want partitions 2 and 3", got)
	}
	if got := ecu.demandsOf(5); len(got) != 1 {
		t.Errorf("MCP marry demands = %v, want one", got)
	}
	want := slices.Clone(img)
	copy(want[0x4000:0x8000], virginNVDM)
	if !bytes.Equal(ecu.flashCopy(), want) {
		t.Error("flash does not hold the image with a virgin NVDM")
	}
	// the ECU took the CIM's VIN only once its NVDM was virgin
	checkWrites(t, "ECU", writesOf(ecu), marriedWrites[1:]...)
	checkWrites(t, "CIM", writesOf(ecu.cim), didWrite{0x60, []byte("1234")}, didWrite{0x63, []byte{}})
	for _, m := range []string{
		"Virginize by pin stored in ECU failed, trying by flashing T8 NVDM",
		"MCP Firmware information: MCP 4.3.21",
		"NVDM virginized",
		"Reset ECU",
		"Done",
	} {
		if !rec.hasMessage(m) {
			t.Errorf("message %q missing from %q", m, rec.messages())
		}
	}
}

// MarryECU stops when the written NVDM does not verify: it neither marries
// the MCP nor leaves the legion, and does not touch the CIM.
func TestMarryECUFlashNVDMVerifyFails(t *testing.T) {
	app := test.NewTempApp(t)
	app.Preferences().SetBool(settings.PrefsNvdm, true)
	ecu := strandedECU(nvdmImage(t, "4711"), "0000")
	ecu.dropWrites = true
	noteWaits(t, ecu)
	c, _ := newClient(t, ecu)

	err := c.MarryECU(testCtx(t), "1234")
	if err == nil || err.Error() != "nvdm verification failed" {
		t.Fatalf("MarryECU = %v, want nvdm verification failed", err)
	}
	checkLog(t, ecu, cat(
		readPIN, pinUnlock, checkVIN,
		bootstrap,
		[]string{"erase 0x6", "read F0", "36 00", "demand 02", "read 07", "demand 02", "read 07"},
	))
	if n := len(ecu.sentTo(0x245)); n != 0 {
		t.Errorf("%d frames sent to the CIM", n)
	}
}

// MarryECU stops with the ECU's negative response, leaving the flash and
// the CIM alone, when the ECU refuses a step of virginizing it once the
// security access delay has passed.
func TestMarryECUVirginizeStepRefused(t *testing.T) {
	for _, c := range []struct {
		name     string
		sid, nrc byte // refused from the security access delay on
		want     []string
	}{
		// the delay was not enough: no PIN is sent
		{"security access before the stored PIN", 0x27, 0x37, cat(readPIN, []string{"wait 30s", "27 01"})},
		// no programming session for starting the legion again
		{"bootstrap for flashing the NVDM", 0x10, 0x22, cat(readPIN, pinUnlock, checkVIN, []string{"ping", "ping", "ping", "10 02"})},
	} {
		t.Run(c.name, func(t *testing.T) {
			app := test.NewTempApp(t)
			app.Preferences().SetBool(settings.PrefsNvdm, true)
			ecu := strandedECU(nvdmImage(t, "4711"), "0000")
			old := wait
			wait = func(ctx context.Context, d time.Duration) error {
				defer ecu.lock()()
				ecu.note("wait %v", d)
				ecu.nrc[c.sid] = c.nrc
				return ctx.Err()
			}
			t.Cleanup(func() { wait = old })
			cl, _ := newClient(t, ecu)

			err := cl.MarryECU(testCtx(t), "1234")
			var gmErr *gmlan.GMError
			if !errors.As(err, &gmErr) {
				t.Fatalf("MarryECU = %v, want the ECU's negative response", err)
			}
			checkLog(t, ecu, c.want)
			checkNoFlashChanges(t, ecu)
			if n := len(ecu.sentTo(0x245)); n != 0 {
				t.Errorf("%d frames sent to the CIM", n)
			}
		})
	}
}

// cancelDuring runs MarryECU's wait number n (counting from 0) for real,
// stretched to at least 1 s so that the cancel 20 ms into it surely lands
// inside it. The other 25 ms delays run for real as well, the rest are
// skipped. The returned duration is how long wait n ran.
func cancelDuring(t *testing.T, n int, cancel context.CancelFunc) *time.Duration {
	real := wait
	waited := new(time.Duration)
	calls := 0
	wait = func(ctx context.Context, d time.Duration) error {
		defer func() { calls++ }()
		switch {
		case calls == n:
			time.AfterFunc(20*time.Millisecond, cancel)
			start := time.Now()
			err := real(ctx, max(d, time.Second))
			*waited = time.Since(start)
			return err
		case d == 25*time.Millisecond:
			return real(ctx, d)
		}
		return ctx.Err()
	}
	t.Cleanup(func() { wait = real })
	return waited
}

// MarryECU ends with context.Canceled when its context is cancelled during
// one of its delays: it cuts the delay short and sends nothing more.
func TestMarryECUCancelledDuringWait(t *testing.T) {
	// what a marry of an ECU whose VIN can be changed sends to each node,
	// delays left out
	ecuFull := cat(checkVIN, checkVIN, []string{"10 02", "04", "20", "1A 90", "3B 90", "27 01", "27 02", "1A 90", "3B 25", "3B 02", "AE 18", "20"})
	cimFull := []string{"04", "27 01", "27 02", "1A 90", "3B 60", "3B 63", "20"}
	virgin := func(*testing.T) *fakeT8 {
		ecu := newFakeT8()
		ecu.dids[0x90] = []byte(ecuVIN)
		ecu.cim = newFakeCIM()
		ecu.cim.dids[0x90] = []byte(cimVIN)
		return ecu
	}
	for _, c := range []struct {
		name    string
		wait    int // the delay cancelled
		ecu     func(*testing.T) *fakeT8
		wantECU []string
		wantCIM []string
	}{
		{"25 ms after the ECU tester present", 0, virgin, ecuFull[:8], nil},
		{"200 ms after clearing the CIM's DTCs", 1, virgin, ecuFull[:11], cimFull[:1]},
		{"1 s after the PIN went to the CIM", 2, virgin, ecuFull[:13], cimFull[:5]},
		{"first 1 s of waiting for the marry", 3, virgin, ecuFull[:13], cimFull[:6]},
		{"30 s security access delay before the stored PIN", 0, func(t *testing.T) *fakeT8 {
			return strandedECU(nvdmImage(t, "4711"), "4711")
		}, readPIN, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			test.NewTempApp(t)
			ecu := c.ecu(t)
			cl, rec := newClient(t, ecu)
			ctx, cancel := context.WithCancel(testCtx(t))
			defer cancel()
			waited := cancelDuring(t, c.wait, cancel)

			err := cl.MarryECU(ctx, "1234")
			if err != context.Canceled {
				t.Fatalf("MarryECU = %v, want context.Canceled", err)
			}
			if *waited == 0 || *waited >= 500*time.Millisecond {
				t.Errorf("the cancelled delay ran for %v, want it cut short", *waited)
			}
			checkLog(t, ecu, c.wantECU)
			if got := requests(ecu.cim); !slices.Equal(got, c.wantCIM) {
				t.Errorf("CIM requests = %q, want %q", got, c.wantCIM)
			}
			if rec.hasMessage("Done") {
				t.Errorf("finished after the cancel: %q", rec.messages())
			}
		})
	}
}

// MarryECU on an ECU whose NVDM holds no flash block (erased by an
// interrupted NVDM flash) has no stored PIN to try; it must still end with
// an error rather than crash, without touching the CIM.
func TestMarryECUErasedNVDM(t *testing.T) {
	test.NewTempApp(t)
	img := flashImage()
	copy(img[0x4000:0x8000], bytes.Repeat([]byte{0xFF}, 0x4000))
	ecu := strandedECU(img, "4711")
	noteWaits(t, ecu)
	c, rec := newClient(t, ecu)

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("MarryECU panicked: %v", r)
			}
		}()
		err = c.MarryECU(testCtx(t), "1234")
	}()
	if err == nil {
		t.Fatal("MarryECU succeeded on a married ECU without a PIN")
	}
	// No security access delay and no device controls for a PIN it does not have.
	checkLog(t, ecu, cat(readPIN, checkVIN))
	if got := devctlOf(ecu); len(got) != 0 {
		t.Errorf("ECU device controls = % X, want none", got)
	}
	if !rec.hasMessage("No PIN stored in ECU") {
		t.Errorf("messages = %q", rec.messages())
	}
	if n := len(ecu.sentTo(0x245)); n != 0 {
		t.Errorf("%d frames sent to the CIM", n)
	}
}
