package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/presets"
)

func (mw *MainWindow) createSelects() {
	mw.selects.layoutSelect = widget.NewSelect(listLayouts(), func(s string) {
		if s == "" {
			return
		}
		switch s {
		case "Save Layout":
			if err := mw.SaveLayout(); err != nil {
				mw.Error(err)
			}
			mw.selects.layoutSelect.ClearSelected()
			mw.buttons.layoutRefreshBtn.Tapped(&fyne.PointEvent{})
		default:
			if err := mw.LoadLayout(s); err != nil {
				mw.Error(err)
			}
		}
	})

	mw.selects.presetSelect = widget.NewSelect(append([]string{"Select preset"}, presets.Names()...), func(s string) {
		if s == "Select preset" {
			return
		}
		preset, err := presets.Get(s)
		if err != nil {
			mw.Error(err)
			return
		}
		mw.symbolList.LoadSymbols(preset...)
		mw.SyncSymbols()
		mw.app.Preferences().SetString(prefsSelectedPreset, s)
	})
	mw.selects.presetSelect.Alignment = fyne.TextAlignLeading
	mw.selects.presetSelect.PlaceHolder = "Select preset"

	mw.selects.ecuSelect = widget.NewSelect([]string{"T5", "T7", "T8"}, func(s string) {
		mw.app.Preferences().SetString(prefsSelectedECU, s)
		mw.SetMainMenu(mw.menu.GetMenu(s))
	})
}
