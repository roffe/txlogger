package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Test struct {
	layout  *TestLayout
	minsize fyne.Size
	value   float64

	text *widget.Label

	fyne.CanvasObject
}

func NewTest(minSize fyne.Size) *Test {
	t := &Test{
		minsize: minSize,
		text:    widget.NewLabel("Test Label"),
	}
	t.layout = &TestLayout{t}
	return t.render()
}

func (t *Test) render() *Test {
	customLayout := container.NewWithoutLayout()
	customLayout.Objects = []fyne.CanvasObject{
		t.text,
	}
	customLayout.Layout = t.layout
	t.CanvasObject = customLayout
	return t
}

func (t *Test) CreateRenderer() fyne.WidgetRenderer {
	return &TestRenderer{t}
}

func (t *Test) SetValue(value float64) {
	t.value = value
}

type TestLayout struct {
	*Test
}

func (t *TestLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return t.minsize
}

func (t *TestLayout) Layout(_ []fyne.CanvasObject, space fyne.Size) {
	// move components around here
	t.text.Move(fyne.NewPos(space.Width/2, space.Height/2))
}

type TestRenderer struct {
	*Test
}

func (t *TestRenderer) Layout(space fyne.Size) {
	t.layout.Layout(nil, space)
}

func (t *TestRenderer) MinSize() fyne.Size {
	return t.minsize
}

func (t *TestRenderer) Refresh() {

}

func (t *TestRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{t.CanvasObject}
}

func (t *TestRenderer) Destroy() {
}
