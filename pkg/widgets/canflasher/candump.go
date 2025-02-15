package canflasher

import (
	"context"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
	"github.com/roffe/gocanflasher/pkg/ecu"
	sdialog "github.com/sqweek/dialog"
)

func (t *CanFlasherWidget) ecuDump() {
	filename, err := sdialog.File().Filter("Bin file", "bin").Title("Save bin file").Save()
	if err != nil {
		t.log(err.Error())
		return
	}

	dev, err := t.cfg.CSW.GetAdapter(t.cfg.GetECU(), t.log)
	if err != nil {
		t.log(err.Error())
		return
	}

	filename = addSuffix(filename, ".bin")
	t.progressBar.SetValue(0)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
		defer cancel()

		defer dev.Close()

		fyne.Do(t.Disable)
		defer fyne.Do(t.Enable)

		c, err := gocan.NewClient(ctx, dev)
		if err != nil {
			t.logValues.Append(err.Error())
			return
		}

		tr, err := ecu.New(c, &ecu.Config{
			Name:       translateName(t.cfg.GetECU()),
			OnProgress: t.progress,
			OnMessage:  func(s string) { t.logValues.Append(s) },
			OnError:    func(err error) { t.logValues.Append(err.Error()) },
		})
		if err != nil {
			t.log(err.Error())
			return
		}

		bin, err := tr.DumpECU(ctx)
		if err == nil {
			t.app.SendNotification(fyne.NewNotification("", "Dump done"))
			if err := os.WriteFile(filename, bin, 0644); err == nil {
				t.log("Saved as " + filename)
			} else {
				t.log(err.Error())
			}
		} else {
			t.log(err.Error())
		}

		if err := tr.ResetECU(ctx); err != nil {
			t.log(err.Error())
		}
	}()
}
