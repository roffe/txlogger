package t7

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/roffe/gocan/v2/t7kwp"
)

// The flash algorithms live in 0x7B000-0x7FE00, which a flash never writes.
const gapStart, gapEnd = 0x7B000, 0x7FE00

// checkFlashed fails unless every byte of the fake's flash outside the
// flash-algorithm gap equals bin and nothing was written into the gap.
func checkFlashed(t *testing.T, e *fakeT7, bin []byte) {
	t.Helper()
	for _, r := range []span{{0, gapStart}, {gapEnd, 0x80000 - gapEnd}} {
		if i := mismatch(e.flash[r.addr:r.addr+r.n], bin[r.addr:r.addr+r.n]); i >= 0 {
			t.Fatalf("flash differs from the bin at 0x%X: %02X, want %02X", r.addr+i, e.flash[r.addr+i], bin[r.addr+i])
		}
	}
	for _, w := range slices.Concat(e.blocks, e.fastBlocks) {
		if w.addr < gapEnd && w.addr+w.n > gapStart {
			t.Fatalf("write 0x%X+0x%X reaches into the flash-algorithm gap", w.addr, w.n)
		}
	}
	for i := gapStart; i < gapEnd; i++ {
		if e.flash[i] != 0xFF {
			t.Fatalf("flash-algorithm gap programmed at 0x%X", i)
		}
	}
}

// mismatch returns the first index where a and b differ, or -1.
func mismatch(a, b []byte) int {
	for i := range a {
		if a[i] != b[i] {
			return i
		}
	}
	return -1
}

// writeSegs turns computeWriteSegments output into spans for comparison.
func writeSegs(bin []byte) []span {
	var out []span
	for _, s := range computeWriteSegments(bin) {
		out = append(out, span{s.start, s.end - s.start})
	}
	return out
}

// FlashECU runs the image reflash in the order the ECU's EOL state machine
// demands: session, security access, fast probe (before EOL, while the
// firmware in flash still answers it), EOL start, erase, testerPresent
// settle, one requestDownload per data segment with 128-byte transferData
// blocks, then requestTransferExit. The flash ends up holding the bin.
func TestFlashECUStandard(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{}
	c := newClient(t, e, rec)
	bin := testBin()

	if err := c.FlashECU(testCtx(t), bin); err != nil {
		t.Fatal(err)
	}
	checkFlashed(t, e, bin)

	if want := writeSegs(bin); !slices.Equal(e.downloads, want) {
		t.Fatalf("requestDownload %X, want one per segment %X", e.downloads, want)
	}
	for _, b := range e.blocks {
		if b.n > flashChunkSize {
			t.Fatalf("transferData block of %d bytes, max %d", b.n, flashChunkSize)
		}
	}
	head := []string{"init", "27 05", "27 06", "31 60", "31 52", "31 53", "3E", "34"}
	if !slices.Equal(e.trace[:len(head)], head) {
		t.Fatalf("trace starts %q, want %q", e.trace[:len(head)], head)
	}
	if last := e.trace[len(e.trace)-1]; last != "37" {
		t.Fatalf("last request %q, want requestTransferExit", last)
	}
	if first, last := rec.progress[0], rec.progress[len(rec.progress)-1]; first != -100 || last != 0x80000 {
		t.Fatalf("progress runs %v..%v, want the erase total -100 first and the end of flash last", first, last)
	}
	if !rec.hasMsg("Erase done") || !rec.hasMsg("Flashing ECU") {
		t.Fatalf("messages %q lack the erase and flash lines", rec.msgs)
	}
}

// FlashECU validates the bin before talking to the ECU.
func TestFlashECURejectsBadBin(t *testing.T) {
	e := &fakeT7{}
	c := newClient(t, e, nil)
	if err := c.FlashECU(testCtx(t), make([]byte, 0x40000)); err == nil {
		t.Fatal("256 KB bin accepted")
	}
	bad := testBin()
	bad[3] = 0x00
	if err := c.FlashECU(testCtx(t), bad); err == nil || !strings.Contains(err.Error(), "Trionic 7") {
		t.Fatalf("err = %v, want a bad-magic error", err)
	}
	if len(e.trace) != 0 {
		t.Fatalf("talked to the ECU about an invalid bin: %q", e.trace)
	}
}

