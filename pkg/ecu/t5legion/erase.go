package t5legion

import (
	"context"
	"fmt"
	"time"
)

func (t *Client) EraseECU(ctx context.Context) error {
	startTime := time.Now()
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return err
		}
	}
	t.cfg.OnProgress(-float64(100))
	t.cfg.OnMessage("Erasing FLASH...")

	cmd := []byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	resp, err := t.request(ctx, cmd, 20*time.Second)
	if err != nil {
		return err
	}
	if resp.Data[0] == 0xC0 && resp.Data[1] == 0x00 {
		t.cfg.OnMessage(fmt.Sprintf("FLASH erased, took: %s\n", time.Since(startTime).Round(time.Millisecond).String()))
		t.cfg.OnProgress(float64(100))
		return nil
	}
	return fmt.Errorf("erase FAILED: %X", resp.Data)
}
