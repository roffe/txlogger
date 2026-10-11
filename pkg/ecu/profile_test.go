package ecu

import (
	"maps"
	"math"
	"slices"
	"testing"

	symbol "github.com/roffe/ecusymbol"
)

func TestProfiles(t *testing.T) {
	if got := ProfileNames(); !slices.Equal(got, []string{"T5", "T7", "T8", "AW55"}) {
		t.Fatalf("selector order changed: %v", got)
	}
	for _, p := range Profiles() {
		// Name is what binaries load as (symbol.Load -> ECUType.String()).
		if p.Name != p.Type.String() {
			t.Errorf("%s: Type %v loads as %q", p.Name, p.Type, p.Type.String())
		}
		if p.CANFilter != nil && p.CANRate == 0 {
			t.Errorf("%s: CANFilter without CANRate", p.Name)
		}
		for sym := range p.Scale {
			if !slices.Contains(slices.Collect(maps.Values(p.Signals)), sym) {
				t.Errorf("%s: Scale for %s, which no Signal names", p.Name, sym)
			}
		}
	}
	if p := GetProfile("nope"); p.Name != "nope" || p.CANFilter != nil {
		t.Errorf("unknown profile: %+v", p)
	}
}

func TestProfileForLog(t *testing.T) {
	t7, t8 := GetProfile("T7"), GetProfile("T8")
	t8log := []string{"ActualIn.n_Engine", "MAF.m_AirInlet", "AirMassMast.m_Request", "Out.X_AccPos"}
	if got := ProfileForLog(t8log, t7); got != t8 {
		t.Errorf("T8 log under T7: got %s", got.Name)
	}
	shared := []string{"ActualIn.n_Engine", "MAF.m_AirInlet"}
	if got := ProfileForLog(shared, t8); got != t8 {
		t.Errorf("tie should keep fallback, got %s", got.Name)
	}
}

func TestT5Scale(t *testing.T) {
	lambda := GetProfile("T5").Scale["Lambdaint"]
	for in, want := range map[float64]float64{0: -25, 128: 0, 255: 25} {
		if got := lambda(in); math.Abs(got-want) > 1e-9 {
			t.Errorf("Lambdaint(%v) = %v, want %v", in, got, want)
		}
	}
	if got := GetProfile("T5").Scale["Medeltrot"](250); got != 100 {
		t.Errorf("Medeltrot clamps at 192: got %v", got)
	}
}

// IsOBDAdapter matches the OBD-II dongle families by name fragment.
func TestIsOBDAdapter(t *testing.T) {
	for name, want := range map[string]bool{
		"ELM327":            true,
		"ELM327 Wifi":       true,
		"STN2120":           true,
		"OBDLink SX":        true,
		"CANUSB":            false,
		"CombiAdapter":      false,
		"J2534 #1 Mongoose": false,
		"txbridge Wifi":     false,
		"":                  false,
	} {
		if got := IsOBDAdapter(name); got != want {
			t.Errorf("IsOBDAdapter(%q) = %v, want %v", name, got, want)
		}
	}
}

// CANFilter picks the acceptance ids per adapter: T7 drops its broadcast
// frames for OBD dongles and Wifi bridges, which can't keep up with them.
func TestCANFilter(t *testing.T) {
	t7broadcast := []uint32{0x1A0, 0x238, 0x258, 0x270, 0x280, 0x3A0}
	t7diag := []uint32{0x238, 0x258, 0x270}
	cases := []struct {
		ecu, adapter string
		want         []uint32
	}{
		{"T5", "CANUSB", []uint32{0xC}},
		{"T7", "CANUSB", t7broadcast},
		{"T7", "CombiAdapter", t7broadcast},
		{"T7", "ELM327", t7diag},
		{"T7", "OBDLink SX", t7diag},
		{"T7", "txbridge Wifi", t7diag},
		{"T8", "CANUSB", []uint32{0x5E8, 0x7E8}},
		{"T8", "ELM327", []uint32{0x5E8, 0x7E8}},
	}
	for _, c := range cases {
		if got := GetProfile(c.ecu).CANFilter(c.adapter); !slices.Equal(got, c.want) {
			t.Errorf("%s CANFilter(%q) = %X, want %X", c.ecu, c.adapter, got, c.want)
		}
	}
	if GetProfile("AW55").CANFilter != nil {
		t.Error("AW55 is binary editing only, it must have no CANFilter")
	}
}

// RAMAddress maps a symbol to its live RAM address per ECU: T5 uses the SRAM
// offset, T7 only RAM-backed symbols (>= 0x80000), T8 address plus offset
// when that lands in SRAM (>= 0x100000).
func TestRAMAddress(t *testing.T) {
	cases := []struct {
		ecu   string
		sym   symbol.Symbol
		addr  uint32
		inRAM bool
	}{
		{"T5", symbol.Symbol{Address: 0x5000, SramOffset: 0x1234}, 0x1234, true},
		{"T7", symbol.Symbol{Address: 0x7FFFF, SramOffset: 0x10}, 0x7FFFF, false},
		{"T7", symbol.Symbol{Address: 0x80000}, 0x80000, true},
		{"T7", symbol.Symbol{Address: 0xF00010}, 0xF00010, true},
		{"T8", symbol.Symbol{Address: 0x100000, SramOffset: 0x4000}, 0x104000, true},
		{"T8", symbol.Symbol{Address: 0xA1046}, 0xA1046, false},
		{"T8", symbol.Symbol{Address: 0x102100}, 0x102100, true},
	}
	for _, c := range cases {
		addr, ok := GetProfile(c.ecu).RAMAddress(&c.sym)
		if addr != c.addr || ok != c.inRAM {
			t.Errorf("%s RAMAddress(addr 0x%X, sram 0x%X) = 0x%X, %v; want 0x%X, %v",
				c.ecu, c.sym.Address, c.sym.SramOffset, addr, ok, c.addr, c.inRAM)
		}
	}
	if GetProfile("AW55").RAMAddress != nil {
		t.Error("AW55 has no live RAM access")
	}
}

