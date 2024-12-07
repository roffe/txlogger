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
)

const rpmIDCconstant = 1.0 / 1200.0

type Dashboard struct {
	cfg *Config

	metricRouter map[string]func(float64)

	text   Texts
	gauges Gauges

	checkEngine *canvas.Image
	limpMode    *canvas.Image
	knockIcon   *widgets.Icon

	fullscreenBtn *widget.Button
	closeBtn      *widget.Button
	logBtn        *widget.Button

	//dbgBar *fyne.Container

	logplayer bool

	timeBuffer []byte

	widget.BaseWidget
}

type Texts struct {
	ioff, activeAirDem, ign, cruise *canvas.Text
	idc, amul                       *canvas.Text
	time                            *canvas.Text
}

type Gauges struct {
	rpm, speed, iat    *widgets.Dial
	throttle, pwm      *widgets.VBar
	engineTemp         *widgets.Dial
	nblambda, wblambda *widgets.CBar
	pressure, airmass  *widgets.DualDial
}

type Config struct {
	App             fyne.App
	Mw              fyne.Window
	Logplayer       bool
	LogBtn          *widget.Button
	OnClose         func()
	AirDemToString  func(float64) string
	FCutToString    func(float64) string
	UseMPH          bool
	SwapRPMandSpeed bool
	HighAFR         float64
	LowAFR          float64
	WidebandSymbol  string
	MetricRouter    map[string]func(float64)
}

// func NewDashboard(a fyne.App, mw fyne.Window, logplayer bool, logBtn *widget.Button, onClose func()) *Dashboard {
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
		logBtn:    cfg.LogBtn,
		logplayer: cfg.Logplayer,
		gauges: Gauges{
			airmass: widgets.NewDualDial(widgets.DualDialConfig{
				Title:   "mg/c",
				Min:     0,
				Max:     2200,
				Steps:   22,
				MinSize: fyne.NewSize(100, 100),
			}),
			speed: widgets.NewDial(widgets.DialConfig{
				Title:         speedometerText,
				Min:           0,
				Max:           300,
				Steps:         30,
				DisplayString: "%.1f",
				MinSize:       fyne.NewSize(100, 100),
			}),
			rpm: widgets.NewDial(widgets.DialConfig{
				Title:   "RPM",
				Min:     0,
				Max:     8000,
				Steps:   20,
				MinSize: fyne.NewSize(100, 100),
			}),
			iat: widgets.NewDial(widgets.DialConfig{
				Title:   "IAT",
				Min:     0,
				Max:     80,
				Steps:   16,
				MinSize: fyne.NewSize(100, 100),
			}),
			pressure: widgets.NewDualDial(widgets.DualDialConfig{
				Title:         "MAP",
				Min:           0,
				Max:           3,
				Steps:         30,
				DisplayString: "%.2f",
				MinSize:       fyne.NewSize(100, 100),
			}),
			throttle: widgets.NewVBar(&widgets.VBarConfig{
				Title:      "TPS",
				Min:        0,
				Max:        100,
				Steps:      20,
				Minsize:    fyne.NewSize(50, 50),
				ColorScale: widgets.TraditionalScale,
			}),
			pwm: widgets.NewVBar(&widgets.VBarConfig{
				Title:      "PWM",
				Min:        0,
				Max:        100,
				Steps:      20,
				Minsize:    fyne.NewSize(50, 50),
				ColorScale: widgets.TraditionalScale,
			}),
			engineTemp: widgets.NewDial(widgets.DialConfig{
				Title: "tEng",
				Min:   -20,
				Max:   130,
				Steps: 16,
			}),
			wblambda: widgets.NewCBar(&widgets.CBarConfig{
				Title:           "",
				Min:             0.50,
				Center:          1,
				Max:             1.50,
				Steps:           20,
				Minsize:         fyne.NewSize(50, 35),
				DisplayString:   "λ %.3f",
				DisplayTextSize: 20,
				TextPosition:    widgets.TextAtTop,
			}),
			nblambda: widgets.NewCBar(&widgets.CBarConfig{
				Title:           "",
				Min:             -25,
				Center:          0,
				Max:             25,
				Steps:           40,
				Minsize:         fyne.NewSize(50, 35),
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
		checkEngine: canvas.NewImageFromResource(fyne.NewStaticResource("checkengine.png", assets.CheckengineBytes)),
		fullscreenBtn: widget.NewButtonWithIcon("Fullscreen", theme.ZoomFitIcon(), func() {
			cfg.Mw.SetFullScreen(!cfg.Mw.FullScreen())
		}),
		knockIcon: widgets.NewIcon(&widgets.IconConfig{
			Image:   canvas.NewImageFromResource(fyne.NewStaticResource("knock.png", assets.KnockBytes)),
			Minsize: fyne.NewSize(90, 90),
		}),
		limpMode: canvas.NewImageFromResource(fyne.NewStaticResource("limp.png", assets.LimpBytes)),
	}
	db.ExtendBaseWidget(db)

	db.metricRouter = db.createRouter()

	db.closeBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		if db.cfg.OnClose != nil {
			db.cfg.OnClose()
		}
	})

	if cfg.Logplayer {
		db.text.time = canvas.NewText("00:00:00.00", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
		db.text.time.TextSize = 35
		db.text.time.Resize(fyne.NewSize(200, 50))
	}

	//db.dbgBar = db.newDebugBar()

	db.knockIcon.Hide()
	db.text.cruise.Hide()
	db.checkEngine.Hide()
	db.limpMode.Hide()

	db.checkEngine.FillMode = canvas.ImageFillContain
	db.checkEngine.SetMinSize(fyne.NewSize(110, 85))
	db.checkEngine.Resize(fyne.NewSize(110, 85))

	db.limpMode.FillMode = canvas.ImageFillContain
	db.limpMode.SetMinSize(fyne.NewSize(110, 85))
	db.limpMode.Resize(fyne.NewSize(110, 85))

	return db
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

	ioff := ioffSetter(db.text.ioff)

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
		"CEL":    showHider(db.checkEngine),
		"LIMP":   showHider(db.limpMode),

		"Knock_offset1234": knkDetSetter(db.knockIcon),
		"KnkDet.KnockCyl":  knkDetSetter(db.knockIcon),

		"Myrtilos.InjectorDutyCycle": idcSetter(db.text.idc, "Idc"),   // t7
		"Insptid_ms10":               idcSetterT5(db.text.idc, "Idc"), // t5
	}

	return router
}

