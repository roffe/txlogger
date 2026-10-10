package ecu

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/ecusim"
)

func symNames(syms []*symbol.Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Name
	}
	return out
}

// t5Flags is a T5 binary with four one-byte error flags in RAM.
func t5Flags() symbol.FirmwareFile {
	return symbol.NewCollection(
		&symbol.Symbol{Name: "Tryck_error", SramOffset: 0x1100, Length: 1},
		&symbol.Symbol{Name: "Rpm", SramOffset: 0x1000, Length: 2},
		&symbol.Symbol{Name: "Lambda_fel", SramOffset: 0x1140, Length: 1},
		&symbol.Symbol{Name: "Knock_error", SramOffset: 0x1120, Length: 1},
		&symbol.Symbol{Name: "Temp_fel", SramOffset: 0x1160, Length: 1},
	)
}

// T5 error flags are the one-byte symbols named *_error or *_fel, sorted
// case-insensitively; a T5 has no DTC service, so a binary is required.
func TestT5DTCSymbols(t *testing.T) {
	fw := symbol.NewCollection(
		&symbol.Symbol{Name: "b_error", Length: 1},
		&symbol.Symbol{Name: "Rpm", Length: 2},
		&symbol.Symbol{Name: "c_error", Length: 1},
		&symbol.Symbol{Name: "Wide_error", Length: 2}, // not a flag byte
		&symbol.Symbol{Name: "error_count", Length: 1},
		&symbol.Symbol{Name: "Upper_FEL", Length: 1}, // suffix match is case-sensitive
		&symbol.Symbol{Name: "A_fel", Length: 1},
	)
	syms, err := t5DTCSymbols(fw)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := symNames(syms), []string{"A_fel", "b_error", "c_error"}; !slices.Equal(got, want) {
		t.Errorf("flags = %v, want %v", got, want)
	}

	if _, err := t5DTCSymbols(nil); err == nil || !strings.Contains(err.Error(), "load a binary") {
		t.Errorf("no binary: error = %v", err)
	}
	if _, err := readT5DTC(testCtx(t), nil, nil); err == nil {
		t.Error("readT5DTC without a binary did not fail")
	}
	if err := clearT5DTC(testCtx(t), nil, nil); err == nil {
		t.Error("clearT5DTC without a binary did not fail")
	}
}

// readT5DTC reads every flag from RAM and reports the set ones, in flag
// order, with the flag byte as status.
func TestReadT5DTC(t *testing.T) {
	sim := &ecusim.T5{}
	sim.Poke(0x1100, []byte{0x03}) // Tryck_error
	sim.Poke(0x1140, []byte{0x01}) // Lambda_fel
	sim.Poke(0x1000, []byte{0xFF, 0xFF})
	got, err := readT5DTC(testCtx(t), openBus(t, sim), t5Flags())
	if err != nil {
		t.Fatal(err)
	}
	want := []dtc.DTC{
		{ECU: symbol.ECU_T5, Code: "Lambda_fel", Status: 0x01},
		{ECU: symbol.ECU_T5, Code: "Tryck_error", Status: 0x03},
	}
	if !slices.Equal(got, want) {
		t.Errorf("DTCs = %+v, want %+v", got, want)
	}

	clean, err := readT5DTC(testCtx(t), openBus(t, &ecusim.T5{}), t5Flags())
	if err != nil || len(clean) != 0 {
		t.Errorf("clean ECU: %+v, %v", clean, err)
	}
}

// t5ReadFails answers the T5 RAM read of addr with error status 04.
func t5ReadFails(addr uint32) func(*gocan.Bus, gocan.Frame) bool {
	return func(bus *gocan.Bus, f gocan.Frame) bool {
		if f.ID != 0x05 || f.Data[0] != 0xC7 || uint32(f.Data[3])<<8|uint32(f.Data[4]) != addr+5 {
			return false
		}
		bus.Deliver(gocan.NewFrame(0x0C, []byte{0xC7, 0x04, 0, 0, 0, 0, 0, 0}))
		return true
	}
}

