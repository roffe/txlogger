package t7

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan/v2/t7kwp"
)

// DefaultTesterSerial is what txlogger stamps into PI field 0x98 — the field
// records which tester ran the EOL procedure, so it should say txlogger rather
// than echo whatever tester programmed the donor binary.
//
// It deliberately copies the factory shape: a 9-character prefix ("PPCAN Nr ")
// followed by the tester number, because the ECU's kill-list check only looks at
// offsets 9..11. Here those are "EOL", and any triple of uppercase letters packs
// to (name[9]<<9 + name[10]<<4 + name[11]) >= 0x8000 — above every entry in the
// kill-list, whose largest is 0x7388. "txlogger EOL" packs to 0x8F3C.
// TestDefaultTesterSerial pins this.
const DefaultTesterSerial = "txlogger EOL"

// EOLParams are the three PI-area fields the tester owns during an EOL run.
// The ECU refuses startRoutine EOLEndOfProcedure (0x54) until all three have
// been written, so none of them is optional.
type EOLParams struct {
	VIN          string // 0x90, 17 chars
	ProgDate     string // 0x99, "YYMMDD"
	TesterSerial string // 0x98, padded to 13 — see t7kwp.TesterSerialBlocked
}

// Validate checks the params the ECU would otherwise reject, plus the one it
// would answer by erasing itself.
func (p EOLParams) Validate() error {
	if p.VIN == "" {
		return errors.New("VIN (0x90) is required: the ECU aborts EOL without one")
	}
	if p.ProgDate == "" {
		return errors.New("programming date (0x99) is required")
	}
	if t7kwp.TesterSerialBlocked(t7kwp.PadTesterSerial(p.TesterSerial)) {
		return fmt.Errorf("tester serial %q is on the ECU's kill-list: writing it runs FlashErase(0) and halts the ECU (BDM recovery only)", p.TesterSerial)
	}
	return nil
}

// piU32 reads a 4-byte big-endian PI-area field.
func piU32(bin []byte, id byte) (uint32, error) {
	s, err := GetHeaderField(bin, id)
	if err != nil {
		return 0, err
	}
	if len(s) != 4 {
		return 0, fmt.Errorf("PI field 0x%02X is %d bytes, want 4", id, len(s))
	}
	return binary.BigEndian.Uint32([]byte(s)), nil
}

// ROMChecksum reproduces XEolprg.c's EOLEndOfProcedure checksum: a plain u32 sum
// from PI field 0xFD (bottom of flash) up to 0xFE (top of program), compared
// against the stored 0xFB. The ECU runs exactly this after an EOL flash and
// aborts with blockTransferDataChecksumError if the two disagree.
func ROMChecksum(bin []byte) (calc, stored uint32, err error) {
	bottom, err := piU32(bin, 0xFD)
	if err != nil {
		return 0, 0, err
	}
	top, err := piU32(bin, 0xFE)
	if err != nil {
		return 0, 0, err
	}
	stored, err = piU32(bin, 0xFB)
	if err != nil {
		return 0, 0, err
	}
	const topOfFlash = 0x7FFFF
	for p := bottom; p < top && p < topOfFlash && p+4 <= uint32(len(bin)); p += 4 {
		calc += binary.BigEndian.Uint32(bin[p : p+4])
	}
	return calc, stored, nil
}

// EOLFlash runs the factory End-Of-Line programming procedure from spec §3.4.1,
// enforced ECU-side by EOLCommCntrl in XEolprg.c:
//
//	startCommunication → securityAccess → EOLProgrammingStart (0x31 0x52) →
//	EOLEraseFlash (0x31 0x53) → requestDownload/transferData → transferExit
//	(0x37) → writeData VIN 0x90 / date 0x99 / tester serial 0x98 (0x3B) →
//	EOLEndOfProcedure (0x31 0x54)
//
// Over FlashECU's plain image reflash this adds the last two steps. They are
// what make the ECU re-derive its PI area the way the factory does: at
// transferExit it rewrites the Delco traceability flag, hardware numbers and
// the four security seed/key words itself, and at 0x54 it verifies the ROM
// checksum against the image's own 0xFB before setting the EOL-success flag
// 0xF9. This cannot be split across sessions — the ECU's EOL state machine runs
// from RAM and resets on reconnect — so the whole thing is one call.
func (t *Client) EOLFlash(ctx context.Context, bin []byte, p EOLParams) error {
	bin, err := NormalizeBin(bin)
	if err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}

	// The ECU aborts EOL on a checksum mismatch after the flash is already
	// written, so say so up front rather than after 20 minutes.
	if calc, stored, err := ROMChecksum(bin); err != nil {
		t.cfg.OnMessage(fmt.Sprintf("warning: cannot verify ROM checksum: %v", err))
	} else if calc != stored {
		return fmt.Errorf("bin ROM checksum mismatch: computed %08X, stored (0xFB) %08X — the ECU would abort EOL at end-of-procedure. Fix the checksum first, or use the plain Flash button", calc, stored)
	}

	if err := t.DataInitialization(ctx); err != nil {
		return err
	}
	ok, err := t.KnockKnock(ctx)
	if err != nil || !ok {
		return fmt.Errorf("failed to authenticate: %v", err)
	}
	if err := t.EraseECU(ctx); err != nil {
		return err
	}
	if err := t.writeBin(ctx, bin); err != nil {
		return err
	}

	// The ECU rewrote the Delco HW numbers and security words at transferExit;
	// these three are the tester's half of the PI area.
	fields := []struct {
		id   byte
		name string
		data []byte
	}{
		{0x90, "VIN", []byte(p.VIN)},
		{0x99, "programming date", []byte(p.ProgDate)},
		{0x98, "tester serial", t7kwp.PadTesterSerial(p.TesterSerial)},
	}
	for _, f := range fields {
		t.cfg.OnMessage(fmt.Sprintf("Writing %s (0x%02X): %q", f.name, f.id, string(f.data)))
		if err := t.kwp.WriteDataByLocalIdentifier(ctx, f.id, f.data); err != nil {
			return fmt.Errorf("EOL: writing %s: %w", f.name, err)
		}
	}

	t.cfg.OnMessage("Running end of procedure (checksum verify + EOL flag)")
	if err := t.kwp.EndEOL(ctx); err != nil {
		return err
	}

	t.cfg.OnMessage("EOL programming complete")
	time.Sleep(200 * time.Millisecond)
	return nil
}
