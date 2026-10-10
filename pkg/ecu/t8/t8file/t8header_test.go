package t8file

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// Test images are synthesized: an erased 1 MiB flash carrying the T8 reset
// vector, a PI area encoded the way the application stores it and an NVDM
// laid out like a stock ECU's (pointer table, info record, flash blocks).

// encPI is the inverse of the PI-area decode (b + 0xD6) ^ 0x21.
func encPI(b byte) byte { return (b ^ 0x21) - 0xD6 }

// encFB is the inverse of the flash-block decode (b + 0x53) ^ 0xA4.
func encFB(b byte) byte { return (b ^ 0xA4) - 0x53 }

// blankBin returns an erased 1 MiB image that passes isValidT8Bin.
func blankBin() []byte {
	b := bytes.Repeat([]byte{0xFF}, 0x100000)
	copy(b[4:8], T8ResetVector)
	return b
}

type piRecord struct {
	typ  byte
	data []byte
}

// putPIArea encodes recs as a PI area at off, points the checksum-area
// pointer at 0x20140 to it and terminates it with F7 F7. The erased flash
// after the terminator marks the end of the area.
func putPIArea(bin []byte, off int, recs ...piRecord) {
	binary.BigEndian.PutUint32(bin[0x20140:], uint32(off))
	p := off
	for _, r := range recs {
		bin[p], bin[p+1] = encPI(byte(len(r.data))), encPI(r.typ)
		p += 2
		for _, c := range r.data {
			bin[p] = encPI(c)
			p++
		}
	}
	bin[p], bin[p+1] = encPI(0xF7), encPI(0xF7)
}

// block is the immobilizer data carried by one NVDM flash block.
type block struct {
	vin, desc, itf, pin string
	psk, isk            []byte
}

// plain returns the decoded 0x130-byte block.
func (b block) plain() []byte {
	p := make([]byte, 0x130)
	copy(p[0x40:], b.pin)
	copy(p[0x55:], b.psk)
	copy(p[0x61:], b.isk)
	copy(p[0xAF:], b.desc)
	copy(p[0xE0:], b.vin)
	for i := 0x101; i < 0x101+0x0B; i++ {
		p[i] = 0xFF
	}
	copy(p[0x101:], b.itf)
	return p
}

// raw returns the block as stored in flash.
func (b block) raw() []byte {
	p := b.plain()
	for i := range p {
		p[i] = encFB(p[i])
	}
	return p
}

type placed struct {
	addr int
	typ  byte
	b    block
}

var tableHdr = []byte{0x44, 0x2A, 0x01, 0x30, 0x00, 0x00}

func ptr(addr int, typ byte) []byte {
	return []byte{byte(addr >> 8), byte(addr), 0x01, 0x00, 0x00, typ}
}

// putTable writes 6-byte entries into the pointer table of the NVDM segment
// at seg (0x4000 or 0x6000).
func putTable(bin []byte, seg int, entries ...[]byte) {
	idx := seg + 0x0E
	for _, e := range entries {
		copy(bin[idx:], e)
		idx += 6
	}
}

// putSegment lays out an NVDM segment like a stock ECU does: a pointer to
// the info record (part and serial number in plain ASCII at seg+0xCE), the
// second pointer the decoder ignores, then a header and pointer per block.
func putSegment(bin []byte, seg int, partNo, serial string, blocks ...placed) {
	entries := [][]byte{
		{0x44, 0x2A, 0x02, 0x00, 0x00, 0x00},
		{byte((seg + 0xCE) >> 8), byte(seg + 0xCE), 0x00, 0xFF, 0xFF, 0xFF},
		{0x44, 0x2A, 0x00, 0x0A, 0x00, 0x00},
		{byte((seg + 0x2CE) >> 8), byte(seg + 0x2CE), 0x02, 0x10, 0x0B, 0x4C},
	}
	for _, p := range blocks {
		entries = append(entries, tableHdr, ptr(p.addr, p.typ))
	}
	putTable(bin, seg, entries...)
	copy(bin[seg+0xCE:], []byte(partNo+strings.Repeat(" ", 10-len(partNo))))
	copy(bin[seg+0xCE+10:], []byte(serial+strings.Repeat(" ", 16-len(serial))))
	for _, p := range blocks {
		copy(bin[p.addr:], p.b.raw())
	}
}