// A failed flag read aborts readT5DTC with the flag's name.
func TestReadT5DTCReadError(t *testing.T) {
	dev := &hookAdapter{Adapter: &ecusim.T5{}, hook: t5ReadFails(0x1120)}
	_, err := readT5DTC(testCtx(t), openBus(t, dev), t5Flags())
	if err == nil || !strings.Contains(err.Error(), "error reading Knock_error") {
		t.Errorf("error = %v", err)
	}
}

// t5Writes answers the T5 console in place of the sim: each completed
// W<addr><byte> command is recorded and poked into sim, except a write to
// failAddr, whose final '\r' goes unanswered so the tester's request times
// out. Characters of other commands are acknowledged with C6 00.
type t5Writes struct {
	sim      *ecusim.T5
	failAddr uint32

	mu    sync.Mutex
	line  []byte
	lines []string
}

func (w *t5Writes) hook(bus *gocan.Bus, f gocan.Frame) bool {
	if f.ID != 0x05 || f.Data[0] != 0xC4 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if f.Data[1] != '\r' {
		w.line = append(w.line, f.Data[1])
		bus.Deliver(gocan.NewFrame(0x0C, []byte{0xC6, 0x00, 0, 0, 0, 0, 0, 0}))
		return true
	}
	line := string(w.line)
	w.line = w.line[:0]
	var addr, val uint32
	if _, err := fmt.Sscanf(line, "W%04X%02X", &addr, &val); err != nil {
		return true
	}
	if addr == w.failAddr {
		return true
	}
	w.lines = append(w.lines, line)
	w.sim.Poke(addr, []byte{byte(val)})
	bus.Deliver(gocan.NewFrame(0x0C, []byte{0xC6, 0x00, 0, 0, 0, 0, 0, 0}))
	return true
}

func (w *t5Writes) written() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.lines)
}

// clearT5DTC zeroes the set flags with console writes, in flag order, and
// leaves flags that are already clear alone.
func TestClearT5DTC(t *testing.T) {
	sim := &ecusim.T5{}
	sim.Poke(0x1100, []byte{0x03}) // Tryck_error
	sim.Poke(0x1160, []byte{0x80}) // Temp_fel
	sim.Poke(0x1000, []byte{0xAA, 0xBB})
	w := &t5Writes{sim: sim, failAddr: 0xFFFFFFFF}
	dev := &hookAdapter{Adapter: sim, hook: w.hook}
	if err := clearT5DTC(testCtx(t), openBus(t, dev), t5Flags()); err != nil {
		t.Fatal(err)
	}
	for _, addr := range []uint32{0x1100, 0x1120, 0x1140, 0x1160} {
		if v := sim.Peek(addr, 1)[0]; v != 0 {
			t.Errorf("flag at 0x%X = %02X after clear", addr, v)
		}
	}
	if got := sim.Peek(0x1000, 2); !bytes.Equal(got, []byte{0xAA, 0xBB}) {
		t.Errorf("non-flag symbol changed to % X", got)
	}
	if got, want := w.written(), []string{"W116000", "W110000"}; !slices.Equal(got, want) {
		t.Errorf("console writes = %q, want %q", got, want)
	}
}

