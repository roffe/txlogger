package dashboard

import (
	_ "embed"
	"image/color"
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
	"github.com/roffe/txlogger/pkg/widgets/widebandgauge"
)

//go:embed wheel_left.svg
var wheelLeftIconBytes []byte

//go:embed wheel_right.svg
var wheelRightIconBytes []byte

const rpmIDCconstant = 1.0 / 1200.0

type Dashboard struct {
	cfg *Config

	metricRouter map[string]func(float64)

	text   Texts
	image  Images
	gauges Gauges

	fullscreenBtn *widget.Button
	editBtn       *widget.Button

	// snap-grid layout; see layout.go
	layout *Layout

	// placed/ready gate the metric router: gauges build their canvas objects
	// in CreateRenderer, so feeding one that the layout doesn't render (or
	// that hasn't been laid out yet) panics on nil objects.
	placed map[string]bool
	ready  bool

	// edit mode state; see edit.go
	editMode  bool
	handles   []*itemHandle
	gridLines []fyne.CanvasObject
	toolbar   *fyne.Container

	// gen invalidates the renderer's cached object list on structural change
	gen int

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
	wheelLeft   *canvas.Image
	wheelRight  *canvas.Image
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
	// wbgauge is the round wideband display; the layout's "wblambda" item
	// style picks between it and the wblambda CBar.
	wbgauge *widebandgauge.WidebandGauge
}

