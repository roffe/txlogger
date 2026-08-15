package canflasher

import (
	"context"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/ecu/t7"
	"github.com/roffe/txlogger/pkg/widgets"
)

// eolFlasher is implemented by Trionic 7 only; EOL programming is a T7 procedure
// and does not belong on the shared ecu.Client interface.
type eolFlasher interface {
	EOLFlash(context.Context, []byte, t7.EOLParams) error
}

// ecuEOL walks the full factory End-Of-Line procedure: pick a bin, confirm the
// three PI-area fields the tester owns, then erase + program + end-of-procedure
// in one unbroken session.
func (t *CanFlasherWidget) ecuEOL() {
	widgets.SelectFile(func(r fyne.URIReadCloser) {
		raw, err := os.ReadFile(r.URI().Path())
		if err != nil {
			t.log(err.Error())
			return
		}
		bin, err := t7.NormalizeBin(raw)
		if err != nil {
			t.log(err.Error())
			return
		}
		t.eolForm(bin)
	}, "Bin file", "bin")
}

func (t *CanFlasherWidget) eolForm(bin []byte) {
	win := fyne.CurrentApp().Driver().AllWindows()[0]
	info := t7.GetBinInfo(bin)

	vin := widget.NewEntry()
	vin.SetText(info.Vin)
	date := widget.NewEntry()
	date.SetText(time.Now().Format("060102"))
	serial := widget.NewEntry()
	serial.SetText(t7.DefaultTesterSerial)
	serial.SetPlaceHolder("was: " + info.Tester)

	items := []*widget.FormItem{
		widget.NewFormItem("VIN (0x90)", vin),
		widget.NewFormItem("Programming date (0x99)", date),
		widget.NewFormItem("Tester serial (0x98)", serial),
	}

	d := dialog.NewForm("EOL programming", "Continue", "Cancel", items, func(confirm bool) {
		if !confirm {
			return
		}
		params := t7.EOLParams{VIN: vin.Text, ProgDate: date.Text, TesterSerial: serial.Text}
		if err := params.Validate(); err != nil {
			dialog.ShowError(err, win)
			return
		}
		dialog.ShowConfirm("⚠️ EOL programming ⚠️",
			"This erases the whole flash and runs the factory EOL sequence.\n\n"+
				"Ignition must be ON, engine off, vehicle stationary.\n"+
				"Do NOT interrupt power — a break between erase and program bricks the ECU.\n\n"+
				"Continue?",
			func(ok bool) {
				if ok {
					t.runEOL(bin, params)
				}
			}, win)
	}, win)
	d.Resize(fyne.NewSize(480, 250))
	d.Show()
}

func (t *CanFlasherWidget) runEOL(bin []byte, params t7.EOLParams) {
	dev, err := t.cfg.CSW.GetAdapterWithOverrideFilters(t.ecuSelect.Selected, ecu.Filters(t.ecuSelect.Selected))
	if err != nil {
		t.log(err.Error())
		return
	}

	t.progressBar.SetValue(0)
	cfg := t.ecuConfig() // reads widget state; must happen on the main thread

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
		defer cancel()

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

		cl, err := ecu.New(c, cfg)
		if err != nil {
			t.log(err.Error())
			return
		}
		eol, ok := cl.(eolFlasher)
		if !ok {
			t.log("EOL programming is only supported on Trionic 7")
			return
		}

		if err := eol.EOLFlash(ctx, bin, params); err != nil {
			t.log(err.Error())
			return
		}
		t.log("EOL done, turn off the ignition or pull the fuse to finish")
	}()
}