// clearT5DTC carries on past a failed read or write and reports every
// failure together, by flag name.
func TestClearT5DTCCarriesOn(t *testing.T) {
	sim := &ecusim.T5{}
	for _, addr := range []uint32{0x1100, 0x1120, 0x1140, 0x1160} {
		sim.Poke(addr, []byte{0x01})
	}
	w := &t5Writes{sim: sim, failAddr: 0x1140} // Lambda_fel write times out
	readFail := t5ReadFails(0x1120)            // Knock_error read fails
	dev := &hookAdapter{Adapter: sim, hook: func(bus *gocan.Bus, f gocan.Frame) bool {
		return readFail(bus, f) || w.hook(bus, f)
	}}
	err := clearT5DTC(testCtx(t), openBus(t, dev), t5Flags())
	if err == nil {
		t.Fatal("no error reported")
	}
	for _, want := range []string{"error reading Knock_error before clearing", "error clearing Lambda_fel"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	for addr, want := range map[uint32]byte{0x1100: 0, 0x1120: 1, 0x1140: 1, 0x1160: 0} {
		if v := sim.Peek(addr, 1)[0]; v != want {
			t.Errorf("flag at 0x%X = %d, want %d", addr, v, want)
		}
	}
}

// readT7DTC reads the stored DTCs (service 0x18, status 0x02) inside a KWP
// session that it closes afterwards, collecting a multi-frame answer.
func TestReadT7DTC(t *testing.T) {
	var status []byte
	ecu := &t7Fake{handle: func(sid byte, p []byte) []byte {
		if sid != 0x18 {
			return nil
		}
		status = append(status, p[0])
		return []byte{0x58, 0x03, 0x01, 0x05, 0x08, 0xC1, 0x23, 0x02, 0x00, 0x10, 0x0A}
	}}
	got, err := readT7DTC(testCtx(t), openBus(t, ecu), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []dtc.DTC{
		{ECU: symbol.ECU_T7, Code: "P0105", Status: 0x08},
		{ECU: symbol.ECU_T7, Code: "PC123", Status: 0x02},
		{ECU: symbol.ECU_T7, Code: "P0010", Status: 0x0A},
	}
	if !slices.Equal(got, want) {
		t.Errorf("DTCs = %+v, want %+v", got, want)
	}
	if !bytes.Equal(status, []byte{0x02}) {
		t.Errorf("status masks requested: % X", status)
	}
	if got := ecu.services(); !bytes.Equal(got, []byte{0x18, 0x82}) {
		t.Errorf("services = % X, want 18 82", got)
	}
}

// clearT7DTC sends clearDiagnosticInformation for all groups (FF 00) and
// closes the session even when the ECU refuses.
func TestClearT7DTC(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		var group []byte
		ecu := &t7Fake{handle: func(sid byte, p []byte) []byte {
			if sid != 0x14 {
				return nil
			}
			group = append([]byte(nil), p[:2]...)
			if refuse {
				return []byte{0x7F, 0x14, 0x22}
			}
			return []byte{0x54, 0xFF, 0x00}
		}}
		err := clearT7DTC(testCtx(t), openBus(t, ecu), nil)
		if refuse != (err != nil) {
			t.Errorf("refuse=%v: error = %v", refuse, err)
		}
		if !bytes.Equal(group, []byte{0xFF, 0x00}) {
			t.Errorf("refuse=%v: group = % X, want FF 00", refuse, group)
		}
		if got := ecu.services(); !bytes.Equal(got, []byte{0x14, 0x82}) {
			t.Errorf("refuse=%v: services = % X, want 14 82", refuse, got)
		}
	}
}

// t7Session fails without running anything when the ECU doesn't answer the
// session start, and reports a refused session stop alongside the result.
func TestT7SessionErrors(t *testing.T) {
	silent := &t7Fake{silent: true}
	if _, err := readT7DTC(testCtx(t), openBus(t, silent), nil); err == nil || !strings.Contains(err.Error(), "StartSession") {
		t.Errorf("silent ECU: error = %v", err)
	}
	if got := silent.services(); len(got) != 0 {
		t.Errorf("silent ECU was sent services % X", got)
	}

	stuck := &t7Fake{handle: func(sid byte, p []byte) []byte {
		switch sid {
		case 0x18:
			return []byte{0x58, 0x01, 0x01, 0x05, 0x08}
		case 0x82:
			return []byte{0x7F, 0x82, 0x22}
		}
		return nil
	}}
	got, err := readT7DTC(testCtx(t), openBus(t, stuck), nil)
	if err == nil || !strings.Contains(err.Error(), "error stopping session") {
		t.Errorf("refused stop: error = %v", err)
	}
	if want := []dtc.DTC{{ECU: symbol.ECU_T7, Code: "P0105", Status: 0x08}}; !slices.Equal(got, want) {
		t.Errorf("refused stop: DTCs = %+v, want %+v", got, want)
	}
}

// readT8DTC asks for DTCs by status mask 0x12 (readDiagnosticInformation
// 0x81) in a diagnostic session, collects the UUDT stream up to its end
// marker and returns to normal mode. Codes carry the failure type suffix.
func TestReadT8DTC(t *testing.T) {
	ecu := &t8Fake{
		handle: func(req []byte) []byte {
			if req[0] == 0xA9 {
				return []byte{0x7F, 0xA9, 0x78} // responsePending, then the stream
			}
			return nil
		},
		uudt: [][]byte{
			{0x81, 0x81, 0x65, 0x02, 0x0B, 0, 0, 0},
			{0x81, 0x03, 0x00, 0x00, 0x2F, 0, 0, 0},
			{0x81, 0x00, 0x00, 0x00, 0x00, 0, 0, 0},
		},
	}
	got, err := readT8DTC(testCtx(t), openBus(t, ecu), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []dtc.DTC{
		{ECU: symbol.ECU_T8, Code: "B0165 02", FailureType: 0x02, Status: 0x0B},
		{ECU: symbol.ECU_T8, Code: "P0300 00", FailureType: 0x00, Status: 0x2F},
	}
	if !slices.Equal(got, want) {
		t.Errorf("DTCs = %+v, want %+v", got, want)
	}
	ecu.mu.Lock()
	reqs := slices.Clone(ecu.reqs)
	ecu.mu.Unlock()
	if len(reqs) != 3 || !bytes.Equal(reqs[0], []byte{0x10, 0x02}) || !bytes.Equal(reqs[1], []byte{0xA9, 0x81, 0x12}) || !bytes.Equal(reqs[2], []byte{0x20}) {
		t.Errorf("requests = % X", reqs)
	}
}

// clearT8DTC sends clearDiagnosticInformation to the functional address
// inside a diagnostic session and reports a refusal.
func TestClearT8DTC(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		ecu := &t8Fake{handle: func(req []byte) []byte {
			if req[0] != 0x04 {
				return nil
			}
			if refuse {
				return []byte{0x7F, 0x04, 0x22}
			}
			return []byte{0x44}
		}}
		err := clearT8DTC(testCtx(t), openBus(t, ecu), nil)
		if refuse != (err != nil) {
			t.Errorf("refuse=%v: error = %v", refuse, err)
		}
		if refuse && !strings.Contains(err.Error(), "ClearDiagnosticInformation") {
			t.Errorf("refusal error = %v", err)
		}
		if got := ecu.requests(); !bytes.Equal(got, []byte{0x10, 0x04, 0x20}) {
			t.Errorf("refuse=%v: requests = % X, want 10 04 20", refuse, got)
		}
		if got := ecu.requestIDs(); !slices.Equal(got, []uint32{0x7E0, 0x7DF, 0x7E0}) {
			t.Errorf("refuse=%v: request ids = %X, want 7E0 7DF 7E0", refuse, got)
		}
	}
}

// A refused diagnostic session start ends readT8DTC before any DTC request
// and without a returnToNormalMode.
func TestT8SessionInitiateFails(t *testing.T) {
	ecu := &t8Fake{handle: func(req []byte) []byte {
		if req[0] == 0x10 {
			return []byte{0x7F, 0x10, 0x22}
		}
		return nil
	}}
	_, err := readT8DTC(testCtx(t), openBus(t, ecu), nil)
	if err == nil || !strings.Contains(err.Error(), "InitiateDiagnosticOperation") {
		t.Errorf("error = %v", err)
	}
	if got := ecu.requests(); !bytes.Equal(got, []byte{0x10}) {
		t.Errorf("requests = % X, want only 10", got)
	}
}
