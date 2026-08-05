package dashboard

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// Gauges build their canvas objects in CreateRenderer, so the metric router
// must never reach a gauge the layout doesn't render — feeding one panics on
// nil canvas objects (CBar.applyBar was the reported crash).
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
