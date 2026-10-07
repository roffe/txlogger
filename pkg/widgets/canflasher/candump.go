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
	dev, err := t.adapter()
	if err != nil {
		t.log(err.Error())
		return
	}

	filename = addSuffix(filename, ".bin")
	t.progressBar.SetValue(0)

	// read widget state on the main thread, before the worker starts
	cfg := t.ecuConfig()

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

		tr, err := ecu.New(c, cfg)
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

		time.Sleep(200 * time.Millisecond)
		if !ecu.Info(t.ecuSelect.Selected).ManualReset {
			_ = tr.ResetECU(ctx)
		}
	}()
}