var (
	blockA = block{vin: "YS3FB49F741000001", desc: "Trionic 8 P6.8  ", itf: "PPCAN 210", pin: "AB12",
		psk: []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB}, isk: []byte{0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54}}
	blockB = block{vin: "YS3FB49F741000002", desc: "Trionic 8 P6.8  ", itf: "PPCAN 210", pin: "CD34",
		psk: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}, isk: []byte{0x66, 0x55, 0x44, 0x33, 0x22, 0x11}}
	blockC = block{vin: "YS3FB49F741000003", desc: "Trionic 8 P6.9  ", itf: "S001015575", pin: "EF56",
		psk: []byte{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}, isk: []byte{0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5}}
)

// testPI are records as found in a stock PI area, text and hex typed.
var testPI = []piRecord{
	{0x0D, bytes.Repeat([]byte{0x58}, 16)}, // layer 1 checksum, hex typed
	{0x92, []byte("GMPT 0100")},
	{0x97, []byte("ECM                 ")},
	{0xC1, []byte("55559436")},
	{0x08, []byte("FA5I_C_FME2_65_FIEF_82h       ")},
	{0xFC, []byte{0x00, 0x0B, 0xAF, 0x06}}, // checksum last address, hex typed
	{0x0A, []byte("2004-08-12 14:13:34")},
	{0x1D, []byte("Test Programmer")},
	{0x10, []byte("EOLStation2")},
}

// testBin returns an image with a PI area and both NVDM segments populated:
// blocks A (active) and B (history) in the low segment, C in the high one.
func testBin() []byte {
	bin := blankBin()
	putPIArea(bin, 0x80000, testPI...)
	putSegment(bin, 0x4000, "55353231", "863232AC0A06C72H",
		placed{0x42D8, 0x03, blockA},
		placed{0x4408, 0x01, blockB},
	)
	putSegment(bin, 0x6000, "55353231", "863232AC0A06C72H",
		placed{0x62D8, 0x03, blockC},
	)
	return bin
}

