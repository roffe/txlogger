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
	text          *canvas.Text
	enabled       bool
	onTapped      func(bool)
	onColorUpdate func(col color.Color)
	onHover       func(bool)
	color         color.Color
}

func NewTappableText(text string, col color.Color, onTapped func(enabled bool), onColorUpdate func(col color.Color), onHover func(bool)) *TappableText {
	tt := &TappableText{
		text:          canvas.NewText(text, col),
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

	tt.text.TextStyle = fyne.TextStyle{Bold: false, Italic: false}
	tt.ExtendBaseWidget(tt)
	return tt
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
	tt.text.TextStyle = fyne.TextStyle{Bold: false, Italic: false}
	tt.text.Refresh()
}

func (tt *TappableText) Disable() {
	tt.enabled = false
	tt.text.TextStyle = fyne.TextStyle{Bold: false, Italic: true}
	tt.text.Color = color.RGBA{128, 128, 128, 255}
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

func (tt *TappableText) SetTextSize(size int) {
	tt.text.TextSize = float32(size)
}

func (tt *TappableText) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(tt.text)
}
