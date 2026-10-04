package dashboard

import (
	"slices"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/roffe/txlogger/pkg/widgets/internal/uitest"
)

// Gauges build their canvas objects in CreateRenderer. Feeding the dashboard
// before its first layout, or with items removed, must not panic on nil canvas
// objects (CBar.applyBar was the reported crash). The router's gate also keeps
// values away from gauges the layout doesn't render.
func TestFeedingUnrenderedGaugesIsSafe(t *testing.T) {
	test.NewApp()
	const wbl = "Lambda.External"

	feedEverything := func(db *Dashboard) {
		for _, name := range db.GetMetricNames() {
			db.SetValue(name, 1.02)
		}
	}

	t.Run("before first layout", func(t *testing.T) {
		db := NewDashboard(&Config{WidebandSymbol: wbl})
		feedEverything(db) // no layout pass yet: nothing has a renderer
	})

	t.Run("wideband styles", func(t *testing.T) {
		db := NewDashboard(&Config{WidebandSymbol: wbl})
		win := test.NewWindow(db)
		win.SetPadded(false)
		win.Resize(fyne.NewSize(800, 600))

		for _, style := range []string{StyleBar, StyleGauge, StyleBar} {
			db.setWBLStyle(style)
			db.SetValue(wbl, 1.02)
		}
	})

	t.Run("after removing every item", func(t *testing.T) {
		db := NewDashboard(&Config{WidebandSymbol: wbl})
		win := test.NewWindow(db)
		win.SetPadded(false)
		win.Resize(fyne.NewSize(800, 600))
		feedEverything(db)

		for _, def := range itemDefs {
			db.removeItem(def.id)
		}
		if len(db.layout.Items) != 0 {
			t.Fatalf("%d items left after removing all", len(db.layout.Items))
		}
		feedEverything(db)
	})

	t.Run("re-added items render again", func(t *testing.T) {
		db := NewDashboard(&Config{WidebandSymbol: wbl})
		win := test.NewWindow(db)
		win.SetPadded(false)
		win.Resize(fyne.NewSize(800, 600))

		db.removeItem("rpm")
		if db.placed["rpm"] {
			t.Error("rpm still marked placed after removal")
		}
		db.addItem(*itemDefByID("rpm"))
		if !db.placed["rpm"] {
			t.Error("rpm not marked placed after being added back")
		}
		db.SetValue("ActualIn.n_Engine", 3000)
	})
}

// The live dashboard has no time text, so its item must never be placed,
// handled or offered in the Add menu there.
func TestTimeItemOnlyInLogplayer(t *testing.T) {
	test.NewApp()

	live := NewDashboard(&Config{})
	if live.itemObject(&Item{ID: "time"}) != nil {
		t.Error("live dashboard exposes a time object")
	}
	win := test.NewWindow(live)
	win.Resize(fyne.NewSize(800, 600))
	if live.placed["time"] {
		t.Error("time item placed on the live dashboard")
	}

	player := NewDashboard(&Config{Logplayer: true})
	if player.itemObject(&Item{ID: "time"}) == nil {
		t.Error("log player dashboard has no time object")
	}
}

func TestEditModeSwapsOverlayObjects(t *testing.T) {
	test.NewApp()
	db := NewDashboard(&Config{})
	win := test.NewWindow(db)
	win.SetPadded(false)
	win.Resize(fyne.NewSize(800, 600))

	renderer := test.WidgetRenderer(db)
	before := len(renderer.Objects())

	db.toggleEditMode()
	if !db.editMode {
		t.Fatal("edit mode did not turn on")
	}
	during := len(renderer.Objects())
	if during <= before {
		t.Errorf("object count %d in edit mode, want more than %d", during, before)
	}
	if len(db.handles) == 0 || len(db.gridLines) == 0 {
		t.Errorf("edit overlay incomplete: %d handles, %d grid lines", len(db.handles), len(db.gridLines))
	}

	db.toggleEditMode()
	if db.editMode {
		t.Fatal("edit mode did not turn off")
	}
	if after := len(renderer.Objects()); after != before {
		t.Errorf("object count %d after leaving edit mode, want %d", after, before)
	}
}

// Publishers feed the dashboard from their own goroutines while the UI
// goroutine re-lays it out. Run with -race.
func TestSetValueWhileRelayingOut(t *testing.T) {
	onUI := uitest.Start(t)

	var db *Dashboard
	var win fyne.Window
	onUI(func() {
		db = NewDashboard(&Config{WidebandSymbol: "Lambda.External"})
		win = test.NewWindow(db)
		win.SetPadded(false)
		win.Resize(fyne.NewSize(800, 600))
	})

	names := db.GetMetricNames()
	var wg sync.WaitGroup
	for p := range 4 {
		wg.Go(func() {
			for i := range 300 {
				v := float64((i+p)%50 - 10)
				for _, name := range names {
					switch name {
					case "KnkDet.KnockCyl", "Knock_offset1234":
						db.SetValue(name, 0) // a knock arms a 5 s hide timer
					default:
						db.SetValue(name, v)
					}
				}
			}
		})
	}

	for i, style := range []string{StyleBar, StyleGauge, StyleBar, StyleGauge} {
		onUI(func() {
			win.Resize(fyne.NewSize(800+float32(i)*40, 600-float32(i)*30))
			db.setWBLStyle(style)
			db.removeItem("rpm")
			db.addItem(*itemDefByID("rpm"))
		})
	}
	wg.Wait()

	db.SetValue("Out.fi_Ignition", 12.3)
	onUI(func() {}) // flush the posts queued so far
	var got string
	onUI(func() { got = db.text.ign.Text })
	if got != "Ign: 12.3" {
		t.Errorf("ign text %q, want %q", got, "Ign: 12.3")
	}
}

// The router is built from the ECU profiles' signals; this is every symbol
// the hand-written per-ECU router fed before, so none silently loses its
// gauge.
func TestRouterCoversProfileSymbols(t *testing.T) {
	test.NewApp()
	db := NewDashboard(&Config{WidebandSymbol: "Lambda.External"})
	names := db.GetMetricNames()
	for _, sym := range []string{
		"In.v_Vehicle", "Bil_hast", "ActualIn.n_Engine", "Rpm",
		"ActualIn.T_AirInlet", "Lufttemp", "ActualIn.T_Engine", "Kyl_temp",
		"P_medel", "In.p_AirInlet", "ActualIn.p_AirInlet",
		"Max_tryck", "In.p_AirBefThrottle", "ActualIn.p_AirBefThrottle",
		"Medeltrot", "Out.X_AccPedal", "Out.X_AccPos",
		"Out.PWM_BoostCntrl", "PWM_ut10", "AdpFuelProt.MulFuelAdapt",
		"Lambda.LambdaInt", "Lambdaint",
		"MAF.m_AirInlet", "m_Request", "AirMassMast.m_Request",
		"Out.fi_Ignition", "Ign_angle", "ECMStat.ST_ActiveAirDem",
		"IgnProt.fi_Offset", "IgnMastProt.fi_Offset",
		"CRUISE", "CEL", "LIMP", "Knock_offset1234", "KnkDet.KnockCyl",
		"Myrtilos.InjectorDutyCycle", "Insptid_ms10", "Lambda.External",
	} {
		if !slices.Contains(names, sym) {
			t.Errorf("%s is not routed", sym)
		}
	}
}
