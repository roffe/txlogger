package windows

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/mainmenu"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/logplayer"
	"github.com/roffe/txlogger/pkg/widgets/msglist"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
	"github.com/roffe/txlogger/pkg/widgets/settings"
	"github.com/roffe/txlogger/pkg/widgets/symbollist"
	sdialog "github.com/sqweek/dialog"
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

	outputData binding.StringList

	selects  *mainWindowSelects
	buttons  *mainWindowButtons
	counters *mainWindowCounters

	loggingRunning bool

	filename   string
	symbolList *symbollist.Widget
	fw         symbol.SymbolCollection

	dlc       datalogger.IClient
	dashboard *dashboard.Dashboard

	wm *windowManager

	buttonsDisabled bool

	settings *settings.SettingsWidget

	statusText *widget.Label

	oCtx *oto.Context

	content *fyne.Container
}

type mainWindowSelects struct {
	ecuSelect    *widget.Select
	presetSelect *widget.Select
}

type mainWindowButtons struct {
	addSymbolBtn      *widget.Button
	logBtn            *widget.Button
	loadSymbolsEcuBtn *widget.Button
	syncSymbolsBtn    *widget.Button
	dashboardBtn      *widget.Button
	logplayerBtn      *widget.Button
}

type mainWindowCounters struct {
	captureCounter       binding.Int
	capturedCounterLabel *widget.Label
	errorCounter         binding.Int
	errorCounterLabel    *widget.Label
	fpsCounter           binding.Int
	fpsLabel             *widget.Label
}

// Remember that you should **not** create more than one context
func newSound() *oto.Context {
	opts := &oto.NewContextOptions{
		// Usually 44100 or 48000. Other values might cause distortions in Oto
		SampleRate: 44100,
		// Number of channels (aka locations) to play sounds from. Either 1 or 2.
		// 1 is mono sound, and 2 is stereo (most speakers are stereo).
		ChannelCount: 2,
		// Format of the source. go-mp3's format is signed 16bit integers.
		Format: oto.FormatSignedInt16LE,
	}
	otoCtx, readyChan, err := oto.NewContext(opts)
	if err != nil {
		panic("oto.NewContext failed: " + err.Error())
	}
	// It might take a bit for the hardware audio devices to be ready, so we wait on the channel.
	select {
	case <-readyChan:
		return otoCtx
	case <-time.After(5 * time.Second):
		fyne.LogError("oto", errors.New("timeout waiting for audio device"))
		return nil
	}
}

