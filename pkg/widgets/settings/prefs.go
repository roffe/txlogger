package settings

import "fyne.io/fyne/v2"

// prefs returns the application preference store. Every setting in this package
// is read and written through the typed descriptors below so that storage keys
// and default values live in exactly one place.
func prefs() fyne.Preferences {
	return fyne.CurrentApp().Preferences()
}

// Exported preference keys read directly by other packages (flash settings).
const (
	PrefsNvdm = "nvdm"
	PrefsBoot = "boot"
)

// --- typed preference descriptors ------------------------------------------
//
// Each descriptor bundles a storage key with its default value and exposes
// get/set helpers, so adding a new setting is a single declaration.

type boolPref struct {
	key string
	def bool
}

func (p boolPref) get() bool  { return prefs().BoolWithFallback(p.key, p.def) }
func (p boolPref) set(v bool) { prefs().SetBool(p.key, v) }

type stringPref struct {
	key string
	def string
}

func (p stringPref) get() string  { return prefs().StringWithFallback(p.key, p.def) }
func (p stringPref) set(v string) { prefs().SetString(p.key, v) }

// getOr reads the value but uses the supplied fallback instead of the
// descriptor's default (used when the default must be computed at runtime).
func (p stringPref) getOr(fallback string) string {
	return prefs().StringWithFallback(p.key, fallback)
}

type intPref struct {
	key string
	def int
}

func (p intPref) get() int  { return prefs().IntWithFallback(p.key, p.def) }
func (p intPref) set(v int) { prefs().SetInt(p.key, v) }

type intListPref struct {
	key string
	def []int
}

func (p intListPref) get() []int  { return prefs().IntListWithFallback(p.key, p.def) }
func (p intListPref) set(v []int) { prefs().SetIntList(p.key, v) }

type floatListPref struct {
	key string
	def []float64
}

func (p floatListPref) get() []float64  { return prefs().FloatListWithFallback(p.key, p.def) }
func (p floatListPref) set(v []float64) { prefs().SetFloatList(p.key, v) }

// --- preference declarations -----------------------------------------------

var (
	// General
	prefFreq                  = intPref{"freq", 25}
	prefAutoLoad              = boolPref{"autoUpdateLoadEcu", true}
	prefAutoSave              = boolPref{"autoUpdateSaveEcu", false}
	prefCursorFollowCrosshair = boolPref{"cursorFollowCrosshair", false}
	prefLivePreview           = boolPref{"livePreview", true}
	prefMeshView              = boolPref{"liveMeshView", true}
	prefRealtimeBars          = boolPref{"realtimeBars", true}
	prefColorBlindMode        = stringPref{"colorBlindMode", "Normal"}

	// Graphics
	prefPlotterRenderer = intPref{"plotterRenderer", 0}
	prefMeshRenderer    = intPref{"meshRenderer", 2}
	prefGaugeStyle      = stringPref{"gaugeStyle", "Modern"}

	// Logging
	prefLogFormat                = stringPref{"logFormat", "CSV"}
	prefLogPath                  = stringPref{"logPath", ""}
	prefExperimentalT5FastLogger = boolPref{"experimentalT5FastLogger", false}

	// Dashboard
	prefUseMPH = boolPref{"useMPH", false}

	// Wideband
	prefWblSource           = stringPref{"wblSource", "None"}
	prefWidebandSymbolName  = stringPref{"widebandSymbolName", ""}
	prefWBLPort             = stringPref{"wblPort", ""}
	prefUseADScanner        = boolPref{"useADScanner", false}
	prefWBLADScannerSymbol  = stringPref{"wblADScannerSymbol", ""}
	prefWBLSupportPoints    = intListPref{"wblSupportPoints", []int{0, 1024}}
	prefWBLLambdaValues     = floatListPref{"wblLambdaValues", []float64{0.5, 1.5}}
	prefLastADScannerPreset = stringPref{"lastADScannerPreset", ""}
	prefLastADScannerECU    = stringPref{"lastADScannerECU", "T7"}

	// CAN
	prefAdapter = stringPref{"adapter", ""}
	prefPort    = stringPref{"port", ""}
	prefSpeed   = stringPref{"speed", "115200"}
	prefDebug   = boolPref{"debug", false}
)
