package ecu

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/ecusymbol/lzhuf"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
)

// msgLog collects progress messages; safe to call from any goroutine.
type msgLog struct {
	mu   sync.Mutex
	msgs []string
}

func (m *msgLog) add(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, s)
}

func (m *msgLog) list() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.msgs)
}

// wantMessages checks got against want, where a want ending in "..." only
// has to prefix its message.
func wantMessages(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("messages = %q, want %q", got, want)
	}
	for i := range want {
		if p, ok := strings.CutSuffix(want[i], "..."); ok && strings.HasPrefix(got[i], p) {
			continue
		}
		if got[i] != want[i] {
			t.Errorf("message %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// symbolFields reduces symbols to the fields the ECU downloads fill in.
type symbolFields struct {
	Name             string
	Number           int
	SramOffset       uint32
	Address          uint32
	Length           uint16
	Type             uint8
	Unit             string
	Correctionfactor float64
}

func fields(syms []*symbol.Symbol) []symbolFields {
	out := make([]symbolFields, len(syms))
	for i, s := range syms {
		out[i] = symbolFields{s.Name, s.Number, s.SramOffset, s.Address, s.Length, s.Type, s.Unit, s.Correctionfactor}
	}
	return out
}

// nameTable is a compressed symbol name table as stored in T7/T8 flash: the
// names CRLF-terminated, LZHUF packed and padded to the 4 KiB minimum.
func nameTable(names ...string) []byte {
	packed := lzhuf.Encode([]byte(strings.Join(names, "\r\n") + "\r\n"))
	if len(packed) < 0x1000 {
		packed = append(packed, make([]byte, 0x1000-len(packed))...)
	}
	return packed
}

// noPanic turns a panic in fn into a test failure.
func noPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	fn()
}

// logf adapts t.Log to a progress callback.
func logf(t *testing.T) func(string) { return func(s string) { t.Log(s) } }

// Every download fails up front when the adapter can't be opened.
func TestGetSymbolsAdapterError(t *testing.T) {
	for name, get := range map[string]func() error{
		"T5": func() error { _, err := GetSymbolsT5(testCtx(t), failOpen{}, logf(t)); return err },
		"T7": func() error { _, err := GetSymbolsT7(testCtx(t), failOpen{}, logf(t)); return err },
		"T8": func() error { _, err := GetSymbolsT8(testCtx(t), failOpen{}, logf(t)); return err },
	} {
		if err := get(); !errors.Is(err, errNoAdapter) {
			t.Errorf("%s: error = %v, want %v", name, err, errNoAdapter)
		}
	}
}

// t5Table is a T5 symbol listing as the console prints it after "S\r": a
// 100100<sw version> line, then <addr:4><len:4><name> lines, then END.
const t5Table = "10010012345678.02\r\n" +
	"0D2C0002Rpm\r\n" +
	"4A20000CIgn_map_0!\x00\x00\r\n" +
	"END\r\n"

// GetSymbolsT5 types "S\r" on the console and builds symbols from the
// listing: SRAM offset and length from the hex columns, the name up to the
// first NUL, numbered in order, with the known correction factor.
func TestGetSymbolsT5(t *testing.T) {
	var msgs msgLog
	syms, err := GetSymbolsT5(testCtx(t), &t5Console{table: []byte(t5Table)}, msgs.add)
	if err != nil {
		t.Fatal(err)
	}
	want := []symbolFields{
		{Name: "Rpm", Number: 0, SramOffset: 0x0D2C, Length: 2, Correctionfactor: 1},
		{Name: "Ign_map_0!", Number: 1, SramOffset: 0x4A20, Length: 12, Correctionfactor: 0.1},
	}
	if got := fields(syms.Symbols()); !slices.Equal(got, want) {
		t.Errorf("symbols = %+v\nwant %+v", got, want)
	}
	if s := syms.GetByName("Ign_map_0!"); s == nil || s.Number != 1 {
		t.Errorf("GetByName = %+v", s)
	}
	wantMessages(t, msgs.list(), []string{
		"Connected to ECU",
		"Downloading symbol table",
		"SW: 12345678.02",
		"Loaded 2 symbols from ECU in ...",
	})
}

// Malformed listings, a silent or confused ECU and an ack the adapter can't
// send end the download with an error.
func TestGetSymbolsT5Errors(t *testing.T) {
	notC6 := func(bus *gocan.Bus, f gocan.Frame) bool { // answers console characters with C7
		if f.ID == 0x05 && f.Data[0] == 0xC4 {
			bus.Deliver(gocan.NewFrame(0x0C, []byte{0xC7, 0x00, 0, 0, 0, 0, 0, 0}))
			return true
		}
		return false
	}
	cases := []struct {
		name string
		dev  gocan.Adapter
		want string
	}{
		{"silent ECU", &t5Console{noReply: true}, "send command failed"},
		{"wrong console ack", &hookAdapter{Adapter: &t5Console{}, hook: notC6}, "invalid response"},
		{"listing reply with error status", &t5Console{table: []byte(t5Table), echo: 0xC7, status: 0x04}, "invalid response"},
		{"listing ack not sent", &sendFails{Adapter: &t5Console{table: []byte(t5Table)}, match: func(f gocan.Frame) bool {
			return f.ID == 0x05 && f.Data[0] == 0xC6
		}}, errSendFailed.Error()},
		{"listing never ends", &t5Console{table: []byte("100100X\r\n0D2C0002Rpm\r\n")}, "deadline exceeded"},
		{"bad address", &t5Console{table: []byte("100100X\r\nZZ2C0002Rpm\r\nEND\r\n")}, "invalid byte"},
		{"bad length", &t5Console{table: []byte("100100X\r\n0D2C00G2Rpm\r\nEND\r\n")}, "invalid byte"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := GetSymbolsT5(testCtx(t), c.dev, logf(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

// A listing line too short to hold the address and length columns is an
// error, not a crash.
func TestGetSymbolsT5ShortLine(t *testing.T) {
	noPanic(t, func() {
		_, err := GetSymbolsT5(testCtx(t), &t5Console{table: []byte("100100X\r\n0D2C\r\n4A20000CIgn_map_0!\r\nEND\r\n")}, logf(t))
		if err == nil {
			t.Error("short line accepted")
		}
	})
}

// Listing bytes arrive as C6 00 <byte>; a reply with another command byte
// is not part of the listing.
func TestGetSymbolsT5RejectsForeignReply(t *testing.T) {
	_, err := GetSymbolsT5(testCtx(t), &t5Console{table: []byte(t5Table), echo: 0xC7}, logf(t))
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Errorf("error = %v, want invalid response", err)
	}
}

// An adapter that hands the ECU's answer to the bus as soon as the ack is
// written (as gocan's Loopback and the ecusim adapters do) still delivers the
// whole listing.
func TestGetSymbolsT5ImmediateReply(t *testing.T) {
	syms, err := GetSymbolsT5(testCtx(t), &t5Console{table: []byte(t5Table), sync: true}, logf(t))
	if err != nil {
		t.Fatal(err)
	}
	if syms.Count() != 2 {
		t.Errorf("got %d symbols, want 2", syms.Count())
	}
}

// t7SymbolTable encodes T7 symbol table entries: address u32, length u16,
// type u8, big endian.
func t7SymbolTable(syms ...symbol.Symbol) []byte {
	var b []byte
	for _, s := range syms {
		b = binary.BigEndian.AppendUint32(b, s.Address)
		b = binary.BigEndian.AppendUint16(b, s.Length)
		b = append(b, s.Type)
	}
	return b
}

// t7SymbolECU answers the symbol download services on a t7Fake: routine
// 0x50 sets up the table, transferData hands it over in one block and then
// an empty end block, and the F0 identifier defined by address (2C F0 03)
// reads flash, which starts at base.
func t7SymbolECU(table []byte, base uint32, flash []byte) func(sid byte, p []byte) []byte {
	sent := false
	var addr, n int
	return func(sid byte, p []byte) []byte {
		switch sid {
		case 0x31:
			if p[0] == 0x50 {
				return []byte{0x71, 0x50}
			}
		case 0x36:
			if sent {
				return []byte{0x76, 0x00, 0x00}
			}
			sent = true
			return append([]byte{0x76, 0x00, 0x00, 0x00}, table...)
		case 0x37:
			return []byte{0x77}
		case 0x2C:
			if p[0] == 0xF0 && p[1] == 0x03 {
				n, addr = int(p[3]), int(p[4])<<16|int(p[5])<<8|int(p[6])
				return []byte{0x6C, 0xF0}
			}
		case 0x21:
			if p[0] == 0xF0 {
				off := addr - int(base)
				return append([]byte{0x61, 0xF0}, flash[off:off+n]...)
			}
		}
		return nil
	}
}

// T7 test symbols: the first entry locates the compressed name table.
var (
	t7Base  uint32 = 0x060000
	t7Names        = []string{"Symbol_Table", "In.v_Vehicle   ", "Out.X_AccPedal"}
)

func t7Syms(nameLen uint16) []symbol.Symbol {
	return []symbol.Symbol{
		{Address: t7Base, Length: nameLen, Type: 0x00},
		{Address: 0xF03A10, Length: 2, Type: 0x21},
		{Address: 0xF03A20, Length: 1, Type: 0x04},
	}
}

// GetSymbolsT7 unlocks the ECU, downloads the symbol table through routine
// 0x50 and transferData, then reads the compressed name table the first
// entry points at and names every symbol, trimmed, with its unit and
// correction factor.
func TestGetSymbolsT7(t *testing.T) {
	names := nameTable(t7Names...)
	ecu := &t7Fake{handle: t7SymbolECU(t7SymbolTable(t7Syms(uint16(len(names)))...), t7Base, names)}
	var msgs msgLog
	syms, err := GetSymbolsT7(testCtx(t), ecu, msgs.add)
	if err != nil {
		t.Fatal(err)
	}
	want := []symbolFields{
		{Name: "Symbol_Table", Number: 0, Address: t7Base, Length: uint16(len(names)), Correctionfactor: 1},
		{Name: "In.v_Vehicle", Number: 1, Address: 0xF03A10, Length: 2, Type: 0x21, Unit: "Km/h", Correctionfactor: 0.1},
		{Name: "Out.X_AccPedal", Number: 2, Address: 0xF03A20, Length: 1, Type: 0x04, Unit: "%", Correctionfactor: 0.1},
	}
	if got := fields(syms.Symbols()); !slices.Equal(got, want) {
		t.Errorf("symbols = %+v\nwant %+v", got, want)
	}
	wantMessages(t, msgs.list(), []string{
		"Connected to ECU",
		"Downloading symbol table",
		"Downloading symbol names",
		"Loaded 3 symbols from ECU in ...",
	})
	sids := ecu.services()
	if !bytes.HasPrefix(sids, []byte{0x27, 0x27, 0x31, 0x36, 0x36, 0x37, 0x2C, 0x21}) || sids[len(sids)-1] != 0x82 {
		t.Errorf("services = % X", sids)
	}
	// 4 KiB of names in 0xF0 byte reads
	if reads := bytes.Count(sids, []byte{0x21}); reads != (len(names)+0xEF)/0xF0 {
		t.Errorf("%d name table reads for %d bytes", reads, len(names))
	}
}

// Each failing step of the T7 download ends it with an error.
func TestGetSymbolsT7Errors(t *testing.T) {
	names := nameTable(t7Names...)
	good := t7SymbolTable(t7Syms(uint16(len(names)))...)
	refuse := func(sid byte, next func(byte, []byte) []byte) func(byte, []byte) []byte {
		return func(s byte, p []byte) []byte {
			if s == sid {
				return []byte{0x7F, sid, 0x22}
			}
			return next(s, p)
		}
	}
	erased := bytes.Repeat([]byte{0xFF}, len(names))
	cases := []struct {
		name string
		ecu  *t7Fake
		want string
	}{
		{"no ECU", &t7Fake{silent: true}, "StartSession"},
		{"routine refused", &t7Fake{handle: refuse(0x31, t7SymbolECU(good, t7Base, names))}, "START_ROUTINE_BY_IDENTIFIER"},
		{"transfer refused", &t7Fake{handle: refuse(0x36, t7SymbolECU(good, t7Base, names))}, "TransferData"},
		{"transfer exit refused", &t7Fake{handle: refuse(0x37, t7SymbolECU(good, t7Base, names))}, "REQUEST_TRANSFER_EXIT"},
		{"name read refused", &t7Fake{handle: refuse(0x2C, t7SymbolECU(good, t7Base, names))}, "failed to read memory by address"},
		{"torn symbol table", &t7Fake{handle: t7SymbolECU(good[:9], t7Base, names)}, "unexpected EOF"},
		{"erased name table", &t7Fake{handle: t7SymbolECU(good, t7Base, erased)}, "no compressed symbol table"},
		{"name table too small", &t7Fake{handle: t7SymbolECU(t7SymbolTable(t7Syms(0x100)...), t7Base, names)}, "invalid symbol table size"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := GetSymbolsT7(testCtx(t), c.ecu, logf(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

// An ECU with an empty symbol table is reported as an error, not a crash.
func TestGetSymbolsT7EmptyTable(t *testing.T) {
	ecu := &t7Fake{handle: t7SymbolECU(nil, t7Base, nameTable(t7Names...))}
	noPanic(t, func() {
		if _, err := GetSymbolsT7(testCtx(t), ecu, logf(t)); err == nil {
			t.Error("empty symbol table accepted")
		}
	})
}

// A name table with more names than the symbol table has entries names the
// symbols it has, like the binary loader does, instead of crashing.
func TestGetSymbolsT7ExtraNames(t *testing.T) {
	names := nameTable(append(slices.Clone(t7Names), "Extra.Name")...)
	ecu := &t7Fake{handle: t7SymbolECU(t7SymbolTable(t7Syms(uint16(len(names)))...), t7Base, names)}
	noPanic(t, func() {
		syms, err := GetSymbolsT7(testCtx(t), ecu, logf(t))
		if err != nil {
			t.Fatal(err)
		}
		if got := symNames(syms.Symbols()); !slices.Equal(got, []string{"Symbol_Table", "In.v_Vehicle", "Out.X_AccPedal"}) {
			t.Errorf("names = %q", got)
		}
	})
}

// rdbi is a positive readDataByIdentifier answer.
func rdbi(did byte, data ...byte) []byte { return append([]byte{0x5A, did}, data...) }

// t8Checksum is the symbol table summary T8s answer identifier 0x11 with:
// data length, symbol count, CRC, a 30 byte NUL padded file name, a
// terminator and the compression flag (0x03 = compressed names).
func t8Checksum(count uint16, crc uint32, name string, compressed byte) []byte {
	b := []byte{0x26}
	b = binary.BigEndian.AppendUint16(b, count)
	b = binary.BigEndian.AppendUint32(b, crc)
	fn := make([]byte, 30)
	copy(fn, name)
	b = append(b, fn...)
	return append(b, 0x00, compressed)
}

// t8Gmlan opens a GMLAN client on a t8Fake answering with handle.
func t8Gmlan(t *testing.T, handle func([]byte) []byte) (*gmlan.Client, *t8Fake) {
	ecu := &t8Fake{handle: handle}
	return gmlan.New(openBus(t, ecu), 0x7E0, 0x7E8), ecu
}

// ReadSymbolChecksum decodes identifier 0x11: symbol count, CRC, the raw 30
// byte file name and whether the name table is compressed (flag 0x03).
// Answers too short for a field fail.
func TestReadSymbolChecksum(t *testing.T) {
	full := t8Checksum(0x1234, 0xDEADBEEF, "EUROPE_T8.bin", 0x03)
	fn := string(full[7:37])
	cases := []struct {
		name       string
		data       []byte
		refuse     bool
		count      uint16
		crc        uint32
		compressed bool
		file       string
		wantErr    string
	}{
		{name: "compressed", data: full, count: 0x1234, crc: 0xDEADBEEF, compressed: true, file: fn},
		{name: "plain", data: t8Checksum(7, 1, "EUROPE_T8.bin", 0x01), count: 7, crc: 1},
		{name: "refused", refuse: true, wantErr: "ReadDataByIdentifier"},
		{name: "empty", data: []byte{}, wantErr: "EOF"},
		{name: "no count", data: full[:2], wantErr: "unexpected EOF"},
		{name: "no crc", data: full[:5], wantErr: "unexpected EOF"},
		{name: "no file name", data: full[:7], wantErr: "EOF"},
		{name: "short file name", data: full[:20], wantErr: "expected 30 bytes, got 13"},
		{name: "no flag", data: full[:38], wantErr: "EOF"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gm, _ := t8Gmlan(t, func(req []byte) []byte {
				if req[0] == 0x1A && req[1] == 0x11 {
					if c.refuse {
						return []byte{0x7F, 0x1A, 0x31}
					}
					return rdbi(0x11, c.data...)
				}
				return nil
			})
			count, crc, name, compressed, err := ReadSymbolChecksum(testCtx(t), gm)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("error = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if count != c.count || crc != c.crc || compressed != c.compressed {
				t.Errorf("got count %d crc %X compressed %v", count, crc, compressed)
			}
			if c.file != "" && name != c.file {
				t.Errorf("file name = %q, want %q", name, c.file)
			}
		})
	}
}

// t8Entries encodes T8 symbol table entries: address u32, length u16, type,
// one pad byte.
func t8Entries(syms ...symbol.Symbol) []byte {
	var b []byte
	for _, s := range syms {
		b = binary.BigEndian.AppendUint32(b, s.Address)
		b = binary.BigEndian.AppendUint16(b, s.Length)
		b = append(b, s.Type, 0xEE)
	}
	return b
}

// t8Stream answers identifier 0x13 from a queue: each entry is the data of
// one answer behind the 3 byte block header, nil for the 3 byte end of
// stream answer, or refuseBlock for a negative response.
var refuseBlock = []byte("refuse")

func t8Stream(queue [][]byte) func() []byte {
	var mu sync.Mutex
	return func() []byte {
		mu.Lock()
		defer mu.Unlock()
		if len(queue) == 0 {
			return []byte{0x7F, 0x1A, 0x31}
		}
		next := queue[0]
		queue = queue[1:]
		if bytes.Equal(next, refuseBlock) {
			return []byte{0x7F, 0x1A, 0x31}
		}
		return rdbi(0x13, append([]byte{0x00, 0x01, 0x02}, next...)...)
	}
}

// split cuts b into n byte pieces, then appends the end of stream marker.
func split(b []byte, n int) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		k := min(n, len(b))
		out = append(out, b[:k])
		b = b[k:]
	}
	return append(out, nil)
}

// ReadSymbolTable selects the table with identifier 0x12, then reads 0x13
// blocks until one carries only its header, reassembling entries across
// block boundaries and keeping the session alive every 16 blocks.
func TestReadSymbolTable(t *testing.T) {
	var syms []symbol.Symbol
	for i := range 20 {
		syms = append(syms, symbol.Symbol{Address: 0x100000 + uint32(i)*0x10, Length: uint16(i + 1), Type: byte(i)})
	}
	stream := t8Stream(split(t8Entries(syms...), 5)) // 32 blocks
	gm, ecu := t8Gmlan(t, func(req []byte) []byte {
		switch {
		case req[0] == 0x1A && req[1] == 0x12:
			return rdbi(0x12, 0x00)
		case req[0] == 0x1A && req[1] == 0x13:
			return stream()
		}
		return nil
	})
	got, err := ReadSymbolTable(testCtx(t), gm)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(syms) {
		t.Fatalf("got %d symbols, want %d", len(got), len(syms))
	}
	for i, s := range got {
		want := symbolFields{Name: "Symbol-" + strconv.Itoa(i), Number: i, Address: syms[i].Address, Length: syms[i].Length, Type: syms[i].Type}
		if f := fields([]*symbol.Symbol{s})[0]; f != want {
			t.Errorf("symbol %d = %+v, want %+v", i, f, want)
		}
	}
	if tp := bytes.Count(ecu.requests(), []byte{0x3E}); tp != 2 {
		t.Errorf("%d tester presents over 32 blocks, want 2", tp)
	}
}

// ReadSymbolTable fails on a refused request, a refused keep-alive and a
// table that ends inside an entry.
func TestReadSymbolTableErrors(t *testing.T) {
	entries := t8Entries(symbol.Symbol{Address: 1, Length: 2}, symbol.Symbol{Address: 3, Length: 4})
	many := split(bytes.Repeat([]byte{0}, 17*8), 8)
	cases := []struct {
		name     string
		select12 []byte
		blocks   [][]byte
		tp       []byte
		want     string
	}{
		{"select refused", []byte{0x7F, 0x1A, 0x31}, nil, nil, "ReadDataByIdentifier"},
		{"block refused", nil, [][]byte{entries[:8], refuseBlock}, nil, "ReadDataByIdentifier"},
		{"torn entry", nil, split(entries[:12], 6), nil, "EOF"},
		{"keep-alive refused", nil, many, []byte{0x7F, 0x3E, 0x12}, "TesterPresent - SubFunction not supported"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stream := t8Stream(c.blocks)
			gm, _ := t8Gmlan(t, func(req []byte) []byte {
				switch {
				case req[0] == 0x1A && req[1] == 0x12:
					if c.select12 != nil {
						return c.select12
					}
					return rdbi(0x12, 0x00)
				case req[0] == 0x1A && req[1] == 0x13:
					return stream()
				case req[0] == 0x3E:
					return c.tp
				}
				return nil
			})
			_, err := ReadSymbolTable(testCtx(t), gm)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

// t8SymbolECU answers the T8 symbol download on a t8Fake: the checksum
// summary, the table select, the name table select (1A 14 19 00 00) and
// identifier 0x13 from stream; fail lists services to refuse, keyed by
// service and first parameter byte.
func t8SymbolECU(sum []byte, stream func() []byte, fail map[[2]byte]bool) func([]byte) []byte {
	return func(req []byte) []byte {
		key := [2]byte{req[0], 0}
		if len(req) > 1 {
			key[1] = req[1]
		}
		if fail[key] {
			return []byte{0x7F, req[0], 0x22}
		}
		if req[0] != 0x1A {
			return nil
		}
		switch req[1] {
		case 0x11:
			return rdbi(0x11, sum...)
		case 0x12:
			return rdbi(0x12, 0x00)
		case 0x13:
			return stream()
		case 0x14:
			if bytes.Equal(req[2:], []byte{0x19, 0x00, 0x00}) {
				return rdbi(0x14)
			}
		}
		return nil
	}
}

var t8TestSyms = []symbol.Symbol{
	{Address: 0x100010, Length: 2, Type: 0x21},
	{Address: 0x100020, Length: 1, Type: 0x04},
	{Address: 0x00F000, Length: 256, Type: 0x02},
}

// GetSymbolsT8 unlocks the ECU at level FD, reports the table summary,
// downloads the symbol table and then the compressed names, naming the
// symbols in order with their units and correction factors, and keeps the
// session alive on 0x101 while it streams.
func TestGetSymbolsT8(t *testing.T) {
	names := nameTable("In.v_Vehicle", "Out.X_AccPos", "MAF.m_AirInlet")
	stream := t8Stream(slices.Concat(split(t8Entries(t8TestSyms...), 7), split(names, 240)))
	ecu := &t8Fake{handle: t8SymbolECU(t8Checksum(3, 0xDEADBEEF, "EUROPE_T8.bin", 0x03), stream, nil)}
	var msgs msgLog
	syms, err := GetSymbolsT8(testCtx(t), ecu, msgs.add)
	if err != nil {
		t.Fatal(err)
	}
	want := []symbolFields{
		{Name: "In.v_Vehicle", Number: 0, Address: 0x100010, Length: 2, Type: 0x21, Unit: "Km/h", Correctionfactor: 0.1},
		{Name: "Out.X_AccPos", Number: 1, Address: 0x100020, Length: 1, Type: 0x04, Correctionfactor: 0.1},
		{Name: "MAF.m_AirInlet", Number: 2, Address: 0x00F000, Length: 256, Type: 0x02, Unit: "Mg/c", Correctionfactor: 1},
	}
	if got := fields(syms.Symbols()); !slices.Equal(got, want) {
		t.Errorf("symbols = %+v\nwant %+v", got, want)
	}
	wantMessages(t, msgs.list(), []string{
		"Connected to ECU",
		"Number of symbols: 3",
		"CRC: DEADBEEF",
		"Filename: EUROPE_T8.bin",
		"Compressed: true",
		"Downloading symbol table (this will take a while)",
		"Downloaded 3 symbols in ...",
		"Downloading symbol names (this will take a while)",
		"Downloaded 4 symbol names in ...",
	})
	reqs := ecu.requests()
	if !bytes.HasPrefix(reqs, []byte{0x10, 0x27, 0x27, 0x1A}) || reqs[len(reqs)-1] != 0x20 {
		t.Errorf("requests = % X", reqs)
	}
	ecu.mu.Lock()
	tp := ecu.testerPresent
	ecu.mu.Unlock()
	// after the session start, after selecting the names, and every 15
	// name blocks (18 here)
	if tp != 3 {
		t.Errorf("%d tester presents on 0x101, want 3", tp)
	}
}

// Each failing step of the T8 download ends it with an error.
func TestGetSymbolsT8Errors(t *testing.T) {
	names := nameTable("In.v_Vehicle", "Out.X_AccPos", "MAF.m_AirInlet")
	table := split(t8Entries(t8TestSyms...), 7)
	sum := t8Checksum(3, 1, "x.bin", 0x03)
	cases := []struct {
		name   string
		fail   map[[2]byte]bool
		blocks [][]byte
		want   string
	}{
		{"session refused", map[[2]byte]bool{{0x10, 0x02}: true}, nil, "InitiateDiagnosticOperation"},
		{"seed refused", map[[2]byte]bool{{0x27, 0xFD}: true}, nil, "SecurityAccessRequestSeed"},
		{"summary refused", map[[2]byte]bool{{0x1A, 0x11}: true}, nil, "ReadDataByIdentifier"},
		{"table refused", map[[2]byte]bool{{0x1A, 0x12}: true}, nil, "ReadDataByIdentifier"},
		{"name select refused", map[[2]byte]bool{{0x1A, 0x14}: true}, table, "ReadDataByIdentifier"},
		{"name block refused", nil, slices.Concat(table, [][]byte{names[:240], refuseBlock}), "ReadDataByIdentifier"},
		{"name table too small", nil, slices.Concat(table, split(names[:0x200], 240)), "invalid symbol table size"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ecu := &t8Fake{handle: t8SymbolECU(sum, t8Stream(c.blocks), c.fail)}
			_, err := GetSymbolsT8(testCtx(t), ecu, logf(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

// A name table with fewer names than the symbol table has entries is an
// error, as it is for the binary loader, not a crash.
func TestGetSymbolsT8MissingNames(t *testing.T) {
	names := nameTable("In.v_Vehicle")
	stream := t8Stream(slices.Concat(split(t8Entries(t8TestSyms...), 7), split(names, 240)))
	ecu := &t8Fake{handle: t8SymbolECU(t8Checksum(3, 1, "x.bin", 0x03), stream, nil)}
	noPanic(t, func() {
		if _, err := GetSymbolsT8(testCtx(t), ecu, logf(t)); err == nil {
			t.Error("missing names accepted")
		}
	})
}
