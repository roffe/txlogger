package secrettext

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/assets"
)

var _ fyne.Tappable = (*SecretText)(nil)

type SecretText struct {
	*widget.Label
	tappedTimes int
	SecretFunc  func()
}

func New(text string) *SecretText {
	label := widget.NewLabel(text)
	return &SecretText{
		Label: label,
	}
}

func (s *SecretText) Tapped(*fyne.PointEvent) {
	s.tappedTimes++
	//	log.Println("tapped", s.tappedTimes)
	if s.tappedTimes >= 10 {
		t := fyne.NewStaticResource("taz.png", assets.Taz)
		cv := canvas.NewImageFromResource(t)
		cv.ScaleMode = canvas.ImageScaleFastest
		cv.SetMinSize(fyne.NewSize(0, 0))
		cont := container.NewStack(cv)
		s.tappedTimes = 0
		if f := s.SecretFunc; f != nil {
			f()
		}
		dialog.ShowCustom("You found the secret", "Leif", cont, fyne.CurrentApp().Driver().AllWindows()[0])
		an := canvas.NewSizeAnimation(fyne.NewSize(0, 0), fyne.NewSize(370, 386), time.Second, func(size fyne.Size) {
			cv.Resize(size)
		})

		an.Start()
	}
}
