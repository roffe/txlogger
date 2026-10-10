package common

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

var zonedOut = fyne.NewStaticResource("zonedout.png", assets.ZonedOut)

func ShowError(title string, err error) {
	image := canvas.NewImageFromResource(zonedOut)
	image.FillMode = canvas.ImageFillContain
	image.SetMinSize(fyne.NewSize(121, 207))
	label := widget.NewLabel(err.Error())
	label.Alignment = fyne.TextAlignCenter
	cont := container.NewBorder(image, nil, nil, nil, label)
	dialog.NewCustom(title, "OK", cont, fyne.CurrentApp().Driver().AllWindows()[0]).Show()
}
