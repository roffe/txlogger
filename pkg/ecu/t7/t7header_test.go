package t7

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// FileHeaderField reads its data as big-endian integers, a byte or a
// five-byte date, and SetInt/intBytes/smallintBytes write the same order.
func TestFileHeaderFieldAccessors(t *testing.T) {
	var f FileHeaderField
	f.SetInt(0x0007FFFF)
	if !bytes.Equal(f.Data, []byte{0x00, 0x07, 0xFF, 0xFF}) || f.Int() != 0x0007FFFF {
		t.Fatalf("SetInt stored % X, Int() = %#x", f.Data, f.Int())
	}
	if !bytes.Equal(intBytes(0x12345678), []byte{0x12, 0x34, 0x56, 0x78}) {
		t.Fatalf("intBytes = % X", intBytes(0x12345678))
	}
	if !bytes.Equal(smallintBytes(0x2356), []byte{0x23, 0x56}) {
		t.Fatalf("smallintBytes = % X", smallintBytes(0x2356))
	}

	f = FileHeaderField{ID: 0xF8, Length: 2, Data: []byte{0x81, 0x42}}
	if f.SmallInt() != 0x8142 || f.Byte() != 0x81 {
		t.Fatalf("SmallInt = %#x, Byte = %#x", f.SmallInt(), f.Byte())
	}

	f = FileHeaderField{ID: 0xFA, Length: 5, Data: []byte{0x42, 0xFB, 0xFA, 0xFF, 0xFF}}
	if f.Date() != [5]byte{0x42, 0xFB, 0xFA, 0xFF, 0xFF} {
		t.Fatalf("Date = % X", f.Date())
	}
	f.Data = []byte{0x01, 0x02}
	if f.Date() != [5]byte{0x01, 0x02} {
		t.Fatalf("short Date = % X, want zero fill", f.Date())
	}

	f = FileHeaderField{ID: 0x90, Length: 5}
	f.SetString("abc")
	if f.String() != "abc" {
		t.Fatalf("String = %q", f.String())
	}
	if got, want := f.Pretty(), `ID: 90, Length: 5, Data: "abc"`; got != want {
		t.Fatalf("Pretty = %q, want %q", got, want)
	}
}

// SetString refuses a value longer than the field and SetVin one longer
// than a VIN.
func TestFileHeaderLengthLimits(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}
	f := FileHeaderField{ID: 0x90, Length: 2}
	mustPanic("SetString of 3 bytes into 2", func() { f.SetString("abc") })

	var fh FileHeader
	fh.SetVin("YS3DD58N512345678")
	if fh.GetVin() != "YS3DD58N512345678" {
		t.Fatalf("GetVin = %q", fh.GetVin())
	}
	mustPanic("SetVin of 18 chars", func() { fh.SetVin("YS3DD58N5123456789") })
}

// reverse reverses in place and returns its argument.
func TestReverse(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"a", "a"},
		{"ab", "ba"},
		{"C10FC0UE", "EU0CF01C"},
	} {
		b := []byte(tt.in)
		if got := reverse(b); string(got) != tt.want || string(b) != tt.want {
			t.Errorf("reverse(%q) = %q", tt.in, got)
		}
	}
}

// fullPI is a PI area carrying every field FileHeader knows.
func fullPI() []piField {
	return []piField{
		{0xFE, be32(0x6B5E0)},
		{0xFD, be32(0)},
		{0xFC, be32(0x7FFFF)},
		{0xFB, be32(0x12345678)},
		{0xFA, []byte{0x42, 0xFB, 0xFA, 0xFF, 0xFF}},
		{0xF9, []byte("1")},
		{0xF8, []byte{0x11, 0x22}},
		{0xF7, []byte{0x33, 0x44}},
		{0xF6, []byte{0x55, 0x66}},
		{0xF5, []byte{0x77, 0x88}},
		{0xF2, be32(0xCAFEBABE)},
		{0x9C, be32(0x00F00000)},
		{0x9B, be32(0x0005A000)},
		{0x9A, []byte("0748")},
		{0x99, []byte("260809")},
		{0x98, []byte("txlogger EOL")},
		{0x97, []byte("B235R 2.3T")},
		{0x95, []byte("EU0AF01C.55P")},
		{0x94, []byte("5387998")},
		{0x93, []byte("1234567")},
		{0x92, []byte("123456789012345")},
		{0x91, []byte("5382019")},
		{0x90, []byte("YS3DD58N512345678")},
	}
}

