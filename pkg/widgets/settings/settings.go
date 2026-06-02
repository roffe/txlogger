package settings

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/proto"
	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ota"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
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

func (cs *Widget) GetAdapter(ecuType string) (gocan.Adapter, error) {
	return cs.GetAdapterWithExtraFilters(ecuType, []uint32{})
}

func (cs *Widget) GetAdapterWithExtraFilters(ecuType string, filters []uint32) (gocan.Adapter, error) {
	debug := fyne.CurrentApp().Preferences().Bool(prefsDebug)
	port := fyne.CurrentApp().Preferences().String(prefsPort)

	baudstring := fyne.CurrentApp().Preferences().String(prefsSpeed)
	switch baudstring {
	case "1mbit":
		baudstring = "1000000"
	case "2mbit":
		baudstring = "2000000"
	case "3mbit":
		baudstring = "3000000"
	}

	if baudstring == "" {
		baudstring = "1000000"
	}

	baudrate, err := strconv.Atoi(baudstring)
	if err != nil {
		return nil, err
	}
	adapterName := fyne.CurrentApp().Preferences().String(prefsAdapter)

	if adapterName == "" {
		return nil, errors.New("Select CANbus adapter in settings") //lint:ignore ST1005 This is ok
	}

	if ad, found := cs.adapters[adapterName]; found {
		if ad.RequiresSerialPort {
			if port == "" {
				return nil, errors.New("Select port in setings") //lint:ignore ST1005 This is ok
			}
			if baudstring == "" {
				return nil, errors.New("Select port speed in settings") //lint:ignore ST1005 This is ok
			}
		}
	}

	var canFilter []uint32
	var canRate float64

	switch ecuType {
	case "T5", "Trionic 5":
		canFilter = []uint32{0xC}
		canRate = 615.384
	case "T7", "Trionic 7":
		if strings.Contains(adapterName, "ELM327") || strings.Contains(adapterName, "STN") || strings.Contains(adapterName, "OBDLink") || strings.HasSuffix(adapterName, "Wifi") {
			canFilter = []uint32{0x238, 0x258, 0x270}
		} else {
			canFilter = []uint32{0x1A0, 0x238, 0x258, 0x270, 0x280, 0x3A0, 0x664, 0x665}
		}
		if fyne.CurrentApp().Preferences().StringWithFallback(prefsWblSource, "None") == "CAN" {
			canFilter = append(canFilter, 0x180)
		}
		canRate = 500
	case "T8", "Trionic 8", "Trionic 8 MCP", "Z22SE", "Z22SE MCP":
		if strings.Contains(adapterName, "ELM327") || strings.Contains(adapterName, "STN") || strings.Contains(adapterName, "OBDLink") {
			canFilter = []uint32{0x5E8, 0x7E8}
		} else {
			canFilter = []uint32{0x5E8, 0x7E8, 0x664, 0x665}
		}
		if fyne.CurrentApp().Preferences().StringWithFallback(prefsWblSource, "None") == "CAN" {
			canFilter = append(canFilter, 0x180)
		}
		canFilter = append(canFilter, filters...)

		canRate = 500
	}

	cfg := &gocan.AdapterConfig{
		Port:         port,
		PortBaudrate: baudrate,
		CANRate:      canRate,
		CANFilter:    canFilter,
		Debug:        debug,
		PrintVersion: true,
	}

	if strings.HasPrefix(adapterName, "J2534") { // || strings.HasPrefix(adapterName, "CANlib") {
		return gocan.NewGWClient(adapterName, cfg)
	}

	if adapterName == "txbridge wifi" {
		//ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		//defer cancel()
		//addr, err := mdns.Query(ctx, "txbridge.local")
		//if err != nil {
		//	cs.cfg.Logger(fmt.Sprintf("Failed to resolve txbridge address via mDNS: %v", err))
		//} else {
		cfg.AdditionalConfig = map[string]string{
			"address":    fmt.Sprintf("%s:%d", "192.168.4.1", 1337),
			"minversion": ota.MinimumtxbridgeVersion,
		}
		//}
	}
	return gocan.NewAdapter(adapterName, cfg)
}

