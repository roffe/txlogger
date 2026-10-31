package customcolors

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

const placeholder = "None set. Add one above or right-click a legend entry in the plotter."

// setup starts a test app with no custom colours (the colors package keeps
// them process wide) and clears whatever the test adds when it ends.
func setup(t *testing.T) {
	t.Helper()
	test.NewTempApp(t)
	reset := func() {
		for _, name := range colors.CustomNames() {
			colors.DeleteCustom(name)
		}
	}
	reset()
	t.Cleanup(reset)
}

// newShown builds the widget in a window.
func newShown(t *testing.T) *Widget {
	t.Helper()
	w := New()
	win := test.NewWindow(w)
	win.Resize(fyne.NewSize(400, 400))
	t.Cleanup(win.Close)
	return w
}

// dialogCanvas is where the picker opens: the app's first window (the
// main window in txlogger, the test driver's dummy window here).
func dialogCanvas() fyne.Canvas {
	return fyne.CurrentApp().Driver().AllWindows()[0].Canvas()
}

type row struct {
	name   string
	swatch color.Color
	edit   *widget.Button
	del    *widget.Button
}

// rows decodes the list: one row per custom colour, or the placeholder
// label (returned as nil rows and its text).
func rows(t *testing.T, w *Widget) ([]row, string) {
	t.Helper()
	var out []row
	for _, o := range w.list.Objects {
		if l, ok := o.(*widget.Label); ok {
			return nil, l.Text
		}
		c := o.(*fyne.Container)
		// Border layout objects: center label, then left swatch, right buttons.
		r := row{name: c.Objects[0].(*widget.Label).Text}
		r.swatch = c.Objects[1].(*fyne.Container).Objects[0].(*canvas.Rectangle).FillColor
		btns := c.Objects[2].(*fyne.Container).Objects
		r.edit, r.del = btns[0].(*widget.Button), btns[1].(*widget.Button)
		out = append(out, r)
	}
	return out, ""
}

// pickerShown reports whether the colour picker dialog is the top overlay.
func pickerShown() bool {
	top := dialogCanvas().Overlays().Top()
	if top == nil {
		return false
	}
	for _, o := range test.LaidOutObjects(top) {
		if l, ok := o.(*widget.Label); ok && l.Text == "Custom color" {
			return true
		}
	}
	return false
}

// tapOverlayButton taps the button with the given text in the top overlay.
func tapOverlayButton(t *testing.T, text string) {
	t.Helper()
	for _, o := range test.LaidOutObjects(dialogCanvas().Overlays().Top()) {
		if b, ok := o.(*widget.Button); ok && b.Text == text {
			test.Tap(b)
			return
		}
	}
	t.Fatalf("no %q button in the overlay", text)
}

// addButton is the button next to the name entry; Border keeps the centre
// objects first, so the top bar follows the list.
func addButton(w *Widget) *widget.Button {
	top := w.container.Objects[1].(*fyne.Container)
	return top.Objects[1].(*widget.Button)
}

// With no custom colours the list explains how to add one.
func TestEmptyShowsPlaceholder(t *testing.T) {
	setup(t)
	w := newShown(t)
	if r, text := rows(t, w); r != nil || text != placeholder {
		t.Fatalf("rows %v text %q, want the placeholder", r, text)
	}
	if got := w.MinSize(); got.Width < 300 || got.Height < 200 {
		t.Errorf("MinSize %v, want at least 300x200", got)
	}
}

// Existing custom colours are listed by name in sorted order, each with a
// swatch of its colour.
func TestListsCustomColors(t *testing.T) {
	setup(t)
	colors.SetCustom("RPM", color.RGBA{1, 2, 3, 255})
	colors.SetCustom("Boost", color.RGBA{4, 5, 6, 255})
	w := newShown(t)

	got, _ := rows(t, w)
	if len(got) != 2 {
		t.Fatalf("%d rows, want 2", len(got))
	}
	if got[0].name != "Boost" || got[0].swatch != (color.RGBA{4, 5, 6, 255}) {
		t.Errorf("row 0: %s %v", got[0].name, got[0].swatch)
	}
	if got[1].name != "RPM" || got[1].swatch != (color.RGBA{1, 2, 3, 255}) {
		t.Errorf("row 1: %s %v", got[1].name, got[1].swatch)
	}
}

