package windows

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/update"
	"github.com/roffe/txlogger/pkg/widgets/combinedlogplayer"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/ledicon"
	"github.com/roffe/txlogger/pkg/widgets/logplayer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/secrettext"
	"github.com/roffe/txlogger/pkg/widgets/settings"
	"github.com/roffe/txlogger/pkg/widgets/symbollist"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	prefsLastBinFile = "lastBinFile"
	//prefsLastConfig     = "lastConfig"
	prefsSelectedECU    = "lastECU"
	prefsSymbolList     = "symbolList"
	prefsSelectedPreset = "selectedPreset"
)

// var _ desktop.Mouseable = (*SecretText)(nil)

/*
func (s *SecretText) MouseDown(e *desktop.MouseEvent) {
	log.Println("MouseDown", e)
}

func (s *SecretText) MouseUp(e *desktop.MouseEvent) {
	log.Println("MouseUp", e)
}
*/

type MainWindow struct {
	fyne.Window
	app             fyne.App
	menu            *MainMenu
	outputData      binding.StringList
	selects         *mainWindowSelects
	buttons         *mainWindowButtons
	counters        *mainWindowCounters
	loggingRunning  bool
	filename        string
	symbolList      *symbollist.Widget
	fw              symbol.SymbolCollection
	dlc             datalogger.IClient
	gwclient        proto.GocanClient
	buttonsDisabled bool
	settings        *settings.Widget
	statusText      *secrettext.SecretText
	wm              *multiwindow.MultipleWindows
	content         *fyne.Container
	startup         bool

	gocanGatewayLED *ledicon.Widget
	canLED          *ledicon.Widget

	previewFeatures bool
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
	capturedCounterLabel *widget.Label
	errorCounterLabel    *widget.Label
	fpsCounterLabel      *widget.Label
}

func NewMainWindow(app fyne.App) *MainWindow {
	symbolListConfig := &symbollist.Config{
		ColorBlindMode: colors.ModeNormal,
	}
	mw := &MainWindow{
		Window:     app.NewWindow("txlogger"),
		app:        app,
		outputData: binding.NewStringList(),

		counters: &mainWindowCounters{},
		selects:  &mainWindowSelects{},
		buttons:  &mainWindowButtons{},

		symbolList: symbollist.New(symbolListConfig),

		gocanGatewayLED: ledicon.New("Gateway"),
		canLED:          ledicon.New("CAN"),
		statusText:      secrettext.New("Harder, Better, Faster, Stronger"),
		previewFeatures: app.Preferences().BoolWithFallback("enable_preview_features", false),
	}

	mw.statusText.SecretFunc = func() {
		mw.app.Preferences().SetBool("enable_preview_features", true)
	}

	ebus.SubscribeFunc(ebus.TOPIC_COLORBLINDMODE, func(v float64) {
		mw.symbolList.SetColorBlindMode(colors.ColorBlindMode(int(v)))
		mw.symbolList.Refresh()
	})

	mw.setupMenu()
	mw.createButtons()
	mw.createSelects()
	mw.createCounters()
	mw.newSymbolnameTypeahead()
	mw.setupShortcuts()

	mw.settings = settings.New(&settings.Config{
		Logger: mw.Log,
		SelectedEcuFunc: func() string {
			return mw.selects.ecuSelect.Selected
		},
	})

	mw.loadPrefs()

	symbolListConfig.ColorBlindMode = mw.settings.GetColorBlindMode()

	mw.wm = multiwindow.NewMultipleWindows()
	mw.wm.LockViewport = true
	mw.wm.OnError = mw.Error

	mw.render()

	mw.Window.SetOnDropped(mw.onDropped)
	mw.SetCloseIntercept(mw.Close)
	mw.SetPadded(true)
	mw.SetContent(mw.content)
	mw.Resize(fyne.NewSize(1000, 700))
	mw.CenterOnScreen()
	mw.SetMaster()

	mw.whatsNew()

	mw.startup = true
	mw.buttons.symbolListBtn.OnTapped()
	mw.buttons.dashboardBtn.OnTapped()
	mw.startup = false

	if !fyne.CurrentApp().Driver().Device().IsMobile() {
		mw.gocanGatewayClient()
	}

	mw.updateCheck()

	return mw
}

