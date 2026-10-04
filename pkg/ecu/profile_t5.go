package ecu

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/txlogger/pkg/dtc"
	"github.com/roffe/txlogger/pkg/t5can"
)

var t5Profile = Profile{
	Name:                "T5",
	Type:                symbol.ECU_T5,
	CANRate:             615.384,
	CANFilter:           func(string) []uint32 { return []uint32{0xC} },
	UnsupportedAdapters: []string{"J2534", "ELM327"}, // can't do 615 kbit/s
	RAMAddress: func(sym *symbol.Symbol) (uint32, bool) {
		return sym.SramOffset, true
	},
	Signals: map[Signal]string{
		RPM:         "Rpm",
		Speed:       "Bil_hast",
		IAT:         "Lufttemp",
		Coolant:     "Kyl_temp",
		MAP:         "P_medel",
		PreThrottle: "Max_tryck",
		Throttle:    "Medeltrot",
		BoostPWM:    "PWM_ut10",
		NBLambda:    "Lambdaint",
		Ignition:    "Ign_angle",
		Knock:       "Knock_offset1234",
		InjTime:     "Insptid_ms10",
	},
	Scale: map[string]func(float64) float64{
		// throttle AD count 0-192
		"Medeltrot": func(v float64) float64 { return min(192, v) / 192 * 100 },
		// integrator around 128: 0..128..255 maps to -25..0..25
		"Lambdaint": func(v float64) float64 {
			if v < 128 {
				return (v - 128) * 25 / 128
			}
			return (v - 128) * 25 / 127
		},
	},
	ADScannerSymbols: []string{"AD_EGR"},
	ADResolution:     255,
	ReadDTC:          readT5DTC,
	ClearDTC:         clearT5DTC,
}

// t5DTCSymbols are the one-byte error flags in the loaded binary; T5 has no
// DTC service, its faults live in RAM.
func t5DTCSymbols(fw symbol.FirmwareFile) ([]*symbol.Symbol, error) {
	if fw == nil {
		return nil, errors.New("it is required to load a binary file to read DTCs")
	}
	var out []*symbol.Symbol
	for _, s := range fw.Symbols() {
		if (strings.HasSuffix(s.Name, "_error") || strings.HasSuffix(s.Name, "_fel")) && s.Length == 1 {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b *symbol.Symbol) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

func readT5DTC(ctx context.Context, cl *gocan.Bus, fw symbol.FirmwareFile) ([]dtc.DTC, error) {
	syms, err := t5DTCSymbols(fw)
	if err != nil {
		return nil, err
	}
	t5 := t5can.NewClient(cl)
	dtcs := []dtc.DTC{}
	for _, sym := range syms {
		val, err := t5.ReadRam(ctx, sym.SramOffset, uint32(sym.Length))
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", sym.Name, err)
		}
		if len(val) != 1 {
			return nil, fmt.Errorf("unexpected DTC length for symbol %s: %d", sym.Name, len(val))
		}
		if val[0] != 0 {
			dtcs = append(dtcs, dtc.DTC{ECU: symbol.ECU_T5, Code: sym.Name, Status: val[0]})
		}
	}
	return dtcs, nil
}

// clearT5DTC zeroes every set error flag, carrying on past failures.
func clearT5DTC(ctx context.Context, cl *gocan.Bus, fw symbol.FirmwareFile) error {
	syms, err := t5DTCSymbols(fw)
	if err != nil {
		return err
	}
	t5 := t5can.NewClient(cl)
	var errs []error
	for _, sym := range syms {
		res, err := t5.ReadRam(ctx, sym.SramOffset, uint32(sym.Length))
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("error reading %s before clearing: %w", sym.Name, err))
			continue
		case len(res) != 1:
			errs = append(errs, fmt.Errorf("unexpected DTC length for symbol %s: %d", sym.Name, len(res)))
			continue
		case res[0] == 0:
			continue // already cleared
		}
		if err := t5.WriteRam(ctx, sym.SramOffset, []byte{0x00}); err != nil {
			errs = append(errs, fmt.Errorf("error clearing %s: %w", sym.Name, err))
		}
	}
	return errors.Join(errs...)
}
