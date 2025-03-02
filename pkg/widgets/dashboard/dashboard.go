package dashboard

import (
	_ "embed"
	"image/color"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
	"github.com/roffe/txlogger/pkg/widgets/cbar"
	"github.com/roffe/txlogger/pkg/widgets/dial"
	"github.com/roffe/txlogger/pkg/widgets/dualdial"
	"github.com/roffe/txlogger/pkg/widgets/icon"
	"github.com/roffe/txlogger/pkg/widgets/vbar"
)

const rpmIDCconstant = 1.0 / 1200.0

type Dashboard struct {
	cfg *Config

	metricRouter map[string]func(float64)

	text   Texts
	image  Images
	gauges Gauges

	fullscreenBtn *widget.Button
	//dbgBar *fyne.Container

	logplayer bool

	timeBuffer []byte

	widget.BaseWidget
}

type Images struct {
	checkEngine *canvas.Image
	limpMode    *canvas.Image
	knockIcon   *icon.Icon
	taz         *canvas.Image
}

type Texts struct {
	ioff, activeAirDem, ign, cruise *canvas.Text
	idc, amul                       *canvas.Text
	time                            *canvas.Text
}

type Gauges struct {
	rpm, speed, iat    *dial.Dial
	throttle, pwm      *vbar.VBar
	engineTemp         *dial.Dial
	nblambda, wblambda *cbar.CBar
	pressure, airmass  *dualdial.DualDial
}

type Config struct {
	Logplayer       bool
	AirDemToString  func(float64) string
	UseMPH          bool
	SwapRPMandSpeed bool
	HighAFR         float64
	LowAFR          float64
	WidebandSymbol  string
	MetricRouter    map[string]func(float64)
	FullscreenFunc  func(bool)
}