// writeBinFile writes a 512 KB 0xFF image with the given PI area to a temp
// file and returns its path.
func writeBinFile(t *testing.T, fields []piField) string {
	t.Helper()
	bin := bytes.Repeat([]byte{0xFF}, 0x80000)
	writePI(bin, fields)
	p := filepath.Join(t.TempDir(), "t7.bin")
	if err := os.WriteFile(p, bin, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// piIDs lists the PI-area ids from the top down.
func piIDs(bin []byte) []byte {
	var ids []byte
	for a := len(bin) - 1; a > 0 && bin[a] != 0xFF && bin[a] != 0x00; a -= 2 + int(bin[a]) {
		ids = append(ids, bin[a-1])
	}
	return ids
}

// NewFileHeader parses every known PI field into the header, decoding the
// integer fields big-endian, and leaves a consistent footer untouched.
func TestNewFileHeaderParses(t *testing.T) {
	p := writeBinFile(t, fullPI())
	before, _ := os.ReadFile(p)

	fh, err := NewFileHeader(p, true)
	if err != nil {
		t.Fatal(err)
	}
	want := FileHeader{
		chassisIDCounter:            1,
		chassisIDDetected:           true,
		immoCodeDetected:            true,
		symbolTableMarkerDetected:   true,
		symbolTableChecksumDetected: true,
		f2ChecksumDetected:          true,
		chassisID:                   "YS3DD58N512345678",
		immobilizerID:               "123456789012345",
		romChecksumType:             0,
		bottomOfFlash:               0x7FFFF,
		romChecksumError:            '1',
		valueF5:                     0x7788,
		valueF6:                     0x5566,
		valueF7:                     0x3344,
		valueF8:                     0x1122,
		value9C:                     0x00F00000,
		symbolTableAddress:          0x0005A000,
		vehicleIDnr:                 "5382019",
		dateModified:                "0748",
		lastModifiedBy:              [5]byte{0x42, 0xFB, 0xFA, 0xFF, 0xFF},
		testSerialNr:                "260809",
		engineType:                  "txlogger EOL",
		ecuHardwareNr:               "1234567",
		softwareVersion:             "EU0AF01C.55P",
		carDescription:              "B235R 2.3T",
		partNumber:                  "5387998",
		checksumF2:                  0xCAFEBABE,
		checksumFB:                  0x12345678,
		fwLength:                    0x6B5E0,
	}
	if *fh != want {
		t.Fatalf("parsed\n%+v\nwant\n%+v", *fh, want)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("a consistent footer was rewritten")
	}
}

// A footer carrying the VIN twice is rebuilt (with auto-fix on) in T7Suite's
// field order with a single VIN, the one appended last, every other field
// kept, and the rest of the PI window erased.
func TestNewFileHeaderRebuildsDuplicateVIN(t *testing.T) {
	fields := append(fullPI(), piField{0x90, []byte("YS3DD58N598765432")})
	p := writeBinFile(t, fields)

	if _, err := NewFileHeader(p, true); err != nil {
		t.Fatal(err)
	}
	bin, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []byte{0x91, 0x94, 0x95, 0x97, 0x9A, 0x9C, 0x9B, 0xF2, 0xFB, 0xFC, 0xFD, 0xFE, 0xFA, 0x92, 0x93, 0xF8, 0xF7, 0xF6, 0xF5, 0x90, 0x99, 0x98, 0xF9}
	if got := piIDs(bin); !slices.Equal(got, wantIDs) {
		t.Fatalf("rebuilt footer ids % X, want % X", got, wantIDs)
	}
	for _, f := range fullPI() {
		want := string(f.data)
		switch f.id {
		case 0x90:
			want = "YS3DD58N598765432"
		case 0xF9:
			continue // see TestNewFileHeaderKeepsEOLFlag
		}
		if got, err := GetHeaderField(bin, f.id); err != nil || got != want {
			t.Errorf("field 0x%02X = %q, %v; want %q", f.id, got, err, want)
		}
	}
	footer := 0
	for _, f := range fullPI() {
		footer += 2 + len(f.data)
	}
	for i := 0x7FE00; i < len(bin)-footer; i++ {
		if bin[i] != 0xFF {
			t.Fatalf("stale PI byte %02X at 0x%X below the rebuilt footer", bin[i], i)
		}
	}
}

// A footer without the immobiliser code is rebuilt with the zero default
// for it; with auto-fix off the file is left alone.
func TestNewFileHeaderMissingImmo(t *testing.T) {
	var fields []piField
	for _, f := range fullPI() {
		if f.id != 0x92 {
			fields = append(fields, f)
		}
	}

	p := writeBinFile(t, fields)
	before, _ := os.ReadFile(p)
	if _, err := NewFileHeader(p, false); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(p); !bytes.Equal(before, after) {
		t.Fatal("footer rewritten with auto-fix off")
	}

	fh, err := NewFileHeader(p, true)
	if err != nil {
		t.Fatal(err)
	}
	if fh.immoCodeDetected {
		t.Fatal("immo code reported as detected")
	}
	bin, _ := os.ReadFile(p)
	if got, err := GetHeaderField(bin, 0x92); err != nil || got != "000000000000000" {
		t.Fatalf("rebuilt 0x92 = %q, %v; want the zero default", got, err)
	}
}

// Rebuilding the footer keeps the EOL-success flag 0xF9: the ECU sets its
// MIL (P0605) and stops saving adaption when the flag is not '1'.
func TestNewFileHeaderKeepsEOLFlag(t *testing.T) {
	fields := append(fullPI(), piField{0x90, []byte("YS3DD58N598765432")})
	p := writeBinFile(t, fields)
	if _, err := NewFileHeader(p, true); err != nil {
		t.Fatal(err)
	}
	bin, _ := os.ReadFile(p)
	if got, err := GetHeaderField(bin, 0xF9); err != nil || got != "1" {
		t.Fatalf("rebuilt 0xF9 = %q, %v; want \"1\"", got, err)
	}
}

// PI ids FileHeader does not model, such as the OBD-II history fields
// 0xF3/0xF4 some firmware branches carry, are skipped like T7Suite does.
func TestNewFileHeaderUnknownField(t *testing.T) {
	fields := append(fullPI(), piField{0xF4, make([]byte, 56)})
	p := writeBinFile(t, fields)

	fh, err := func() (fh *FileHeader, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return NewFileHeader(p, false)
	}()
	if err != nil {
		t.Fatal(err)
	}
	if fh.GetVin() != "YS3DD58N512345678" {
		t.Fatalf("VIN = %q", fh.GetVin())
	}
}

// An empty file has no footer to read.
func TestNewFileHeaderEmptyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileHeader(p, false); err == nil {
		t.Fatal("empty file parsed")
	}
}

// ReadField returns the 0xFF id as the end of the footer.
func TestReadFieldTerminator(t *testing.T) {
	p := writeBinFile(t, []piField{{0x95, []byte("AB")}})
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Seek(-1, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	first, _ := ReadField(f)
	if first.ID != 0x95 || first.Length != 2 || string(first.Data) != "AB" {
		t.Fatalf("first field %s", first.Pretty())
	}
	end, _ := ReadField(f)
	if end.ID != 0xFF || end.Length != 0 {
		t.Fatalf("terminator read as %s", end.Pretty())
	}
}

// NewFileHeader returns the open error for a file that isn't there.
func TestNewFileHeaderMissingFile(t *testing.T) {
	_, err := NewFileHeader(filepath.Join(t.TempDir(), "nope.bin"), true)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

// failWriter is a seekable footer target whose writes fail.
type failWriter struct{ *bytes.Reader }

var errWrite = errors.New("write failed")

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

// A failing write while clearing or rewriting the footer comes back as an
// error.
func TestFooterRewriteWriteError(t *testing.T) {
	rws := failWriter{bytes.NewReader(make([]byte, 0x80000))}
	fh := &FileHeader{}
	if err := fh.clearFooter(rws); !errors.Is(err, errWrite) {
		t.Errorf("clearFooter err = %v, want %v", err, errWrite)
	}
	if err := fh.createNewFooter(rws, true, true, true); !errors.Is(err, errWrite) {
		t.Errorf("createNewFooter err = %v, want %v", err, errWrite)
	}
}
