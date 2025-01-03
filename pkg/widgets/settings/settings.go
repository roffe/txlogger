package settings

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/widgets/cansettings"
	sdialog "github.com/sqweek/dialog"
)

const (
	prefsFreq                   = "freq"
	prefsAutoUpdateLoadEcu      = "autoUpdateLoadEcu"
	prefsAutoUpdateSaveEcu      = "autoUpdateSaveEcu"
	prefsLivePreview            = "livePreview"
	prefsMeshView               = "liveMeshView"
	prefsRealtimeBars           = "realtimeBars"
	prefsLogFormat              = "logFormat"
	prefsLogPath                = "logPath"
	prefsLambdaSource           = "lambdaSource"
	prefsWidebandSymbolName     = "widebandSymbolName"
	prefsUseMPH                 = "useMPH"
	prefsSwapRPMandSpeed        = "swapRPMandSpeed"
	prefsPlotResolution         = "plotResolution"
	prefsCursorFollowCrosshair  = "cursorFollowCrosshair"
	prefsWBLPort                = "wblPort"
	prefsminimumVoltageWideband = "minimumVoltageWideband"
	prefsmaximumVoltageWideband = "maximumVoltageWideband"
	prefslowAFR                 = "lowAFR"
	prefshighAFR                = "highAFR"
)

type SettingsWidget struct {
	cfg *Config

	CanSettings           *cansettings.Widget
	freqSlider            *widget.Slider
	freqValue             *widget.Label
	autoSave              *widget.Check
	cursorFollowCrosshair *widget.Check
	autoLoad              *widget.Check
	livePreview           *widget.Check
	meshView              *widget.Check
	realtimeBars          *widget.Check
	logFormat             *widget.Select
	logPath               *widget.Label
	wblSource             *widget.Select
	useMPH                *widget.Check
	swapRPMandSpeed       *widget.Check
	plotResolution        *widget.Select
	container             *container.AppTabs

	// WBL Specific
	wblPortLabel                *widget.Label
	wblPortSelect               *widget.Select
	wblPortRefreshButton        *widget.Button
	minimumVoltageWidebandLabel *widget.Label
	minimumVoltageWidebandEntry *widget.Entry
	maximumVoltageWidebandLabel *widget.Label
	maximumVoltageWidebandEntry *widget.Entry
	lowAFRLabel                 *widget.Label
	lowAFREntry                 *widget.Entry
	highAFRLabel                *widget.Label
	highAFREntry                *widget.Entry

	minimumVoltageWideband float64
	maximumVoltageWideband float64
	lowAFR                 float64
	highAFR                float64

	//images
	mtxl        *canvas.Image
	lc2         *canvas.Image
	uego        *canvas.Image
	lambdatocan *canvas.Image
	t7          *canvas.Image

	widget.BaseWidget
}

func (sw *SettingsWidget) GetWidebandType() string {
	return sw.wblSource.Selected
}

func (sw *SettingsWidget) GetWidebandSymbolName() string {
	switch sw.wblSource.Selected {
	case "ECU":
		switch sw.cfg.EcuSelect.Selected {
		case "T5":
			return "AD_EGR"
		case "T7", "T8":
			return "DisplProt.LambdaScanner"
		default:
			return "None"
		}
	case ecumaster.ProductString, innovate.ProductString, aem.ProductString:
		return datalogger.EXTERNALWBLSYM
	default:
		return "None"
	}
}

func (sw *SettingsWidget) GetWidebandPort() string {
	return sw.wblPortSelect.Selected
}

func (sw *SettingsWidget) GetMinimumVoltageWideband() float64 {
	return sw.minimumVoltageWideband
}

func (sw *SettingsWidget) GetMaximumVoltageWideband() float64 {
	return sw.maximumVoltageWideband
}

func (sw *SettingsWidget) GetLowAFR() float64 {
	return sw.lowAFR
}

