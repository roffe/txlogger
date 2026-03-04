package t8

import (
	"context"
	"errors"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/ecu/t8legion"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

func (t *Client) FlashECU(ctx context.Context, bin []byte) error {
	if len(bin) != 0x100000 {
		return errors.New("err: Invalid T8 file size")
	}

	if err := t.legion.Bootstrap(ctx, false); err != nil {
		return err
	}

	nvdm := fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsNvdm, false)
	boot := fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsBoot, false)

	fmask, err := t.legion.DeterminePartitionmask(ctx, bin, t8legion.EcuByte_T8, boot, nvdm, false)
	if err != nil {
		return err
	}

	err = t.legion.EraseFlash(ctx, t8legion.EcuByte_T8, fmask)
	if err != nil {
		return err
	}
	start := time.Now()
	err = t.legion.WriteFlash(ctx, t8legion.EcuByte_T8, 0x100000, bin, fmask)
	if err != nil {
		return err
	}
	t.cfg.OnMessage("Done, took: " + time.Since(start).String())

	status, err := t.legion.VerifyFlash(ctx, bin, t8legion.EcuByte_T8, fmask)
	if err != nil {
		return err
	}
	if !status {
		return errors.New("failed md5 verification")
	}
	t.cfg.OnMessage("Verifying md5: sucess")

	err = t.legion.MarryMCP(ctx)
	if err != nil {
		return err
	}

	return nil
}
