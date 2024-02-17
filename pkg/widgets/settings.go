package widgets

import (
	"errors"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ecumaster"
	sdialog "github.com/sqweek/dialog"
)

const (
	prefsFreq              = "freq"
	prefsAutoUpdateLoadEcu = "autoUpdateLoadEcu"
	prefsAutoUpdateSaveEcu = "autoUpdateSaveEcu"
	prefsLivePreview       = "livePreview"
	prefsMeshView          = "liveMeshView"
	prefsRealtimeBars      = "realtimeBars"
	prefsLogFormat         = "logFormat"
	prefsLogPath           = "logPath"
	prefsLambdaSource      = "lambdaSource"
	prefsUseMPH            = "useMPH"
)

type SettingsWidget struct {
	widget.BaseWidget

	freqSlider   *widget.Slider
	freqValue    *widget.Label
	autoSave     *widget.Check
	autoLoad     *widget.Check
	livePreview  *widget.Check
	meshView     *widget.Check
	realtimeBars *widget.Check
	logFormat    *widget.Select
	logPath      *widget.Label
	lambdaSource *widget.Select
	useMPH       *widget.Check

	container *fyne.Container

	OnClose func()
}

func (sw *SettingsWidget) GetLambdaSource() string {
	return sw.lambdaSource.Selected
}

func (sw *SettingsWidget) GetLambdaSymbolName() string {
	switch sw.lambdaSource.Selected {
	case "ECU":
		return "DisplProt.LambdaScanner"
	case ecumaster.ProductString:
		return datalogger.EXTERNALWBLSYM
	default:
		return "DisplProt.LambdaScanner"
	}
}

func (sw *SettingsWidget) GetFreq() int {
	return int(sw.freqSlider.Value)
}

func (sw *SettingsWidget) GetAutoSave() bool {
	return sw.autoSave.Checked
}

func (sw *SettingsWidget) GetAutoLoad() bool {
	return sw.autoLoad.Checked
}

func (sw *SettingsWidget) GetLivePreview() bool {
	return sw.livePreview.Checked
}

func (sw *SettingsWidget) GetRealtimeBars() bool {
	return sw.realtimeBars.Checked
}

func (sw *SettingsWidget) GetMeshView() bool {
	return sw.meshView.Checked
}

func (sw *SettingsWidget) GetLogFormat() string {
	return sw.logFormat.Selected
}

func (sw *SettingsWidget) GetLogPath() string {
	return sw.logPath.Text
}

func (sw *SettingsWidget) GetUseMPH() bool {
	return sw.useMPH.Checked
}

func NewSettingsWidget() *SettingsWidget {
	sw := &SettingsWidget{}
	sw.ExtendBaseWidget(sw)

	sw.freqSlider = sw.newFreqSlider()
	sw.freqValue = widget.NewLabel("")

	sw.autoSave = sw.newAutoUpdateSave()
	sw.autoLoad = sw.newAutoUpdateLoad()
	sw.livePreview = sw.newLivePreview()
	sw.meshView = sw.newMeshView()
	sw.realtimeBars = sw.newRealtimeBars()

	sw.logFormat = sw.newLogFormat()
	sw.logPath = widget.NewLabel("")

	lambdaSel := sw.newLambdaSelector()
	sw.useMPH = sw.newUserMPH()

	sw.container =
		container.NewVBox(
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Logging rate (Hz)"),
				sw.freqValue,
				sw.freqSlider,
			),
			widget.NewSeparator(),
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Log format"),
				nil,
				sw.logFormat,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Log folder"),
				container.NewGridWithColumns(2,
					widget.NewButtonWithIcon("Reset", theme.ContentClearIcon(), func() {
						sw.logPath.SetText(datalogger.LOGPATH)
						fyne.CurrentApp().Preferences().SetString(prefsLogPath, datalogger.LOGPATH)
					}),
					widget.NewButtonWithIcon("Browse", theme.FileIcon(), func() {
						dir, err := sdialog.Directory().Title("Select log folder").Browse()
						if err != nil {
							if errors.Is(err, sdialog.ErrCancelled) {
								return
							}
							log.Println(err)
							return
						}
						sw.logPath.SetText(dir)
						fyne.CurrentApp().Preferences().SetString(prefsLogPath, dir)
					}),
				),
				sw.logPath,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.InfoIcon()),
				nil,
				sw.autoLoad,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.WarningIcon()),
				nil,
				sw.autoSave,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.SearchIcon()),
				nil,
				container.NewVBox(
					sw.livePreview,
					sw.realtimeBars,
					lambdaSel,
				),
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.ViewFullScreenIcon()),
				nil,
				sw.meshView,
			),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.InfoIcon()),
				nil,
				sw.useMPH,
			),
		)
	sw.loadPrefs()
	return sw
}

func (sw *SettingsWidget) newLogFormat() *widget.Select {
	return widget.NewSelect([]string{"CSV", "TXL"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLogFormat, s)
	})
}

func (sw *SettingsWidget) newLambdaSelector() *fyne.Container {
	sw.lambdaSource = widget.NewSelect([]string{"ECU", ecumaster.ProductString}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLambdaSource, s)
	})
	return container.NewBorder(
		nil,
		nil,
		widget.NewLabel("Lambda source"),
		nil,
		sw.lambdaSource,
	)
}

func (sw *SettingsWidget) newFreqSlider() *widget.Slider {
	slider := widget.NewSlider(5, 100)
	slider.OnChanged = func(f float64) {
		sw.freqValue.SetText(strconv.FormatFloat(f, 'f', 0, 64))
	}
	slider.OnChangeEnded = func(f float64) {
		fyne.CurrentApp().Preferences().SetInt(prefsFreq, int(f))
	}

	return slider
}

func (sw *SettingsWidget) newMeshView() *widget.Check {
	return widget.NewCheck("3D Mesh on map viewing", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsMeshView, b)
	})
}

func (sw *SettingsWidget) newAutoUpdateLoad() *widget.Check {
	return widget.NewCheck("Load maps from ECU when connected (X & Y-Axis will still be loaded from binary)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateLoadEcu, b)
	})
}

func (sw *SettingsWidget) newAutoUpdateSave() *widget.Check {
	return widget.NewCheck("Upload map changes directly when editing if connected to ECU (requires open bin)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateSaveEcu, b)
	})
}

func (sw *SettingsWidget) newLivePreview() *widget.Check {
	return widget.NewCheck("Live preview of values in symbol list (uncheck this if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsLivePreview, b)
	})
}

func (sw *SettingsWidget) newRealtimeBars() *widget.Check {
	return widget.NewCheck("Bars on live preview of values (uncheck this if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsRealtimeBars, b)
	})
}

func (sw *SettingsWidget) newUserMPH() *widget.Check {
	return widget.NewCheck("Use mph instead of km/h", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsUseMPH, b)
	})
}

func (sw *SettingsWidget) loadPrefs() {
	freq := fyne.CurrentApp().Preferences().IntWithFallback(prefsFreq, 25)
	sw.freqSlider.SetValue(float64(freq))
	sw.autoLoad.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateLoadEcu, true))
	sw.autoSave.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateSaveEcu, false))
	sw.livePreview.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsLivePreview, true))
	sw.meshView.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsMeshView, true))
	sw.realtimeBars.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsRealtimeBars, true))
	sw.logFormat.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsLogFormat, "TXL"))
	sw.logPath.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsLogPath, datalogger.LOGPATH))
	sw.lambdaSource.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsLambdaSource, "ECU"))
	sw.useMPH.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsUseMPH, false))
}

func (sw *SettingsWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(sw.container)
}