func (sw *SettingsWidget) GetHighAFR() float64 {
	return sw.highAFR
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

func (sw *SettingsWidget) GetSwapRPMandSpeed() bool {
	return sw.swapRPMandSpeed.Checked
}

func (sw *SettingsWidget) GetPlotResolution() float32 {
	switch sw.plotResolution.Selected {
	case "Full":
		return 1
	case "Half":
		return 0.5
	case "Quarter":
		return 0.25
	default:
		return 1
	}
}

func (sw *SettingsWidget) GetCursorFollowCrosshair() bool {
	return sw.cursorFollowCrosshair.Checked
}

type Config struct {
	EcuSelect *widget.Select
}

func New(cfg *Config) *SettingsWidget {
	sw := &SettingsWidget{
		cfg: cfg,
	}
	sw.ExtendBaseWidget(sw)

	sw.freqSlider = sw.newFreqSlider()
	sw.freqValue = widget.NewLabel("")

	sw.autoLoad = sw.newAutoUpdateLoad()
	sw.autoSave = sw.newAutoUpdateSave()
	sw.cursorFollowCrosshair = sw.newCursorFollowCrosshair()

	sw.livePreview = sw.newLivePreview()
	sw.meshView = sw.newMeshView()
	sw.realtimeBars = sw.newRealtimeBars()

	sw.logFormat = sw.newLogFormat()
	sw.logPath = widget.NewLabel("")
	sw.logPath.Truncation = fyne.TextTruncateEllipsis

	sw.useMPH = sw.newUserMPH()
	sw.swapRPMandSpeed = sw.newSwapRPMandSpeed()

	sw.plotResolution = sw.newPlotResolution()

	app := fyne.CurrentApp()

	sw.CanSettings = cansettings.NewCanSettingsWidget(app)

	sw.wblPortLabel = widget.NewLabel("WBL Port")
	sw.wblPortSelect = widget.NewSelect(append([]string{"txbridge", "CAN"}, sw.CanSettings.ListPorts()...), func(s string) {
		app.Preferences().SetString(prefsWBLPort, s)
	})

	sw.wblPortRefreshButton = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.wblPortSelect.Options = append([]string{"txbridge", "CAN"}, sw.CanSettings.ListPorts()...)
		sw.wblPortSelect.Refresh()
	})

	sw.minimumVoltageWidebandLabel = widget.NewLabel("Minimum voltage")
	sw.minimumVoltageWidebandEntry = widget.NewEntry()
	sw.minimumVoltageWidebandEntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		fyne.CurrentApp().Preferences().SetString(prefsminimumVoltageWideband, s)
		sw.minimumVoltageWideband = val
		return nil
	}

	sw.maximumVoltageWidebandLabel = widget.NewLabel("Maximum voltage")
	sw.maximumVoltageWidebandEntry = widget.NewEntry()
	sw.maximumVoltageWidebandEntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		fyne.CurrentApp().Preferences().SetString(prefsmaximumVoltageWideband, s)
		sw.maximumVoltageWideband = val
		return nil
	}

	sw.lowAFRLabel = widget.NewLabel("Low AFR")
	sw.lowAFREntry = widget.NewEntry()
	sw.lowAFREntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		fyne.CurrentApp().Preferences().SetString(prefslowAFR, s)
		sw.lowAFR = val
		return nil
	}

	sw.highAFRLabel = widget.NewLabel("High AFR")
	sw.highAFREntry = widget.NewEntry()
	sw.highAFREntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		fyne.CurrentApp().Preferences().SetString(prefshighAFR, s)
		sw.highAFR = val
		return nil
	}

	tabs := container.NewAppTabs()

	tabs.Append(container.NewTabItem("CAN", sw.CanSettings))

	tabs.Append(container.NewTabItem("Logging", container.NewVBox(
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
			widget.NewLabel("Log folder"),
			nil,
			sw.logPath,
		),
	)))

	tabs.Append(container.NewTabItem("General", container.NewVBox(
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
			widget.NewIcon(theme.MoveUpIcon()),
			nil,
			sw.cursorFollowCrosshair,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.SearchIcon()),
			nil,
			container.NewVBox(
				sw.livePreview,
				sw.realtimeBars,
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
			//widget.NewIcon(theme.ZoomFitIcon()),
			widget.NewLabel("Plot resolution"),
			nil,
			sw.plotResolution,
		),
	)))

	sw.mtxl = newImageFromResource("mtx-l")
	sw.lc2 = newImageFromResource("lc-2")
	sw.uego = newImageFromResource("uego")
	sw.lambdatocan = newImageFromResource("lambdatocan")
	sw.t7 = newImageFromResource("t7")

	wblSel := sw.newWBLSelector()

	tabs.Append(container.NewTabItem("WBL", container.NewVBox(
		container.NewHBox(layout.NewSpacer(), sw.mtxl, sw.lc2, sw.uego, sw.lambdatocan, sw.t7, layout.NewSpacer()),
		wblSel,
		container.NewBorder(
			nil,
			nil,
			sw.wblPortLabel,
			sw.wblPortRefreshButton,
			sw.wblPortSelect,
		),
		container.NewBorder(
			nil,
			nil,
			sw.minimumVoltageWidebandLabel,
			nil,
			sw.minimumVoltageWidebandEntry,
		),
		container.NewBorder(
			nil,
			nil,
			sw.maximumVoltageWidebandLabel,
			nil,
			sw.maximumVoltageWidebandEntry,
		),
		container.NewBorder(
			nil,
			nil,
			sw.lowAFRLabel,
			nil,
			sw.lowAFREntry,
		),
		container.NewBorder(
			nil,
			nil,
			sw.highAFRLabel,
			nil,
			sw.highAFREntry,
		),
	)))

	tabs.Append(container.NewTabItem("Dashboard", container.NewVBox(
		widget.NewLabel("Dashboard settings"),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.InfoIcon()),
			nil,
			sw.swapRPMandSpeed,
		),
		container.NewBorder(
			nil,
			nil,
			widget.NewIcon(theme.InfoIcon()),
			nil,
			sw.useMPH,
		),
	)))

	sw.container = tabs

	sw.loadPrefs()
	return sw
}

