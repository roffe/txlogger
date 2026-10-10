package t8file

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// isValidT8Bin accepts only full 1 MiB images whose second longword is the
// T8 reset vector.
func TestIsValidT8Bin(t *testing.T) {
	wrongVector := blankBin()
	wrongVector[7] = 0x00
	short := blankBin()[:0x100000-1]
	for _, c := range []struct {
		name string
		data []byte
		want bool
	}{
		{"valid", blankBin(), true},
		{"wrong reset vector", wrongVector, false},
		{"short", short, false},
		{"nil", nil, false},
	} {
		if got := isValidT8Bin(c.data); got != c.want {
			t.Errorf("%s: isValidT8Bin = %v, want %v", c.name, got, c.want)
		}
	}
}

// GetChecksumAreaOffsetFromBytes reads the big-endian pointer at 0x20140 and
// returns 0 when the data does not reach it.
func TestGetChecksumAreaOffsetFromBytes(t *testing.T) {
	data := make([]byte, 0x20144)
	binary.BigEndian.PutUint32(data[0x20140:], 0x000BAF06)
	if got := GetChecksumAreaOffsetFromBytes(data); got != 0x000BAF06 {
		t.Errorf("offset = %#x, want 0xbaf06", got)
	}
	if got := GetChecksumAreaOffsetFromBytes(data[:0x20143]); got != 0 {
		t.Errorf("offset of short data = %#x, want 0", got)
	}
}

// GetEmptySpaceStartFrom returns the start of the first FF FF FF run at or
// after offset, searching at most 0x1000 bytes, and the end of that window
// when there is none.
func TestGetEmptySpaceStartFrom(t *testing.T) {
	withRun := func(n, at int) []byte {
		d := make([]byte, n)
		copy(d[at:], []byte{0xFF, 0xFF, 0xFF})
		return d
	}
	partial := make([]byte, 0x40)
	copy(partial[4:], []byte{0xFF, 0x00, 0xFF, 0xFF, 0x00, 0xFF, 0xFF, 0xFF})

	for _, c := range []struct {
		name   string
		data   []byte
		offset int
		want   int
	}{
		{"empty", nil, 0, 0},
		{"offset at end", make([]byte, 16), 16, 0},
		{"run found", withRun(0x100, 0x20), 0, 0x20},
		{"run before offset ignored", withRun(0x2000, 0x10), 0x20, 0x1020},
		{"single and double FF skipped", partial, 0, 9},
		{"run past window", withRun(0x3000, 0x1800), 0x100, 0x1100},
		{"run at the very end", withRun(0x10, 0x0D), 0, 0x0D},
		{"no run, window clamped to data", make([]byte, 0x20), 0, 0x1E},
	} {
		if got := GetEmptySpaceStartFrom(c.data, c.offset); got != c.want {
			t.Errorf("%s: GetEmptySpaceStartFrom = %#x, want %#x", c.name, got, c.want)
		}
	}
}

// readDataFromBytes returns length bytes from address, wrapping the address
// around the data and zero filling what lies past its end.
func TestReadDataFromBytes(t *testing.T) {
	data := make([]byte, 16)
	for i := range data {
		data[i] = byte(i)
	}
	for _, c := range []struct {
		name          string
		data          []byte
		address, size int
		want          []byte
	}{
		{"plain", data, 2, 3, []byte{2, 3, 4}},
		{"address wraps", data, 20, 4, []byte{4, 5, 6, 7}},
		{"past end zero filled", data, 14, 4, []byte{14, 15, 0, 0}},
		{"no data", nil, 5, 3, []byte{0, 0, 0}},
	} {
		if got := readDataFromBytes(c.data, c.address, c.size); !bytes.Equal(got, c.want) {
			t.Errorf("%s: got % X, want % X", c.name, got, c.want)
		}
	}
}

