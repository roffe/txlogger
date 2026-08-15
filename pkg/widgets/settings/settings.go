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
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/ebus"
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
	meshView              *widget.Check
	realtimeBars          *widget.Check
	logFormat             *widget.Select
	logPath               *widget.Label
	useMPH                *widget.Check
	colorBlindMode        *widget.Select

	// Graphics
	plotRendererSelect *widget.Select
	meshRendererSelect *widget.Select
	gaugeStyleSelect   *widget.Select

	// AI chat
	aiURL      *widget.Entry
	aiModel    *widget.SelectEntry
	aiThink    *widget.Check
	aiRefresh  *widget.Button
	aiCtxEntry *widget.Entry
	aiStatus   *widget.Label

	// CAN
	debugCheckbox   *widget.Check
	adapterSelector *widget.Select
	refreshBtn      *widget.Button
	portSelector    *widget.SelectEntry
	portDescription *widget.Label
	speedSelector   *widget.Select
	adapters        map[string]*gocan.AdapterInfo

	// Loggging
	experimentalT5FastLogger *widget.Check

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

	for _, adapter := range gocan.Adapters() {
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
	sw.meshView = checkBox("3D Mesh on map viewing", prefMeshView)
	sw.realtimeBars = checkBoxTopic("Value bars in symbol list (uncheck if you have a slow pc)", prefRealtimeBars, ebus.TOPIC_REALTIMEBARS)

	// Logging
	sw.logFormat = sw.newLogFormat()
	sw.logPath = widget.NewLabel("")
	sw.logPath.Truncation = fyne.TextTruncateEllipsis
	sw.experimentalT5FastLogger = checkBox("Experimental T5 fast logger", prefExperimentalT5FastLogger)

	sw.useMPH = checkBox("Use mph instead of km/h", prefUseMPH)
	sw.colorBlindMode = sw.newColorBlindMode()
	sw.wblSelectContainer = sw.newWBLSelector()

	// Graphics
	sw.plotRendererSelect = indexSelect([]string{"Software", "Shader"}, prefPlotterRenderer)
	sw.meshRendererSelect = indexSelect([]string{"Shader", "Polygons", "Software"}, prefMeshRenderer)
	sw.gaugeStyleSelect = widget.NewSelect(gaugeStyles, prefGaugeStyle.set)

	// AI chat
	sw.newAIControls()

	// CAN
	sw.adapterSelector = sw.newAdapterSelector()
	sw.portSelector = sw.newPortSelector()
	sw.portDescription = widget.NewLabel("")
	sw.portDescription.Importance = widget.LowImportance
	sw.speedSelector = sw.newSpeedSelector()
	sw.debugCheckbox = checkBox("Enable CAN Debugging", prefDebug)
	sw.refreshBtn = sw.newPortRefreshButton()

	sw.adapterSelector.SetOptions(sw.sortedAdapterNames())
	if ad := prefAdapter.get(); ad != "" {
		sw.adapterSelector.SetSelected(ad)
	}

	tabs := container.NewAppTabs()
	tabs.Append(sw.generalTab())
	tabs.Append(sw.graphicsTab())
	// tabs.Append(sw.customColorsTab())
	tabs.Append(sw.canTab())
	tabs.Append(sw.loggingTab())
	tabs.Append(sw.wblTab())
	tabs.Append(sw.adScannerTab())
	if prefPreviewFeatures.get() {
		tabs.Append(sw.aiTab())
	}

	// tabs.Append(container.NewTabItemWithIcon("txbridge", theme.DownloadIcon(), txconfigurator.NewConfigurator(prefPort.get)))

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
		// log.Println(port)
		portsList = append(portsList, port.Name)
		portCache[port.Name] = port
	}
	sort.Strings(portsList)
	return portsList
}

// RefreshAdapters rescans gocan's device scanners and rebuilds the adapter
// selector, picking up devices plugged in after startup.
func (sw *Widget) RefreshAdapters() {
	adapters := gocan.Rescan()
	sw.mu.Lock()
	clear(sw.adapters)
	for _, adapter := range adapters {
		sw.adapters[adapter.Name] = &adapter
	}
	sw.mu.Unlock()
	sw.adapterSelector.SetOptions(sw.sortedAdapterNames())
}

// AddAdapters extends the adapter list with adapters discovered after
// startup (e.g. the 32-bit J2534 DLLs served by j2534proxy).
func (sw *Widget) AddAdapters(adapters []gocan.AdapterInfo) {
	if len(adapters) == 0 {
		return
	}
	sw.mu.Lock()
	for _, adapter := range adapters {
		if _, found := sw.adapters[adapter.Name]; found {
			continue
		}
		sw.adapters[adapter.Name] = &adapter
	}
	sw.mu.Unlock()
	if sw.adapterSelector != nil {
		sw.adapterSelector.SetOptions(sw.sortedAdapterNames())
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
