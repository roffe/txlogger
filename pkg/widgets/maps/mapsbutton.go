package maps

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type mapButton struct {
	widget.Button
}

func newMapButton(icon fyne.Resource, f func()) *mapButton {
	b := &mapButton{}
	b.ExtendBaseWidget(b)

	b.Icon = icon
	b.OnTapped = f
	return b
}

func (b *mapButton) CreateRenderer() fyne.WidgetRenderer {
	return &mapButtonRenderer{WidgetRenderer: b.Button.CreateRenderer(),
		bg: canvas.NewRectangle(theme.Color(theme.ColorNameShadow))}
}

type mapButtonRenderer struct {
	fyne.WidgetRenderer

	bg *canvas.Rectangle
}

func (r *mapButtonRenderer) Layout(s fyne.Size) {
	halfPad := theme.Padding() / 2
	r.bg.Move(fyne.NewPos(halfPad, halfPad))
	r.bg.Resize(s.Subtract(fyne.NewSize(theme.Padding(), theme.Padding())))

	r.WidgetRenderer.Layout(s)
}

func (r *mapButtonRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.bg}, r.WidgetRenderer.Objects()...)
}

func (r *mapButtonRenderer) Refresh() {
	r.bg.FillColor = theme.Color(theme.ColorNameShadow)
	r.WidgetRenderer.Refresh()
}
