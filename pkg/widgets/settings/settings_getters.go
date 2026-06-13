package settings

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan"
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
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
)

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

func (sw *Widget) GetPlotterRenderer() plotter.PlotBackend {
	return plotter.PlotBackend(fyne.CurrentApp().Preferences().IntWithFallback(prefsPlotterRenderer, 0))
}

func (sw *Widget) GetMeshRenderer() meshgrid.RenderBackend {
	return meshgrid.RenderBackend(fyne.CurrentApp().Preferences().IntWithFallback(prefsMeshRenderer, 0))
}
