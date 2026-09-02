package windows

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/ecusymbol/as2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/j2534proxy/client"
	"github.com/roffe/txlogger/pkg/common"
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
	"github.com/roffe/txlogger/pkg/widgets/seedkey"
	"github.com/roffe/txlogger/pkg/widgets/settings"
	"github.com/roffe/txlogger/pkg/widgets/shortcuts"
	"github.com/roffe/txlogger/pkg/widgets/symbollist"
)

const (
	prefsLastBinFile = "lastBinFile"
	prefsSelectedECU = "lastECU"
	prefsRemoteMode  = "remoteMode"
	prefsWindowW     = "windowWidth"
	prefsWindowH     = "windowHeight"
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
	app                         fyne.App
	leadingMenus, trailingMenus []*fyne.Menu
	recentItem                  *fyne.MenuItem
	outputData                  binding.StringList
	selects                     *mainWindowSelects
	buttons                     *mainWindowButtons
	counters                    *mainWindowCounters
	loggingRunning              bool
	filename                    string
	symbolList                  *symbollist.Viewer

	as2 *as2.File
	fw  symbol.FirmwareFile

	// logz is the most recently opened log file, kept so tools that work on
	// "the log" (currently the AI chat) have something to read. Several log
	// players can be open at once; this is the last one loaded.
	logz        logfile.Logfile
	logFilename string

	// seedKey is the T7 SecurityAccess pair read out of the loaded binary,
	// tried first when logging so ECUs with a patched/custom algorithm unlock.
	seedKey *t7kwp.SeedKey

	dlc             datalogger.IClient
	dlcCancel       context.CancelFunc
	buttonsDisabled bool
	settings        *settings.Widget
	statusText      *widget.Label
	wm              *multiwindow.MultipleWindows
	content         *fyne.Container
	startup         bool

	j2534LED *ledicon.Widget
	canLED   *ledicon.Widget

	keyBindings []shortcuts.Binding

	previewFeatures bool
}

type mainWindowSelects struct {
	ecuSelect    *widget.Select
	layoutSelect *widget.Select
	remoteSelect *widget.Select
}

type mainWindowButtons struct {
	debugBtn     *widget.Button
	logBtn       *widget.Button
	dashboardBtn *widget.Button

	layoutRefreshBtn *widget.Button
	symbolListBtn    *widget.Button
	addGaugeBtn      *widget.Button
	livePlotBtn      *widget.Button
}

type mainWindowCounters struct {
	capturedCounterLabel *widget.Label
	errorCounterLabel    *widget.Label
	fpsCounterLabel      *widget.Label
}

func NewMainWindow(app fyne.App) *MainWindow {
	mw := &MainWindow{
		Window:     app.NewWindow("txlogger"),
		app:        app,
		outputData: binding.NewStringList(),

		counters: &mainWindowCounters{},
		selects:  &mainWindowSelects{},
		buttons:  &mainWindowButtons{},

		j2534LED:        ledicon.New("J2534"),
		canLED:          ledicon.New("CAN"),
		statusText:      widget.NewLabel("Harder, Better, Faster, Stronger"),
		previewFeatures: app.Preferences().BoolWithFallback("enable_preview_features1337", false),
	}

	mw.symbolList = symbollist.NewViewer(&symbollist.ViewerConfig{
		App:    app,
		Window: mw.Window,
		ECU:    func() string { return mw.selects.ecuSelect.Selected },
		Log:    mw.Log,
		Error:  mw.Error,
	})

	mw.setupMenu()
	mw.createButtons()
	mw.createSelects()
	mw.createCounters()
	mw.setupShortcuts()

	mw.settings = settings.New(&settings.Config{
		Logger: mw.Log,
		SelectedEcuFunc: func() string {
			return mw.selects.ecuSelect.Selected
		},
	})

	mw.loadPrefs()

	mw.symbolList.SetColorBlindMode(mw.settings.GetColorBlindMode())
	mw.symbolList.UpdateBars(mw.settings.GetRealtimeBars())

	mw.wm = multiwindow.NewMultipleWindows()
	mw.wm.LockViewport = true
	mw.wm.OnError = mw.Error

	mw.render()

	mw.Window.SetOnDropped(mw.onDropped)
	mw.SetCloseIntercept(mw.Close)
	mw.SetPadded(true)
	mw.SetContent(mw.content)
	/*
		// ponytail: size only, Fyne has no API to get or set window position
		p := app.Preferences()
		mw.Resize(fyne.NewSize(
			float32(p.FloatWithFallback(prefsWindowW, 1000)),
			float32(p.FloatWithFallback(prefsWindowH, 700)),
		))
		mw.CenterOnScreen()
	*/
	mw.SetMaster()

	mw.whatsNew()

	mw.startup = true
	mw.buttons.symbolListBtn.OnTapped()
	mw.buttons.dashboardBtn.OnTapped()
	mw.startup = false

	if !fyne.CurrentApp().Driver().Device().IsMobile() {
		mw.startJ2534Proxy()
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

// startJ2534Proxy launches the 32-bit J2534 helper next to the executable
// and folds the DLL adapters it serves into the settings adapter list. The
// proxy keeps itself alive only as long as txlogger runs (stdin lifeline +
// pings), so there is nothing to tear down on exit.
func (mw *MainWindow) startJ2534Proxy() {
	if runtime.GOOS != "windows" {
		return
	}
	go func() {
		_, adapters, err := client.Start(context.Background(), debug.Log)
		if err != nil {
			debug.Log("j2534proxy: " + err.Error())
			fyne.Do(mw.j2534LED.Off)
			return
		}
		fyne.Do(func() {
			mw.j2534LED.On()
			mw.settings.AddAdapters(adapters)
		})
	}()
}

func (mw *MainWindow) setupShortcuts() {
	ctrlEnter := &desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierControl}
	altEnter := &desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierAlt}
	mw.Window.Canvas().AddShortcut(ctrlEnter, func(shortcut fyne.Shortcut) {
		mw.wm.Arrange(&multiwindow.GridArranger{})
	})

	mw.Window.Canvas().AddShortcut(altEnter, func(shortcut fyne.Shortcut) {
		mw.Window.SetFullScreen(!mw.Window.FullScreen())
	})

	mw.applyShortcuts(shortcuts.Load())
}

