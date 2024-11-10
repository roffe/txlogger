package dashboard

import (
	_ "embed"
	"image/color"
	"log"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/common"
	"github.com/roffe/txlogger/pkg/widgets"
)

const rpmIDCconstant = 1.0 / 1200.0

type Dashboard struct {
	cfg          *DashboardConfig
	metricRouter map[string]func(float64)
	container    fyne.CanvasObject

	text   Texts
	gauges DasboardGauges

	checkEngine *canvas.Image
	limpMode    *canvas.Image
	knockIcon   *widgets.Icon

	fullscreenBtn *widget.Button
	closeBtn      *widget.Button
	logBtn        *widget.Button

	//dbgBar *fyne.Container

	logplayer bool
	focused   bool

	timeBuffer []byte

	widget.BaseWidget
}

type Texts struct {
	ioff, activeAirDem, ign, cruise *canvas.Text
	idc, amul                       *canvas.Text
	time                            *canvas.Text
}

type DasboardGauges struct {
	rpm, speed, iat    *widgets.Dial
	throttle, pwm      *widgets.VBar
	engineTemp         *widgets.Dial
	nblambda, wblambda *widgets.CBar
	pressure, airmass  *widgets.DualDial
}

type DashboardConfig struct {
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
func NewDashboard(cfg *DashboardConfig) *Dashboard {
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
		gauges: DasboardGauges{
			airmass: widgets.NewDualDial(widgets.DualDialConfig{
				Title: "mg/c",
				Min:   0,
				Max:   2200,
				Steps: 22,
			}),
			speed: widgets.NewDial(widgets.DialConfig{
				Title:         speedometerText,
				Min:           0,
				Max:           300,
				Steps:         30,
				DisplayString: "%.1f",
			}),
			rpm: widgets.NewDial(widgets.DialConfig{
				Title: "RPM",
				Min:   0,
				Max:   8000,
				Steps: 20,
			}),
			iat: widgets.NewDial(widgets.DialConfig{
				Title: "IAT",
				Min:   0,
				Max:   80,
				Steps: 16,
			}),
			pressure: widgets.NewDualDial(widgets.DualDialConfig{
				Title:         "MAP",
				Min:           0,
				Max:           3,
				Steps:         30,
				DisplayString: "%.2f",
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
				Title:         "",
				Min:           0.50,
				Center:        1,
				Max:           1.50,
				Steps:         20,
				Minsize:       fyne.NewSize(50, 35),
				DisplayString: "λ %.3f",
			}),
			nblambda: widgets.NewCBar(&widgets.CBarConfig{
				Title:         "",
				Min:           -25,
				Center:        0,
				Max:           25,
				Steps:         40,
				Minsize:       fyne.NewSize(50, 35),
				DisplayString: "%.2f%%",
				//DisplayTextSize: 40,
				TextPosition: widgets.TextAtBottom,
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
	db.container = db.render()

	return db
}

func (db *Dashboard) render() *fyne.Container {
	content := container.NewWithoutLayout(
		db.limpMode,
		//db.dbgBar,

		db.gauges.rpm,
		db.gauges.speed,
		db.gauges.airmass,
		db.gauges.pressure,
		db.gauges.iat,
		db.gauges.engineTemp,

		db.text.ign,
		db.text.ioff,
		db.text.idc,
		db.text.amul,

		db.text.activeAirDem,

		db.gauges.nblambda,
		db.gauges.wblambda,
		db.gauges.throttle,
		db.gauges.pwm,
		db.checkEngine,
		db.text.cruise,
		db.knockIcon,
	)

	if !db.logplayer {
		content.Add(db.fullscreenBtn)
		content.Add(db.closeBtn)
		content.Add(db.logBtn)
	} else {
		content.Add(db.text.time)
	}

	return content
}

func (db *Dashboard) GetMetricNames() []string {
	names := make([]string, 0, len(db.metricRouter))
	for k := range db.metricRouter {
		names = append(names, k)
	}
	return names
}

func (db *Dashboard) FocusGained() {
	db.focused = true
}
func (db *Dashboard) FocusLost() {
	db.focused = false
}
func (db *Dashboard) Focused() bool {
	return db.focused
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

// func (db *Dashboard) SetTimeText(text string) {
// 	if db.text.time != nil {
// 		db.text.time.Text = text
// 		db.text.time.Refresh()
// 	}
// }

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

	//activeAirDem := func(value float64) {
	//	db.text.activeAirDem.Text = db.cfg.AirDemToString(value) + " (" + strconv.FormatFloat(value, 'f', 0, 64) + ")"
	//	db.text.activeAirDem.Refresh()
	//}

	showHider := func(obj fyne.CanvasObject) func(float64) {
		var oldValue float64
		return func(value float64) {
			if value == oldValue {
				return
			}
			if value == 1 {
				obj.Show()
			} else {
				obj.Hide()
			}
		}
	}

	knkDet := func(value float64) {
		if value > 0 {
			kn := int(value)
			knockValue := 0
			if kn&1<<24 == 1<<24 {
				knockValue += 1000
			}
			if kn&1<<16 == 1<<16 {
				knockValue += 200
			}
			if kn&1<<8 == 1<<8 {
				knockValue += 30
			}
			if kn&1 == 1 {
				knockValue += 4
			}
			db.knockIcon.SetText(strconv.Itoa(knockValue))
			db.knockIcon.Show()
		} else {
			db.knockIcon.Hide()
		}
	}

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
		//"DisplProt.LambdaScanner": db.wblambda.SetValue, // t7 & t8
		//"Lambda.External":     db.wblambda.SetValue,
		db.cfg.WidebandSymbol: db.gauges.wblambda.SetValue, // Wideband lambda

		"AD_EGR": db.gauges.wblambda.SetValue, // t5

		// NB lambda
		"Lambda.LambdaInt": db.gauges.nblambda.SetValue, // t7 & t8
		"Lambdaint":        t5setnbl,                    // t5

		"MAF.m_AirInlet":        db.gauges.airmass.SetValue,  // t7 & t8
		"m_Request":             db.gauges.airmass.SetValue2, // t7
		"AirMassMast.m_Request": db.gauges.airmass.SetValue2, // t8

		"Out.fi_Ignition": textSetter(db.text.ign, "Ign", "°", 1),
		"Ign_angle":       textSetter(db.text.ign, "Ign", "°", 1),

		"ECMStat.ST_ActiveAirDem": activeAirDem, // t7 & t8

		"IgnProt.fi_Offset":     ioff, // t7
		"IgnMastProt.fi_Offset": ioff, // t8

		"CRUISE": showHider(db.text.cruise),
		"CEL":    showHider(db.checkEngine),
		"LIMP":   showHider(db.limpMode),

		"Knock_offset1234": knkDet,
		"KnkDet.KnockCyl":  knkDet,

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

func (db *Dashboard) setValue(value float64) {
	db.gauges.speed.SetValue(value)
	db.gauges.rpm.SetValue(value)
	db.gauges.iat.SetValue(value)
	db.gauges.engineTemp.SetValue(value)
	db.gauges.pressure.SetValue(value)
	db.gauges.throttle.SetValue(value)
	db.gauges.pwm.SetValue(value)
	db.gauges.nblambda.SetValue(value)
	db.gauges.wblambda.SetValue(value)
	db.gauges.airmass.SetValue(value)
	db.gauges.airmass.SetValue2(value)
}

func (db *Dashboard) NewDebugBar() *fyne.Container {
	var mockValue float64 = 0
	return container.NewGridWithColumns(13,
		widget.NewButton("-100", func() {
			mockValue -= 100
			db.setValue(mockValue)

		}),
		widget.NewButton("-10", func() {
			mockValue -= 10
			db.setValue(mockValue)

		}),
		widget.NewButton("-1", func() {
			mockValue -= 1
			db.setValue(mockValue)

		}),
		widget.NewButton("-0.1", func() {
			mockValue -= 0.1
			db.setValue(mockValue)

		}),
		widget.NewButton("-0.01", func() {
			mockValue -= 0.01
			db.setValue(mockValue)

		}),
		widget.NewButton("CEL", func() {
			if !db.checkEngine.Visible() {
				db.checkEngine.Show()
			} else {
				db.checkEngine.Hide()
			}
		}),
		widget.NewButton("Sweep", func() {
			db.Sweep()
		}),
		widget.NewButton("Mock", func() {
			db.gauges.speed.SetValue(110)
			db.gauges.rpm.SetValue(3320)
			db.gauges.iat.SetValue(30)
			db.gauges.engineTemp.SetValue(85)
			db.gauges.pressure.SetValue(1.2)
			db.gauges.throttle.SetValue(85)
			db.gauges.pwm.SetValue(47)
			db.gauges.nblambda.SetValue(2.13)
			db.gauges.wblambda.SetValue(1.03)
			db.gauges.airmass.SetValue(1003)
			db.gauges.airmass.SetValue2(1200)
		}),
		widget.NewButton("+0.01", func() {
			mockValue += 0.01
			db.setValue(mockValue)
		}),
		widget.NewButton("+0.1", func() {
			mockValue += 0.1
			db.setValue(mockValue)
		}),
		widget.NewButton("+1", func() {
			mockValue += 1
			db.setValue(mockValue)
		}),
		widget.NewButton("+10", func() {
			mockValue += 10
			db.setValue(mockValue)
		}),
		widget.NewButton("+100", func() {
			mockValue += 100
			db.setValue(mockValue)
		}),
	)
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

type DashboardRenderer struct {
	db   *Dashboard
	size fyne.Size
}

func (dr *DashboardRenderer) Layout(space fyne.Size) {
	if dr.size.Width == space.Width && dr.size.Height == space.Height {
		return
	}
	dr.size = space
	// log.Println("dashboard.Layout", space.Width, space.Height)
	dr.db.container.Resize(space)

	db := dr.db

	var sixthWidth float32 = space.Width * common.OneSixth
	var thirdHeight float32 = (space.Height - 50) * .33
	var halfHeight float32 = (space.Height - 50) * .5

	// Dials
	if !db.cfg.SwapRPMandSpeed {
		// Top left
		db.gauges.rpm.Resize(fyne.NewSize(sixthWidth, thirdHeight))
		db.gauges.rpm.Move(fyne.NewPos(0, 5))

		// Center dial
		db.gauges.speed.Resize(fyne.NewSize(space.Width-sixthWidth*2-(sixthWidth*common.OneThird*2)-20, space.Height-115))
		db.gauges.speed.Move(fyne.NewPos(space.Width*.5-db.gauges.speed.Size().Width*.5, space.Height*.5-db.gauges.speed.Size().Height*.5+25))
	} else {
		db.gauges.speed.Resize(fyne.NewSize(sixthWidth, thirdHeight))
		db.gauges.speed.Move(fyne.NewPos(0, 5))
		// Center dial
		db.gauges.rpm.Resize(fyne.NewSize(space.Width-sixthWidth*2-(sixthWidth*common.OneThird*2)-20, space.Height-115))
		db.gauges.rpm.Move(fyne.NewPos(space.Width*.5-db.gauges.rpm.Size().Width*.5, space.Height*.5-db.gauges.rpm.Size().Height*.5+25))
	}

	db.gauges.pressure.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.gauges.pressure.Move(fyne.NewPos(0, (thirdHeight*2)+5))

	db.gauges.airmass.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.gauges.airmass.Move(fyne.NewPos(0, thirdHeight+5))

	db.gauges.iat.Resize(fyne.NewSize(sixthWidth, halfHeight))
	db.gauges.iat.Move(fyne.NewPos(space.Width-db.gauges.iat.Size().Width, 0))

	db.gauges.engineTemp.Resize(fyne.NewSize(sixthWidth, halfHeight))
	db.gauges.engineTemp.Move(fyne.NewPos(space.Width-db.gauges.engineTemp.Size().Width, halfHeight))

	// Vbar
	pwm := db.gauges.pwm
	pwm.Resize(fyne.NewSize(sixthWidth*common.OneThird, space.Height-125))
	pwm.Move(fyne.NewPos(sixthWidth+8, 25))

	tps := db.gauges.throttle
	tps.Resize(fyne.NewSize(sixthWidth*common.OneThird, space.Height-125))
	tps.Move(fyne.NewPos(space.Width-sixthWidth-tps.Size().Width-8, 25))

	// Cbar
	db.gauges.nblambda.Resize(fyne.NewSize((sixthWidth * 3), 65))
	db.gauges.nblambda.Move(fyne.NewPos(sixthWidth*1.5, 0))

	db.gauges.wblambda.Resize(fyne.NewSize((sixthWidth * 3), 65))
	db.gauges.wblambda.Move(fyne.NewPos(sixthWidth*1.5, space.Height-65))

	//db.amul.Resize(fyne.NewSize(sixthWidth, space.Height-thirdHeight))
	db.text.amul.Move(fyne.NewPos(sixthWidth*1.5, space.Height-(db.gauges.wblambda.Size().Height*1.6)))

	// Icons
	db.limpMode.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.limpMode.Move(fyne.NewPos(space.Width*.5-db.limpMode.Size().Width*.5, space.Height*.5-db.limpMode.Size().Height*.5-(thirdHeight*.5)))

	db.checkEngine.Resize(fyne.NewSize(sixthWidth*.5, thirdHeight*.5))
	db.checkEngine.Move(fyne.NewPos(space.Width-db.gauges.engineTemp.Size().Width-db.gauges.throttle.Size().Width-db.checkEngine.Size().Width-15, space.Height-db.checkEngine.Size().Height-db.gauges.wblambda.Size().Height))

	db.knockIcon.Move(fyne.NewPos((space.Width*.5)-(db.checkEngine.Size().Width*.5)-(sixthWidth*.7), space.Height*.5-60))

	// Buttons

	db.closeBtn.Resize(fyne.NewSize(sixthWidth, 55))
	db.closeBtn.Move(fyne.NewPos(space.Width-sixthWidth, space.Height-55))

	if !db.logplayer {
		if space.Width < 1000 {
			db.fullscreenBtn.SetText("(F)")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth*common.OneHalfOne, 55))
		} else if space.Width < 1300 {
			db.fullscreenBtn.SetText("Fullscrn")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth*common.OneOneEight, 55))
		} else {
			db.fullscreenBtn.SetText("Fullscreen")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth*common.OneOneFive, 55))
		}

		db.logBtn.Resize(fyne.NewSize(db.gauges.wblambda.Position().X-db.fullscreenBtn.Size().Width-14, 55))
		db.logBtn.Move(fyne.NewPos(db.fullscreenBtn.Size().Width+5, space.Height-55))
	} else {
		db.text.time.Move(fyne.NewPos(space.Width*.5-100, space.Height*common.OneHalfSix))
	}
	db.fullscreenBtn.Move(fyne.NewPos(0, space.Height-55))

	// Text
	//textSize := min(space.Width*oneTwentyFifth, 45)

	//db.ign.TextSize = textSize
	db.text.ign.Move(fyne.NewPos(db.gauges.nblambda.Position().X, db.gauges.nblambda.Size().Height-14))

	//db.ioff.TextSize = textSize
	db.text.ioff.Move(fyne.NewPos(db.gauges.nblambda.Position().X, db.text.ign.Position().Y+54))

	//db.idc.TextSize = textSize
	db.text.idc.Move(fyne.NewPos((db.gauges.nblambda.Position().X+db.gauges.nblambda.Size().Width)-db.text.idc.MinSize().Width, db.gauges.nblambda.Size().Height-14))

	db.text.activeAirDem.TextSize = min(space.Width*common.OneTwentyFifth, 45)
	db.text.activeAirDem.Move(fyne.NewPos(space.Width*.5, thirdHeight))

	db.text.cruise.Move(fyne.NewPos(sixthWidth*1.45, space.Height-(db.checkEngine.Size().Height*.6)-db.gauges.wblambda.Size().Height))

}

func (dr *DashboardRenderer) MinSize() fyne.Size {
	return fyne.NewSize(600, 500)
}

func (dr *DashboardRenderer) Refresh() {
}

func (dr *DashboardRenderer) Destroy() {
}

func (dr *DashboardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{dr.db.container}
}
