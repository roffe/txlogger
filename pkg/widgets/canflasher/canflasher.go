package canflasher

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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

	logText     *widget.Label
	logScroll   *container.Scroll
	infoBTN     *widget.Button
	dumpBTN     *widget.Button
	flashBTN    *widget.Button
	eolBTN      *widget.Button
	recoveryBTN *widget.Button
	marryBTN    *widget.Button
	resetBTN    *widget.Button
	bootBOX     *widget.Check
	nvdmBOX     *widget.Check
	pinEntry    *widget.Entry
	progressBar *widget.ProgressBar
	flashLabel  *widget.Label
	pinLabel    *widget.Label

	seedKeyLabel *widget.Label
	seedKeyXOR   *widget.Entry
	seedKeySub   *widget.Entry
	seedKeyBox   *fyne.Container

	cfg *Config
}

type Config struct {
	CSW *settings.Widget
}

func New(cfg *Config) *CanFlasherWidget {
	t := &CanFlasherWidget{
		app: fyne.CurrentApp(),
		cfg: cfg,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *CanFlasherWidget) Disable() {
	t.infoBTN.Disable()
	t.dumpBTN.Disable()
	t.flashBTN.Disable()
	t.eolBTN.Disable()
	t.marryBTN.Disable()
	t.recoveryBTN.Disable()
	t.resetBTN.Disable()
	t.bootBOX.Disable()
	t.nvdmBOX.Disable()
	t.pinEntry.Disable()
	t.seedKeyXOR.Disable()
	t.seedKeySub.Disable()
}

func (t *CanFlasherWidget) Enable() {
	t.infoBTN.Enable()
	t.dumpBTN.Enable()
	t.flashBTN.Enable()
	t.eolBTN.Enable()
	t.marryBTN.Enable()
	t.recoveryBTN.Enable()
	t.resetBTN.Enable()
	t.bootBOX.Enable()
	t.nvdmBOX.Enable()
	t.pinEntry.Enable()
	t.seedKeyXOR.Enable()
	t.seedKeySub.Enable()
}

func (t *CanFlasherWidget) log(s string) {
	text := "\n"
	if s != "" {
		text = fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), s)
	}
	fyne.Do(func() {
		// ponytail: O(n²) string append, fine for flash-session log volumes
		t.logText.SetText(t.logText.Text + text)
		t.logScroll.ScrollToBottom()
	})
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
	t.ecuSelect = widget.NewSelect([]string{"Trionic 5", "Trionic 7", "Trionic 8", "Trionic 8 MCP", "Z22SE", "Z22SE MCP"}, nil)

	t.logText = widget.NewLabel("")
	t.logText.TextStyle.Monospace = true
	t.logText.Selectable = true
	t.logScroll = container.NewScroll(t.logText)

	t.progressBar = widget.NewProgressBar()
	t.pinEntry = widget.NewEntry()
	t.pinEntry.Validator = func(s string) error {
		if len(s) != 4 {
			return errors.New("wrong PIN")
		}
		return nil
	}

	// t.wizzardBTN = widget.NewButton("Wizzard", nil) //t.wizzard)
	t.infoBTN = widget.NewButton("Info", t.ecuInfo) // t.ecuInfo)
	// t.dtcBTN = widget.NewButton("Read DTC", nil)   //t.readDTC)
	t.dumpBTN = widget.NewButton("Dump", func() {
		widgets.SaveFile(func(filename string) {
			t.ecuDump(filename)
		}, "Bin file", "bin")
	})
	// t.sramBTN = widget.NewButton("Dump SRAM", nil) //t.dumpSRAM)
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
	// EOL programming: the full factory sequence (T7 only). Unlike Flash it also
	// writes VIN/date/tester serial and runs end-of-procedure, which verifies the
	// ROM checksum and sets the EOL-success flag.
	t.eolBTN = widget.NewButton("EOL Flash", t.ecuEOL)

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

	t.recoveryBTN = widget.NewButton("Recovery", func() {
		widgets.SelectFile(func(r fyne.URIReadCloser) {
			t.ecuRecover(r.URI().Path())
		}, "Bin file", "bin")
	})

	t.resetBTN = widget.NewButton("Reset ECU", func() {
		if t.ecuSelect.Selected != "Trionic 7" {
			t.ecuReset()
			return
		}
		dialog.ShowConfirm("⚠️ Warning ⚠️", "TURN OFF THE IGNITION!!\nIf the ignition is ON, the throttle body will go into limp mode.\n\n"+
			"Continue?", func(confirm bool) {
			if confirm {
				t.ecuReset()
			}
		}, fyne.CurrentApp().Driver().AllWindows()[0])
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
	// t.bootBOX.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(settings.PrefsBoot, false))
	// t.ecuList.PlaceHolder = "Select ECU"
	// t.adapterList.PlaceHolder = "Select Adapter"
	// t.portList.PlaceHolder = "Select Port"
	// t.speedList.PlaceHolder = "Select Speed"
	// Custom SecurityAccess algorithm for T7 ECUs flashed with a patched
	// seed/key routine, which answers to none of the known-good pairs. Read the
	// values out of the ECU's binary with Tools > T7 Seed/Key patcher.
	t.seedKeyLabel = widget.NewLabel("Seed/key XOR & SUB (hex, optional):")
	t.seedKeyXOR = widget.NewEntry()
	t.seedKeyXOR.SetPlaceHolder("XOR")
	t.seedKeySub = widget.NewEntry()
	t.seedKeySub.SetPlaceHolder("SUB")
	for _, e := range []*widget.Entry{t.seedKeyXOR, t.seedKeySub} {
		e.Validator = func(s string) error {
			if strings.TrimSpace(s) == "" {
				return nil // empty = use the known-good pairs
			}
			_, err := parseHex16(s)
			return err
		}
	}
	t.seedKeyXOR.SetText(t.app.Preferences().String("canflasher_t7_xor"))
	t.seedKeySub.SetText(t.app.Preferences().String("canflasher_t7_sub"))
	t.seedKeyXOR.OnChanged = func(s string) { t.app.Preferences().SetString("canflasher_t7_xor", s) }
	t.seedKeySub.OnChanged = func(s string) { t.app.Preferences().SetString("canflasher_t7_sub", s) }
	t.seedKeyBox = container.NewGridWithColumns(2, t.seedKeyXOR, t.seedKeySub)

	t.flashLabel = widget.NewLabel("Flash options:")
	t.pinLabel = widget.NewLabel("PIN code:")
	left := t.logScroll
	right := container.NewVBox(
		t.ecuSelect,
		t.infoBTN,
		// t.dtcBTN,
		t.dumpBTN,
		// t.sramBTN,
		t.flashBTN,
		t.eolBTN,
		t.marryBTN,
		t.recoveryBTN,
		t.resetBTN,
		t.flashLabel,
		t.bootBOX,
		t.nvdmBOX,
		t.pinLabel,
		t.pinEntry,
		t.seedKeyLabel,
		t.seedKeyBox,
	)

	split := container.NewHSplit(left, right)
	split.Offset = 0.8

	t.container = container.NewVSplit(split, t.progressBar)
	t.container.Offset = 1

	t.ecuSelect.OnChanged = func(s string) {
		t.app.Preferences().SetString("canflasher_ecu", s)
		showIf(s == "Trionic 8", t.marryBTN, t.recoveryBTN, t.bootBOX, t.nvdmBOX, t.pinEntry, t.pinLabel, t.flashLabel)
		showIf(s == "Trionic 7", t.seedKeyLabel, t.seedKeyBox, t.eolBTN)
	}

	t.ecuSelect.SetSelected(t.app.Preferences().StringWithFallback("canflasher_ecu", "Trionic 5"))

	// return widget.NewSimpleRenderer(t.container)
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
	// return tr.t.container.MinSize()
	return fyne.NewSize(600, 450)
}

func (tr *CanFlasherWidgetRenderer) Refresh() {
}

func (tr *CanFlasherWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *CanFlasherWidgetRenderer) Destroy() {}
