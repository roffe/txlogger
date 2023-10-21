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

	text string
}

func NewTest(minSize fyne.Size) *Test {
	t := &Test{
		minsize: minSize,
		text:    "hejsan!",
	}
	t.ExtendBaseWidget(t)
	return t.render()
}

func (t *Test) render() *Test {

	return t
}

func (t *Test) CreateRenderer() fyne.WidgetRenderer {
	text := widget.NewLabel(t.text)

	con := container.NewWithoutLayout(text)

	return &TestRenderer{
		container: con,
		text:      text,
		t:         t,
	}
}

func (t *Test) SetValue(value float64) {
	t.value = value
}

type TestRenderer struct {
	t         *Test
	text      *widget.Label
	container *fyne.Container
}

func (tr *TestRenderer) Layout(space fyne.Size) {

}

func (tr *TestRenderer) MinSize() fyne.Size {
	return tr.t.minsize
}

func (tr *TestRenderer) Refresh() {

}

func (tr *TestRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.container}
}

func (tr *TestRenderer) Destroy() {
}