// writeBin stores data as a file in a fresh temp dir and returns its path.
func writeBin(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t8.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// GetInfo decodes both the PI area and the NVDM of a valid T8 file.
func TestGetInfo(t *testing.T) {
	tf := new(T8File)
	th := tf.GetInfo(writeBin(t, testBin()))
	if th.SoftwareVersion() != "FA5I_C_FME2_65_FIEF_82h       " {
		t.Errorf("SoftwareVersion = %q", th.SoftwareVersion())
	}
	if th.VIN() != blockC.vin || th.PartNumber() != "55353231" || len(th.FlashBlocks()) != 3 {
		t.Errorf("VIN %q part %q blocks %d, want %q %q 3", th.VIN(), th.PartNumber(), len(th.FlashBlocks()), blockC.vin, "55353231")
	}
}

// GetInfo returns an empty header for files that are not T8 images, and
// for files that cannot be read.
func TestGetInfoNotT8(t *testing.T) {
	t7 := make([]byte, 0x80000)
	for _, path := range []string{
		writeBin(t, t7),
		filepath.Join(t.TempDir(), "missing.bin"),
	} {
		tf := new(T8File)
		th := tf.GetInfo(path)
		if th == nil {
			t.Fatalf("%s: GetInfo returned nil", path)
		}
		if th.VIN() != "" || th.SoftwareVersion() != "" || len(th.FlashBlocks()) != 0 {
			t.Errorf("%s: header not empty: VIN %q sw %q blocks %d", path, th.VIN(), th.SoftwareVersion(), len(th.FlashBlocks()))
		}
	}
}

// SaveFlashBlock writes the header's immobilizer data into every flash block
// of the file and leaves every byte outside the blocks untouched.
func TestSaveFlashBlock(t *testing.T) {
	orig := testBin()
	path := writeBin(t, orig)
	tf := new(T8File)
	th := tf.GetInfo(path)
	th.SetVIN("YS3FB49F741999999")
	th.SetPIN("ZZ99")
	th.SetPSK("CAFEBABE0001")

	if err := tf.SaveFlashBlock(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != len(orig) {
		t.Fatalf("file size = %d, want %d", len(saved), len(orig))
	}

	inBlock := func(i int) bool {
		for _, a := range []int{0x42D8, 0x4408, 0x62D8} {
			if i >= a && i < a+0x130 {
				return true
			}
		}
		return false
	}
	for i := range orig {
		if !inBlock(i) && saved[i] != orig[i] {
			t.Fatalf("byte %#x outside the flash blocks changed: %02X -> %02X", i, orig[i], saved[i])
		}
	}

	for _, a := range []int{0x42D8, 0x4408, 0x62D8} {
		back := new(T8Header)
		(&FlashBlock{BlockData: saved[a : a+0x130]}).DecodeBlock(back)
		if back.VIN() != "YS3FB49F741999999" || back.PIN() != "ZZ99" || back.PSK() != "CAFEBABE0001" {
			t.Errorf("block %#x: VIN %q PIN %q PSK %q", a, back.VIN(), back.PIN(), back.PSK())
		}
		if back.ISK() != "B0B1B2B3B4B5" {
			t.Errorf("block %#x: ISK = %q, want the header's B0B1B2B3B4B5", a, back.ISK())
		}
	}

	again := new(T8File).GetInfo(path)
	if again.VIN() != "YS3FB49F741999999" || len(again.FlashBlocks()) != 3 {
		t.Errorf("reloaded VIN %q blocks %d", again.VIN(), len(again.FlashBlocks()))
	}
}

// SaveFlashBlock reports a file that cannot be opened for writing.
func TestSaveFlashBlockOpenError(t *testing.T) {
	tf := new(T8File)
	tf.GetInfo(writeBin(t, testBin()))
	tf.filePath = t.TempDir() // a directory cannot be opened read-write
	if err := tf.SaveFlashBlock(); err == nil {
		t.Fatal("SaveFlashBlock into a directory succeeded")
	}
}

// saveDataToBinary writes at most length bytes of data at address.
func TestSaveDataToBinaryTruncates(t *testing.T) {
	path := writeBin(t, make([]byte, 8))
	tf := &T8File{filePath: path}
	if err := tf.saveDataToBinary(2, 3, []byte{1, 2, 3, 4, 5}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := []byte{0, 0, 1, 2, 3, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("file = % X, want % X", got, want)
	}
}

// A stock image decodes to the identification it carries (values read from
// the image independently of this package: the NVDM info record's part and
// serial number override the PI area's part number 55559737), and every one
// of its flash blocks re-encodes byte for byte from its own decoded values.
// The image lives in the ecusymbol checkout next to this repository.
func TestStockImage(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "ecusymbol", "testdata", "T8Binaries", "55353231_FA5I_C_FME2_72_FIEF_80c.BIN")
	if _, err := os.Stat(path); err != nil {
		t.Skip("stock T8 binary not available")
	}
	th := new(T8File).GetInfo(path)
	for _, c := range []struct{ name, got, want string }{
		{"PartNumber", th.PartNumber(), "55353231"},
		{"SerialNumber", th.SerialNumber(), "863232AC0AE02JDE"},
		{"SoftwareVersion", th.SoftwareVersion(), "FA5I_C_FME2_72_FIEF_80c       "},
		{"HardwareID", th.HardwareID(), "GMPT 0100"},
		{"DeviceType", th.DeviceType(), "ECM                 "},
		{"ReleaseDate", th.ReleaseDate(), "2004-08-12 14:13:34"},
		{"ProgrammerName", th.ProgrammerName(), "Martin B Andersson"},
		{"ProgrammerDevice", th.ProgrammerDevice(), "SPSTool\x00"}, // the record is 8 bytes, NUL included
		{"EcuDescription", th.EcuDescription(), "Trionic 8 P6.8  "},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if len(th.FlashBlocks()) != 28 {
		t.Errorf("got %d flash blocks, want 28", len(th.FlashBlocks()))
	}
	for _, fb := range th.FlashBlocks() {
		own := new(T8Header)
		fb.DecodeBlock(own)
		if !bytes.Equal(fb.EncodeBlock(own), fb.BlockData) {
			t.Errorf("block %#x does not re-encode to itself", fb.BlockAddress)
		}
	}
}
