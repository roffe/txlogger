package t7esp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	symbol "github.com/roffe/ecusymbol"
)

var _ desktop.Mouseable = (*Widget)(nil)

var (
	espTypes = [...]string{
		"15 inch brakes TCS/ESP B205",
		"15/16 inch brakes TCS/ESP B235E MY07 -> and B235R MY00-01 ",
		"16 inch brakes ESP B235L",
		"16+ inch brakes ESP B235R MY02 ->",
	}
	espTypeMap = map[int]byte{
		0: 0x82,
		1: 0x90,
		2: 0x91,
		3: 0x92,
	}
	espIdxMap = map[byte]int{
		0x82: 0,
		0x90: 1,
		0x91: 2,
		0x92: 3,
	}
)

type Widget struct {
	widget.BaseWidget
	espSelect *widget.Select

	container *fyne.Container

	filename string
	fw       *symbol.T7File
}

func New(filename string, fw *symbol.T7File) *Widget {
	t := &Widget{
		filename: filename,
		fw:       fw,
	}
	t.ExtendBaseWidget(t)

	return t
}

func (t *Widget) MouseDown(e *desktop.MouseEvent) {
}

func (t *Widget) MouseUp(e *desktop.MouseEvent) {
}

func (t *Widget) GetCalibration() byte {
	return espTypeMap[t.espSelect.SelectedIndex()]
}

func (t *Widget) CreateRenderer() fyne.WidgetRenderer {
	t.espSelect = widget.NewSelect(espTypes[:], nil)
	t.espSelect.SetSelectedIndex(espIdxMap[t.fw.GetESPCalibration()])

	title := widget.NewLabel("After chaging this you need to reflash your ECU")
	description := widget.NewLabel("Select ESP Calibration")
	saveButton := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		t.fw.SetESPCalibration(t.GetCalibration())
		if err := t.fw.Save(t.filename); err != nil {
			dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		}
	})

	t.container = container.NewBorder(
		title,
		saveButton,
		description,
		nil,
		t.espSelect,
	)
	return &TestWidgetRenderer{
		t: t,
	}
}

type TestWidgetRenderer struct {
	t *Widget
}

func (tr *TestWidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
}

func (tr *TestWidgetRenderer) MinSize() fyne.Size {
	return fyne.Size{Width: 550, Height: 116}
}

func (tr *TestWidgetRenderer) Refresh() {

}

func (tr *TestWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *TestWidgetRenderer) Destroy() {
}