func newImageFromResource(name string) *canvas.Image {
	var img *canvas.Image
	switch name {
	case "mtx-l":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.MtxL))
		img.SetMinSize(fyne.NewSize(224, 224))
	case "lc-2":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.Lc2))
		img.SetMinSize(fyne.NewSize(400, 224))
	case "uego":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.Uego))
		img.SetMinSize(fyne.NewSize(315, 224))
	case "lambdatocan":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.LambdaToCan))
		img.SetMinSize(fyne.NewSize(481, 224))
	case "t7":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.T7))
		img.SetMinSize(fyne.NewSize(320, 224))
	}
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleSmooth
	return img
}

func (sw *SettingsWidget) newLogFormat() *widget.Select {
	return widget.NewSelect([]string{"CSV", "TXL"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLogFormat, s)
	})
}

func (sw *SettingsWidget) newWBLSelector() *fyne.Container {
	sw.wblSource = widget.NewSelect([]string{
		"None",
		"ECU",
		ecumaster.ProductString,
		innovate.ProductString,
		aem.ProductString,
	}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLambdaSource, s)
		fyne.CurrentApp().Preferences().SetString(prefsWidebandSymbolName, sw.GetWidebandSymbolName())
		var ecuSet bool
		switch s {
		case "ECU":
			sw.mtxl.Hide()
			sw.lc2.Hide()
			sw.uego.Hide()
			sw.lambdatocan.Hide()
			sw.t7.Show()
			ecuSet = true
		case ecumaster.ProductString:
			sw.mtxl.Hide()
			sw.lc2.Hide()
			sw.uego.Hide()
			sw.lambdatocan.Show()
			sw.t7.Hide()
		case innovate.ProductString:
			sw.mtxl.Show()
			sw.lc2.Show()
			sw.uego.Hide()
			sw.lambdatocan.Hide()
			sw.t7.Hide()
		case aem.ProductString:
			sw.mtxl.Hide()
			sw.lc2.Hide()
			sw.uego.Show()
			sw.lambdatocan.Hide()
			sw.t7.Hide()
		default:
			sw.mtxl.Hide()
			sw.lc2.Hide()
			sw.uego.Hide()
			sw.lambdatocan.Hide()
			sw.t7.Hide()
		}

		if ecuSet {
			sw.wblPortLabel.Hide()
			sw.wblPortSelect.Hide()
			sw.wblPortRefreshButton.Hide()
			sw.minimumVoltageWidebandLabel.Show()
			sw.maximumVoltageWidebandLabel.Show()
			sw.lowAFRLabel.Show()
			sw.highAFRLabel.Show()
			sw.minimumVoltageWidebandEntry.Show()
			sw.maximumVoltageWidebandEntry.Show()
			sw.lowAFREntry.Show()
			sw.highAFREntry.Show()
		} else {
			sw.wblPortLabel.Show()
			sw.wblPortSelect.Show()
			sw.wblPortRefreshButton.Show()
			sw.minimumVoltageWidebandLabel.Hide()
			sw.maximumVoltageWidebandLabel.Hide()
			sw.lowAFRLabel.Hide()
			sw.highAFRLabel.Hide()
			sw.minimumVoltageWidebandEntry.Hide()
			sw.maximumVoltageWidebandEntry.Hide()
			sw.lowAFREntry.Hide()
			sw.highAFREntry.Hide()
		}

		sw.container.Refresh()
	})
	return container.NewBorder(
		nil,
		nil,
		widget.NewLabel("Source"),
		nil,
		sw.wblSource,
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
	return widget.NewCheck("Load maps from ECU when connected", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateLoadEcu, b)
	})
}

