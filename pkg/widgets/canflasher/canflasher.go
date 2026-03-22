package canflasher

import (
	"errors"
	"fmt"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	// Import ecu packages

	_ "github.com/roffe/txlogger/pkg/ecu/t5"
	_ "github.com/roffe/txlogger/pkg/ecu/t5legion"
	_ "github.com/roffe/txlogger/pkg/ecu/t7"
	_ "github.com/roffe/txlogger/pkg/ecu/t8"
	_ "github.com/roffe/txlogger/pkg/ecu/t8mcp"
	_ "github.com/roffe/txlogger/pkg/ecu/z22se"
	_ "github.com/roffe/txlogger/pkg/ecu/z22semcp"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/settings"
)

var _ fyne.Widget = (*CanFlasherWidget)(nil)

type CanFlasherWidget struct {
	widget.BaseWidget

	app fyne.App

	container *container.Split

	ecuSelect *widget.Select

	logList     *widget.List
	logValues   binding.StringList
	infoBTN     *widget.Button
	dumpBTN     *widget.Button
	flashBTN    *widget.Button
	marryBTN    *widget.Button
	bootBOX     *widget.Check
	nvdmBOX     *widget.Check
	pinEntry    *widget.Entry
	progressBar *widget.ProgressBar

	l binding.DataListener

	cfg *Config
}

type Config struct {
	CSW *settings.Widget
}

func New(cfg *Config) *CanFlasherWidget {
	t := &CanFlasherWidget{
		app:       fyne.CurrentApp(),
		logValues: binding.NewStringList(),
		cfg:       cfg,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *CanFlasherWidget) Disable() {
	t.infoBTN.Disable()
	t.dumpBTN.Disable()
	t.flashBTN.Disable()
	t.marryBTN.Disable()
	t.bootBOX.Disable()
	t.nvdmBOX.Disable()
	t.pinEntry.Disable()
}

func (t *CanFlasherWidget) Enable() {
	t.infoBTN.Enable()
	t.dumpBTN.Enable()
	t.flashBTN.Enable()
	t.marryBTN.Enable()
	t.bootBOX.Enable()
	t.nvdmBOX.Enable()
	t.pinEntry.Enable()
}

func (t *CanFlasherWidget) log(s string) {
	var text string
	if s != "" {
		text = fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), s)
	}
	//logData = append(logData, text)
	t.logValues.Append(text)
}

func (t *CanFlasherWidget) progress(v float64) {
	if v < 0 {
		t.progressBar.Max = math.Abs(v)
		fyne.Do(func() {
			t.progressBar.SetValue(0)
		})
		return
	}
	fyne.Do(func() {
		t.progressBar.SetValue(v)
	})
}