// applyShortcuts takes the user-configured shortcuts into use, replacing the
// previous set. The menu is rebuilt because that is where they live.
func (mw *MainWindow) applyShortcuts(bindings []shortcuts.Binding) {
	mw.keyBindings = bindings
	mw.SetMainMenu(mw.GetMenu(mw.selects.ecuSelect.Selected))
}

// shortcutMenu holds the user-configured shortcuts that apply to the given
// ECU. They are menu items rather than canvas shortcuts because menu shortcuts
// are matched before the focused widget gets a shot: a focused map viewer
// would otherwise swallow them, so hopping between two maps with Ctrl-1/Ctrl-2
// would only work once. The menu is rebuilt on every ECU switch, which is what
// makes per-ECU bindings work.
func (mw *MainWindow) shortcutMenu(ecu string) *fyne.Menu {
	items := make([]*fyne.MenuItem, 0, len(mw.keyBindings))
	for _, b := range mw.keyBindings {
		if b.Key == "" || !b.AppliesTo(ecu) {
			continue
		}
		item := fyne.NewMenuItem(b.Label(), func() { mw.runShortcut(b) })
		item.Shortcut = &desktop.CustomShortcut{KeyName: b.Key, Modifier: b.Modifier}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	return fyne.NewMenu("Shortcuts", items...)
}

func (mw *MainWindow) runShortcut(b shortcuts.Binding) {
	switch b.Action {
	case shortcuts.ActionSettings:
		mw.openSettings()
	case shortcuts.ActionSymbolList:
		mw.buttons.symbolListBtn.OnTapped()
	case shortcuts.ActionMap:
		// Raise an already open viewer instead of leaning on openMap's own
		// check, which looks for the bare symbol name while the window it
		// creates is titled "<symbol> - <description>".
		for _, w := range mw.wm.Windows() {
			if t := w.Title(); t == b.Target || strings.HasPrefix(t, b.Target+" - ") {
				mw.wm.Raise(w)
				return
			}
		}
		mw.openMap(symbol.ECUTypeFromString(mw.selects.ecuSelect.Selected), "", b.Target, "")
	case shortcuts.ActionLayout:
		if err := mw.LoadLayout(b.Target); err != nil {
			mw.Error(err)
		}
	}
}

func (mw *MainWindow) render() {
	toolbar := mw.newToolbar()
	footer := container.NewBorder(
		nil,
		nil,
		nil,
		container.NewHBox(
			container.NewHBox(
				mw.j2534LED,
				mw.canLED,
				mw.counters.capturedCounterLabel,
				mw.counters.errorCounterLabel,
				mw.counters.fpsCounterLabel,
			),
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
	mw.logz, mw.logFilename = logz, fp

	dbcfg := &dashboard.Config{
		Logplayer:      true,
		UseMPH:         mw.settings.GetUseMPH(),
		ClassicGauges:  mw.settings.GetClassicGauges(),
		High:           1.5,
		Low:            0.5,
		WidebandSymbol: mw.settings.GetWidebandSymbolName(),
	}

	rec := logz.Next()
	if !rec.EOF {
		for k := range rec.Values {
			if k == "AirMassMast.m_Request" {
				dbcfg.AirDemToString = datalogger.AirDemToStringT8
				break
			} else if k == "Lufttemp" {
				// T5
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
	// iw := multiwindow.NewSystemWindow(fp, cp)
	// iw.Icon = theme.MediaPlayIcon()

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

	// w.Show()
	// mw.wm.Add(iw, p)
	mw.Log("loaded log file " + filename + " in combined logplayer")
	mw.addRecent(filename)
}

func (mw *MainWindow) LoadAS2File(filename string) error {
	f, err := as2.Load(filename)
	if err != nil {
		return fmt.Errorf("failed to load AS2 file: %w", err)
	}
	mw.as2 = f
	mw.Log("Loaded AS2 file " + filename)
	return nil
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

	mw.logz, mw.logFilename = logz, fp

	mw.Log("loaded log file " + filename)
	mw.addRecent(filename)

	lp := logplayer.New(&logplayer.Config{
		EBus:            ebus.CONTROLLER,
		Logfile:         logz,
		PlotterRenderer: mw.settings.GetPlotterRenderer(),
		OnExport: func(records []logfile.Record) {
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fp)), ".")
			prefix := strings.TrimSuffix(fp, filepath.Ext(fp)) + "-clip"
			logPath := mw.settings.GetLogPath()
			go func() {
				path, err := datalogger.ExportRecords(logPath, prefix, ext, records)
				fyne.Do(func() {
					if err != nil {
						mw.Error(fmt.Errorf("failed to export selection: %w", err))
						return
					}
					mw.Log(fmt.Sprintf("exported %d samples to %s", len(records), path))
					dialog.ShowInformation("Selection exported", fmt.Sprintf("Saved %d samples to\n%s", len(records), path), mw)
				})
			}()
		},
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
	if pos2.Y < 0 {
		pos2.Y = 80
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
		// dialog.ShowError(err, mw.Window)
		common.ShowError("Dang it!", err)
	})
	// log.Printf("error: %s", err)
}

func (mw *MainWindow) Disable() {
	mw.buttonsDisabled = true
	if !mw.loggingRunning {
		mw.buttons.logBtn.Disable()
	}

	mw.selects.ecuSelect.Disable()
	mw.selects.remoteSelect.Disable()

	mw.symbolList.Disable()
}

func (mw *MainWindow) Enable() {
	mw.buttonsDisabled = false
	mw.buttons.logBtn.Enable()

	mw.selects.ecuSelect.Enable()
	mw.selects.remoteSelect.Enable()

	mw.symbolList.Enable()
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
	mw.addRecent(filename)

	mw.captureSeedKey(ecuType, data)
	mw.LoadSymbols(symbols, ecuType.String())
	// mw.selects.ecuSelect.SetSelected(ecuType.String())
	// mw.fw = symbols
	mw.filename = filename
	// mw.SyncSymbols()
	return nil
}

func (mw *MainWindow) LoadSymbolsFromBytes(filename string, data []byte) error {
	ecuType, symbols, err := symbol.Load(filename, data, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.SetTitle(filepath.Base(filename))
	mw.app.Preferences().SetString(prefsLastBinFile, filename)

	mw.captureSeedKey(ecuType, data)
	mw.LoadSymbols(symbols, ecuType.String())
	return nil
}

// captureSeedKey reads the T7 SecurityAccess algorithm (XOR/SUB) out of the
// loaded binary so logging can unlock ECUs flashed with a patched/custom
// algorithm. Cleared for non-T7 or when the routine isn't found (falls back to
// the stock pairs).
func (mw *MainWindow) captureSeedKey(ecuType symbol.ECUType, data []byte) {
	mw.seedKey = nil
	if ecuType != symbol.ECU_T7 {
		return
	}
	_, xor, sub, err := seedkey.Find(data)
	if err != nil {
		return
	}
	mw.seedKey = &t7kwp.SeedKey{XOR: xor, Sub: sub}
	mw.Log(fmt.Sprintf("Seed/key from binary: XOR %04X SUB %04X (%s)", xor, sub, seedkey.MethodName(xor, sub)))
}

func (mw *MainWindow) LoadSymbols(symbols symbol.FirmwareFile, ecuType string) {
	mw.selects.ecuSelect.SetSelected(ecuType)
	mw.fw = symbols
	mw.symbolList.SetSymbols(symbols)
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
