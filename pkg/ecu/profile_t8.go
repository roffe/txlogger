package ecu

import (
	"context"
	"fmt"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gmlan"
	"github.com/roffe/txlogger/pkg/dtc"
)

var t8Profile = Profile{
	Name:        "T8",
	Type:        symbol.ECU_T8,
	CANRate:     500,
	CANFilter:   func(string) []uint32 { return []uint32{0x5E8, 0x7E8} },
	CANWideband: true,
	RAMAddress: func(sym *symbol.Symbol) (uint32, bool) {
		return sym.Address + sym.SramOffset, true
	},
	Signals: map[Signal]string{
		RPM:           "ActualIn.n_Engine",
		Speed:         "In.v_Vehicle", // left front, the driven wheel
		SpeedUndriven: "In.v_Vehicle2",
		IAT:           "ActualIn.T_AirInlet",
		Coolant:       "ActualIn.T_Engine",
		MAP:           "In.p_AirInlet",
		PreThrottle:   "ActualIn.p_AirBefThrottle",
		Throttle:      "Out.X_AccPos",
		BoostPWM:      "Out.PWM_BoostCntrl",
		NBLambda:      "Lambda.LambdaInt",
		Airmass:       "MAF.m_AirInlet",
		AirmassReq:    "AirMassMast.m_Request",
		Ignition:      "Out.fi_Ignition",
		IgnOffset:     "IgnMastProt.fi_Offset",
		Knock:         "KnkDet.KnockCyl",
		ActiveAirDem:  "ECMStat.ST_ActiveAirDem",
	},
	ForeignSymbols:   []string{"m_Request"},
	AirDemToString:   airDemT8,
	WidebandSymbol:   "LambdaScan.LambdaScanner",
	ADScannerSymbols: []string{"LambdaScan.AD_Scanner", "LambdaScan.AD_Scanner2"},
	ADResolution:     1023,
	ReadDTC:          readT8DTC,
	ClearDTC:         clearT8DTC,
}

// t8Session runs fn in a GMLAN diagnostic session.
func t8Session(ctx context.Context, cl *gocan.Bus, fn func(*gmlan.Client) error) error {
	gm := gmlan.New(cl, 0x7e0, 0x7e8)
	if err := gm.InitiateDiagnosticOperation(ctx, gmlan.LEV_DADTC); err != nil {
		return err
	}
	defer func() {
		_ = gm.ReturnToNormalMode(ctx)
		time.Sleep(75 * time.Millisecond)
	}()
	return fn(gm)
}

func readT8DTC(ctx context.Context, cl *gocan.Bus, _ symbol.FirmwareFile) (out []dtc.DTC, err error) {
	err = t8Session(ctx, cl, func(gm *gmlan.Client) error {
		dtcs, err := gm.ReadDiagnosticInformation(ctx, 0x81, 0x12)
		for _, d := range dtcs {
			out = append(out, dtc.DTC{
				ECU: symbol.ECU_T8,
				// failure type suffix in the same form WIS uses, e.g. "B0165 02"
				Code:        fmt.Sprintf("%s %02X", d.Code, d.FailureType),
				FailureType: d.FailureType,
				Status:      d.Status,
			})
		}
		return err
	})
	return out, err
}

func clearT8DTC(ctx context.Context, cl *gocan.Bus, _ symbol.FirmwareFile) error {
	return t8Session(ctx, cl, func(gm *gmlan.Client) error {
		return gm.ClearDiagnosticInformation(ctx, 0x7DF)
	})
}

func airDemT8(v float64) string {
	switch v {
	case 10:
		return "PedalMap"
	case 11:
		return "Cruise Control"
	case 12:
		return "Idle Control"
	case 20:
		return "Max Engine Torque"
	case 21:
		return "Traction Control"
	case 22:
		return "Manual Gearbox Limit"
	case 23:
		return "Automatic Gearbox Lim"
	case 24:
		return "Stall Limit (Automatic)"
	case 25:
		return "Hardcoded Limit"
	case 26:
		return "Reverse Limit (Automatic)"
	case 27:
		return "Max Vehicle speed"
	case 28:
		return "Brake Management"
	case 29:
		return "System Action"
	case 30:
		return "Max Engine Speed"
	case 40:
		return "Min Load"
	case 50:
		return "Knock Airmass Limit"
	case 52:
		return "Max Turbo Speed"
	default:
		return "Unknown"
	}
}
