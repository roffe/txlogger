package plotter

import (
	"image/color"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/lusingander/colorpicker"
)

type TappableText struct {
	widget.BaseWidget
	container     *fyne.Container
	text          *canvas.Text
	value         *canvas.Text
	enabled       bool
	onTapped      func(bool)
	onColorUpdate func(col color.Color)
	onHover       func(bool)
	color         color.Color

	oldSize fyne.Size
}

func NewTappableText(text string, col color.Color, onTapped func(enabled bool), onColorUpdate func(col color.Color), onHover func(bool)) *TappableText {
	tt := &TappableText{
		container:     container.NewWithoutLayout(),
		text:          canvas.NewText(text, col),
		value:         canvas.NewText("0", col),
		enabled:       true,
		onTapped:      onTapped,
		onColorUpdate: onColorUpdate,
		onHover:       onHover,
		color:         col,
	}

	if tt.onTapped == nil {
		tt.onTapped = func(enabled bool) {
			log.Println("Tapped", enabled)
		}
	}

	if tt.onColorUpdate == nil {
		tt.onColorUpdate = func(col color.Color) {
			log.Println("ColorUpdate", col)
		}
	}

	if tt.onHover == nil {
		tt.onHover = func(hover bool) {
			log.Println("Hover", hover)
		}
	}

	tt.text.TextSize = 14
	tt.text.TextStyle = fyne.TextStyle{Bold: false, Italic: false}

	tt.value.TextSize = 14
	tt.value.TextStyle = fyne.TextStyle{Bold: false, Italic: false}

	tt.container.Add(tt.value)
	tt.container.Add(tt.text)

	tt.ExtendBaseWidget(tt)
	return tt
}

func (tt *TappableText) Refresh() {
	tt.value.Refresh()
	tt.text.Refresh()
}

func (tt *TappableText) MouseIn(e *desktop.MouseEvent) {
	tt.onHover(true)
}

func (tt *TappableText) MouseMoved(e *desktop.MouseEvent) {
	//log.Println("MouseMoved", e)
}

func (tt *TappableText) MouseOut() {
	tt.onHover(false)
}

func (tt *TappableText) Enable() {
	tt.enabled = true
	tt.text.Color = tt.color
	tt.value.Color = tt.color
	tt.text.TextStyle = fyne.TextStyle{Bold: false, Italic: false}
	tt.value.TextStyle = fyne.TextStyle{Bold: false, Italic: false}
	tt.text.Refresh()
}

func (tt *TappableText) Disable() {
	tt.enabled = false
	tt.text.TextStyle = fyne.TextStyle{Bold: false, Italic: true}
	tt.value.TextStyle = fyne.TextStyle{Bold: false, Italic: true}
	tt.text.Color = color.RGBA{128, 128, 128, 255}
	tt.value.Color = color.RGBA{128, 128, 128, 255}
	tt.text.Refresh()
}

func (tt *TappableText) Tapped(*fyne.PointEvent) {
	if tt.enabled {
		tt.Disable()
	} else {
		tt.Enable()
	}
	tt.onTapped(tt.enabled)
}

func (tt *TappableText) TappedSecondary(*fyne.PointEvent) {
	picker := colorpicker.New(250, colorpicker.StyleHueCircle)
	picker.SetOnChanged(func(c color.Color) {
		tt.color = c
		if tt.enabled {
			tt.text.Color = c
			tt.text.Refresh()
		}
		tt.onColorUpdate(c)
	})

	canvas := fyne.CurrentApp().Driver().CanvasForObject(tt.text)

	var modal *widget.PopUp
	modal = widget.NewModalPopUp(container.NewVBox(
		picker,
		widget.NewButton("Close", func() {
			modal.Hide()
		}),
	), canvas)
	modal.Show()
}

func (tt *TappableText) CreateRenderer() fyne.WidgetRenderer {
	return &TappableTextRenderer{tt}
}

type TappableTextRenderer struct {
	t *TappableText
}

func (tr *TappableTextRenderer) Layout(size fyne.Size) {
	if tr.t.oldSize == size {
		return
	}
	tr.t.oldSize = size
	tr.t.value.Move(fyne.NewPos(0, 0))
	tr.t.text.Move(fyne.NewPos(60, 0))
}

func (tr *TappableTextRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 18)
}

func (tr *TappableTextRenderer) Refresh() {
	tr.t.value.Refresh()

}

func (tr *TappableTextRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.value, tr.t.text}
}

func (tr *TappableTextRenderer) Destroy() {
}
