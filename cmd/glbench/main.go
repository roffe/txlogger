// Command glbench drives txlogger's busiest widgets with live values in a real
// GL window, so renderer changes can be measured before and after without an
// ECU attached. The default view is a typical session: the symbol list, the
// dashboard and three map viewers as inner windows on a 1920x1080 desktop.
//
//	FYNE_GL_DEBUG=1 go run ./cmd/glbench -cpuprofile cpu.out -shot frame.png
//	go tool pprof -top -cum cpu.out
//
// FYNE_GL_DEBUG makes the GL painter log draw calls, paint time and texture
// churn every 120 frames. It measures whichever fyne the build resolves; add
// `replace fyne.io/fyne/v2 => ../fyne` to go.work to measure a local checkout.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"runtime/pprof"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/ebus"
	txtheme "github.com/roffe/txlogger/pkg/theme"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/multiwindow"
	"github.com/roffe/txlogger/pkg/widgets/symbollist"
)

var (
	view       = flag.String("view", "real", "real (symbol list, dashboard and -maps map viewers as inner windows), dashboard, symbols, map or all")
	maps       = flag.Int("maps", 3, "map viewers in the real view")
	dur        = flag.Duration("d", 20*time.Second, "how long to run after a 3s warmup")
	hz         = flag.Int("hz", 50, "value updates per second, like a logger's sample rate")
	cpuprofile = flag.String("cpuprofile", "", "write a CPU profile of the run here")
	shot       = flag.String("shot", "", "write a PNG capture of the window here at the end of the run")
)

// demoValues are the T7 symbols the dashboard shows, each with a typical value
// the run swings around.
var demoValues = map[string]float64{
	"ActualIn.n_Engine":          3240,
	"In.v_Vehicle":               112,
	"ActualIn.T_AirInlet":        31,
	"ActualIn.T_Engine":          92,
	"In.p_AirInlet":              1.42,
	"In.p_AirBefThrottle":        1.55,
	"Out.X_AccPedal":             73,
	"Out.PWM_BoostCntrl":         61,
	"MAF.m_AirInlet":             1240,
	"m_Request":                  1180,
	"Lambda.LambdaInt":           3,
	"Out.fi_Ignition":            17.4,
	"AdpFuelProt.MulFuelAdapt":   2.1,
	"IgnProt.fi_Offset":          1.5,
	"Myrtilos.InjectorDutyCycle": 54,
	"DisplProt.LambdaScanner":    0.87,
}

// updates are called from the ticker goroutine, as ebus subscribers are.
var updates []func(phase float64)

func main() {
	flag.Parse()
	// txlogger declares the fyneDo migration in FyneApp.toml, which switches
	// off fyne's per-call thread checks. Without it those checks cost about a
	// third of the CPU and swamp everything this is meant to measure, so it is
	// set here rather than depending on where the binary happens to run from.
	app.SetMetadata(fyne.AppMetadata{
		ID:         "com.roffe.txlogger.glbench",
		Name:       "glbench",
		Migrations: map[string]bool{"fyneDo": true},
	})
	a := app.NewWithID("com.roffe.txlogger.glbench") // not txlogger's ID: keep its preferences out of this
	a.Settings().SetTheme(&txtheme.TxTheme{})
	w := a.NewWindow("glbench " + *view)

	var content fyne.CanvasObject
	var arrange func()
	size := fyne.NewSize(1600, 900)
	switch *view {
	case "dashboard":
		content = newDashboard()
	case "symbols":
		content = newSymbolList(a, w)
	case "map":
		content = newMapViewer(fuelMap("BFuelCal.Map", 18, 16), 0)
	case "all":
		content = container.NewHSplit(newDashboard(),
			container.NewVSplit(newSymbolList(a, w), newMapViewer(fuelMap("BFuelCal.Map", 18, 16), 0)))
	case "real":
		wm := multiwindow.NewMultipleWindows()
		wm.LockViewport = true
		w.SetContent(wm) // Add needs the desktop's renderer, as in the main window
		sl := multiwindow.NewSystemWindow("Symbol list", newSymbolList(a, w))
		sl.Icon = theme.ListIcon()
		wm.Add(sl)
		wm.Add(multiwindow.NewInnerWindow("Dashboard", newDashboard()))
		tables := []*mapviewer.Config{
			fuelMap("BFuelCal.Map", 18, 16),
			fuelMap("IgnNormCal.Map", 18, 16),
			fuelMap("BoostCal.RegMap", 16, 12),
		}
		for i := range *maps {
			cfg := tables[i%len(tables)]
			if i >= len(tables) {
				cfg = fuelMap(fmt.Sprintf("%s#%d", cfg.Name, i), len(cfg.XData), len(cfg.YData))
			}
			mw := multiwindow.NewInnerWindow(cfg.Name+" - "+cfg.ZLabel, newMapViewer(cfg, float64(i)))
			mw.Icon = theme.GridIcon()
			wm.Add(mw)
		}
		content = wm
		size = fyne.NewSize(1920, 1080)
		arrange = func() { wm.Arrange(&multiwindow.GridArranger{}) }
	default:
		log.Fatalf("unknown view %q", *view)
	}
	w.SetContent(content)
	w.Resize(size)

	go func() {
		if arrange != nil {
			time.Sleep(time.Second) // the arranger needs the laid out desktop size
			fyne.DoAndWait(arrange)
			time.Sleep(2 * time.Second)
		} else {
			time.Sleep(3 * time.Second) // let the window map and the caches fill
		}
		if *cpuprofile != "" {
			f, err := os.Create(*cpuprofile)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				log.Fatal(err)
			}
			defer pprof.StopCPUProfile()
		}
		t := time.NewTicker(time.Second / time.Duration(*hz))
		defer t.Stop()
		end := time.Now().Add(*dur)
		for now := range t.C {
			if now.After(end) {
				break
			}
			ph := float64(now.UnixNano()) / 1e9 * 2
			for _, u := range updates {
				u(ph)
			}
		}
		if *shot != "" {
			// One fixed final state, left to settle, so captures from two builds
			// can be compared pixel for pixel. Capture reads the front buffer,
			// which some drivers leave a frame behind, so the settled state is
			// painted into both buffers before it is read.
			for _, u := range updates {
				u(0)
			}
			time.Sleep(time.Second)
			for range 2 {
				fyne.DoAndWait(content.Refresh)
				time.Sleep(200 * time.Millisecond)
			}
			var img image.Image
			fyne.DoAndWait(func() { img = w.Canvas().Capture() })
			if err := writePNG(*shot, img); err != nil {
				log.Println(err)
			}
		}
		fyne.Do(a.Quit)
	}()
	w.ShowAndRun()
}

