package progressmodal

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

type ProgressModal struct {
	*widget.PopUp
	pb *widget.ProgressBarInfinite
}

func New(c fyne.Canvas, message string) *ProgressModal {
	bobrK := canvas.NewImageFromResource(fyne.NewStaticResource("bobr.jpg", assets.Bobr))
	bobrK.SetMinSize(fyne.NewSize(150, 150))
	bobrK.FillMode = canvas.ImageFillOriginal
	bobrK.ScaleMode = canvas.ImageScaleFastest
	pb := widget.NewProgressBarInfinite()
	msg := container.NewBorder(bobrK, pb, nil, nil, widget.NewLabel(message))
	return &ProgressModal{
		PopUp: widget.NewModalPopUp(msg, c),
		pb:    pb,
	}
}

func (pm *ProgressModal) Stop() {
	pm.pb.Stop()
}

func (pm *ProgressModal) Show() {
	pm.pb.Start()
	pm.PopUp.Show()
}

func (pm *ProgressModal) Hide() {
	pm.pb.Stop()
	pm.PopUp.Hide()
}
