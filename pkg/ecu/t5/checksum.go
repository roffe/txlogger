package t5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
)

// GetECUChecksum runs the bootloader C8 command. MyBooty calculates the
// checksum over [ROM_Offset .. Code_End] (both read from the flash footer)
// and compares it against the value stored in the last 4 bytes of flash.
// Status 0x01 means the checksum did NOT match (or the footer is unreadable);
// no value is returned in that case.
func (t *Client) GetECUChecksum(ctx context.Context) ([]byte, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return nil, err
		}
	}
	data, err := t.command(ctx, []byte{0xC8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, checksumTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECU checksum: %w", err)
	}
	if data[1] != 0x00 {
		return nil, errors.New("ECU reports FLASH checksum mismatch")
	}
	out := make([]byte, 4)
	copy(out, data[2:6])
	return out, nil
}

// CalculateBinChecksum calculates the checksum of a bin the same way MyBooty
// does in flash: sum of the bytes from file start through Code_End (both
// ROM_Offset and Code_End are read from the footer at the end of the bin).
func (t *Client) CalculateBinChecksum(bin []byte) ([]byte, error) {
	end, err := binCodeEnd(bin)
	if err != nil {
		return nil, err
	}
	var calculated uint32
	for _, b := range bin[:end+1] {
		calculated += uint32(b)
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, calculated)
	return out, nil
}

// ValidateBinChecksum verifies a bin before flashing: the calculated checksum
// must match the one stored in the last 4 bytes, exactly like the ECU will
// verify it with the C8 command after flashing.
func (t *Client) ValidateBinChecksum(bin []byte) error {
	calculated, err := t.CalculateBinChecksum(bin)
	if err != nil {
		return err
	}
	stored := bin[len(bin)-4:]
	if binary.BigEndian.Uint32(stored) != binary.BigEndian.Uint32(calculated) {
		return fmt.Errorf("bin checksum mismatch: stored %X, calculated %X", stored, calculated)
	}
	return nil
}

// binCodeEnd returns the file offset of the last byte included in the
// checksum, derived from the ROM_Offset (0xFD) and Code_End (0xFE) footer
// fields just like MyBooty's Get_Checksum routine.
func binCodeEnd(bin []byte) (int64, error) {
	if len(bin) < 0x100 {
		return -1, errors.New("bin too small")
	}
	romOffsetStr := GetIdentifierFromFooter(bin, ROMoffset)
	codeEndStr := GetIdentifierFromFooter(bin, CodeEnd)
	if romOffsetStr == "" || codeEndStr == "" {
		return -1, errors.New("could not read ROM offset / code end from bin footer")
	}
	romOffset, err := strconv.ParseInt(romOffsetStr, 16, 64)
	if err != nil {
		return -1, fmt.Errorf("invalid ROM offset %q in footer: %w", romOffsetStr, err)
	}
	codeEnd, err := strconv.ParseInt(codeEndStr, 16, 64)
	if err != nil {
		return -1, fmt.Errorf("invalid code end %q in footer: %w", codeEndStr, err)
	}
	end := codeEnd - romOffset
	if codeEnd <= romOffset || end >= int64(len(bin))-4 {
		return -1, errors.New("code end outside of bin")
	}
	// MyBooty sums two bytes per loop iteration and only stops on an exact
	// address match: an odd byte count makes the ECU hang in the C8 command.
	if (end+1)%2 != 0 {
		return -1, errors.New("unaligned code end, ECU would hang calculating checksum")
	}
	return end, nil
}