// AirDemToString names every active air demand code the ECU documents, each
// once, and "Unknown" for anything else. T7 and T8 number some sources
// differently, so each profile carries its own table.
func TestAirDemToString(t *testing.T) {
	codes := map[string][]float64{
		"T7": {10, 11, 12, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 40, 41, 42, 50, 51, 52, 53, 54, 55, 60, 61, 62, 70, 71, 80, 81},
		"T8": {10, 11, 12, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 40, 50, 52},
	}
	for ecu, known := range codes {
		name := GetProfile(ecu).AirDemToString
		seen := map[string]float64{}
		for _, c := range known {
			s := name(c)
			if s == "Unknown" || s == "" {
				t.Errorf("%s: code %v has no name", ecu, c)
			}
			if prev, dup := seen[s]; dup {
				t.Errorf("%s: codes %v and %v both named %q", ecu, prev, c, s)
			}
			seen[s] = c
		}
		for _, c := range []float64{0, 9, 13, 99, 10.5, -1} {
			if s := name(c); s != "Unknown" {
				t.Errorf("%s: code %v = %q, want Unknown", ecu, c, s)
			}
		}
	}
	t7, t8 := GetProfile("T7").AirDemToString, GetProfile("T8").AirDemToString
	for _, c := range []struct {
		code   float64
		t7, t8 string
	}{
		{10, "PedalMap", "PedalMap"},
		{27, "Misfire diagnose", "Max Vehicle speed"},
		{30, "Not used", "Max Engine Speed"},
		{51, "Max Engine Speed", "Unknown"},
		{52, "Max Air for Lambda 1", "Max Turbo Speed"},
		{81, "Overheating", "Unknown"},
	} {
		if got := t7(c.code); got != c.t7 {
			t.Errorf("T7 %v = %q, want %q", c.code, got, c.t7)
		}
		if got := t8(c.code); got != c.t8 {
			t.Errorf("T8 %v = %q, want %q", c.code, got, c.t8)
		}
	}
}

// Profile fields that only make sense together are set together, and each
// ForeignSymbols entry is a signal of some other profile (that is how it
// betrays a preset made for that ECU).
func TestProfileInvariants(t *testing.T) {
	for _, p := range Profiles() {
		if (len(p.ADScannerSymbols) > 0) != (p.ADResolution > 0) {
			t.Errorf("%s: ADScannerSymbols %v with ADResolution %d", p.Name, p.ADScannerSymbols, p.ADResolution)
		}
		if (p.ReadDTC == nil) != (p.ClearDTC == nil) {
			t.Errorf("%s: ReadDTC and ClearDTC must both be set or both nil", p.Name)
		}
		if p.ReadDTC != nil && p.CANFilter == nil {
			t.Errorf("%s: DTC access without CAN", p.Name)
		}
		if (p.Signals[ActiveAirDem] != "") != (p.AirDemToString != nil) {
			t.Errorf("%s: ActiveAirDem signal and AirDemToString must come together", p.Name)
		}
		own := slices.Collect(maps.Values(p.Signals))
		for _, f := range p.ForeignSymbols {
			if slices.Contains(own, f) {
				t.Errorf("%s: foreign symbol %s is one of its own signals", p.Name, f)
			}
			found := false
			for _, o := range Profiles() {
				if o != p && slices.Contains(slices.Collect(maps.Values(o.Signals)), f) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: foreign symbol %s is no other profile's signal", p.Name, f)
			}
		}
		for from, to := range p.SymbolAliases {
			if from == to || to == "" {
				t.Errorf("%s: alias %q -> %q", p.Name, from, to)
			}
		}
	}
}

// ProfileForLog falls back when no profile recognises more columns than the
// fallback does, and otherwise picks the best match even over a fallback
// with no signals at all; an exact tie between profiles goes to the one
// listed first.
func TestProfileForLogMore(t *testing.T) {
	t5, t7, t8 := GetProfile("T5"), GetProfile("T7"), GetProfile("T8")
	unknown := GetProfile("nope")
	cases := []struct {
		name     string
		cols     []string
		fallback *Profile
		want     *Profile
	}{
		{"empty log keeps fallback", nil, t8, t8},
		{"foreign columns keep fallback", []string{"Time", "Lambda.External"}, t7, t7},
		{"T5 log under T7", []string{"Rpm", "P_medel", "Lufttemp", "ActualIn.n_Engine"}, t7, t5},
		{"unknown fallback loses to any match", []string{"Rpm"}, unknown, t5},
		{"T7/T8 tie under unknown goes to T7", []string{"ActualIn.n_Engine", "MAF.m_AirInlet"}, unknown, t7},
		{"T7-only symbol breaks the tie", []string{"ActualIn.n_Engine", "MAF.m_AirInlet", "m_Request"}, t8, t7},
	}
	for _, c := range cases {
		if got := ProfileForLog(c.cols, c.fallback); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got.Name, c.want.Name)
		}
	}
}
