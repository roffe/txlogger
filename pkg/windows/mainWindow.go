package windows

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/ebitengine/oto/v3"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/widgets/logplayer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
	"github.com/roffe/txlogger/pkg/widgets/settings"
	"github.com/roffe/txlogger/pkg/widgets/symbollist"
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
	app             fyne.App
	menu            *mainmenu.MainMenu
	outputData      binding.StringList
	selects         *mainWindowSelects
	buttons         *mainWindowButtons
	counters        *mainWindowCounters
	loggingRunning  bool
	filename        string
	symbolList      *symbollist.Widget
	fw              symbol.SymbolCollection
	dlc             datalogger.IClient
	buttonsDisabled bool
	settings        *settings.SettingsWidget
	statusText      *widget.Label
	oCtx            *oto.Context
	wm              *multiwindow.MultipleWindows
	content         *fyne.Container
}

type mainWindowSelects struct {
	symbolLookup *xwidget.CompletionEntry
	ecuSelect    *widget.Select
	presetSelect *widget.Select
	layoutSelect *widget.Select
}

type mainWindowButtons struct {
	debugBtn     *widget.Button
	addSymbolBtn *widget.Button
	logBtn       *widget.Button
	// loadSymbolsEcuBtn *widget.Button
	syncSymbolsBtn   *widget.Button
	dashboardBtn     *widget.Button
	openLogBtn       *widget.Button
	layoutRefreshBtn *widget.Button
	symbolListBtn    *widget.Button
	addGaugeBtn      *widget.Button
}

type mainWindowCounters struct {
	captureCounter       binding.Int
	capturedCounterLabel *widget.Label
	errorCounter         binding.Int
	errorCounterLabel    *widget.Label
	fpsCounter           binding.Int
	fpsLabel             *widget.Label
}

func NewMainWindow(app fyne.App, filename string) *MainWindow {
	mw := &MainWindow{
		Window:     app.NewWindow("txlogger"),
		app:        app,
		outputData: binding.NewStringList(),

		counters: &mainWindowCounters{
			captureCounter: binding.NewInt(),
			errorCounter:   binding.NewInt(),
			fpsCounter:     binding.NewInt(),
		},

		selects: &mainWindowSelects{},
		buttons: &mainWindowButtons{},

		statusText: widget.NewLabel("Harder, Better, Faster, Stronger"),

		oCtx: newOtoContext(),
	}

	mw.wm = multiwindow.NewMultipleWindows()
	mw.wm.LockViewport = true

	mw.wm.OnError = mw.Error

	updateSymbols := func(syms []*symbol.Symbol) {
		if mw.dlc != nil {
			if err := mw.dlc.SetSymbols(mw.symbolList.Symbols()); err != nil {
				if err.Error() == "pending" {
					return
				}
				mw.Error(err)
			}
		}
	}

	mw.symbolList = symbollist.New(mw, updateSymbols)

	mw.setupMenu()

	mw.createButtons()
	mw.createSelects()
	mw.createCounters()

	mw.newSymbolnameTypeahead()

	mw.settings = settings.New(&settings.Config{
		EcuSelect: mw.selects.ecuSelect,
	})

	mw.loadPrefs(filename)
	if mw.fw == nil {
		mw.SetTitle("No symbols loaded")
	}

	mw.SetOnDropped(mw.onDropped)
	mw.SetCloseIntercept(mw.closeIntercept)

	mw.render()

	mw.SetPadded(true)
	mw.SetContent(mw.content)
	mw.Resize(fyne.NewSize(1024, 768))
	mw.CenterOnScreen()
	mw.SetMaster()

	mw.whatsNew()

	return mw
}

func (mw *MainWindow) render() {
	mw.content = container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			container.NewHBox(
				container.NewBorder(
					nil,
					nil,
					widget.NewLabel("ECU"),
					nil,
					mw.selects.ecuSelect,
				),
				widget.NewSeparator(),
				mw.buttons.symbolListBtn,
				mw.buttons.logBtn,
			),
			nil,
			container.NewHBox(
				//mw.buttons.logplayerBtn,
				mw.buttons.openLogBtn,
				mw.buttons.dashboardBtn,
				widget.NewButtonWithIcon("", theme.GridIcon(), func() {
					mw.wm.Arrange(&multiwindow.GridArranger{})
				}),
				mw.buttons.addGaugeBtn,
				widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
					mw.wm.CloseAll()
				}),
			),
		),
		container.NewBorder(
			nil,
			nil,
			container.NewBorder(
				nil,
				nil,
				nil,
				mw.buttons.layoutRefreshBtn,
				mw.selects.layoutSelect,
			),
			container.NewHBox(
				container.NewGridWithColumns(4,
					mw.counters.capturedCounterLabel,
					mw.counters.errorCounterLabel,
					mw.counters.fpsLabel,
					mw.buttons.debugBtn,
				),
				widget.NewButtonWithIcon("", theme.ComputerIcon(), mw.openEBUSMonitor),
			),
			mw.statusText,
		),
		nil,
		nil,
		mw.wm,
	)
}

