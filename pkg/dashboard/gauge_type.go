package dashboard

type GaugeType int

const (
	SpeedDial GaugeType = iota
	RpmDial
	IatDial
	EngineTempDial
	AirmassDialPrimary
	AirmassDialSecondary
	PressureDialPrimary
	PressureDialSecondary
	ThrottleBar
	PWMBar
	WBLambdaBar
	NBLambdaBar
)

func (d GaugeType) String() string {
	switch d {
	case SpeedDial:
		return "Speed"
	case RpmDial:
		return "Rpm"
	case IatDial:
		return "Iat"
	case EngineTempDial:
		return "Engine Temp"
	case AirmassDialPrimary:
		return "Airmass Primary"
	case AirmassDialSecondary:
		return "Airmass Secondary"
	case PressureDialPrimary:
		return "Pressure Primary"
	case PressureDialSecondary:
		return "Pressure Secondary"
	case ThrottleBar:
		return "Throttle"
	case PWMBar:
		return "PWM"
	case WBLambdaBar:
		return "WBLambda"
	case NBLambdaBar:
		return "NBLambda"
	default:
		return "Unknown"
	}
}
