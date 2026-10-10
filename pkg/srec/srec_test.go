package srec

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
)

// sample holds one record of every supported type. Checksums were computed
// independently of this package; the S0/S1/S5/S9 lines are the Wikipedia
// SREC example.
var sample = []string{
	"S00F000068656C6C6F202020202000003C",
	"S11F00007C0802A6900100049421FFF07C6C1B787C8C23783C6000003863000026",
	"S11F001C4BFFFFE5398000007D83637880010014382100107C0803A64E800020E9",
	"S111003848656C6C6F20776F726C642E0A0042",
	"S2080123450102030484",
	"S30712345678AABB7F",
	"S5030003F9",
	"S70512345678E6",
	"S80401234592",
	"S9030000FC",
}

// parseNoPanic runs Parse and turns a panic into a test failure, so a
// malformed-input case reports instead of killing the test binary.
func parseNoPanic(t *testing.T, r io.Reader) (sr *Srec, err error) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("Parse panicked: %v", p)
		}
	}()
	sr = NewSrec()
	err = sr.Parse(r)
	return sr, err
}

func recordStrings(sr *Srec) []string {
	var out []string
	for _, r := range sr.Records {
		out = append(out, r.String())
	}
	return out
}

// Parse splits each line into type, length, address, data and checksum.
func TestParseFields(t *testing.T) {
	sr, err := parseNoPanic(t, strings.NewReader(strings.Join(sample, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.Records) != len(sample) {
		t.Fatalf("got %d records, want %d", len(sr.Records), len(sample))
	}
	tests := []struct {
		idx      int
		typ      string
		length   uint8
		addr     uint32
		data     []byte
		checksum uint8
	}{
		{0, "S0", 0x0F, 0x0000, []byte("hello     \x00\x00"), 0x3C},
		{3, "S1", 0x11, 0x0038, []byte("Hello world.\n\x00"), 0x42},
		{4, "S2", 0x08, 0x012345, []byte{1, 2, 3, 4}, 0x84},
		{5, "S3", 0x07, 0x12345678, []byte{0xAA, 0xBB}, 0x7F},
		{6, "S5", 0x03, 0, []byte{0x00, 0x03}, 0xF9},
		{7, "S7", 0x05, 0x12345678, []byte{}, 0xE6},
		{8, "S8", 0x04, 0x012345, []byte{}, 0x92},
		{9, "S9", 0x03, 0x0000, []byte{}, 0xFC},
	}
	for _, tt := range tests {
		r := sr.Records[tt.idx]
		if r.Srectype != tt.typ || r.Length != tt.length || r.Address != tt.addr || r.Checksum != tt.checksum {
			t.Errorf("record %d: got {%s %#x %#x %#x}, want {%s %#x %#x %#x}",
				tt.idx, r.Srectype, r.Length, r.Address, r.Checksum, tt.typ, tt.length, tt.addr, tt.checksum)
		}
		if !bytes.Equal(r.Data, tt.data) {
			t.Errorf("record %d data: got % X, want % X", tt.idx, r.Data, tt.data)
		}
	}
}

// Record.String re-encodes a parsed record to the exact input line, and
// Srec.String joins them one per line so the output parses back the same.
func TestParseStringRoundTrip(t *testing.T) {
	sr, err := parseNoPanic(t, strings.NewReader(strings.Join(sample, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := recordStrings(sr); !slices.Equal(got, sample) {
		t.Fatalf("round trip mismatch:\ngot  %q\nwant %q", got, sample)
	}
	want := strings.Join(sample, "\n") + "\n"
	if got := sr.String(); got != want {
		t.Fatalf("Srec.String:\ngot  %q\nwant %q", got, want)
	}
	again, err := parseNoPanic(t, strings.NewReader(sr.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := recordStrings(again); !slices.Equal(got, sample) {
		t.Fatalf("reparse mismatch: %q", got)
	}
}

// Parse accepts LF, CRLF and bare CR line endings, mixed, with or without a
// final terminator.
func TestParseLineEndings(t *testing.T) {
	tests := map[string]string{
		"LF":             strings.Join(sample, "\n") + "\n",
		"CRLF":           strings.Join(sample, "\r\n") + "\r\n",
		"CR":             strings.Join(sample, "\r") + "\r",
		"no terminator":  strings.Join(sample, "\n"),
		"mixed":          sample[0] + "\r\n" + sample[1] + "\n" + sample[2] + "\r" + strings.Join(sample[3:], "\r\n"),
		"LF then CR run": sample[0] + "\n" + strings.Join(sample[1:], "\r"),
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := parseNoPanic(t, strings.NewReader(in))
			if err != nil {
				t.Fatal(err)
			}
			if got := recordStrings(sr); !slices.Equal(got, sample) {
				t.Fatalf("got %q", got)
			}
		})
	}
}

// Parse handles a CRLF file however the reader chunks it, including reads
// that end between the CR and the LF. Short reads (pipes, sockets,
// decompressors) can split there; iotest.OneByteReader always does.
func TestParseCRLFSplitAcrossReads(t *testing.T) {
	in := strings.Join(sample, "\r\n") + "\r\n"
	sr, err := parseNoPanic(t, iotest.OneByteReader(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	if got := recordStrings(sr); !slices.Equal(got, sample) {
		t.Fatalf("got %q", got)
	}
}

// Parse returns an error for record types it does not handle (S4, S6) and for
// lines that are not S-records at all.
func TestParseRejectsUnknownTypes(t *testing.T) {
	for _, line := range []string{"S4030000FC", "S6040000010A", "X1030000FC", "s1030000FC"} {
		sr, err := parseNoPanic(t, strings.NewReader(sample[0]+"\n"+line+"\n"+sample[9]))
		if err == nil || !strings.Contains(err.Error(), "invalid srectype") {
			t.Errorf("%s: got err %v, want invalid srectype", line, err)
		}
		if len(sr.Records) != 1 {
			t.Errorf("%s: parsing should stop at the bad line, got %d records", line, len(sr.Records))
		}
	}
}

// Parse returns the strconv hex decoding error for a non-hex length, address,
// data or checksum field.
func TestParseRejectsBadHex(t *testing.T) {
	tests := map[string]string{
		"length":   "S1ZZ0038AA00",
		"address":  "S1040G38AA00",
		"data":     "S1040038AZ00",
		"checksum": "S1040038AAQQ",
	}
	for name, line := range tests {
		_, err := parseNoPanic(t, strings.NewReader(line))
		var numErr *strconv.NumError
		if !errors.As(err, &numErr) {
			t.Errorf("%s: %q gave %v, want a hex parse error", name, line, err)
		}
	}
}

// Parse keeps the checksum stored in the file even when it is wrong;
// checking it is left to the caller through CalcChecksum.
func TestParseKeepsWrongChecksum(t *testing.T) {
	sr, err := parseNoPanic(t, strings.NewReader("S111003848656C6C6F20776F726C642E0A0043"))
	if err != nil {
		t.Fatal(err)
	}
	r := sr.Records[0]
	calc, err := r.CalcChecksum()
	if err != nil {
		t.Fatal(err)
	}
	if r.Checksum != 0x43 || calc != 0x42 {
		t.Fatalf("stored %#x calculated %#x, want 0x43 and 0x42", r.Checksum, calc)
	}
}

// Parse returns an error for a line shorter than its own length field says,
// rather than slicing past the end of it.
func TestParseTruncatedLine(t *testing.T) {
	for _, line := range []string{"S", "S1", "S113", "S1130000", "S11300000102", "S111003848656C6C6F20776F726C642E0A00"} {
		t.Run(line, func(t *testing.T) {
			if _, err := parseNoPanic(t, strings.NewReader(line)); err == nil {
				t.Errorf("%q parsed without error", line)
			}
		})
	}
}

// Parse either skips a blank line or reports it, without losing the records
// around it or panicking.
func TestParseBlankLine(t *testing.T) {
	for name, in := range map[string]string{
		"blank line between": sample[3] + "\n\n" + sample[9],
		"trailing blank":     sample[3] + "\n" + sample[9] + "\n\n",
		"CRLF blank line":    sample[3] + "\r\n\r\n" + sample[9],
	} {
		t.Run(name, func(t *testing.T) {
			sr, err := parseNoPanic(t, strings.NewReader(in))
			if err != nil {
				return
			}
			if got, want := recordStrings(sr), []string{sample[3], sample[9]}; !slices.Equal(got, want) {
				t.Errorf("%q: got %q, want %q", in, got, want)
			}
		})
	}
}

// CalcChecksum is the ones' complement of the byte sum over length, address
// and data, so it reproduces every sample line's checksum.
func TestCalcChecksum(t *testing.T) {
	sr, err := parseNoPanic(t, strings.NewReader(strings.Join(sample, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range sr.Records {
		got, err := r.CalcChecksum()
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		if got != r.Checksum {
			t.Errorf("%s: calculated %#02x, stored %#02x", r, got, r.Checksum)
		}
	}
	for _, typ := range []string{"S4", "S6", "", "S10"} {
		if _, err := (&Record{Srectype: typ}).CalcChecksum(); err == nil {
			t.Errorf("CalcChecksum(%q) returned no error", typ)
		}
	}
}

// getAddrLen gives the address width in bytes for each handled type.
func TestGetAddrLen(t *testing.T) {
	want := map[string]int{"S0": 2, "S1": 2, "S2": 3, "S3": 4, "S5": 0, "S7": 4, "S8": 3, "S9": 2}
	for typ, n := range want {
		got, err := getAddrLen(typ)
		if err != nil || got != n {
			t.Errorf("getAddrLen(%s) = %d, %v; want %d", typ, got, err, n)
		}
	}
	for _, typ := range []string{"S4", "S6", "SX", ""} {
		if _, err := getAddrLen(typ); err == nil {
			t.Errorf("getAddrLen(%q) returned no error", typ)
		}
	}
}

// MakeRec fills in length and checksum, and widens S1/S2 data records to the
// smallest type whose address field holds the record's end address.
func TestMakeRec(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		addr uint32
		data []byte
		want string
	}{
		{"S1 example", "S1", 0x0038, []byte("Hello world.\n\x00"), "S111003848656C6C6F20776F726C642E0A0042"},
		{"S1 16 bytes", "S1", 0x0000, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, "S1130000000102030405060708090A0B0C0D0E0F74"},
		{"S1 ends at 0xFFFF", "S1", 0xFFFF, []byte{0x11}, "S104FFFF11EC"},
		{"S1 crosses 0xFFFF", "S1", 0xFFFF, []byte{0x11, 0x22}, "S20600FFFF1122C8"},
		{"S2 ends at 0xFFFFFF", "S2", 0xFFFFFF, []byte{0x11}, "S205FFFFFF11EC"},
		{"S2 crosses 0xFFFFFF", "S2", 0xFFFFFF, []byte{0x11, 0x22}, "S30700FFFFFF1122C8"},
		{"S1 crosses 0xFFFFFF", "S1", 0xFFFFFF, []byte{0x11, 0x22}, "S30700FFFFFF1122C8"},
		{"S2 is not narrowed", "S2", 0x10, []byte{0x11}, "S20500001011D9"},
		{"S3 is not narrowed", "S3", 0x10, []byte{0x11}, "S3060000001011D8"},
		{"S0 header", "S0", 0, []byte("hello     \x00\x00"), "S00F000068656C6C6F202020202000003C"},
		{"S9 start address", "S9", 0x5000, nil, "S9035000AC"},
		{"S7 start address", "S7", 0x12345678, nil, "S70512345678E6"},
		{"S8 start address", "S8", 0x012345, nil, "S80401234592"},
		{"S5 count", "S5", 0, []byte{0x00, 0x03}, "S5030003F9"},
	}
	for _, tt := range tests {
		r, err := MakeRec(tt.typ, tt.addr, tt.data)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := r.String(); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, got, tt.want)
		}
	}
}

// MakeRec returns an error for types it cannot encode.
func TestMakeRecRejectsUnknownType(t *testing.T) {
	for _, typ := range []string{"S4", "S6", "", "X1"} {
		if r, err := MakeRec(typ, 0, []byte{1}); err == nil {
			t.Errorf("MakeRec(%q) = %v, want error", typ, r)
		}
	}
}

// MakeRec returns an error when the data does not fit the one-byte length
// field (address + data + checksum <= 255 bytes) instead of wrapping it.
func TestMakeRecRejectsOversizeData(t *testing.T) {
	if r, err := MakeRec("S1", 0x1000, make([]byte, 252)); err != nil || r.Length != 0xFF {
		t.Fatalf("252 bytes: got %v, %v; want Length 0xFF", r, err)
	}
	if r, err := MakeRec("S1", 0x1000, make([]byte, 253)); err == nil {
		t.Fatalf("253 bytes: got record with Length %#x, want error", r.Length)
	}
}

// EndAddress is the address of the record's last data byte.
func TestRecordEndAddress(t *testing.T) {
	r := &Record{Address: 0x0038, Data: make([]byte, 14)}
	if got := r.EndAddress(); got != 0x0045 {
		t.Fatalf("got %#x, want 0x45", got)
	}
}

// EndAddr is the end address of the data record (S1/S2/S3) with the highest
// start address, whatever order the records are in; header and termination
// records do not count.
func TestSrecEndAddr(t *testing.T) {
	if got := NewSrec().EndAddr(); got != 0 {
		t.Errorf("empty: got %#x, want 0", got)
	}
	sr, err := parseNoPanic(t, strings.NewReader(strings.Join(sample[:4], "\n")+"\n"+sample[9]))
	if err != nil {
		t.Fatal(err)
	}
	if got := sr.EndAddr(); got != 0x0045 {
		t.Errorf("S1 records: got %#x, want 0x45", got)
	}

	mk := func(typ string, addr uint32, n int) *Record {
		r, err := MakeRec(typ, addr, make([]byte, n))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	sr = &Srec{Records: []*Record{
		mk("S0", 0, 4),
		mk("S3", 0x00100000, 8),
		mk("S2", 0x00020000, 16),
		mk("S1", 0x1000, 4),
		mk("S7", 0xFFFFFFF0, 0),
	}}
	if got := sr.EndAddr(); got != 0x00100007 {
		t.Errorf("mixed types: got %#x, want 0x100007", got)
	}
}

// EndAddr counts a data record that starts at address 0.
func TestSrecEndAddrRecordAtZero(t *testing.T) {
	r, err := MakeRec("S1", 0, make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	sr := &Srec{Records: []*Record{r}}
	if got := sr.EndAddr(); got != 0x0F {
		t.Fatalf("got %#x, want 0xf", got)
	}
}

// Record.String reports a record with an unknown type instead of encoding it.
func TestRecordStringInvalidType(t *testing.T) {
	r := &Record{Srectype: "S4", Length: 3, Data: []byte{1}}
	if got := r.String(); got != "<invalid srectype>" {
		t.Fatalf("got %q", got)
	}
}

// scanLinesCustom splits on LF, CRLF or a lone CR, returns an unterminated
// final line at EOF and asks for more data otherwise, also when the data
// ends in a CR.
func TestScanLinesCustom(t *testing.T) {
	tests := []struct {
		data    string
		atEOF   bool
		advance int
		token   string
		more    bool // expects (0, nil): request more data
	}{
		{"ab\ncd", false, 3, "ab", false},
		{"ab\r\ncd", false, 4, "ab", false},
		{"ab\rcd", false, 3, "ab", false},
		{"ab\n\rcd", false, 3, "ab", false},
		{"ab\r\rcd", false, 3, "ab", false},
		{"ab\r", false, 0, "", true}, // the LF of a CRLF may follow
		{"ab\r", true, 3, "ab", false},
		{"\ncd", false, 1, "", false},
		{"abcd", true, 4, "abcd", false},
		{"abcd", false, 0, "", true},
		{"", true, 0, "", true},
	}
	for _, tt := range tests {
		adv, tok, err := scanLinesCustom([]byte(tt.data), tt.atEOF)
		if err != nil {
			t.Fatalf("%q: %v", tt.data, err)
		}
		if tt.more {
			if adv != 0 || tok != nil {
				t.Errorf("%q atEOF=%v: got (%d, %q), want (0, nil)", tt.data, tt.atEOF, adv, tok)
			}
			continue
		}
		if adv != tt.advance || string(tok) != tt.token {
			t.Errorf("%q atEOF=%v: got (%d, %q), want (%d, %q)", tt.data, tt.atEOF, adv, tok, tt.advance, tt.token)
		}
	}
}