// The list follows changes made elsewhere (e.g. from the plotter legend).
func TestListFollowsExternalChanges(t *testing.T) {
	setup(t)
	w := newShown(t)
	colors.SetCustom("IAT", color.RGBA{9, 9, 9, 255})
	if got, _ := rows(t, w); len(got) != 1 || got[0].name != "IAT" {
		t.Fatalf("rows %v after SetCustom, want IAT", got)
	}
	colors.DeleteCustom("IAT")
	if _, text := rows(t, w); text != placeholder {
		t.Fatalf("text %q after DeleteCustom, want the placeholder", text)
	}
}

// Add with an empty name does nothing; with a name it stores the name's
// current plot colour as a custom one, clears the entry, lists it and opens
// the picker for it.
func TestAddButton(t *testing.T) {
	setup(t)
	w := newShown(t)

	test.Tap(addButton(w))
	if names := colors.CustomNames(); len(names) != 0 {
		t.Fatalf("empty name added %v", names)
	}
	if pickerShown() {
		t.Fatal("picker opened for an empty name")
	}

	want := colors.GetColor("Boost")
	test.Type(w.nameEntry, "Boost")
	test.Tap(addButton(w))

	if names := colors.CustomNames(); !slices.Equal(names, []string{"Boost"}) {
		t.Fatalf("custom names %v, want [Boost]", names)
	}
	if got := colors.GetColor("Boost"); got != want {
		t.Errorf("stored colour %v, want the default %v", got, want)
	}
	if w.nameEntry.Text != "" {
		t.Errorf("entry still holds %q", w.nameEntry.Text)
	}
	if got, _ := rows(t, w); len(got) != 1 || got[0].name != "Boost" || got[0].swatch != want {
		t.Errorf("rows %v, want Boost with its colour", got)
	}
	if !pickerShown() {
		t.Fatal("picker not opened after adding")
	}
	tapOverlayButton(t, "Confirm")
	if pickerShown() {
		t.Error("picker still shown after Confirm")
	}
	if got := colors.GetColor("Boost"); got != want {
		t.Errorf("colour %v after confirming the unchanged picker, want %v", got, want)
	}
}

// Pressing Return in the name entry adds the name like the Add button.
func TestAddOnSubmit(t *testing.T) {
	setup(t)
	w := newShown(t)
	test.Type(w.nameEntry, "Lambda")
	w.nameEntry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if names := colors.CustomNames(); !slices.Equal(names, []string{"Lambda"}) {
		t.Fatalf("custom names %v, want [Lambda]", names)
	}
	if !pickerShown() {
		t.Fatal("picker not opened after submitting")
	}
}

// The edit button opens the picker preset to the entry's current colour, so
// confirming without changes keeps it.
func TestEditOpensPickerWithCurrentColor(t *testing.T) {
	setup(t)
	custom := color.RGBA{0x12, 0x34, 0x56, 0xFF}
	colors.SetCustom("RPM", custom)
	w := newShown(t)

	got, _ := rows(t, w)
	test.Tap(got[0].edit)
	if !pickerShown() {
		t.Fatal("picker not opened by the edit button")
	}
	tapOverlayButton(t, "Confirm")
	if c := colors.GetColor("RPM"); c != custom {
		t.Errorf("colour %v after confirming, want the preset %v", c, custom)
	}
}

// The delete button forgets the entry's custom colour and drops its row.
func TestDeleteButton(t *testing.T) {
	setup(t)
	colors.SetCustom("RPM", color.RGBA{1, 1, 1, 255})
	colors.SetCustom("Speed", color.RGBA{2, 2, 2, 255})
	w := newShown(t)

	got, _ := rows(t, w)
	test.Tap(got[0].del) // RPM
	if names := colors.CustomNames(); !slices.Equal(names, []string{"Speed"}) {
		t.Fatalf("custom names %v, want [Speed]", names)
	}
	if got, _ := rows(t, w); len(got) != 1 || got[0].name != "Speed" {
		t.Errorf("rows %v, want only Speed", got)
	}
}
