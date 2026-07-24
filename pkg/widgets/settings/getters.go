package settings

import (
	"log"

	"github.com/roffe/txlogger/pkg/colors"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/wbl/aem"
	"github.com/roffe/txlogger/pkg/wbl/ecumaster"
	"github.com/roffe/txlogger/pkg/wbl/innovate"
	"github.com/roffe/txlogger/pkg/wbl/plx"
	"github.com/roffe/txlogger/pkg/wbl/stag"
	"github.com/roffe/txlogger/pkg/wbl/zeitronix"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
)

// General
func (sw *Widget) GetFreq() int                   { return prefFreq.get() }
func (sw *Widget) GetAutoSave() bool              { return prefAutoSave.get() }
func (sw *Widget) GetAutoLoad() bool              { return prefAutoLoad.get() }
func (sw *Widget) GetLivePreview() bool           { return prefLivePreview.get() }
func (sw *Widget) GetRealtimeBars() bool          { return prefRealtimeBars.get() }
func (sw *Widget) GetMeshView() bool              { return prefMeshView.get() }
func (sw *Widget) GetCursorFollowCrosshair() bool { return prefCursorFollowCrosshair.get() }

func (sw *Widget) GetColorBlindMode() colors.ColorBlindMode {
	return colors.StringToColorBlindMode(prefColorBlindMode.get())
}

// Graphics
func (sw *Widget) GetPlotterRenderer() plotter.PlotBackend {
	return plotter.PlotBackend(prefPlotterRenderer.get())
}

func (sw *Widget) GetMeshRenderer() meshgrid.RenderBackend {
	return meshgrid.RenderBackend(prefMeshRenderer.get())
}

// GetClassicGauges reports whether dials should render in the old look.
func (sw *Widget) GetClassicGauges() bool { return prefGaugeStyle.get() == "Classic" }

// Logging
func (sw *Widget) GetLogFormat() string { return prefLogFormat.get() }

func (sw *Widget) GetLogPath() string {
	if p := prefLogPath.get(); p != "" {
		return p
	}
	p, err := common.GetLogPath()
	if err != nil {
		log.Println("GetLogPath: ", err)
	}
	return p
}

func (sw *Widget) GetExperimentalT5FastLogger() bool { return prefExperimentalT5FastLogger.get() }

// Dashboard
func (sw *Widget) GetUseMPH() bool          { return prefUseMPH.get() }
func (sw *Widget) GetSwapRPMandSpeed() bool { return prefSwapRPMandSpeed.get() }

// Wideband
func (sw *Widget) GetADScannerSymbolName() string { return prefWBLADScannerSymbol.get() }
func (sw *Widget) GetWidebandName() string        { return prefWblSource.get() }
func (sw *Widget) GetWidebandPort() string        { return prefWBLPort.get() }
func (sw *Widget) GetWBLSupportPoints() []int     { return prefWBLSupportPoints.get() }
func (sw *Widget) GetWBLLambdaValues() []float64  { return prefWBLLambdaValues.get() }

func (sw *Widget) GetUseADScanner() bool {
	if prefWblSource.get() != "ECU" {
		return false
	}
	return prefUseADScanner.get()
}

// GetWidebandSymbolName resolves the symbol to log for the selected wideband
// source, taking the connected ECU and AD scanner mode into account.
func (sw *Widget) GetWidebandSymbolName() string {
	switch sw.GetWidebandName() {
	case "ECU":
		useADScanner := prefUseADScanner.get()
		switch sw.cfg.SelectedEcuFunc() {
		case "T5":
			return datalogger.LAMBDAADSCANNER
		case "T7":
			if useADScanner {
				return datalogger.LAMBDAADSCANNER
			}
			return "DisplProt.LambdaScanner"
		case "T8":
			if useADScanner {
				return datalogger.LAMBDAADSCANNER
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
		return datalogger.EXTERNALWBLSYM
	default:
		return "None"
	}
}
