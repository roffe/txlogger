package ebusmonitor

import (
	"fmt"
	"sort"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/colors"
)

type Widget struct {
	widget.BaseWidget

	text      *widget.Label
	container *fyne.Container

	values map[string]*canvas.Text
	order  []string

	mu sync.Mutex
}

func New() *Widget {
	t := &Widget{
		text:   widget.NewLabel("test"),
		values: make(map[string]*canvas.Text),
	}
	t.ExtendBaseWidget(t)

	return t.render()
}

func (t *Widget) render() *Widget {
	t.container = container.NewAdaptiveGrid(4)
	return t
}

func (t *Widget) SetText(value string, data float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if itm, ok := t.values[value]; !ok {
		t.values[value] = canvas.NewText(fmt.Sprintf("%s: %.3g", value, data), colors.GetColor(value))
		t.order = append(t.order, value)
		sort.Strings(t.order)

		objs := make([]fyne.CanvasObject, 0, len(t.values))
		for _, v := range t.order {
			objs = append(objs, t.values[v])
		}

		t.container.Objects = objs
		t.container.Refresh()
		return
	} else {
		itm.Text = fmt.Sprintf("%s: %g", value, data)
		itm.Refresh()
	}
}

func (t *Widget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
	// return &WidgetRenderer{t: t}
}

type WidgetRenderer struct {
	t    *Widget
	size fyne.Size
}

func (tr *WidgetRenderer) Layout(space fyne.Size) {
	if tr.size == space {
		return
	}
	tr.size = space
	tr.t.container.Resize(space)
}

func (tr *WidgetRenderer) MinSize() fyne.Size {
	return fyne.NewSize(350, 175)
}

func (tr *WidgetRenderer) Refresh() {

}

func (tr *WidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *WidgetRenderer) Destroy() {
}
