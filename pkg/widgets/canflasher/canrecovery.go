package canflasher

import (
	"context"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

func (t *CanFlasherWidget) ecuRecover(filename string) {
	dev, err := t.cfg.CSW.GetAdapterWithExtraFilters(t.ecuSelect.Selected, []uint32{0x011, 0x311}, true)
	if err != nil {
		t.log(err.Error())
		return
	}

	bin, err := os.ReadFile(filename)
	if err != nil {
		t.log(err.Error())
		return
	}

	t.progressBar.SetValue(0)

	// read widget state on the main thread, before the worker starts
	cfg := t.ecuConfig()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
		defer cancel()

		// defer dev.Close()

		fyne.Do(t.Disable)
		defer fyne.Do(t.Enable)

		c, err := gocan.OpenAdapter(ctx, dev, gocan.WithEventFunc(func(e gocan.Event) {
			t.log(e.String())
		}))
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

		err = tr.RecoverECU(ctx, bin)
		if err != nil {
			t.log(err.Error())
			return
		}

		t.app.SendNotification(fyne.NewNotification("txlogger", "ECU flash completed"))

		time.Sleep(200 * time.Millisecond)

		if err := tr.ResetECU(ctx); err != nil {
			t.log(err.Error())
		}
	}()
}
