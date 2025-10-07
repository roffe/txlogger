package settings

import (
	"errors"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/settings/cansettings"
	"github.com/roffe/txlogger/pkg/widgets/txconfigurator"
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
	prefsCursorFollowCrosshair  = "cursorFollowCrosshair"
	prefsWBLPort                = "wblPort"
	prefsminimumVoltageWideband = "minimumVoltageWideband"
	prefsmaximumVoltageWideband = "maximumVoltageWideband"
	prefslowValue               = "lowValue"
	prefshighValue              = "highValue"
	prefsUseADScanner           = "useADScanner"
	prefsColorBlindMode         = "colorBlindMode"
)

type SettingsWidgetInterface interface {
	Get(key string) (string, error)
	Widget() fyne.Widget
}

type Widget struct {
	cfg *Config

	CANSettings           *cansettings.Widget
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
	useMPH                *widget.Check
	swapRPMandSpeed       *widget.Check
	colorBlindMode        *widget.Select
	container             *container.AppTabs

	// WBL Specific
	wblADscanner                *widget.Check
	wblSource                   *widget.Select
	wblPortLabel                *widget.Label
	wblPortSelect               *widget.Select
	wblPortRefreshButton        *widget.Button
	minimumVoltageWidebandLabel *widget.Label
	minimumVoltageWidebandEntry *widget.Entry
	maximumVoltageWidebandLabel *widget.Label
	maximumVoltageWidebandEntry *widget.Entry
	lowLabel                    *widget.Label
	lowEntry                    *widget.Entry
	highLabel                   *widget.Label
	highEntry                   *widget.Entry

	minimumVoltageWideband float64
	maximumVoltageWideband float64

	low  float64
	high float64

	wblImage *canvas.Image

	images struct {
		mtxl        *canvas.Image
		lc2         *canvas.Image
		uego        *canvas.Image
		lambdatocan *canvas.Image
		t7          *canvas.Image
		plx         *canvas.Image
		combi       *canvas.Image
		zeitronix   *canvas.Image
	}

	widget.BaseWidget
}

func (sw *Widget) GetWidebandType() string {
	return sw.wblSource.Selected
}

func (sw *Widget) GetWidebandSymbolName() string {
	switch sw.wblSource.Selected {
	case "ECU":
		switch sw.cfg.GetEcu() {
		case "T5":
			return "AD_EGR"
		case "T7":
			if sw.wblADscanner.Checked {
				return "DisplProt.AD_Scanner"
			}
			return "DisplProt.LambdaScanner"
		case "T8":
			return "LambdaScan.LambdaScanner"
		default:
			return "None"
		}
	case ecumaster.ProductString,
		innovate.ProductString,
		aem.ProductString,
		plx.ProductString,
		"CombiAdapter",
		zeitronix.ProductString:
		return datalogger.EXTERNALWBLSYM // Lambda.External
	default:
		return "None"
	}
}

func (sw *Widget) GetColorBlindMode() widgets.ColorBlindMode {
	return widgets.ColorBlindMode(sw.colorBlindMode.SelectedIndex())
}

func (sw *Widget) GetWidebandPort() string {
	return sw.wblPortSelect.Selected
}

func (sw *Widget) GetMinimumVoltageWideband() float64 {
	return sw.minimumVoltageWideband
}

func (sw *Widget) GetMaximumVoltageWideband() float64 {
	return sw.maximumVoltageWideband
}

func (sw *Widget) GetLow() float64 {
	return sw.low
}

func (sw *Widget) GetHigh() float64 {
	return sw.high
}

func (sw *Widget) GetFreq() int {
	return int(sw.freqSlider.Value)
}

func (sw *Widget) GetAutoSave() bool {
	return sw.autoSave.Checked
}

func (sw *Widget) GetAutoLoad() bool {
	return sw.autoLoad.Checked
}

func (sw *Widget) GetLivePreview() bool {
	return sw.livePreview.Checked
}

func (sw *Widget) GetRealtimeBars() bool {
	return sw.realtimeBars.Checked
}

func (sw *Widget) GetMeshView() bool {
	return sw.meshView.Checked
}

func (sw *Widget) GetLogFormat() string {
	return sw.logFormat.Selected
}

func (sw *Widget) GetLogPath() string {
	return sw.logPath.Text
}

func (sw *Widget) GetUseMPH() bool {
	return sw.useMPH.Checked
}

func (sw *Widget) GetSwapRPMandSpeed() bool {
	return sw.swapRPMandSpeed.Checked
}

