package t7

import (
	"context"
	"fmt"
	"time"
)

// eraseProgressSteps is the progress bar's denominator. A chip erase takes
// ~20 s of 250 ms polls, so this is an estimate, not a bound.
const eraseProgressSteps = 100

func (t *Client) EraseECU(ctx context.Context) error {
	t.cfg.OnProgress(-float64(eraseProgressSteps))
	t.cfg.OnMessage("Erasing FLASH")

	progress := 0
	start := time.Now()
	err := t.kwp.EraseFlash(ctx, func() {
		progress++
		t.cfg.OnProgress(float64(min(progress, eraseProgressSteps)))
	})
	if err != nil {
		return err
	}

	t.cfg.OnProgress(eraseProgressSteps)
	// The duration is worth showing: a chip erase is ~20 s, so anything much
	// faster means the ECU was still busy and the writes that follow will all
	// come back busyRepeatRequest.
	t.cfg.OnMessage(fmt.Sprintf("Erase done, took: %s", time.Since(start).Round(time.Second)))
	return nil
}