func (t *CanFlasherWidget) CreateRenderer() fyne.WidgetRenderer {
	t.ecuSelect = widget.NewSelect([]string{"Trionic 5", "Trionic 7", "Trionic 8", "Trionic 8 MCP", "Z22SE", "Z22SE MCP"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString("canflasher_ecu", s)
	})

	t.ecuSelect.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback("canflasher_ecu", "Trionic 5"))

	t.logList = widget.NewListWithData(
		t.logValues,
		func() fyne.CanvasObject {
			w := widget.NewLabel("")
			w.TextStyle.Monospace = true
			w.Selectable = true
			return w
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				panic(err)
			}
			obj.(*widget.Label).SetText(txt)
		},
	)

	t.l = binding.NewDataListener(func() {
		t.logList.ScrollToBottom()
	})

	t.logValues.AddListener(t.l)

	t.progressBar = widget.NewProgressBar()
	t.pinEntry = widget.NewEntry()
	t.pinEntry.Validator = func(s string) error {
		if len(s) != 4 {
			return errors.New("wrong PIN")
		}
		return nil
	}

	// t.wizzardBTN = widget.NewButton("Wizzard", nil) //t.wizzard)
	t.infoBTN = widget.NewButton("Info", t.ecuInfo) //t.ecuInfo)
	//t.dtcBTN = widget.NewButton("Read DTC", nil)   //t.readDTC)
	t.dumpBTN = widget.NewButton("Dump", func() {
		widgets.SaveFile(func(filename string) {
			t.ecuDump(filename)
		}, "Bin file", "bin")
	})
	//t.sramBTN = widget.NewButton("Dump SRAM", nil) //t.dumpSRAM)
	t.flashBTN = widget.NewButton("Flash", func() {
		if t.nvdmBOX.Checked {
			dialog.ShowConfirm("⚠️ Warning ⚠️", "Are you sure you want to overwrite keys and marriage status in the ECU?", func(confirm bool) {
				if !confirm {
					return
				}
				widgets.SelectFile(func(r fyne.URIReadCloser) {
					t.ecuFlash(r.URI().Path())
				}, "Bin file", "bin")
			}, fyne.CurrentApp().Driver().AllWindows()[0])
			return
		}

		widgets.SelectFile(func(r fyne.URIReadCloser) {
			t.ecuFlash(r.URI().Path())
		}, "Bin file", "bin")

	})
	t.marryBTN = widget.NewButton("MarryECM", func() {
		done := make(chan bool)
		d := dialog.NewConfirm("Confirmation", "You must do it with ignition ON. "+
			"This operation will erase NVDM flash, are you sure to continue ??", func(b bool) {
			done <- b
		}, fyne.CurrentApp().Driver().AllWindows()[0])
		d.Show()

		go func() {
			result := <-done
			if result {
				t.ecuMarry(t.pinEntry.Text)
			}
		}()
	})

	t.bootBOX = widget.NewCheck("Unlock boot partition", func(b bool) {
		if b {
			confirmFN := func(confirm bool) {
				if confirm {
					fyne.CurrentApp().Preferences().SetBool(settings.PrefsBoot, b)
					return
				}
				t.bootBOX.SetChecked(false)
			}

			dialog.ShowConfirm("⚠️ Warning ⚠️", "Boot is for advanced users only. If you don't know what it is, don't use it as it can possibly brick your ECU.", func(confirm bool) {
				confirmFN(confirm)
			}, fyne.CurrentApp().Driver().AllWindows()[0])
			return
		}
		fyne.CurrentApp().Preferences().SetBool(settings.PrefsBoot, b)
	})

	t.nvdmBOX = widget.NewCheck("Unlock systems partition", func(b bool) {
		if b {
			confirmFN := func(confirm bool) {
				if confirm {
					fyne.CurrentApp().Preferences().SetBool(settings.PrefsNvdm, b)
					return
				}
				t.nvdmBOX.SetChecked(false)
			}

			dialog.ShowConfirm("⚠️ Warning ⚠️", "This will overwrite any keys and marriage status in the ECU", func(confirm bool) {
				confirmFN(confirm)
			}, fyne.CurrentApp().Driver().AllWindows()[0])
			return
		}
		fyne.CurrentApp().Preferences().SetBool(settings.PrefsNvdm, b)
	})

	t.nvdmBOX.Checked = (fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsNvdm, false))
	t.bootBOX.Checked = fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsBoot, false)
	//t.bootBOX.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsBoot, false))
	// t.ecuList.PlaceHolder = "Select ECU"
	// t.adapterList.PlaceHolder = "Select Adapter"
	// t.portList.PlaceHolder = "Select Port"
	// t.speedList.PlaceHolder = "Select Speed"

	left := container.New(layout.NewStackLayout(), t.logList)
	right := container.NewVBox(
		t.ecuSelect,
		t.infoBTN,
		//t.dtcBTN,
		t.dumpBTN,
		//t.sramBTN,
		t.flashBTN,
		t.marryBTN,
		widget.NewLabel("Flash options:"),
		t.bootBOX,
		t.nvdmBOX,
		widget.NewLabel("PIN code:"),
		t.pinEntry,
	)

	split := container.NewHSplit(left, right)
	split.Offset = 0.8

	t.container = container.NewVSplit(split, t.progressBar)
	t.container.Offset = 1

	//return widget.NewSimpleRenderer(t.container)
	return &CanFlasherWidgetRenderer{
		t: t,
	}
}

type CanFlasherWidgetRenderer struct {
	t *CanFlasherWidget
}

func (tr *CanFlasherWidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
}

func (tr *CanFlasherWidgetRenderer) MinSize() fyne.Size {
	//return tr.t.container.MinSize()
	return fyne.NewSize(600, 450)
}

func (tr *CanFlasherWidgetRenderer) Refresh() {

}

func (tr *CanFlasherWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *CanFlasherWidgetRenderer) Destroy() {
	tr.t.logValues.RemoveListener(tr.t.l)
}
