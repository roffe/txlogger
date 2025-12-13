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

	size fyne.Size

	widget.BaseWidget
}

type Images struct {
	checkEngine *canvas.Image
	limpMode    *canvas.Image
	knockIcon   *icon.Icon
	taz         *canvas.Image
}

type Texts struct {
	ign, ioff, idc       *canvas.Text
	activeAirDem, cruise *canvas.Text
	amul                 *canvas.Text
	time                 *canvas.Text
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
	Low             float64
	High            float64
	WidebandSymbol  string
	MetricRouter    map[string]func(float64)
	FullscreenFunc  func(bool)
}

func NewDashboard(cfg *Config) *Dashboard {
	if cfg.AirDemToString == nil {
		cfg.AirDemToString = func(f float64) string {
			return "Unknown"
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
				Max:     2500,
				Steps:   20,
				MinSize: fyne.NewSize(100, 100),
			}),
			speed: dial.New(&widgets.GaugeConfig{
				Title:           speedometerText,
				Min:             0,
				Max:             300,
				Steps:           30,
				DisplayString:   "%.1f",
				GaugeTextString: "%.0f",
				MinSize:         fyne.NewSize(100, 100),
			}),
			rpm: dial.New(&widgets.GaugeConfig{
				Title:       "RPM",
				Min:         0,
				Max:         8000,
				Steps:       16,
				MinSize:     fyne.NewSize(100, 100),
				GaugeFactor: 0.001,
			}),
			iat: dial.New(&widgets.GaugeConfig{
				Title:   "IAT",
				Min:     0,
				Max:     80,
				Steps:   16,
				MinSize: fyne.NewSize(100, 100),
			}),
			pressure: dualdial.New(&widgets.GaugeConfig{
				Title:           "MAP",
				Min:             0,
				Max:             3,
				Steps:           30,
				DisplayString:   "%.2f",
				GaugeTextString: "%.1f",
				MinSize:         fyne.NewSize(100, 100),
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
				Max:   140,
				Steps: 16,
			}),
			wblambda: cbar.New(&widgets.GaugeConfig{
				Title:           "",
				Min:             0.50,
				Center:          1,
				Max:             1.50,
				Steps:           20,
				MinSize:         fyne.NewSize(50, 35),
				DisplayString:   "λ %.2f",
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
				Text:      "Idc:  0%",
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

func (db *Dashboard) layoutDials(dims *dims) {
	left := db.gauges.pwm.Position().X + db.gauges.pwm.Size().Width
	right := db.gauges.throttle.Position().X
	width := right - left

	centerDialSize := fyne.Size{
		Width:  width,
		Height: db.size.Height - 125,
	}
	centerDialPos := fyne.Position{
		X: dims.centerX - centerDialSize.Width*0.5,
		Y: dims.centerY - centerDialSize.Height*0.46,
	}

	if !db.cfg.SwapRPMandSpeed {
		db.gauges.rpm.Resize(fyne.Size{Width: dims.sixthWidth, Height: dims.thirdHeight})
		db.gauges.rpm.Move(fyne.Position{X: 0, Y: 5})
		db.gauges.speed.Resize(centerDialSize)
		db.gauges.speed.Move(centerDialPos)
	} else {
		db.gauges.speed.Resize(fyne.Size{Width: dims.sixthWidth, Height: dims.thirdHeight})
		db.gauges.speed.Move(fyne.Position{X: 0, Y: 5})
		db.gauges.rpm.Resize(centerDialSize)
		db.gauges.rpm.Move(centerDialPos)
	}

	// Calculate dial size
	//dialWidth := dims.sixthWidth
	//dialHeight := dims.thirdHeight

	// Calculate total height needed for all dials (3 dials on left side including the main dial)
	totalDialHeight := dims.thirdHeight * 3 // Three dials on left side (main + 2), two on right

	// Calculate vertical spacing between dials
	spacing := (db.size.Height - totalDialHeight) / 4 // Divide remaining space into 4 parts for spacing

	// Left side dials positioning
	// Note: Top dial (speed or rpm) is already positioned by layoutMainDials
	// Second dial: airmass
	db.gauges.airmass.Resize(fyne.Size{Width: dims.sixthWidth, Height: dims.thirdHeight})
	db.gauges.airmass.Move(fyne.Position{X: 0, Y: spacing*2 + dims.thirdHeight}) // After top dial + spacing

	// Third dial: pressure
	db.gauges.pressure.Resize(fyne.Size{Width: dims.sixthWidth, Height: dims.thirdHeight})
	db.gauges.pressure.Move(fyne.Position{X: 0, Y: spacing*3 + dims.thirdHeight*2}) // After top dial + middle dial + spacing
	// Right side dials (only 2 dials, different spacing)
	rightX := db.size.Width - dims.sixthWidth

	// Calculate spacing for right side (only 2 dials)
	rightSideSpacing := (db.size.Height - (dims.thirdHeight * 2)) / 3 // Three spaces for two dials

	// First dial: IAT
	db.gauges.iat.Resize(fyne.Size{Width: dims.sixthWidth, Height: dims.thirdHeight})
	db.gauges.iat.Move(fyne.Position{X: rightX, Y: rightSideSpacing})

	// Second dial: engineTemp
	db.gauges.engineTemp.Resize(fyne.Size{Width: dims.sixthWidth, Height: dims.thirdHeight})
	db.gauges.engineTemp.Move(fyne.Position{X: rightX, Y: rightSideSpacing*2 + dims.thirdHeight})
}

func (db *Dashboard) layoutBars(dims *dims) {
	//Horizontal bars
	cbarHeight := min(dims.tenthHeight, 50)
	cbarSize := fyne.Size{Width: (dims.sixthWidth * 3), Height: cbarHeight}
	cbarX := dims.sixthWidth * 1.5

	db.gauges.nblambda.Resize(cbarSize)
	db.gauges.nblambda.Move(fyne.Position{X: cbarX, Y: 0})

	db.gauges.wblambda.Resize(cbarSize)
	db.gauges.wblambda.Move(fyne.Position{X: cbarX, Y: db.size.Height - cbarHeight})

	// Vertical bars
	vbarSize := fyne.Size{Width: min(dims.sixthWidth*common.OneThird, 70), Height: db.size.Height - 120}
	db.gauges.pwm.Resize(vbarSize)
	db.gauges.pwm.Move(fyne.Position{X: dims.sixthWidth + 8, Y: 25})

	db.gauges.throttle.Resize(vbarSize)
	db.gauges.throttle.Move(fyne.Position{X: db.size.Width - dims.sixthWidth - vbarSize.Width - 8, Y: 25})
}

func (db *Dashboard) layoutIcons(dims *dims) {
	// Limp mode icon
	limpSize := fyne.Size{Width: dims.sixthWidth * .5, Height: dims.thirdHeight * 0.5}
	db.image.limpMode.Resize(limpSize)
	db.image.limpMode.Move(fyne.Position{
		X: dims.centerX - limpSize.Width*0.5,
		Y: dims.centerY - limpSize.Height*0.5 - (dims.thirdHeight * 0.5),
	})

	// Check engine icon
	checkEngineSize := fyne.Size{Width: dims.sixthWidth * 0.5, Height: dims.thirdHeight * 0.5}
	db.image.checkEngine.Resize(checkEngineSize)
	db.image.checkEngine.Move(fyne.Position{
		X: db.size.Width - db.gauges.engineTemp.Size().Width - db.gauges.throttle.Size().Width - checkEngineSize.Width - 15,
		Y: db.size.Height - checkEngineSize.Height - db.gauges.wblambda.Size().Height,
	})

	// Knock icon
	db.image.knockIcon.Move(fyne.Position{
		X: db.gauges.pwm.Position().X + db.gauges.pwm.Size().Width,
		Y: dims.centerY - 60,
	})

	// Taz icon

	tazMin := fyne.Min(dims.sixthWidth, dims.thirdHeight)
	tazSize := fyne.Size{Width: tazMin, Height: tazMin + 16}
	db.image.taz.Resize(tazSize)
	db.image.taz.Move(fyne.Position{
		X: dims.centerX - tazSize.Width*0.58,
		Y: dims.centerY - tazSize.Height,
	})
}

func (db *Dashboard) layoutTexts(dims *dims) {
	// Calculate responsive text sizes based on window dimensions
	// Use the smaller of width/height to ensure text stays proportional
	baseSize := min(db.size.Width, db.size.Height)

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
	db.text.amul.Move(fyne.Position{
		X: db.gauges.wblambda.Position().X,
		Y: db.size.Height - db.gauges.wblambda.Size().Height - db.text.amul.MinSize().Height,
	})

	// IGN text (large)
	db.text.ign.TextSize = dims.textSize
	db.text.ign.Move(fyne.Position{
		X: db.gauges.nblambda.Position().X,
		Y: db.gauges.nblambda.Size().Height,
	})

	// IOFF text (small)
	db.text.ioff.TextSize = dims.smallTextSize
	db.text.ioff.Move(fyne.Position{
		X: db.gauges.nblambda.Position().X,
		Y: db.text.ign.Position().Y + db.text.ign.MinSize().Height,
	})

	// IDC text (large)
	db.text.idc.TextSize = dims.textSize
	db.text.idc.Move(fyne.Position{
		X: db.gauges.nblambda.Position().X + db.gauges.nblambda.Size().Width - (db.text.idc.MinSize().Width - 4),
		Y: db.gauges.nblambda.Size().Height,
	})

	// Active air demand text (large)
	db.text.activeAirDem.TextSize = dims.textSize
	db.text.activeAirDem.Move(fyne.Position{
		X: dims.centerX,
		Y: dims.thirdHeight * 1.24,
	})

	// Cruise text (special size - larger)
	cruiseSize := dims.textSize * 1.1
	db.text.cruise.TextSize = min(max(cruiseSize, 35), 45)
	db.text.cruise.Move(fyne.Position{
		X: dims.sixthWidth * 1.45,
		Y: db.size.Height - (db.image.checkEngine.Size().Height * 0.6) - db.gauges.wblambda.Size().Height,
	})

	// Time text for logplayer (small)
	if db.logplayer {
		db.text.time.TextSize = dims.smallTextSize
		db.text.time.Move(fyne.Position{
			X: dims.centerX - (db.text.time.MinSize().Width * 0.5),
			Y: db.size.Height * common.OneHalfSix,
		})
	}

	//if db.logplayer {
	//	db.text.time.Move(fyne.NewPos(dims.centerX-100, space.Height*common.OneHalfSix))
	//}
}

type DashboardRenderer struct {
	db *Dashboard
}

func (dr *DashboardRenderer) Layout(space fyne.Size) {
	if dr.db.size == space {
		return
	}
	dr.db.size = space

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
	dr.db.layoutBars(dims)

	// Layout dials
	dr.db.layoutDials(dims)

	// Layout buttons
	btnWidth := dims.sixthWidth * 0.8
	btnHeigh := dims.tenthHeight * 0.8
	dr.db.fullscreenBtn.Resize(fyne.NewSize(btnWidth, btnHeigh))
	dr.db.fullscreenBtn.Move(fyne.NewPos(space.Width-btnWidth, space.Height-btnHeigh))

	dims.textSize = dr.db.gauges.nblambda.Size().Height - 2
	dims.smallTextSize = dims.textSize * 0.5

	// Layout text elements
	dr.db.layoutTexts(dims)

	// Layout icons
	dr.db.layoutIcons(dims)
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
