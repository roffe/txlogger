package canflasher

import (
	"context"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

func (t *CanFlasherWidget) ecuFlash(filename string) {
	/*
		filename, err := native.OpenFileDialog("Bin file", native.FileFilter{
			Description: "Bin file",
			Extensions:  []string{"bin"},
		})
		if err != nil {
			t.log(err.Error())
			return
		}
	*/
	dev, err := t.cfg.CSW.GetAdapterWithOverrideFilters(t.ecuSelect.Selected, ecu.Filters(t.ecuSelect.Selected))
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

		tr, err := ecu.New(c, &ecu.Config{
			Name:       t.ecuSelect.Selected,
			OnProgress: t.progress,
			OnMessage:  t.log,
			OnError:    func(err error) { t.log(err.Error()) },
		})
		if err != nil {
			t.log(err.Error())
			return
		}

		err = tr.FlashECU(ctx, bin)
		if err != nil {
			t.log(err.Error())
			return
		}

		// Show what the ECU reports after flashing as confirmation the
		// right bin landed (the bootloader is still running at this point).
		if res, err := tr.Info(ctx); err == nil {
			for _, r := range res {
				t.log(r.String())
			}
		} else {
			t.log(err.Error())
		}

		t.app.SendNotification(fyne.NewNotification("txlogger", "ECU flash completed"))

		time.Sleep(200 * time.Millisecond)

		if err := tr.ResetECU(ctx); err != nil {
			t.log(err.Error())
		}
	}()
}
