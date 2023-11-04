package windows

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
)

func (mw *MainWindow) setTitle(str string) {
	meta := mw.app.Metadata()
	mw.SetTitle(fmt.Sprintf("txlogger v%s Build %d - %s", meta.Version, meta.Build, str))
}

func (mw *MainWindow) loadPrefs(filename string) {
	if cfg := mw.app.Preferences().String(prefsSymbolList); cfg != "" {
		mw.LoadConfigFromString(cfg)
	}

	if ecu := mw.app.Preferences().StringWithFallback(prefsSelectedECU, "T7"); ecu != "" {
		mw.ecuSelect.SetSelected(ecu)
	}
	if filename == "" {
		if filename := mw.app.Preferences().String(prefsLastBinFile); filename != "" {
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Log(err.Error())
			}
		}
	} else {
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			mw.Log(err.Error())
		}
	}
}

func (mw *MainWindow) newOutputList() {
	mw.output = widget.NewListWithData(
		mw.outputData,
		func() fyne.CanvasObject {
			return &widget.Label{
				Alignment: fyne.TextAlignLeading,
				TextStyle: fyne.TextStyle{Monospace: true},
				//Wrapping:   fyne.TextWrapBreak,
				Truncation: fyne.TextTruncateEllipsis,
			}
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				mw.Log(err.Error())
				return
			}

			//l := obj.(*container.Scroll)
			//l.Content.(*widget.Label).SetText(txt)

			obj.(*widget.Label).SetText(txt)

		},
	)
}

func (mw *MainWindow) newSymbolnameTypeahead() {
	mw.symbolLookup = xwidget.NewCompletionEntry([]string{})
	mw.symbolLookup.PlaceHolder = "Search for symbol"
	mw.symbolLookup.OnChanged = func(s string) {
		if mw.symbols == nil {
			return
		}
		// completion start for text length >= 3
		if len(s) < 3 {
			mw.symbolLookup.HideCompletion()
			return
		}
		// Get the list of possible completion
		var results []string
		for _, sym := range mw.symbols.Symbols() {
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) {
				results = append(results, sym.Name)
			}
		}
		// no results
		if len(results) == 0 {
			mw.symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		// Show results
		if len(results) > 0 {
			mw.symbolLookup.SetOptions(results)
			mw.symbolLookup.ShowCompletion()
		}
	}
}