func (db *Dashboard) Sweep() {
	db.checkEngine.Hide()
	an := fyne.NewAnimation(900*time.Millisecond, func(p float32) {
		pa := float64(p)
		db.gauges.speed.SetValue(300 * pa)
		db.gauges.rpm.SetValue(8000 * pa)
		db.gauges.iat.SetValue(80 * pa)
		db.gauges.airmass.SetValue(2100 * pa)
		db.gauges.airmass.SetValue2(2200 * pa)
		db.gauges.engineTemp.SetValue(160 * pa)
		db.gauges.pressure.SetValue(3 * pa)
		db.gauges.throttle.SetValue(100 * pa)
		db.gauges.pwm.SetValue(100 * pa)
		db.gauges.nblambda.SetValue(25 * pa)
		db.gauges.wblambda.SetValue(1.52 * pa)
		//db.metricsChan <- &model.DashboardMetric{Name: "Out.fi_Ignition", Value: 30.0 * pa}
		//db.metricsChan <- &model.DashboardMetric{Name: "IgnProt.fi_Offset", Value: 15.0 * pa}

	})
	an.AutoReverse = true
	an.Curve = fyne.AnimationEaseInOut
	an.Start()
	time.Sleep(1800 * time.Millisecond)
	db.checkEngine.Show()
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
func (db *Dashboard) CreateRenderer() fyne.WidgetRenderer {
	return &DashboardRenderer{
		db: db,
	}
}

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
	centerDialSize := fyne.NewSize(
		space.Width,
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
	// Left side dials
	db.gauges.pressure.Resize(fyne.NewSize(dims.sixthWidth, dims.thirdHeight))
	db.gauges.pressure.Move(fyne.NewPos(0, (dims.thirdHeight*2)+5))

	db.gauges.airmass.Resize(fyne.NewSize(dims.sixthWidth, dims.thirdHeight))
	db.gauges.airmass.Move(fyne.NewPos(0, dims.thirdHeight+5))

	// Right side dials
	rightDialSize := fyne.NewSize(dims.sixthWidth, dims.halfHeight)
	rightX := space.Width - dims.sixthWidth

	db.gauges.iat.Resize(rightDialSize)
	db.gauges.iat.Move(fyne.NewPos(rightX, 0))

	db.gauges.engineTemp.Resize(rightDialSize)
	db.gauges.engineTemp.Move(fyne.NewPos(rightX, dims.halfHeight))
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
	db.limpMode.Resize(fyne.NewSize(dims.sixthWidth, dims.thirdHeight))
	db.limpMode.Move(fyne.NewPos(
		dims.centerX-db.limpMode.Size().Width*0.5,
		dims.centerY-db.limpMode.Size().Height*0.5-(dims.thirdHeight*0.5),
	))

	// Check engine icon
	db.checkEngine.Resize(fyne.NewSize(dims.sixthWidth*0.5, dims.thirdHeight*0.5))
	db.checkEngine.Move(fyne.NewPos(
		space.Width-db.gauges.engineTemp.Size().Width-db.gauges.throttle.Size().Width-db.checkEngine.Size().Width-15,
		space.Height-db.checkEngine.Size().Height-db.gauges.wblambda.Size().Height,
	))

	// Knock icon
	db.knockIcon.Move(fyne.NewPos(
		dims.centerX-(db.checkEngine.Size().Width*0.5)-(dims.sixthWidth*0.7),
		dims.centerY-60,
	))
}

func layoutButtons(db *Dashboard, space fyne.Size, dims *dims) {
	// Close button
	db.closeBtn.Resize(fyne.NewSize(dims.sixthWidth, 55))
	db.closeBtn.Move(fyne.NewPos(space.Width-dims.sixthWidth, dims.bottomY))

	if !db.logplayer {
		// Fullscreen button sizing based on screen width
		if space.Width < 1000 {
			db.fullscreenBtn.SetText("(F)")
			db.fullscreenBtn.Resize(fyne.NewSize(dims.sixthWidth*common.OneHalfOne, 55))
		} else if space.Width < 1300 {
			db.fullscreenBtn.SetText("Fullscrn")
			db.fullscreenBtn.Resize(fyne.NewSize(dims.sixthWidth*common.OneOneEight, 55))
		} else {
			db.fullscreenBtn.SetText("Fullscreen")
			db.fullscreenBtn.Resize(fyne.NewSize(dims.sixthWidth*common.OneOneFive, 55))
		}

		// Log button
		db.logBtn.Resize(fyne.NewSize(db.gauges.wblambda.Position().X-db.fullscreenBtn.Size().Width-14, 55))
		db.logBtn.Move(fyne.NewPos(db.fullscreenBtn.Size().Width+5, dims.bottomY))
	} else {
		db.text.time.Move(fyne.NewPos(dims.centerX-100, space.Height*common.OneHalfSix))
	}
	db.fullscreenBtn.Move(fyne.NewPos(0, dims.bottomY))
}

func layoutTexts(db *Dashboard, space fyne.Size, dims *dims) {
	// AMUL text
	db.text.amul.TextSize = dims.smallTextSize
	db.text.amul.Move(fyne.NewPos(
		db.gauges.wblambda.Position().X,
		space.Height-db.gauges.wblambda.Size().Height-db.text.amul.MinSize().Height,
	))

	// IGN text
	db.text.ign.TextSize = dims.textSize
	db.text.ign.Move(fyne.NewPos(
		db.gauges.nblambda.Position().X,
		db.gauges.nblambda.Size().Height,
	))

	// IOFF text
	db.text.ioff.TextSize = dims.smallTextSize
	db.text.ioff.Move(fyne.NewPos(
		db.gauges.nblambda.Position().X,
		db.text.ign.Position().Y+db.text.ign.MinSize().Height,
	))

	// IDC text
	db.text.idc.TextSize = dims.textSize
	db.text.idc.Move(fyne.NewPos(
		db.gauges.nblambda.Position().X+db.gauges.nblambda.Size().Width-db.text.idc.MinSize().Width,
		db.gauges.nblambda.Size().Height,
	))

	// Active air demand text
	db.text.activeAirDem.TextSize = dims.textSize
	db.text.activeAirDem.Move(fyne.NewPos(dims.centerX, dims.thirdHeight))

	// Cruise text
	db.text.cruise.Move(fyne.NewPos(
		dims.sixthWidth*1.45,
		space.Height-(db.checkEngine.Size().Height*0.6)-db.gauges.wblambda.Size().Height,
	))

	if db.logplayer {
		db.text.time.TextSize = dims.smallTextSize
		db.text.time.Move(
			fyne.NewPos(
				dims.centerX-(db.text.time.MinSize().Width*0.5),
				space.Height*common.OneHalfSix,
			),
		)
	}
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

	// Layout main dials
	layoutMainDials(dr.db, space, dims)

	// Layout side dials
	layoutSideDials(dr.db, space, dims)

	// Layout vertical bars
	layoutVerticalBars(dr.db, space, dims)

	// Layout icons
	layoutIcons(dr.db, space, dims)

	// Layout buttons
	layoutButtons(dr.db, space, dims)

	dims.textSize = dr.db.gauges.nblambda.Size().Height
	dims.smallTextSize = dims.textSize * 0.5

	// Layout text elements
	layoutTexts(dr.db, space, dims)
}

func (dr *DashboardRenderer) MinSize() fyne.Size {
	return fyne.NewSize(400, 250)
}

func (dr *DashboardRenderer) Refresh() {
}

func (dr *DashboardRenderer) Destroy() {
}

func (dr *DashboardRenderer) Objects() []fyne.CanvasObject {
	cont := []fyne.CanvasObject{
		dr.db.limpMode,
		//db.dbgBar,

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
		dr.db.checkEngine,
		dr.db.text.cruise,
		dr.db.knockIcon,
	}

	if !dr.db.logplayer {
		cont = append(cont, dr.db.fullscreenBtn)
		cont = append(cont, dr.db.closeBtn)
		cont = append(cont, dr.db.logBtn)
	} else {
		cont = append(cont, dr.db.text.time)
	}
	return cont
}