func (sw *Widget) GetCursorFollowCrosshair() bool {
	return sw.cursorFollowCrosshair.Checked
}

type Config struct {
	//EcuSelect *widget.Select
	GetEcu func() string
}

func New(cfg *Config) *Widget {
	sw := &Widget{
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

	sw.colorBlindMode = sw.newColorBlindMode()

	sw.CANSettings = cansettings.NewCANSettingsWidget()

	app := fyne.CurrentApp()

	sw.wblPortLabel = widget.NewLabel("WBL Port")
	sw.wblPortSelect = widget.NewSelect(append([]string{"txbridge", "CAN"}, sw.CANSettings.ListPorts()...), func(s string) {
		app.Preferences().SetString(prefsWBLPort, s)
	})

	sw.wblPortRefreshButton = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		sw.wblPortSelect.Options = append([]string{"txbridge", "CAN"}, sw.CANSettings.ListPorts()...)
		sw.wblPortSelect.Refresh()
	})

	sw.minimumVoltageWidebandLabel = widget.NewLabel("Minimum voltage")
	sw.minimumVoltageWidebandEntry = widget.NewEntry()
	sw.minimumVoltageWidebandEntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		app.Preferences().SetString(prefsminimumVoltageWideband, s)
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

	sw.lowLabel = widget.NewLabel("Low")
	sw.lowEntry = widget.NewEntry()
	sw.lowEntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		app.Preferences().SetString(prefslowValue, s)
		sw.low = val
		return nil
	}

	sw.highLabel = widget.NewLabel("High")
	sw.highEntry = widget.NewEntry()
	sw.highEntry.Validator = func(s string) error {
		val, err := positiveFloatValidator(s)
		if err != nil {
			return err
		}
		fyne.CurrentApp().Preferences().SetString(prefshighValue, s)
		sw.high = val
		return nil
	}

	tabs := container.NewAppTabs()

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
			widget.NewLabel("Color blind mode"),
			nil,
			sw.colorBlindMode,
		),
	)))

	tabs.Append(container.NewTabItem("CAN", sw.CANSettings))

	logPath, err := common.GetLogPath()
	if err != nil {
		fyne.LogError("Could not get log path", err)
	}

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
					sw.logPath.SetText(logPath)
					app.Preferences().SetString(prefsLogPath, logPath)
				}),
				widget.NewButtonWithIcon("Browse", theme.FileIcon(), func() {
					cb := func(dir string) {
						sw.logPath.SetText(dir)
						app.Preferences().SetString(prefsLogPath, dir)
					}
					widgets.SelectFolder(cb)
				}),
			),
			widget.NewLabel("Log folder"),
			nil,
			sw.logPath,
		),
	)))

	//tabs.Append(container.NewTabItem("Security", container.NewVBox(
	//	widget.NewLabel("Security settings"),
	//)))

	sw.images.mtxl = newImageFromResource("mtx-l")
	sw.images.lc2 = newImageFromResource("lc-2")
	sw.images.uego = newImageFromResource("uego")
	sw.images.lambdatocan = newImageFromResource("lambdatocan")
	sw.images.t7 = newImageFromResource("t7")
	sw.images.plx = newImageFromResource("plx")
	sw.images.combi = newImageFromResource("combi")
	sw.images.zeitronix = newImageFromResource("zeitronix")

	wblSel := sw.newWBLSelector()
	sw.wblADscanner = sw.newADscannerCheck()

	sw.wblImage = &canvas.Image{}

	tabs.Append(container.NewTabItem("WBL", container.NewVBox(
		container.NewHBox(
			layout.NewSpacer(),
			sw.images.mtxl,
			sw.images.lc2,
			sw.images.uego,
			sw.images.lambdatocan,
			sw.images.t7,
			sw.images.plx,
			sw.images.combi,
			sw.images.zeitronix,
			layout.NewSpacer(),
		),
		wblSel,
		container.NewBorder(
			nil,
			nil,
			nil,
			nil,
			sw.wblADscanner,
		),
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
			sw.lowLabel,
			nil,
			sw.lowEntry,
		),
		container.NewBorder(
			nil,
			nil,
			sw.highLabel,
			nil,
			sw.highEntry,
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

	tabs.Append(container.NewTabItem("txbridge", txconfigurator.NewConfigurator()))

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
	case "plx":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.PLX))
		img.SetMinSize(fyne.NewSize(470, 224))
	case "combi":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.CombiV2))
		img.SetMinSize(fyne.NewSize(360, 245))
	case "zeitronix":
		img = canvas.NewImageFromResource(fyne.NewStaticResource(name, assets.ZeitronixZT2))
		img.SetMinSize(fyne.NewSize(252, 252))
	}
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleFastest

	return img
}

