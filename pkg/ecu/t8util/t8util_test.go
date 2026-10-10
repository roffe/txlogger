package t8util

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"testing"
)

// codeKey is the 6-byte XOR key Legion uses for flash transfer blocks (the
// same key OEM .gbf files are coded with).
var codeKey = [6]byte{0x39, 0x68, 0x77, 0x6D, 0x47, 0x39}

// patterned returns n bytes where every byte depends on its offset, so a
// hash or block taken from the wrong range cannot match by accident.
func patterned(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i ^ i>>8 ^ i>>16)
	}
	return b
}

// swapPairs returns a copy of b with every 16-bit word byte swapped.
func swapPairs(b []byte) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b); i += 2 {
		out[i], out[i+1] = b[i+1], b[i]
	}
	return out
}

func md5Of(b []byte) []byte {
	s := md5.Sum(b)
	return s[:]
}

// GetPartitionMD5 for the T8 main flash (device 6) hashes the partition
// table ranges for partitions 1-9, the whole 1 MiB for 0, and for the
// "oddball" regions 10-12 hashes from 0x0, 0x4000 and 0x20000 up to the
// binary's last used address plus 0x200.
func TestGetPartitionMD5Main(t *testing.T) {
	bin := patterned(0x100000)
	// last used address pointer at 0x20141..0x20143 = 0x0A1234
	bin[0x20141], bin[0x20142], bin[0x20143] = 0x0A, 0x12, 0x34
	const last = 0x0A1234 + 0x200

	ranges := []struct {
		partition  int
		start, end int
	}{
		{0, 0x000000, 0x100000},
		{1, 0x000000, 0x004000},
		{2, 0x004000, 0x006000},
		{3, 0x006000, 0x008000},
		{4, 0x008000, 0x020000},
		{5, 0x020000, 0x040000},
		{6, 0x040000, 0x060000},
		{7, 0x060000, 0x080000},
		{8, 0x080000, 0x0C0000},
		{9, 0x0C0000, 0x100000},
		{10, 0x000000, last},
		{11, 0x004000, last},
		{12, 0x020000, last},
	}
	for _, r := range ranges {
		got := GetPartitionMD5(bin, 6, r.partition)
		if want := md5Of(bin[r.start:r.end]); !bytes.Equal(got, want) {
			t.Errorf("partition %d: md5 %X, want md5 of [%#x:%#x) %X", r.partition, got, r.start, r.end, want)
		}
	}
}

// GetPartitionMD5 for the MCP (device 5) hashes 32 KiB partitions 1-8, the
// 256-byte shadow area at 0x40000 as partition 9 and the whole 0x40100
// image as partition 0. A byteswapped MCP dump is hashed in natural byte
// order, so it matches the md5 of the same image stored unswapped.
func TestGetPartitionMD5MCP(t *testing.T) {
	natural := patterned(0x40100)
	// natural-order MCP image starts 00 08 20 00; a byteswapped dump of it
	// starts 08 00 00 20, which is what MCPSwapped looks for
	copy(natural, []byte{0x00, 0x08, 0x20, 0x00})
	swapped := swapPairs(natural)

	ranges := []struct {
		partition  int
		start, end int
	}{
		{0, 0x00000, 0x40100},
		{1, 0x00000, 0x08000},
		{2, 0x08000, 0x10000},
		{3, 0x10000, 0x18000},
		{4, 0x18000, 0x20000},
		{5, 0x20000, 0x28000},
		{6, 0x28000, 0x30000},
		{7, 0x30000, 0x38000},
		{8, 0x38000, 0x40000},
		{9, 0x40000, 0x40100},
	}
	for _, r := range ranges {
		want := md5Of(natural[r.start:r.end])
		if got := GetPartitionMD5(natural, 5, r.partition); !bytes.Equal(got, want) {
			t.Errorf("natural partition %d: md5 %X, want md5 of [%#x:%#x) %X", r.partition, got, r.start, r.end, want)
		}
		if got := GetPartitionMD5(swapped, 5, r.partition); !bytes.Equal(got, want) {
			t.Errorf("byteswapped partition %d: md5 %X, want natural-order md5 %X", r.partition, got, want)
		}
	}
}

