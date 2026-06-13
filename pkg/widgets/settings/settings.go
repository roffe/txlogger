package settings

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/widgets/txconfigurator"
	"go.bug.st/serial/enumerator"
)

const (
	prefsFreq                  = "freq"
	prefsAutoUpdateLoadEcu     = "autoUpdateLoadEcu"
	prefsAutoUpdateSaveEcu     = "autoUpdateSaveEcu"
	prefsLivePreview           = "livePreview"
	prefsMeshView              = "liveMeshView"
	prefsRealtimeBars          = "realtimeBars"
	prefsLogFormat             = "logFormat"
	prefsLogPath               = "logPath"
	prefsWblSource             = "wblSource"
	prefsWidebandSymbolName    = "widebandSymbolName"
	prefsUseMPH                = "useMPH"
	prefsSwapRPMandSpeed       = "swapRPMandSpeed"
	prefsCursorFollowCrosshair = "cursorFollowCrosshair"
	prefsWBLPort               = "wblPort"

	prefsLastADScannerPreset = "lastADScannerPreset"
	prefsWBLSupportPoints    = "wblSupportPoints"
	prefsWBLLambdaValues     = "wblLambdaValues"
	prefsLastADScannerECU    = "lastADScannerECU"
	prefsWBLADScannerSymbol  = "wblADScannerSymbol"

	prefsUseADScanner   = "useADScanner"
	prefsColorBlindMode = "colorBlindMode"

	prefsPlotterRenderer = "plotterRenderer"
	prefsMeshRenderer    = "meshRenderer"

	// CAN
	prefsAdapter = "adapter"
	prefsPort    = "port"
	prefsSpeed   = "speed"
	prefsDebug   = "debug"

	// Flash
	PrefsNvdm = "nvdm"
	PrefsBoot = "boot"
)

var portSpeeds = []string{"9600", "19200", "38400", "57600", "115200", "230400", "460800", "500000", "921600", "1mbit", "2mbit", "3mbit"}

type SettingsWidgetInterface interface {
	Get(key string) (string, error)
	Widget() fyne.Widget
}

type SetText interface {
	SetText(string)
}

type Config struct {
	Logger          func(string)
	SelectedEcuFunc func() string
}

type Widget struct {
	widget.BaseWidget

	cfg *Config

	workDir *widget.Label

	// CANSettings           *cansettings.Widget
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
	// Graphics
	plotRendererSelect *widget.Select
	meshRendererSelect *widget.Select

	// can settings
	debugCheckbox   *widget.Check
	adapterSelector *widget.Select
	refreshBtn      *widget.Button
	portSelector    *widget.Select
	portDescription *widget.Label
	speedSelector   *widget.Select

	adapters map[string]*gocan.AdapterInfo

	// WBL Specific

	wbleditor *WBLEditor

	wblADscanner         *widget.Check
	wblADScannerSymbol   *widget.Select
	wblSelectContainer   *fyne.Container
	wblSource            *widget.Select
	wblPortLabel         *widget.Label
	wblPortSelect        *widget.Select
	wblPortRefreshButton *widget.Button

	images struct {
		wblImage *canvas.Image

		/*
			mtxl        *canvas.Image
			lc2         *canvas.Image
			uego        *canvas.Image
			lambdatocan *canvas.Image
			t7          *canvas.Image
			plx         *canvas.Image
			combi       *canvas.Image
			zeitronix   *canvas.Image
			stagafr     *canvas.Image
		*/
	}

	mu sync.Mutex
}

func New(cfg *Config) *Widget {
	sw := &Widget{
		cfg:      cfg,
		adapters: make(map[string]*gocan.AdapterInfo),
	}

	for _, adapter := range gocan.ListAdapters() {
		sw.adapters[adapter.Name] = &adapter
	}

	sw.images.wblImage = newImageFromResource("t7")

	sw.ExtendBaseWidget(sw)
	return sw
}