func (sw *Widget) newLogFormat() *widget.Select {
	return widget.NewSelect([]string{"CSV", "TXL"}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLogFormat, s)
	})
}

func (sw *Widget) newWBLSelector() *fyne.Container {
	sw.wblSource = widget.NewSelect([]string{
		"None",
		"ECU",
		aem.ProductString,
		"CombiAdapter",
		ecumaster.ProductString,
		innovate.ProductString,
		plx.ProductString,
		zeitronix.ProductString,
	}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsLambdaSource, s)
		fyne.CurrentApp().Preferences().SetString(prefsWidebandSymbolName, sw.GetWidebandSymbolName())
		var ecuSet bool
		var portSelect bool
		switch s {
		case "ECU":
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Show()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			ecuSet = true
			portSelect = false
		case ecumaster.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Show()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			portSelect = false
		case innovate.ProductString:
			sw.images.mtxl.Show()
			sw.images.lc2.Show()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			portSelect = true
		case aem.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Show()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			portSelect = true
		case plx.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Show()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			portSelect = true
		case "CombiAdapter":
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Show()
			sw.images.zeitronix.Hide()
			portSelect = false
		case zeitronix.ProductString:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Show()
			portSelect = true
		default:
			sw.images.mtxl.Hide()
			sw.images.lc2.Hide()
			sw.images.uego.Hide()
			sw.images.lambdatocan.Hide()
			sw.images.t7.Hide()
			sw.images.plx.Hide()
			sw.images.combi.Hide()
			sw.images.zeitronix.Hide()
			portSelect = false
		}

		if portSelect {
			sw.wblPortLabel.Show()
			sw.wblPortSelect.Show()
			sw.wblPortRefreshButton.Show()
		} else {
			sw.wblPortLabel.Hide()
			sw.wblPortSelect.Hide()
			sw.wblPortRefreshButton.Hide()
		}

		if ecuSet || s == "CombiAdapter" {
			sw.wblADscanner.Show()
			if sw.wblADscanner.Checked {
				sw.minimumVoltageWidebandLabel.Show()
				sw.maximumVoltageWidebandLabel.Show()
				sw.lowLabel.Show()
				sw.highLabel.Show()
				sw.minimumVoltageWidebandEntry.Show()
				sw.maximumVoltageWidebandEntry.Show()
				sw.lowEntry.Show()
				sw.highEntry.Show()
			}
		} else {
			sw.wblADscanner.Hide()
			sw.minimumVoltageWidebandLabel.Hide()
			sw.maximumVoltageWidebandLabel.Hide()
			sw.lowLabel.Hide()
			sw.highLabel.Hide()
			sw.minimumVoltageWidebandEntry.Hide()
			sw.maximumVoltageWidebandEntry.Hide()
			sw.lowEntry.Hide()
			sw.highEntry.Hide()
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

func (sw *Widget) newFreqSlider() *widget.Slider {
	slider := widget.NewSlider(5, 300)
	slider.Step = 5
	slider.OnChanged = func(f float64) {
		sw.freqValue.SetText(strconv.FormatFloat(f, 'f', 0, 64))
	}
	slider.OnChangeEnded = func(f float64) {
		fyne.CurrentApp().Preferences().SetInt(prefsFreq, int(f))
	}
	return slider
}

func (sw *Widget) newADscannerCheck() *widget.Check {
	return widget.NewCheck("use AD Scanner (don't forget to add symbol)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsUseADScanner, b)
		if b {
			sw.minimumVoltageWidebandLabel.Show()
			sw.maximumVoltageWidebandLabel.Show()
			sw.lowLabel.Show()
			sw.highLabel.Show()
			sw.minimumVoltageWidebandEntry.Show()
			sw.maximumVoltageWidebandEntry.Show()
			sw.lowEntry.Show()
			sw.highEntry.Show()
		} else {
			sw.minimumVoltageWidebandLabel.Hide()
			sw.maximumVoltageWidebandLabel.Hide()
			sw.lowLabel.Hide()
			sw.highLabel.Hide()
			sw.minimumVoltageWidebandEntry.Hide()
			sw.maximumVoltageWidebandEntry.Hide()
			sw.lowEntry.Hide()
			sw.highEntry.Hide()
		}
	})
}

func (sw *Widget) newMeshView() *widget.Check {
	return widget.NewCheck("3D Mesh on map viewing", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsMeshView, b)
	})
}

