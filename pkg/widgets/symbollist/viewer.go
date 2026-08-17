package symbollist

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/presets"
	txtheme "github.com/roffe/txlogger/pkg/theme"
	"github.com/roffe/txlogger/pkg/widgets"
)

const (
	prefsSelectedPreset = "selectedPreset"
	prefsSymbolList     = "symbolList"
	noPreset            = "Select preset"
)

// ViewerConfig wires the Viewer to its surroundings.
type ViewerConfig struct {
	App    fyne.App
	Window fyne.Window   // parent for dialogs
	ECU    func() string // currently selected ECU, used for per-ECU preset prefs
	Log    func(string)
	Error  func(error)
}

// Viewer is the complete symbol list UI: symbol search/add/sync on top,
// the list in the middle and preset handling at the bottom.
type Viewer struct {
	widget.BaseWidget
	cfg *ViewerConfig

	list         *Widget
	symbolLookup *widgets.CompletionEntry
	presetSelect *widget.Select
	addBtn       *widget.Button
	syncBtn      *widget.Button

	fw symbol.FirmwareFile

	content fyne.CanvasObject
}

func NewViewer(cfg *ViewerConfig) *Viewer {
	v := &Viewer{
		cfg:  cfg,
		list: New(&Config{ColorBlindMode: colors.ModeNormal}),
	}
	v.ExtendBaseWidget(v)

	v.newSymbolnameTypeahead()

	v.addBtn = widget.NewButtonWithIcon("", theme.ContentAddIcon(), v.addSymbol)
	v.syncBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), v.Sync)

	v.presetSelect = widget.NewSelect(append([]string{noPreset}, presets.Names()...), func(presetName string) {
		if presetName == noPreset {
			return
		}
		preset, err := presets.Get(presetName)
		if err != nil {
			cfg.Error(err)
			return
		}
		v.list.LoadSymbols(preset...)
		v.Sync()
		cfg.App.Preferences().SetString(cfg.ECU()+prefsSelectedPreset, presetName)
	})
	v.presetSelect.Alignment = fyne.TextAlignLeading
	v.presetSelect.PlaceHolder = noPreset

	ebus.SubscribeFunc(ebus.TOPIC_COLORBLINDMODE, func(f float64) {
		v.list.SetColorBlindMode(colors.ColorBlindMode(int(f)))
		v.list.Refresh()
	})

	ebus.SubscribeFunc(ebus.TOPIC_REALTIMEBARS, func(f float64) {
		v.list.UpdateBars(f != 0)
	})

	v.content = container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.SearchIcon()),
			container.NewHBox(v.addBtn, v.syncBtn),
			v.symbolLookup,
		),
		container.NewBorder(
			nil,
			nil,
			nil,
			container.NewGridWithColumns(5,
				widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), v.savePreset),
				widget.NewButtonWithIcon("", theme.ContentAddIcon(), v.newPreset),
				widget.NewButtonWithIcon("", txtheme.ExportIcon(), v.exportPreset),
				widget.NewButtonWithIcon("", txtheme.ImportIcon(), v.importPreset),
				widget.NewButtonWithIcon("", theme.DeleteIcon(), v.deletePreset),
			),
			v.presetSelect,
		),
		nil,
		nil,
		v.list,
	)
	return v
}

func (v *Viewer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(v.content)
}

// SetSymbols sets the symbol collection used for lookup and syncs the
// listed symbols against it.
func (v *Viewer) SetSymbols(fw symbol.FirmwareFile) {
	v.fw = fw
	v.Sync()
}

// Sync refreshes the listed symbols with fresh data from the loaded binary.
func (v *Viewer) Sync() {
	if v.fw == nil {
		return
	}
	cnt := 0
	for _, s := range v.list.Symbols() {
		sym := v.fw.GetByName(s.Name)
		if sym != nil {
			s.Name = sym.Name
			s.Number = sym.Number
			s.Address = sym.Address
			s.SramOffset = sym.SramOffset
			s.Length = sym.Length
			s.Mask = sym.Mask
			s.Type = sym.Type
			s.Unit = sym.Unit
			s.Correctionfactor = sym.Correctionfactor
			cnt++
		}
	}
	v.list.Refresh()
	v.cfg.Log(fmt.Sprintf("Synced %d / %d symbols", cnt, v.list.Count()))
}

func (v *Viewer) addSymbol() {
	if v.symbolLookup.Text == "" {
		return
	}
	if v.fw == nil {
		v.cfg.Error(fmt.Errorf("Cannot add symbol, no binary loaded"))
		return
	}
	sym := v.fw.GetByName(v.symbolLookup.Text)
	if sym == nil {
		v.cfg.Error(fmt.Errorf("%q not found in binary", v.symbolLookup.Text))
		return
	}
	v.list.Add(sym)
}

func (v *Viewer) newSymbolnameTypeahead() {
	v.symbolLookup = widgets.NewCompletionEntry([]string{})
	v.symbolLookup.PlaceHolder = "Search for symbol"
	v.symbolLookup.OnChanged = func(s string) {
		if v.fw == nil {
			return
		}
		// completion start for text length >= 3
		if len(s) < 3 {
			v.symbolLookup.HideCompletion()
			return
		}
		var results []string
		for _, sym := range v.fw.Symbols() {
			if sym.Length > 8 {
				continue
			}
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) {
				results = append(results, sym.Name)
			}
		}
		if len(results) == 0 {
			v.symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		v.symbolLookup.SetOptions(results)
		v.symbolLookup.ShowCompletion()
	}
}

