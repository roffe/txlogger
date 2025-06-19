package settings

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type LoggingSettingsWidget struct {
	widget.BaseWidget

	container *fyne.Container
}

func NewTest(minSize fyne.Size) *LoggingSettingsWidget {
	t := &LoggingSettingsWidget{}
	t.ExtendBaseWidget(t)
	t.render()
	return t
}

func (t *LoggingSettingsWidget) render() {
	t.container = container.NewStack()
}

func (t *LoggingSettingsWidget) CreateRenderer() fyne.WidgetRenderer {
	return &LoggingSettingsWidgetRenderer{
		t: t,
	}
}

type LoggingSettingsWidgetRenderer struct {
	t *LoggingSettingsWidget
}

func (tr *LoggingSettingsWidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
	// do stuff
}

func (tr *LoggingSettingsWidgetRenderer) MinSize() fyne.Size {
	return tr.t.container.MinSize()
}

func (tr *LoggingSettingsWidgetRenderer) Refresh() {

}

func (tr *LoggingSettingsWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *LoggingSettingsWidgetRenderer) Destroy() {
}
