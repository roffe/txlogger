package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/datalogger"
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

type SetText interface {
	SetText(string)
}

type Config struct {
	SelectedEcuFunc func() string
}

type Widget struct {
	widget.BaseWidget

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
	wblSelectContainer          *fyne.Container
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
	sw.CANSettings = cansettings.New()
	sw.wblSelectContainer = sw.newWBLSelector()

	tabs := container.NewAppTabs()
	tabs.Append(sw.generalTab())
	tabs.Append(container.NewTabItem("CAN", sw.CANSettings))
	tabs.Append(sw.loggingTab())
	tabs.Append(sw.wblTab())
	tabs.Append(sw.dashboardTab())
	tabs.Append(container.NewTabItem("txbridge", txconfigurator.NewConfigurator()))

	sw.container = tabs

	sw.loadPreferences()
	return sw
}

func (sw *Widget) CreateRenderer() fyne.WidgetRenderer {

	return widget.NewSimpleRenderer(sw.container)
}

// Public API
func (sw *Widget) GetWidebandType() string {
	return sw.wblSource.Selected
}

func (sw *Widget) GetWidebandSymbolName() string {
	switch sw.wblSource.Selected {
	case "ECU":
		switch sw.cfg.SelectedEcuFunc() {
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
