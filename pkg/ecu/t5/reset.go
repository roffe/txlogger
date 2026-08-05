package t5

import (
	"context"
	"fmt"
)

func (t *Client) ResetECU(ctx context.Context) error {
	data, err := t.command(ctx, []byte{0xC2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, generalTimeout)
	if err != nil {
		return fmt.Errorf("failed to reset ECU: %w", err)
	}
	if data[1] != 0x00 || data[2] != 0x08 {
		return fmt.Errorf("invalid response to reset ECU: %X", data)
	}
	// The bootloader is gone after a restart.
	t.bootloaded = false
	t.cfg.OnMessage("ECU has been reset")
	return nil
}
