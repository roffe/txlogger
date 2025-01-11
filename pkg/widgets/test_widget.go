package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type TestWidget struct {
	widget.BaseWidget

	minsize fyne.Size
	value   float64

	text      *widget.Label
	container *fyne.Container
}

func NewTest(minSize fyne.Size) *TestWidget {
	t := &TestWidget{
		minsize: minSize,
		text:    widget.NewLabel("test"),
	}
	t.ExtendBaseWidget(t)
	return t.render()
}

func (t *TestWidget) render() *TestWidget {
	t.container = container.NewStack(t.text)
	return t
}

func (t *TestWidget) CreateRenderer() fyne.WidgetRenderer {
	return &TestWidgetRenderer{
		t: t,
	}
}

func (t *TestWidget) SetValue(value float64) {
	t.value = value
}

func (t *TestWidget) SetValue2(value float64) {
	t.value = value
}

type TestWidgetRenderer struct {
	t *TestWidget
}

func (tr *TestWidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
	// do stuff
}

func (tr *TestWidgetRenderer) MinSize() fyne.Size {
	return tr.t.minsize
}

func (tr *TestWidgetRenderer) Refresh() {

}

func (tr *TestWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *TestWidgetRenderer) Destroy() {
}
