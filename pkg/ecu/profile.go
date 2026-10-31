package ecu

import (
	"context"
	"slices"
	"strings"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/dtc"
)

// Signal is a hardware-agnostic log channel. Widgets ask for a Signal and
// each Profile names the symbol carrying it on that ECU, so no widget needs
// to know which ECU is selected.
type Signal string

const (
	RPM           Signal = "rpm"
	Speed         Signal = "speed"          // vehicle speed, km/h
	SpeedUndriven Signal = "speed-undriven" // undriven wheel: immune to wheelspin
	IAT           Signal = "iat"            // intake air temperature, °C
	Coolant       Signal = "coolant"        // °C
	MAP           Signal = "map"            // manifold pressure
	PreThrottle   Signal = "pre-throttle"   // pressure before the throttle
	Throttle      Signal = "throttle"       // pedal / throttle position, %
	BoostPWM      Signal = "boost-pwm"      // boost control valve duty, %
	NBLambda      Signal = "nb-lambda"      // narrowband lambda integrator, %
	Airmass       Signal = "airmass"        // measured, mg/c
	AirmassReq    Signal = "airmass-req"    // requested, mg/c
	Ignition      Signal = "ignition"       // ignition advance, °
	IgnOffset     Signal = "ign-offset"     // knock retard
	Knock         Signal = "knock"          // knocking cylinders
	ActiveAirDem  Signal = "active-airdem"  // see Profile.AirDemToString
	FuelAdapt     Signal = "fuel-adapt"     // multiplicative fuel adaption
	InjDuty       Signal = "inj-duty"       // injector duty cycle, %
	InjTime       Signal = "inj-time"       // injection time, ms
)

// Profile is everything the app knows about one ECU family outside its wire
// protocol clients. Code that behaves differently per ECU reads it from here
// instead of switching on the ECU name; every optional field has a "not
// supported" zero value; see profiles for adding an ECU.
type Profile struct {
	Name string // selector, preferences, layouts and shortcuts key
	Type symbol.ECUType

	// CANRate (kbit/s) and CANFilter (acceptance ids for the named adapter)
	// used for logging and diagnostics. nil CANFilter: txlogger can't talk to
	// this ECU, only edit its binaries.
	CANRate   float64
	CANFilter func(adapterName string) []uint32
	// CANWideband: a CAN wideband (wbl.CANIDs) can share this ECU's bus.
	CANWideband bool
	// UnsupportedAdapters are adapter name fragments that can't reach it.
	UnsupportedAdapters []string

	// RAMAddress is where sym lives for live RAM read/write; ok is false
	// when it isn't RAM backed. nil: no live RAM access.
	RAMAddress func(sym *symbol.Symbol) (addr uint32, ok bool)

	// Signals names the log symbol for each Signal the ECU has.
	Signals map[Signal]string
	// Scale converts the raw value of Signals symbols that aren't logged in
	// their Signal's unit.
	Scale map[string]func(float64) float64
	// ForeignSymbols betray a symbol preset made for another ECU.
	ForeignSymbols []string
	// SymbolAliases maps a map name to the name an older firmware variant
	// uses for the same map (T5.2 vs T5.5), tried when the first is absent.
	SymbolAliases map[string]string
	// AirDemToString names an ActiveAirDem value.
	AirDemToString func(float64) string

	// WidebandSymbol is the ECU's own wideband input; "" when it only has
	// the AD scanner.
	WidebandSymbol string
	// ADScannerSymbols can carry an AD scanner wideband; ADResolution is the
	// scanner's full-scale count. Empty/0: no AD scanner.
	ADScannerSymbols []string
	ADResolution     int

	// ReadDTC and ClearDTC talk to the ECU over an open bus; fw is the loaded
	// binary, may be nil. nil funcs: not supported.
	ReadDTC  func(ctx context.Context, cl *gocan.Bus, fw symbol.FirmwareFile) ([]dtc.DTC, error)
	ClearDTC func(ctx context.Context, cl *gocan.Bus, fw symbol.FirmwareFile) error
}

// profiles is every selectable ECU, in selector order.
//
// Adding an ECU:
//  1. ecusymbol: its ECUType, detection and loader, for binary editing.
//  2. profile_<ecu>.go here, listed below. Name and Type alone give the
//     selector, map viewer, symbol browser, shortcuts and scripts dir; each
//     optional field lights up its feature (CAN*: adapters, ReadDTC: DTC
//     reader, RAMAddress: live map editing, Signals: dashboard, street dyno
//     and VE, WidebandSymbol/AD*: wideband settings).
//  3. Per-subsystem tables keyed by Name, for what can't be data:
//     datalogger loggers (+ txbridgeECUs), windows ecuMenus (mainmenu_<ecu>.go),
//     dtc infoDB/wisModels/Title, estimatedoutput calculators, a "<Name> Dash"
//     preset.
//  4. Flashing is a separate registry: Register an EcuInfo from the client
//     package (pkg/ecu/t7 etc.); canflasher lists it by itself.
var profiles = []*Profile{&t5Profile, &t7Profile, &t8Profile, &aw55Profile}

// Profiles returns every selectable ECU's profile, in selector order.
func Profiles() []*Profile { return profiles }

// ProfileNames returns every selectable ECU's name, in selector order.
func ProfileNames() []string {
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names
}

// GetProfile returns the named ECU's profile, or one with nothing supported
// when name is unknown.
func GetProfile(name string) *Profile {
	for _, p := range profiles {
		if p.Name == name {
			return p
		}
	}
	return &Profile{Name: name, Type: symbol.ECU_UNKNOWN}
}

// ProfileForLog guesses which ECU recorded a log from its column names: the
// profile with the most of its signal symbols in cols. fallback wins ties.
func ProfileForLog(cols []string, fallback *Profile) *Profile {
	count := func(p *Profile) (n int) {
		for _, sym := range p.Signals {
			if slices.Contains(cols, sym) {
				n++
			}
		}
		return n
	}
	best, bestN := fallback, count(fallback)
	for _, p := range profiles {
		if n := count(p); n > bestN {
			best, bestN = p, n
		}
	}
	return best
}

// IsOBDAdapter reports whether the adapter is an OBD-II dongle, which can
// only keep up with a few CAN ids.
func IsOBDAdapter(name string) bool {
	return strings.Contains(name, "ELM327") ||
		strings.Contains(name, "STN") ||
		strings.Contains(name, "OBDLink")
}