func (sw *Widget) GetADScannerSymbolName() string {
	return fyne.CurrentApp().Preferences().String(prefsWBLADScannerSymbol)
}

func (sw *Widget) GetWidebandName() string {
	return fyne.CurrentApp().Preferences().StringWithFallback(prefsWblSource, "None")
}

func (sw *Widget) GetWidebandSymbolName() string {
	useADScanner := fyne.CurrentApp().Preferences().Bool(prefsUseADScanner)
	switch sw.GetWidebandName() {
	case "ECU":
		switch sw.cfg.SelectedEcuFunc() {
		case "T5":
			return datalogger.LAMBDAADSCANNER // Lambda.ADScanner
		case "T7":
			if useADScanner {
				return datalogger.LAMBDAADSCANNER // Lambda.ADScanner
			}
			return "DisplProt.LambdaScanner"
		case "T8":
			if useADScanner {
				return datalogger.LAMBDAADSCANNER // Lambda.ADScanner
			}
			return "LambdaScan.LambdaScanner"
		default:
			return "None"
		}
	case aem.ProductString,
		"CombiAdapter",
		ecumaster.ProductString,
		innovate.ProductString,
		plx.ProductString,
		stag.ProductString,
		zeitronix.ProductString:
		return datalogger.EXTERNALWBLSYM // Lambda.External
	default:
		return "None"
	}
}

func (sw *Widget) GetColorBlindMode() colors.ColorBlindMode {
	return colors.StringToColorBlindMode(fyne.CurrentApp().Preferences().StringWithFallback(prefsColorBlindMode, "Normal"))
}

func (sw *Widget) GetWidebandPort() string {
	return fyne.CurrentApp().Preferences().String(prefsWBLPort)
}

func (sw *Widget) GetWBLSupportPoints() []int {
	return fyne.CurrentApp().Preferences().IntListWithFallback(prefsWBLSupportPoints, []int{0, 1024})
}

func (sw *Widget) GetWBLLambdaValues() []float64 {
	return fyne.CurrentApp().Preferences().FloatListWithFallback(prefsWBLLambdaValues, []float64{0.5, 1.5})
}

func (sw *Widget) GetUseADScanner() bool {
	if fyne.CurrentApp().Preferences().String(prefsWblSource) != "ECU" {
		return false
	}
	return fyne.CurrentApp().Preferences().Bool(prefsUseADScanner)
}

func (sw *Widget) GetFreq() int {
	return int(fyne.CurrentApp().Preferences().IntWithFallback(prefsFreq, 25))
}

func (sw *Widget) GetAutoSave() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsAutoUpdateSaveEcu)
}

func (sw *Widget) GetAutoLoad() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsAutoUpdateLoadEcu)
}

func (sw *Widget) GetLivePreview() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsLivePreview)
}

func (sw *Widget) GetRealtimeBars() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsRealtimeBars)
}

func (sw *Widget) GetMeshView() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsMeshView)
}

func (sw *Widget) GetLogFormat() string {
	return fyne.CurrentApp().Preferences().StringWithFallback(prefsLogFormat, "CSV")
}

func (sw *Widget) GetLogPath() string {
	p := fyne.CurrentApp().Preferences().String(prefsLogPath)
	if p == "" {
		var err error
		p, err = common.GetLogPath()
		if err != nil {
			log.Println("GetLogPath: ", err)
		}
	}
	return p
}

func (sw *Widget) GetUseMPH() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsUseMPH)
}

func (sw *Widget) GetSwapRPMandSpeed() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsSwapRPMandSpeed)
}

func (sw *Widget) GetCursorFollowCrosshair() bool {
	return fyne.CurrentApp().Preferences().Bool(prefsCursorFollowCrosshair)
}
