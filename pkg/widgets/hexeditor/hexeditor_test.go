package hexeditor

import (
	"bytes"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestLineFormat(t *testing.T) {
	data := make([]byte, 20)
	copy(data, "ABCDEFGHIJKLMNOP")
	data[16], data[17], data[18], data[19] = 0x00, 0xFF, 0x41, 0x7F
	he := &HexEditor{cfg: Config{Data: data}, bpr: 16}

	got := he.line(0)
	want := "00000000  41 42 43 44 45 46 47 48  49 4A 4B 4C 4D 4E 4F 50 |ABCDEFGHIJKLMNOP|"
	if got != want {
		t.Errorf("line(0)\n got %q\nwant %q", got, want)
	}
	if len(got) != lineCharsFor(16) {
		t.Errorf("line length = %d, want %d", len(got), lineCharsFor(16))
	}
	// partial last row pads hex and truncates ascii
	got = he.line(1)
	as := asciiStartFor(16)
	if !bytes.HasPrefix([]byte(got), []byte("00000010  00 FF 41 7F ")) {
		t.Errorf("line(1) prefix wrong: %q", got)
	}
	if got[as-1] != '|' || got[as:as+4] != "..A." || got[as+4] != '|' {
		t.Errorf("line(1) ascii section wrong: %q", got)
	}
	if he.line(2) != "" {
		t.Error("line past end should be empty")
	}

	// 8 wide: no mid-row gap
	he.bpr = 8
	if he.line(0) != "00000000  41 42 43 44 45 46 47 48 |ABCDEFGH|" {
		t.Errorf("bpr=8 line(0) = %q", he.line(0))
	}
	// 32 wide: gaps after bytes 8, 16 and 24 (needs a full row of data,
	// partial rows truncate the ascii column)
	he.bpr = 32
	he.cfg.Data = make([]byte, 32)
	l := he.line(0)
	if len(l) != lineCharsFor(32) {
		t.Errorf("bpr=32 line length = %d, want %d", len(l), lineCharsFor(32))
	}
	if l[asciiStartFor(32)-1] != '|' {
		t.Errorf("bpr=32 pipe misplaced: %q", l)
	}
}

func TestLayoutMath(t *testing.T) {
	if hexX(0) != 10 || hexX(7) != 31 || hexX(8) != 35 || hexX(15) != 56 {
		t.Errorf("hexX positions wrong: %d %d %d %d", hexX(0), hexX(7), hexX(8), hexX(15))
	}
	if asciiStartFor(16) != 60 || lineCharsFor(16) != 77 {
		t.Errorf("bpr=16 layout: asciiStart=%d lineChars=%d", asciiStartFor(16), lineCharsFor(16))
	}
	if asciiStartFor(8) != 35 || asciiStartFor(32) != 110 {
		t.Errorf("asciiStartFor: 8→%d 32→%d", asciiStartFor(8), asciiStartFor(32))
	}
	// tapCol must invert hexX and the ascii layout for every width and byte
	for _, bpr := range []int{8, 16, 24, 32} {
		for i := range bpr {
			for _, ci := range []int{hexX(i), hexX(i) + 1, hexX(i) + 2} { // pair + trailing space
				col, ascii, ok := tapCol(ci, bpr)
				if !ok || ascii || col != i {
					t.Fatalf("tapCol(%d, bpr=%d) = %d,%v,%v want %d,false,true", ci, bpr, col, ascii, ok, i)
				}
			}
			col, ascii, ok := tapCol(asciiStartFor(bpr)+i, bpr)
			if !ok || !ascii || col != i {
				t.Fatalf("tapCol ascii(%d, bpr=%d) = %d,%v,%v want %d,true,true", asciiStartFor(bpr)+i, bpr, col, ascii, ok, i)
			}
		}
	}
	if _, _, ok := tapCol(3, 16); ok {
		t.Error("tap in offset column should not hit")
	}
}

func TestNibbleEditAndUndo(t *testing.T) {
	test.NewApp()
	data := []byte{0x00, 0x11, 0x22}
	var edits []int
	he := New(Config{Data: data, OnEdit: func(off int, b byte) { edits = append(edits, off) }})

	he.TypedRune('a') // high nibble
	if data[0] != 0xA0 || he.cursor != 0 {
		t.Fatalf("after high nibble: data[0]=%02X cursor=%d", data[0], he.cursor)
	}
	he.TypedRune('5') // low nibble, advances
	if data[0] != 0xA5 || he.cursor != 1 || he.nibble != 0 {
		t.Fatalf("after low nibble: data[0]=%02X cursor=%d nibble=%d", data[0], he.cursor, he.nibble)
	}
	he.TypedRune('F')
	if data[1] != 0xF1 {
		t.Fatalf("data[1]=%02X, want F1", data[1])
	}
	he.undoOne() // reverts byte 1
	he.undoOne() // reverts byte 0
	if data[0] != 0x00 || data[1] != 0x11 {
		t.Fatalf("after undo: % X", data)
	}
	if len(edits) != 5 { // 3 edits + 2 undos
		t.Fatalf("OnEdit called %d times, want 5", len(edits))
	}
}

func TestASCIIEdit(t *testing.T) {
	test.NewApp()
	data := []byte{'x', 'y', 'z'}
	he := New(Config{Data: data})
	he.asciiMode = true

	he.TypedRune('A') // writes the byte and advances
	he.TypedRune('B')
	if data[0] != 'A' || data[1] != 'B' || he.cursor != 2 {
		t.Fatalf("ascii edit: % X cursor=%d", data, he.cursor)
	}
	he.TypedRune('\t') // non-printable is ignored
	if data[2] != 'z' {
		t.Fatalf("non-printable should not edit: %02X", data[2])
	}
	he.undoOne()
	he.undoOne()
	if !bytes.Equal(data, []byte("xyz")) {
		t.Fatalf("after undo: %q", data)
	}
}

func TestSetBytesPerRow(t *testing.T) {
	test.NewApp()
	data := make([]byte, 100)
	he := New(Config{Data: data})
	he.setCursor(50)
	he.setBytesPerRow(8)
	if he.bpr != 8 || he.cursor != 50 {
		t.Fatalf("bpr=%d cursor=%d", he.bpr, he.cursor)
	}
	if he.rowMin.Width >= fyneMeasureWidth(t, 16) {
		t.Error("8-wide rows should be narrower than 16-wide rows")
	}
}

func fyneMeasureWidth(t *testing.T, bpr int) float32 {
	t.Helper()
	he := &HexEditor{bpr: bpr}
	he.recalcRowMin()
	return he.rowMin.Width
}

func TestParsePattern(t *testing.T) {
	if !bytes.Equal(parsePattern("DE AD be ef"), []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Error("hex pattern not parsed")
	}
	if !bytes.Equal(parsePattern("YS3F"), []byte("YS3F")) {
		t.Error("non-hex input should be literal text")
	}
	if parsePattern("  ") != nil {
		t.Error("blank input should be nil")
	}
}
