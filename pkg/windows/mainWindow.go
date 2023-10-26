package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/roffe/txlogger/pkg/capture"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/ecu"
	"github.com/roffe/txlogger/pkg/interpolate"
	"github.com/roffe/txlogger/pkg/kwp2000"
	"github.com/roffe/txlogger/pkg/layout"
	"github.com/roffe/txlogger/pkg/presets"
	"github.com/roffe/txlogger/pkg/symbol"
	"github.com/roffe/txlogger/pkg/widgets"
	sdialog "github.com/sqweek/dialog"
	"golang.org/x/net/context"
)

const (
	prefsLastConfig  = "lastConfig"
	prefsSelectedECU = "lastECU"
	prefsSymbolList  = "symbolList"
)

type MainWindow struct {
	fyne.Window
	app fyne.App

	menu *MainMenu

	symbolMap map[string]*kwp2000.VarDefinition

	symbolLookup     *xwidget.CompletionEntry
	symbolConfigList *widget.List

	output     *widget.List
	outputData binding.StringList

	canSettings *widgets.CanSettingsWidget

	ecuSelect *widget.Select

	addSymbolBtn *widget.Button
	logBtn       *widget.Button
	//mockBtn            *widget.Button

	loadSymbolsEcuBtn  *widget.Button
	loadSymbolsFileBtn *widget.Button
	syncSymbolsBtn     *widget.Button

	dashboardBtn *widget.Button

	//mapSelector *widget.Tree
	//fuelBtn     *widget.Button
	//ignitionBtn *widget.Button

	logplayerBtn *widget.Button
	logfolderBtn *widget.Button
	helpBtn      *widget.Button

	loadConfigBtn *widget.Button
	saveConfigBtn *widget.Button
	presetSelect  *widget.Select
	symbolsHeader *fyne.Container

	captureCounter        binding.Int
	errorCounter          binding.Int
	errorPerSecondCounter binding.Int
	freqValue             binding.Float
	//progressBar           *widget.ProgressBarInfinite

	freqSlider *widget.Slider

	capturedCounterLabel     *widget.Label
	errorCounterLabel        *widget.Label
	errPerSecondCounterLabel *widget.Label
	freqValueLabel           *widget.Label

	//sinkManager *sink.Manager

	loggingRunning bool
	//mockRunning    bool

	dlc  datalogger.Logger
	vars *kwp2000.VarDefinitionList

	symbols symbol.SymbolCollection

	dashboard *widgets.Dashboard
	//metricChan chan *model.DashboardMetric
	buttonsDisabled bool

	leading *fyne.Container

	openMaps map[string]MapViewerWindowInterface

	mvh *MapViewerHandler
}