// DecodeInfo reads the text-typed PI-area records into the header and
// ignores the hex-typed ones.
func TestDecodeInfo(t *testing.T) {
	th := new(T8Header)
	th.DecodeInfo(testBin())

	for _, c := range []struct{ name, got, want string }{
		{"HardwareID", th.HardwareID(), "GMPT 0100"},
		{"DeviceType", th.DeviceType(), "ECM                 "},
		{"PartNumber", th.PartNumber(), "55559436"},
		{"SoftwareVersion", th.SoftwareVersion(), "FA5I_C_FME2_65_FIEF_82h       "},
		{"ReleaseDate", th.ReleaseDate(), "2004-08-12 14:13:34"},
		{"ProgrammerName", th.ProgrammerName(), "Test Programmer"},
		{"ProgrammerDevice", th.ProgrammerDevice(), "EOLStation2"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// DecodeInfo stops at a record whose length runs past the end of the area,
// keeping what it decoded before it.
func TestDecodeInfoTruncatedRecord(t *testing.T) {
	bin := blankBin()
	putPIArea(bin, 0x80000, piRecord{0x92, []byte("GMPT 0100")})
	// overwrite the terminator with a record claiming 0x40 bytes when only
	// erased flash follows
	p := 0x80000 + 2 + len("GMPT 0100")
	bin[p], bin[p+1] = encPI(0x40), encPI(0x08)

	th := new(T8Header)
	th.DecodeInfo(bin)
	if th.HardwareID() != "GMPT 0100" {
		t.Errorf("HardwareID = %q, want %q", th.HardwareID(), "GMPT 0100")
	}
	if th.SoftwareVersion() != "" {
		t.Errorf("SoftwareVersion = %q, want empty for a truncated record", th.SoftwareVersion())
	}
}

// DecodeInfo leaves the header untouched when given no data.
func TestDecodeInfoEmpty(t *testing.T) {
	th := new(T8Header)
	th.SetHardwareID("kept")
	th.DecodeInfo(nil)
	if th.HardwareID() != "kept" {
		t.Errorf("HardwareID = %q, want it untouched", th.HardwareID())
	}
}

// DecodeInfo treats a checksum-area pointer past the end of the image (as in
// a dump with an erased application) as "no PI area" instead of panicking.
func TestDecodeInfoPointerPastEnd(t *testing.T) {
	bin := blankBin() // 0x20140 holds FF FF FF FF
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DecodeInfo panicked: %v", r)
		}
	}()
	th := new(T8Header)
	th.DecodeInfo(bin)
	if th.SoftwareVersion() != "" {
		t.Errorf("SoftwareVersion = %q, want empty", th.SoftwareVersion())
	}
}

// DecodeExtraInfo collects the flash blocks of both NVDM segments in table
// order, reads part and serial number from the info record and takes the
// immobilizer data from the last block.
func TestDecodeExtraInfo(t *testing.T) {
	th := new(T8Header)
	th.DecodeExtraInfo(testBin())

	type fbWant struct{ addr, typ, num int }
	want := []fbWant{{0x42D8, 0x03, 0}, {0x4408, 0x01, 1}, {0x62D8, 0x03, 2}}
	fbs := th.FlashBlocks()
	if len(fbs) != len(want) {
		t.Fatalf("got %d flash blocks, want %d", len(fbs), len(want))
	}
	for i, w := range want {
		fb := fbs[i]
		if fb.BlockAddress != w.addr || fb.BlockType != w.typ || fb.BlockNumber != w.num {
			t.Errorf("block %d = addr %#x type %#x num %d, want addr %#x type %#x num %d",
				i, fb.BlockAddress, fb.BlockType, fb.BlockNumber, w.addr, w.typ, w.num)
		}
		if len(fb.BlockData) != 0x130 {
			t.Errorf("block %d data length = %#x, want 0x130", i, len(fb.BlockData))
		}
	}
	if !bytes.Equal(fbs[1].BlockData, blockB.raw()) {
		t.Error("block 1 data is not the raw flash content")
	}

	for _, c := range []struct{ name, got, want string }{
		{"PartNumber", th.PartNumber(), "55353231"},
		{"SerialNumber", th.SerialNumber(), "863232AC0A06C72H"},
		{"VIN", th.VIN(), blockC.vin},
		{"EcuDescription", th.EcuDescription(), blockC.desc},
		{"InterfaceDevice", th.InterfaceDevice(), blockC.itf},
		{"PIN", th.PIN(), blockC.pin},
		{"PSK", th.PSK(), "A0A1A2A3A4A5"},
		{"ISK", th.ISK(), "B0B1B2B3B4B5"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// DecodeExtraInfo finds the NVDM at 0x4000 in a full image, in the 0x8000
// buffer a flash read of 0..0x8000 returns (MarryECU) and in a bare 0x4000
// NVDM dump alike.
func TestDecodeExtraInfoInputSizes(t *testing.T) {
	full := testBin()
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"full image", full},
		{"0x8000 read", full[:0x8000]},
		{"bare NVDM", full[0x4000:0x8000]},
	} {
		t.Run(c.name, func(t *testing.T) {
			th := new(T8Header)
			th.DecodeExtraInfo(c.data)
			if len(th.FlashBlocks()) != 3 {
				t.Errorf("got %d flash blocks, want 3", len(th.FlashBlocks()))
			}
			if th.VIN() != blockC.vin || th.PartNumber() != "55353231" {
				t.Errorf("VIN %q part %q, want %q %q", th.VIN(), th.PartNumber(), blockC.vin, "55353231")
			}
		})
	}
}

// The second non-header pointer of each table is not a block pointer and is
// ignored even when it carries a block type.
func TestDecodeExtraInfoSkipsSecondPointer(t *testing.T) {
	bin := blankBin()
	putTable(bin, 0x4000,
		ptr(0x42D8, 0x03), // first pointer: used
		ptr(0x4408, 0x03), // second pointer: ignored
		ptr(0x4538, 0x01), // third and later: used
	)
	copy(bin[0x42D8:], blockA.raw())
	copy(bin[0x4408:], blockB.raw())
	copy(bin[0x4538:], blockC.raw())

	th := new(T8Header)
	th.DecodeExtraInfo(bin)
	var addrs []int
	for _, fb := range th.FlashBlocks() {
		addrs = append(addrs, fb.BlockAddress)
	}
	if len(addrs) != 2 || addrs[0] != 0x42D8 || addrs[1] != 0x4538 {
		t.Errorf("block addresses = %#x, want [0x42d8 0x4538]", addrs)
	}
}

// Blocks of unknown type and pointers whose block falls outside the NVDM
// window are skipped, and block numbers stay consecutive.
func TestDecodeExtraInfoSkipsUnusableBlocks(t *testing.T) {
	bin := blankBin()
	putSegment(bin, 0x4000, "55353231", "",
		placed{0x42D8, 0x03, blockA},
		placed{0x4408, 0x02, blockB}, // unknown type
		placed{0x4538, 0x01, blockC},
	)
	// after the 10 entries putSegment wrote: pointers whose block would fall
	// outside the 0x4000..0x8000 window
	putTable(bin, 0x4000+10*6, tableHdr, ptr(0x3F00, 0x03), tableHdr, ptr(0x7F00, 0x03))

	th := new(T8Header)
	th.DecodeExtraInfo(bin)
	fbs := th.FlashBlocks()
	if len(fbs) != 2 {
		t.Fatalf("got %d flash blocks, want 2", len(fbs))
	}
	if fbs[0].BlockAddress != 0x42D8 || fbs[0].BlockNumber != 0 ||
		fbs[1].BlockAddress != 0x4538 || fbs[1].BlockNumber != 1 {
		t.Errorf("blocks = (%#x #%d) (%#x #%d), want (0x42d8 #0) (0x4538 #1)",
			fbs[0].BlockAddress, fbs[0].BlockNumber, fbs[1].BlockAddress, fbs[1].BlockNumber)
	}
	if th.VIN() != blockC.vin {
		t.Errorf("VIN = %q, want %q", th.VIN(), blockC.vin)
	}
}

// A pointer to an erased (all 0xFF) block is not a flash block: the T8Suite
// reference rejects blocks that decode to F6 F6 F6 F6 F6 F6 F6 F6.
func TestDecodeExtraInfoSkipsErasedBlock(t *testing.T) {
	bin := blankBin()
	putSegment(bin, 0x4000, "55353231", "",
		placed{0x42D8, 0x03, blockA},
		placed{0x4408, 0x01, blockB},
	)
	copy(bin[0x4408:], bytes.Repeat([]byte{0xFF}, 0x130))

	th := new(T8Header)
	th.DecodeExtraInfo(bin)
	if n := len(th.FlashBlocks()); n != 1 {
		t.Errorf("got %d flash blocks, want 1 (the erased one skipped)", n)
	}
	if th.VIN() != blockA.vin {
		t.Errorf("VIN = %q, want %q from the only valid block", th.VIN(), blockA.vin)
	}
}

// DecodeExtraInfo on an NVDM without any valid flash block (erased, or not
// an NVDM at all) leaves the header without blocks instead of panicking.
func TestDecodeExtraInfoNoBlocks(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"erased NVDM", blankBin()},
		{"no data", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("DecodeExtraInfo panicked: %v", r)
				}
			}()
			th := new(T8Header)
			th.DecodeExtraInfo(c.data)
			if n := len(th.FlashBlocks()); n != 0 {
				t.Errorf("got %d flash blocks, want 0", n)
			}
		})
	}
}

