package windows

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"golang.org/x/net/context"
)

type T7Extras struct {
	widget.BaseWidget
	minsize fyne.Size
	mw      *MainWindow
}

func NewT7Extras(mw *MainWindow, minSize fyne.Size) *T7Extras {
	t := &T7Extras{
		minsize: minSize,
		mw:      mw,
	}
	t.ExtendBaseWidget(t)
	return t.render()
}

func (t *T7Extras) render() *T7Extras {
	return t
}

func (t *T7Extras) CreateRenderer() fyne.WidgetRenderer {
	con := container.NewVBox(
		widget.NewLabel("T7 stuff"),
		widget.NewButtonWithIcon("Reset ECU", theme.ViewRefreshIcon(), func() {
			dialog.ShowConfirm("Reset ECU", "Ignition OFF or the TB will go into limp mode", func(b bool) {
				if b {
					err := t.resetECU()
					if err != nil {
						dialog.ShowError(err, t.mw.Window)
						t.mw.Log(fmt.Sprintf("failed to reset ECU: %v", err))
					}
				}
			}, t.mw)
		}),
		container.NewBorder(
			nil,
			nil,
			widget.NewLabel("Set E85 %"),
			widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
			}),
			widget.NewEntry(),
		),
	)

	return &T7ExtrasRenderer{
		container: con,
		t:         t,
	}
}

func (t *T7Extras) resetECU() error {

	adapter, err := t.mw.settings.CanSettings.GetAdapter("T7", t.mw.Log)
	if err != nil {
		return err
	}
	ctx := context.Background()
	c, err := gocan.New(ctx, adapter)
	if err != nil {
		return err
	}
	defer c.Close()

	/* 	kwp := kwp2000.New(c)
	   	if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
	   		return err
	   	}
	   	granted, err := kwp.RequestSecurityAccess(ctx, false)
	   	if err != nil {
	   		return err
	   	}

	   	if !granted {
	   		return fmt.Errorf("security access denied")
	   	}

	   	time.Sleep(100 * time.Millisecond) */

	frame := gocan.NewFrame(0x240, []byte{0x40, 0xA1, 0x02, 0x11, 0x01}, gocan.ResponseRequired)
	f, err := c.SendAndPoll(ctx, frame, 400*time.Millisecond, 0x258)
	if err != nil {
		return err
	}
	d := f.Data()
	if d[3] == 0x7F {
		return fmt.Errorf("failed to reset ECU: %x", d[5])
	}
	if d[3] != 0x51 || d[4] != 0x81 {
		return fmt.Errorf("abnormal ecu reset response: %X", d[3:])
	}
	return nil
}

type T7ExtrasRenderer struct {
	t         *T7Extras
	container *fyne.Container
}

func (tr *T7ExtrasRenderer) Layout(space fyne.Size) {
	tr.container.Resize(space)
	tr.container.Move(fyne.NewPos(0, 0))
}

func (tr *T7ExtrasRenderer) MinSize() fyne.Size {
	return tr.t.minsize
}

func (tr *T7ExtrasRenderer) Refresh() {

}

func (tr *T7ExtrasRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.container}
}

func (tr *T7ExtrasRenderer) Destroy() {
}
