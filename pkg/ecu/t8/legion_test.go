package t8

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

// flashImage returns a mostly erased 1 MiB T8 image with data in the boot,
// NVDM and application partitions.
func flashImage() []byte {
	b := bytes.Repeat([]byte{0xFF}, 0x100000)
	copy(b, []byte{0x00, 0x10, 0x0C, 0x00, 0x00, 0x00, 0x04, 0x8C}) // stack pointer, reset vector
	for i := 0x4000; i < 0x4200; i++ {
		b[i] = byte(i)
	}
	for i := 0x20000; i < 0x20100; i++ {
		b[i] = byte(i * 7)
	}
	b[0xFFFFF] = 0x5A
	return b
}

// legionECU returns a fake T8 already running the legion bootloader with
// img in flash.
func legionECU(img []byte) *fakeT8 {
	ecu := newFakeT8()
	ecu.legion = true
	ecu.flash = append([]byte(nil), img...)
	ecu.mcpVersion = "MCP 4.3.21"
	return ecu
}

func (s *fakeT8) demandsOf(cmd byte) []uint16 {
	defer s.lock()()
	return slices.Clone(s.demands[cmd])
}

func (s *fakeT8) flashCopy() []byte {
	defer s.lock()()
	return slices.Clone(s.flash)
}

// DumpECU reads the whole flash through the legion bootloader, including
// the blocks the loader reports as erased, and checks it against the
// loader's md5 of the full image.
func TestDumpECU(t *testing.T) {
	img := flashImage()
	ecu := legionECU(img)
	c, rec := newClient(t, ecu)

	bin, err := c.DumpECU(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, img) {
		t.Fatal("dump differs from the flash content")
	}
	if got := ecu.demandsOf(2); !slices.Equal(got, []uint16{0}) {
		t.Errorf("md5 demands = %v, want one for the full image (0)", got)
	}
	if !rec.hasMessage("Dumping ECU") || !rec.hasMessage("Verifying md5..") {
		t.Errorf("messages = %q", rec.msgs)
	}
}

// DumpECU rejects a dump whose md5 does not match the loader's.
func TestDumpECUMD5Mismatch(t *testing.T) {
	ecu := legionECU(flashImage())
	ecu.md5Lie[0] = 1
	c, _ := newClient(t, ecu)
	bin, err := c.DumpECU(testCtx(t))
	if err == nil || !strings.Contains(err.Error(), "md5") {
		t.Fatalf("DumpECU = %d bytes, %v; want an md5 verification error", len(bin), err)
	}
	if bin != nil {
		t.Error("DumpECU returned data with the error")
	}
}

// DumpECU stops when the legion bootloader cannot be started and reads no
// flash.
func TestDumpECUBootstrapFails(t *testing.T) {
	ecu := newFakeT8() // no legion, and the ECU refuses the diagnostic session
	ecu.nrc[0x10] = 0x22
	c, _ := newClient(t, ecu)
	if _, err := c.DumpECU(testCtx(t)); err == nil {
		t.Fatal("DumpECU succeeded without a bootloader")
	}
	if ecu.sentWith(0x06, 0x21) {
		t.Error("flash read attempted without a bootloader")
	}
}

// FlashECU and RecoverECU accept only full 1 MiB images and send nothing
// for anything else.
func TestFlashWrongSize(t *testing.T) {
	ecu := newFakeT8()
	c, _ := newClient(t, ecu)
	short := make([]byte, 0x80000)
	if err := c.FlashECU(testCtx(t), short); err == nil {
		t.Error("FlashECU accepted 512 KiB")
	}
	if err := c.RecoverECU(testCtx(t), short); err == nil {
		t.Error("RecoverECU accepted 512 KiB")
	}
	if n := len(ecu.sentTo(0x7E0)); n != 0 {
		t.Errorf("%d frames sent", n)
	}
}

// FlashECU and RecoverECU leave an ECU that already holds the image
// unerased and unwritten, then marry the MCP.
func TestFlashIdentical(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(*Client, context.Context, []byte) error
	}{
		{"FlashECU", (*Client).FlashECU},
		{"RecoverECU", (*Client).RecoverECU},
	} {
		t.Run(c.name, func(t *testing.T) {
			test.NewTempApp(t)
			img := flashImage()
			ecu := legionECU(img)
			cl, rec := newClient(t, ecu)
			if err := c.do(cl, testCtx(t), img); err != nil {
				t.Fatal(err)
			}
			if len(ecu.erases) != 0 || len(ecu.transfers) != 0 {
				t.Errorf("erases %v transfers %v, want none", ecu.erases, ecu.transfers)
			}
			// boot and NVDM are not flashed by default: partitions 2..9 compared
			if got := ecu.demandsOf(2); !slices.Equal(got, []uint16{2, 3, 4, 5, 6, 7, 8, 9}) {
				t.Errorf("md5 demands = %v, want partitions 2..9", got)
			}
			if got := ecu.demandsOf(5); len(got) != 1 {
				t.Errorf("marry demands = %v, want one", got)
			}
			if !rec.hasMessage("MCP Firmware information: MCP 4.3.21") {
				t.Errorf("messages = %q", rec.msgs)
			}
		})
	}
}