func NewMainWindow(a fyne.App, vars *kwp2000.VarDefinitionList) *MainWindow {
	mw := &MainWindow{
		Window:                a.NewWindow("txlogger"),
		app:                   a,
		symbolMap:             make(map[string]*kwp2000.VarDefinition),
		outputData:            binding.NewStringList(),
		canSettings:           widgets.NewCanSettingsWidget(a),
		captureCounter:        binding.NewInt(),
		errorCounter:          binding.NewInt(),
		errorPerSecondCounter: binding.NewInt(),
		freqValue:             binding.NewFloat(),
		//progressBar:           widget.NewProgressBarInfinite(),
		//sinkManager:           singMgr,
		vars:     vars,
		openMaps: make(map[string]MapViewerWindowInterface),
		mvh:      NewMapViewerHandler(),
	}

	mw.setupMenu()

	mw.Window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		//log.Println(ev.Name)
		switch ev.Name {
		case fyne.KeyF12:
			capture.Screenshot(mw.Canvas())
		}
	})

	/*
		quitChan := make(chan os.Signal, 2)
		signal.Notify(quitChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-quitChan
			if mw.dlc != nil {
				mw.dlc.Close()
			}
			mw.app.Quit()
			}()
	*/

	mw.Window.SetCloseIntercept(func() {
		mw.SaveSymbolList()
		debug.Close()
		if mw.dlc != nil {
			mw.dlc.Close()
		}
		mw.Close()
	})

	mw.createButtons()

	mw.presetSelect = &widget.Select{
		Alignment:   fyne.TextAlignCenter,
		PlaceHolder: "Select preset",
		Options:     append([]string{"Select preset"}, presets.Names()...),

		OnChanged: func(s string) {
			preset, ok := presets.Map[s]
			if !ok {
				return
			}
			if err := mw.LoadConfigFromString(preset); err != nil {
				mw.Log(err.Error())
				return
			}
			mw.symbolConfigList.Refresh()
			mw.presetSelect.SetSelected("Select preset")
			mw.SyncSymbols()
		},
	}

	//mw.progressBar.Stop()

	mw.freqSlider = widget.NewSliderWithData(1, 120, mw.freqValue)
	mw.freqValue.Set(25)

	mw.symbolConfigList = widget.NewList(
		func() int {
			return mw.vars.Len()
		},
		func() fyne.CanvasObject {
			disabled := mw.dlc != nil
			return widgets.NewVarDefinitionWidgetEntry(mw.symbolConfigList, mw.vars, mw.SaveSymbolList, disabled)
		},
		func(lii widget.ListItemID, co fyne.CanvasObject) {
			coo := co.(*widgets.VarDefinitionWidgetEntry)
			coo.Update(lii, mw.vars.GetPos(lii))
			if !mw.buttonsDisabled {
				coo.Enable()
				return
			}
			coo.Disable()
		},
	)

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

	mw.errPerSecondCounterLabel = &widget.Label{
		Alignment: fyne.TextAlignLeading,
	}
	mw.errorPerSecondCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.errorPerSecondCounter.Get(); err == nil {
			mw.errPerSecondCounterLabel.SetText(fmt.Sprintf("Err/s: %d", val))
		}
	}))

	mw.freqValueLabel = widget.NewLabel("")
	mw.freqValue.AddListener(binding.NewDataListener(func() {
		if val, err := mw.freqValue.Get(); err == nil {
			mw.freqValueLabel.SetText(fmt.Sprintf("Freq: %0.f", val))
		}
	}))

	mw.ecuSelect = widget.NewSelect([]string{"T7", "T8"}, func(s string) {
		mw.app.Preferences().SetString(prefsSelectedECU, s)
		mw.SetMainMenu(mw.menu.GetMenu(s))
	})

	mw.symbolsHeader = container.New(&layout.RatioContainer{
		Widths: []float32{
			.35, // name
			.12, // value
			.14, // method
			.12, // number
			//.08, // type
			//.06, // signed
			.11, // correctionfactor
			.08, // deletebtn
		},
	},
		&widget.Label{
			Text:      "Name",
			Alignment: fyne.TextAlignLeading,
		},
		&widget.Label{
			Text:      "Value",
			Alignment: fyne.TextAlignLeading,
		},
		&widget.Label{
			Text:      "Method",
			Alignment: fyne.TextAlignLeading,
		},
		&widget.Label{
			Text:      "#",
			Alignment: fyne.TextAlignLeading,
		},
		//&widget.Label{
		//	Text:      "Type",
		//	Alignment: fyne.TextAlignLeading,
		//},
		//&widget.Label{
		//	Text:      "Signed",
		//	Alignment: fyne.TextAlignLeading,
		//},
		&widget.Label{
			Text:      "Factor",
			Alignment: fyne.TextAlignLeading,
		},
	)

	mw.loadPrefs()

	mw.setTitle("No symbols loaded")
	mw.SetMaster()
	mw.Resize(fyne.NewSize(1024, 768))
	mw.SetContent(mw.Render())

	return mw
}

