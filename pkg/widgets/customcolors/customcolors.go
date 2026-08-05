package customcolors

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

// Widget lists the user's custom plot color mappings and lets them add,
// edit and delete entries.
type Widget struct {
	widget.BaseWidget

	nameEntry *widget.Entry
	list      *fyne.Container
	container *fyne.Container
}

func New() *Widget {
	w := &Widget{list: container.NewVBox()}
	w.ExtendBaseWidget(w)

	w.nameEntry = widget.NewEntry()
	w.nameEntry.PlaceHolder = "Value name"
	add := func() {
		name := w.nameEntry.Text
		if name == "" {
			return
		}
		colors.SetCustom(name, colors.GetColor(name))
		w.nameEntry.SetText("")
		w.showPicker(name)
	}
	w.nameEntry.OnSubmitted = func(string) { add() }

	w.container = container.NewBorder(
		container.NewBorder(nil, nil, nil, widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), add), w.nameEntry),
		nil, nil, nil,
		w.list,
	)

	w.rebuild()
	colors.OnChange(w.rebuild) // ponytail: listener can't be unregistered, one leaks per widget instance (same as the old window)
	return w
}

func (w *Widget) rebuild() {
	w.list.RemoveAll()
	names := colors.CustomNames()
	if len(names) == 0 {
		w.list.Add(widget.NewLabel("None set. Add one above or right-click a legend entry in the plotter."))
	}
	for _, name := range names {
		swatch := canvas.NewRectangle(colors.GetColor(name))
		swatch.SetMinSize(fyne.NewSize(24, 18))
		edit := widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
			w.showPicker(name)
		})
		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			colors.DeleteCustom(name)
		})
		w.list.Add(container.NewBorder(nil, nil,
			container.NewCenter(swatch),
			container.NewHBox(edit, del),
			widget.NewLabel(name),
		))
	}
	w.list.Refresh()
}

func (w *Widget) showPicker(name string) {
	picker := dialog.NewColorPicker("Custom color", name, func(c color.Color) {
		r, g, b, a := c.RGBA()
		colors.SetCustom(name, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)})
	}, fyne.CurrentApp().Driver().AllWindows()[0])
	picker.Advanced = true
	picker.SetColor(colors.GetColor(name))
	picker.Show()
}

func (w *Widget) MinSize() fyne.Size {
	return w.BaseWidget.MinSize().Max(fyne.NewSize(300, 200))
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.container)
}