// FlashECU leaves the boot and NVDM partitions alone unless their
// preferences are set, even when they differ from the image.
func TestFlashKeepsBootAndNVDM(t *testing.T) {
	for _, c := range []struct {
		name string
		addr int
	}{
		{"boot", 0x1000},
		{"NVDM", 0x4300},
	} {
		t.Run(c.name, func(t *testing.T) {
			test.NewTempApp(t)
			img := flashImage()
			ecu := legionECU(img)
			cl, _ := newClient(t, ecu)
			file := slices.Clone(img)
			file[c.addr] = 0x22
			if err := cl.FlashECU(testCtx(t), file); err != nil {
				t.Fatal(err)
			}
			if len(ecu.erases) != 0 || len(ecu.transfers) != 0 {
				t.Errorf("erases %v transfers %v, want none", ecu.erases, ecu.transfers)
			}
			if !bytes.Equal(ecu.flashCopy(), img) {
				t.Error("flash changed")
			}
		})
	}
}

// FlashECU erases exactly the partitions that differ (boot and NVDM
// included when their preferences are set), writes the non-erased blocks of
// the image into them and verifies them.
func TestFlashECUWritesDifferences(t *testing.T) {
	app := test.NewTempApp(t)
	app.Preferences().SetBool(settings.PrefsBoot, true)
	app.Preferences().SetBool(settings.PrefsNvdm, true)

	img := flashImage()
	ecu := legionECU(img)
	c, _ := newClient(t, ecu)
	file := slices.Clone(img)
	file[0x1000] = 0x01                                    // partition 1, boot
	file[0x4300] = 0x02                                    // partition 2, NVDM
	copy(file[0x20100:], bytes.Repeat([]byte{0x11}, 0x80)) // partition 5
	if err := c.FlashECU(testCtx(t), file); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(ecu.erases, []uint64{0x13}) {
		t.Errorf("erase masks = %#x, want [0x13] (partitions 1, 2, 5)", ecu.erases)
	}
	// device 6, 34 01, ~mask, mask with its bytes swapped
	if !ecu.sentWith(0x06, 0x34, 0x01, 0xFF, 0xEC, 0x00, 0x13, 0x00) {
		t.Errorf("no erase command for mask 0x13 in % X", ecu.sentTo(0x7E0))
	}
	wantBlocks := []int{0x0000, 0x1000, 0x4000, 0x4080, 0x4100, 0x4180, 0x4300, 0x20000, 0x20080, 0x20100}
	if !slices.Equal(ecu.transfers, wantBlocks) {
		t.Errorf("written blocks = %#x, want %#x", ecu.transfers, wantBlocks)
	}
	if !bytes.Equal(ecu.flashCopy(), file) {
		t.Error("flash does not hold the image after flashing")
	}
	if got := ecu.demandsOf(5); len(got) != 1 {
		t.Errorf("marry demands = %v, want one", got)
	}
}

// FlashECU fails when the flashed partitions do not verify, and does not
// marry the MCP then.
func TestFlashECUVerifyFails(t *testing.T) {
	test.NewTempApp(t)
	img := flashImage()
	ecu := legionECU(img)
	ecu.dropWrites = true
	c, _ := newClient(t, ecu)
	file := slices.Clone(img)
	copy(file[0x20100:], bytes.Repeat([]byte{0x11}, 0x80))

	err := c.FlashECU(testCtx(t), file)
	if err == nil || !strings.Contains(err.Error(), "md5") {
		t.Fatalf("FlashECU = %v, want an md5 verification error", err)
	}
	if got := ecu.demandsOf(5); len(got) != 0 {
		t.Errorf("MCP married after a failed verification: %v", got)
	}
}

