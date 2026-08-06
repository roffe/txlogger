// Command screenshots renders txlogger widgets headless (Fyne test driver,
// software rasterizer) and writes JPEGs for the webpage gallery.
//
//	go run -tags combi,canusb,ftdi,j2534,pcan,rcan,socketcan ./cmd/screenshots \
//	    -out ../txlogger-webpage/static/screenshots -log some.t7l -bin some.bin
package main

import (
	"flag"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	symbol "github.com/roffe/ecusymbol"
	"github.com/roffe/txlogger/pkg/datalogger"
	"github.com/roffe/txlogger/pkg/ebus"
	"github.com/roffe/txlogger/pkg/logfile"
	"github.com/roffe/txlogger/pkg/theme"
	"github.com/roffe/txlogger/pkg/widgets/dashboard"
	"github.com/roffe/txlogger/pkg/widgets/logplayer"
	"github.com/roffe/txlogger/pkg/widgets/mapviewer"
	"github.com/roffe/txlogger/pkg/widgets/meshgrid"
	"github.com/roffe/txlogger/pkg/widgets/plotter"
	"github.com/roffe/txlogger/pkg/widgets/settings"
	"github.com/roffe/txlogger/pkg/widgets/symbollist"
)

var (
	outDir  = flag.String("out", "screenshots", "directory images are written to")
	logFile = flag.String("log", "", "log file for the log player screenshot (skipped if empty)")
	binFile = flag.String("bin", "", "ECU binary for real map viewer data (synthetic if empty)")
)

func main() {
	flag.Parse()
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	a := test.NewApp()
	a.Settings().SetTheme(&theme.TxTheme{})

	snap("settings.jpg", fyne.NewSize(1000, 720), settings.New(&settings.Config{
		Logger:          func(string) {},
		SelectedEcuFunc: func() string { return "T7" },
	}), nil)

	snap("symbollist.jpg", fyne.NewSize(560, 640), symbolList(), func() {
		for k, v := range demoValues {
			ebus.Publish(k, v)
		}
		time.Sleep(300 * time.Millisecond) // ebus delivery is async
	})

	db := dashboard.NewDashboard(&dashboard.Config{
		WidebandSymbol: "DisplProt.LambdaScanner",
		AirDemToString: datalogger.AirDemToStringT7,
	})
	snap("dashboard.jpg", fyne.NewSize(1280, 720), db, func() {
		for k, v := range demoValues {
			db.SetValue(k, v)
		}
		db.SetTime(time.Now())
	})

	snapMapViewer()

	if *logFile != "" {
		snapLogplayer(*logFile)
	}
}

func snapMapViewer() {
	var cfg *mapviewer.Config
	if *binFile != "" {
		cfg = mapViewerConfigFromBin(*binFile)
	} else {
		cfg = syntheticMapViewerConfig()
	}
	cfg.MeshView = true
	cfg.MeshRenderer = meshgrid.BackendImage // no GPU headless
	mv, err := mapviewer.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	snap("mapviewer.jpg", fyne.NewSize(1024, 768), mv, func() {
		time.Sleep(300 * time.Millisecond) // mesh redraw is throttled via fyne.Do
	})
}

// mapViewerConfigFromBin loads the main fuel map with real axes from an ECU binary.
func mapViewerConfigFromBin(path string) *mapviewer.Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	ecuType, fw, err := symbol.Load(path, data, func(string) {})
	if err != nil {
		log.Fatal(err)
	}
	var mapName string
	switch ecuType {
	case symbol.ECU_T5:
		mapName = "Insp_mat!"
	case symbol.ECU_T8:
		mapName = "BFuelCal.LambdaOneFacMap"
	default:
		mapName = "BFuelCal.Map"
	}
	axis := symbol.GetInfo(ecuType, mapName)
	symX := fw.GetByName(axis.X)
	symY := fw.GetByName(axis.Y)
	symZ := fw.GetByName(axis.Z)
	if symX == nil || symY == nil || symZ == nil {
		log.Fatalf("%s: missing symbols for %s", path, mapName)
	}
	return &mapviewer.Config{
		Name:       axis.Z,
		XData:      symX.Float64s(),
		YData:      symY.Float64s(),
		ZData:      symZ.Float64s(),
		XPrecision: symbol.GetPrecision(symX.Correctionfactor),
		YPrecision: symbol.GetPrecision(symY.Correctionfactor),
		ZPrecision: symbol.GetPrecision(symZ.Correctionfactor),
		XLabel:     axis.XDescription,
		YLabel:     axis.YDescription,
		ZLabel:     axis.ZDescription,
	}
}