func (mw *MainWindow) LoadLogfile(filename string, p fyne.Position) {
	// Just filename, used for Window title
	fp := filepath.Base(filename)

	if w := mw.wm.HasWindow(fp); w != nil {
		mw.wm.Raise(w)
		return
	}

	logz, err := logfile.Open(filename)
	if err != nil {
		mw.Error(fmt.Errorf("Failed to open log file: %w", err))
		return
	}

	lp := logplayer.New(logz)
	iw := multiwindow.NewSystemWindow(fp, lp)
	iw.Icon = theme.MediaPlayIcon()

	iw.CloseIntercept = func() {
		lp.Close()
	}

	mw.wm.Add(iw, p)
}

func (mw *MainWindow) Log(s string) {
	debug.Log(s)
	mw.outputData.Append(s)
}

func (mw *MainWindow) Error(err error) {
	debug.Log("error:" + err.Error())
	mw.outputData.Append(err.Error())
	dialog.ShowError(err, mw)
}

func (mw *MainWindow) Disable() {
	mw.buttonsDisabled = true
	mw.buttons.addSymbolBtn.Disable()
	//mw.buttons.loadSymbolsEcuBtn.Disable()
	mw.buttons.syncSymbolsBtn.Disable()
	if !mw.loggingRunning {
		mw.buttons.logBtn.Disable()
	}
	mw.selects.ecuSelect.Disable()
	mw.selects.presetSelect.Disable()
	mw.symbolList.Disable()
}

func (mw *MainWindow) Enable() {
	mw.buttonsDisabled = false
	mw.buttons.addSymbolBtn.Enable()
	//mw.buttons.loadSymbolsEcuBtn.Enable()
	mw.buttons.syncSymbolsBtn.Enable()
	mw.buttons.logBtn.Enable()
	mw.selects.ecuSelect.Enable()
	mw.selects.presetSelect.Enable()
	mw.symbolList.Enable()
}

func (mw *MainWindow) SyncSymbols() {
	if mw.fw == nil {
		return
	}
	cnt := 0
	for _, v := range mw.symbolList.Symbols() {
		sym := mw.fw.GetByName(v.Name)
		if sym != nil {
			v.Name = sym.Name
			v.Number = sym.Number
			v.Address = sym.Address
			v.SramOffset = sym.SramOffset
			v.Length = sym.Length
			v.Mask = sym.Mask
			v.Type = sym.Type
			v.Unit = sym.Unit
			v.Correctionfactor = sym.Correctionfactor
			cnt++
		}
	}
	mw.symbolList.Refresh()
	mw.Log(fmt.Sprintf("Synced %d / %d symbols", cnt, mw.symbolList.Count()))
}

func (mw *MainWindow) LoadSymbolsFromECU() error {
	device, err := mw.settings.CanSettings.GetAdapter(mw.selects.ecuSelect.Selected, mw.Log)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	p := progressmodal.New(mw.Window.Content(), "Loading symbols from ECU")
	p.Show()
	defer p.Hide()

	switch mw.selects.ecuSelect.Selected {
	case "T5":
		symbols, err := ecu.GetSymbolsT5(ctx, device, mw.Log)
		if err != nil {
			return err
		}
		mw.fw = symbols
		mw.SyncSymbols()
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

	mw.Log("Symbols loaded from ECU " + time.Now().Format("2006-01-02 15:04:05.000"))
	return nil
}

func (mw *MainWindow) LoadSymbolsFromFile(filename string) error {
	ecuType, symbols, err := symbol.Load(filename, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.SetTitle(filepath.Base(filename))
	mw.app.Preferences().SetString(prefsLastBinFile, filename)
	mw.selects.ecuSelect.SetSelected(ecuType.String())
	mw.fw = symbols
	mw.filename = filename
	mw.SyncSymbols()
	return nil
}

func (mw *MainWindow) LoadPreset(filename string) error {
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

func (mw *MainWindow) SavePreset(filename string) error {
	b, err := json.Marshal(mw.symbolList.Symbols())
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %w", err)
	}
	if err := os.WriteFile(filename, b, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// -----

/*
var (
	u32             = syscall.NewLazyDLL("user32.dll")
	procPostMessage = u32.NewProc("PostMessageW")
)

const (
	WM_SYSCOMMAND = 274
	SC_RESTORE    = 0xF120
	SC_MINIMIZE   = 0xF020
	SC_MAXIMIZE   = 0xF030
)

func PostMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	ret, _, _ := procPostMessage.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret != 0
}
func (mw *MainWindow) Maximize() {
	ctx, ok := mw.Window.(driver.NativeWindow)
	if ok {
		ctx.RunNative(func(c any) {
			switch t := c.(type) {
			case driver.WindowsWindowContext:
				PostMessage(t.HWND, WM_SYSCOMMAND, SC_MAXIMIZE, 0)
			}
		})
	}
}

func (mw *MainWindow) Minimize() {
	ctx, ok := mw.Window.(driver.NativeWindow)
	if ok {
		ctx.RunNative(func(c any) {
			switch t := c.(type) {
			case driver.WindowsWindowContext:
				PostMessage(t.HWND, WM_SYSCOMMAND, SC_MINIMIZE, 0)
			}
		})
	}
}

func (mw *MainWindow) Restore() {
	ctx, ok := mw.Window.(driver.NativeWindow)
	if ok {
		ctx.RunNative(func(c any) {
			switch t := c.(type) {
			case driver.WindowsWindowContext:
				PostMessage(t.HWND, WM_SYSCOMMAND, SC_RESTORE, 0)
			}
		})
	}
}
*/
