package tappabletext

import (
	"bytes"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/hajimehoshi/go-mp3"
	"github.com/roffe/txlogger/pkg/assets"
	"github.com/roffe/txlogger/pkg/sound"
	"github.com/roffe/txlogger/pkg/widgets/tunnel"
)

var _ fyne.Tappable = (*SecretText)(nil)

type SecretText struct {
	*widget.Label
	tappedTimes int
	Taps        int
	Func        func()
}

func New(text string, taps int, fn func()) *SecretText {
	label := widget.NewLabel(text)
	return &SecretText{
		Label: label,
		Taps:  taps,
		Func:  fn,
	}
}

func (s *SecretText) Tapped(*fyne.PointEvent) {
	s.tappedTimes++
	//	log.Println("tapped", s.tappedTimes)
	if s.tappedTimes >= s.Taps {
		s.tappedTimes = 0

		fileBytesReader := bytes.NewReader(assets.Korvring)
		decodedMp3, err := mp3.NewDecoder(fileBytesReader)
		if err != nil {
			panic("mp3.NewDecoder failed: " + err.Error())
		}

		player := sound.NewPlayer(decodedMp3)

		player.Play()

		if f := s.Func; f != nil {
			f()
		}

		t := tunnel.New()
		t.SetCredits([]string{
			"SAAB",
			"MattiasC",
			"Dilemma",
			"J.K Nilsson",
			"Manick",
			"Artursson",
			"Schottis",
			"Chriva",
			"Myrtilos",
			"Mackan",
			"Kalej",
			"Bojer",
			"TrionicTuning",
			"o2o Crew",
		})

		d := dialog.NewCustom("You found the secret", "Leif", t, fyne.CurrentApp().Driver().AllWindows()[0])
		d.SetOnClosed(func() {
			player.Pause()
		})
		d.Show()
	}
}

//func (s *SecretText) Cursor() desktop.Cursor {
//	return desktop.CrosshairCursor
//}
