package logplayer

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (l *Logplayer) Tapped(_ *fyne.PointEvent) {
	log.Println("Tapped")
	fyne.CurrentApp().Driver().CanvasForObject(l).Focus(l)
}

func (l *Logplayer) MouseUp(event *desktop.MouseEvent) {
}

func (l *Logplayer) MouseDown(event *desktop.MouseEvent) {
	if f := l.OnMouseDown; f != nil {
		f()
	}
}
