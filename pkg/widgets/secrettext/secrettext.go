package secrettext

import (
	"bytes"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/hajimehoshi/go-mp3"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/sound"
)

var _ fyne.Tappable = (*SecretText)(nil)

type SecretText struct {
	*widget.Label
	tappedTimes int
	SecretFunc  func()
	initOnce    sync.Once
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

		fileBytesReader := bytes.NewReader(assets.Korvring)

		// Decode file
		decodedMp3, err := mp3.NewDecoder(fileBytesReader)
		if err != nil {
			panic("mp3.NewDecoder failed: " + err.Error())
		}

		player := sound.NewPlayer(decodedMp3)

		player.Play()

		s.tappedTimes = 0
		if f := s.SecretFunc; f != nil {
			f()
		}

		cont := container.NewStack(cv)
		d := dialog.NewCustom("You found the secret", "Leif", cont, fyne.CurrentApp().Driver().AllWindows()[0])
		d.SetOnClosed(func() {
			player.Pause()
		})
		d.Show()
		an := canvas.NewSizeAnimation(fyne.NewSize(0, 0), fyne.NewSize(370, 386), time.Second, func(size fyne.Size) {
			cv.Resize(size)
		})

		an.Start()
	}
}

func (s *SecretText) Cursor() desktop.Cursor {
	return desktop.CrosshairCursor
}
