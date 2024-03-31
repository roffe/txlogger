package widgets

import (
	_ "embed"
	"fmt"
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
)

type Dashboard struct {
	cfg *DashboardConfig

	widget.BaseWidget
	rpm, speed, iat                 *Dial
	throttle, pwm                   *VBar
	engineTemp                      *Dial
	nblambda, wblambda              *CBar
	boost, air                      *DualDial
	ioff, activeAirDem, ign, cruise *canvas.Text
	idc                             *canvas.Text
	checkEngine                     *canvas.Image
	limpMode                        *canvas.Image
	knockIcon                       *Icon
	time                            *canvas.Text

	container fyne.CanvasObject

	fullscreenBtn *widget.Button
	closeBtn      *widget.Button
	logBtn        *widget.Button

	//dbgBar *fyne.Container

	metrics map[string]func(float64)

	logplayer bool
	focused   bool
}

type DashboardConfig struct {
	App             fyne.App
	Mw              fyne.Window
	Logplayer       bool
	LogBtn          *widget.Button
	OnClose         func()
	AirDemToString  func(float64) string
	UseMPH          bool
	SwapRPMandSpeed bool
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
		speed: NewDial(DialConfig{
			Title:         speedometerText,
			Min:           0,
			Max:           300,
			Steps:         30,
			DisplayString: "%.1f",
		}),
		rpm: NewDial(DialConfig{
			Title: "RPM",
			Min:   0,
			Max:   8000,
			Steps: 20,
		}),
		iat: NewDial(DialConfig{
			Title: "IAT",
			Min:   -40,
			Max:   80,
			Steps: 16,
		}),
		boost: NewDualDial(DualDialConfig{
			Title:         "MAP",
			Min:           0,
			Max:           3,
			Steps:         30,
			DisplayString: "%.2f",
		}),
		throttle: NewVBar(&VBarConfig{
			Title:   "TPS",
			Min:     0,
			Max:     100,
			Steps:   20,
			Minsize: fyne.NewSize(50, 100),
		}),
		pwm: NewVBar(&VBarConfig{
			Title:   "PWM",
			Min:     0,
			Max:     100,
			Steps:   20,
			Minsize: fyne.NewSize(50, 100),
		}),
		engineTemp: NewDial(DialConfig{
			Title: "tEng",
			Min:   -40,
			Max:   160,
			Steps: 16,
		}),
		wblambda: NewCBar(&CBarConfig{
			Title:         "",
			Min:           0.50,
			Center:        1,
			Max:           1.50,
			Steps:         20,
			Minsize:       fyne.NewSize(100, 35),
			DisplayString: "λ %.2f",
		}),
		nblambda: NewCBar(&CBarConfig{
			Title:           "",
			Min:             -25,
			Center:          0,
			Max:             25,
			Steps:           40,
			Minsize:         fyne.NewSize(100, 45),
			DisplayString:   "%.2f%%",
			DisplayTextSize: 40,
			TextPosition:    TextAtBottom,
		}),
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
		air: NewDualDial(DualDialConfig{
			Title: "mg/c",
			Min:   0,
			Max:   2200,
			Steps: 22,
		}),
		checkEngine: canvas.NewImageFromResource(fyne.NewStaticResource("checkengine.png", assets.CheckengineBytes)),
		fullscreenBtn: widget.NewButtonWithIcon("Fullscreen", theme.ZoomFitIcon(), func() {
			cfg.Mw.SetFullScreen(!cfg.Mw.FullScreen())
		}),
		knockIcon: NewIcon(&IconConfig{
			Image:   canvas.NewImageFromResource(fyne.NewStaticResource("knock.png", assets.KnockBytes)),
			Minsize: fyne.NewSize(90, 90),
		}),
		limpMode: canvas.NewImageFromResource(fyne.NewStaticResource("limp.png", assets.LimpBytes)),
	}
	db.ExtendBaseWidget(db)

	db.metrics = db.createRouter()

	db.closeBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		if db.cfg.OnClose != nil {
			db.cfg.OnClose()
		}
	})

	if cfg.Logplayer {
		db.time = canvas.NewText("00:00:00.00", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
		db.time.TextSize = 35
		db.time.Resize(fyne.NewSize(200, 50))
	}

	//db.dbgBar = db.newDebugBar()

	db.knockIcon.Hide()
	db.cruise.Hide()
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

		db.rpm,
		db.speed,
		db.air,
		db.boost,
		db.iat,
		db.engineTemp,

		db.ign,
		db.ioff,
		db.idc,

		db.activeAirDem,

		db.nblambda,
		db.wblambda,
		db.throttle,
		db.pwm,
		db.checkEngine,
		db.cruise,
		db.knockIcon,
	)

	if !db.logplayer {
		content.Add(db.fullscreenBtn)
		content.Add(db.closeBtn)
		content.Add(db.logBtn)
	} else {
		content.Add(db.time)
	}

	return content
}

