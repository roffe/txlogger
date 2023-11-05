package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type PopupMenu struct {
	widget.BaseWidget
	content *fyne.Container
}

func NewPopupMenu() fyne.Widget {
	p := &PopupMenu{
		content: container.NewWithoutLayout(),
	}

	background := canvas.NewRectangle(color.RGBA{70, 140, 0, 255})
	background.Resize(fyne.NewSize(100, 200))

	hello := widget.NewLabel("Hello")

	p.content.Objects = append(p.content.Objects, background, hello)

	p.ExtendBaseWidget(p)
	return p
}

func (p *PopupMenu) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}