func (mw *MainWindow) updateCheck() {
	nextUpdateCheck := mw.app.Preferences().String("nextUpdateCheck")
	if nextUpdateCheck == "" {
		nextUpdateCheck = time.Now().Add(336 * time.Hour).String()
		log.Println("nextUpdateCheck:", nextUpdateCheck)
		mw.app.Preferences().SetString("nextUpdateCheck", nextUpdateCheck)

	}
	nextCheckTime, _ := time.Parse(time.RFC3339, nextUpdateCheck)
	if time.Now().After(nextCheckTime) {
		dialog.ShowConfirm("It's been a while", "Do you want to check for updates to txlogger?", func(b bool) {
			if b {
				update.UpdateCheck(mw.app, mw.Window)
			}
			if tt, err := time.Now().Add(336 * time.Hour).MarshalText(); err == nil {
				log.Println("nextUpdateCheck:", string(tt))
				mw.app.Preferences().SetString("nextUpdateCheck", string(tt))
			}
		}, mw.Window)
	}
}

func (mw *MainWindow) gocanGatewayClient() {
	_, client, err := gocan.NewGRPCClient()
	if err != nil {
		mw.Error(fmt.Errorf("failed to connect to gocan gateway: %w", err))
		mw.gocanGatewayLED.Off()
		return
	}

	mw.gocanGatewayLED.On()
	res, err := client.GetAdapters(context.Background(), &emptypb.Empty{})
	if err != nil {
		mw.Error(fmt.Errorf("failed to get adapters from gocan gateway: %w", err))
		return
	}
	mw.settings.AddAdapters(res.Adapters)
	mw.gwclient = client
}

func (mw *MainWindow) setupShortcuts() {
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
		log.Println("open settings with Ctrl-1")
		mw.openSettings()
	})

	mw.Window.Canvas().AddShortcut(ctrl2, func(shortcut fyne.Shortcut) {
		log.Println("open symbol list with Ctrl-2")
		mw.buttons.symbolListBtn.OnTapped()
	})

	mw.Window.Canvas().AddShortcut(ctrl3, func(shortcut fyne.Shortcut) {
		log.Println("Ctrl-3")
	})

	mw.Window.Canvas().AddShortcut(ctrl4, func(shortcut fyne.Shortcut) {
		log.Println("Ctrl-4")
	})
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

	toolbar := mw.newToolbar()

	footer := container.NewBorder(
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
			container.NewHBox(
				mw.gocanGatewayLED,
				mw.canLED,
				mw.counters.capturedCounterLabel,
				mw.counters.errorCounterLabel,
				mw.counters.fpsCounterLabel,
			),
			widget.NewButtonWithIcon("", theme.ComputerIcon(), mw.openEBUSMonitor),
			mw.buttons.debugBtn,
		),
		mw.statusText,
	)

	mw.content = container.NewBorder(toolbar, footer, nil, nil, mw.wm)
}

