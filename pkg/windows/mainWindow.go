package windows

import (
	"encoding/json"
	"fmt"
	"os"
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
	dashboardBtn       *widget.Button
	logplayerBtn       *widget.Button
	helpBtn            *widget.Button

	loadConfigBtn *widget.Button
	saveConfigBtn *widget.Button
	//syncSymbolsBtn *widget.Button
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

	dashboard *Dashboard
	//metricChan chan *model.DashboardMetric
	buttonsDisabled bool
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
		vars: vars,
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

	mw.addSymbolBtn = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		defer mw.symbolConfigList.Refresh()
		s, ok := mw.symbolMap[mw.symbolLookup.Text]
		if !ok {
			mw.vars.Add(&kwp2000.VarDefinition{
				Name: mw.symbolLookup.Text,
			})
			return
		}
		mw.vars.Add(s)
		mw.SaveSymbolList()
		//log.Printf("Name: %s, Method: %d, Value: %d, Type: %X", s.Name, s.Method, s.Value, s.Type)
	})

	mw.loadSymbolsFileBtn = widget.NewButtonWithIcon("Load from binary", theme.FileIcon(), func() {
		filename, err := sdialog.File().Filter("Binary file", "bin").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		if err := mw.loadSymbolsFromFile(filename); err != nil {
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		mw.SyncSymbols()
	})

	mw.loadSymbolsEcuBtn = widget.NewButtonWithIcon("Load from ECU", theme.DownloadIcon(), func() {
		//		mw.progressBar.Start()
		mw.disableBtns()
		go func() {
			defer mw.enableBtns()
			//		defer mw.progressBar.Stop()
			if err := mw.loadSymbolsFromECU(); err != nil {
				// dialog.ShowError(err, mw)
				mw.Log(err.Error())
				return
			}
			mw.SyncSymbols()
		}()
	})

	mw.loadConfigBtn = widget.NewButtonWithIcon("Load config", theme.FileIcon(), func() {
		filename, err := sdialog.File().Filter("*.json", "json").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		if err := mw.LoadConfig(filename); err != nil {
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		mw.symbolConfigList.Refresh()
		mw.SyncSymbols()
	})

	mw.saveConfigBtn = widget.NewButtonWithIcon("Save config", theme.DocumentSaveIcon(), func() {
		filename, err := sdialog.File().Filter("json", "json").Save()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		if !strings.HasSuffix(filename, ".json") {
			filename += ".json"
		}
		if err := mw.SaveConfig(filename); err != nil {
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return

		}
	})

	mw.helpBtn = widget.NewButtonWithIcon("Help", theme.HelpIcon(), func() {
		Help(mw.app)
	})

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
	/*
		mw.syncSymbolsBtn = widget.NewButtonWithIcon("Sync symbols", theme.ViewRefreshIcon(), func() {
			if len(mw.symbolMap) == 0 {
				dialog.ShowError(errors.New("Load symbols first"), mw.Window) //lint:ignore ST1005 ignore error
				return
			}
			for i, v := range mw.vars.Get() {
				for k, vv := range mw.symbolMap {
					if strings.EqualFold(k, v.Name) {
						mw.vars.UpdatePos(i, vv)
						break
					}
				}
			}
			mw.symbolConfigList.Refresh()
		})
	*/

	mw.dashboardBtn = widget.NewButtonWithIcon("Dashboard", theme.InfoIcon(), func() {
		mw.dashboard = NewDashboard(mw, false, mw.logBtn)
		if mw.dlc != nil {
			mw.dlc.AttachDashboard(mw.dashboard)
		}
		mw.SetContent(mw.dashboard.Content())
	})

	mw.logplayerBtn = widget.NewButtonWithIcon("Log Player", theme.MediaFastForwardIcon(), func() {
		filename, err := sdialog.File().Filter("trionic logfile", "t7l", "t8l").Load()
		if err != nil {
			if err.Error() == "Cancelled" {
				return
			}
			// dialog.ShowError(err, mw)
			mw.Log(err.Error())
			return
		}
		NewLogPlayer(mw.app, filename, mw)
	})

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
			return widgets.NewVarDefinitionWidget(mw.symbolConfigList, mw.vars, mw.SaveSymbolList, disabled)
		},
		func(lii widget.ListItemID, co fyne.CanvasObject) {
			coo := co.(*widgets.VarDefinitionWidget)
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
	mw.newLogBtn()

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

	/*
		mw.symbolsHeader = container.NewHBox(
			widgets.FixedWidth(197, &widget.Label{
				Text:      "Name",
				Alignment: fyne.TextAlignLeading,
			}),
			widgets.FixedWidth(70, &widget.Label{
				Text:      "Value",
				Alignment: fyne.TextAlignLeading,
			}),
			widgets.FixedWidth(90, &widget.Label{
				Text:      "Method",
				Alignment: fyne.TextAlignLeading,
			}),
			widgets.FixedWidth(44, &widget.Label{
				Text:      "#",
				Alignment: fyne.TextAlignLeading,
			}),
			widgets.FixedWidth(28, &widget.Label{
				Text:      "Type",
				Alignment: fyne.TextAlignLeading,
			}),
			widgets.FixedWidth(40, &widget.Label{
				Text:      "Signed",
				Alignment: fyne.TextAlignLeading,
			}),
			widgets.FixedWidth(55, &widget.Label{
				Text:      "Factor",
				Alignment: fyne.TextAlignLeading,
			}),
			//widgets.FixedWidth(110, &widget.Label{
			//	Text:      "Group",
			//	Alignment: fyne.TextAlignLeading,
			//}),
		)
	*/
	mw.symbolsHeader = container.New(&ratioContainer{
		widths: []float32{
			.30, // name
			.10, // value
			.12, // method
			.08, // number
			.08, // type
			.06, // signed
			.10, // correctionfactor
			.05, // deletebtn
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
		&widget.Label{
			Text:      "Type",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
		&widget.Label{
			Text:      "Signed",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
		&widget.Label{
			Text:      "Factor",
			TextStyle: fyne.TextStyle{Monospace: true},
			Alignment: fyne.TextAlignCenter,
		},
	)

	mw.loadPrefs()
	mw.setTitle("No symbols loaded")

	mw.Resize(fyne.NewSize(1280, 720))

	return mw
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

func (mw *MainWindow) Layout() fyne.CanvasObject {
	return &container.Split{
		Offset:     0.68,
		Horizontal: true,
		Leading: container.NewBorder(
			container.NewVBox(
				container.NewBorder(
					nil,
					nil,
					widget.NewLabel("Symbol lookup"),
					container.NewHBox(
						mw.addSymbolBtn,
						mw.loadSymbolsFileBtn,
						mw.loadSymbolsEcuBtn,
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
		),
		Trailing: &container.Split{
			Offset:     0,
			Horizontal: false,
			Leading: container.NewVBox(
				container.NewBorder(
					nil,
					nil,
					widgets.FixedWidth(75, widget.NewLabel("ECU")),
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
					mw.logplayerBtn,
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
	for i, v := range mw.vars.Get() {
		for k, vv := range mw.symbolMap {
			if strings.EqualFold(k, v.Name) {
				mw.vars.UpdatePos(i, vv)
				break
			}
		}
	}
	mw.symbolConfigList.Refresh()
}

func (mw *MainWindow) Content() fyne.CanvasObject {
	return mw.Layout()
}

func (mw *MainWindow) disableBtns() {
	mw.buttonsDisabled = true
	mw.addSymbolBtn.Disable()
	mw.loadConfigBtn.Disable()
	mw.saveConfigBtn.Disable()
	//mw.syncSymbolsBtn.Disable()
	mw.loadSymbolsFileBtn.Disable()
	mw.loadSymbolsEcuBtn.Disable()
	if !mw.loggingRunning {
		mw.logBtn.Disable()
	}
	//	mw.mockBtn.Disable()
	mw.ecuSelect.Disable()
	mw.canSettings.Disable()
	mw.presetSelect.Disable()
	for _, v := range mw.vars.Get() {
		if v.Widget != nil {
			v.Widget.(*widgets.VarDefinitionWidget).Disable()
		}
	}
}

func (mw *MainWindow) enableBtns() {
	mw.buttonsDisabled = false
	mw.addSymbolBtn.Enable()
	mw.loadConfigBtn.Enable()
	mw.saveConfigBtn.Enable()
	//mw.syncSymbolsBtn.Enable()
	mw.loadSymbolsFileBtn.Enable()
	mw.loadSymbolsEcuBtn.Enable()
	mw.logBtn.Enable()
	//	mw.mockBtn.Enable()
	mw.ecuSelect.Enable()
	mw.canSettings.Enable()
	mw.presetSelect.Enable()
	for _, v := range mw.vars.Get() {
		if v.Widget != nil {
			v.Widget.(*widgets.VarDefinitionWidget).Enable()
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

func (mw *MainWindow) loadSymbolsFromFile(filename string) error {
	symbols, err := symbol.LoadSymbols(filename, mw.Log)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.loadSymbols(symbols)
	mw.setTitle(filename)
	return nil
}

func (mw *MainWindow) loadSymbols(symbols []*symbol.Symbol) {
	newSymbolMap := make(map[string]*kwp2000.VarDefinition)
	for _, s := range symbols {
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
}