func (sw *Widget) CreateRenderer() fyne.WidgetRenderer {
	sw.workDir = widget.NewLabel("")
	sw.workDir.Selectable = true
	wd, err := os.Getwd()
	if err != nil {
		sw.workDir.SetText(fmt.Sprintf("Error getting working directory: %v", err))
	} else {
		sw.workDir.SetText(wd)
	}

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
	sw.wblSelectContainer = sw.newWBLSelector()

	// Graphics
	sw.plotRendererSelect = sw.newPlotRendererSelect()
	sw.meshRendererSelect = sw.newMeshRendererSelect()

	// CAN
	sw.adapterSelector = sw.newAdapterSelector()
	sw.portSelector = sw.newPortSelector()
	sw.portDescription = widget.NewLabel("")
	sw.portDescription.Importance = widget.LowImportance
	sw.speedSelector = sw.newSpeedSelector()
	sw.debugCheckbox = sw.newDebugCheckbox()
	sw.refreshBtn = sw.newPortRefreshButton()

	names := make([]string, 0, len(sw.adapters))
	for name := range sw.adapters {
		names = append(names, name)
	}
	slices.SortFunc(names, func(i, j string) int {
		return strings.Compare(strings.ToLower(i), strings.ToLower(j))
	})
	sw.adapterSelector.SetOptions(names)
	if ad := fyne.CurrentApp().Preferences().String(prefsAdapter); ad != "" {
		sw.adapterSelector.SetSelected(ad)
	}

	tabs := container.NewAppTabs()
	tabs.Append(sw.generalTab())
	tabs.Append(sw.graphicsTab())
	tabs.Append(sw.canTab())
	tabs.Append(sw.loggingTab())
	tabs.Append(sw.dashboardTab())
	tabs.Append(sw.wblTab())
	tabs.Append(sw.adScannerTab())
	tabs.Append(container.NewTabItem("txbridge", txconfigurator.NewConfigurator()))

	for _, adapter := range gocan.ListAdapters() {
		sw.adapters[adapter.Name] = &adapter
	}

	sw.loadPreferences()
	return widget.NewSimpleRenderer(tabs)
}

// Public API

var portCache = make(map[string]*enumerator.PortDetails)

func (sw *Widget) ListPorts() []string {
	var portsList []string
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		// m.output(err.Error())
		return []string{}
	}
	if len(ports) == 0 {
		// m.output("No serial ports found!")
		return []string{}
	}
	for _, port := range ports {
		// m.output(fmt.Sprintf("Found port: %s", port.Name))
		// if port.IsUSB {
		// m.output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
		// m.output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
		portsList = append(portsList, port.Name)
		portCache[port.Name] = port
	}
	sort.Strings(portsList)
	return portsList
}

func (sw *Widget) AddAdapters(adapters []*proto.AdapterInfo) {
	if len(adapters) == 0 {
		return
	}
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for _, adapter := range adapters {
		adapter := &gocan.AdapterInfo{
			Name:        adapter.GetName(),
			Description: adapter.GetDescription(),
			Capabilities: gocan.AdapterCapabilities{
				HSCAN: adapter.GetCapabilities().GetHSCAN(),
				SWCAN: adapter.GetCapabilities().GetSWCAN(),
				KLine: adapter.GetCapabilities().GetKLine(),
			},
			RequiresSerialPort: adapter.GetRequireSerialPort(),
		}

		if _, found := sw.adapters[adapter.Name]; found {
			continue
		}
		sw.adapters[adapter.Name] = adapter
	}
}

func (c *Widget) Disable() {
	c.adapterSelector.Disable()
	c.portSelector.Disable()
	c.speedSelector.Disable()
	c.debugCheckbox.Disable()
	c.refreshBtn.Disable()
}

func (c *Widget) Enable() {
	c.adapterSelector.Enable()
	c.portSelector.Enable()
	c.speedSelector.Enable()
	c.debugCheckbox.Enable()
	c.refreshBtn.Enable()

	if info, found := c.adapters[c.adapterSelector.Selected]; found {
		if info.RequiresSerialPort {
			c.portSelector.Enable()
			c.speedSelector.Enable()
		} else {
			c.portSelector.Disable()
			c.speedSelector.Disable()
		}
	}
}