// CheckIfCanChangeVIN unlocks the ECU and writes its own VIN back to it.
func TestCheckIfCanChangeVIN(t *testing.T) {
	ecu := newFakeT8()
	ecu.dids[0x90] = []byte("YS3FB49F741000001")
	c, _ := newClient(t, ecu)
	if err := c.CheckIfCanChangeVIN(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	w := writesOf(ecu)
	if len(w) != 1 || w[0].pid != 0x90 || string(w[0].data) != "YS3FB49F741000001" {
		t.Errorf("writes = %+v, want the VIN written back", w)
	}
}

// CheckIfCanChangeVIN reports an ECU that refuses the VIN write.
func TestCheckIfCanChangeVINRefused(t *testing.T) {
	ecu := newFakeT8()
	ecu.dids[0x90] = []byte("YS3FB49F741000001")
	ecu.nrc[0x3B] = 0x22
	c, _ := newClient(t, ecu)
	if err := c.CheckIfCanChangeVIN(testCtx(t)); err == nil {
		t.Fatal("CheckIfCanChangeVIN succeeded with the write refused")
	}
}

// MarryECU on an ECU whose VIN cannot be changed tries to virginize it
// through the bootloader and gives up, without touching the CIM, when the
// bootloader cannot be started.
func TestMarryECUBootstrapFails(t *testing.T) {
	test.NewTempApp(t)
	ecu := newFakeT8()
	ecu.nrc[0x27] = 0x37 // securityAccess: requiredTimeDelayNotExpired
	ecu.nrc[0x10] = 0x22 // no diagnostic session for the bootloader either
	c, rec := newClient(t, ecu)

	if err := c.MarryECU(testCtx(t), "1234"); err == nil {
		t.Fatal("MarryECU succeeded")
	}
	if !rec.hasMessage("Virginize ECU by pin stored in ECU") {
		t.Errorf("messages = %q", rec.msgs)
	}
	if n := len(ecu.sentTo(0x245)); n != 0 {
		t.Errorf("%d frames sent to the CIM", n)
	}
	if w := writesOf(ecu); len(w) != 0 {
		t.Errorf("writes = %+v, want none", w)
	}
}

// MarryECU on an ECU whose VIN can be changed skips both virginize steps and
// marries it to the CIM: it clears the DTCs of both, unlocks the CIM with the
// CIM key, copies the CIM's VIN into the ECU, sends the PIN and the marry
// command to the CIM, then sets oil quality 50%, speed limiter 240 km/h and
// E85 10%.
func TestMarryECU(t *testing.T) {
	skipWaits(t)
	const ecuVIN, cimVIN = "YS3FB49F741000001", "YS3FH41U861000002"
	ecu := newFakeT8()
	ecu.dids[0x90] = []byte(ecuVIN)
	ecu.cim = newFakeCIM()
	ecu.cim.dids[0x90] = []byte(cimVIN)
	c, rec := newClient(t, ecu)

	if err := c.MarryECU(testCtx(t), "1234"); err != nil {
		t.Fatal(err)
	}

	// CheckIfCanChangeVIN runs twice and writes the ECU's own VIN back each time
	wantECU := []didWrite{
		{0x90, []byte(ecuVIN)},
		{0x90, []byte(ecuVIN)},
		{0x90, []byte(cimVIN)},
		{0x25, []byte{0x00, 0x00, 0x32, 0x00}},
		{0x02, []byte{0x09, 0x60}},
	}
	wantCIM := []didWrite{
		{0x60, []byte("1234")},
		{0x63, []byte{}},
	}
	for _, w := range []struct {
		node string
		got  []didWrite
		want []didWrite
	}{
		{"ECU", writesOf(ecu), wantECU},
		{"CIM", writesOf(ecu.cim), wantCIM},
	} {
		if !slices.EqualFunc(w.got, w.want, func(a, b didWrite) bool { return a.pid == b.pid && bytes.Equal(a.data, b.data) }) {
			t.Errorf("%s writes = %+v, want %+v", w.node, w.got, w.want)
		}
	}

	unlock := ecu.lock()
	devctl := slices.Clone(ecu.devctl)
	unlock()
	if len(devctl) != 1 || !bytes.Equal(devctl[0], []byte{0x18, 0x00, 0x0A}) {
		t.Errorf("ECU device control = % X, want [18 00 0A] (E85 10%%)", devctl)
	}
	if got := ecu.sentTo(0x7DF); len(got) != 1 || !bytes.Equal(got[0], []byte{0x01, 0x04}) {
		t.Errorf("functional requests = % X, want one clear DTCs (01 04)", got)
	}
	var cimClear bool
	for _, d := range ecu.sentTo(0x245) {
		cimClear = cimClear || bytes.Equal(d, []byte{0x01, 0x04})
	}
	if !cimClear {
		t.Errorf("no clear DTCs (01 04) in the CIM requests % X", ecu.sentTo(0x245))
	}
	for _, m := range []string{"Current VIN CIM: " + cimVIN, "VIN programed to ECU", "Done"} {
		if !rec.hasMessage(m) {
			t.Errorf("message %q missing from %q", m, rec.msgs)
		}
	}
	if rec.hasMessage("Virginize ECU by pin stored in ECU") {
		t.Error("tried to virginize an ECU whose VIN can be changed")
	}
}