func (mw *MainWindow) setupMenu() {
	menus := []*fyne.Menu{
		fyne.NewMenu("File",
			fyne.NewMenuItem("Load binary", func() {
				filename, err := sdialog.File().Filter("Binary file", "bin").Load()
				if err != nil {
					if err.Error() == "Cancelled" {
						return
					}
					// dialog.ShowError(err, mw)
					mw.Log(err.Error())
					return
				}
				if err := mw.LoadSymbolsFromFile(filename); err != nil {
					// dialog.ShowError(err, mw)
					mw.Log(err.Error())
					return
				}
				mw.SyncSymbols()
			}),
			fyne.NewMenuItem("Load log", func() {
				filename, err := sdialog.File().Filter("trionic logfile", "t7l", "t8l").SetStartDir("logs").Load()
				if err != nil {
					if err.Error() == "Cancelled" {
						return
					}
					// dialog.ShowError(err, mw)
					mw.Log(err.Error())
					return
				}

				onClose := func() {
					if mw.dlc != nil {
						mw.dlc.Detach(mw.dashboard)
					}
					mw.dashboard = nil
					mw.SetFullScreen(false)
					mw.SetContent(mw.Content())
				}
				go NewLogPlayer(mw.app, filename, mw.symbols, onClose)
			}),
		),
	}
	mw.menu = NewMainMenu(mw, menus, mw.openMap, mw.openMapz)
}

func (mw *MainWindow) loadPrefs() {
	if cfg := mw.app.Preferences().String(prefsSymbolList); cfg != "" {
		mw.LoadConfigFromString(cfg)
	}

	if ecu := mw.app.Preferences().StringWithFallback(prefsSelectedECU, "T7"); ecu != "" {
		mw.ecuSelect.SetSelected(ecu)
	}
}

func (mw *MainWindow) setTitle(str string) {
	meta := mw.app.Metadata()
	mw.SetTitle(fmt.Sprintf("txlogger v%s Build %d - %s", meta.Version, meta.Build, str))
}

func (mw *MainWindow) Render() *container.Split {
	mw.leading = container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil,
				nil,
				//widget.NewLabel("Search"),
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
		container.NewVBox(
			container.NewGridWithColumns(3,
				mw.loadConfigBtn,
				mw.saveConfigBtn,
				mw.presetSelect,
			),
		),
		nil,
		nil,
		container.NewBorder(
			mw.symbolsHeader,
			nil,
			nil,
			nil,
			mw.symbolConfigList,
		),
	)

	return &container.Split{
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
				mw.canSettings,
				mw.logBtn,
				//mw.progressBar,
			),
			Trailing: &container.Split{
				Offset:     1,
				Horizontal: false,
				Leading:    mw.output,
				Trailing: container.NewVBox(
					mw.dashboardBtn,
					container.NewGridWithColumns(2,
						mw.logplayerBtn,
						mw.logfolderBtn,
					),
					mw.helpBtn,
					mw.freqSlider,
					container.NewGridWithColumns(4,
						mw.capturedCounterLabel,
						mw.errorCounterLabel,
						mw.errPerSecondCounterLabel,
						mw.freqValueLabel,
					),
				),
			},
		},
	}

}

func (mw *MainWindow) Log(s string) {
	debug.Log(s)
	mw.outputData.Append(s)
	mw.output.Refresh()
	mw.output.ScrollToBottom()
}

func (mw *MainWindow) SaveSymbolList() {
	b, err := json.Marshal(mw.vars.Get())
	if err != nil {
		// dialog.ShowError(err, mw)
		mw.Log(err.Error())
		return
	}
	mw.app.Preferences().SetString(prefsSymbolList, string(b))
}

