package windows

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
	"github.com/roffe/txlogger/pkg/model"
	"github.com/roffe/txlogger/pkg/widgets"
)

//go:embed checkengine.png
var checkengineBytes []byte

//go:embed limp.png
var limpBytes []byte

//go:embed knock.png
var knockBytes []byte

type Gauge interface {
	SetValue(float64)
	Value() float64
	Content() fyne.CanvasObject
}

type Dashboard struct {
	speed, rpm, iat/*, mReq, mAir*/ Gauge
	throttle, pwm, engineTemp, nblambda, wblambda Gauge
	boost, dual                                   *widgets.DualDial
	ioff, activeAirDem, ign, cruise               *canvas.Text
	checkEngine                                   *canvas.Image
	limpMode                                      *canvas.Image
	knockIcon                                     *widgets.Icon
	time                                          *canvas.Text

	canvas fyne.CanvasObject

	fullscreenBtn *widget.Button
	closeBtn      *widget.Button
	logBtn        *widget.Button

	//dbgBar *fyne.Container

	metricsChan chan *model.DashboardMetric

	logplayer bool
}

func NewDashboard(mw *MainWindow, logplayer bool, logBtn *widget.Button) *Dashboard {
	db := &Dashboard{
		logBtn:    logBtn,
		logplayer: logplayer,
		speed: widgets.NewDial(widgets.DialConfig{
			Title:         "km/h",
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
			Min:   -40,
			Max:   80,
			Steps: 16,
		}),
		//mReq: widgets.NewDial(widgets.DialConfig{
		//	Title: "mReq",
		//	Min:   0,
		//	Max:   2200,
		//	Steps: 22,
		//}),
		//mAir: widgets.NewDial(widgets.DialConfig{
		//	Title: "mAir",
		//	Min:   0,
		//	Max:   2200,
		//	Steps: 22,
		//}),
		boost: widgets.NewDualDial(widgets.DualDialConfig{
			Title:         "MAP",
			Min:           0,
			Max:           3,
			Steps:         30,
			DisplayString: "%.2f",
		}),
		throttle: widgets.NewVBar(&widgets.VBarConfig{
			Title:   "TPS",
			Min:     0,
			Max:     100,
			Steps:   20,
			Minsize: fyne.NewSize(75, 100),
		}),
		pwm: widgets.NewVBar(&widgets.VBarConfig{
			Title:   "PWM",
			Min:     0,
			Max:     100,
			Steps:   20,
			Minsize: fyne.NewSize(75, 100),
		}),
		engineTemp: widgets.NewDial(widgets.DialConfig{
			Title: "tEng",
			Min:   -40,
			Max:   160,
			Steps: 16,
		}),
		wblambda: widgets.NewCBar(&widgets.CBarConfig{
			Title:         "",
			Min:           0.50,
			Center:        1,
			Max:           1.52,
			Steps:         20,
			Minsize:       fyne.NewSize(100, 45),
			DisplayString: "λ %.2f",
		}),
		nblambda: widgets.NewCBar(&widgets.CBarConfig{
			Title:         "",
			Min:           -25,
			Center:        0,
			Max:           25,
			Steps:         40,
			Minsize:       fyne.NewSize(100, 45),
			DisplayString: "%.2f%%",
			TextAtBottom:  true,
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
		dual: widgets.NewDualDial(widgets.DualDialConfig{
			Title: "mg/c",
			Min:   0,
			Max:   2200,
			Steps: 22,
		}),
		checkEngine: canvas.NewImageFromResource(fyne.NewStaticResource("checkengine.png", checkengineBytes)),
		fullscreenBtn: widget.NewButtonWithIcon("Fullscreen", theme.ZoomFitIcon(), func() {
			mw.SetFullScreen(!mw.FullScreen())
		}),
		closeBtn: widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
			if mw.dlc != nil {
				mw.dlc.DetachDashboard(mw.dashboard)
			}
			close(mw.dashboard.metricsChan)
			mw.dashboard = nil
			mw.SetFullScreen(false)
			mw.SetContent(mw.Content())
		}),
		knockIcon: widgets.NewIcon(&widgets.IconConfig{
			Image:   canvas.NewImageFromResource(fyne.NewStaticResource("knock.png", knockBytes)),
			Minsize: fyne.NewSize(90, 90),
		}),
		limpMode:    canvas.NewImageFromResource(fyne.NewStaticResource("limp.png", limpBytes)),
		metricsChan: make(chan *model.DashboardMetric, 60),
	}

	if logplayer {
		db.time = &canvas.Text{
			Text:     "00:00:00.000",
			Color:    color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF},
			TextSize: 35,
		}
		db.time.Resize(fyne.NewSize(200, 50))
	}

	//db.dbgBar = db.newDebugBar()

	db.knockIcon.Content().Hide()
	db.cruise.Hide()
	db.checkEngine.Hide()
	db.limpMode.Hide()

	db.checkEngine.FillMode = canvas.ImageFillContain
	db.checkEngine.SetMinSize(fyne.NewSize(110, 85))
	db.checkEngine.Resize(fyne.NewSize(110, 85))

	db.limpMode.FillMode = canvas.ImageFillContain
	db.limpMode.SetMinSize(fyne.NewSize(110, 85))
	db.limpMode.Resize(fyne.NewSize(110, 85))

	db.canvas = db.render()

	//db.knockIcon.Hide()

	go db.startParser()
	return db
}

