package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Test struct {
	widget.BaseWidget

	minsize fyne.Size
	value   float64

	text      *widget.Label
	container *fyne.Container
}

func NewTest(minSize fyne.Size) *Test {
	t := &Test{
		minsize: minSize,
		text:    widget.NewLabel("test"),
	}
	t.ExtendBaseWidget(t)

	t.container = container.NewWithoutLayout(t.text)
	return t.render()
}

func (t *Test) render() *Test {

	return t
}

func (t *Test) CreateRenderer() fyne.WidgetRenderer {
	return &TestRenderer{
		t: t,
	}
}

func (t *Test) SetValue(value float64) {
	t.value = value
}

type TestRenderer struct {
	t *Test
}

func (tr *TestRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
	// do stuff
}

func (tr *TestRenderer) MinSize() fyne.Size {
	return tr.t.minsize
}

func (tr *TestRenderer) Refresh() {

}

func (tr *TestRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *TestRenderer) Destroy() {
}
