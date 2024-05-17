package windows

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

type MyrtilosRegistration struct {
	fyne.Window
	app   fyne.App
	mw    *MainWindow
	input *widget.Entry
	text  *widget.Label
}

func NewMyrtilosRegistration(app fyne.App, mw *MainWindow) *MyrtilosRegistration {
	mr := &MyrtilosRegistration{
		Window: app.NewWindow("EU0D registration"),
		app:    app,
		mw:     mw,
		input:  widget.NewEntry(),
		text:   widget.NewLabel("Enter security key"),
	}

	mr.input.MultiLine = false
	mr.input.Validator = func(s string) error {
		s = strings.ReplaceAll(s, " ", "")
		if len(s) != 8 {
			return errors.New("invalid security key length")
		}
		_, err := hex.DecodeString(s)
		if err != nil {
			return errors.New("invalid security key")
		}
		return nil
	}

	mr.Resize(fyne.NewSize(500, 30))
	mr.SetContent(mr.render())
	return mr
}

func (mr *MyrtilosRegistration) render() *fyne.Container {
	return container.NewBorder(
		mr.text,
		widget.NewButtonWithIcon("Register", theme.InfoIcon(), func() {
			key, err := hex.DecodeString(mr.input.Text)
			if err != nil {
				mr.text.SetText(err.Error())
				return
			}
			if err := mr.register(key); err != nil {
				mr.text.SetText(err.Error())
			} else {
				mr.text.SetText("key saved in ecu")
			}
		}),
		nil,
		nil,
		mr.input,
	)
}

func (mr *MyrtilosRegistration) register(key []byte) error {
	if len(key) != 4 {
		return errors.New("invalid key length")
	}
	if mr.mw.dlc != nil {
		return errors.New("stop logging before registering")
	}
	adapter, err := mr.mw.settings.CanSettings.GetAdapter("T7", mr.mw.Log)
	if err != nil {
		return err
	}
	ctx := context.Background()
	c, err := gocan.New(ctx, adapter)
	if err != nil {
		return err
	}
	defer c.Close()
	kwp := kwp2000.New(c)

	log.Println("Starting session")
	if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
		return err
	}
	defer func() {
		log.Println("Stopping session")
		kwp.StopSession(ctx)
		time.Sleep(50 * time.Millisecond)
	}()

	gotIt, err := kwp.RequestSecurityAccess(ctx, false)
	if err != nil {
		return err
	}

	if gotIt {
		log.Println("Got security access")
	} else {
		return errors.New("didn't get security access")
	}

	if err := kwp.SendEU0DRegistrationKey(ctx, key); err != nil {
		return err
	}
	return nil
}