// SelectedPreset returns the name of the currently selected preset.
func (v *Viewer) SelectedPreset() string {
	return v.presetSelect.Selected
}

// SelectPreset selects the named preset, loading its symbols.
func (v *Viewer) SelectPreset(name string) {
	v.presetSelect.SetSelected(name)
}

// SelectPresetForECU selects the last used preset for the given ECU.
func (v *Viewer) SelectPresetForECU(ecu string) {
	v.presetSelect.SetSelected(v.cfg.App.Preferences().StringWithFallback(ecu+prefsSelectedPreset, ecu+" Dash"))
}

func (v *Viewer) reloadPresets() {
	v.presetSelect.SetOptions(append([]string{noPreset}, presets.Names()...))
}

func (v *Viewer) savePreset() {
	if v.presetSelect.Selected == noPreset {
		v.newPreset()
		return
	}
	if err := presets.Set(v.presetSelect.Selected, v.list.Symbols()); err != nil {
		v.cfg.Error(err)
		return
	}
	if err := presets.Save(v.cfg.App); err != nil {
		v.cfg.Error(err)
		return
	}
}

func (v *Viewer) newPreset() {
	presetName := widget.NewEntry()
	dialog.NewForm("Create new preset", "Create", "Cancel", []*widget.FormItem{
		widget.NewFormItem("name", presetName),
	},
		func(create bool) {
			if create {
				if presetName.Text == "" {
					v.cfg.Error(fmt.Errorf("name can't be empty"))
					return
				}
				if err := presets.Set(presetName.Text, v.list.Symbols()); err != nil {
					v.cfg.Error(err)
					return
				}
				if err := presets.Save(v.cfg.App); err != nil {
					v.cfg.Error(err)
					return
				}
				v.reloadPresets()
				v.presetSelect.SetSelected(presetName.Text)
			}
		},
		v.cfg.Window,
	).Show()
	v.cfg.Window.Canvas().Focus(presetName)
}

func (v *Viewer) importPreset() {
	widgets.SelectFile(func(r fyne.URIReadCloser) {
		defer r.Close()
		if err := v.LoadPreset(r); err != nil {
			v.cfg.Error(err)
			return
		}
		v.Sync()
	}, "Preset file", "txp")
}

func (v *Viewer) exportPreset() {
	widgets.SaveFile(func(filename string) {
		if !strings.HasSuffix(filename, ".txp") {
			filename += ".txp"
		}
		if err := v.ExportPreset(filename); err != nil {
			v.cfg.Error(err)
			return
		}
	}, "Preset file", "txp")
}

func (v *Viewer) deletePreset() {
	if v.presetSelect.Selected == noPreset {
		dialog.ShowInformation("No preset selected", "Select a preset to delete", v.cfg.Window)
		return
	}
	if presets.IsSystem(v.presetSelect.Selected) {
		v.cfg.Error(fmt.Errorf("can't delete built-in preset"))
		return
	}
	dialog.ShowConfirm("Confirm preset delete", "Delete preset '"+v.presetSelect.Selected+"', are you sure?", func(b bool) {
		if b {
			if err := presets.Delete(v.presetSelect.Selected); err != nil {
				v.cfg.Error(err)
				return
			}
			if err := presets.Save(v.cfg.App); err != nil {
				v.cfg.Error(err)
				return
			}
			v.reloadPresets()
			v.presetSelect.SetSelected(noPreset)
		}
	}, v.cfg.Window)
}

// LoadPreset loads symbols from a .txp preset file.
func (v *Viewer) LoadPreset(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg []*symbol.Symbol
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	v.list.LoadSymbols(cfg...)
	v.cfg.App.Preferences().SetString(prefsSymbolList, string(b))
	return nil
}

// ExportPreset writes the listed symbols to a .txp preset file.
func (v *Viewer) ExportPreset(filename string) error {
	b, err := json.Marshal(v.list.Symbols())
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %w", err)
	}
	if err := os.WriteFile(filename, b, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func (v *Viewer) Disable() {
	v.addBtn.Disable()
	v.syncBtn.Disable()
	v.presetSelect.Disable()
	v.list.Disable()
}

func (v *Viewer) Enable() {
	v.addBtn.Enable()
	v.syncBtn.Enable()
	v.presetSelect.Enable()
	v.list.Enable()
}

// The rest is the list API, delegated.

func (v *Viewer) Names() []string                       { return v.list.Names() }
func (v *Viewer) Symbols() []*symbol.Symbol             { return v.list.Symbols() }
func (v *Viewer) Count() int                            { return v.list.Count() }
func (v *Viewer) SetValue(name string, value float64)   { v.list.SetValue(name, value) }
func (v *Viewer) Clear()                                { v.list.Clear() }
func (v *Viewer) UpdateBars(enabled bool)               { v.list.UpdateBars(enabled) }
func (v *Viewer) LoadSymbols(symbols ...*symbol.Symbol) { v.list.LoadSymbols(symbols...) }
func (v *Viewer) SetColorBlindMode(mode colors.ColorBlindMode) {
	v.list.SetColorBlindMode(mode)
	v.list.Refresh()
}
