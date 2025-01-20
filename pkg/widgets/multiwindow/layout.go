package multiwindow

import "fyne.io/fyne/v2"

type multiWinLayout struct {
}

func (m *multiWinLayout) Layout(objects []fyne.CanvasObject, _ fyne.Size) {
	for _, w := range objects { // update the windows so they have real size
		w.Resize(w.MinSize().Max(w.Size()))
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
