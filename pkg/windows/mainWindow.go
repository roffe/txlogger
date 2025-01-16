package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/ebitengine/oto/v3"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/widgets/combinedlogplayer"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
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

var _ fyne.Tappable = (*secretText)(nil)

type secretText struct {
	*widget.Label
	tappedTimes int
	SecretFunc  func()
}

func (s *secretText) Tapped(*fyne.PointEvent) {
	s.tappedTimes++
	//	log.Println("tapped", s.tappedTimes)
	if s.tappedTimes >= 10 {
		t := fyne.NewStaticResource("taz.png", assets.Taz)
		cv := canvas.NewImageFromResource(t)
		cv.SetMinSize(fyne.NewSize(0, 0))
		cont := container.NewStack(cv)
		s.tappedTimes = 0
		if f := s.SecretFunc; f != nil {
			f()
		}
		dialog.ShowCustom("You found the secret", "Leif", cont, fyne.CurrentApp().Driver().AllWindows()[0])
		an := canvas.NewSizeAnimation(fyne.NewSize(0, 0), fyne.NewSize(370, 386), time.Second, func(size fyne.Size) {
			cv.Resize(size)
		})
		an.Start()
	}
}

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
	statusText      *secretText
	oCtx            *oto.Context
	wm              *multiwindow.MultipleWindows
	content         *fyne.Container
	startup         bool
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
	//captureCounter       binding.Int
	capturedCounterLabel *widget.Label
	//errorCounter         binding.Int
	errorCounterLabel *widget.Label
	//fpsCounter           binding.Int
	fpsCounterLabel *widget.Label
}

func NewMainWindow(app fyne.App, filename string) *MainWindow {
	mw := &MainWindow{
		Window:     app.NewWindow("txlogger"),
		app:        app,
		outputData: binding.NewStringList(),

		counters: &mainWindowCounters{
			//captureCounter: binding.NewInt(),
			//errorCounter:   binding.NewInt(),
			//fpsCounter:     binding.NewInt(),
		},

		selects: &mainWindowSelects{},
		buttons: &mainWindowButtons{},

		statusText: &secretText{Label: widget.NewLabel("Harder, Better, Faster, Stronger")},

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

	mw.symbolList = symbollist.New(&symbollist.Config{
		EBus:       ebus.CONTROLLER,
		Window:     mw,
		UpdateFunc: updateSymbols,
	})

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

	mw.Window.SetOnDropped(mw.onDropped)
	mw.SetCloseIntercept(mw.closeIntercept)

	mw.render()

	mw.SetPadded(true)
	mw.SetContent(mw.content)
	mw.Resize(fyne.NewSize(1000, 700))
	mw.CenterOnScreen()
	mw.SetMaster()

	mw.whatsNew()

	ctrlEnter := &desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierControl}
	altEnter := &desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierAlt}
	ctrl1 := &desktop.CustomShortcut{KeyName: fyne.Key1, Modifier: fyne.KeyModifierControl}
	ctrl2 := &desktop.CustomShortcut{KeyName: fyne.Key2, Modifier: fyne.KeyModifierControl}
	ctrl3 := &desktop.CustomShortcut{KeyName: fyne.Key3, Modifier: fyne.KeyModifierControl}
	ctrl4 := &desktop.CustomShortcut{KeyName: fyne.Key4, Modifier: fyne.KeyModifierControl}

	mw.Window.Canvas().AddShortcut(ctrlEnter, func(shortcut fyne.Shortcut) {
		mw.wm.Arrange(&multiwindow.GridArranger{})
	})

	mw.Window.Canvas().AddShortcut(altEnter, func(shortcut fyne.Shortcut) {
		mw.Window.SetFullScreen(!mw.Window.FullScreen())
	})

	mw.Window.Canvas().AddShortcut(ctrl1, func(shortcut fyne.Shortcut) {
		log.Println("ctrl1")
		mw.openSettings()
	})

	mw.Window.Canvas().AddShortcut(ctrl2, func(shortcut fyne.Shortcut) {
		log.Println("ctrl2")
		mw.buttons.symbolListBtn.OnTapped()
	})

	mw.Window.Canvas().AddShortcut(ctrl3, func(shortcut fyne.Shortcut) {
		log.Println("ctrl3")
	})

	mw.Window.Canvas().AddShortcut(ctrl4, func(shortcut fyne.Shortcut) {
		log.Println("ctrl4")
	})

	mw.startup = true
	mw.buttons.symbolListBtn.OnTapped()
	mw.buttons.dashboardBtn.OnTapped()
	mw.startup = false

	return mw
}

