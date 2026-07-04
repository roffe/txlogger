package t5

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

// Erase failure codes as documented in MyBooty.asm. For codes 01 and 02 the
// reply also carries the address that failed in bytes 2-5.
var eraseErrors = map[byte]string{
	0x01: "unable to erase FLASH chips",
	0x02: "cannot write zeroes to 28F512/010 chips",
	0x03: "unrecognised FLASH chips, unknown make",
	0x04: "unrecognised Intel FLASH chips",
	0x05: "unrecognised AMD FLASH chips",
	0x06: "unrecognised CSI/Catalyst FLASH chips",
	0x07: "unrecognised Atmel FLASH chips",
	0x08: "unrecognised Microchip/SST FLASH chips",
	0x09: "unrecognised ST FLASH chips",
	0x0A: "unrecognised AMIC FLASH chips",
}

func (t *Client) EraseECU(ctx context.Context) error {
	startTime := time.Now()
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return err
		}
	}
	t.cfg.OnProgress(-float64(100))
	t.cfg.OnMessage("Erasing FLASH...")

	// Flash content changes no matter how this ends.
	t.invalidateFooter()

	data, err := t.command(ctx, []byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, eraseTimeout)
	if err != nil {
		return fmt.Errorf("erase failed: %w", err)
	}
	if data[1] == 0x00 {
		t.cfg.OnMessage(fmt.Sprintf("FLASH erased, took: %s", time.Since(startTime).Round(time.Millisecond).String()))
		t.cfg.OnProgress(float64(100))
		return nil
	}

	if desc, ok := eraseErrors[data[1]]; ok {
		if data[1] <= 0x02 {
			return fmt.Errorf("erase failed: %s (address 0x%06X)", desc, binary.BigEndian.Uint32(data[2:6]))
		}
		return fmt.Errorf("erase failed: %s", desc)
	}
	return fmt.Errorf("erase failed: %X", data)
}
