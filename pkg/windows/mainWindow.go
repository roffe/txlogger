package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/roffe/t7logger/pkg/datalogger"
	"github.com/roffe/t7logger/pkg/kwp2000"
	"github.com/roffe/t7logger/pkg/sink"
	"github.com/roffe/t7logger/pkg/symbol"
	"github.com/roffe/t7logger/pkg/widgets"
	sdialog "github.com/sqweek/dialog"
)

const (
	prefsLastConfig = "lastConfig"
)

var (
	definedVars    = kwp2000.NewVarDefinitionList()
	loggingRunning bool
	dlc            *datalogger.Client
)

type MainWindow struct {
	W   fyne.Window
	app fyne.App
	//symbols   []*symbol.Symbol
	symbolMap map[string]*kwp2000.VarDefinition

	symbolLookup     *xwidget.CompletionEntry
	symbolConfigList *widget.List
	//	symbolConfigTable *widget.Table

	output     *widget.List
	outputData binding.StringList

	canSettings *widgets.CanSettingsWidget

	captureCounter binding.Int
	errorCounter   binding.Int
	freqValue      binding.Float
	progressBar    *widget.ProgressBarInfinite

	freqSlider *widget.Slider

	sinkManager *sink.Manager
}

func NewMainWindow(a fyne.App, singMgr *sink.Manager) *MainWindow {
	mw := &MainWindow{
		W:              a.NewWindow("Trionic7 Logger - No file loaded"),
		app:            a,
		symbolMap:      make(map[string]*kwp2000.VarDefinition),
		outputData:     binding.NewStringList(),
		canSettings:    widgets.NewCanSettingsWidget(a),
		captureCounter: binding.NewInt(),
		errorCounter:   binding.NewInt(),
		freqValue:      binding.NewFloat(),
		progressBar:    widget.NewProgressBarInfinite(),
		sinkManager:    singMgr,
	}

	mw.freqSlider = widget.NewSliderWithData(1, 50, mw.freqValue)
	mw.freqSlider.SetValue(25)
	mw.progressBar.Stop()

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
				mw.writeOutput(err.Error())
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)
	/*
		mw.symbolConfigTable = widget.NewTable(
			func() (int, int) {
				return definedVars.Len(), 1
			},
			func() fyne.CanvasObject {
				return widgets.NewVarDefinitionWidget(mw.symbolConfigList, definedVars)
			},
			func(tci widget.TableCellID, co fyne.CanvasObject) {
				coo := co.(*widgets.VarDefinitionWidget)
				coo.Update(tci.Row, definedVars.GetPos(tci.Row))
			},
		)
	*/
	mw.symbolConfigList = widget.NewList(
		func() int {
			return definedVars.Len()
		},
		func() fyne.CanvasObject {
			return widgets.NewVarDefinitionWidget(mw.symbolConfigList, definedVars)
		},
		func(lii widget.ListItemID, co fyne.CanvasObject) {
			coo := co.(*widgets.VarDefinitionWidget)
			coo.Update(lii, definedVars.GetPos(lii))
		},
	)
	//if err := mw.loadSymbolsFromFile("EU0DF21C_55P(YS3EB55A843014952).bin"); err != nil {
	//	dialog.ShowError(err, mw.W)
	//}

	mw.symbolLookup = xwidget.NewCompletionEntry([]string{})

	// When the use typed text, complete the list.
	mw.symbolLookup.OnChanged = func(s string) {
		// completion start for text length >= 3
		if len(s) < 3 {
			mw.symbolLookup.HideCompletion()
			return
		}

		// Get the list of possible completion
		var results []string

		for _, sym := range mw.symbolMap {
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(s)) {
				results = append(results, sym.Name)
				//log.Println(sym)
			}
		}
		// no results
		if len(results) == 0 {
			mw.symbolLookup.HideCompletion()
			return
		}
		sort.Slice(results, func(i, j int) bool { return strings.ToLower(results[i]) < strings.ToLower(results[j]) })

		// then show them
		mw.symbolLookup.SetOptions(results)
		mw.symbolLookup.ShowCompletion()
	}

	if filename := mw.app.Preferences().String(prefsLastConfig); filename != "" {
		mw.LoadConfig(filename)
	}

	return mw
}

func (mw *MainWindow) SaveConfig(filename string) error {
	b, err := json.Marshal(definedVars.Get())
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %w", err)
	}
	if err := os.WriteFile(filename, b, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	mw.app.Preferences().SetString(prefsLastConfig, filename)
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
	definedVars.Set(cfg)
	mw.app.Preferences().SetString(prefsLastConfig, filename)
	return nil
}

func (mw *MainWindow) setTitle(str string) {
	mw.W.SetTitle("Trionic7 Logger - " + str)
}