func (mw *MainWindow) render() {
	/*
		mw.tabs = container.NewAppTabs(
			container.NewTabItem("Symbols", container.NewBorder(
				container.NewGridWithRows(2,
					container.NewHBox(
						container.NewBorder(
							nil,
							nil,
							widget.NewLabel("Preset"),
							nil,
							mw.selects.presetSelect,
						),
						widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), mw.savePreset),
						widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), mw.newPreset),
						widget.NewButtonWithIcon("", theme.UploadIcon(), mw.exportPreset),
						widget.NewButtonWithIcon("", theme.DownloadIcon(), mw.importPreset),
						widget.NewButtonWithIcon("", theme.DeleteIcon(), mw.deletePreset),
					),
					container.NewBorder(
						nil,
						nil,
						widget.NewIcon(theme.SearchIcon()),
						container.NewHBox(
							mw.buttons.addSymbolBtn,
							mw.buttons.syncSymbolsBtn,
						),
						mw.selects.symbolLookup,
					),
				),
				nil,
				nil,
				nil,
				mw.symbolList,
			)),
			container.NewTabItem("Settings", mw.settings),
			container.NewTabItem("Open Windows", mw.wm),
		)
	*/

	mw.content = container.NewBorder(
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
					mw.counters.fpsCounterLabel,
					mw.buttons.debugBtn,
				),
				widget.NewButtonWithIcon("", theme.ComputerIcon(), mw.openEBUSMonitor),
			),
			mw.statusText,
		),
		nil,
		nil,
		mw.wm,
		// mw.tabs,
	)
}
func (mw *MainWindow) LoadLogfileCombined(filename string, p fyne.Position) {
	// Just filename, used for Window title
	fp := filepath.Base(filename)

	// if w := mw.wm.HasWindow(fp); w != nil {
	// 	mw.wm.Raise(w)
	// 	return
	// }

	logz, err := logfile.Open(filename)
	if err != nil {
		mw.Error(fmt.Errorf("failed to open log file: %w", err))
		return
	}

	dbcfg := &dashboard.Config{
		Logplayer:       true,
		UseMPH:          mw.settings.GetUseMPH(),
		SwapRPMandSpeed: mw.settings.GetSwapRPMandSpeed(),
		HighAFR:         mw.settings.GetHighAFR(),
		LowAFR:          mw.settings.GetLowAFR(),
		WidebandSymbol:  mw.settings.GetWidebandSymbolName(),
	}

	rec := logz.Next()
	if !rec.EOF {
		for k := range rec.Values {
			if k == "AirMassMast.m_Request" {
				dbcfg.AirDemToString = datalogger.AirDemToStringT8
				break
			} else if k == "Lufttemp" {
				//T5
				break
			} else {
				dbcfg.AirDemToString = datalogger.AirDemToStringT7
				break
			}
		}
	}
	logz.Seek(0)

	switch mw.selects.ecuSelect.Selected {
	case "T7":
		dbcfg.AirDemToString = datalogger.AirDemToStringT7
	case "T8":
		dbcfg.AirDemToString = datalogger.AirDemToStringT8
	}

	cpCfg := &combinedlogplayer.CombinedLogplayerConfig{
		Logfile: logz,
		DBcfg:   dbcfg,
	}

	cp := combinedlogplayer.New(cpCfg)
	//iw := multiwindow.NewSystemWindow(fp, cp)
	//iw.Icon = theme.MediaPlayIcon()

	/*
		dbcfg.FullscreenFunc = func(b bool) {
			if b {
				mw.SetMainMenu(nil)
				mw.Window.SetContent(cp)
				mw.SetFullScreen(true)
			} else {
				mw.SetMainMenu(mw.menu.GetMenu(mw.selects.ecuSelect.Selected))
				mw.Window.SetContent(mw.content)
				cp.Close()
				//mw.buttons.dashboardBtn.OnTapped()
				mw.SetFullScreen(false)
				iw.SetContent(cp)
			}
		}
	*/

	//cp.OnMouseDown = func() {
	//	mw.wm.Raise(iw)
	//}

	//iw.CloseIntercept = func() {
	//	cp.Close()
	//}
	w := mw.app.NewWindow(fp)

	w.SetCloseIntercept(func() {
		cp.Close()
		w.Close()
	})
	w.Canvas().SetOnTypedKey(cp.TypedKey)
	//fyne.Do(func() {
	w.SetContent(cp)
	w.Show()
	//})

	//w.Show()
	//mw.wm.Add(iw, p)
	mw.Log("loaded log file " + filename + " in combined logplayer")
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
		mw.Error(fmt.Errorf("failed to open log file: %w", err))
		return
	}

	lp := logplayer.New(&logplayer.Config{
		EBus:    ebus.CONTROLLER,
		Logfile: logz,
	})
	iw := multiwindow.NewSystemWindow(fp, lp)
	iw.Icon = theme.MediaPlayIcon()

	lp.OnMouseDown = func() {
		mw.wm.Raise(iw)
	}
	iw.OnClose = func() {
		lp.Close()
	}

	mw.wm.Add(iw, p)

	mw.Log("loaded log file " + filename)
}

func (mw *MainWindow) Log(s string) {
	debug.Log(s)
	//go fyne.Do(func() {
	mw.outputData.Append(s)
	//})
}

func (mw *MainWindow) Error(err error) {
	debug.Log("error:" + err.Error())
	// go fyne.Do(func() {
	mw.outputData.Append(err.Error())
	dialog.ShowError(err, mw.Window)
	//})
	//log.Printf("error: %s", err)
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

	p := progressmodal.New(mw.Window.Canvas(), "Loading symbols from ECU")
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
