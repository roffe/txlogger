package canflasher

import (
	"context"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

func (t *CanFlasherWidget) ecuDump(filename string) {
	/*
		filename, err := native.SaveFileDialog("Bin file", "bin", native.FileFilter{
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

	filename = addSuffix(filename, ".bin")
	t.progressBar.SetValue(0)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Second)
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

		bin, err := tr.DumpECU(ctx)
		if err != nil {
			t.log(err.Error())
			return
		}

		if err := os.WriteFile(filename, bin, 0o644); err == nil {
			t.log("Saved as " + filename)
		} else {
			t.log(err.Error())
			return
		}

		t.app.SendNotification(fyne.NewNotification("txlogger", "ECU download completed"))

		time.Sleep(200 * time.Millisecond)

		_ = tr.ResetECU(ctx)
	}()
}
