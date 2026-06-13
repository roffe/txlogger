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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/widgets/txconfigurator"
	"go.bug.st/serial/enumerator"
)

// Config carries callbacks the settings widget needs from its host.
type Config struct {
	Logger          func(string)
	SelectedEcuFunc func() string
}

// Widget is the settings panel. All persisted state lives in the application
// preferences (see prefs.go); the fields below are just the UI controls bound
// to those preferences.
type Widget struct {
	widget.BaseWidget

	cfg *Config

	workDir *widget.Label

	// General
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

	// CAN
	debugCheckbox   *widget.Check
	adapterSelector *widget.Select
	refreshBtn      *widget.Button
	portSelector    *widget.Select
	portDescription *widget.Label
	speedSelector   *widget.Select
	adapters        map[string]*gocan.AdapterInfo

	// Wideband
	wbleditor            *WBLEditor
	wblADscanner         *widget.Check
	wblADScannerSymbol   *widget.Select
	wblSelectContainer   *fyne.Container
	wblSource            *widget.Select
	wblPortLabel         *widget.Label
	wblPortSelect        *widget.Select
	wblPortRefreshButton *widget.Button
	wblImage             *canvas.Image

	mu sync.Mutex
}

func New(cfg *Config) *Widget {
	sw := &Widget{
		cfg:      cfg,
		adapters: make(map[string]*gocan.AdapterInfo),
		wblImage: newImageFromResource("t7"),
	}

	for _, adapter := range gocan.ListAdapters() {
		sw.adapters[adapter.Name] = &adapter
	}

	sw.ExtendBaseWidget(sw)
	return sw
}

func (sw *Widget) CreateRenderer() fyne.WidgetRenderer {
	sw.workDir = widget.NewLabel("")
	sw.workDir.Selectable = true
	if wd, err := os.Getwd(); err != nil {
		sw.workDir.SetText(fmt.Sprintf("Error getting working directory: %v", err))
	} else {
		sw.workDir.SetText(wd)
	}

	// General
	sw.freqSlider = sw.newFreqSlider()
	sw.freqValue = widget.NewLabel("")
	sw.autoLoad = checkBox("Load maps from ECU when connected", prefAutoLoad)
	sw.autoSave = checkBox("Save changes automaticly if connected to ECU (requires open bin)", prefAutoSave)
	sw.cursorFollowCrosshair = checkBox("Cursor follows crosshair in MapViewer (one hand mapping)", prefCursorFollowCrosshair)
	sw.livePreview = checkBox("Live preview values in symbollist (uncheck if you have a slow pc)", prefLivePreview)
	sw.meshView = checkBox("3D Mesh on map viewing", prefMeshView)
	sw.realtimeBars = checkBox("Bars on live preview values (uncheck if you have a slow pc)", prefRealtimeBars)
	sw.logFormat = sw.newLogFormat()
	sw.logPath = widget.NewLabel("")
	sw.logPath.Truncation = fyne.TextTruncateEllipsis
	sw.useMPH = checkBox("Use mph instead of km/h", prefUseMPH)
	sw.swapRPMandSpeed = checkBox("Swap RPM and speed gauge position", prefSwapRPMandSpeed)
	sw.colorBlindMode = sw.newColorBlindMode()
	sw.wblSelectContainer = sw.newWBLSelector()

	// Graphics
	sw.plotRendererSelect = indexSelect([]string{"Software", "Shader"}, prefPlotterRenderer)
	sw.meshRendererSelect = indexSelect([]string{"Shader", "Polygons", "Software"}, prefMeshRenderer)

	// CAN
	sw.adapterSelector = sw.newAdapterSelector()
	sw.portSelector = sw.newPortSelector()
	sw.portDescription = widget.NewLabel("")
	sw.portDescription.Importance = widget.LowImportance
	sw.speedSelector = sw.newSpeedSelector()
	sw.debugCheckbox = checkBox("Debug", prefDebug)
	sw.refreshBtn = sw.newPortRefreshButton()

	sw.adapterSelector.SetOptions(sw.sortedAdapterNames())
	if ad := prefAdapter.get(); ad != "" {
		sw.adapterSelector.SetSelected(ad)
	}

	tabs := container.NewAppTabs()
	tabs.Append(sw.generalTab())
	tabs.Append(sw.graphicsTab())
	tabs.Append(sw.canTab())
	tabs.Append(sw.loggingTab())
	tabs.Append(sw.wblTab())
	tabs.Append(sw.adScannerTab())
	tabs.Append(container.NewTabItemWithIcon("txbridge", theme.DownloadIcon(), txconfigurator.NewConfigurator()))

	sw.loadPreferences()
	return widget.NewSimpleRenderer(tabs)
}

func (sw *Widget) sortedAdapterNames() []string {
	names := make([]string, 0, len(sw.adapters))
	for name := range sw.adapters {
		names = append(names, name)
	}
	slices.SortFunc(names, func(i, j string) int {
		return strings.Compare(strings.ToLower(i), strings.ToLower(j))
	})
	return names
}

// --- public API ------------------------------------------------------------

var portCache = make(map[string]*enumerator.PortDetails)

func (sw *Widget) ListPorts() []string {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil || len(ports) == 0 {
		return []string{}
	}
	portsList := make([]string, 0, len(ports))
	for _, port := range ports {
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
		info := &gocan.AdapterInfo{
			Name:        adapter.GetName(),
			Description: adapter.GetDescription(),
			Capabilities: gocan.AdapterCapabilities{
				HSCAN: adapter.GetCapabilities().GetHSCAN(),
				SWCAN: adapter.GetCapabilities().GetSWCAN(),
				KLine: adapter.GetCapabilities().GetKLine(),
			},
			RequiresSerialPort: adapter.GetRequireSerialPort(),
		}
		if _, found := sw.adapters[info.Name]; found {
			continue
		}
		sw.adapters[info.Name] = info
	}
}

func (sw *Widget) Disable() {
	sw.adapterSelector.Disable()
	sw.portSelector.Disable()
	sw.speedSelector.Disable()
	sw.debugCheckbox.Disable()
	sw.refreshBtn.Disable()
}

func (sw *Widget) Enable() {
	sw.adapterSelector.Enable()
	sw.portSelector.Enable()
	sw.speedSelector.Enable()
	sw.debugCheckbox.Enable()
	sw.refreshBtn.Enable()

	if info, found := sw.adapters[sw.adapterSelector.Selected]; found {
		if info.RequiresSerialPort {
			sw.portSelector.Enable()
			sw.speedSelector.Enable()
		} else {
			sw.portSelector.Disable()
			sw.speedSelector.Disable()
		}
	}
}