// GetPartitionMD5 returns sixteen zero bytes for devices other than the
// T8 main flash (6) and the MCP (5).
func TestGetPartitionMD5UnknownDevice(t *testing.T) {
	bin := patterned(0x100000)
	for _, dev := range []byte{0, 4, 7, 0xFF} {
		if got := GetPartitionMD5(bin, dev, 1); !bytes.Equal(got, make([]byte, 16)) {
			t.Errorf("device %d: md5 %X, want 16 zero bytes", dev, got)
		}
	}
}

// GetPartitionMD5 returns sixteen zero bytes for partition numbers the
// device does not have, like TrionicCANLib's GetPartitionmd5 does, instead
// of hashing an arbitrary range.
func TestGetPartitionMD5OutOfRangePartition(t *testing.T) {
	bin := patterned(0x100000)
	for _, c := range []struct {
		device    byte
		partition int
	}{
		{6, 13}, {6, 20}, {6, -1},
		{5, 10}, {5, 12},
	} {
		if got := GetPartitionMD5(bin, c.device, c.partition); !bytes.Equal(got, make([]byte, 16)) {
			t.Errorf("device %d partition %d: md5 %X, want 16 zero bytes", c.device, c.partition, got)
		}
	}
}

// GetLasAddress reads the 24-bit big-endian header pointer at 0x20141 and
// adds 0x200 to cover the header region.
func TestGetLasAddress(t *testing.T) {
	for _, c := range []struct {
		ptr  [3]byte
		want uint32
	}{
		{[3]byte{0x00, 0x00, 0x00}, 0x000200},
		{[3]byte{0x0A, 0x12, 0x34}, 0x0A1434},
		{[3]byte{0x0F, 0xFE, 0x00}, 0x100000},
		{[3]byte{0xFF, 0xFF, 0xFF}, 0x10001FF},
	} {
		bin := make([]byte, 0x20200)
		copy(bin[0x20141:], c.ptr[:])
		if got := GetLasAddress(bin); got != c.want {
			t.Errorf("pointer %X: GetLasAddress = %#x, want %#x", c.ptr, got, c.want)
		}
	}
}

// MCPSwapped recognises a byteswapped MCP dump by its first word reading
// 08 00 00 20; anything else, including the natural 00 08 20 00, is not.
func TestMCPSwapped(t *testing.T) {
	for _, c := range []struct {
		head []byte
		want bool
	}{
		{[]byte{0x08, 0x00, 0x00, 0x20}, true},
		{[]byte{0x00, 0x08, 0x20, 0x00}, false},
		{[]byte{0x08, 0x00, 0x00, 0x21}, false},
		{[]byte{0x09, 0x00, 0x00, 0x20}, false},
		{[]byte{0xFF, 0xFF, 0xFF, 0xFF}, false},
	} {
		bin := append(append([]byte{}, c.head...), 0xAA, 0xBB)
		if got := MCPSwapped(bin); got != c.want {
			t.Errorf("MCPSwapped(% X) = %t, want %t", c.head, got, c.want)
		}
	}
}

// CodeByte XORs a byte with the key byte for its position in the 6-byte
// cycle, so coding twice gives the original byte back.
func TestCodeByte(t *testing.T) {
	for count, k := range codeKey {
		if got := CodeByte(0x00, count); got != k {
			t.Errorf("CodeByte(0x00, %d) = %#02x, want key byte %#02x", count, got, k)
		}
		for _, b := range []byte{0x01, 0x5A, 0xA5, 0xFF} {
			coded := CodeByte(b, count)
			if coded != b^k {
				t.Errorf("CodeByte(%#02x, %d) = %#02x, want %#02x", b, count, coded, b^k)
			}
			if back := CodeByte(coded, count); back != b {
				t.Errorf("CodeByte(CodeByte(%#02x, %d)) = %#02x, want the original", b, count, back)
			}
		}
	}
}

