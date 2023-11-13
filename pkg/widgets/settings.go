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
	sdialog "github.com/sqweek/dialog"
)

const (
	prefsFreq              = "freq"
	prefsAutoUpdateLoadEcu = "autoUpdateLoadEcu"
	prefsAutoUpdateSaveEcu = "autoUpdateSaveEcu"
	prefsLivePreview       = "livePreview"
	prefsMeshView          = "liveMeshView"
)

type SettingsWidget struct {
	widget.BaseWidget

	freqSlider  *widget.Slider
	freqValue   *widget.Label
	autoSave    *widget.Check
	autoLoad    *widget.Check
	livePreview *widget.Check
	meshView    *widget.Check

	container *fyne.Container

	OnClose func()
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

func (sw *SettingsWidget) GetMeshView() bool {
	return sw.meshView.Checked
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

	logPath := widget.NewEntry()
	logPath.SetText(datalogger.LOGPATH)

	sw.container =
		container.NewVBox(
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Frequency (Hz)"),
				sw.freqValue,
				sw.freqSlider,
			),
			widget.NewSeparator(),
			container.NewBorder(
				nil,
				nil,
				widget.NewLabel("Log folder"),
				widget.NewButtonWithIcon("Browse", theme.FileIcon(), func() {
					dir, err := sdialog.Directory().Title("Select log folder").Browse()
					if err != nil {
						if errors.Is(err, sdialog.ErrCancelled) {
							return
						}
						log.Println(err)
						return
					}
					logPath.SetText(dir)
				}),
				logPath,
			),
			widget.NewSeparator(),
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
			widget.NewSeparator(),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.SearchIcon()),
				nil,
				sw.livePreview,
			),
			widget.NewSeparator(),
			container.NewBorder(
				nil,
				nil,
				widget.NewIcon(theme.ViewFullScreenIcon()),
				nil,
				sw.meshView,
			),
		)
	sw.loadPrefs()
	return sw
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

func (sw *SettingsWidget) loadPrefs() {
	freq := fyne.CurrentApp().Preferences().IntWithFallback(prefsFreq, 25)
	sw.freqSlider.SetValue(float64(freq))

	sw.autoLoad.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateLoadEcu, true))
	sw.autoSave.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateSaveEcu, false))
	sw.livePreview.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsLivePreview, true))
	sw.meshView.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsMeshView, true))
}

func (sw *SettingsWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(sw.container)
}