// On an ECU with the fast-transfer extension every segment goes out with
// fast download blocks (even-aligned, at most the probed block size) and no
// transferData; the download still ends with requestTransferExit.
func TestFlashECUFast(t *testing.T) {
	e := &fakeT7{fastBlock: 0x100}
	c := newClient(t, e, nil)
	bin := testBin()

	if err := c.FlashECU(testCtx(t), bin); err != nil {
		t.Fatal(err)
	}
	checkFlashed(t, e, bin)
	if len(e.blocks) != 0 || len(e.downloads) != 0 {
		t.Fatalf("KWP download used (%X, %X) although fast download worked", e.downloads, e.blocks)
	}
	for _, b := range e.fastBlocks {
		if b.addr%2 != 0 || b.n%2 != 0 || b.n > 0x100 {
			t.Fatalf("fast block 0x%X+0x%X: want even and at most the 0x100 block size", b.addr, b.n)
		}
	}
	if last := e.trace[len(e.trace)-1]; last != "37" {
		t.Fatalf("last request %q, want requestTransferExit", last)
	}
}

// A fast block that keeps failing hands that block and every later segment
// to the KWP path, which picks up at the failed block's address.
func TestFlashECUFastFallsBack(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{fastBlock: 0x100}
	e.fault = func(sid byte, p []byte) []byte {
		if sid == t7kwp.START_ROUTINE_BY_IDENTIFIER && p[0] == t7kwp.RLI_FAST_COMMIT && e.fastAdr == 0x1000 {
			e.armed = false
			return []byte{0x7F, sid, t7kwp.BLOCK_TRANSFER_DATA_CHECKSUM_ERROR}
		}
		return nil
	}
	c := newClient(t, e, rec)
	bin := testBin()

	if err := c.FlashECU(testCtx(t), bin); err != nil {
		t.Fatal(err)
	}
	checkFlashed(t, e, bin)
	segs := writeSegs(bin)
	if want := segs[1:]; !slices.Equal(e.downloads, want) {
		t.Fatalf("requestDownload %X, want the failed segment and the rest %X", e.downloads, want)
	}
	if !slices.Equal(e.fastBlocks, segs[:1]) {
		t.Fatalf("fast blocks %X, want only the first segment %X", e.fastBlocks, segs[:1])
	}
	if !rec.hasMsg("Fast flash failed, continuing with standard transfer") {
		t.Fatalf("messages %q do not report the fallback", rec.msgs)
	}
}

// A transferData block the ECU rejects is retried by re-anchoring the write
// pointer at that block with requestDownload, so the blocks before it are not
// written again.
func TestWriteBinResumesAtFailedBlock(t *testing.T) {
	failed := false
	e := &fakeT7{}
	e.fault = func(sid byte, _ []byte) []byte {
		if sid == t7kwp.TRANSFER_DATA && e.writePtr == 0x1080 && !failed {
			failed = true
			return []byte{0x7F, sid, t7kwp.BLOCK_TRANSFER_DATA_CHECKSUM_ERROR}
		}
		return nil
	}
	c := newClient(t, e, nil)
	bin := testBin()

	if err := c.FlashECU(testCtx(t), bin); err != nil {
		t.Fatal(err)
	}
	checkFlashed(t, e, bin)
	if !slices.Contains(e.downloads, span{0x1080, 0x80}) {
		t.Fatalf("requestDownload %X: no re-anchor at the failed block 0x1080", e.downloads)
	}
	n := 0
	for _, b := range e.blocks {
		if b.addr == 0x1000 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("block 0x1000 written %d times, want once", n)
	}
}

// busyRepeatRequest on transferData is answered by resending the identical
// block, never by re-anchoring with requestDownload (a busy T7 would read the
// address out of its kept block buffer).
func TestWriteBinResendsBusyBlock(t *testing.T) {
	busy := false
	e := &fakeT7{}
	e.fault = func(sid byte, _ []byte) []byte {
		if sid == t7kwp.TRANSFER_DATA && e.writePtr == 0x1080 && !busy {
			busy = true
			return []byte{0x7F, sid, t7kwp.BUSY_REPEAT_REQUEST}
		}
		return nil
	}
	c := newClient(t, e, nil)
	bin := testBin()

	if err := c.FlashECU(testCtx(t), bin); err != nil {
		t.Fatal(err)
	}
	checkFlashed(t, e, bin)
	if want := writeSegs(bin); !slices.Equal(e.downloads, want) {
		t.Fatalf("requestDownload %X, want exactly one per segment %X", e.downloads, want)
	}
}

