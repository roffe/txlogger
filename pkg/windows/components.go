package windows

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/roffe/t7logger/pkg/datalogger"
	"github.com/roffe/t7logger/pkg/widgets"
)

func (mw *MainWindow) newLogBtn() *widget.Button {
	var logBtn *widget.Button
	logBtn = widget.NewButtonWithIcon("Start logging", theme.DownloadIcon(), func() {
		if mw.loggingRunning {
			if mw.dlc != nil {
				mw.dlc.Close()
			}
			return
		}
		if !mw.loggingRunning {
			device, err := mw.canSettings.GetAdapter(mw.writeOutput)
			if err != nil {
				dialog.ShowError(err, mw)
				return
			}
			logBtn.SetText("Stop logging")
			mw.dlc = datalogger.New(datalogger.Config{
				Dev:            device,
				Variables:      mw.vars,
				Freq:           int(mw.freqSlider.Value),
				OnMessage:      mw.writeOutput,
				CaptureCounter: mw.captureCounter,
				ErrorCounter:   mw.errorCounter,
				Sink:           mw.sinkManager,
			})

			go func() {
				mw.mockBtn.Disable()
				defer mw.mockBtn.Enable()
				mw.loggingRunning = true
				mw.progressBar.Start()
				if err := mw.dlc.Start(); err != nil {
					dialog.ShowError(err, mw)
				}
				mw.progressBar.Stop()
				mw.loggingRunning = false
				mw.dlc = nil
				logBtn.SetText("Start logging")
			}()
		}
	})
	return logBtn
}

func (mw *MainWindow) newOutputList() *widget.List {
	list := widget.NewListWithData(
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
				mw.writeOutput(err.Error())
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)
	mw.symbolConfigList = widget.NewList(
		func() int {
			return mw.vars.Len()
		},
		func() fyne.CanvasObject {
			return widgets.NewVarDefinitionWidget(mw.symbolConfigList, mw.vars)
		},
		func(lii widget.ListItemID, co fyne.CanvasObject) {
			coo := co.(*widgets.VarDefinitionWidget)
			coo.Update(lii, mw.vars.GetPos(lii))
		},
	)
	return list
}

func (mw *MainWindow) newSymbolnameTypeahead() *xwidget.CompletionEntry {
	symbolLookup := xwidget.NewCompletionEntry([]string{})

	symbolLookup.PlaceHolder = "Type to search for symbols"

	// When the use typed text, complete the list.
	symbolLookup.OnChanged = func(s string) {
		// completion start for text length >= 3
		if len(s) < 3 {
			symbolLookup.HideCompletion()
			return
		}

		// Get the list of possible completion
		var results []string

		for _, sym := range mw.symbolMap {
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) {
				results = append(results, sym.Name)
			}
		}
		// no results
		if len(results) == 0 {
			symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		// then show them
		symbolLookup.SetOptions(results)
		symbolLookup.ShowCompletion()
	}

	if filename := mw.app.Preferences().String(prefsLastConfig); filename != "" {
		mw.LoadConfig(filename)
	}
	return symbolLookup
}
