package windows

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/plotter"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/widgets"
	"golang.org/x/net/context"
)

const (
	prefsLastBinFile    = "lastBinFile"
	prefsLastConfig     = "lastConfig"
	prefsSelectedECU    = "lastECU"
	prefsSymbolList     = "symbolList"
	prefsSelectedPreset = "selectedPreset"
)

type MainWindow struct {
	fyne.Window
	app fyne.App

	menu *mainmenu.MainMenu

	symbolLookup *xwidget.CompletionEntry

	output     *widget.List
	outputData binding.StringList

	canSettings *widgets.CanSettingsWidget

	ecuSelect *widget.Select

	addSymbolBtn *widget.Button
	logBtn       *widget.Button

	loadSymbolsEcuBtn  *widget.Button
	loadSymbolsFileBtn *widget.Button
	syncSymbolsBtn     *widget.Button

	dashboardBtn *widget.Button
	plotterBtn   *widget.Button
	logplayerBtn *widget.Button
	helpBtn      *widget.Button
	settingsBtn  *widget.Button

	//loadConfigBtn *widget.Button
	//saveConfigBtn *widget.Button
	presetSelect *widget.Select

	captureCounter binding.Int
	errorCounter   binding.Int
	fpsCounter     binding.Int

	capturedCounterLabel *widget.Label
	errorCounterLabel    *widget.Label
	fpsLabel             *widget.Label

	loggingRunning bool

	filename   string
	symbolList *widgets.SymbolListWidget
	fw         symbol.SymbolCollection

	dlc       datalogger.Logger
	dashboard *widgets.Dashboard
	plotter   *plotter.Plotter

	buttonsDisabled bool

	leading *fyne.Container

	settings *widgets.SettingsWidget

	openMaps map[string]fyne.Window

	tab *container.AppTabs
	//doctab *container.DocTabs
}

func NewMainWindow(a fyne.App, filename string) *MainWindow {

	mw := &MainWindow{
		Window:         a.NewWindow("txlogger"),
		app:            a,
		outputData:     binding.NewStringList(),
		canSettings:    widgets.NewCanSettingsWidget(a),
		captureCounter: binding.NewInt(),
		errorCounter:   binding.NewInt(),
		fpsCounter:     binding.NewInt(),

		openMaps: make(map[string]fyne.Window),
		settings: widgets.NewSettingsWidget(),
	}
	mw.Resize(fyne.NewSize(1024, 768))

	updateSymbols := func(syms []*symbol.Symbol) {
		if mw.dlc != nil {
			if err := mw.dlc.SetSymbols(mw.symbolList.Symbols()); err != nil {
				if err.Error() == "pending" {
					return
				}
				mw.Log(err.Error())
			}
		}
	}

	mw.symbolList = widgets.NewSymbolListWidget(mw, updateSymbols)

	mw.setupMenu()

	mw.Window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyF12:
			capture.Screenshot(mw.Canvas())
		}
	})

	mw.Window.SetCloseIntercept(mw.CloseIntercept)

	mw.createButtons()

	mw.presetSelect = &widget.Select{
		Alignment:   fyne.TextAlignLeading,
		PlaceHolder: "Select preset",
		Options:     append([]string{"Select preset"}, presets.Names()...),

		OnChanged: func(s string) {
			if s == "Select preset" {
				return
			}
			preset, err := presets.Get(s)
			if err != nil {
				dialog.ShowError(err, mw)
				return
			}
			mw.symbolList.LoadSymbols(preset...)
			//mw.SaveSymbolList()
			//mw.symbolList.Refresh()
			mw.SyncSymbols()
			mw.app.Preferences().SetString(prefsSelectedPreset, s)
		},
	}

	mw.newOutputList()
	mw.newSymbolnameTypeahead()

	mw.capturedCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.captureCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.captureCounter.Get(); err == nil {
			mw.capturedCounterLabel.SetText(fmt.Sprintf("Cap: %d", val))
		}
	}))

	mw.errorCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.errorCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.errorCounter.Get(); err == nil {
			mw.errorCounterLabel.SetText(fmt.Sprintf("Err: %d", val))
		}
	}))

	mw.fpsLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.fpsCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.fpsCounter.Get(); err == nil {
			mw.fpsLabel.SetText(fmt.Sprintf("Fps: %d", val))
		}
	}))

	mw.ecuSelect = widget.NewSelect([]string{"T7", "T8"}, func(s string) {
		mw.app.Preferences().SetString(prefsSelectedECU, s)
		mw.SetMainMenu(mw.menu.GetMenu(s))
	})

	mw.setupTabs()

	mw.loadPrefs(filename)
	if mw.fw == nil {
		mw.setTitle("No symbols loaded")
	}

	mw.SetMaster()
	mw.SetContent(mw.tab)
	return mw
}
func (mw *MainWindow) CloseIntercept() {
	if mw.dlc != nil {
		mw.dlc.Close()
		time.Sleep(500 * time.Millisecond)
	}
	//mw.SaveSymbolList()
	debug.Close()
	mw.Close()
}

func (mw *MainWindow) createLeading() *fyne.Container {
	//mw.doctab = container.NewDocTabs()
	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.SearchIcon()),
				container.NewHBox(
					mw.addSymbolBtn,
					mw.loadSymbolsFileBtn,
					mw.loadSymbolsEcuBtn,
					mw.syncSymbolsBtn,
				),
				mw.symbolLookup,
			),
		),
		//container.NewVBox(
		//	container.NewGridWithColumns(2,
		//		mw.loadConfigBtn,
		//		mw.saveConfigBtn,
		//	),
		//),
		nil,
		nil,
		nil,
		//widget.NewLabel("Symbols..."),
		mw.symbolList,
	)
}

