package common

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

var (
	zonedOut      = fyne.NewStaticResource("zonedout.png", assets.ZonedOut)
	zonedOutImage = canvas.NewImageFromResource(zonedOut)
)

func init() {
	zonedOutImage.FillMode = canvas.ImageFillContain
	zonedOutImage.SetMinSize(fyne.NewSize(121, 207))
	zonedOutImage.Resize(fyne.NewSize(121, 207))
}

func ShowError(title string, err error) {
	label := widget.NewLabel(err.Error())
	label.Alignment = fyne.TextAlignCenter
	cont := container.NewBorder(zonedOutImage, nil, nil, nil, label)
	dialog.NewCustom(title, "OK", cont, fyne.CurrentApp().Driver().AllWindows()[0]).Show()
}