func (db *Dashboard) render() fyne.CanvasObject {
	content := container.NewStack(
		container.NewWithoutLayout(
			db.limpMode,
			//db.dbgBar,
			db.ign,
			db.ioff,
			db.rpm.Content(),
			db.dual.Content(),
			db.boost.Content(),
			db.iat.Content(),
			db.engineTemp.Content(),

			//db.mReq.Content(),
			//db.mAir.Content(),

			db.speed.Content(),
			db.activeAirDem,

			db.nblambda.Content(),
			db.wblambda.Content(),
			db.throttle.Content(),
			db.pwm.Content(),
			db.checkEngine,
			db.cruise,
			db.knockIcon.Content(),
		))

	if !db.logplayer {
		content.Add(db.fullscreenBtn)
		content.Add(db.closeBtn)
		content.Add(db.logBtn)
	} else {
		content.Add(db.time)
	}

	content.Layout = db

	return content
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
	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
		}
	}()
	select {
	case db.metricsChan <- &model.DashboardMetric{Name: key, Value: value}:
	default:
		log.Println("failed to set value")
	}
}

func (db *Dashboard) startParser() {
	metrics := map[string]Gauge{
		"In.v_Vehicle":        db.speed,
		"ActualIn.n_Engine":   db.rpm,
		"ActualIn.T_AirInlet": db.iat,
		//"m_Request":               db.mReq,
		//"MAF.m_AirInlet":          db.mAir,
		"ActualIn.T_Engine": db.engineTemp,
		//"ECMStat.p_Diff":          db.boost,
		//"ActualIn.p_AirInlet":     db.boost,
		//"In.p_AirInlet":           db.boost,
		"Out.X_AccPedal":          db.throttle, // t7
		"Out.X_AccPos":            db.throttle, // t8
		"Out.PWM_BoostCntrl":      db.pwm,
		"DisplProt.LambdaScanner": db.wblambda,
		"Lambda.LambdaInt":        db.nblambda,
	}
	for metric := range db.metricsChan {
		if m, ok := metrics[metric.Name]; ok {
			if metric.Value != m.Value() {
				m.SetValue(metric.Value)
				continue
			}
		}
		switch metric.Name {
		//case "Out.X_AccPos":
		//	db.throttle.SetValue(metric.Value * .1)
		case "CRUISE":
			if metric.Value == 1 {
				db.cruise.Show()
			} else {
				db.cruise.Hide()
			}
		case "CEL":
			if metric.Value == 1 {
				db.checkEngine.Show()
			} else {
				db.checkEngine.Hide()
			}
		case "LIMP":
			if metric.Value == 1 {
				db.limpMode.Show()
			} else {
				db.limpMode.Hide()
			}
		case "KnkDet.KnockCyl":
			if metric.Value > 0 {
				kn := int(metric.Value)
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
				db.knockIcon.SetText(fmt.Sprintf("%d", knockValue))
				db.knockIcon.Content().Show()
			} else {
				db.knockIcon.Content().Hide()
			}
		case "In.p_AirInlet", "ActualIn.p_AirInlet":
			db.boost.SetValue(metric.Value)

		case "In.p_AirBefThrottle", "ActualIn.p_AirBefThrottle":
			db.boost.SetValue2(metric.Value)
		case "MAF.m_AirInlet":
			db.dual.SetValue(metric.Value)
		case "m_Request", "AirMassMast.m_Request":
			db.dual.SetValue2(metric.Value)
		case "Out.fi_Ignition":
			db.ign.Text = "Ign: " + strconv.FormatFloat(metric.Value, 'f', 1, 64)
			//db.ign.Text = fmt.Sprintf("Ign: %-6s", strconv.FormatFloat(metric.Value, 'f', 1, 64))
			db.ign.Refresh()
		case "IgnProt.fi_Offset", "IgnMastProt.fi_Offset":
			db.ioff.Text = "Ioff: " + strconv.FormatFloat(metric.Value, 'f', 1, 64)
			//db.ioff.Text = fmt.Sprintf("Ioff: %-6s", strconv.FormatFloat(metric.Value, 'f', 1, 64))
			switch {
			case metric.Value >= 0:
				db.ioff.Color = color.RGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}
			case metric.Value < 0 && metric.Value >= -3:
				db.ioff.Color = color.RGBA{R: 0xFF, G: 0xA5, B: 0, A: 0xFF}
			case metric.Value < -3:
				db.ioff.Color = color.RGBA{R: 0xFF, G: 0, B: 0, A: 0xFF}
			}
			db.ioff.Refresh()
		case "ECMStat.ST_ActiveAirDem":
			db.activeAirDem.Text = AirDemToString(metric.Value) + " (" + strconv.FormatFloat(metric.Value, 'f', 0, 64) + ")"
			db.activeAirDem.Refresh()
		}
	}
}