func (mw *MainWindow) Layout() fyne.CanvasObject {
	var logBtn *widget.Button
	logBtn = widget.NewButtonWithIcon("Start logging", theme.DownloadIcon(), func() {
		if loggingRunning {
			if dlc != nil {
				dlc.Close()
			}
			return
		}
		if !loggingRunning {
			device, err := mw.canSettings.GetAdapter(mw.writeOutput)
			if err != nil {
				dialog.ShowError(err, mw.W)
				return
			}
			logBtn.SetText("Stop logging")
			dlc = datalogger.New(datalogger.Config{
				Dev:            device,
				Variables:      definedVars,
				Freq:           int(mw.freqSlider.Value),
				OnMessage:      mw.writeOutput,
				CaptureCounter: mw.captureCounter,
				ErrorCounter:   mw.errorCounter,
				Sink:           mw.sinkManager,
			})

			go func() {
				loggingRunning = true
				mw.progressBar.Start()
				if err := dlc.Start(); err != nil {
					dialog.ShowError(err, mw.W)
				}
				mw.progressBar.Stop()
				loggingRunning = false
				dlc = nil
				logBtn.SetText("Start logging")
			}()
		}
	})
	left := container.NewBorder(
		container.NewBorder(
			nil,
			nil,
			nil,
			container.NewHBox(
				widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
					defer mw.symbolConfigList.Refresh()
					s, ok := mw.symbolMap[mw.symbolLookup.Text]
					if !ok {
						//definedVars = append(definedVars, &kwp2000.VarDefinition{
						//	Name: mw.symbolLookup.Text,
						//})
						definedVars.Add(&kwp2000.VarDefinition{
							Name: mw.symbolLookup.Text,
						})
						return
					}
					//definedVars = append(definedVars, s)
					definedVars.Add(s)
					log.Printf("Name: %s, Method: %d, Value: %d, Type: %X", s.Name, s.Method, s.Value, s.Type)
				}),
				widget.NewButtonWithIcon("Load binary", theme.FileIcon(), func() {
					filename, err := sdialog.File().Filter("Binary file", "bin").Load()
					if err != nil {
						if err.Error() == "Cancelled" {
							return
						}
						dialog.ShowError(err, mw.W)
					}
					mw.loadSymbolsFromFile(filename)
				}),
			),

			mw.symbolLookup,
		),
		container.NewVBox(
			container.NewGridWithColumns(3,
				widget.NewButtonWithIcon("Load config", theme.FileIcon(), func() {
					filename, err := sdialog.File().Filter("Config file", "json").Load()
					if err != nil {
						if err.Error() == "Cancelled" {
							return
						}
						dialog.ShowError(err, mw.W)
					}
					if err := mw.LoadConfig(filename); err != nil {
						dialog.ShowError(err, mw.W)
						return
					}
					mw.symbolConfigList.Refresh()
				}),
				widget.NewButtonWithIcon("Sync symbols with binary", theme.ViewRefreshIcon(), func() {

					for i, v := range definedVars.Get() {

						//if sym, ok := mw.symbolMap[v.Name]; ok {
						//	definedVars.UpdatePos(i, sym)
						//}

						for k, vv := range mw.symbolMap {
							//if strings.ToLower(k) == strings.ToLower(v.Name) {
							if strings.EqualFold(k, v.Name) {
								definedVars.UpdatePos(i, vv)
								break
							}
						}

					}
					mw.symbolConfigList.Refresh()
				}),
				widget.NewButtonWithIcon("Save config", theme.DocumentSaveIcon(), func() {

					filename, err := sdialog.File().Filter("Config file", "json").Save()
					if err != nil {
						if err.Error() == "Cancelled" {
							return
						}
						dialog.ShowError(err, mw.W)
					}
					if err := mw.SaveConfig(filename); err != nil {
						dialog.ShowError(err, mw.W)
						return

					}
				}),
			),
		),
		nil,
		nil,
		mw.symbolConfigList,
		//mw.symbolConfigTable,
	)

	capturedCounter := widget.NewLabel("")
	capturedCounter.Alignment = fyne.TextAlignLeading

	errorCounter := widget.NewLabel("")
	errorCounter.Alignment = fyne.TextAlignLeading

	mw.captureCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.captureCounter.Get(); err == nil {
			capturedCounter.SetText(fmt.Sprintf("Cap: %d", val))
		}
	}))

	mw.errorCounter.AddListener(binding.NewDataListener(func() {
		if val, err := mw.errorCounter.Get(); err == nil {
			errorCounter.SetText(fmt.Sprintf("Err: %d", val))
		}
	}))

	freqValue := widget.NewLabel("")

	mw.freqValue.AddListener(binding.NewDataListener(func() {
		if val, err := mw.freqValue.Get(); err == nil {
			freqValue.SetText(fmt.Sprintf("Freq: %0.f", val))
		}
	}))

	rSplit := &container.Split{
		Offset:     0,
		Horizontal: false,
		Leading: container.NewVBox(
			mw.canSettings,
			logBtn,
			mw.progressBar,
		),
		Trailing: &container.Split{
			Offset:     1,
			Horizontal: false,
			Leading:    mw.output,
			Trailing: container.NewVBox(
				mw.freqSlider,
				container.NewGridWithColumns(3,
					capturedCounter,
					errorCounter,
					freqValue,
				),
			),
		},
	}

	split := &container.Split{
		Offset:     1,
		Horizontal: true,
		Leading:    left,
		Trailing:   rSplit,
	}
	return split

}

func (mw *MainWindow) loadSymbolsFromFile(filename string) error {
	newSymbols, err := symbol.LoadSymbols(filename)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	mw.symbolMap = make(map[string]*kwp2000.VarDefinition)
	for _, s := range newSymbols {

		def := &kwp2000.VarDefinition{
			Name:             s.Name,
			Method:           kwp2000.VAR_METHOD_SYMBOL,
			Value:            s.Number,
			Type:             s.Type,
			Length:           s.Length,
			Correctionfactor: symbol.GetCorrectionfactor(s.Name),
		}

		mw.symbolMap[s.Name] = def
	}
	mw.setTitle(filename)
	return nil
}

func (mw *MainWindow) writeOutput(s string) {
	mw.outputData.Append(s)
	mw.output.ScrollToBottom()
}