func (sw *Widget) newAutoUpdateLoad() *widget.Check {
	return widget.NewCheck("Load maps from ECU when connected", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateLoadEcu, b)
	})
}

func (sw *Widget) newAutoUpdateSave() *widget.Check {
	return widget.NewCheck("Save changes automaticly if connected to ECU (requires open bin)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsAutoUpdateSaveEcu, b)
	})
}

func (sw *Widget) newCursorFollowCrosshair() *widget.Check {
	return widget.NewCheck("Cursor follows crosshair in MapViewer (one hand mapping)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsCursorFollowCrosshair, b)
	})
}

func (sw *Widget) newLivePreview() *widget.Check {
	return widget.NewCheck("Live preview values in symbollist (uncheck if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsLivePreview, b)
	})
}

func (sw *Widget) newRealtimeBars() *widget.Check {
	return widget.NewCheck("Bars on live preview values (uncheck if you have a slow pc)", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsRealtimeBars, b)
	})
}

func (sw *Widget) newUserMPH() *widget.Check {
	return widget.NewCheck("Use mph instead of km/h", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsUseMPH, b)
	})
}

func (sw *Widget) newSwapRPMandSpeed() *widget.Check {
	return widget.NewCheck("Swap RPM and speed gauge position", func(b bool) {
		fyne.CurrentApp().Preferences().SetBool(prefsSwapRPMandSpeed, b)
	})
}

func (sw *Widget) newColorBlindMode() *widget.Select {
	return widget.NewSelect([]string{
		widgets.ModeNormal.String(),
		widgets.ModeUniversal.String(),
		widgets.ModeProtanopia.String(),
		widgets.ModeTritanopia.String(),
	}, func(s string) {
		fyne.CurrentApp().Preferences().SetString(prefsColorBlindMode, s)
		ebus.Publish(ebus.TOPIC_COLORBLINDMODE, float64(sw.colorBlindMode.SelectedIndex()))
	})
}

func (sw *Widget) loadPrefs() {
	freq := fyne.CurrentApp().Preferences().IntWithFallback(prefsFreq, 25)
	sw.freqSlider.SetValue(float64(freq))

	sw.autoLoad.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateLoadEcu, true))
	sw.autoSave.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsAutoUpdateSaveEcu, false))
	sw.cursorFollowCrosshair.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsCursorFollowCrosshair, false))
	sw.livePreview.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsLivePreview, true))
	sw.meshView.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsMeshView, true))
	sw.realtimeBars.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsRealtimeBars, true))
	sw.logFormat.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsLogFormat, "TXL"))

	logPath, err := common.GetLogPath()
	if err != nil {
		fyne.LogError("Could not get log path", err)
	}
	sw.logPath.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsLogPath, logPath))

	sw.wblSource.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsLambdaSource, "None"))
	sw.wblADscanner.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsUseADScanner, false))
	sw.useMPH.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsUseMPH, false))
	sw.swapRPMandSpeed.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback(prefsSwapRPMandSpeed, false))
	sw.wblPortSelect.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsWBLPort, ""))
	sw.minimumVoltageWidebandEntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsminimumVoltageWideband, "0.0"))
	sw.maximumVoltageWidebandEntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefsmaximumVoltageWideband, "5.0"))
	sw.lowEntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefslowValue, "0.5"))
	sw.highEntry.SetText(fyne.CurrentApp().Preferences().StringWithFallback(prefshighValue, "1.5"))
	sw.colorBlindMode.SetSelected(fyne.CurrentApp().Preferences().StringWithFallback(prefsColorBlindMode, widgets.ModeNormal.String()))

	if sw.wblADscanner.Checked {
		sw.minimumVoltageWidebandLabel.Show()
		sw.maximumVoltageWidebandLabel.Show()
		sw.lowLabel.Show()
		sw.highLabel.Show()
		sw.minimumVoltageWidebandEntry.Show()
		sw.maximumVoltageWidebandEntry.Show()
		sw.lowEntry.Show()
		sw.highEntry.Show()
	} else {
		sw.minimumVoltageWidebandLabel.Hide()
		sw.maximumVoltageWidebandLabel.Hide()
		sw.lowLabel.Hide()
		sw.highLabel.Hide()
		sw.minimumVoltageWidebandEntry.Hide()
		sw.maximumVoltageWidebandEntry.Hide()
		sw.lowEntry.Hide()
		sw.highEntry.Hide()
	}

}

func (sw *Widget) CreateRenderer() fyne.WidgetRenderer {
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
