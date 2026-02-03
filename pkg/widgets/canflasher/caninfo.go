package canflasher

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/ecu"
)

func (t *CanFlasherWidget) ecuInfo() {
	// if !m.checkSelections() {
	// 	return
	// }

	dev, err := t.cfg.CSW.GetAdapter(t.ecuSelect.Selected)
	if err != nil {
		t.log(err.Error())
		return
	}

	go func() {

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fyne.Do(t.Disable)
		defer fyne.Do(t.Enable)

		c, err := gocan.NewWithOpts(ctx, dev)
		if err != nil {
			t.logValues.Append(err.Error())
			return
		}
		defer c.Close()

		tr, err := ecu.New(c, &ecu.Config{
			Name:       t.ecuSelect.Selected,
			OnProgress: t.progress,
			OnMessage:  func(s string) { t.logValues.Append(s) },
			OnError:    func(err error) { t.logValues.Append(err.Error()) },
		})
		if err != nil {
			t.log(err.Error())
			return
		}

		val, err := tr.Info(ctx)
		if err != nil {
			t.log(err.Error())
		}

		for _, v := range val {
			t.log(v.String())
		}

		if err := tr.ResetECU(ctx); err != nil {
			t.log(err.Error())
			return
		}
	}()
}
