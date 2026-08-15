package canflasher

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

func (t *CanFlasherWidget) ecuReset() {
	dev, err := t.cfg.CSW.GetAdapterWithOverrideFilters(t.ecuSelect.Selected, ecu.Filters(t.ecuSelect.Selected))
	if err != nil {
		t.log(err.Error())
		return
	}

	// read widget state on the main thread, before the worker starts
	cfg := t.ecuConfig()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fyne.Do(t.Disable)
		defer fyne.Do(t.Enable)

		c, err := gocan.OpenAdapter(ctx, dev)
		if err != nil {
			t.log(err.Error())
			return
		}
		defer c.Close()

		tr, err := ecu.New(c, cfg)
		if err != nil {
			t.log(err.Error())
			return
		}

		if err := tr.ResetECU(ctx); err != nil {
			t.log(err.Error())
			return
		}
		t.log("ECU reset")
	}()
}