// A refused requestTransferExit fails the flash.
func TestWriteBinTransferExitFails(t *testing.T) {
	e := &fakeT7{fault: func(sid byte, _ []byte) []byte {
		if sid == t7kwp.REQUEST_TRANSFER_EXIT {
			return []byte{0x7F, sid, t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR}
		}
		return nil
	}}
	err := newClient(t, e, nil).FlashECU(testCtx(t), testBin())
	if err == nil || !strings.Contains(err.Error(), "exit download mode failed") {
		t.Fatalf("err = %v, want an exit-download failure", err)
	}
}

// A context cancelled during the fast download stops the flash with the
// context's error instead of falling back to the KWP path.
func TestWriteBinFastCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(testCtx(t))
	defer cancel()
	e := &fakeT7{fastBlock: 0x100}
	e.fault = func(sid byte, p []byte) []byte {
		if sid == t7kwp.START_ROUTINE_BY_IDENTIFIER && p[0] == t7kwp.RLI_FAST_COMMIT {
			cancel()
			return noReply
		}
		return nil
	}
	err := newClient(t, e, nil).FlashECU(ctx, testBin())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(e.downloads) != 0 {
		t.Fatalf("fell back to requestDownload %X after cancellation", e.downloads)
	}
}

// A failed erase stops the flash before anything is written.
func TestFlashECUEraseFails(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{fault: func(sid byte, p []byte) []byte {
		if sid == t7kwp.START_ROUTINE_BY_IDENTIFIER && p[0] == t7kwp.RLI_ERASE {
			return []byte{0x7F, sid, t7kwp.CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR}
		}
		return nil
	}}
	err := newClient(t, e, rec).FlashECU(testCtx(t), testBin())
	if !errors.Is(err, t7kwp.ErrConditionsNotCorrectOrRequestSequenceError) {
		t.Fatalf("err = %v, want conditionsNotCorrect", err)
	}
	if e.count("34") != 0 {
		t.Fatalf("trace %q: download started after a failed erase", e.trace)
	}
	if rec.hasMsg("Erase done") {
		t.Fatal("reported a finished erase")
	}
}

// EraseECU drives the progress bar from -eraseProgressSteps up to
// eraseProgressSteps, one step per ECU poll.
func TestEraseECUProgress(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{}
	c := newClient(t, e, rec)
	openSession(t, c)

	if err := c.EraseECU(testCtx(t)); err != nil {
		t.Fatal(err)
	}
	if !e.erased {
		t.Fatal("ECU not erased")
	}
	p := rec.progress
	if len(p) < 3 || p[0] != -eraseProgressSteps || p[len(p)-1] != eraseProgressSteps {
		t.Fatalf("progress %v, want -%d, ticks, %d", p, eraseProgressSteps, eraseProgressSteps)
	}
	for i := 2; i < len(p)-1; i++ {
		if p[i] != p[i-1]+1 {
			t.Fatalf("progress %v does not count up one per poll", p)
		}
	}
}

// eolParams are valid EOL fields with a tester serial off the kill-list.
var eolParams = EOLParams{VIN: "YS3DD58N598765432", ProgDate: "261010", TesterSerial: DefaultTesterSerial}

// EOLFlash runs the image flash, then writes VIN 0x90, programming date 0x99
// and the padded tester serial 0x98, and ends with EOLEndOfProcedure 0x54.
func TestEOLFlash(t *testing.T) {
	rec := &recorder{}
	e := &fakeT7{}
	c := newClient(t, e, rec)
	bin := testBin()

	if err := c.EOLFlash(testCtx(t), bin, eolParams); err != nil {
		t.Fatal(err)
	}
	checkFlashed(t, e, bin)
	tail := []string{"37", "3B 90", "3B 99", "3B 98", "31 54"}
	if got := e.trace[len(e.trace)-len(tail):]; !slices.Equal(got, tail) {
		t.Fatalf("trace ends %q, want %q", got, tail)
	}
	if string(e.piWrites[0x90]) != eolParams.VIN || string(e.piWrites[0x99]) != eolParams.ProgDate {
		t.Fatalf("PI writes %q", e.piWrites)
	}
	if got, want := e.piWrites[0x98], t7kwp.PadTesterSerial(eolParams.TesterSerial); !bytes.Equal(got, want) {
		t.Fatalf("tester serial written as %q, want padded %q", got, want)
	}
	if !rec.hasMsg("EOL programming complete") {
		t.Fatalf("messages %q lack the completion line", rec.msgs)
	}
}