func newDashboard() fyne.CanvasObject {
	db := dashboard.NewDashboard(&dashboard.Config{WidebandSymbol: "DisplProt.LambdaScanner"})
	updates = append(updates, func(ph float64) {
		for k, v := range demoValues {
			db.SetValue(k, v*(1+0.3*math.Sin(ph+v)))
		}
		db.SetTime(time.Now())
	})
	return db
}

func newSymbolList(a fyne.App, w fyne.Window) fyne.CanvasObject {
	v := symbollist.NewViewer(&symbollist.ViewerConfig{
		App:    a,
		Window: w,
		ECU:    func() string { return "T7" },
		Log:    func(string) {},
		Error:  func(err error) { log.Println(err) },
	})
	syms := make([]*symbol.Symbol, 60)
	for i := range syms {
		syms[i] = &symbol.Symbol{Name: fmt.Sprintf("Bench.Sym%02d", i), Number: i + 1, Unit: "%", Correctionfactor: 1}
	}
	v.LoadSymbols(syms...)
	v.UpdateBars(true)
	updates = append(updates, func(ph float64) {
		for i, s := range syms {
			ebus.Publish(s.Name, 50+50*math.Sin(ph+float64(i)))
		}
	})
	return v
}

// newMapViewer builds a map viewer the way the main window's openMap does,
// with its crosshair swept across the table at a per-map phase offset.
func newMapViewer(cfg *mapviewer.Config, offset float64) fyne.CanvasObject {
	nop := func() {}
	cfg.LoadFileFunc, cfg.LoadECUFunc = nop, nop
	cfg.SaveFileFunc, cfg.SaveECUFunc = func([]float64) {}, func([]float64) {}
	cfg.Editable = true
	cfg.Buttons = []*mapviewer.MapViewerButton{
		{Label: "Load File", Icon: theme.DocumentIcon(), OnTapped: nop},
		{Label: "Save File", Icon: theme.DocumentSaveIcon(), OnTapped: nop},
		{Label: "Load ECU", Icon: theme.DownloadIcon(), OnTapped: nop},
		{Label: "Save ECU", Icon: theme.UploadIcon(), OnTapped: nop},
	}
	mv, err := mapviewer.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	xs, ys := cfg.XData, cfg.YData
	updates = append(updates, func(ph float64) {
		mv.SetX(xs[0] + (xs[len(xs)-1]-xs[0])*(0.5+0.5*math.Sin(ph*0.7+offset)))
		mv.SetY(ys[0] + (ys[len(ys)-1]-ys[0])*(0.5+0.5*math.Cos(ph*0.5+offset)))
	})
	return mv
}

// fuelMap is a synthetic BFuelCal.Map-style table: enrichment rising with
// load and rpm, with a little ripple so neighbouring cells differ.
func fuelMap(name string, cols, rows int) *mapviewer.Config {
	x := make([]float64, cols)
	y := make([]float64, rows)
	z := make([]float64, cols*rows)
	for i := range x {
		x[i] = 720 + float64(i)*400
	}
	for i := range y {
		y[i] = 100 + float64(i)*90
	}
	for r := range rows {
		for c := range cols {
			load := float64(r) / float64(rows-1)
			rpm := float64(c) / float64(cols-1)
			z[r*cols+c] = 0.85 + 0.35*load*load*(0.6+0.4*rpm) + 0.01*float64((r*7+c*13)%5)
		}
	}
	return &mapviewer.Config{
		Name: name, XData: x, YData: y, ZData: z,
		ZPrecision: 2, XLabel: "Rpm", YLabel: "mg/c", ZLabel: "Fuel enrichment",
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