type Config struct {
	Logplayer      bool
	AirDemToString func(float64) string
	UseMPH         bool
	ClassicGauges  bool
	// Low/High set the wideband lambda display range (defaults 0.5–1.5).
	Low            float64
	High           float64
	WidebandSymbol string
	FullscreenFunc func(bool)
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

	wbLow, wbHigh := cfg.Low, cfg.High
	if wbLow >= wbHigh {
		wbLow, wbHigh = 0.5, 1.5
	}

	db := &Dashboard{
		cfg:       cfg,
		logplayer: cfg.Logplayer,
		layout:    loadActiveLayout(),
		placed:    make(map[string]bool, len(itemDefs)),
		gauges: Gauges{
			airmass: dualdial.New(&widgets.GaugeConfig{
				Title:   "mg/c",
				Min:     0,
				Max:     2500,
				Steps:   20,
				MinSize: fyne.NewSize(100, 100),
				Classic: cfg.ClassicGauges,
			}),
			speed: dial.New(&widgets.GaugeConfig{
				Title:           speedometerText,
				Min:             0,
				Max:             260,
				Steps:           26,
				DisplayString:   "%.1f",
				GaugeTextString: "%.0f",
				MinSize:         fyne.NewSize(100, 100),
				GaugeFactor:     0.1,
				Classic:         cfg.ClassicGauges,
			}),
			rpm: dial.New(&widgets.GaugeConfig{
				Title:       "RPM",
				Min:         0,
				Max:         8000,
				Steps:       16,
				MinSize:     fyne.NewSize(100, 100),
				GaugeFactor: 0.001,
				Classic:     cfg.ClassicGauges,
			}),
			iat: dial.New(&widgets.GaugeConfig{
				Title:   "IAT",
				Min:     0,
				Max:     80,
				Steps:   16,
				MinSize: fyne.NewSize(100, 100),
				Classic: cfg.ClassicGauges,
			}),
			pressure: dualdial.New(&widgets.GaugeConfig{
				Title:           "MAP",
				Min:             0,
				Max:             3,
				Steps:           30,
				DisplayString:   "%.2f",
				GaugeTextString: "%.1f",
				MinSize:         fyne.NewSize(100, 100),
				Classic:         cfg.ClassicGauges,
			}),
			throttle: vbar.New(&widgets.GaugeConfig{
				Title:      "TPS",
				Min:        0,
				Max:        100,
				Steps:      20,
				MinSize:    fyne.NewSize(50, 50),
				ColorScale: widgets.TraditionalScale,
				Classic:    cfg.ClassicGauges,
			}),
			pwm: vbar.New(&widgets.GaugeConfig{
				Title:      "PWM",
				Min:        0,
				Max:        100,
				Steps:      20,
				MinSize:    fyne.NewSize(50, 50),
				ColorScale: widgets.TraditionalScale,
				Classic:    cfg.ClassicGauges,
			}),
			engineTemp: dial.New(&widgets.GaugeConfig{
				Title:   "tEng",
				Min:     -20,
				Max:     140,
				Steps:   16,
				Classic: cfg.ClassicGauges,
			}),
			wblambda: cbar.New(&widgets.GaugeConfig{
				Title:           "",
				Min:             wbLow,
				Center:          (wbLow + wbHigh) * 0.5,
				Max:             wbHigh,
				Steps:           20,
				MinSize:         fyne.NewSize(50, 35),
				DisplayString:   "λ %.2f",
				DisplayTextSize: 20,
				TextPosition:    widgets.TextAtTop,
				Classic:         cfg.ClassicGauges,
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
				Classic:         cfg.ClassicGauges,
			}),
			wbgauge: widebandgauge.New(&widgets.GaugeConfig{
				Title:         "λ",
				Min:           wbLow,
				Center:        (wbLow + wbHigh) * 0.5,
				Max:           wbHigh,
				Steps:         20,
				DisplayString: "%.2f",
				MinSize:       fyne.NewSize(100, 100),
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
				Color:     color.RGBA{R: 0x6C, G: 0xC4, B: 0x31, A: 0xFF},
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
			knockIcon: icon.New(&icon.Config{
				Image:   canvas.NewImageFromResource(fyne.NewStaticResource("knock.png", assets.KnockBytes)),
				Minsize: fyne.NewSize(90, 90),
			}),
			limpMode:   canvas.NewImageFromResource(fyne.NewStaticResource("limp.png", assets.LimpBytes)),
			taz:        canvas.NewImageFromResource(fyne.NewStaticResource("taz.png", assets.Taz)),
			wheelLeft:  canvas.NewImageFromResource(fyne.NewStaticResource("wheelLeft.svg", wheelLeftIconBytes)),
			wheelRight: canvas.NewImageFromResource(fyne.NewStaticResource("wheelRight.svg", wheelRightIconBytes)),
		},
	}

	db.ExtendBaseWidget(db)

	db.metricRouter = db.createRouter()

	var isFullscreen bool
	db.fullscreenBtn = widget.NewButtonWithIcon("", theme.ViewFullScreenIcon(), func() {
		if db.cfg.FullscreenFunc != nil {
			isFullscreen = !isFullscreen
			db.cfg.FullscreenFunc(isFullscreen)
		}
	})
	db.editBtn = widget.NewButtonWithIcon("", theme.GridIcon(), db.toggleEditMode)

	if db.cfg.Logplayer {
		db.text.time = canvas.NewText("00:00:00.00", color.RGBA{R: 0x2c, G: 0xfc, B: 0x03, A: 0xFF})
		db.text.time.TextSize = 35
	}

	db.image.knockIcon.Hide()
	db.text.cruise.Hide()
	db.image.checkEngine.Hide()
	db.image.limpMode.Hide()
	db.image.taz.Hide()
	db.image.taz.Translucency = 0.75
	db.image.wheelLeft.Hide()
	db.image.wheelRight.Hide()

	db.image.checkEngine.FillMode = canvas.ImageFillContain
	db.image.checkEngine.ScaleMode = canvas.ImageScaleFastest

	db.image.limpMode.FillMode = canvas.ImageFillContain
	db.image.limpMode.ScaleMode = canvas.ImageScaleFastest

	db.image.taz.FillMode = canvas.ImageFillContain
	db.image.taz.ScaleMode = canvas.ImageScaleFastest

	return db
}

func (db *Dashboard) CreateRenderer() fyne.WidgetRenderer {
	return &DashboardRenderer{db: db, gen: -1}
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

// setWBLambda feeds whichever wideband display the layout renders.
func (db *Dashboard) setWBLambda(value float64) {
	if !db.ready || !db.placed["wblambda"] {
		return
	}
	if it := db.layout.item("wblambda"); it != nil && it.Style == StyleGauge {
		db.gauges.wbgauge.SetValue(value)
		return
	}
	db.gauges.wblambda.SetValue(value)
}

// gate wraps a gauge setter so values only reach a widget the layout
// currently renders.
func (db *Dashboard) gate(id string, set func(float64)) func(float64) {
	return func(value float64) {
		if db.ready && db.placed[id] {
			set(value)
		}
	}
}

func interpol(x0, y0, x1, y1, x float64) float64 {
	return y0 + (x-x0)*(y1-y0)/(x1-x0)
}

// itemObject maps a layout item to its canvas object. Returns nil for items
// that don't exist in this instance (the time text outside the log player).
func (db *Dashboard) itemObject(it *Item) fyne.CanvasObject {
	switch it.ID {
	case "rpm":
		return db.gauges.rpm
	case "speed":
		return db.gauges.speed
	case "iat":
		return db.gauges.iat
	case "engineTemp":
		return db.gauges.engineTemp
	case "airmass":
		return db.gauges.airmass
	case "pressure":
		return db.gauges.pressure
	case "throttle":
		return db.gauges.throttle
	case "pwm":
		return db.gauges.pwm
	case "nblambda":
		return db.gauges.nblambda
	case "wblambda":
		if it.Style == StyleGauge {
			return db.gauges.wbgauge
		}
		return db.gauges.wblambda
	case "ign":
		return db.text.ign
	case "ioff":
		return db.text.ioff
	case "idc":
		return db.text.idc
	case "amul":
		return db.text.amul
	case "activeAirDem":
		return db.text.activeAirDem
	case "time":
		if db.text.time == nil {
			return nil
		}
		return db.text.time
	case "cruise":
		return db.text.cruise
	case "checkEngine":
		return db.image.checkEngine
	case "limpMode":
		return db.image.limpMode
	case "knock":
		return db.image.knockIcon
	case "taz":
		return db.image.taz
	}
	return nil
}

func (db *Dashboard) cellSize() (float32, float32) {
	return db.size.Width / float32(db.layout.Cols), db.size.Height / float32(db.layout.Rows)
}

// applyLayout positions every layout item (and the fixed corner buttons)
// from its grid rect. Safe to call after any layout mutation.
func (db *Dashboard) applyLayout() {
	if db.size.Width <= 0 || db.size.Height <= 0 {
		return
	}
	cw, ch := db.cellSize()
	clear(db.placed)
	for i := range db.layout.Items {
		it := &db.layout.Items[i]
		obj := db.itemObject(it)
		if obj == nil {
			continue
		}
		db.placed[it.ID] = true
		// MinSize() creates the widget renderer if it doesn't exist yet, so
		// the gauge's canvas objects are ready before any value arrives.
		obj.MinSize()
		size := fyne.NewSize(float32(it.W)*cw, float32(it.H)*ch)
		if t, ok := obj.(*canvas.Text); ok {
			t.TextSize = common.Clamp(size.Height*0.75, 12, 60)
			// ponytail: empty Align leaves the last applied alignment in
			// place; self-heals the next time the user picks one.
			switch it.Align {
			case AlignLeft:
				t.Alignment = fyne.TextAlignLeading
			case AlignCenter:
				t.Alignment = fyne.TextAlignCenter
			case AlignRight:
				t.Alignment = fyne.TextAlignTrailing
			}
			t.Refresh()
		}
		obj.Resize(size)
		obj.Move(fyne.NewPos(float32(it.X)*cw, float32(it.Y)*ch))
	}

	btnSize := fyne.NewSize(40, 40)
	db.fullscreenBtn.Resize(btnSize)
	db.fullscreenBtn.Move(fyne.NewPos(db.size.Width-btnSize.Width, db.size.Height-btnSize.Height))
	db.editBtn.Resize(btnSize)
	db.editBtn.Move(fyne.NewPos(db.size.Width-btnSize.Width*2-4, db.size.Height-btnSize.Height))

	if db.editMode {
		db.layoutEditOverlay()
	}
	db.ready = true
}

type DashboardRenderer struct {
	db      *Dashboard
	objects []fyne.CanvasObject
	gen     int
}

func (dr *DashboardRenderer) Layout(space fyne.Size) {
	if dr.db.size == space {
		return
	}
	dr.db.size = space
	if dr.db.editMode {
		dr.db.rebuildGridLines()
	}
	dr.db.applyLayout()
}

func (dr *DashboardRenderer) MinSize() fyne.Size {
	return fyne.Size{Width: 480, Height: 300}
}

// Refresh marks the canvas dirty so a changed object list (items added,
// removed or restyled, edit mode toggled) is picked up on the next paint.
// BaseWidget.Refresh only calls this, it does not dirty the canvas itself.
func (dr *DashboardRenderer) Refresh() {
	canvas.Refresh(dr.db)
}

func (dr *DashboardRenderer) Destroy() {
}

func (dr *DashboardRenderer) Objects() []fyne.CanvasObject {
	if dr.gen != dr.db.gen || dr.objects == nil {
		dr.gen = dr.db.gen
		objs := make([]fyne.CanvasObject, 0, len(dr.db.layout.Items)+len(dr.db.gridLines)+len(dr.db.handles)+3)
		for i := range dr.db.layout.Items {
			if obj := dr.db.itemObject(&dr.db.layout.Items[i]); obj != nil {
				objs = append(objs, obj)
			}
		}
		objs = append(objs, dr.db.editBtn, dr.db.fullscreenBtn)
		if dr.db.editMode {
			objs = append(objs, dr.db.gridLines...)
			for _, h := range dr.db.handles {
				objs = append(objs, h)
			}
			objs = append(objs, dr.db.toolbar)
		}
		dr.objects = objs
	}
	return dr.objects
}
