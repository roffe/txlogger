package ecu

import (
	"maps"
	"math"
	"slices"
	"testing"
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
