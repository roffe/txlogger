package ecu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/t7kwp"
	"github.com/roffe/txlogger/pkg/dtc"
)

var t7Profile = Profile{
	Name:    "T7",
	Type:    symbol.ECU_T7,
	CANRate: 500,
	CANFilter: func(adapterName string) []uint32 {
		if IsOBDAdapter(adapterName) || strings.HasSuffix(adapterName, "Wifi") {
			return []uint32{0x238, 0x258, 0x270}
		}
		return []uint32{0x1A0, 0x238, 0x258, 0x270, 0x280, 0x3A0}
	},
	CANWideband: true,
	// Closed binaries keep calibration in flash (< 0x80000): the stock ECU
	// refuses 0x23 reads there (NRC 0x12), MapTun-patched ones reset, and
	// flash can't be written live anyway. Only RAM-backed maps are live.
	RAMAddress: func(sym *symbol.Symbol) (uint32, bool) {
		return sym.Address, sym.Address >= 0x80000
	},
	Signals: map[Signal]string{
		RPM:           "ActualIn.n_Engine",
		Speed:         "In.v_Vehicle", // left front, the driven wheel
		SpeedUndriven: "In.v_Vehicle2",
		IAT:           "ActualIn.T_AirInlet",
		Coolant:       "ActualIn.T_Engine",
		MAP:           "ActualIn.p_AirInlet",
		PreThrottle:   "In.p_AirBefThrottle",
		Throttle:      "Out.X_AccPedal",
		BoostPWM:      "Out.PWM_BoostCntrl",
		NBLambda:      "Lambda.LambdaInt",
		Airmass:       "MAF.m_AirInlet",
		AirmassReq:    "m_Request",
		Ignition:      "Out.fi_Ignition",
		IgnOffset:     "IgnProt.fi_Offset",
		Knock:         "KnkDet.KnockCyl",
		ActiveAirDem:  "ECMStat.ST_ActiveAirDem",
		FuelAdapt:     "AdpFuelProt.MulFuelAdapt",
		InjDuty:       "Myrtilos.InjectorDutyCycle",
	},
	ForeignSymbols:   []string{"AirMassMast.m_Request"},
	AirDemToString:   airDemT7,
	WidebandSymbol:   "DisplProt.LambdaScanner",
	ADScannerSymbols: []string{"DisplProt.AD_Scanner"},
	ADResolution:     1023,
	ReadDTC:          readT7DTC,
	ClearDTC:         clearT7DTC,
}

// t7Session runs fn in a KWP session, reporting a failed StopSession too.
func t7Session(ctx context.Context, cl *gocan.Bus, fn func(*t7kwp.Client) error) (err error) {
	kwp := t7kwp.New(cl)
	if err := kwp.StartSession(ctx, t7kwp.INIT_MSG_ID, t7kwp.INIT_RESP_ID); err != nil {
		return err
	}
	defer func() {
		if serr := kwp.StopSession(ctx); serr != nil {
			err = errors.Join(err, fmt.Errorf("error stopping session: %w", serr))
		}
		time.Sleep(75 * time.Millisecond)
	}()
	return fn(kwp)
}

func readT7DTC(ctx context.Context, cl *gocan.Bus, _ symbol.FirmwareFile) (out []dtc.DTC, err error) {
	err = t7Session(ctx, cl, func(kwp *t7kwp.Client) error {
		dtcs, err := kwp.ReadDTCByStatus(ctx, 0x02)
		for _, d := range dtcs {
			out = append(out, dtc.DTC{ECU: symbol.ECU_T7, Code: d.Code, Status: d.Status})
		}
		return err
	})
	return out, err
}

func clearT7DTC(ctx context.Context, cl *gocan.Bus, _ symbol.FirmwareFile) error {
	return t7Session(ctx, cl, func(kwp *t7kwp.Client) error {
		return kwp.ClearDTCS(ctx)
	})
}

func airDemT7(v float64) string {
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
		return "Stall Limit"
	case 25:
		return "Special Mode"
	case 26:
		return "Reverse Limit (Auto)"
	case 27:
		return "Misfire diagnose"
	case 28:
		return "Brake Management"
	case 29:
		return "Diff Prot (Automatic)"
	case 30:
		return "Not used"
	case 31:
		return "Max Vehicle Speed"
	case 40:
		return "LDA Request"
	case 41:
		return "Min Load"
	case 42:
		return "Dash Pot"
	case 50:
		return "Knock Airmass Limit"
	case 51:
		return "Max Engine Speed"
	case 52:
		return "Max Air for Lambda 1"
	case 53:
		return "Max Turbo Speed"
	case 54:
		return "Crankcase vent error"
	case 55:
		return "Faulty APC valve"
	case 60:
		return "Emission Limitation"
	case 61:
		return "Engine Tipin"
	case 62:
		return "Engine Tipout"
	case 70:
		return "Safety Switch"
	case 71:
		return "O2 Sens fault, E85"
	case 80:
		return "Cold engine temp"
	case 81:
		return "Overheating"
	default:
		return "Unknown"
	}
}
