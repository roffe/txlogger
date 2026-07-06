package t5legion

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (t *Client) ResetECU(ctx context.Context) error {
	//if !t.bootloaded {
	//	if err := t.UploadBootLoader(ctx); err != nil {
	//		return err
	//	}
	//}
	//log.Println("Resetting ECU")
	resp, err := t.request(ctx, []byte{0x01, 0x20}, 150*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to reset ECU: %v", err)
	}
	if resp.Data[0] != 0x01 || resp.Data[1] != 0x50 && resp.Data[1] != 0x60 {
		return errors.New("invalid response to reset ECU")
	}
	t.cfg.OnMessage("ECU has been reset")
	return nil
}