func (mw *MainWindow) setupTabs() {
	mw.leading = mw.createLeading()
	mw.tab = container.NewAppTabs()
	mw.tab.Append(container.NewTabItemWithIcon("Symbols", theme.ListIcon(), &container.Split{
		Offset:     0.7,
		Horizontal: true,
		Leading:    mw.leading,
		Trailing: &container.Split{
			Offset:     0,
			Horizontal: false,
			Leading: container.NewVBox(
				container.NewBorder(
					nil,
					nil,
					layout.NewFixedWidth(75, widget.NewLabel("ECU")),
					nil,
					mw.ecuSelect,
				),
				container.NewBorder(
					nil,
					nil,
					layout.NewFixedWidth(75, widget.NewLabel("Preset")),
					nil,
					mw.presetSelect,
				),
				mw.canSettings,
				mw.logBtn,
			),
			Trailing: &container.Split{
				Offset:     1,
				Horizontal: false,
				Leading:    mw.output,
				Trailing: container.NewVBox(
					mw.dashboardBtn,
					//mw.plotterBtn,
					mw.logplayerBtn,
					mw.helpBtn,
					//mw.settingsBtn,
					container.NewGridWithColumns(3,
						mw.capturedCounterLabel,
						mw.errorCounterLabel,
						mw.fpsLabel,
					),
				),
			},
		},
	}))

	mw.tab.Append(container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), mw.settings))
}

func (mw *MainWindow) Log(s string) {
	debug.Log(s)
	mw.outputData.Append(s)
	mw.output.ScrollToBottom()
	//mw.output.Refresh()
}

func (mw *MainWindow) SyncSymbols() {
	if mw.fw == nil {
		mw.Log("Load bin to sync symbols")
		return
	}
	cnt := 0
	for _, v := range mw.symbolList.Symbols() {
		sym := mw.fw.GetByName(v.Name)
		if sym != nil {
			v.Name = sym.Name
			v.Number = sym.Number
			v.Address = sym.Address
			v.Length = sym.Length
			v.Mask = sym.Mask
			v.Type = sym.Type
			v.Unit = sym.Unit
			v.Correctionfactor = sym.Correctionfactor
			cnt++
		}
	}
	mw.symbolList.Refresh()
	//mw.SaveSymbolList()
	mw.Log(fmt.Sprintf("Synced %d / %d symbols", cnt, len(mw.symbolList.Symbols())))
}

func (mw *MainWindow) Disable() {
	mw.buttonsDisabled = true
	//mw.addSymbolBtn.Disable()
	//mw.loadConfigBtn.Disable()
	//mw.saveConfigBtn.Disable()
	mw.loadSymbolsFileBtn.Disable()
	mw.loadSymbolsEcuBtn.Disable()
	mw.syncSymbolsBtn.Disable()
	if !mw.loggingRunning {
		mw.logBtn.Disable()
	}
	mw.ecuSelect.Disable()
	mw.canSettings.Disable()
	//mw.presetSelect.Disable()
	//mw.symbolList.Disable()
}

func (mw *MainWindow) Enable() {
	mw.buttonsDisabled = false
	//mw.addSymbolBtn.Enable()
	//mw.loadConfigBtn.Enable()
	//mw.saveConfigBtn.Enable()
	mw.loadSymbolsFileBtn.Enable()
	mw.loadSymbolsEcuBtn.Enable()
	mw.syncSymbolsBtn.Enable()
	mw.logBtn.Enable()
	mw.ecuSelect.Enable()
	mw.canSettings.Enable()
	//mw.presetSelect.Enable()
	//mw.symbolList.Enable()
}

func (mw *MainWindow) LoadSymbolsFromECU() error {
	device, err := mw.canSettings.GetAdapter(mw.ecuSelect.Selected, mw.Log)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	switch mw.ecuSelect.Selected {
	case "T7":
		symbols, err := ecu.GetSymbolsT7(ctx, device, mw.Log)
		if err != nil {
			return err
		}
		mw.fw = symbols
		mw.SyncSymbols()
	case "T8":
		symbols, err := ecu.GetSymbolsT8(ctx, device, mw.Log)
		if err != nil {
			return err
		}
		mw.fw = symbols
		mw.SyncSymbols()
	}

	mw.setTitle("Symbols loaded from ECU " + time.Now().Format("2006-01-02 15:04:05.000"))
	return nil
}

func (mw *MainWindow) LoadSymbolsFromFile(filename string) error {
	ecuType, symbols, err := symbol.Load(filename, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.setTitle(filename)
	mw.app.Preferences().SetString(prefsLastBinFile, filename)

	mw.ecuSelect.SetSelected(ecuType.String())

	mw.fw = symbols
	mw.SyncSymbols()

	//os.WriteFile("symbols.txt", []byte(symbols.Dump()), 0644)
	return nil
}

func (mw *MainWindow) LoadConfig(filename string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg []*symbol.Symbol
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	mw.symbolList.LoadSymbols(cfg...)
	mw.app.Preferences().SetString(prefsSymbolList, string(b))
	return nil
}

func (mw *MainWindow) LoadPresetFromStringx(str string) error {
	var cfg []*symbol.Symbol
	if err := json.Unmarshal([]byte(str), &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	mw.symbolList.LoadSymbols(cfg...)
	mw.SaveSymbolListx()
	return nil
}

func (mw *MainWindow) SaveSymbolListx() {
	b, err := json.Marshal(mw.symbolList.Symbols())
	if err != nil {
		mw.Log(err.Error())
		return
	}
	mw.app.Preferences().SetString(prefsSymbolList, string(b))
}

func (mw *MainWindow) SaveConfig(filename string) error {
	b, err := json.Marshal(mw.symbolList.Symbols())
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %w", err)
	}
	if err := os.WriteFile(filename, b, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}
