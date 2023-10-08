package windows

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/roffe/txlogger/pkg/datalogger"
)

func (mw *MainWindow) newLogBtn() {
	mw.logBtn = widget.NewButtonWithIcon("Start logging", theme.MediaPlayIcon(), func() {
		if mw.loggingRunning {
			if mw.dlc != nil {
				mw.dlc.Close()
			}
			return
		}
		if !mw.loggingRunning {
			device, err := mw.canSettings.GetAdapter(mw.ecuSelect.Selected, mw.Log)
			if err != nil {
				mw.Log(err.Error())
				return
			}
			mw.dlc, err = datalogger.New(datalogger.Config{
				ECU:                   mw.ecuSelect.Selected,
				Dev:                   device,
				Variables:             mw.vars.Get(),
				Freq:                  int(mw.freqSlider.Value),
				OnMessage:             mw.Log,
				CaptureCounter:        mw.captureCounter,
				ErrorCounter:          mw.errorCounter,
				ErrorPerSecondCounter: mw.errorPerSecondCounter,
			})
			if err != nil {
				mw.Log(err.Error())
				return
			}
			go func() {
				mw.loggingRunning = true
				mw.logBtn.SetIcon(theme.MediaStopIcon())
				mw.logBtn.SetText("Stop logging")
				mw.disableBtns()
				defer mw.enableBtns()

				if mw.dashboard != nil {
					mw.dlc.AttachDashboard(mw.dashboard)
				}

				if err := mw.dlc.Start(); err != nil {
					mw.Log(err.Error())
				}

				if mw.dashboard != nil {
					mw.dlc.DetachDashboard(mw.dashboard)
				}

				mw.loggingRunning = false
				mw.dlc = nil
				mw.logBtn.SetIcon(theme.MediaPlayIcon())
				mw.logBtn.SetText("Start logging")
			}()
		}
	})
}

func (mw *MainWindow) newOutputList() {
	mw.output = widget.NewListWithData(
		mw.outputData,
		func() fyne.CanvasObject {
			return &widget.Label{
				Alignment: fyne.TextAlignLeading,
				Wrapping:  fyne.TextWrapBreak,
				TextStyle: fyne.TextStyle{Monospace: true},
			}
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				mw.Log(err.Error())
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)
}

func (mw *MainWindow) newSymbolnameTypeahead() {
	mw.symbolLookup = xwidget.NewCompletionEntry([]string{})
	mw.symbolLookup.PlaceHolder = "Type to search for symbols"
	// When the use typed text, complete the list.
	mw.symbolLookup.OnChanged = func(s string) {
		// completion start for text length >= 3
		if len(s) < 3 && s != "*" {
			mw.symbolLookup.HideCompletion()
			return
		}
		// Get the list of possible completion
		var results []string
		for _, sym := range mw.symbolMap {
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) || s == "*" {
				results = append(results, sym.Name)
			}
		}
		// no results
		if len(results) == 0 {
			mw.symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		// then show them
		if len(results) > 0 {
			mw.symbolLookup.SetOptions(results)
			mw.symbolLookup.ShowCompletion()
		}
	}
}