func (db *Dashboard) Content() fyne.CanvasObject {
	return db.canvas
}

func (db *Dashboard) Sweep() {
	db.checkEngine.Hide()
	an := fyne.NewAnimation(900*time.Millisecond, func(p float32) {
		pa := float64(p)
		db.speed.SetValue(300 * pa)
		db.rpm.SetValue(8000 * pa)
		db.iat.SetValue(80 * pa)
		//db.mReq.SetValue(2200 * pa)
		//db.mAir.SetValue(2200 * pa)
		db.dual.SetValue(2200 * pa)
		db.dual.SetValue2(2200 * pa)
		db.engineTemp.SetValue(160 * pa)
		db.boost.SetValue(3 * pa)
		db.throttle.SetValue(100 * pa)
		db.pwm.SetValue(100 * pa)
		db.nblambda.SetValue(25 * pa)
		db.wblambda.SetValue(1.52 * pa)
		db.metricsChan <- &model.DashboardMetric{Name: "Out.fi_Ignition", Value: 30.0 * pa}
		db.metricsChan <- &model.DashboardMetric{Name: "IgnProt.fi_Offset", Value: 15.0 * pa}

	})
	an.AutoReverse = true
	an.Curve = fyne.AnimationEaseInOut
	an.Start()
	time.Sleep(1800 * time.Millisecond)
	db.checkEngine.Show()
}

