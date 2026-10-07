package multiwindow

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// Tapping a title drops its menu below it, hovering another title switches
// menus, and tapping an item runs it and closes the menu.
func TestMenuBar(t *testing.T) {
	test.NewTempApp(t)

	ran := false
	bar := NewMenuBar(
		fyne.NewMenu("File", fyne.NewMenuItem("Open", nil)),
		fyne.NewMenu("Edit", fyne.NewMenuItem("Undo", func() { ran = true })),
	)
	win := test.NewTempWindow(t, container.NewVBox(bar))
	win.Resize(fyne.NewSize(400, 300))
	file, edit := bar.items[0], bar.items[1]

	test.Tap(file)
	if bar.drop == nil || win.Canvas().Overlays().Top() != bar.drop {
		t.Fatal("tapping File did not open a menu")
	}
	m := bar.drop.menu()
	if below := file.Position().Y + file.Size().Height; m.Position().Y < below {
		t.Fatalf("menu at y=%v, want below the title (y >= %v)", m.Position().Y, below)
	}

	at := fyne.CurrentApp().Driver().AbsolutePositionForObject(edit).AddXY(2, 2)
	bar.drop.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{AbsolutePosition: at}})
	if bar.active != 1 || bar.drop.menu() == m {
		t.Fatalf("hovering Edit: active = %d, want 1 with a new menu", bar.active)
	}

	m = bar.drop.menu()
	test.TapCanvas(win.Canvas(), m.Position().AddXY(10, 10))
	if !ran {
		t.Fatal("tapping Undo did not run its action")
	}
	if bar.drop != nil || win.Canvas().Overlays().Top() != nil {
		t.Fatal("menu still open after triggering an item")
	}

	test.Tap(file)
	test.TapCanvas(win.Canvas(), fyne.NewPos(390, 290))
	if bar.drop != nil {
		t.Fatal("tapping outside did not close the menu")
	}
}
