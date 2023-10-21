package windows

import (
	"encoding/json"
	"fmt"
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

	mapSelector *widget.Tree
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

	dlc  datalogger.DataClient
	vars *kwp2000.VarDefinitionList

	symbols symbol.SymbolCollection

	dashboard *widgets.Dashboard
	//metricChan chan *model.DashboardMetric
	buttonsDisabled bool

	leading *fyne.Container

	openMaps map[string]*widgets.MapViewer
}

func NewMainWindow(a fyne.App, vars *kwp2000.VarDefinitionList) *MainWindow {
	mw := &MainWindow{
		Window:                a.NewWindow("TrionicLogger"),
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
		openMaps: make(map[string]*widgets.MapViewer),
	}

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
				// dialog.ShowError(err, mw)
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
			//log.Println("newList: creating new VarDefinitionWidget")
			return widgets.NewVarDefinitionWidgetEntry(mw.symbolConfigList, mw.vars, mw.SaveSymbolList, disabled)
		},
		func(lii widget.ListItemID, co fyne.CanvasObject) {
			coo := co.(*widgets.VarDefinitionWidgetEntry)
			coo.Update(lii, mw.vars.GetPos(lii))
			if !mw.buttonsDisabled {
				coo.Enable()
			} else {
				coo.Disable()
			}

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
	})

	mw.symbolsHeader = container.New(&ratioContainer{
		widths: []float32{
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
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
		&widget.Label{
			Text:      "Value",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
		&widget.Label{
			Text:      "Method",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
		&widget.Label{
			Text:      "#",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
		//&widget.Label{
		//	Text:      "Type",
		//	TextStyle: fyne.TextStyle{Monospace: true},
		//	Alignment: fyne.TextAlignCenter,
		//},
		//&widget.Label{
		//	Text:      "Signed",
		//	TextStyle: fyne.TextStyle{Monospace: true},
		//	Alignment: fyne.TextAlignCenter,
		//},
		&widget.Label{
			Text:      "Factor",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
	)

	mw.mapSelector = widget.NewTreeWithStrings(mapToTreeMap(symbol.T7SymbolsTuning))
	mw.mapSelector.OnSelected = func(uid widget.TreeNodeID) {
		//		log.Printf("%q", uid)
		if uid == "" || !strings.Contains(uid, ".") {
			if !mw.mapSelector.IsBranchOpen(uid) {
				mw.mapSelector.OpenBranch(uid)
			} else {
				mw.mapSelector.CloseBranch(uid)
			}
			mw.mapSelector.UnselectAll()
			return

		}
		mw.openMap(symbol.GetInfo(symbol.ECU_T7, uid))
		mw.mapSelector.UnselectAll()
	}

	/*
		widget.NewSelect([]string{"Select map", "Fuel", "Ignition"}, func(s string) {
			if s == "Select map" {
				return
			}
			mw.openShort(s)
		})
		mw.mapSelector.SetSelectedIndex(0)
	*/

	menu := fyne.NewMainMenu(
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
		),
	)

	mw.loadPrefs()
	mw.setTitle("No symbols loaded")
	mw.Window.SetMainMenu(menu)
	mw.Resize(fyne.NewSize(1280, 720))

	return mw
}

func mapToTreeMap(data map[string][]string) map[string][]string {
	result := make(map[string][]string)

	// Sort map keys to maintain a consistent order
	var sortedKeys []string
	for key := range data {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		id := "" + key
		result[""] = append(result[""], id)
		items := data[key]
		// Sort the items if necessary
		sort.Strings(items)
		result[id] = append(result[id], items...)
	}

	return result
}

type ratioContainer struct {
	widths []float32
}

func (d *ratioContainer) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, 34)
}

func (d *ratioContainer) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var x float32
	for i, o := range objects {
		az := size.Width * d.widths[i]
		o.Resize(fyne.NewSize(az, size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += o.Size().Width + size.Width*.015
	}
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

func (mw *MainWindow) Layout() *container.Split {
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

	bottomRightSplit := container.NewVSplit(
		container.New(&layout.MinHeight{Height: 170}, mw.mapSelector),
		container.NewVBox(
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
	)

	bottomRightSplit.SetOffset(1)

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
				Offset:     0.4,
				Horizontal: false,
				Leading:    mw.output,
				Trailing:   bottomRightSplit,
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
	return mw.Layout()
}

func (mw *MainWindow) disableBtns() {
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

func (mw *MainWindow) enableBtns() {
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

func (mw *MainWindow) loadSymbolsFromECU() error {
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
	symbols, err := symbol.LoadSymbols(filename, mw.ecuSelect.Selected, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	//os.WriteFile("symbols.txt", []byte(symbols.Dump()), 0644)
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
