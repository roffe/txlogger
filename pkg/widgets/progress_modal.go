package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

type ProgressModal struct {
	p  *widget.PopUp
	pb *widget.ProgressBarInfinite
}

func NewProgressModal(obj fyne.CanvasObject, message string) *ProgressModal {
	bobrK := canvas.NewImageFromResource(fyne.NewStaticResource("bobr.jpg", assets.Bobr))
	bobrK.SetMinSize(fyne.NewSize(150, 150))
	bobrK.FillMode = canvas.ImageFillOriginal
	bobrK.ScaleMode = canvas.ImageScalePixels
	pb := widget.NewProgressBarInfinite()
	msg := container.NewBorder(bobrK, pb, nil, nil, widget.NewLabel(message))
	return &ProgressModal{
		p:  widget.NewModalPopUp(msg, fyne.CurrentApp().Driver().CanvasForObject(obj)),
		pb: pb,
	}
}

func (pm *ProgressModal) Show() {
	pm.pb.Start()
	pm.p.Show()
}

func (pm *ProgressModal) Hide() {
	pm.pb.Stop()
	pm.p.Hide()
}