func (sw *SettingsWidget) newAutoUpdateSave() *widget.Check {
	return widget.NewCheck("Save changes automaticly if connected to ECU (requires open bin)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateSaveEcu, b)
	})
}

func (sw *SettingsWidget) newCursorFollowCrosshair() *widget.Check {
	return widget.NewCheck("Cursor follows crosshair in MapViewer (one hand mapping)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsCursorFollowCrosshair, b)
	})
}

func (sw *SettingsWidget) newLivePreview() *widget.Check {
	return widget.NewCheck("Live preview values in symbollist (uncheck if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsLivePreview, b)
	})
}

func (sw *SettingsWidget) newRealtimeBars() *widget.Check {
	return widget.NewCheck("Bars on live preview values (uncheck if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsRealtimeBars, b)
	})
}

func (sw *SettingsWidget) newUserMPH() *widget.Check {
	return widget.NewCheck("Use mph instead of km/h", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsUseMPH, b)
	})
}

func (sw *SettingsWidget) newSwapRPMandSpeed() *widget.Check {
	return widget.NewCheck("Swap RPM and speed gauge position", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsSwapRPMandSpeed, b)
	})
}

func (sw *SettingsWidget) newPlotResolution() *widget.Select {
	return widget.NewSelect([]string{"Full", "Half", "Quarter"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsPlotResolution, s)
	})
}

func (sw *SettingsWidget) loadPrefs() {
	freq := fyne.CurrentApp().Preferences().IntWithFallback(prefsFreq, 25)
	sw.freqSlider.SetValue(float64(freq))
	sw.autoLoad.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateLoadEcu, true))
	sw.autoSave.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateSaveEcu, false))
	sw.cursorFollowCrosshair.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsCursorFollowCrosshair, false))
	sw.livePreview.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsLivePreview, true))
	sw.meshView.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsMeshView, true))
	sw.realtimeBars.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsRealtimeBars, true))
	sw.logFormat.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsLogFormat, "TXL"))
	sw.logPath.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsLogPath, datalogger.LOGPATH))
	sw.wblSource.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsLambdaSource, "None"))
	sw.useMPH.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsUseMPH, false))
	sw.swapRPMandSpeed.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsSwapRPMandSpeed, false))
	sw.plotResolution.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsPlotResolution, "Full"))
	sw.wblPortSelect.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsWBLPort, ""))
	sw.minimumVoltageWidebandEntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsminimumVoltageWideband, "0.0"))
	sw.maximumVoltageWidebandEntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsmaximumVoltageWideband, "5.0"))
	sw.lowAFREntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefslowAFR, "0.5"))
	sw.highAFREntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefshighAFR, "1.5"))

}

func (sw *SettingsWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(sw.container)
}

func positiveFloatValidator(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSuffix(s, ".")

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.New("invalid number")
	}
	if val < 0 {
		return 0, errors.New("must be positive")
	}
	return val, nil
}