func (db *Dashboard) GetMetricNames() []string {
	names := make([]string, 0, len(db.metrics))
	for k := range db.metrics {
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
	if db.time != nil {
		db.time.Text = t.Format("15:04:05.000")
		db.time.Refresh()
	}
}
func (db *Dashboard) SetTimeText(text string) {
	if db.time != nil {
		db.time.Text = text
		db.time.Refresh()
	}
}

func (db *Dashboard) SetValue(key string, value float64) {
	if fun, ok := db.metrics[key]; ok {
		fun(value)
	}
}

func (db *Dashboard) createRouter() map[string]func(float64) {
	textSetter := func(obj *canvas.Text, text string, precission int) func(float64) {
		return func(value float64) {
			obj.Text = text + ": " + strconv.FormatFloat(value, 'f', precission, 64)
			obj.Refresh()
		}
	}

	idcSetter := func(obj *canvas.Text, text string) func(float64) {
		return func(value float64) {
			obj.Text = fmt.Sprintf(text+": %02.0f%%", value)
			switch {
			case value > 60 && value < 85:
				obj.Color = color.RGBA{R: 0xFF, G: 0xA5, B: 0, A: 0xFF}
			case value >= 85:
				obj.Color = color.RGBA{R: 0xFF, G: 0, B: 0, A: 0xFF}
			default:
				obj.Color = color.RGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}
			}
			obj.Refresh()
		}
	}

	ioff := func(value float64) {
		db.ioff.Text = "Ioff: " + strconv.FormatFloat(value, 'f', 1, 64)
		switch {
		case value >= 0:
			db.ioff.Color = color.RGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}
		case value < 0 && value >= -3:
			db.ioff.Color = color.RGBA{R: 0xFF, G: 0xA5, B: 0, A: 0xFF}
		case value < -3:
			db.ioff.Color = color.RGBA{R: 0xFF, G: 0, B: 0, A: 0xFF}
		}
		db.ioff.Refresh()
	}

	ecmstat := func(value float64) {
		db.activeAirDem.Text = db.cfg.AirDemToString(value) + " (" + strconv.FormatFloat(value, 'f', 0, 64) + ")"
		db.activeAirDem.Refresh()
	}

	showHider := func(obj fyne.CanvasObject) func(float64) {
		return func(value float64) {
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

	var setVehicleSpeed func(float64)

	if db.cfg.UseMPH {
		setVehicleSpeed = func(value float64) {
			db.speed.SetValue(value * 0.621371)
		}
	} else {
		setVehicleSpeed = db.speed.SetValue
	}

	router := map[string]func(float64){
		"In.v_Vehicle":               setVehicleSpeed,
		"ActualIn.n_Engine":          db.rpm.SetValue,
		"ActualIn.T_AirInlet":        db.iat.SetValue,
		"ActualIn.T_Engine":          db.engineTemp.SetValue,
		"In.p_AirInlet":              db.boost.SetValue,
		"ActualIn.p_AirInlet":        db.boost.SetValue,
		"In.p_AirBefThrottle":        db.boost.SetValue2,
		"ActualIn.p_AirBefThrottle":  db.boost.SetValue2,
		"Out.X_AccPedal":             db.throttle.SetValue, // t7
		"Out.X_AccPos":               db.throttle.SetValue, // t8
		"Out.PWM_BoostCntrl":         db.pwm.SetValue,
		"DisplProt.LambdaScanner":    db.wblambda.SetValue,
		"Lambda.External":            db.wblambda.SetValue,
		"Lambda.LambdaInt":           db.nblambda.SetValue,
		"MAF.m_AirInlet":             db.air.SetValue,
		"m_Request":                  db.air.SetValue2,
		"AirMassMast.m_Request":      db.air.SetValue2,
		"Out.fi_Ignition":            textSetter(db.ign, "Ign", 1),
		"ECMStat.ST_ActiveAirDem":    ecmstat,
		"IgnProt.fi_Offset":          ioff,
		"IgnMastProt.fi_Offset":      ioff,
		"CRUISE":                     showHider(db.cruise),
		"CEL":                        showHider(db.checkEngine),
		"LIMP":                       showHider(db.limpMode),
		"KnkDet.KnockCyl":            knkDet,
		"Myrtilos.InjectorDutyCycle": idcSetter(db.idc, "Idc"),
	}

	return router
}

func (db *Dashboard) Sweep() {
	db.checkEngine.Hide()
	an := fyne.NewAnimation(900*time.Millisecond, func(p float32) {
		pa := float64(p)
		db.speed.SetValue(300 * pa)
		db.rpm.SetValue(8000 * pa)
		db.iat.SetValue(80 * pa)
		db.air.SetValue(2100 * pa)
		db.air.SetValue2(2200 * pa)
		db.engineTemp.SetValue(160 * pa)
		db.boost.SetValue(3 * pa)
		db.throttle.SetValue(100 * pa)
		db.pwm.SetValue(100 * pa)
		db.nblambda.SetValue(25 * pa)
		db.wblambda.SetValue(1.52 * pa)
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
	db.speed.SetValue(value)
	db.rpm.SetValue(value)
	db.iat.SetValue(value)
	db.engineTemp.SetValue(value)
	db.boost.SetValue(value)
	db.throttle.SetValue(value)
	db.pwm.SetValue(value)
	db.nblambda.SetValue(value)
	db.wblambda.SetValue(value)
	db.air.SetValue(value)
	db.air.SetValue2(value)
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
			db.speed.SetValue(110)
			db.rpm.SetValue(3320)
			db.iat.SetValue(30)
			db.engineTemp.SetValue(85)
			db.boost.SetValue(1.2)
			db.throttle.SetValue(85)
			db.pwm.SetValue(47)
			db.nblambda.SetValue(2.13)
			db.wblambda.SetValue(1.03)
			db.air.SetValue(1003)
			db.air.SetValue2(1200)
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

	log.Println("dashboard.Layout", space.Width, space.Height)
	dr.db.container.Resize(space)

	db := dr.db

	var sixthWidth float32 = space.Width * oneSixth
	var thirdHeight float32 = (space.Height - 50) * .33
	var halfHeight float32 = (space.Height - 50) * .5

	// Dials

	if !db.cfg.SwapRPMandSpeed {
		// Top left
		db.rpm.Resize(fyne.NewSize(sixthWidth, thirdHeight))
		db.rpm.Move(fyne.NewPos(0, 0))

		// Center dial
		db.speed.Resize(fyne.NewSize(space.Width-sixthWidth*2-(sixthWidth*oneThird*2)-20, space.Height-115))
		db.speed.Move(fyne.NewPos(space.Width*.5-db.speed.Size().Width*.5, space.Height*.5-db.speed.Size().Height*.5+25))
	} else {
		db.speed.Resize(fyne.NewSize(sixthWidth, thirdHeight))
		db.speed.Move(fyne.NewPos(0, 0))
		// Center dial
		db.rpm.Resize(fyne.NewSize(space.Width-sixthWidth*2-(sixthWidth*oneThird*2)-20, space.Height-115))
		db.rpm.Move(fyne.NewPos(space.Width*.5-db.rpm.Size().Width*.5, space.Height*.5-db.rpm.Size().Height*.5+25))
	}

	db.air.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.air.Move(fyne.NewPos(0, thirdHeight))

	db.boost.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.boost.Move(fyne.NewPos(0, thirdHeight*2))

	db.iat.Resize(fyne.NewSize(sixthWidth, halfHeight))
	db.iat.Move(fyne.NewPos(space.Width-db.iat.Size().Width, 0))

	db.engineTemp.Resize(fyne.NewSize(sixthWidth, halfHeight))
	db.engineTemp.Move(fyne.NewPos(space.Width-db.engineTemp.Size().Width, halfHeight))

	// Vbar
	pwm := db.pwm
	pwm.Resize(fyne.NewSize(sixthWidth*oneThird, space.Height-125))
	pwm.Move(fyne.NewPos(sixthWidth+8, 25))

	tps := db.throttle
	tps.Resize(fyne.NewSize(sixthWidth*oneThird, space.Height-125))
	tps.Move(fyne.NewPos(space.Width-sixthWidth-tps.Size().Width-8, 25))

	// Cbar
	db.nblambda.Resize(fyne.NewSize((sixthWidth * 3), 65))
	db.nblambda.Move(fyne.NewPos(sixthWidth*1.5, 0))

	db.wblambda.Resize(fyne.NewSize((sixthWidth * 3), 65))
	db.wblambda.Move(fyne.NewPos(sixthWidth*1.5, space.Height-65))

	// Icons
	db.limpMode.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.limpMode.Move(fyne.NewPos(space.Width*.5-db.limpMode.Size().Width*.5, space.Height*.5-db.limpMode.Size().Height*.5-(thirdHeight*.5)))

	db.checkEngine.Resize(fyne.NewSize(sixthWidth*.5, thirdHeight*.5))
	db.checkEngine.Move(fyne.NewPos(space.Width-db.engineTemp.Size().Width-db.throttle.Size().Width-db.checkEngine.Size().Width-15, space.Height-db.checkEngine.Size().Height-db.wblambda.Size().Height))

	db.knockIcon.Move(fyne.NewPos((space.Width*.5)-(db.checkEngine.Size().Width*.5)-(sixthWidth*.7), space.Height*.5-60))

	// Buttons

	db.closeBtn.Resize(fyne.NewSize(sixthWidth, 55))
	db.closeBtn.Move(fyne.NewPos(space.Width-sixthWidth, space.Height-55))

	if !db.logplayer {
		if space.Width < 1000 {
			db.fullscreenBtn.SetText("(F)")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth*oneHalfOne, 55))
		} else if space.Width < 1300 {
			db.fullscreenBtn.SetText("Fullscrn")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth*oneOneEight, 55))
		} else {
			db.fullscreenBtn.SetText("Fullscreen")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth*oneOneFive, 55))
		}

		db.logBtn.Resize(fyne.NewSize(db.wblambda.Position().X-db.fullscreenBtn.Size().Width-14, 55))
		db.logBtn.Move(fyne.NewPos(db.fullscreenBtn.Size().Width+5, space.Height-55))
	} else {
		db.time.Move(fyne.NewPos(space.Width*.5-100, space.Height*oneHalfSix))
	}
	db.fullscreenBtn.Move(fyne.NewPos(0, space.Height-55))

	// Text

	//db.ign.TextSize = textSize
	db.ign.Move(fyne.NewPos(db.nblambda.Position().X, db.nblambda.Size().Height-14))

	//db.ioff.TextSize = textSize
	db.ioff.Move(fyne.NewPos(db.nblambda.Position().X, db.ign.Position().Y+54))

	//db.idc.TextSize = textSize
	db.idc.Move(fyne.NewPos((db.nblambda.Position().X+db.nblambda.Size().Width)-db.idc.MinSize().Width, db.nblambda.Size().Height-14))

	db.activeAirDem.TextSize = min(space.Width*oneTwentyFifth, 45)
	db.activeAirDem.Move(fyne.NewPos(space.Width*.5, thirdHeight))

	db.cruise.Move(fyne.NewPos(sixthWidth*1.45, space.Height-(db.checkEngine.Size().Height*.6)-db.wblambda.Size().Height))

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