// GetCurrentBlock returns block n (bytes n*0x80..n*0x80+0x7F) coded with
// the XOR key restarting at the block's first byte, in a 0x88-byte buffer
// whose last 8 bytes are left zero for the checksum.
func TestGetCurrentBlock(t *testing.T) {
	bin := patterned(0x400)
	for _, block := range []int{0, 1, 5, 7} {
		got := GetCurrentBlock(bin, block, false)
		if len(got) != 0x88 {
			t.Fatalf("block %d: len %d, want 0x88", block, len(got))
		}
		for i := 0; i < 0x80; i++ {
			if want := bin[block*0x80+i] ^ codeKey[i%6]; got[i] != want {
				t.Fatalf("block %d byte %#x = %#02x, want %#02x", block, i, got[i], want)
			}
		}
		if !bytes.Equal(got[0x80:], make([]byte, 8)) {
			t.Errorf("block %d trailer = % X, want zeros", block, got[0x80:])
		}
	}

	// an erased block codes to the key bytes inverted, repeating every 6
	ff := bytes.Repeat([]byte{0xFF}, 0x100)
	got := GetCurrentBlock(ff, 1, false)
	if want := []byte{0xC6, 0x97, 0x88, 0x92, 0xB8, 0xC6, 0xC6, 0x97}; !bytes.Equal(got[:8], want) {
		t.Errorf("erased block starts % X, want % X", got[:8], want)
	}
}

// GetCurrentBlock with byteswapped set swaps each 16-bit word back to
// natural order before coding, so a byteswapped image yields exactly the
// blocks of the natural image.
func TestGetCurrentBlockByteswapped(t *testing.T) {
	natural := patterned(0x200)
	swapped := swapPairs(natural)
	for block := 0; block < 4; block++ {
		got := GetCurrentBlock(swapped, block, true)
		want := GetCurrentBlock(natural, block, false)
		if !bytes.Equal(got, want) {
			t.Errorf("block %d: byteswapped % X..., want % X...", block, got[:8], want[:8])
		}
	}
}

// FFblock reports whether size bytes from address are all 0xFF; a range
// running off the end of the image counts as not erased.
func TestFFblock(t *testing.T) {
	img := bytes.Repeat([]byte{0xFF}, 0x200)
	img[0x150] = 0xFE

	for _, c := range []struct {
		name    string
		address int
		size    byte
		want    bool
	}{
		{"erased block", 0x000, 0x80, true},
		{"erased block ending at the image end", 0x180, 0x80, true},
		{"one programmed byte", 0x100, 0x80, false},
		{"programmed byte is the last one checked", 0x0D1, 0x80, false},
		{"programmed byte just past the range", 0x0D0, 0x80, true},
		{"range runs past the image end", 0x1C0, 0x80, false},
		{"starts at the image end", 0x200, 0x80, false},
	} {
		if got := FFblock(img, c.address, c.size); got != c.want {
			t.Errorf("%s: FFblock(%#x, %#x) = %t, want %t", c.name, c.address, c.size, got, c.want)
		}
	}
}

// Erasedregion maps an address to its 0-based partition (the T8 partition
// table for device 6, 32 KiB slots for the MCP) and reports whether that
// partition's bit is set in the format mask.
func TestErasedregion(t *testing.T) {
	cases := []struct {
		device  byte
		address int
		part    int
	}{
		{6, 0x00000, 0},
		{6, 0x03FFF, 0},
		{6, 0x04000, 1},
		{6, 0x05FFF, 1},
		{6, 0x06000, 2},
		{6, 0x07FFF, 2},
		{6, 0x08000, 3},
		{6, 0x1FFFF, 3},
		{6, 0x20000, 4},
		{6, 0x3FFFF, 4},
		{6, 0x40000, 5},
		{6, 0x60000, 6},
		{6, 0x80000, 7},
		{6, 0xBFFFF, 7},
		{6, 0xC0000, 8},
		{6, 0xFFF80, 8},
		{5, 0x00000, 0},
		{5, 0x07FFF, 0},
		{5, 0x08000, 1},
		{5, 0x10000, 2},
		{5, 0x38000, 7},
		{5, 0x40000, 8}, // MCP shadow area
		{5, 0x40080, 8},
		{1, 0x80000, 0}, // unknown device: always partition 0
	}
	for _, c := range cases {
		only := uint64(1) << c.part
		if !Erasedregion(c.address, c.device, only) {
			t.Errorf("device %d address %#x: mask %#x (partition %d) not selected", c.device, c.address, only, c.part)
		}
		if Erasedregion(c.address, c.device, ^only) {
			t.Errorf("device %d address %#x: selected by mask %#x lacking partition %d", c.device, c.address, ^only, c.part)
		}
	}
	if Erasedregion(0x20000, 6, 0) {
		t.Error("an empty format mask selected a region")
	}
}