func NewMainWindow(a fyne.App, filename string) *MainWindow {
	mw := &MainWindow{
		Window:     a.NewWindow("txlogger"),
		app:        a,
		outputData: binding.NewStringList(),

		counters: &mainWindowCounters{
			captureCounter: binding.NewInt(),
			errorCounter:   binding.NewInt(),
			fpsCounter:     binding.NewInt(),
		},

		selects: &mainWindowSelects{},
		buttons: &mainWindowButtons{},

		statusText: widget.NewLabel("Harder, Better, Faster, Stronger"),

		oCtx: newSound(),
	}

	mw.wm = newWindowManager(mw)
	mw.Window.SetPadded(true)

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

	mw.Window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyF12:
			capture.Screenshot(mw.Canvas())
		}
	})

	mw.Window.SetCloseIntercept(mw.CloseIntercept)

	mw.createButtons()

	mw.selects.presetSelect = widget.NewSelect(append([]string{"Select preset"}, presets.Names()...), func(s string) {
		if s == "Select preset" {
			return
		}
		preset, err := presets.Get(s)
		if err != nil {
			mw.Error(err)
			return
		}
		mw.symbolList.LoadSymbols(preset...)
		mw.SyncSymbols()
		mw.app.Preferences().SetString(prefsSelectedPreset, s)
	})
	mw.selects.presetSelect.Alignment = fyne.TextAlignLeading
	mw.selects.presetSelect.PlaceHolder = "Select preset"

	mw.newSymbolnameTypeahead()

	mw.counters.capturedCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.counters.captureCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.counters.captureCounter.Get(); err == nil {
			mw.counters.capturedCounterLabel.SetText(fmt.Sprintf("Cap: %d", val))
		}
	}))

	mw.counters.errorCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.counters.errorCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.counters.errorCounter.Get(); err == nil {
			mw.counters.errorCounterLabel.SetText(fmt.Sprintf("Err: %d", val))
		}
	}))

	mw.counters.fpsLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.counters.fpsCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.counters.fpsCounter.Get(); err == nil {
			mw.counters.fpsLabel.SetText(fmt.Sprintf("Fps: %d", val))
		}
	}))

	mw.selects.ecuSelect = widget.NewSelect([]string{"T5", "T7", "T8"}, func(s string) {
		mw.app.Preferences().SetString(prefsSelectedECU, s)
		mw.SetMainMenu(mw.menu.GetMenu(s))
	})

	mw.settings = settings.New(&settings.Config{
		EcuSelect: mw.selects.ecuSelect,
	})

	mw.loadPrefs(filename)
	if mw.fw == nil {
		mw.SetTitle("No symbols loaded")
	}

	mw.SetOnDropped(func(p fyne.Position, uris []fyne.URI) {
		log.Println("Dropped", uris)
		for _, u := range uris {
			filename := u.Path()
			switch strings.ToLower(path.Ext(filename)) {
			case ".bin":
				mw.LoadSymbolsFromFile(filename)
			case ".t5l", ".t7l", ".t8l", ".csv":
				a := NewLogPlayer(a, filename, mw.fw)
				a.Show()
			}
		}
	})

	opts, err := listLayouts()
	if err != nil {
		mw.Error(err)
	}

	var selEntry *widget.Select
	var selEntryRefreshBtn *widget.Button

	selEntry = widget.NewSelect(opts, func(s string) {
		if s == "" {
			return
		}
		switch s {
		case "Save Layout":
			if err := mw.wm.SaveLayout(); err != nil {
				mw.Error(err)
			}
			selEntry.ClearSelected()
			selEntryRefreshBtn.Tapped(&fyne.PointEvent{})
		default:
			if err := mw.wm.LoadLayout(s); err != nil {
				mw.Error(err)
			}
		}
	})

	selEntryRefreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		opts, err := listLayouts()
		if err != nil {
			mw.Error(err)
			return
		}
		selEntry.SetOptions(opts)

	})

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
				container.NewBorder(
					nil,
					nil,
					widget.NewLabel("Preset"),
					nil,
					mw.selects.presetSelect,
				),
				widget.NewSeparator(),
				widget.NewButtonWithIcon("Symbol list", theme.ListIcon(), func() {
					if mw.wm.HasWindow("Symbol list") {
						return
					}
					symbolListWindow := newInnerWindow("Symbol list", container.NewBorder(
						container.NewVBox(
							container.NewBorder(
								nil,
								nil,
								widget.NewIcon(theme.SearchIcon()),
								container.NewHBox(
									mw.buttons.addSymbolBtn,
									mw.buttons.loadSymbolsEcuBtn,
									mw.buttons.syncSymbolsBtn,
								),
								mw.symbolLookup,
							),
						),
						nil,
						nil,
						nil,
						mw.symbolList,
					))
					symbolListWindow.Icon = theme.ListIcon()
					mw.wm.Add(symbolListWindow)
					//symbolListWindow.Resize(fyne.NewSize(800, 700))
				}),
				mw.buttons.logBtn,
			),
			nil,
			container.NewHBox(
				mw.buttons.logplayerBtn,
				mw.buttons.dashboardBtn,
				widget.NewButtonWithIcon("", theme.GridIcon(), func() {
					mw.wm.MultipleWindows.Arrange(&multiwindow.GridArranger{})
				}),
				widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
					if mw.wm.HasWindow("Create gauge") {
						return
					}
					gs := NewGaugeCreator(mw)
					iw := newSystemWindow("Create gauge", gs)
					iw.Icon = theme.ContentAddIcon()
					mw.wm.Add(iw)
				}),
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
				selEntryRefreshBtn,
				selEntry,
			),
			container.NewGridWithColumns(5,
				mw.counters.capturedCounterLabel,
				mw.counters.errorCounterLabel,
				mw.counters.fpsLabel,
				widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
					//filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").SetStartDir(mw.settings.GetLogPath()).Load()
					filename, err := sdialog.File().Filter("logfile", "t5l", "t7l", "t8l", "csv").Load()
					if err != nil {
						if err.Error() == "Cancelled" {
							return
						}
						fyne.LogError("Error loading log file", err)
						return
					}

					// Just filename, used for Window title
					fp := filepath.Base(filename)

					if mw.wm.HasWindow(fp) {
						return
					}

					logz, err := logfile.Open(filename)
					if err != nil {
						fyne.LogError("Failed to open log file", err)
						return
					}

					lp := logplayer.New(logz, "pos_"+fp)
					iw := newSystemWindow(fp, lp)
					iw.Icon = theme.MediaPlayIcon()

					iw.CloseIntercept = func() {
						lp.Close()
					}

					mw.wm.Add(iw)
				}),
				widget.NewButtonWithIcon("Debug log", theme.InfoIcon(), func() {
					if mw.wm.HasWindow("Debug log") {
						return
					}
					dbl := msglist.New(mw.outputData)
					debugWindow := newSystemWindow("Debug log", dbl)
					debugWindow.Icon = theme.ContentCopyIcon()
					debugWindow.OnTappedIcon = func() {
						str, err := mw.outputData.Get()
						if err != nil {
							mw.Error(err)
							return
						}
						fyne.CurrentApp().Clipboard().SetContent(strings.Join(str, "\n"))
						dialog.ShowInformation("Debug log", "Content copied to clipboard", mw)
					}
					xy := mw.wm.MultipleWindows.Size().Subtract(dbl.MinSize().AddWidthHeight(20, 60))
					mw.wm.Add(debugWindow, fyne.NewPos(xy.Width, xy.Height))
				}),
			),
			mw.statusText,
		),
		nil,
		nil,
		mw.wm.MultipleWindows,
	)
	mw.SetContent(mw.content)
	mw.Resize(fyne.NewSize(1024, 768))
	mw.CenterOnScreen()
	mw.SetMaster()
	return mw
}

func (mw *MainWindow) CloseIntercept() {
	if mw.dlc != nil {
		mw.dlc.Close()
		time.Sleep(500 * time.Millisecond)
	}
	debug.Close()
	mw.Close()
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

func (mw *MainWindow) Disable() {
	mw.buttonsDisabled = true
	mw.buttons.addSymbolBtn.Disable()
	mw.buttons.loadSymbolsEcuBtn.Disable()
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
	mw.buttons.loadSymbolsEcuBtn.Enable()
	mw.buttons.syncSymbolsBtn.Enable()
	mw.buttons.logBtn.Enable()
	mw.selects.ecuSelect.Enable()
	mw.selects.presetSelect.Enable()
	mw.symbolList.Enable()
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
