package ledicon

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Widget struct {
	widget.BaseWidget

	Text string

	ledicon *canvas.Circle
	label   *widget.Label

	content *fyne.Container

	state bool
}

func New(label string) *Widget {
	w := &Widget{
		Text: label,
	}
	w.ExtendBaseWidget(w)

	w.ledicon = &canvas.Circle{FillColor: color.RGBA{0x80, 0x80, 0x80, 0xFF}}
	w.label = widget.NewLabel(label)

	return w
}

func (w *Widget) On() {
	w.setState(true)
}

func (w *Widget) Off() {
	w.setState(false)
}

func (w *Widget) setState(state bool) {
	if state == w.state {
		return
	}
	if state {
		w.ledicon.FillColor = color.RGBA{0x00, 0xFF, 0x00, 0xFF}
	} else {
		w.ledicon.FillColor = color.RGBA{0x80, 0x80, 0x80, 0xFF}
	}
	w.state = state
	w.ledicon.Refresh()
}

func (w *Widget) CreateRenderer() fyne.WidgetRenderer {
	w.content = container.NewBorder(
		nil,
		nil,
		w.ledicon,
		nil,
		w.label,
	)
	//return widget.NewSimpleRenderer(w.content)
	return &iconRenderer{w: w}

}

var _ fyne.WidgetRenderer = (*iconRenderer)(nil)

type iconRenderer struct {
	w *Widget
}

func (r *iconRenderer) MinSize() fyne.Size {
	return fyne.NewSize(20+r.w.label.MinSize().Width, 34)
}

func (r *iconRenderer) Layout(size fyne.Size) {
	r.w.ledicon.Resize(fyne.NewSize(20, 20))
	r.w.ledicon.Move(fyne.NewPos(0, 8))
	r.w.label.Move(fyne.NewPos(20, 0))
	r.w.label.Resize(fyne.NewSize(size.Width-20, 32))
}

func (r *iconRenderer) Refresh() {
	r.w.label.Text = r.w.Text
	r.w.label.Refresh()
}

func (r *iconRenderer) Destroy() {
}

func (r *iconRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.w.ledicon, r.w.label}
}
