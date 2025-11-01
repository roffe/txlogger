package windows

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/ebus"
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

	mw.selects.ecuSelect = widget.NewSelect(common.EcuList, func(s string) {
		mw.app.Preferences().SetString(prefsSelectedECU, s)
		idx := symbol.ECUTypeFromString(s)
		ebus.Publish(ebus.TOPIC_ECU, float64(idx))
		mw.SetMainMenu(mw.menu.GetMenu(s))
		pres := mw.app.Preferences().StringWithFallback(s+prefsSelectedPreset, s+" Dash")
		mw.selects.presetSelect.SetSelected(pres)
	})

	mw.selects.presetSelect = widget.NewSelect(append([]string{"Select preset"}, presets.Names()...), func(presetName string) {
		if presetName == "Select preset" {
			return
		}
		preset, err := presets.Get(presetName)
		if err != nil {
			mw.Error(err)
			return
		}
		mw.symbolList.LoadSymbols(preset...)
		mw.SyncSymbols()
		ecu := mw.app.Preferences().String(prefsSelectedECU)
		mw.app.Preferences().SetString(ecu+prefsSelectedPreset, presetName)
	})
	mw.selects.presetSelect.Alignment = fyne.TextAlignLeading
	mw.selects.presetSelect.PlaceHolder = "Select preset"
}
