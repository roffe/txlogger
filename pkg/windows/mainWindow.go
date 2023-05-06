package windows

import (
	"encoding/json"
	"fmt"
	"os"
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

type MainWindow struct {
	fyne.Window
	app fyne.App

	symbolMap map[string]*kwp2000.VarDefinition

	symbolLookup     *xwidget.CompletionEntry
	symbolConfigList *widget.List

	output     *widget.List
	outputData binding.StringList

	canSettings *widgets.CanSettingsWidget

	logBtn  *widget.Button
	mockBtn *widget.Button

	captureCounter binding.Int
	errorCounter   binding.Int
	freqValue      binding.Float
	progressBar    *widget.ProgressBarInfinite

	freqSlider *widget.Slider

	sinkManager *sink.Manager

	loggingRunning bool
	mockRunning    bool

	dlc  *datalogger.Client
	vars *kwp2000.VarDefinitionList
}

func NewMainWindow(a fyne.App, singMgr *sink.Manager, vars *kwp2000.VarDefinitionList) *MainWindow {
	mw := &MainWindow{
		Window:         a.NewWindow("Trionic7 Logger - No file loaded"),
		app:            a,
		symbolMap:      make(map[string]*kwp2000.VarDefinition),
		outputData:     binding.NewStringList(),
		canSettings:    widgets.NewCanSettingsWidget(a),
		captureCounter: binding.NewInt(),
		errorCounter:   binding.NewInt(),
		freqValue:      binding.NewFloat(),
		progressBar:    widget.NewProgressBarInfinite(),
		sinkManager:    singMgr,
		vars:           vars,
	}
	mw.progressBar.Stop()

	mw.freqSlider = widget.NewSliderWithData(1, 50, mw.freqValue)
	mw.freqSlider.SetValue(25)

	mw.output = mw.newOutputList()
	mw.symbolLookup = mw.newSymbolnameTypeahead()
	mw.logBtn = mw.newLogBtn()
	mw.mockBtn = mw.newMockBtn()

	if filename := mw.app.Preferences().String(prefsLastConfig); filename != "" {
		mw.LoadConfig(filename)
	}

	return mw
}

func (mw *MainWindow) setTitle(str string) {
	mw.SetTitle("Trionic7 Logger - " + str)
}

func (mw *MainWindow) Layout() fyne.CanvasObject {
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
						mw.vars.Add(&kwp2000.VarDefinition{
							Name: mw.symbolLookup.Text,
						})
						return
					}
					mw.vars.Add(s)
					//log.Printf("Name: %s, Method: %d, Value: %d, Type: %X", s.Name, s.Method, s.Value, s.Type)
				}),
				widget.NewButtonWithIcon("Load binary", theme.FileIcon(), func() {
					filename, err := sdialog.File().Filter("Binary file", "bin").Load()
					if err != nil {
						if err.Error() == "Cancelled" {
							return
						}
						dialog.ShowError(err, mw)
					}
					mw.loadSymbolsFromFile(filename)
				}),
			),

			mw.symbolLookup,
		),
		container.NewVBox(
			container.NewGridWithColumns(4,
				widget.NewButtonWithIcon("Load config", theme.FileIcon(), func() {
					filename, err := sdialog.File().Filter("Config file", "json").Load()
					if err != nil {
						if err.Error() == "Cancelled" {
							return
						}
						dialog.ShowError(err, mw)
					}
					if err := mw.LoadConfig(filename); err != nil {
						dialog.ShowError(err, mw)
						return
					}
					mw.symbolConfigList.Refresh()
				}),
				widget.NewButtonWithIcon("Sync symbols with binary", theme.ViewRefreshIcon(), func() {
					for i, v := range mw.vars.Get() {
						for k, vv := range mw.symbolMap {
							if strings.EqualFold(k, v.Name) {
								mw.vars.UpdatePos(i, vv)
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
						dialog.ShowError(err, mw)
					}
					if err := mw.SaveConfig(filename); err != nil {
						dialog.ShowError(err, mw)
						return

					}
				}),
				widget.NewButtonWithIcon("Dashboard", theme.InfoIcon(), func() {
					NewDashboard(mw.app).Show()
				}),
			),
		),
		nil,
		nil,
		mw.symbolConfigList,
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

	return &container.Split{
		Offset:     0.6,
		Horizontal: true,
		Leading:    left,
		Trailing: &container.Split{
			Offset:     0,
			Horizontal: false,
			Leading: container.NewVBox(
				mw.canSettings,
				mw.logBtn,
				mw.progressBar,
			),
			Trailing: &container.Split{
				Offset:     1,
				Horizontal: false,
				Leading:    mw.output,
				Trailing: container.NewVBox(
					mw.mockBtn,
					mw.freqSlider,
					container.NewGridWithColumns(3,
						capturedCounter,
						errorCounter,
						freqValue,
					),
				),
			},
		},
	}

}

func (mw *MainWindow) loadSymbolsFromFile(filename string) error {
	symbols, err := symbol.LoadSymbols(filename)
	if err != nil {
		return fmt.Errorf("error loading symbols: %w", err)
	}
	newSymbolMap := make(map[string]*kwp2000.VarDefinition)
	for _, s := range symbols {
		def := &kwp2000.VarDefinition{
			Name:             s.Name,
			Method:           kwp2000.VAR_METHOD_SYMBOL,
			Value:            s.Number,
			Type:             s.Type,
			Length:           s.Length,
			Correctionfactor: symbol.GetCorrectionfactor(s.Name),
		}
		newSymbolMap[s.Name] = def
	}
	mw.symbolMap = newSymbolMap
	mw.setTitle(filename)
	return nil
}

func (mw *MainWindow) writeOutput(s string) {
	mw.outputData.Append(s)
	mw.output.ScrollToBottom()
}

func (mw *MainWindow) SaveConfig(filename string) error {
	b, err := json.Marshal(mw.vars.Get())
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
	mw.vars.Set(cfg)
	mw.app.Preferences().SetString(prefsLastConfig, filename)
	return nil
}
