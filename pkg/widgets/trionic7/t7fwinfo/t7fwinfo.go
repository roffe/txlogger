package t7fwinfo

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
)

var _ desktop.Mouseable = (*Widget)(nil)

type Widget struct {
	fw *symbol.T7File
	widget.BaseWidget
	container *fyne.Container
}

func New(fw *symbol.T7File) *Widget {
	t := &Widget{
		fw: fw,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *Widget) CreateRenderer() fyne.WidgetRenderer {

	fwinfo := t.fw.GetInfo()

	engineType := widget.NewEntry()
	engineType.SetText(fwinfo.EngineType)

	softwareVersion := widget.NewEntry()
	softwareVersion.SetText(fwinfo.SoftwareVersion)

	partnumber := widget.NewEntry()
	partnumber.SetText(fwinfo.Partnumber)

	immoCode := widget.NewEntry()
	immoCode.SetText(fwinfo.ImmobilizerCode)

	chassisID := widget.NewEntry()
	chassisID.SetText(fwinfo.ChassisID)

	isBioPower := widget.NewCheck("BioPower", func(b bool) {})
	isBioPower.SetChecked(fwinfo.BioPowerEnabled)

	t.container = container.NewVBox(
		container.NewBorder(
			nil,
			nil,
			container.NewVBox(
				widget.NewLabel("Engine type"),
				widget.NewLabel("Software version"),
				widget.NewLabel("Partnumber"),
				widget.NewLabel("Immobilizer code"),
				widget.NewLabel("Chassis ID"),
				widget.NewLabel("Original cartype"),
				widget.NewLabel("Original enginetype"),
				widget.NewLabel("Programming date"),
				widget.NewLabel("SID date"),
			),
			nil,
			container.NewVBox(
				engineType,
				softwareVersion,
				partnumber,
				immoCode,
				chassisID,
				widget.NewEntry(),
				widget.NewEntry(),
				widget.NewEntry(),
				widget.NewEntry(),
			),
		),

		container.NewGridWithColumns(3,
			widget.NewCheck("Checksum enabled", func(b bool) {}),
			widget.NewCheck("Compressed symboltable", func(b bool) {}),
			widget.NewCheck("No symboltable present", func(b bool) {}),
		),
		layout.NewSpacer(),
		widget.NewLabel("Options"),
		container.NewGridWithColumns(3,
			widget.NewCheck("Open SID info", func(b bool) {}),
			widget.NewCheck("Second lambda sonde", func(b bool) {}),
			widget.NewCheck("Fast throttle response", func(b bool) {}),

			widget.NewCheck("Torque limiters", func(b bool) {}),
			widget.NewCheck("OBDII functions", func(b bool) {}),
			widget.NewCheck("Extra fast throttle response", func(b bool) {}),

			widget.NewCheck("Catalyst lightoff", func(b bool) {}),
			isBioPower,
			widget.NewCheck("Disable emission limiting", func(b bool) {}),

			widget.NewCheck("No TCS", func(b bool) {}),
		),
		widget.NewLabel("Advaced SID options"),
		container.NewGridWithColumns(3,
			container.NewBorder(nil, nil, widget.NewCheck("", func(b bool) {}), nil, widget.NewLabel("Disable startscreen")),
			container.NewBorder(nil, nil, widget.NewCheck("", func(b bool) {}), nil, widget.NewLabel("Disable adaptation message")),
		),
	)

	return &WidgetRenderer{
		t: t,
	}
}

func (t *Widget) MouseDown(e *desktop.MouseEvent) {
}

func (t *Widget) MouseUp(e *desktop.MouseEvent) {
}

type WidgetRenderer struct {
	t *Widget
}

func (tr *WidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
	// do stuff
}

func (tr *WidgetRenderer) MinSize() fyne.Size {
	return tr.t.container.MinSize()
}

func (tr *WidgetRenderer) Refresh() {

}

func (tr *WidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *WidgetRenderer) Destroy() {
}