func NewDashboard(cfg *Config) *Dashboard {
	if cfg.AirDemToString == nil {
		cfg.AirDemToString = func(f float64) string {
			return "Undefined"
		}
	}

	speedometerText := "km/h"
	if cfg.UseMPH {
		speedometerText = "mph"
	}

	db := &Dashboard{
		cfg:       cfg,
		logplayer: cfg.Logplayer,
		gauges: Gauges{
			airmass: dualdial.New(&widgets.GaugeConfig{
				Title:   "mg/c",
				Min:     0,
				Max:     2200,
				Steps:   22,
				MinSize: fyne.NewSize(100, 100),
			}),
			speed: dial.New(&widgets.GaugeConfig{
				Title:         speedometerText,
				Min:           0,
				Max:           300,
				Steps:         30,
				DisplayString: "%.1f",
				MinSize:       fyne.NewSize(100, 100),
			}),
			rpm: dial.New(&widgets.GaugeConfig{
				Title:   "RPM",
				Min:     0,
				Max:     8000,
				Steps:   20,
				MinSize: fyne.NewSize(100, 100),
			}),
			iat: dial.New(&widgets.GaugeConfig{
				Title:   "IAT",
				Min:     0,
				Max:     80,
				Steps:   16,
				MinSize: fyne.NewSize(100, 100),
			}),
			pressure: dualdial.New(&widgets.GaugeConfig{
				Title:         "MAP",
				Min:           0,
				Max:           3,
				Steps:         30,
				DisplayString: "%.2f",
				MinSize:       fyne.NewSize(100, 100),
			}),
			throttle: vbar.New(&widgets.GaugeConfig{
				Title:      "TPS",
				Min:        0,
				Max:        100,
				Steps:      20,
				MinSize:    fyne.NewSize(50, 50),
				ColorScale: widgets.TraditionalScale,
			}),
			pwm: vbar.New(&widgets.GaugeConfig{
				Title:      "PWM",
				Min:        0,
				Max:        100,
				Steps:      20,
				MinSize:    fyne.NewSize(50, 50),
				ColorScale: widgets.TraditionalScale,
			}),
			engineTemp: dial.New(&widgets.GaugeConfig{
				Title: "tEng",
				Min:   -20,
				Max:   130,
				Steps: 16,
			}),
			wblambda: cbar.New(&widgets.GaugeConfig{
				Title:           "",
				Min:             0.50,
				Center:          1,
				Max:             1.50,
				Steps:           20,
				MinSize:         fyne.NewSize(50, 35),
				DisplayString:   "λ %.3f",
				DisplayTextSize: 20,
				TextPosition:    widgets.TextAtTop,
			}),
			nblambda: cbar.New(&widgets.GaugeConfig{
				Title:           "",
				Min:             -25,
				Center:          0,
				Max:             25,
				Steps:           40,
				MinSize:         fyne.NewSize(50, 35),
				DisplayString:   "%.2f%%",
				DisplayTextSize: 20,
				TextPosition:    widgets.TextAtBottom,
			}),
		},
		text: Texts{
			cruise: &canvas.Text{
				Text:      "Cruise",
				Alignment: fyne.TextAlignLeading,
				Color:     color.RGBA{R: 0xFF, G: 0x67, B: 0, A: 0xFF},
				TextSize:  45,
			},
			activeAirDem: &canvas.Text{
				Text:      "None (0)",
				Alignment: fyne.TextAlignCenter,
				TextSize:  35,
			},
			ign: &canvas.Text{
				Text:      "Ign: 0.0",
				Alignment: fyne.TextAlignLeading,
				TextSize:  44,
			},
			ioff: &canvas.Text{
				Text:      "Ioff: 0.0",
				Alignment: fyne.TextAlignLeading,
				TextSize:  28,
				Color:     color.RGBA{R: 0, G: 255, B: 0, A: 255},
			},
			idc: &canvas.Text{
				Text:      "Idc: 00%",
				Alignment: fyne.TextAlignLeading,
				TextSize:  44,
				Color:     color.RGBA{R: 0, G: 255, B: 0, A: 255},
			},
			amul: &canvas.Text{
				Text:      "Amul: 0%",
				Alignment: fyne.TextAlignLeading,
				TextSize:  28,
				Color:     color.RGBA{R: 255, G: 255, B: 255, A: 255},
			},
		},
		image: Images{
			checkEngine: canvas.NewImageFromResource(fyne.NewStaticResource("checkengine.png", assets.CheckengineBytes)),
			// fullscreenBtn: widget.NewButtonWithIcon("Fullscreen", theme.ZoomFitIcon(), func() {
			// 	cfg.Mw.SetFullScreen(!cfg.Mw.FullScreen())
			// }),
			knockIcon: icon.New(&icon.Config{
				Image:   canvas.NewImageFromResource(fyne.NewStaticResource("knock.png", assets.KnockBytes)),
				Minsize: fyne.NewSize(90, 90),
			}),
			limpMode: canvas.NewImageFromResource(fyne.NewStaticResource("limp.png", assets.LimpBytes)),
			taz:      canvas.NewImageFromResource(fyne.NewStaticResource("taz.png", assets.Taz)),
		},
	}
	db.ExtendBaseWidget(db)

	db.text.cruise.Hide()
	db.image.checkEngine.Hide()
	db.image.limpMode.Hide()

	db.metricRouter = db.createRouter()

	var isFullscreen bool
	db.fullscreenBtn = widget.NewButtonWithIcon("", theme.ViewFullScreenIcon(), func() {
		if db.cfg.FullscreenFunc != nil {
			isFullscreen = !isFullscreen
			db.cfg.FullscreenFunc(isFullscreen)
			if true {
				db.fullscreenBtn.SetText("Exit Fullscreen")
			} else {
				db.fullscreenBtn.SetText("Fullscreen")
			}
		}
	})

	if db.cfg.Logplayer {
		db.text.time = canvas.NewText("00:00:00.00", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
		db.text.time.TextSize = 35
		db.text.time.Resize(fyne.NewSize(200, 50))
	}

	//db.dbgBar = db.newDebugBar()

	db.image.knockIcon.Hide()
	db.text.cruise.Hide()
	db.image.checkEngine.Hide()
	db.image.limpMode.Hide()
	db.image.taz.Hide()

	db.image.checkEngine.FillMode = canvas.ImageFillContain
	db.image.checkEngine.ScaleMode = canvas.ImageScaleFastest
	db.image.checkEngine.SetMinSize(fyne.NewSize(110, 85))
	db.image.checkEngine.Resize(fyne.NewSize(110, 85))

	db.image.limpMode.FillMode = canvas.ImageFillContain
	db.image.limpMode.ScaleMode = canvas.ImageScaleFastest
	db.image.limpMode.SetMinSize(fyne.NewSize(110, 85))
	db.image.limpMode.Resize(fyne.NewSize(110, 85))

	db.image.taz.FillMode = canvas.ImageFillContain
	db.image.taz.ScaleMode = canvas.ImageScaleFastest
	db.image.taz.SetMinSize(fyne.NewSize(110, 85))
	db.image.taz.Resize(fyne.NewSize(110, 85))

	return db
}

func (db *Dashboard) CreateRenderer() fyne.WidgetRenderer {
	return &DashboardRenderer{db: db}
}

func (db *Dashboard) GetMetricNames() []string {
	names := make([]string, 0, len(db.metricRouter))
	for k := range db.metricRouter {
		names = append(names, k)
	}
	return names
}

func (db *Dashboard) Close() {
}

func (db *Dashboard) SetTime(t time.Time) {
	if db.text.time != nil {
		db.timeBuffer = db.timeBuffer[:0]
		db.timeBuffer = t.AppendFormat(db.timeBuffer, "15:04:05.00")
		db.text.time.Text = string(db.timeBuffer)
		db.text.time.Refresh()
	}
}

func (db *Dashboard) SetValue(key string, value float64) {
	if setFunc, ok := db.metricRouter[key]; ok {
		setFunc(value)
	}
}

func (db *Dashboard) Set(gauge GaugeType, value float64) {
	switch gauge {
	case SpeedDial:
		db.gauges.speed.SetValue(value)
	case RpmDial:
		db.gauges.rpm.SetValue(value)
	case IatDial:
		db.gauges.iat.SetValue(value)
	case EngineTempDial:
		db.gauges.engineTemp.SetValue(value)
	case AirmassDialPrimary:
		db.gauges.airmass.SetValue(value)
	case AirmassDialSecondary:
		db.gauges.airmass.SetValue2(value)
	case PressureDialPrimary:
		db.gauges.pressure.SetValue(value)
	case PressureDialSecondary:
		db.gauges.pressure.SetValue2(value)
	case ThrottleBar:
		db.gauges.throttle.SetValue(value)
	case PWMBar:
		db.gauges.pwm.SetValue(value)
	case WBLambdaBar:
		db.gauges.wblambda.SetValue(value)
	case NBLambdaBar:
		db.gauges.nblambda.SetValue(value)
	default:
		log.Println("Unknown gauge", gauge)
	}
}

func interpol(x0, y0, x1, y1, x float64) float64 {
	return y0 + (x-x0)*(y1-y0)/(x1-x0)
}

func (db *Dashboard) createRouter() map[string]func(float64) {
	var rpm float64
	t5rpmSetter := func(value float64) {
		rpm = value
		db.gauges.rpm.SetValue(value)
	}

	idcSetterT5 := func(obj *canvas.Text, text string) func(float64) {
		idcc := idcSetter(obj, text)
		return func(value float64) {
			idcc((value * rpm) * rpmIDCconstant)
		}
	}

	ioff := ioffSetter(db.text.ioff, db.image.taz)

	activeAirDem := activeAirDemSetter(db.text.activeAirDem, db.cfg.AirDemToString)

	setVehicleSpeed := db.gauges.speed.SetValue
	if db.cfg.UseMPH {
		setVehicleSpeed = func(value float64) {
			db.gauges.speed.SetValue(value * 0.621371)
		}
	}

	t5throttle := func(value float64) {
		// value should be 0-100% input is 0 - 192
		valuePercent := min(192, value) / 192 * 100
		db.gauges.throttle.SetValue(valuePercent)
	}

	t5setnbl := func(value float64) {
		if value < 128 {
			// Interpolate in the range 0 to 128, mapping to -25 to 0.
			db.gauges.nblambda.SetValue(interpol(0, -25, 128, 0, value))
			return
		}
		// Interpolate in the range 128 to 255, mapping to 0 to 25.
		db.gauges.nblambda.SetValue(interpol(128, 0, 255, 25, value))
	}

	router := map[string]func(float64){
		"In.v_Vehicle": setVehicleSpeed, // t7 & t8
		"Bil_hast":     setVehicleSpeed, // t5

		"ActualIn.n_Engine": db.gauges.rpm.SetValue,
		"Rpm":               t5rpmSetter, // t5

		"ActualIn.T_AirInlet": db.gauges.iat.SetValue,
		"Lufttemp":            db.gauges.iat.SetValue, // t5

		"ActualIn.T_Engine": db.gauges.engineTemp.SetValue,
		"Kyl_temp":          db.gauges.engineTemp.SetValue, // t5

		"P_medel":             db.gauges.pressure.SetValue, // t5
		"In.p_AirInlet":       db.gauges.pressure.SetValue,
		"ActualIn.p_AirInlet": db.gauges.pressure.SetValue,

		"Max_tryck":                 db.gauges.pressure.SetValue2, // t5
		"In.p_AirBefThrottle":       db.gauges.pressure.SetValue2,
		"ActualIn.p_AirBefThrottle": db.gauges.pressure.SetValue2,

		"Medeltrot":      t5throttle,                  // t5
		"Out.X_AccPedal": db.gauges.throttle.SetValue, // t7
		"Out.X_AccPos":   db.gauges.throttle.SetValue, // t8

		"Out.PWM_BoostCntrl": db.gauges.pwm.SetValue, // t7 & t8
		"PWM_ut10":           db.gauges.pwm.SetValue, // t5

		//"AdpFuelProt.MulFuelAdapt": amulSetter(db.text.amul, "Amul"), // t7
		"AdpFuelProt.MulFuelAdapt": textSetter(db.text.amul, "Amul", "%", 2), // t7

		// Wideband lambda
		//"AD_EGR": db.gauges.wblambda.SetValue, // t5
		//"DisplProt.LambdaScanner": db.wblambda.SetValue, // t7 & t8
		//"Lambda.External":     db.wblambda.SetValue,
		db.cfg.WidebandSymbol: db.gauges.wblambda.SetValue, // Wideband lambda

		// NB lambda
		"Lambda.LambdaInt": db.gauges.nblambda.SetValue, // t7 & t8
		"Lambdaint":        t5setnbl,                    // t5

		"MAF.m_AirInlet":        db.gauges.airmass.SetValue,  // t7 & t8
		"m_Request":             db.gauges.airmass.SetValue2, // t7
		"AirMassMast.m_Request": db.gauges.airmass.SetValue2, // t8

		"Out.fi_Ignition": textSetter(db.text.ign, "Ign", "", 1),
		"Ign_angle":       textSetter(db.text.ign, "Ign", "", 1),

		"ECMStat.ST_ActiveAirDem": activeAirDem, // t7 & t8

		"IgnProt.fi_Offset":     ioff, // t7
		"IgnMastProt.fi_Offset": ioff, // t8

		"CRUISE": showHider(db.text.cruise),
		"CEL":    showHider(db.image.checkEngine),
		"LIMP":   showHider(db.image.limpMode),

		"Knock_offset1234": knkDetSetter(db.image.knockIcon),
		"KnkDet.KnockCyl":  knkDetSetter(db.image.knockIcon),

		"Myrtilos.InjectorDutyCycle": idcSetter(db.text.idc, "Idc"),   // t7
		"Insptid_ms10":               idcSetterT5(db.text.idc, "Idc"), // t5
	}

	return router
}

/*
	func lambdaToString(v float64) string {
		switch v {
		case 0:
			return "Closed loop activated"
		case 1:
			return "Load to high during a specific time"
		case 2:
			return "Load to low"
		case 3:
			return "Load to high, no knocking"
		case 4:
			return "Load to high, knocking"
		case 5:
			return "Cooling water temp to low, closed throttle"
		case 6:
			return "Cooling water temp to low, open throttle"
		case 7:
			return "Engine speed to low"
		case 8:
			return "Throttle transient in progress"
		case 9:
			return "Throttle transient in progress and low temp"
		case 10:
			return "Fuel cut"
		case 11:
			return "Load to high and exhaust temperature algorithm decides it is time to enrich."
		case 12:
			return "Diagnostic failure that affects the lambda control"
		case 13:
			return "Cloosed loop not enabled"
		case 14:
			return "Waiting number of combustion before hardware check" //, ie U_lambda_probe < 300mV AND U_lambda_probe > 600mV"
		case 15:
			return "Waiting until engine probe is warm"
		case 16:
			return "Waiting until number of combustions have past after probe is warm"
		case 17:
			return "SAI request open loop"
		case 18:
			return "Number of combustion to start closed loop has not passed. Only active when SAI is activated"
		case 19:
			return "Lambda integrator is freezed to 0 by SAI Lean Clamp"
		case 20:
			return "Catalyst diagnose for V6 controls the fuel"
		case 21:
			return "Gas hybrid active, T7 lambdacontrol stopped"
		case 22:
			return "Lambda integrator may not decrease below 0 during start"
		default:
			return "Unknown"
		}
	}
*/

type dims struct {
	sixthWidth    float32
	thirdHeight   float32
	tenthHeight   float32
	halfHeight    float32
	centerX       float32
	centerY       float32
	bottomY       float32
	textSize      float32
	smallTextSize float32
}

func layoutMainDials(db *Dashboard, space fyne.Size, dims *dims) {
	left := db.gauges.pwm.Position().X + db.gauges.pwm.Size().Width
	right := db.gauges.throttle.Position().X
	width := right - left

	centerDialSize := fyne.NewSize(
		width,
		space.Height-125,
	)
	centerDialPos := fyne.NewPos(
		dims.centerX-centerDialSize.Width*0.5,
		dims.centerY-centerDialSize.Height*0.5,
	)

	if !db.cfg.SwapRPMandSpeed {
		db.gauges.rpm.Resize(fyne.NewSize(dims.sixthWidth, dims.thirdHeight))
		db.gauges.rpm.Move(fyne.NewPos(0, 5))
		db.gauges.speed.Resize(centerDialSize)
		db.gauges.speed.Move(centerDialPos)
	} else {
		db.gauges.speed.Resize(fyne.NewSize(dims.sixthWidth, dims.thirdHeight))
		db.gauges.speed.Move(fyne.NewPos(0, 5))
		db.gauges.rpm.Resize(centerDialSize)
		db.gauges.rpm.Move(centerDialPos)
	}
}

func layoutSideDials(db *Dashboard, space fyne.Size, dims *dims) {
	// Calculate dial size
	dialWidth := dims.sixthWidth
	dialHeight := dims.thirdHeight

	// Calculate total height needed for all dials (3 dials on left side including the main dial)
	totalDialHeight := dialHeight * 3 // Three dials on left side (main + 2), two on right

	// Calculate vertical spacing between dials
	spacing := (space.Height - totalDialHeight) / 4 // Divide remaining space into 4 parts for spacing

	// Left side dials positioning
	// Note: Top dial (speed or rpm) is already positioned by layoutMainDials
	// Second dial: airmass
	db.gauges.airmass.Resize(fyne.NewSize(dialWidth, dialHeight))
	db.gauges.airmass.Move(fyne.NewPos(0, spacing*2+dialHeight)) // After top dial + spacing

	// Third dial: pressure
	db.gauges.pressure.Resize(fyne.NewSize(dialWidth, dialHeight))
	db.gauges.pressure.Move(fyne.NewPos(0, spacing*3+dialHeight*2)) // After top dial + middle dial + spacing

	// Right side dials (only 2 dials, different spacing)
	rightX := space.Width - dialWidth

	// Calculate spacing for right side (only 2 dials)
	rightSideSpacing := (space.Height - (dialHeight * 2)) / 3 // Three spaces for two dials

	// First dial: IAT
	db.gauges.iat.Resize(fyne.NewSize(dialWidth, dialHeight))
	db.gauges.iat.Move(fyne.NewPos(rightX, rightSideSpacing))

	// Second dial: engineTemp
	db.gauges.engineTemp.Resize(fyne.NewSize(dialWidth, dialHeight))
	db.gauges.engineTemp.Move(fyne.NewPos(rightX, rightSideSpacing*2+dialHeight))
}

func layoutVerticalBars(db *Dashboard, space fyne.Size, dims *dims) {

	vbarSize := fyne.NewSize(min(dims.sixthWidth*common.OneThird, 70), space.Height-120)

	db.gauges.pwm.Resize(vbarSize)
	db.gauges.pwm.Move(fyne.NewPos(dims.sixthWidth+8, 25))

	db.gauges.throttle.Resize(vbarSize)
	db.gauges.throttle.Move(fyne.NewPos(space.Width-dims.sixthWidth-vbarSize.Width-8, 25))
}

func layoutHorizontalBars(db *Dashboard, space fyne.Size, dims *dims) {
	cbarHeight := min(dims.tenthHeight, 50)
	cbarSize := fyne.NewSize((dims.sixthWidth * 3), cbarHeight)
	cbarX := dims.sixthWidth * 1.5

	db.gauges.nblambda.Resize(cbarSize)
	db.gauges.nblambda.Move(fyne.NewPos(cbarX, 0))

	db.gauges.wblambda.Resize(cbarSize)
	db.gauges.wblambda.Move(fyne.NewPos(cbarX, space.Height-cbarHeight))
}

func layoutIcons(db *Dashboard, space fyne.Size, dims *dims) {
	// Limp mode icon
	db.image.limpMode.Resize(fyne.NewSize(dims.sixthWidth, dims.thirdHeight))
	db.image.limpMode.Move(fyne.NewPos(
		dims.centerX-db.image.limpMode.Size().Width*0.5,
		dims.centerY-db.image.limpMode.Size().Height*0.5-(dims.thirdHeight*0.5),
	))

	// Check engine icon
	db.image.checkEngine.Resize(fyne.NewSize(dims.sixthWidth*0.5, dims.thirdHeight*0.5))
	db.image.checkEngine.Move(fyne.NewPos(
		space.Width-db.gauges.engineTemp.Size().Width-db.gauges.throttle.Size().Width-db.image.checkEngine.Size().Width-15,
		space.Height-db.image.checkEngine.Size().Height-db.gauges.wblambda.Size().Height,
	))

	// Knock icon
	db.image.knockIcon.Move(fyne.NewPos(
		db.gauges.pwm.Position().X+db.gauges.pwm.Size().Width,
		dims.centerY-60,
	))

	// Taz icon

	asd := fyne.Min(dims.sixthWidth, dims.thirdHeight)

	db.image.taz.Resize(fyne.NewSize(asd, asd+16))
	db.image.taz.Move(fyne.NewPos(
		dims.centerX-db.image.taz.Size().Width*0.58,
		dims.centerY-db.image.taz.Size().Height,
	))
}

func layoutButtons(db *Dashboard, space fyne.Size, dims *dims) {
	//move dr.fullscreenBtn to bottom right

	db.fullscreenBtn.Resize(fyne.NewSize(dims.sixthWidth, dims.tenthHeight))
	db.fullscreenBtn.Move(fyne.NewPos(space.Width-dims.sixthWidth, space.Height-dims.tenthHeight))
}

func layoutTexts(db *Dashboard, space fyne.Size, dims *dims) {
	// Calculate responsive text sizes based on window dimensions
	// Use the smaller of width/height to ensure text stays proportional
	baseSize := min(space.Width, space.Height)

	// Large text (like IGN, IDC) - ~4.5% of smallest window dimension
	dims.textSize = baseSize * 0.045
	// Ensure text size stays within reasonable bounds
	dims.textSize = min(max(dims.textSize, 24), 44)

	// Small text (like IOFF, AMUL) - ~60% of large text size
	dims.smallTextSize = dims.textSize * 0.6
	// Ensure small text size stays within reasonable bounds
	dims.smallTextSize = min(max(dims.smallTextSize, 18), 28)

	// AMUL text (small)
	db.text.amul.TextSize = dims.smallTextSize
	db.text.amul.Move(fyne.NewPos(
		db.gauges.wblambda.Position().X,
		space.Height-db.gauges.wblambda.Size().Height-db.text.amul.MinSize().Height,
	))

	// IGN text (large)
	db.text.ign.TextSize = dims.textSize
	db.text.ign.Move(fyne.NewPos(
		db.gauges.nblambda.Position().X,
		db.gauges.nblambda.Size().Height,
	))

	// IOFF text (small)
	db.text.ioff.TextSize = dims.smallTextSize
	db.text.ioff.Move(fyne.NewPos(
		db.gauges.nblambda.Position().X,
		db.text.ign.Position().Y+db.text.ign.MinSize().Height,
	))

	// IDC text (large)
	db.text.idc.TextSize = dims.textSize
	db.text.idc.Move(fyne.NewPos(
		db.gauges.nblambda.Position().X+db.gauges.nblambda.Size().Width-db.text.idc.MinSize().Width,
		db.gauges.nblambda.Size().Height,
	))

	// Active air demand text (large)
	db.text.activeAirDem.TextSize = dims.textSize
	db.text.activeAirDem.Move(fyne.NewPos(
		dims.centerX,
		dims.thirdHeight,
	))

	// Cruise text (special size - larger)
	cruiseSize := dims.textSize * 1.1
	db.text.cruise.TextSize = min(max(cruiseSize, 35), 45)
	db.text.cruise.Move(fyne.NewPos(
		dims.sixthWidth*1.45,
		space.Height-(db.image.checkEngine.Size().Height*0.6)-db.gauges.wblambda.Size().Height,
	))

	// Time text for logplayer (small)
	if db.logplayer {
		db.text.time.TextSize = dims.smallTextSize
		db.text.time.Move(
			fyne.NewPos(
				dims.centerX-(db.text.time.MinSize().Width*0.5),
				space.Height*common.OneHalfSix,
			),
		)
	}

	//if db.logplayer {
	//	db.text.time.Move(fyne.NewPos(dims.centerX-100, space.Height*common.OneHalfSix))
	//}
}

type DashboardRenderer struct {
	db   *Dashboard
	size fyne.Size
}

func (dr *DashboardRenderer) Layout(space fyne.Size) {
	if dr.size == space {
		return
	}
	dr.size = space

	// Calculate common dimensions
	dims := &dims{
		sixthWidth:  space.Width * common.OneSixth,
		thirdHeight: (space.Height - 50) * .33,
		tenthHeight: (space.Height - 50) * .1,
		halfHeight:  (space.Height - 50) * .5,
		centerX:     space.Width * 0.5,
		centerY:     space.Height * 0.5,
		bottomY:     space.Height - 55,

		//textSize: max(min(space.Height, space.Width)*0.07, 20),
	}
	// Layout horizontal bars
	layoutHorizontalBars(dr.db, space, dims)

	// Layout side dials
	layoutSideDials(dr.db, space, dims)

	// Layout vertical bars
	layoutVerticalBars(dr.db, space, dims)

	// Layout main dials
	layoutMainDials(dr.db, space, dims)

	// Layout buttons
	layoutButtons(dr.db, space, dims)

	dims.textSize = dr.db.gauges.nblambda.Size().Height
	dims.smallTextSize = dims.textSize * 0.5

	// Layout text elements
	layoutTexts(dr.db, space, dims)

	// Layout icons
	layoutIcons(dr.db, space, dims)
}

func (dr *DashboardRenderer) MinSize() fyne.Size {
	return fyne.Size{Width: 480, Height: 300}
}

func (dr *DashboardRenderer) Refresh() {
}

func (dr *DashboardRenderer) Destroy() {
}

func (dr *DashboardRenderer) Objects() []fyne.CanvasObject {
	cont := []fyne.CanvasObject{
		dr.db.image.limpMode,
		dr.db.image.taz,
		dr.db.gauges.rpm,
		dr.db.gauges.speed,
		dr.db.gauges.airmass,
		dr.db.gauges.pressure,
		dr.db.gauges.iat,
		dr.db.gauges.engineTemp,
		dr.db.text.ign,
		dr.db.text.ioff,
		dr.db.text.idc,
		dr.db.text.amul,
		dr.db.text.activeAirDem,
		dr.db.gauges.nblambda,
		dr.db.gauges.wblambda,
		dr.db.gauges.throttle,
		dr.db.gauges.pwm,
		dr.db.image.checkEngine,
		dr.db.text.cruise,
		dr.db.image.knockIcon,
		dr.db.fullscreenBtn,
	}

	if dr.db.logplayer {
		cont = append(cont, dr.db.text.time)
	}

	return cont
}