func (mw *MainWindow) SaveConfig(filename string) error {
	b, err := json.Marshal(mw.vars.Get())
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %w", err)
	}
	if err := os.WriteFile(filename, b, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func (mw *MainWindow) LoadConfig(filename string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg []*kwp2000.VarDefinition
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	mw.vars.Set(cfg)
	mw.app.Preferences().SetString(prefsSymbolList, string(b))
	return nil
}

func (mw *MainWindow) LoadConfigFromString(str string) error {
	var cfg []*kwp2000.VarDefinition
	if err := json.Unmarshal([]byte(str), &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	mw.vars.Set(cfg)
	mw.SaveSymbolList()
	return nil
}

func (mw *MainWindow) SyncSymbols() {
	if len(mw.symbolMap) == 0 {
		// dialog.ShowError(errors.New("Load bin to sync symbols"), mw.Window) //lint:ignore ST1005 ignore error
		mw.Log("Load bin to sync symbols")
		return
	}
	cnt := 0
	for i, v := range mw.vars.Get() {
		for k, vv := range mw.symbolMap {
			if strings.EqualFold(k, v.Name) {
				mw.vars.UpdatePos(i, vv)
				cnt++
				break
			}
		}
	}
	mw.symbolConfigList.Refresh()
	mw.SaveSymbolList()
	mw.Log(fmt.Sprintf("Synced %d / %d symbols", cnt, mw.vars.Len()))
}

func (mw *MainWindow) Content() fyne.CanvasObject {
	return mw.Render()
}

func (mw *MainWindow) DisableBtns() {
	mw.buttonsDisabled = true
	mw.addSymbolBtn.Disable()
	mw.loadConfigBtn.Disable()
	mw.saveConfigBtn.Disable()
	mw.loadSymbolsFileBtn.Disable()
	mw.loadSymbolsEcuBtn.Disable()
	mw.syncSymbolsBtn.Disable()
	if !mw.loggingRunning {
		mw.logBtn.Disable()
	}
	//	mw.mockBtn.Disable()
	mw.ecuSelect.Disable()
	mw.canSettings.Disable()
	mw.presetSelect.Disable()
	for _, v := range mw.vars.Get() {
		if v.Widget != nil {
			v.Widget.(*widgets.VarDefinitionWidgetEntry).Disable()
		}
	}
}

func (mw *MainWindow) EnableBtns() {
	mw.buttonsDisabled = false
	mw.addSymbolBtn.Enable()
	mw.loadConfigBtn.Enable()
	mw.saveConfigBtn.Enable()
	mw.loadSymbolsFileBtn.Enable()
	mw.loadSymbolsEcuBtn.Enable()
	mw.syncSymbolsBtn.Enable()
	mw.logBtn.Enable()
	//	mw.mockBtn.Enable()
	mw.ecuSelect.Enable()
	mw.canSettings.Enable()
	mw.presetSelect.Enable()
	for _, v := range mw.vars.Get() {
		if v.Widget != nil {
			v.Widget.(*widgets.VarDefinitionWidgetEntry).Enable()
		}
	}
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
		mw.loadSymbols(symbols)
	case "T8":
		symbols, err := ecu.GetSymbolsT8(ctx, device, mw.Log)
		if err != nil {
			return err
		}
		mw.loadSymbols(symbols)
	}

	mw.setTitle("Symbols loaded from ECU " + time.Now().Format("2006-01-02 15:04:05.000"))
	return nil
}

func (mw *MainWindow) LoadSymbolsFromFile(filename string) error {
	ecuType, symbols, err := symbol.LoadSymbols(filename, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.ecuSelect.SetSelected(ecuType.String())
	os.WriteFile("symbols.txt", []byte(symbols.Dump()), 0644)
	mw.loadSymbols(symbols)
	mw.setTitle(filename)
	return nil
}

func (mw *MainWindow) loadSymbols(symbols symbol.SymbolCollection) {
	newSymbolMap := make(map[string]*kwp2000.VarDefinition)
	for _, s := range symbols.Symbols() {
		newSymbolMap[s.Name] = &kwp2000.VarDefinition{
			Name:             s.Name,
			Method:           kwp2000.VAR_METHOD_SYMBOL,
			Value:            s.Number,
			Type:             s.Type,
			Length:           s.Length,
			Correctionfactor: s.Correctionfactor,
			Unit:             s.Unit,
		}
	}
	mw.symbolMap = newSymbolMap
	mw.symbols = symbols
}

func (mw *MainWindow) newOutputList() {
	mw.output = widget.NewListWithData(
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
				mw.Log(err.Error())
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)
}

func (mw *MainWindow) newSymbolnameTypeahead() {
	mw.symbolLookup = xwidget.NewCompletionEntry([]string{})
	mw.symbolLookup.PlaceHolder = "Type to search for symbols"
	// When the use typed text, complete the list.
	mw.symbolLookup.OnChanged = func(s string) {
		// completion start for text length >= 3
		if len(s) < 3 && s != "*" {
			mw.symbolLookup.HideCompletion()
			return
		}
		// Get the list of possible completion
		var results []string
		for _, sym := range mw.symbolMap {
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) || s == "*" {
				results = append(results, sym.Name)
			}
		}
		// no results
		if len(results) == 0 {
			mw.symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		// then show them
		if len(results) > 0 {
			mw.symbolLookup.SetOptions(results)
			mw.symbolLookup.ShowCompletion()
		}
	}
}

func (mw *MainWindow) openMapz(typ symbol.ECUType, mapNames ...string) {
	joinedNames := strings.Join(mapNames, "|")
	mv, found := mw.openMaps[joinedNames]
	if !found {
		w := mw.app.NewWindow("Map Viewer - " + strings.Join(mapNames, ", "))
		//w.SetFixedSize(true)
		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		view, err := widgets.NewMapViewerMulti(typ, mw.symbols, mapNames...)
		if err != nil {
			mw.Log(err.Error())
			return
		}
		mw.openMaps[joinedNames] = mw.newMapViewerWindow(w, view, symbol.Axis{})

		for _, mv := range view.Children() {
			mw.mvh.Subscribe(mv.Info().XFrom, mv)
			mw.mvh.Subscribe(mv.Info().YFrom, mv)
		}

		w.SetCloseIntercept(func() {
			log.Println("closing", joinedNames)
			delete(mw.openMaps, joinedNames)
			for _, mv := range view.Children() {
				mw.mvh.Unsubscribe(mv.Info().XFrom, mv)
				mw.mvh.Unsubscribe(mv.Info().YFrom, mv)
			}

			w.Close()
		})

		w.SetContent(view)
		w.Show()
		return
	}
	mv.RequestFocus()
}

func (mw *MainWindow) openMap(typ symbol.ECUType, mapName string) {
	axis := symbol.GetInfo(typ, mapName)
	//log.Printf("openMap: %s %s %s", axis.X, axis.Y, axis.Z)
	mv, found := mw.openMaps[axis.Z]
	if !found {
		w := mw.app.NewWindow("Map Viewer - " + axis.Z)
		//w.SetFixedSize(true)
		if mw.symbols == nil {
			mw.Log("No binary loaded")
			return
		}
		xData, yData, zData, xCorrFac, yCorrFac, zCorrFac, err := mw.symbols.GetXYZ(axis.X, axis.Y, axis.Z)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		mv, err := widgets.NewMapViewer(
			widgets.WithXData(xData),
			widgets.WithYData(yData),
			widgets.WithZData(zData),
			widgets.WithXCorrFac(xCorrFac),
			widgets.WithYCorrFac(yCorrFac),
			widgets.WithZCorrFac(zCorrFac),
			widgets.WithXFrom(axis.XFrom),
			widgets.WithYFrom(axis.YFrom),
			widgets.WithInterPolFunc(interpolate.Interpolate),
		)
		if err != nil {
			mw.Log(err.Error())
			return
		}

		w.Canvas().SetOnTypedKey(mv.TypedKey)

		w.SetCloseIntercept(func() {
			log.Println("closing", axis.Z)
			delete(mw.openMaps, axis.Z)
			mw.mvh.Unsubscribe(axis.XFrom, mv)
			mw.mvh.Unsubscribe(axis.YFrom, mv)

			w.Close()
		})

		mw.openMaps[axis.Z] = mw.newMapViewerWindow(w, mv, axis)
		w.SetContent(mv)
		w.Show()

		return
	}
	mv.RequestFocus()
}