func (db *Dashboard) Layout(_ []fyne.CanvasObject, space fyne.Size) {
	var sixthWidth float32 = space.Width / 6
	var thirdHeight float32 = (space.Height - 50) / 3
	var halfHeight float32 = (space.Height - 50) / 2

	// Dials
	rpm := db.rpm.Content()
	rpm.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	rpm.Move(fyne.NewPos(0, 0))

	dual := db.dual.Content()
	dual.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	dual.Move(fyne.NewPos(0, thirdHeight))

	boost := db.boost.Content()
	boost.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	boost.Move(fyne.NewPos(0, thirdHeight*2))

	iat := db.iat.Content()
	iat.Resize(fyne.NewSize(sixthWidth, halfHeight))
	iat.Move(fyne.NewPos(space.Width-iat.Size().Width, 0))

	teng := db.engineTemp.Content()
	teng.Resize(fyne.NewSize(sixthWidth, halfHeight))
	teng.Move(fyne.NewPos(space.Width-teng.Size().Width, halfHeight))

	//mreq := db.mReq.Content()
	//mreq.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	//mreq.Move(fyne.NewPos(space.Width-mreq.Size().Width, 0+thirdHeight))
	//
	//mair := db.mAir.Content()
	//mair.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	//mair.Move(fyne.NewPos(space.Width-mair.Size().Width, 0+thirdHeight*2))

	// Center dial
	speed := db.speed.Content()
	speed.Resize(fyne.NewSize(space.Width-sixthWidth*2-(sixthWidth/3*2)-20, space.Height-115))
	speed.Move(fyne.NewPos(space.Width/2-speed.Size().Width/2, space.Height/2-speed.Size().Height/2+25))

	// Vbar
	pwm := db.pwm.Content()
	pwm.Resize(fyne.NewSize(sixthWidth/3, space.Height-125))
	pwm.Move(fyne.NewPos(sixthWidth+8, 25))

	tps := db.throttle.Content()
	tps.Resize(fyne.NewSize(sixthWidth/3, space.Height-125))
	tps.Move(fyne.NewPos(space.Width-sixthWidth-tps.Size().Width-8, 25))

	// Cbar
	nblambda := db.nblambda.Content()
	nblambda.Resize(fyne.NewSize((sixthWidth*3)-10, 65))
	nblambda.Move(fyne.NewPos(sixthWidth*1.5, 0))

	wblambda := db.wblambda.Content()
	wblambda.Resize(fyne.NewSize((sixthWidth*3)-10, 65))
	wblambda.Move(fyne.NewPos(sixthWidth*1.5, space.Height-65))

	// Icons
	db.limpMode.Resize(fyne.NewSize(sixthWidth, thirdHeight))
	db.limpMode.Move(fyne.NewPos(space.Width/2-db.limpMode.Size().Width/2, space.Height/2-db.limpMode.Size().Height/2-(thirdHeight/2)))

	db.checkEngine.Resize(fyne.NewSize(sixthWidth/2, thirdHeight/2))
	//db.checkEngine.Move(fyne.NewPos(space.Width/2-db.checkEngine.Size().Width/2+sixthWidth*1.3, space.Height-db.checkEngine.Size().Height-db.wblambda.Content().Size().Height))
	db.checkEngine.Move(fyne.NewPos(space.Width-db.engineTemp.Content().Size().Width-db.throttle.Content().Size().Width-db.checkEngine.Size().Width-15, space.Height-db.checkEngine.Size().Height-db.wblambda.Content().Size().Height))

	db.knockIcon.Content().Move(fyne.NewPos((space.Width/2)-(db.checkEngine.Size().Width/2)-(sixthWidth*.7), space.Height/2-60))

	// Buttons

	db.closeBtn.Resize(fyne.NewSize(sixthWidth, 55))
	db.closeBtn.Move(fyne.NewPos(space.Width-sixthWidth, space.Height-55))

	if !db.logplayer {
		if space.Width < 1000 {
			db.fullscreenBtn.SetText("(F)")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth/2.1, 55))
		} else if space.Width < 1300 {
			db.fullscreenBtn.SetText("Fullscrn")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth/1.8, 55))
		} else {
			db.fullscreenBtn.SetText("Fullscreen")
			db.fullscreenBtn.Resize(fyne.NewSize(sixthWidth/1.5, 55))
		}

		db.logBtn.Resize(fyne.NewSize(db.wblambda.Content().Position().X-db.fullscreenBtn.Size().Width-14, 55))
		db.logBtn.Move(fyne.NewPos(db.fullscreenBtn.Size().Width+5, space.Height-55))
		//} else {
		//	db.logBtn.Resize(fyne.NewSize(sixthWidth/2, 30))
		//	db.logBtn.Move(fyne.NewPos(0, space.Height-80))
		//}
		//db.logBtn.Resize(fyne.NewSize(125, 45))
	} else {
		db.time.Move(fyne.NewPos(space.Width/2-100, space.Height/2.6))
	}
	db.fullscreenBtn.Move(fyne.NewPos(0, space.Height-55))

	//db.dbgBar.Resize(fyne.NewSize(sixthWidth*3, 25))
	//db.dbgBar.Move(fyne.NewPos(space.Width/2-db.dbgBar.Size().Width/2, 0))

	// Text

	//db.ign.TextSize = textSize
	db.ign.Move(fyne.NewPos(nblambda.Position().X, nblambda.Size().Height-14))

	//db.ioff.TextSize = textSize
	db.ioff.Move(fyne.NewPos(nblambda.Position().X, db.ign.Position().Y+54))

	db.activeAirDem.TextSize = min(space.Width/25.0, 45)
	db.activeAirDem.Move(fyne.NewPos(space.Width/2, thirdHeight))

	db.cruise.Move(fyne.NewPos(sixthWidth*1.45, space.Height-(db.checkEngine.Size().Height*.6)-db.wblambda.Content().Size().Height))

}

func (db *Dashboard) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(800, 600)
}

func (db *Dashboard) setValue(value float64) {
	db.speed.SetValue(value)
	db.rpm.SetValue(value)
	db.iat.SetValue(value)
	//db.mReq.SetValue(value)
	//db.mAir.SetValue(value)
	db.engineTemp.SetValue(value)
	db.boost.SetValue(value)
	db.throttle.SetValue(value)
	db.pwm.SetValue(value)
	db.nblambda.SetValue(value)
	db.wblambda.SetValue(value)
	db.dual.SetValue(value)
	db.dual.SetValue2(value)
}

func (db *Dashboard) Capture() {

}

func (db *Dashboard) newDebugBar() *fyne.Container {
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
			//db.mReq.SetValue(1200)
			//db.mAir.SetValue(1200)
			db.engineTemp.SetValue(85)
			db.boost.SetValue(1.2)
			db.throttle.SetValue(85)
			db.pwm.SetValue(47)
			db.nblambda.SetValue(2.13)
			db.wblambda.SetValue(1.03)
			db.dual.SetValue(1003)
			db.dual.SetValue2(1200)
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

func AirDemToString(v float64) string {
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
		return "N.A"
	case 55:
		return "Faulty APC valve"
	case 60:
		return "Emission Limitation"
	case 70:
		return "Safety Switch Limit"
	default:
		return "Unknown"
	}
}
