package canflasher

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/native"
)

func (t *CanFlasherWidget) ecuFlash() {
	filename, err := native.OpenFileDialog("Bin file", native.FileFilter{
		Description: "Bin file",
		Extensions:  []string{"bin"},
	})
	if err != nil {
		t.log(err.Error())
		return
	}

	dev, err := t.cfg.CSW.GetAdapter(t.ecuSelect.Selected)
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

	done := make(chan struct{})

	go func() {
		for {
			select {
			case err := <-dev.Err():
				log.Println("Error:", err)
			case <-done:
				return
			}
		}
	}()

	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
		defer cancel()

		//defer dev.Close()

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
			OnMessage: func(s string) {
				t.logValues.Append(fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), s))
			},
			OnError: func(err error) {
				t.logValues.Append(fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), err.Error()))
			},
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

		t.app.SendNotification(fyne.NewNotification("txlogger", "ECU flash completed"))

		time.Sleep(200 * time.Millisecond)

		if err := tr.ResetECU(ctx); err != nil {
			t.log(err.Error())
		}
	}()
}