// syntheticMapViewerConfig renders a synthetic BFuelCal.Map-style fuel enrichment
// map: 12 rpm columns x 12 airmass rows, enrichment rising with load and rpm.
func syntheticMapViewerConfig() *mapviewer.Config {
	const cols, rows = 12, 12
	xData := make([]float64, cols)
	yData := make([]float64, rows)
	zData := make([]float64, cols*rows)
	for i := range xData {
		xData[i] = 720 + float64(i)*480 // rpm
	}
	for i := range yData {
		yData[i] = 100 + float64(i)*121 // mg/c
	}
	for r := range rows {
		for c := range cols {
			load := float64(r) / (rows - 1)
			rpm := float64(c) / (cols - 1)
			ripple := 0.01 * float64((r*7+c*13)%5)
			zData[r*cols+c] = 0.85 + 0.35*load*load*(0.6+0.4*rpm) + ripple
		}
	}
	return &mapviewer.Config{
		Name:       "BFuelCal.Map",
		XData:      xData,
		YData:      yData,
		ZData:      zData,
		XPrecision: 0,
		YPrecision: 0,
		ZPrecision: 2,
		XLabel:     "Rpm",
		YLabel:     "mg/c",
		ZLabel:     "Fuel enrichment",
	}
}

// demoValues are T7-style symbol values used for both the dashboard and the
// symbol list so the two screenshots agree.
var demoValues = map[string]float64{
	"ActualIn.n_Engine":          3240,
	"In.v_Vehicle":               112, // datalogger publishes corrected values
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
	"ECMStat.ST_ActiveAirDem":    10, // PedalMap
}

func symbolList() fyne.CanvasObject {
	syms := []*symbol.Symbol{
		{Name: "ActualIn.n_Engine", Number: 1, Unit: "rpm", Correctionfactor: 1},
		{Name: "In.v_Vehicle", Number: 2, Unit: "km/h", Correctionfactor: 0.1},
		{Name: "ActualIn.T_AirInlet", Number: 3, Unit: "°C", Correctionfactor: 1},
		{Name: "ActualIn.T_Engine", Number: 4, Unit: "°C", Correctionfactor: 1},
		{Name: "In.p_AirInlet", Number: 5, Unit: "bar", Correctionfactor: 0.01},
		{Name: "MAF.m_AirInlet", Number: 6, Unit: "mg/c", Correctionfactor: 1},
		{Name: "Out.X_AccPedal", Number: 7, Unit: "%", Correctionfactor: 0.1},
		{Name: "Out.PWM_BoostCntrl", Number: 8, Unit: "%", Correctionfactor: 1},
		{Name: "Out.fi_Ignition", Number: 9, Unit: "°", Correctionfactor: 0.1},
		{Name: "Lambda.LambdaInt", Number: 10, Unit: "", Correctionfactor: 1},
		{Name: "Myrtilos.InjectorDutyCycle", Number: 11, Unit: "%", Correctionfactor: 1},
	}
	return symbollist.New(&symbollist.Config{Symbols: syms})
}

func snapLogplayer(path string) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	lf, err := logfile.Open(path, f)
	if err != nil {
		log.Fatal(err)
	}
	lp := logplayer.New(&logplayer.Config{
		EBus:            ebus.CONTROLLER,
		Logfile:         lf,
		PlotterRenderer: plotter.PlotBackendImage, // no GPU headless
	})
	defer lp.Close()
	snap("logplayer.jpg", fyne.NewSize(1280, 720), lp, func() {
		// seek into the log so the legend shows real values, not zeros
		for range 5 {
			lp.TypedKey(&fyne.KeyEvent{Name: fyne.KeyPageUp})
		}
		time.Sleep(500 * time.Millisecond)
	})
}

func snap(name string, size fyne.Size, obj fyne.CanvasObject, after func()) {
	w := test.NewWindow(obj)
	defer w.Close()
	w.Resize(size)
	if after != nil {
		after()
	}
	out := filepath.Join(*outDir, name)
	fh, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer fh.Close()
	if err := jpeg.Encode(fh, w.Canvas().Capture(), &jpeg.Options{Quality: 90}); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote", out)
}