// getUlong assembles count bytes starting at startIndex as a little-endian
// integer.
func TestGetUlong(t *testing.T) {
	arr := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
	for _, c := range []struct {
		start, count int
		want         uint64
	}{
		{0, 0, 0},
		{0, 1, 0x01},
		{0, 2, 0x0201},
		{2, 4, 0x06050403},
		{1, 8, 0x0908070605040302},
	} {
		if got := getUlong(arr, c.start, c.count); got != c.want {
			t.Errorf("getUlong(%d, %d) = %#x, want %#x", c.start, c.count, got, c.want)
		}
	}
}

// GetFrameCmd packs a consecutive frame as an integer whose little-endian
// bytes are the frame number followed by 7 data bytes from startIndex.
// Vectors come from TrionicCANLib's BitTools.GetFrameBytes run under mono.
func TestGetFrameCmd(t *testing.T) {
	arr := make([]byte, 16)
	for i := range arr {
		arr[i] = byte(0x10 + i)
	}
	for _, c := range []struct {
		frameNo, start int
		want           uint64
		wire           []byte
	}{
		{0x21, 0, 0x1615141312111021, []byte{0x21, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16}},
		{0x2F, 7, 0x1D1C1B1A1918172F, []byte{0x2F, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D}},
		// only the low byte of the frame number is used: bit 8 must not
		// leak into the even first data byte
		{0x1FF, 2, 0x18171615141312FF, []byte{0xFF, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}},
	} {
		got := GetFrameCmd(c.frameNo, arr, c.start)
		if got != c.want {
			t.Errorf("GetFrameCmd(%#x, %d) = %#016x, want %#016x", c.frameNo, c.start, got, c.want)
		}
		wire := binary.LittleEndian.AppendUint64(nil, got)
		if !bytes.Equal(wire, c.wire) {
			t.Errorf("GetFrameCmd(%#x, %d) on the wire = % X, want % X", c.frameNo, c.start, wire, c.wire)
		}
	}
}

// GetVirginNVDM returns a fresh 1 MiB image erased to 0xFF except for the
// two NVDM partitions (0x4000-0x7FFF), which hold the virgin NVDM that
// starts with the "MFS*" file system magic.
func TestGetVirginNVDM(t *testing.T) {
	img := GetVirginNVDM()
	if uint64(len(img)) != T8binSize || T8binSize != 0x100000 {
		t.Fatalf("len = %#x (T8binSize %#x), want 0x100000", len(img), T8binSize)
	}
	if len(nvdmBytes) != 0x4000 {
		t.Fatalf("virgin NVDM is %#x bytes, want 0x4000 (partitions 2 and 3)", len(nvdmBytes))
	}
	if !bytes.Equal(img[0x4000:0x8000], nvdmBytes) {
		t.Error("NVDM partitions do not hold the virgin NVDM")
	}
	if !bytes.HasPrefix(img[0x4000:], []byte("MFS*")) {
		t.Errorf("NVDM starts % X, want MFS* magic", img[0x4000:0x4004])
	}
	ff := func(b []byte) bool { return bytes.Count(b, []byte{0xFF}) == len(b) }
	if !ff(img[:0x4000]) {
		t.Error("boot partition is not erased")
	}
	if !ff(img[0x8000:]) {
		t.Error("image past the NVDM is not erased")
	}

	img[0x4000] = 0x00
	if again := GetVirginNVDM(); again[0x4000] != 'M' {
		t.Error("modifying one image changed the next one returned")
	}
}