func (mw *MainWindow) LoadLogfileCombined(filename string, reader io.ReadCloser, p fyne.Position, fromRoutine bool) {
	// Just filename, used for Window title
	fp := filepath.Base(filename)

	// if w := mw.wm.HasWindow(fp); w != nil {
	// 	mw.wm.Raise(w)
	// 	return
	// }

	logz, err := logfile.Open(filename, reader)
	if err != nil {
		mw.Error(fmt.Errorf("failed to open log file: %w", err))
		return
	}

	dbcfg := &dashboard.Config{
		Logplayer:       true,
		UseMPH:          mw.settings.GetUseMPH(),
		SwapRPMandSpeed: mw.settings.GetSwapRPMandSpeed(),
		High:            mw.settings.GetHigh(),
		Low:             mw.settings.GetLow(),
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

	do := func() {
		w := mw.app.NewWindow(fp)
		w.SetCloseIntercept(func() {
			cp.Close()
			w.Close()
		})
		w.Canvas().SetOnTypedKey(cp.TypedKey)
		w.SetContent(cp)
		w.Show()
	}

	if fromRoutine {
		fyne.Do(do)
	} else {
		do()
	}

	//w.Show()
	//mw.wm.Add(iw, p)
	mw.Log("loaded log file " + filename + " in combined logplayer")
}

func (mw *MainWindow) LoadLogfile(filename string, r io.Reader, pos fyne.Position) {
	// Just filename, used for Window title
	fp := filepath.Base(filename)

	if w := mw.wm.HasWindow(fp); w != nil {
		mw.wm.Raise(w)
		return
	}

	logz, err := logfile.Open(filename, r)
	if err != nil {
		mw.Error(fmt.Errorf("failed to open log file: %w", err))
		return
	}

	mw.Log("loaded log file " + filename)

	lp := logplayer.New(&logplayer.Config{
		EBus:    ebus.CONTROLLER,
		Logfile: logz,
	})
	/*
		content := container.NewBorder(
			container.NewHBox(
				widget.NewButton("Lambda", func() {

					mv, err := mapviewer.New(
						mapviewer.WithAxisLabels("RPM", "Airmass", "Lambda"),
					)
					if err != nil {
						mw.Error(fmt.Errorf("failed to create mapviewer: %w", err))
						return
					}

					iw := multiwindow.NewInnerWindow("Lambda feedback", mv)
					iw.Icon = theme.InfoIcon()

				}),
			),
			nil,
			nil,
			nil,
			lp,
		)
	*/
	iw := multiwindow.NewSystemWindow(fp, lp)
	iw.Icon = theme.MediaPlayIcon()

	lp.OnMouseDown = func() {
		mw.wm.Raise(iw)
	}
	iw.OnClose = func() {
		lp.Close()
	}
	mw.wm.Add(iw)
	m := iw.MinSize()
	pos2 := fyne.NewPos(pos.X-m.Width*0.5, pos.Y-m.Height*0.5)
	if pos2.X < 0 {
		pos2.X = 0
	}
	iw.Move(pos2)

}

func (mw *MainWindow) Log(s string) {
	debug.LogDepth(2, s)
	_ = mw.outputData.Append(s)
}

func (mw *MainWindow) Error(err error) {
	debug.LogDepth(2, err.Error())
	_ = mw.outputData.Append(err.Error())
	go fyne.Do(func() {
		dialog.ShowError(err, mw.Window)
	})
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

/*
func (mw *MainWindow) LoadSymbolsFromECU() error {
	device, err := mw.settings.GetAdapter(mw.selects.ecuSelect.Selected)
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
*/

func (mw *MainWindow) LoadSymbolsFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}
	ecuType, symbols, err := symbol.Load(filename, data, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.SetTitle(filepath.Base(filename))
	mw.app.Preferences().SetString(prefsLastBinFile, filename)

	mw.LoadSymbols(symbols, ecuType.String())
	//mw.selects.ecuSelect.SetSelected(ecuType.String())
	//mw.fw = symbols
	mw.filename = filename
	//mw.SyncSymbols()
	return nil
}

func (mw *MainWindow) LoadSymbolsFromBytes(filename string, data []byte) error {
	ecuType, symbols, err := symbol.Load(filename, data, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.SetTitle(filepath.Base(filename))
	mw.app.Preferences().SetString(prefsLastBinFile, filename)

	mw.LoadSymbols(symbols, ecuType.String())
	return nil
}

func (mw *MainWindow) LoadSymbols(symbols symbol.SymbolCollection, ecuType string) {
	mw.selects.ecuSelect.SetSelected(ecuType)
	mw.fw = symbols
	mw.SyncSymbols()
}

func (mw *MainWindow) LoadPreset(r io.Reader) error {
	b, err := io.ReadAll(r)
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
