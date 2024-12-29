package windows

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2/theme"
	xwidget "fyne.io/x/fyne/widget"
)

// list .json files in the folder layouts
func listLayouts() ([]string, error) {
	opts := []string{"Save Layout"}
	files, err := os.ReadDir("layouts")
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read layouts folder: %w", err)
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		opts = append(opts, strings.TrimSuffix(f.Name(), ".json"))
	}
	return opts, nil
}

func (mw *MainWindow) openSettings() {
	if mw.wm.HasWindow("Settings") {
		return
	}
	inner := newInnerWindow("Settings", mw.settings)
	inner.Icon = theme.SettingsIcon()
	mw.wm.Add(inner)
}

func (mw *MainWindow) loadPrefs(filename string) {
	if ecu := mw.app.Preferences().StringWithFallback(prefsSelectedECU, "T7"); ecu != "" {
		mw.selects.ecuSelect.SetSelected(ecu)
	}

	if preset := mw.app.Preferences().String(prefsSelectedPreset); preset != "" {
		mw.selects.presetSelect.SetSelected(preset)
	}

	if filename == "" {
		if filename := mw.app.Preferences().String(prefsLastBinFile); filename != "" {
			if err := mw.LoadSymbolsFromFile(filename); err != nil {
				mw.Error(err)
				return
			}
			mw.filename = filename
			return
		}
	} else {
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			mw.Error(err)
			return
		}
		mw.filename = filename
	}

}

func (mw *MainWindow) newSymbolnameTypeahead() {
	mw.symbolLookup = xwidget.NewCompletionEntry([]string{})
	mw.symbolLookup.PlaceHolder = "Search for symbol"
	mw.symbolLookup.OnChanged = func(s string) {
		if mw.fw == nil {
			return
		}
		// completion start for text length >= 3
		if len(s) < 3 {
			mw.symbolLookup.HideCompletion()
			return
		}
		// Get the list of possible completion
		var results []string
		for _, sym := range mw.fw.Symbols() {
			if sym.Length > 8 {
				continue
			}

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
