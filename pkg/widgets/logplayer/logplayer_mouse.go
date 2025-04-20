package logplayer

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

var _ desktop.Mouseable = (*Logplayer)(nil)

func (l *Logplayer) Tapped(_ *fyne.PointEvent) {
	// fyne.CurrentApp().Driver().CanvasForObject(l).Focus(l)
	//l.objs.playbackToggleBtn.OnTapped()
}

func (l *Logplayer) MouseUp(event *desktop.MouseEvent) {
}

func (l *Logplayer) MouseDown(event *desktop.MouseEvent) {
	if !l.focused {
		if f := l.OnMouseDown; f != nil {
			f()
		}
	}
}