// EncodeBlock of an unchanged header reproduces the stored block byte for
// byte, and DecodeBlock of an encoded block reads back what was set.
func TestFlashBlockRoundTrip(t *testing.T) {
	fb := FlashBlock{BlockData: blockA.raw()}
	th := new(T8Header)
	fb.DecodeBlock(th)
	if got := fb.EncodeBlock(th); !bytes.Equal(got, blockA.raw()) {
		t.Fatal("re-encoding an unchanged block changed it")
	}

	th.SetVIN(blockB.vin)
	th.SetPIN(blockB.pin)
	th.SetInterfaceDevice("PPCAN 45")
	th.SetPSK("112233445566")
	th.SetISK("665544332211")
	enc := FlashBlock{BlockData: fb.EncodeBlock(th)}

	back := new(T8Header)
	enc.DecodeBlock(back)
	for _, c := range []struct{ name, got, want string }{
		{"VIN", back.VIN(), blockB.vin},
		{"PIN", back.PIN(), blockB.pin},
		{"InterfaceDevice", back.InterfaceDevice(), "PPCAN 45"},
		{"PSK", back.PSK(), "112233445566"},
		{"ISK", back.ISK(), "665544332211"},
		{"EcuDescription", back.EcuDescription(), blockA.desc},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !bytes.Equal(fb.BlockData, blockA.raw()) {
		t.Error("EncodeBlock modified the block it was called on")
	}
}

// EncodeBlock keeps the stored keys when PSK or ISK are not valid hex.
func TestEncodeBlockKeepsKeysOnBadHex(t *testing.T) {
	fb := FlashBlock{BlockData: blockA.raw()}
	th := new(T8Header)
	fb.DecodeBlock(th)
	th.SetPSK("not hex!")
	th.SetISK("0123456789ABCDEF") // valid hex, but longer than 6 bytes

	back := new(T8Header)
	(&FlashBlock{BlockData: fb.EncodeBlock(th)}).DecodeBlock(back)
	if back.PSK() != "0123456789AB" || back.ISK() != "FEDCBA987654" {
		t.Errorf("PSK %q ISK %q, want the stored 0123456789AB FEDCBA987654", back.PSK(), back.ISK())
	}
}

// EncodeBlock stores the whole 16-character ECU description that DecodeBlock
// reads and the edit dialog requires.
func TestEncodeBlockEcuDescription(t *testing.T) {
	fb := FlashBlock{BlockData: blockA.raw()}
	th := new(T8Header)
	fb.DecodeBlock(th)
	th.SetEcuDescription("ABCDEFGHIJKLMNOP")

	back := new(T8Header)
	(&FlashBlock{BlockData: fb.EncodeBlock(th)}).DecodeBlock(back)
	if back.EcuDescription() != "ABCDEFGHIJKLMNOP" {
		t.Errorf("EcuDescription = %q, want %q", back.EcuDescription(), "ABCDEFGHIJKLMNOP")
	}
}

// isValid rejects blocks shorter than the 8-byte marker and accepts a block
// carrying immobilizer data. (Which 8-byte marker it rejects is pinned by
// TestDecodeExtraInfoSkipsErasedBlock.)
func TestFlashBlockIsValid(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
		want bool
	}{
		{"short", []byte{1, 2, 3}, false},
		{"data", blockA.raw(), true},
	} {
		fb := FlashBlock{BlockData: c.data}
		if got := fb.isValid(); got != c.want {
			t.Errorf("%s: isValid = %v, want %v", c.name, got, c.want)
		}
	}
}