// EOLFlash refuses, before any CAN traffic, params the ECU would reject or
// brick itself over, and a bin whose ROM checksum the ECU would fail at
// end-of-procedure.
func TestEOLFlashRefusesUpFront(t *testing.T) {
	badSum := testBin()
	badSum[0x1000] ^= 0x01
	noVIN := eolParams
	noVIN.VIN = ""
	noDate := eolParams
	noDate.ProgDate = ""
	for _, tt := range []struct {
		name string
		bin  []byte
		p    EOLParams
		want string
	}{
		{"short bin", make([]byte, 0x70100), eolParams, "512KB"},
		{"missing VIN", testBin(), noVIN, "VIN"},
		{"missing programming date", testBin(), noDate, "programming date"},
		{"checksum mismatch", badSum, eolParams, "checksum mismatch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := &fakeT7{}
			err := newClient(t, e, nil).EOLFlash(testCtx(t), tt.bin, tt.p)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tt.want)
			}
			if len(e.trace) != 0 {
				t.Fatalf("talked to the ECU: %q", e.trace)
			}
		})
	}
}

// A bin without the PI checksum fields cannot be verified up front: EOLFlash
// warns and goes ahead.
func TestEOLFlashUnverifiableChecksum(t *testing.T) {
	rec := &recorder{}
	bin := bytes.Repeat([]byte{0xFF}, 0x80000)
	copy(bin, []byte{0xFF, 0xFF, 0xEF, 0xFC})
	e := &fakeT7{}
	if err := newClient(t, e, rec).EOLFlash(testCtx(t), bin, eolParams); err != nil {
		t.Fatal(err)
	}
	if !rec.hasMsg("warning: cannot verify ROM checksum") {
		t.Fatalf("messages %q lack the checksum warning", rec.msgs)
	}
	if e.count("31 54") != 1 {
		t.Fatalf("trace %q: end of procedure not run", e.trace)
	}
}

// A refused PI field write stops the EOL run before end-of-procedure.
func TestEOLFlashWriteFieldFails(t *testing.T) {
	e := &fakeT7{fault: func(sid byte, p []byte) []byte {
		if sid == t7kwp.WRITE_DATA_BY_IDENTIFIER && p[0] == 0x99 {
			return []byte{0x7F, sid, t7kwp.REQUEST_OUT_OF_RANGE}
		}
		return nil
	}}
	err := newClient(t, e, nil).EOLFlash(testCtx(t), testBin(), eolParams)
	if err == nil || !strings.Contains(err.Error(), "EOL: writing programming date") {
		t.Fatalf("err = %v, want a programming-date write failure", err)
	}
	if e.count("31 54") != 0 {
		t.Fatalf("trace %q: end of procedure run after a failed write", e.trace)
	}
}

// When EOLEndOfProcedure is refused the error carries the ECU's raw routine
// results, the only clue to why it refused.
func TestEOLFlashEndFails(t *testing.T) {
	e := &fakeT7{eolResult: 0x77, fault: func(sid byte, p []byte) []byte {
		if sid == t7kwp.START_ROUTINE_BY_IDENTIFIER && p[0] == t7kwp.RLI_END_EOL {
			return []byte{0x7F, sid, t7kwp.BLOCK_TRANSFER_DATA_CHECKSUM_ERROR}
		}
		return nil
	}}
	err := newClient(t, e, nil).EOLFlash(testCtx(t), testBin(), eolParams)
	if !errors.Is(err, t7kwp.ErrBlockTransferDataChecksumError) || !strings.Contains(err.Error(), "routine results") || !strings.Contains(err.Error(), "73 54 77") {
		t.Fatalf("err = %v, want the checksum NRC with routine results 73 54 77", err)
	}
}
