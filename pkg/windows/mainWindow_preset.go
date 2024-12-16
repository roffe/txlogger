package windows

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/presets"
	sdialog "github.com/sqweek/dialog"
)

func (mw *MainWindow) reloadPresets() {
	sets := append([]string{"Select preset"}, presets.Names()...)
	mw.selects.presetSelect.SetOptions(sets)
}

func (mw *MainWindow) savePreset() {
	if mw.selects.presetSelect.Selected == "Select preset" {
		//dialog.ShowInformation("No preset selected", "Use 'New' to create a new preset", mw)
		mw.newPreset()
		return
	}
	if err := presets.Set(mw.selects.presetSelect.Selected, mw.symbolList.Symbols()); err != nil {
		mw.Error(err)
		return
	}
	if err := presets.Save(mw.app); err != nil {
		mw.Error(err)
		return
	}
}

func (mw *MainWindow) newPreset() {
	presetName := widget.NewEntry()
	dialog.NewForm("Create new preset         ", "Create", "Cancel", []*widget.FormItem{
		widget.NewFormItem("name", presetName),
	},
		func(create bool) {
			if create {
				if presetName.Text == "" {
					mw.Error(fmt.Errorf("name can't be empty"))
					return
				}
				if err := presets.Set(presetName.Text, mw.symbolList.Symbols()); err != nil {
					mw.Error(err)
					return
				}
				if err := presets.Save(mw.app); err != nil {
					mw.Error(err)
					return
				}
				mw.reloadPresets()
				mw.selects.presetSelect.SetSelected(presetName.Text)
			}
		},
		mw,
	).Show()
	mw.Window.Canvas().Focus(presetName)
}

func (mw *MainWindow) importPreset() {
	filename, err := sdialog.File().Filter("Preset file", "txp").Load()
	if err != nil {
		if err.Error() == "Cancelled" {
			return
		}
		mw.Error(err)
		return
	}
	if err := mw.LoadPreset(filename); err != nil {
		mw.Error(err)
		return
	}
	mw.SyncSymbols()
}

func (mw *MainWindow) exportPreset() {
	filename, err := sdialog.File().Filter("Preset file", "txp").Save()
	if err != nil {
		if err.Error() == "Cancelled" {
			return
		}
		mw.Error(err)
		return
	}
	if !strings.HasSuffix(filename, ".txp") {
		filename += ".txp"
	}
	if err := mw.SavePreset(filename); err != nil {
		mw.Error(err)
		return
	}
}

func (mw *MainWindow) deletePreset() {
	if mw.selects.presetSelect.Selected == "Select preset" {
		dialog.ShowInformation("No preset selected", "Select a preset to delete", mw)
		return
	}

	if strings.EqualFold(mw.selects.presetSelect.Selected, "T7 Dash") || strings.EqualFold(mw.selects.presetSelect.Selected, "T8 Dash") {
		mw.Error(fmt.Errorf("can't delete built-in preset"))
		return
	}

	dialog.ShowConfirm("Confirm preset delete", "Delete preset '"+mw.selects.presetSelect.Selected+"', are you sure?", func(b bool) {
		if b {
			if err := presets.Delete(mw.selects.presetSelect.Selected); err != nil {
				mw.Error(err)
				return
			}
			if err := presets.Save(mw.app); err != nil {
				mw.Error(err)
				return
			}
			mw.reloadPresets()
			mw.selects.presetSelect.SetSelected("Select preset")
		}
	}, mw)
}