// GetFormattedLog lists the decoded header fields and one entry per flash
// block with its address, type and immobilizer data.
func TestGetFormattedLog(t *testing.T) {
	bin := testBin()
	th := new(T8Header)
	th.DecodeInfo(bin)
	th.DecodeExtraInfo(bin)
	out := th.GetFormattedLog()

	for _, want := range []string{
		"ECU Description:  Trionic 8 P6.9  \n",
		"Part Number:      55353231\n",
		"Serial Number:    863232AC0A06C72H\n",
		"Hardware ID:      GMPT 0100\n",
		"Release Date:     2004-08-12 14:13:34\n",
		"Programmer Name:  Test Programmer\n",
		"Total Blocks:     3\n",
		"[Block 00] Addr: 0x42D8, Type: 0x03\n\\__ VIN: YS3FB49F741000001 PIN: AB12\n \\_ ISK: FEDCBA987654 PSK: 0123456789AB\n",
		"[Block 01] Addr: 0x4408, Type: 0x01\n\\__ VIN: YS3FB49F741000002 PIN: CD34\n",
		"[Block 02] Addr: 0x62D8, Type: 0x03\n\\__ VIN: YS3FB49F741000003 PIN: EF56\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log is missing %q\n%s", want, out)
		}
	}
}

// Each setter stores its own field, which only its getter returns.
func TestHeaderAccessors(t *testing.T) {
	th := new(T8Header)
	fields := []struct {
		name string
		set  func(string)
		get  func() string
	}{
		{"PartNumber", th.SetPartNumber, th.PartNumber},
		{"SerialNumber", th.SetSerialNumber, th.SerialNumber},
		{"ProgrammerDevice", th.SetProgrammerDevice, th.ProgrammerDevice},
		{"ProgrammerName", th.SetProgrammerName, th.ProgrammerName},
		{"ReleaseDate", th.SetReleaseDate, th.ReleaseDate},
		{"SoftwareVersion", th.SetSoftwareVersion, th.SoftwareVersion},
		{"HardwareID", th.SetHardwareID, th.HardwareID},
		{"DeviceType", th.SetDeviceType, th.DeviceType},
		{"InterfaceDevice", th.SetInterfaceDevice, th.InterfaceDevice},
		{"EcuDescription", th.SetEcuDescription, th.EcuDescription},
		{"VIN", th.SetVIN, th.VIN},
		{"PIN", th.SetPIN, th.PIN},
		{"PSK", th.SetPSK, th.PSK},
		{"ISK", th.SetISK, th.ISK},
	}
	for _, f := range fields {
		f.set("value of " + f.name)
	}
	for _, f := range fields {
		if got := f.get(); got != "value of "+f.name {
			t.Errorf("%s() = %q, want %q", f.name, got, "value of "+f.name)
		}
	}
}
