package multiwindow

import "fyne.io/fyne/v2"

type multiWinLayout struct {
	mw *MultipleWindows
}

func (m *multiWinLayout) Layout(objects []fyne.CanvasObject, _ fyne.Size) {
	for _, w := range objects { // update the windows so they have real size
		w.Resize(w.MinSize().Max(w.Size()))
	}
	if m.mw != nil {
		m.mw.layoutTray() // keep minimized windows docked to the bottom on resize
	}
}

func (m *multiWinLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.Size{Width: 700, Height: 400}
}

func clamp32(value, min, max float32) float32 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
